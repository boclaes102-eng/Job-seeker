package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"job-seeker/server/internal/matcher"
	"job-seeker/server/internal/models"
	"job-seeker/server/internal/scrapers"
	"job-seeker/server/internal/store"
)

type Handler struct {
	store       *store.Store
	matcher     *matcher.Matcher
	profilePath string
}

// scrapeResult is what one scrape goroutine produces per (source, query) pair.
// Defined at package level so scrapeQueriesSequentially can take a typed channel
// rather than an anonymous-struct one (which doesn't compose across functions).
type scrapeResult struct {
	source string
	query  string
	jobs   []*models.Job
	err    error
}

func NewHandler(s *store.Store, m *matcher.Matcher, profilePath string) *Handler {
	return &Handler{store: s, matcher: m, profilePath: profilePath}
}

// ─── Profile ─────────────────────────────────────────────────────────────────

func (h *Handler) GetProfile(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(h.profilePath)
	if err != nil {
		jsonError(w, "profile not found", http.StatusNotFound)
		return
	}
	jsonOK(w, map[string]string{"content": string(data)})
}

func (h *Handler) SaveProfile(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := os.WriteFile(h.profilePath, []byte(body.Content), 0644); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Editing the profile invalidates the in-memory score cache; the next
	// refresh should re-evaluate everything against the new profile hash.
	h.matcher.ResetCache()
	jsonOK(w, map[string]string{"ok": "saved"})
}

// ─── Jobs list ───────────────────────────────────────────────────────────────

func (h *Handler) ListJobs(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	opts := store.ListOptions{
		Source:  q.Get("source"),
		Status:  q.Get("status"),
		NewOnly: q.Get("status") == "",
	}
	jobs, err := h.store.List(opts)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if jobs == nil {
		jobs = []*models.Job{}
	}
	jsonOK(w, jobs)
}

// ─── SSE refresh stream ──────────────────────────────────────────────────────
//
// Lifecycle:
//   1. Phase 1 (scrape): per source × per query, a goroutine fetches jobs.
//   2. Dedup by URL.
//   3. Phase 2 (pipelined): hydration workers fetch missing LinkedIn descriptions
//      and forward them on a channel; scoring workers pick up each job as soon
//      as its description is ready and run the two-stage matcher.
//   4. Done.
//
// The pipeline phase replaces what used to be two sequential phases (hydrate-all
// → score-all). Wall-clock time is now bounded by the slower of the two pools
// rather than their sum.

type sseEvent struct {
	Type string `json:"type"`

	// scrape_start
	Total int `json:"total,omitempty"`

	// source_done / source_error
	Source string `json:"source,omitempty"`
	Query  string `json:"query,omitempty"`
	Count  int    `json:"count,omitempty"`
	Error  string `json:"error,omitempty"`

	// dedup
	Unique int `json:"unique,omitempty"`

	// hydrate_progress
	Hydrated int `json:"hydrated,omitempty"`

	// scoring
	Current int    `json:"current,omitempty"`
	Title   string `json:"title,omitempty"`
	Company string `json:"company,omitempty"`
	Score   int    `json:"score,omitempty"`

	// done
	Fetched int               `json:"fetched,omitempty"`
	Errors  map[string]string `json:"errors,omitempty"`
}

