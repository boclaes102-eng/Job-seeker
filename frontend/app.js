"use strict";

const $ = (sel) => document.querySelector(sel);
const esc = (s) =>
  String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

const STATUS_LABELS = {
  new: "New", saved: "Saved", applied: "Applied", interview: "Interview", rejected: "Rejected", hidden: "Hidden",
};
const REC_LABELS = { strong_apply: "Strong match", apply: "Good match", maybe: "Worth a look", skip: "Weak match" };
// Order and weight of the score parts, matching jobseeker/matching.py
const PARTS = [["role", "Role", 35], ["skills", "Skills", 30], ["location", "Distance", 15],
  ["coverage", "Coverage", 10], ["freshness", "Freshness", 10]];

let appStatus = null;
let pollTimer = null;
let minGrade = "65";
let statusFilter = "";
let onlyNew = false;
let openId = null;

// ------------------------------------------------------------------ helpers
async function api(path, options = {}) {
  const res = await fetch(path, {
    headers: { "Content-Type": "application/json" },
    ...options,
    body: options.body ? JSON.stringify(options.body) : undefined,
  });
  if (!res.ok) {
    let msg = `${res.status} ${res.statusText}`;
    try { msg = (await res.json()).detail || msg; } catch (_) { /* not json */ }
    throw new Error(msg);
  }
  return res.json();
}

function toast(msg, isError = false) {
  const t = $("#toast");
  t.textContent = msg;
  t.className = "toast" + (isError ? " error" : "");
  t.hidden = false;
  clearTimeout(t._timer);
  t._timer = setTimeout(() => { t.hidden = true; }, isError ? 7000 : 3000);
}

function band(g) {
  if (g === null || g === undefined) return "s-weak";
  return g >= 80 ? "s-strong" : g >= 65 ? "s-good" : g >= 45 ? "s-fair" : "s-weak";
}

// The gauge: four segments sized by weight, each filled by its part score.
function gauge(parts, cls = "") {
  if (!parts) return `<div class="gauge empty ${cls}"></div>`;
  const label = PARTS.map(([k, name, w]) => `${name} ${Math.round((parts[k] ?? 0) * w)} of ${w}`).join(", ");
  return `<div class="gauge ${cls}" role="img" aria-label="${label}">${
    PARTS.map(([k, , w]) => `<span style="flex:${w}"><i style="width:${Math.round((parts[k] ?? 0) * 100)}%"></i></span>`).join("")
  }</div>`;
}

function scoreBlock(job) {
  const g = job.grade_details;
  return `<div class="score ${band(job.grade)}">
    <b>${job.grade ?? "–"}</b>
    ${gauge(g?.parts)}
    <span class="rec">${g ? esc(REC_LABELS[g.recommendation]) : "Not scored"}</span>
  </div>`;
}

function sourceLabel(name) {
  if (name === "manual") return "Added by you";
  return appStatus?.sources?.[name]?.label?.replace(/ \(.*\)$/, "") || name;
}

// "Apply on Jooble", or the site's own name for a job you added yourself
function applyLabel(job) {
  if (job.source !== "manual") return `Apply on ${sourceLabel(job.source)}`;
  try { return `Apply on ${new URL(job.url).hostname.replace(/^www\./, "")}`; } catch (_) { return "Open posting"; }
}

const isNewToYou = (j) => Boolean(appStatus?.seen_until && j.fetched_at > appStatus.seen_until);

function ago(iso) {
  if (!iso) return "";
  const days = Math.round((Date.now() - new Date(iso).getTime()) / 864e5);
  if (Number.isNaN(days)) return iso;
  return days <= 0 ? "today" : days === 1 ? "yesterday" : days < 30 ? `${days} days ago` : iso;
}

// Tracking dates are local calendar dates ("2026-11-06") or times ("2026-11-06T14:00").
function localDate(iso) {
  const [y, m, d] = iso.slice(0, 10).split("-").map(Number);
  return new Date(y, m - 1, d);
}
function daysFrom(iso) {  // whole days from today; negative is in the past
  const today = new Date(); today.setHours(0, 0, 0, 0);
  return Math.round((localDate(iso) - today) / 864e5);
}
function rel(iso) {
  const n = daysFrom(iso);
  return n === 0 ? "today" : n === 1 ? "tomorrow" : n === -1 ? "yesterday" : n > 0 ? `in ${n} days` : `${-n} days ago`;
}
function fmtWhen(iso) {
  const day = localDate(iso).toLocaleDateString("en-GB", { weekday: "short", day: "numeric", month: "short" });
  return iso.length > 10 ? `${day}, ${iso.slice(11, 16)}` : day;
}
const isTracked = (j) => j.status === "applied" || j.status === "interview";

