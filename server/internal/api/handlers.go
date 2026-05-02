package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
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

func NewHandler(s *store.Store, m *matcher.Matcher, profilePath string) *Handler {
	return &Handler{store: s, matcher: m, profilePath: profilePath}
}

// --- Profile ---

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
	jsonOK(w, map[string]string{"ok": "saved"})
}

// --- Jobs list ---

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

// --- SSE refresh stream ---

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

	send := func(ev sseEvent) {
		b, _ := json.Marshal(ev)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	profile := h.readProfile()
	cfg := ParseSearchConfig(profile)

	// Determine which sources to scrape (defaults to all three)
	wantSources := map[string]bool{"adzuna": true, "linkedin": true}
	if sp := r.URL.Query().Get("sources"); sp != "" {
		wantSources = map[string]bool{}
		for _, s := range strings.Split(sp, ",") {
			wantSources[strings.TrimSpace(s)] = true
		}
	}

	radius := 50 // km default
	if rv := r.URL.Query().Get("radius"); rv != "" {
		if n, err := strconv.Atoi(rv); err == nil && n > 0 {
			radius = n
		}
	}

	maxJobs := 50 // balanced cap across queries
	if mj := r.URL.Query().Get("maxJobs"); mj != "" {
		if n, err := strconv.Atoi(mj); err == nil {
			maxJobs = n // 0 means no limit
		}
	}

	maxDaysOld := 0 // 0 = no limit
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
	slog.Info("refresh.start", "sources", wantSources, "queries", len(cfg.Queries), "radius_km", radius, "max_jobs", maxJobs, "max_days_old", maxDaysOld)
	send(sseEvent{Type: "scrape_start", Total: totalScrapes})

	// --- Phase 1: scrape selected sources concurrently ---
	type scrapeResult struct {
		source string
		query  string
		jobs   []*models.Job
		err    error
	}

	ch := make(chan scrapeResult, totalScrapes)

	// LinkedIn: sequential with a short delay to avoid 429 rate-limiting.
	// Firing all 30 goroutines at once reliably triggers LinkedIn's bot detection.
	if wantSources["linkedin"] {
		go func() {
			for i, q := range cfg.Queries {
				if i > 0 {
					time.Sleep(500 * time.Millisecond)
				}
				jobs, err := scrapers.NewLinkedIn(q, cfg.Location, cfg.City, radius, maxDaysOld).Fetch()
				ch <- scrapeResult{"linkedin", q, jobs, err}
			}
		}()
	}

	// Adzuna: sequential with delay to respect trial rate limits
	if wantSources["adzuna"] {
		go func() {
			appID := os.Getenv("ADZUNA_APP_ID")
			appKey := os.Getenv("ADZUNA_APP_KEY")
			for i, q := range cfg.Queries {
				if i > 0 {
					time.Sleep(700 * time.Millisecond)
				}
				jobs, err := scrapers.NewAdzuna(q, appID, appKey, cfg.City, radius, maxDaysOld).Fetch()
				ch <- scrapeResult{"adzuna", q, jobs, err}
			}
		}()
	}

	// Collect results and track which query produced each job
	urlToQuery := map[string]string{}
	queryJobs := map[string][]*models.Job{} // query → jobs (deduped per query)
	seenURL := map[string]bool{}
	scrapeErrors := map[string]string{}
	rawTotal := 0

	for i := 0; i < totalScrapes; i++ {
		res := <-ch
		if res.err != nil {
			key := res.query + "/" + res.source
			scrapeErrors[key] = res.err.Error()
			send(sseEvent{Type: "source_error", Source: res.source, Query: res.query, Error: res.err.Error()})
		} else {
			rawTotal += len(res.jobs)
			for _, j := range res.jobs {
				if !seenURL[j.URL] {
					seenURL[j.URL] = true
					urlToQuery[j.URL] = res.query
					queryJobs[res.query] = append(queryJobs[res.query], j)
				}
			}
			send(sseEvent{Type: "source_done", Source: res.source, Query: res.query, Count: len(res.jobs)})
		}
	}

	// Sort each query bucket by recency (newest first) so we pick fresh jobs first
	for q, jobs := range queryJobs {
		sort.Slice(jobs, func(i, j int) bool {
			return jobs[i].PostedAt.After(jobs[j].PostedAt)
		})
		queryJobs[q] = jobs
	}

	// Build stable query order for round-robin
	queryOrder := make([]string, 0, len(queryJobs))
	for q := range queryJobs {
		queryOrder = append(queryOrder, q)
	}
	sort.Strings(queryOrder)

	// Select jobs: round-robin across query buckets to maximise diversity.
	// Each pass takes 1 job from each bucket (the freshest not yet taken).
	// Stops when maxJobs is reached or all buckets are exhausted.
	var unique []*models.Job
	if maxJobs == 0 {
		for _, q := range queryOrder {
			unique = append(unique, queryJobs[q]...)
		}
	} else {
		offsets := make(map[string]int)
		for len(unique) < maxJobs {
			madeProgress := false
			for _, q := range queryOrder {
				if len(unique) >= maxJobs {
					break
				}
				off := offsets[q]
				if off < len(queryJobs[q]) {
					unique = append(unique, queryJobs[q][off])
					offsets[q]++
					madeProgress = true
				}
			}
			if !madeProgress {
				break
			}
		}
	}

	slog.Info("refresh.dedup", "raw_total", rawTotal, "unique_after_dedup", len(unique), "errors", len(scrapeErrors))
	send(sseEvent{Type: "dedup", Unique: len(unique), Total: rawTotal})

	// --- Phase 2: score concurrently ---
	// Build keyword list once from the candidate profile for fast pre-filtering.
	// Jobs with zero tech overlap skip Ollama entirely and are scored instantly.
	candidateKeywords := matcher.ExtractKeywords(profile)

	ollamaParallel := 4
	if op := os.Getenv("OLLAMA_NUM_PARALLEL"); op != "" {
		if n, err := strconv.Atoi(op); err == nil && n > 0 {
			ollamaParallel = n
		}
	}
	total := len(unique)
	var scored atomic.Int32
	sem := make(chan struct{}, ollamaParallel)
	var wg sync.WaitGroup

	for _, job := range unique {
		select {
		case <-ctx.Done():
			break
		default:
		}
		sem <- struct{}{}
		wg.Add(1)
		go func(j *models.Job) {
			defer func() {
				<-sem
				wg.Done()
			}()

			if !matcher.HasTechOverlap(j, candidateKeywords) {
				// No technology overlap — skip Ollama, auto-score low instantly
				j.MatchScore = 15
				j.MatchReason = "Auto-filtered: none of the candidate's technologies were detected in this job description."
			} else {
				score, reason, err := h.matcher.Score(ctx, j, profile)
				if err == nil {
					j.MatchScore = score
					j.MatchReason = reason
				}
			}

			_ = h.store.Upsert(j)
			n := int(scored.Add(1))
			send(sseEvent{Type: "scoring", Current: n, Total: total, Title: j.Title, Company: j.Company, Score: j.MatchScore})
		}(job)
	}
	wg.Wait()

	slog.Info("refresh.done", "scored", total, "errors", scrapeErrors)
	send(sseEvent{Type: "done", Fetched: total, Errors: scrapeErrors})
}

// --- Reset ---

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

// --- Status update ---

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

// --- Re-analyze single job ---

func (h *Handler) AnalyzeJob(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.store.Get(id)
	if err != nil {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}
	profile := h.readProfile()
	score, reason, err := h.matcher.Score(r.Context(), job, profile)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_ = h.store.UpdateMatch(id, score, reason)
	jsonOK(w, map[string]any{"score": score, "reason": reason})
}

// --- Draft email ---

func (h *Handler) DraftEmail(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	job, err := h.store.Get(id)
	if err != nil {
		jsonError(w, "job not found", http.StatusNotFound)
		return
	}
	profile := h.readProfile()
	draft, err := h.matcher.DraftEmail(r.Context(), job, profile)
	if err != nil {
		jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}
	jsonOK(w, draft)
}

// --- Helpers ---

func (h *Handler) readProfile() string {
	data, err := os.ReadFile(h.profilePath)
	if err != nil {
		return ""
	}
	return string(data)
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