func (h *Handler) RefreshJobsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	runID := time.Now().Format("2006-01-02T15-04-05")

	// `send` is called from many goroutines (scrapers, hydration workers, scoring
	// workers). Without the mutex, concurrent writes interleave bytes mid-event
	// and break JSON parsing on the client. Events are small so contention is
	// negligible.
	var sendMu sync.Mutex
	send := func(ev sseEvent) {
		sendMu.Lock()
		defer sendMu.Unlock()
		b, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	profile := h.readProfile()
	cfg := ParseSearchConfig(profile)

	wantSources := map[string]bool{"adzuna": true, "linkedin": true}
	if sp := r.URL.Query().Get("sources"); sp != "" {
		wantSources = map[string]bool{}
		for _, s := range strings.Split(sp, ",") {
			wantSources[strings.TrimSpace(s)] = true
		}
	}

	radius := 50
	if rv := r.URL.Query().Get("radius"); rv != "" {
		if n, err := strconv.Atoi(rv); err == nil && n > 0 {
			radius = n
		}
	}

	maxJobs := 50
	if mj := r.URL.Query().Get("maxJobs"); mj != "" {
		if n, err := strconv.Atoi(mj); err == nil {
			maxJobs = n // 0 = no limit
		}
	}

	maxDaysOld := 0
	switch r.URL.Query().Get("dateRange") {
	case "today":
		maxDaysOld = 1
	case "3days":
		maxDaysOld = 3
	case "week":
		maxDaysOld = 7
	case "month":
		maxDaysOld = 30
	}

	totalScrapes := len(cfg.Queries) * len(wantSources)
	slog.Info("refresh.start", "sources", wantSources, "queries", len(cfg.Queries),
		"radius_km", radius, "max_jobs", maxJobs, "max_days_old", maxDaysOld)
	send(sseEvent{Type: "scrape_start", Total: totalScrapes})

	// ── Phase 1: scrape ─────────────────────────────────────────────────
	ch := make(chan scrapeResult, totalScrapes)

	if wantSources["linkedin"] {
		// Build a list of real Belgian cities within the user's radius.
		// Each job query gets the next city in the list (round-robin), so the 30
		// queries are spread across the nearby cities instead of all hitting "Belgium".
		// LinkedIn's city-name text search is far more reliable than geoId for
		// unauthenticated requests.
		nearbyCities := scrapers.CitiesWithinRadius(cfg.City, float64(radius))
		locationStrings := make([]string, len(nearbyCities))
		for i, c := range nearbyCities {
			locationStrings[i] = c.Name + ", Belgium"
		}
		if len(locationStrings) == 0 {
			locationStrings = []string{"Belgium"}
		}
		slog.Info("linkedin.locations", "center", cfg.City, "radius_km", radius, "cities", len(locationStrings))

		i := 0
		go scrapeQueriesSequentially(ctx, "linkedin", cfg.Queries, 500*time.Millisecond, ch,
			func(q string) ([]*models.Job, error) {
				loc := locationStrings[i%len(locationStrings)]
				i++
				return scrapers.NewLinkedIn(q, loc, maxDaysOld).Fetch()
			})
	}
	if wantSources["adzuna"] {
		appID := os.Getenv("ADZUNA_APP_ID")
		appKey := os.Getenv("ADZUNA_APP_KEY")
		go scrapeQueriesSequentially(ctx, "adzuna", cfg.Queries, 700*time.Millisecond, ch,
			func(q string) ([]*models.Job, error) {
				return scrapers.NewAdzuna(q, appID, appKey, cfg.City, radius, maxDaysOld).Fetch()
			})
	}

	queryJobs := map[string][]*models.Job{}
	seenURL := map[string]bool{}
	scrapeErrors := map[string]string{}
	rawTotal := 0

	for i := 0; i < totalScrapes; i++ {
		res := <-ch
		if res.err != nil {
			key := res.query + "/" + res.source
			scrapeErrors[key] = res.err.Error()
			send(sseEvent{Type: "source_error", Source: res.source, Query: res.query, Error: res.err.Error()})
			continue
		}
		rawTotal += len(res.jobs)
		for _, j := range res.jobs {
			// Hard exclude: senior/lead/principal titles are never relevant.
			if isSeniorTitle(j.Title) {
				continue
			}
			// Primary dedup: exact URL match.
			// Secondary dedup: same (title, company) — catches the same job posted
			// under multiple LinkedIn job IDs (common with agencies and large companies).
			titleCompanyKey := strings.ToLower(strings.TrimSpace(j.Title)) + "|" + strings.ToLower(strings.TrimSpace(j.Company))
			if !seenURL[j.URL] && !seenURL[titleCompanyKey] {
				seenURL[j.URL] = true
				seenURL[titleCompanyKey] = true
				queryJobs[res.query] = append(queryJobs[res.query], j)
			}
		}
		send(sseEvent{Type: "source_done", Source: res.source, Query: res.query, Count: len(res.jobs)})
	}

	// Sort each bucket newest-first.
	for q, jobs := range queryJobs {
		sort.Slice(jobs, func(i, j int) bool {
			return jobs[i].PostedAt.After(jobs[j].PostedAt)
		})
		queryJobs[q] = jobs
	}

	queryOrder := make([]string, 0, len(queryJobs))
	for q := range queryJobs {
		queryOrder = append(queryOrder, q)
	}
	sort.Strings(queryOrder)

	// Flatten all URL-deduped jobs — pre-filter below will rank and cap them.
	var allUnique []*models.Job
	for _, q := range queryOrder {
		allUnique = append(allUnique, queryJobs[q]...)
	}

	slog.Info("refresh.dedup", "raw_total", rawTotal, "unique", len(allUnique), "errors", len(scrapeErrors))
	send(sseEvent{Type: "dedup", Unique: len(allUnique), Total: rawTotal})

	// ── Pre-filter: score every unique job deterministically (µs each), keep top maxJobs ──
	candidateIdx := matcher.BuildCandidateIndex(profile)
	slog.Info("refresh.candidate_index",
		"techs", len(candidateIdx.Tech),
		"roles", len(candidateIdx.RoleKeywords),
		"profile_hash", candidateIdx.Hash,
		"model", h.matcher.Model())

	type preScored struct {
		job       *models.Job
		score     int
		breakdown matcher.ScoreBreakdown
	}
	ps := make([]preScored, len(allUnique))
	for i, j := range allUnique {
		s := matcher.DeterministicScore(j, candidateIdx)
		ps[i] = preScored{j, s.Total, s}
	}
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].score > ps[j].score })

	limit := len(ps)
	if maxJobs > 0 && len(ps) > maxJobs {
		limit = maxJobs
	}

	// Teaching jobs score low (no tech in title) but are high-priority.
	// Guarantee them a spot regardless of the competitive pre-filter cut-off.
	keptURLs := make(map[string]bool, limit)
	var unique []*models.Job
	for i := 0; i < len(ps); i++ {
		if isTeachingRole(ps[i].job.Title) {
			keptURLs[ps[i].job.URL] = true
			unique = append(unique, ps[i].job)
		}
	}
	teachingCount := len(unique)
	remaining := limit - teachingCount
	if remaining < 0 {
		remaining = 0
	}
	added := 0
	for i := 0; i < len(ps) && added < remaining; i++ {
		if !keptURLs[ps[i].job.URL] {
			keptURLs[ps[i].job.URL] = true
			unique = append(unique, ps[i].job)
			added++
		}
	}
	if teachingCount > 0 {
		slog.Info("refresh.pre_filter_teaching", "guaranteed", teachingCount)
	}
	if len(allUnique) > limit {
		slog.Info("refresh.pre_filter", "total_unique", len(allUnique), "kept", limit, "dropped", len(allUnique)-limit)
		send(sseEvent{Type: "pre_filter", Unique: limit, Total: len(allUnique)})
	}

	// Write every scraped job to the audit table so we can review what was kept/dropped.
	now := time.Now()
	auditRecords := make([]*models.AuditRecord, len(ps))
	for i, p := range ps {
		auditRecords[i] = &models.AuditRecord{
			ID:          p.job.ID,
			RunID:       runID,
			URL:         p.job.URL,
			Title:       p.job.Title,
			Company:     p.job.Company,
			Location:    p.job.Location,
			Source:      p.job.Source,
			PostedAt:    p.job.PostedAt,
			DetScore:    p.score,
			MatchedTech: p.breakdown.MatchedTech,
			MissingTech: p.breakdown.MissingTech,
			WasKept:     keptURLs[p.job.URL],
			ScrapedAt:   now,
		}
	}
	if err := h.store.BulkInsertAudit(auditRecords); err != nil {
		slog.Warn("audit.insert_failed", "err", err)
	}

	// ── Phase 2: pipelined hydration + scoring ──────────────────────────

	scoreConcurrency := 4
	if v := os.Getenv("OLLAMA_NUM_PARALLEL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			scoreConcurrency = n
		}
	}

	// scoreCh feeds scoring workers.
	scoreCh := make(chan *models.Job, len(unique))

	// 1. Hydration: fetch LinkedIn descriptions sequentially with a delay between
	// each request. Concurrent bursts are what trigger LinkedIn's rate limiter —
	// sequential + delay keeps us well under the threshold.
	// A circuit breaker stops hydration early if a 429 is detected.
	var toHydrate int
	for _, j := range unique {
		if j.Source == "linkedin" && j.Description == "" {
			toHydrate++
		}
	}
	if toHydrate > 0 {
		send(sseEvent{Type: "hydrate_start", Total: toHydrate})
	}

	go func() {
		var hydrated atomic.Int32
		blocked := false
		first := true
		for _, job := range unique {
			j := job
			if j.Source != "linkedin" || j.Description != "" {
				scoreCh <- j
				continue
			}
			if blocked {
				// Rate-limited — skip description fetch, score on title only
				slog.Debug("linkedin.hydration_skipped", "url", j.URL, "reason", "circuit breaker open")
				scoreCh <- j
				n := int(hydrated.Add(1))
				send(sseEvent{Type: "hydrate_progress", Current: n, Total: toHydrate})
				continue
			}
			// Space requests: 2 seconds apart (skip delay for the very first one)
			if !first {
				select {
				case <-ctx.Done():
					blocked = true
				case <-time.After(2 * time.Second):
				}
			}
			first = false
			dctx, dcancel := context.WithTimeout(ctx, 12*time.Second)
			desc, err := scrapers.FetchLinkedInDescription(dctx, j.URL)
			dcancel()
			if err == nil && desc != "" {
				j.Description = desc
			} else if err != nil {
				slog.Debug("linkedin.detail_failed", "url", j.URL, "err", err)
				if strings.Contains(err.Error(), "429") {
					blocked = true
					slog.Warn("linkedin.hydration_blocked", "msg", "429 on detail page — skipping remaining descriptions")
				}
			}
			n := int(hydrated.Add(1))
			send(sseEvent{Type: "hydrate_progress", Current: n, Total: toHydrate})
			scoreCh <- j
		}
		close(scoreCh)
	}()

	// 2. Scoring pool. Pulls jobs off scoreCh as they become available.
	total := len(unique)
	var scored atomic.Int32
	var llmCalls atomic.Int32
	var scoreWG sync.WaitGroup
	for i := 0; i < scoreConcurrency; i++ {
		scoreWG.Add(1)
		go func() {
			defer scoreWG.Done()
			for j := range scoreCh {
				if ctx.Err() != nil {
					return
				}
				result := h.matcher.ScoreJob(ctx, j, candidateIdx)
				j.MatchScore = result.Score
				j.MatchReason = result.Reason
				j.MatchedTech = result.MatchedTech
				if result.UsedLLM {
					llmCalls.Add(1)
				}
				_ = h.store.Upsert(j)
				_ = h.store.UpdateAuditFinal(j.ID, runID, result.Score, result.Reason, result.UsedLLM)
				n := int(scored.Add(1))
				send(sseEvent{Type: "scoring", Current: n, Total: total,
					Title: j.Title, Company: j.Company, Score: j.MatchScore})
			}
		}()
	}
	scoreWG.Wait()

	slog.Info("refresh.done",
		"scored", total,
		"llm_calls", llmCalls.Load(),
		"deterministic_only", int32(total)-llmCalls.Load(),
		"errors", scrapeErrors)
	send(sseEvent{Type: "done", Fetched: total, Errors: scrapeErrors})
}