function trackLine(j) {
  if (j.status === "interview" && j.interview_at) {
    return `<div class="track">Interview ${esc(fmtWhen(j.interview_at))}, ${rel(j.interview_at)}</div>`;
  }
  if (!isTracked(j)) return "";
  const bits = [];
  if (j.applied_on) bits.push(`Applied ${rel(j.applied_on)}`);
  if (j.follow_up_on) {
    bits.push(daysFrom(j.follow_up_on) <= 0 ? `<b class="due">follow-up due</b>` : `follow up ${rel(j.follow_up_on)}`);
  }
  return bits.length ? `<div class="track">${bits.join(", ")}</div>` : "";
}

function skillList(items, cls, limit = 99) {
  return items.slice(0, limit).map((s) => `<span class="sk ${cls}">${esc(s)}</span>`).join("");
}

function inlineSkills(g, limit) {
  return `<div class="skills">${
    skillList(g.dealbreakers, "bad") +
    skillList(g.matched_skills, "ok", limit) +
    skillList(g.related_skills || [], "rel", Math.max(1, limit - 3)) +
    skillList(g.missing_skills, "miss", Math.max(1, limit - 3))
  }</div>`;
}

function distanceCell(g) {
  if (!g) return `<div class="dist"></div>`;
  if (g.work_mode === "remote") return `<div class="dist"><b>Remote</b></div>`;
  if (g.distance_km === null || g.distance_km === undefined) {
    return `<div class="dist"><b>?</b>${g.work_mode === "hybrid" ? "Hybrid, town unknown" : "Town unknown"}</div>`;
  }
  return `<div class="dist"><b>${g.distance_km}<small>km</small></b>from ${esc(g.nearest_home || "home")}${g.work_mode === "hybrid" ? ", hybrid" : ""}</div>`;
}

function meta(job, extra = []) {
  const bits = [job.company || "Company not listed", job.location || "Belgium", ...extra];
  return bits.filter(Boolean).map((b) => `<span>${esc(b)}</span>`).join("");
}

// ------------------------------------------------------------------- status
function renderPipeline(counts) {
  const by = counts.by_status || {};
  const visible = counts.total;
  const tabs = [["", "All", visible], ...Object.entries(STATUS_LABELS).map(([k, v]) => [k, v, by[k] ?? 0])];
  $("#pipeline").innerHTML = tabs.map(([k, label, n]) =>
    `<button data-status="${k}" class="${k === statusFilter ? "on" : ""}${n ? "" : " zero"}" aria-pressed="${k === statusFilter}">
       ${label}<span class="n">${n}</span></button>`).join("");
}

async function refreshStatus() {
  appStatus = await api("/api/status");
  const s = appStatus;
  $("#pills").innerHTML = Object.values(s.sources)
    .map((v) => `<span class="src ${v.configured ? "on" : "off"}" title="${v.configured ? "Active" : "Needs an API key in .env"}">${esc(v.label.replace(/ \(.*\)$/, ""))}</span>`)
    .join("");
  $("#counts").innerHTML = s.counts.total
    ? `<span class="num">${s.counts.good}</span> jobs fit your CV well, <span class="num">${s.counts.strong}</span> of them strongly.
       <span class="sub">Out of ${s.counts.total} English and Dutch jobs${s.counts.language_filtered ? `. ${s.counts.language_filtered} French jobs were left out` : ""}.</span>`
    : `No jobs yet. <span class="sub">Search now looks through ictjob.be, Jooble and Le Forem and scores every job against your CV.</span>`;
  renderPipeline(s.counts);
  renderNewLine();

  const run = s.search;
  const prog = $("#progress");
  if (run.running) {
    prog.hidden = false;
    $("#progressText").textContent = run.message || "Searching";
    $("#progressFill").style.width = run.total ? `${Math.round((run.progress / run.total) * 100)}%` : "8%";
    $("#searchBtn").disabled = true;
    $("#searchBtn").innerHTML = `<span class="spinner"></span> Searching`;
  } else {
    $("#searchBtn").disabled = false;
    $("#searchBtn").textContent = "Search now";
    if (run.phase === "done" || run.phase === "failed") {
      prog.hidden = false;
      const errs = run.errors?.length ? ` ${run.errors.length} source warning(s): ${run.errors.slice(0, 2).join(" | ")}` : "";
      $("#progressText").textContent = (run.message || "") + errs;
      $("#progressFill").style.width = "100%";
    }
  }
  return s;
}

function startPolling() {
  clearInterval(pollTimer);
  pollTimer = setInterval(async () => {
    const s = await refreshStatus();
    if (!s.search.running) {
      clearInterval(pollTimer);
      pollTimer = null;
      loadJobs();
      toast(s.search.new_jobs ? `Search finished: ${s.search.new_jobs} new jobs` : "Search finished, no new jobs");
      lastUnseen = s.counts.unseen;
    }
  }, 1500);
}

