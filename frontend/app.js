"use strict";

const $ = (sel) => document.querySelector(sel);
const esc = (s) =>
  String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

const STATUS_LABELS = {
  new: "New", saved: "Saved", applied: "Applied", interview: "Interview", rejected: "Rejected", hidden: "Hidden",
};
const REC_LABELS = { strong_apply: "Strong match", apply: "Good match", maybe: "Worth a look", skip: "Weak match" };
// Order and weight of the score parts, matching jobseeker/matching.py
const PARTS = [["role", "Role", 40], ["skills", "Skills", 30], ["coverage", "Coverage", 15], ["location", "Distance", 15]];

let appStatus = null;
let pollTimer = null;
let minGrade = "65";
let statusFilter = "";
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
  return appStatus?.sources?.[name]?.label?.replace(/ \(.*\)$/, "") || name;
}

function ago(iso) {
  if (!iso) return "";
  const days = Math.round((Date.now() - new Date(iso).getTime()) / 864e5);
  if (Number.isNaN(days)) return iso;
  return days <= 0 ? "today" : days === 1 ? "yesterday" : days < 30 ? `${days} days ago` : iso;
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
      loadJobs();
      toast("Search finished");
    }
  }, 1500);
}

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
            ${j.status !== "new" ? `<span class="tag t-${esc(j.status)}">${esc(STATUS_LABELS[j.status] || j.status)}</span>` : ""}</h3>
          <div class="who">${meta(j, [sourceLabel(j.source), ago(j.posted_at)])}</div>
          ${g ? inlineSkills(g, 6) : ""}
        </div>
        ${distanceCell(g)}
        <div class="job-actions">
          ${j.url ? `<a class="btn primary" href="${esc(j.url)}" target="_blank" rel="noopener" data-act="link">Apply on ${esc(sourceLabel(j.source))}</a>` : ""}
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
    loadJobs(); refreshStatus();
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

    <div class="actions">
      ${job.url ? `<a class="btn primary" href="${esc(job.url)}" target="_blank" rel="noopener">Apply on ${esc(sourceLabel(job.source))}</a>` : ""}
      <label class="status-pick"><span class="hint">Status</span><select id="jobStatus">${statusOptions}</select></label>
    </div>

    <h3 class="sec">Description${job.full_description ? "" : ", short version from the job site"}</h3>
    <div class="desc">${esc(job.full_description || job.description || "No description available.")}</div>

    <h3 class="sec">Get a more accurate score</h3>
    <p class="hint">Job sites only share a snippet. Paste the full posting text, or fetch it, and the job is scored again on the complete text.</p>
    <textarea id="fullDesc" rows="6" placeholder="Paste the full job description here" aria-label="Full job description">${esc(job.full_description || "")}</textarea>
    <div class="actions">
      <button class="btn primary" id="rescoreBtn">Score this text</button>
      <button class="btn" id="fetchDesc">Fetch from the job site</button>
    </div>
  `);

  $("#jobStatus").addEventListener("change", async (e) => {
    await api(`/api/jobs/${id}`, { method: "PATCH", body: { status: e.target.value } });
    toast(`Marked as ${STATUS_LABELS[e.target.value].toLowerCase()}`);
    loadJobs(); refreshStatus();
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

// ----------------------------------------------------------------- settings
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
    if (s.search.running) startPolling();
    if (!s.cv_loaded) toast("CV.pdf was not found in the project folder.", true);
  } catch (e) { toast("Cannot reach the app server: " + e.message, true); }
  await loadJobs();
  const linked = Number(new URLSearchParams(location.search).get("job"));  // ?job=123 opens that job
  if (linked) openJob(linked);
})();