// scrapeQueriesSequentially fires one query at a time with a fixed delay
// between them. Sequential pacing avoids tripping the rate limiters of LinkedIn
// (no API key) and Adzuna (trial tier ~25 calls / minute).
func scrapeQueriesSequentially(
	ctx context.Context,
	source string,
	queries []string,
	delay time.Duration,
	out chan<- scrapeResult,
	fetch func(string) ([]*models.Job, error),
) {
	for i, q := range queries {
		if ctx.Err() != nil {
			out <- scrapeResult{source, q, nil, ctx.Err()}
			continue
		}
		if i > 0 {
			select {
			case <-ctx.Done():
				out <- scrapeResult{source, q, nil, ctx.Err()}
				continue
			case <-time.After(delay):
			}
		}
		jobs, err := fetch(q)
		out <- scrapeResult{source, q, jobs, err}
	}
}

// ─── Reset ───────────────────────────────────────────────────────────────────

func (h *Handler) ResetJobs(w http.ResponseWriter, r *http.Request) {
	n, err := h.store.Reset()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]any{"deleted": n})
}

func (h *Handler) ClearNewJobs(w http.ResponseWriter, r *http.Request) {
	if err := h.store.ClearNew(); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"ok": "cleared"})
}

// ─── Status update ───────────────────────────────────────────────────────────