// ---------------------------------------------------- found since last visit
let lastUnseen = null;

function renderNewLine() {
  const n = appStatus.counts.unseen;
  document.title = n ? `(${n}) Job Seeker` : "Job Seeker";
  if (!n) onlyNew = false;
  const el = $("#newLine");
  el.hidden = !n;
  el.innerHTML = n ? `
    <span><b>${n}</b> ${n === 1 ? "job was" : "jobs were"} found since you last looked.</span>
    <button class="btn quiet" data-act="only" aria-pressed="${onlyNew}">${onlyNew ? "Show all jobs" : "Show only these"}</button>
    <button class="btn quiet" data-act="seen">Mark all as seen</button>` : "";
}

$("#newLine").addEventListener("click", async (ev) => {
  const act = ev.target.closest("[data-act]")?.dataset.act;
  if (act === "only") onlyNew = !onlyNew;
  if (act === "seen") {
    try { await api("/api/seen", { method: "POST" }); } catch (e) { toast(e.message, true); return; }
    onlyNew = false;
    await refreshStatus();
    lastUnseen = 0;
  }
  if (act) { renderNewLine(); loadJobs(); }
});

function notifyEnabled() {
  try { return localStorage.getItem("jobseeker.notify") === "1"; } catch (_) { return false; }
}

// Automatic searches run on the server; check now and then whether they found anything.
async function checkForNewJobs() {
  if (pollTimer || (document.hidden && !notifyEnabled())) return;
  let s;
  try { s = await refreshStatus(); } catch (_) { return; }
  if (s.search.running) { startPolling(); return; }
  const n = s.counts.unseen;
  if (lastUnseen !== null && n > lastUnseen) {
    const added = n - lastUnseen;
    const msg = `${added} new ${added === 1 ? "job" : "jobs"} found`;
    toast(msg);
    if (notifyEnabled() && "Notification" in window && Notification.permission === "granted") {
      new Notification("Job Seeker", { body: `${msg}. ${n} waiting for you to look at.` });
    }
    loadJobs();
  }
  lastUnseen = n;
}
setInterval(checkForNewJobs, 60_000);

$("#searchBtn").addEventListener("click", async () => {
  try {
    const r = await api("/api/search", { method: "POST" });
    if (!r.started) toast("A search is already running");
    await refreshStatus();
    startPolling();
  } catch (e) { toast(e.message, true); }
});

// --------------------------------------------------------------------- tabs
document.querySelectorAll(".nav-link").forEach((btn) =>
  btn.addEventListener("click", () => {
    document.querySelectorAll(".nav-link").forEach((b) => {
      b.classList.toggle("active", b === btn);
      if (b === btn) b.setAttribute("aria-current", "page"); else b.removeAttribute("aria-current");
    });
    document.querySelectorAll(".tab-panel").forEach((p) => { p.hidden = true; });
    $(`#tab-${btn.dataset.tab}`).hidden = false;
    if (btn.dataset.tab === "settings") loadSettings();
    if (btn.dataset.tab === "jobs") loadJobs();
  })
);

// --------------------------------------------------------------------- jobs
let jobsCache = [];

async function loadJobs() {
  const params = new URLSearchParams();
  const q = $("#fQuery").value.trim();
  if (q) params.set("q", q);
  if (minGrade) params.set("min_grade", minGrade);
  if ($("#fSource").value) params.set("source", $("#fSource").value);
  if (statusFilter) params.set("status", statusFilter);
  if (onlyNew && appStatus?.seen_until) params.set("since", appStatus.seen_until);
  params.set("sort", $("#fSort").value);
  try {
    jobsCache = await api(`/api/jobs?${params}`);
  } catch (e) { toast(e.message, true); return; }
  renderJobs();
}

