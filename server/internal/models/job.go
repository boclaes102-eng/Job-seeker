package models

import "time"

// AuditRecord is one row in the scrape_audit table — every job seen in a
// scrape run, whether or not it passed the pre-filter.
type AuditRecord struct {
	ID          string    `json:"id"`
	RunID       string    `json:"runId"`
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	Company     string    `json:"company"`
	Location    string    `json:"location"`
	Source      string    `json:"source"`
	PostedAt    time.Time `json:"postedAt"`
	DetScore    int       `json:"detScore"`
	MatchedTech []string  `json:"matchedTech"`
	MissingTech []string  `json:"missingTech"`
	WasKept     bool      `json:"wasKept"`
	UsedLLM     bool      `json:"usedLlm"`
	FinalScore  int       `json:"finalScore"`
	Reason      string    `json:"reason"`
	ScrapedAt   time.Time `json:"scrapedAt"`
}

// AuditRunSummary is the per-run overview returned by ListAuditRuns.
type AuditRunSummary struct {
	RunID     string    `json:"runId"`
	Total     int       `json:"total"`
	Kept      int       `json:"kept"`
	ScrapedAt time.Time `json:"scrapedAt"`
}

type JobStatus string

const (
	StatusNew       JobStatus = "new"
	StatusPipeline  JobStatus = "pipeline"
	StatusApplied   JobStatus = "applied"
	StatusDismissed JobStatus = "dismissed"
)

// Job is one job posting tracked through the user's pipeline.
//
// MatchedTech is the canonical list of technologies from the candidate's
// profile that appeared in this job's description. It's stored as a
// JSON array in SQLite and surfaced in the UI as colored badges so the
// user can see at a glance why a job scored what it did.
type Job struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	Company     string    `json:"company"`
	Location    string    `json:"location"`
	Description string    `json:"description"`
	URL         string    `json:"url"`
	Source      string    `json:"source"` // "linkedin" | "adzuna"
	PostedAt    time.Time `json:"postedAt"`
	FetchedAt   time.Time `json:"fetchedAt"`
	MatchScore  int       `json:"matchScore"`
	MatchReason string    `json:"matchReason"`
	MatchedTech []string  `json:"matchedTech"`
	Status      JobStatus `json:"status"`
}