func (h *Handler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var body struct {
		Status models.JobStatus `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonError(w, "invalid body", http.StatusBadRequest)
		return
	}
	if err := h.store.UpdateStatus(id, body.Status); err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, map[string]string{"status": string(body.Status)})
}

// ─── Re-analyze single job ───────────────────────────────────────────────────

func (h *Handler) AnalyzeJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.store.Get(id)
	if err != nil {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}
	profile := h.readProfile()
	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()
	score, reason, matched, err := h.matcher.Score(ctx, job, profile)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = h.store.UpdateMatch(id, score, reason, matched)
	jsonOK(w, map[string]any{"score": score, "reason": reason, "matchedTech": matched})
}

// ─── Draft email ─────────────────────────────────────────────────────────────

func (h *Handler) DraftEmail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.store.Get(id)
	if err != nil {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}
	profile := h.readProfile()
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	draft, err := h.matcher.DraftEmail(ctx, job, profile)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, draft)
}

// ─── Helpers ─────────────────────────────────────────────────────────────────

// ─── Audit ───────────────────────────────────────────────────────────────────

func (h *Handler) ListAuditRuns(w http.ResponseWriter, r *http.Request) {
	runs, err := h.store.ListAuditRuns()
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if runs == nil {
		runs = []models.AuditRunSummary{}
	}
	jsonOK(w, runs)
}

func (h *Handler) GetAuditRun(w http.ResponseWriter, r *http.Request) {
	runID := chi.URLParam(r, "runID")
	records, err := h.store.GetAuditRun(runID)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if records == nil {
		records = []*models.AuditRecord{}
	}
	jsonOK(w, records)
}

func (h *Handler) readProfile() string {
	data, err := os.ReadFile(h.profilePath)
	if err != nil {
		return ""
	}
	return string(data)
}

// isSeniorTitle returns true when the job title clearly indicates a seniority
// level that is beyond the candidate's target (Senior, Lead, Principal, etc.).
// These jobs are dropped before dedup and never enter the pipeline.
var seniorTitleRe = regexp.MustCompile(
	`(?i)\b(senior|sr\.?|lead|principal|staff\s+engineer|head\s+of|leidinggevende)\b`,
)

var studentTitleRe = regexp.MustCompile(
	`(?i)\b(thesis\s+student|student\s+thesis|thesis\s+worker|bachelorproef|masterproef|eindwerk|internship|stageopdracht|stagiair)\b`,
)

func isSeniorTitle(title string) bool {
	return seniorTitleRe.MatchString(title) || studentTitleRe.MatchString(title)
}

// isTeachingRole returns true when the job title is an IT/ICT teaching position.
// Both a teaching keyword AND an IT/tech subject keyword must be present.
// These jobs score low in the pre-filter (no tech stack in title) so we
// guarantee them a slot regardless of the competitive score cut-off.
var teachingWordRe = regexp.MustCompile(
	`(?i)\b(docent|leerkracht|lesgever|lector|leraar|instructeur|lecturer|teacher|trainer)\b`,
)
var itSubjectRe = regexp.MustCompile(
	`(?i)\b(ict|informatica|coding|cybersecurity|software|digitaal|digital|` +
		`computer|netwerk|network|web|cloud|devops|security)\b` +
		`|\bprogramm` + // matches programmeren, programmeur, programmeer etc.
		`|\bdata\s+(engineer|science|analyst|platform)` + // data in IT context only
		`|\b(it|ai)\s+(teacher|docent|trainer|lecturer|lesgever)` + // "IT teacher" etc.
		`|\b(docent|leerkracht|lesgever|trainer|lecturer)\s+(it|ai|ict)\b`, // "docent IT" etc.
)

func isTeachingRole(title string) bool {
	return teachingWordRe.MatchString(title) && itSubjectRe.MatchString(title)
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

func ProfilePath() string {
	if p := os.Getenv("PROFILE_PATH"); p != "" {
		return p
	}
	dir, _ := os.Getwd()
	for i := 0; i < 4; i++ {
		candidate := filepath.Join(dir, "profile.md")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		dir = filepath.Dir(dir)
	}
	return "profile.md"
}