function renderJobs() {
  const list = $("#jobList");
  if (!jobsCache.length) {
    list.innerHTML = (appStatus?.counts?.total ?? 0) === 0
      ? `<p class="empty">Nothing here yet. <b>Search now</b> fetches the latest Belgian jobs and scores each one against your CV.</p>`
      : statusFilter && !minGrade
        ? `<p class="empty">No jobs are marked ${esc(STATUS_LABELS[statusFilter].toLowerCase())} yet.</p>`
        : `<p class="empty">No jobs match these filters. Choose <b>Any score</b> or <b>45+</b> to see more.</p>`;
    return;
  }
  list.innerHTML = jobsCache.map((j) => {
    const g = j.grade_details;
    return `
      <article class="job${j.id === openId ? " selected" : ""}" data-id="${j.id}">
        ${scoreBlock(j)}
        <div class="job-main">
          <h3><button class="job-open" data-act="open">${esc(j.title)}</button>
            ${isNewToYou(j) ? `<span class="tag t-fresh">Just found</span>` : ""}
            ${j.status !== "new" ? `<span class="tag t-${esc(j.status)}">${esc(STATUS_LABELS[j.status] || j.status)}</span>` : ""}</h3>
          <div class="who">${meta(j, [sourceLabel(j.source), ago(j.posted_at)])}</div>
          ${trackLine(j)}
          ${g ? inlineSkills(g, 6) : ""}
        </div>
        ${distanceCell(g)}
        <div class="job-actions">
          ${j.url ? `<a class="btn primary" href="${esc(j.url)}" target="_blank" rel="noopener" data-act="link">${esc(applyLabel(j))}</a>` : ""}
          <button class="btn quiet" data-act="${j.status === "saved" ? "unsave" : "save"}" aria-pressed="${j.status === "saved"}">${j.status === "saved" ? "Saved" : "Save"}</button>
          <button class="btn quiet" data-act="${j.status === "hidden" ? "unhide" : "hide"}">${j.status === "hidden" ? "Unhide" : "Hide"}</button>
        </div>
      </article>`;
  }).join("");
}

$("#jobList").addEventListener("click", async (ev) => {
  const card = ev.target.closest(".job");
  if (!card) return;
  const id = Number(card.dataset.id);
  const act = ev.target.closest("[data-act]")?.dataset.act;
  if (act === "link") return;
  const newStatus = { hide: "hidden", unhide: "new", save: "saved", unsave: "new" }[act];
  if (newStatus) {
    ev.stopPropagation();
    await api(`/api/jobs/${id}`, { method: "PATCH", body: { status: newStatus } });
    toast({ hidden: "Hidden", saved: "Saved", new: "Restored" }[newStatus]);
    loadJobs(); refreshStatus(); loadAgenda();
    return;
  }
  openJob(id);
});

$("#fGradeSeg").addEventListener("click", (e) => {
  const b = e.target.closest("button");
  if (!b) return;
  minGrade = b.dataset.min;
  document.querySelectorAll("#fGradeSeg button").forEach((x) => x.classList.toggle("on", x === b));
  loadJobs();
});

$("#pipeline").addEventListener("click", (e) => {
  const b = e.target.closest("button");
  if (!b) return;
  statusFilter = b.dataset.status;
  // Tracked jobs are few: show them all regardless of score
  if (statusFilter && statusFilter !== "new") {
    minGrade = "";
    document.querySelectorAll("#fGradeSeg button").forEach((x) => x.classList.toggle("on", x.dataset.min === ""));
  }
  renderPipeline(appStatus.counts);
  loadJobs();
});

// ------------------------------------------------------------------- agenda
async function loadAgenda() {
  let a;
  try { a = await api("/api/agenda"); } catch (_) { return; }
  const items = [
    ...a.interviews.map((j) => `
      <li class="ag ag-interview" data-id="${j.id}">
        <span class="ag-when">${esc(fmtWhen(j.interview_at))}</span>
        <span class="ag-body"><button class="ag-what" data-act="open">Interview at ${esc(j.company || j.title)}</button>
          <span class="ag-sub">${esc(j.title)}, ${rel(j.interview_at)}</span></span>
      </li>`),
    ...a.follow_ups.map((j) => `
      <li class="ag ag-follow" data-id="${j.id}">
        <span class="ag-when">${daysFrom(j.follow_up_on) === 0 ? "Due today" : `Due ${rel(j.follow_up_on)}`}</span>
        <span class="ag-body"><button class="ag-what" data-act="open">Follow up with ${esc(j.company || "the company")}</button>
          <span class="ag-sub">${esc(j.title)}${j.applied_on ? `, applied ${rel(j.applied_on)}` : ""}</span></span>
        <button class="btn" data-act="followed">I followed up</button>
      </li>`),
  ];
  const el = $("#agenda");
  el.hidden = !items.length;
  el.innerHTML = items.length ? `<h2>Waiting on you</h2><ul>${items.join("")}</ul>` : "";
}

async function markFollowedUp(id) {
  try {
    const job = await api(`/api/jobs/${id}/followed-up`, { method: "POST" });
    toast(`Noted. Next follow-up ${fmtWhen(job.follow_up_on)}.`);
    loadAgenda(); loadJobs();
    return job;
  } catch (e) { toast(e.message, true); }
}

$("#agenda").addEventListener("click", (ev) => {
  const act = ev.target.closest("[data-act]")?.dataset.act;
  const id = Number(ev.target.closest(".ag")?.dataset.id);
  if (!act || !id) return;
  if (act === "open") openJob(id);
  if (act === "followed") markFollowedUp(id);
});

let filterTimer;
["#fQuery", "#fSource", "#fSort"].forEach((sel) =>
  $(sel).addEventListener("input", () => { clearTimeout(filterTimer); filterTimer = setTimeout(loadJobs, 250); })
);

// ------------------------------------------------------------------- drawer
let lastFocus = null;
function openDrawer(html) {
  if ($("#drawer").hidden) lastFocus = document.activeElement;
  $("#drawerBody").innerHTML = html;
  $("#drawer").hidden = false;
  $("#scrim").hidden = false;
  $("#drawer").scrollTop = 0;
  $("#drawer").focus();
}
function closeDrawer() {
  if ($("#drawer").hidden) return;
  $("#drawer").hidden = true;
  $("#scrim").hidden = true;
  openId = null;
  document.querySelectorAll(".job.selected").forEach((el) => el.classList.remove("selected"));
  lastFocus?.focus?.();
}
$("#drawerClose").addEventListener("click", closeDrawer);
$("#scrim").addEventListener("click", closeDrawer);
document.addEventListener("keydown", (e) => { if (e.key === "Escape") closeDrawer(); });

// Update the tracking inputs from the server copy, leaving the one being typed in alone.
function fillTracking(job) {
  document.querySelectorAll("[data-track]").forEach((el) => {
    if (document.activeElement !== el) el.value = job[el.dataset.track] || "";
  });
  $("#jobStatus").value = job.status;
  $("#followedUp").hidden = !isTracked(job);
}

function savedHint() {
  const el = $("#trackSaved");
  if (!el) return;
  el.textContent = "Saved";
  clearTimeout(el._timer);
  el._timer = setTimeout(() => { el.textContent = ""; }, 2000);
}

async function saveJob(id, body) {
  try {
    const job = await api(`/api/jobs/${id}`, { method: "PATCH", body });
    if (openId === id) fillTracking(job);
    loadJobs(); refreshStatus(); loadAgenda();
    return job;
  } catch (e) { toast(e.message, true); return null; }
}

function breakdown(g, total) {
  const points = PARTS.map(([k, , w]) => Math.round((g.parts[k] ?? 0) * w));
  const adjust = total - points.reduce((a, b) => a + b, 0);
  const rows = PARTS.map(([k, name, w], i) => `
      <span class="label">${name}</span>
      <span class="track"><i style="width:${Math.round((g.parts[k] ?? 0) * 100)}%"></i></span>
      <span class="val"><b>${points[i]}</b> of ${w}</span>`).join("");
  const adj = Math.abs(adjust) >= 2
    ? `<span class="label">Adjusted</span><span class="adj-note">${adjust < 0 ? "Seniority, internship or years asked" : "Junior or medior title"}</span>
       <span class="val"><b>${adjust > 0 ? "+" : "−"}${Math.abs(adjust)}</b></span>` : "";
  return `<div class="breakdown">${rows}${adj}</div>`;
}

async function openJob(id) {
  let job;
  try { job = await api(`/api/jobs/${id}`); } catch (e) { toast(e.message, true); return; }
  openId = id;
  document.querySelectorAll(".job").forEach((el) => el.classList.toggle("selected", Number(el.dataset.id) === id));
  const g = job.grade_details;
  const statusOptions = Object.entries(STATUS_LABELS)
    .map(([k, v]) => `<option value="${k}" ${k === job.status ? "selected" : ""}>${v}</option>`).join("");
  const skillRow = (label, items, cls) => items.length
    ? `<div class="skill-row"><div class="lbl">${label}</div><div class="skills">${skillList(items, cls)}</div></div>` : "";

  openDrawer(`
    <h2>${esc(job.title)}</h2>
    <div class="who">${meta(job, [sourceLabel(job.source), job.contract, job.salary, job.posted_at ? `posted ${ago(job.posted_at)}` : ""])}</div>

    ${g ? `
    <div class="verdict ${band(job.grade)}">
      <b class="big">${job.grade}</b>
      <div>
        ${gauge(g.parts, "wide")}
        <p><b>${esc(REC_LABELS[g.recommendation])}.</b> ${esc(g.summary)}</p>
      </div>
    </div>
    ${breakdown(g, job.grade)}

    <h3 class="sec">Skills in this posting</h3>
    <div class="skill-cols">
      ${skillRow("Problems", g.dealbreakers, "bad")}
      ${skillRow("On your CV", g.matched_skills, "ok")}
      ${skillRow("Related to your CV", g.related_skills || [], "rel")}
      ${skillRow("Not on your CV", g.missing_skills, "miss")}
      ${!g.matched_skills.length && !g.missing_skills.length && !(g.related_skills || []).length
        ? `<p class="hint">The text from the job site names no specific skills. Paste the full posting below for a better score.</p>` : ""}
    </div>` : ""}

    ${job.url ? `<div class="actions">
      <a class="btn primary" href="${esc(job.url)}" target="_blank" rel="noopener">${esc(applyLabel(job))}</a>
    </div>` : ""}

    <h3 class="sec">Your application <span class="hint" id="trackSaved" aria-live="polite"></span></h3>
    <div class="tracking">
      <label>Status<select id="jobStatus">${statusOptions}</select></label>
      <label>Applied on<input type="date" data-track="applied_on"></label>
      <label>Follow up on
        <span class="with-btn"><input type="date" data-track="follow_up_on"><button class="btn" type="button" id="followedUp">I followed up</button></span></label>
      <label>Interview<input type="datetime-local" data-track="interview_at"></label>
      <label class="wide">Contact<input type="text" data-track="contact" placeholder="Recruiter name, email or phone"></label>
      <label class="wide">Notes<textarea data-track="notes" rows="4" placeholder="What you sent, who you spoke to, what they asked"></textarea></label>
    </div>

    <h3 class="sec">Description${job.full_description ? "" : ", short version from the job site"}</h3>
    <div class="desc">${esc(job.full_description || job.description || "No description available.")}</div>

    <h3 class="sec">Get a more accurate score</h3>
    <p class="hint">Job sites only share a snippet. Open the posting and click <b>Send to Job Seeker</b> in your bookmarks bar, or paste the text here. The job is then scored on the complete text.</p>
    <textarea id="fullDesc" rows="6" placeholder="Paste the full job description here" aria-label="Full job description">${esc(job.full_description || "")}</textarea>
    <div class="actions">
      <button class="btn primary" id="rescoreBtn">Score this text</button>
      <button class="btn" id="fetchDesc">Fetch from the job site</button>
    </div>
  `);

  fillTracking(job);

  $("#jobStatus").addEventListener("change", async (e) => {
    const updated = await saveJob(id, { status: e.target.value });
    if (!updated) return;
    const followUp = updated.status === "applied" && !job.follow_up_on && updated.follow_up_on;
    toast(`Marked as ${STATUS_LABELS[updated.status].toLowerCase()}.` +
      (followUp ? ` Follow-up set for ${fmtWhen(updated.follow_up_on)}.` : ""));
    job = updated;
  });

  let notesTimer;
  document.querySelectorAll("[data-track]").forEach((el) => {
    const save = () => saveJob(id, { [el.dataset.track]: el.value }).then((j) => { if (j) savedHint(); });
    if (el.tagName === "TEXTAREA" || el.type === "text") {
      el.addEventListener("input", () => { clearTimeout(notesTimer); notesTimer = setTimeout(save, 800); });
      el.addEventListener("blur", () => { clearTimeout(notesTimer); save(); });
    } else {
      el.addEventListener("change", save);
    }
  });

  $("#followedUp").addEventListener("click", async () => {
    const updated = await markFollowedUp(id);
    if (updated) fillTracking(updated);
  });

  $("#fetchDesc").addEventListener("click", async (e) => {
    const btn = e.currentTarget;
    btn.disabled = true; btn.innerHTML = `<span class="spinner"></span> Fetching`;
    try {
      await api(`/api/jobs/${id}/fetch-description`, { method: "POST" });
      toast("Full description fetched and scored");
      loadJobs(); refreshStatus(); openJob(id);
    } catch (err) {
      toast(err.message, true);
      btn.disabled = false; btn.textContent = "Fetch from the job site";
    }
  });

  $("#rescoreBtn").addEventListener("click", async () => {
    const text = $("#fullDesc").value.trim();
    if (!text) { toast("Paste the job description first", true); return; }
    try {
      await api(`/api/jobs/${id}`, { method: "PATCH", body: { full_description: text } });
      toast("Scored on the full text");
      loadJobs(); refreshStatus(); openJob(id);
    } catch (err) { toast(err.message, true); }
  });
}

// ------------------------------------------------- add a job / bookmarklet
// The bookmarklet runs on the job page, opens this app with ?add=1 and posts the
// page to it with postMessage. Nothing is saved until you press the button.
function findPosting(x) {
  if (Array.isArray(x)) {
    for (const item of x) { const found = findPosting(item); if (found) return found; }
  } else if (x && typeof x === "object") {
    const t = x["@type"];
    if (t === "JobPosting" || (Array.isArray(t) && t.includes("JobPosting"))) return x;
    for (const k of ["@graph", "mainEntity", "itemListElement"]) {
      if (x[k]) { const found = findPosting(x[k]); if (found) return found; }
    }
  }
  return null;
}

function htmlToText(html) {
  const doc = new DOMParser().parseFromString(String(html), "text/html");  // parsing only, scripts never run
  doc.querySelectorAll("br").forEach((b) => b.replaceWith("\n"));
  doc.querySelectorAll("p, li, h1, h2, h3, h4, div").forEach((el) => el.append("\n"));
  return doc.body.textContent.replace(/[ \t]+\n/g, "\n").replace(/\n{3,}/g, "\n\n").trim();
}

function fromPage(d) {
  let posting = null;
  for (const raw of d.ld || []) {
    try { posting = findPosting(JSON.parse(raw)); } catch (_) { /* not JSON */ }
    if (posting) break;
  }
  // "(3) Full Stack Developer | Acme | LinkedIn" -> title and company
  const parts = String(d.title || "").replace(/^\(\d+\)\s*/, "").split(" | ").map((p) => p.trim());
  const org = posting?.hiringOrganization;
  const places = [].concat(posting?.jobLocation || []);
  const town = places.map((p) => p?.address?.addressLocality).filter(Boolean).join(", ");
  const ldText = posting?.description ? htmlToText(posting.description) : "";
  return {
    url: d.url,
    title: posting?.title || parts[0] || "",
    company: (typeof org === "string" ? org : org?.name) || (parts.length >= 3 ? parts[1] : ""),
    location: posting?.jobLocationType === "TELECOMMUTE" ? "Remote" : town,
    posted_at: String(posting?.datePosted || "").slice(0, 10),
    text: ldText.length > 200 ? ldText : String(d.text || "").trim(),
  };
}

function waitForSentPage() {
  history.replaceState(null, "", location.pathname);
  openDrawer(`<h2>Add a job</h2><p class="hint">Receiving the job page…</p>`);
  const timer = setTimeout(() => {
    removeEventListener("message", onMessage);
    showAddForm({});
    toast("Nothing arrived from the job page. Paste the posting here instead.", true);
  }, 15000);
  function onMessage(e) {
    if (e.data?.type !== "jobseeker-import") return;
    e.source?.postMessage("jobseeker-ok", e.origin);
    removeEventListener("message", onMessage);
    clearTimeout(timer);
    showAddForm(fromPage(e.data));
  }
  addEventListener("message", onMessage);
}

async function showAddForm(p, allowMatch = true) {
  let match = null;
  if (allowMatch && (p.url || p.title)) {
    try { match = (await api("/api/jobs/match", { method: "POST", body: { url: p.url, title: p.title, company: p.company } })).job; }
    catch (_) { /* treat as new */ }
  }
  const field = (name, label, value, attrs = "") =>
    `<label>${label}<input name="${name}" value="${esc(value || "")}" ${attrs}></label>`;
  openDrawer(`
    <h2>${match ? "Update a job in your list" : "Add a job"}</h2>
    ${match ? `<p class="match-note">This is <b>${esc(match.title)}</b>${match.company ? ` at ${esc(match.company)}` : ""}, already in your list.
      The text below replaces its short snippet and the job is scored again.
      <button class="btn quiet" type="button" id="addSeparate">Add it as a separate job</button></p>` : ""}
    <form class="add-form" id="addForm">
      ${match ? "" : `
        ${field("title", "Job title", p.title, "required")}
        ${field("company", "Company", p.company)}
        ${field("location", "Location", p.location, 'placeholder="Town, or Remote"')}
        ${field("url", "Link to the posting", p.url, 'type="url" placeholder="https://"')}`}
      ${field("posted_at", "Posted on", p.posted_at, 'type="date"')}
      <label class="wide">Full posting text
        <textarea name="text" rows="14" required placeholder="Paste the whole job description">${esc(p.text || "")}</textarea></label>
      <p class="hint wide">If the page showed more than one job, check that this is only the job you want. Selecting its text before you click the bookmarklet sends just that.</p>
      <div class="actions wide"><button class="btn primary" type="submit">${match ? "Update and score" : "Add and score"}</button></div>
    </form>`);
  $("#addSeparate")?.addEventListener("click", () => showAddForm(p, false));
  $("#addForm").addEventListener("submit", async (e) => {
    e.preventDefault();
    const form = Object.fromEntries(new FormData(e.currentTarget));
    const body = match ? { ...form, title: match.title, job_id: match.id } : { ...form };
    try {
      const r = await api("/api/jobs/import", { method: "POST", body });
      toast(`${r.created ? "Added" : "Updated"} and scored: ${r.job.grade ?? "no score"}`);
      refreshStatus(); loadJobs();
      openJob(r.job.id);
    } catch (err) { toast(err.message, true); }
  });
}

$("#addBtn").addEventListener("click", () => showAddForm({}, false));

function bookmarkletHref(origin) {
  const code = `(()=>{const A=${JSON.stringify(origin)};const s=String(getSelection()).trim();`
    + `const m=document.querySelector('main,article,[role=main]')||document.body;`
    + `const d={type:'jobseeker-import',url:location.href,title:document.title,`
    + `ld:[...document.querySelectorAll('script[type="application/ld+json"]')].map(x=>x.textContent),`
    + `text:(s.length>200?s:m.innerText).slice(0,30000)};`
    + `const w=window.open(A+'/?add=1','jobseeker');`
    + `if(!w){alert('Allow pop-ups for this site to send jobs to Job Seeker.');return}`
    + `let n=0;const t=setInterval(()=>{try{w.postMessage(d,A)}catch(e){}if(++n>60)clearInterval(t)},250);`
    + `addEventListener('message',e=>{if(e.origin===A&&e.data==='jobseeker-ok')clearInterval(t)})})()`;
  return "javascript:" + encodeURIComponent(code);
}

// ----------------------------------------------------------------- settings
$("#bookmarklet").href = bookmarkletHref(location.origin);
$("#bookmarklet").addEventListener("click", (e) => {
  e.preventDefault();
  toast("Drag this button to your bookmarks bar, then click it on a job page.");
});

$("#notifyNew").checked = notifyEnabled();
$("#notifyNew").addEventListener("change", async (e) => {
  let on = e.target.checked;
  if (on && "Notification" in window && Notification.permission !== "granted") {
    on = (await Notification.requestPermission()) === "granted";
    if (!on) toast("Your browser blocked notifications for this page. Allow them in its site settings.", true);
  }
  e.target.checked = on;
  try { localStorage.setItem("jobseeker.notify", on ? "1" : "0"); } catch (_) { /* private window */ }
});

async function loadSettings() {
  try {
    const s = await api("/api/settings");
    $("#sKeywords").value = s.keywords.join("\n");
    $("#sHome").value = s.home_towns.join(", ");
    $("#sDistance").value = s.max_distance_km;
    $("#sYears").value = s.experience_years;
    $("#sAge").value = s.max_age_days;
    document.querySelectorAll("[data-lang]").forEach((c) => { c.checked = s.job_languages.includes(c.dataset.lang); });
    $("#sHideFr").checked = s.hide_french_required;
    $("#sSources").innerHTML = Object.entries(appStatus.sources).map(([name, info]) => `
      <label class="check"><input type="checkbox" data-src="${name}" id="src-${name}" ${s.sources[name] ? "checked" : ""}>
        ${esc(info.label)} ${info.configured ? "" : `<span class="hint">needs an API key in .env</span>`}</label>`).join("");
  } catch (e) { toast(e.message, true); }
  try {
    const cv = await api("/api/cv");
    $("#cvPath").textContent = cv.path;
    $("#cvSkills").innerHTML = cv.skills.map((s) => `<span class="sk ok">${esc(s)}</span>`).join("");
  } catch (e) { $("#cvSkills").innerHTML = `<span class="sk bad">${esc(e.message)}</span>`; }
}

$("#settingsForm").addEventListener("submit", async (e) => {
  e.preventDefault();
  const sources = {};
  document.querySelectorAll("[data-src]").forEach((c) => { sources[c.dataset.src] = c.checked; });
  try {
    await api("/api/settings", {
      method: "PUT",
      body: {
        keywords: $("#sKeywords").value.split("\n"),
        home_towns: $("#sHome").value.split(",").map((t) => t.trim()).filter(Boolean),
        max_distance_km: Number($("#sDistance").value) || 60,
        experience_years: Number($("#sYears").value) || 0,
        max_age_days: Number($("#sAge").value) || 30,
        job_languages: [...document.querySelectorAll("[data-lang]")].filter((c) => c.checked).map((c) => c.dataset.lang),
        hide_french_required: $("#sHideFr").checked,
        sources,
      },
    });
    toast("Saved. All jobs scored again.");
    refreshStatus();
  } catch (err) { toast(err.message, true); }
});

// --------------------------------------------------------------------- init
(async function init() {
  try {
    const s = await refreshStatus();
    $("#fSource").innerHTML += Object.entries(s.sources)
      .map(([n, v]) => `<option value="${n}">${esc(v.label.replace(/ \(.*\)$/, ""))}</option>`).join("");
    if (s.counts.good === 0) {  // nothing scored high yet: show everything
      minGrade = "";
      document.querySelectorAll("#fGradeSeg button").forEach((x) => x.classList.toggle("on", x.dataset.min === ""));
    }
    lastUnseen = s.counts.unseen;
    if (s.search.running) startPolling();
    if (!s.cv_loaded) toast("CV.pdf was not found in the project folder.", true);
  } catch (e) { toast("Cannot reach the app server: " + e.message, true); }
  const query = new URLSearchParams(location.search);
  if (query.has("add")) waitForSentPage();  // opened by the bookmarklet: listen before anything slow
  loadAgenda();
  await loadJobs();
  const linked = Number(query.get("job"));  // ?job=123 opens that job
  if (linked) openJob(linked);
})();
