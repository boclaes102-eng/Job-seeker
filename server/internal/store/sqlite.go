package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"job-seeker/server/internal/models"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	s := &Store{db: db}
	return s, s.migrate()
}

// migrate creates the jobs table and applies any in-flight column additions.
// We use ALTER TABLE ADD COLUMN for forward-compat — older deployments will
// transparently gain the matched_tech column without losing data.
func (s *Store) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS jobs (
			id          TEXT PRIMARY KEY,
			title       TEXT NOT NULL,
			company     TEXT NOT NULL,
			location    TEXT NOT NULL,
			description TEXT NOT NULL,
			url         TEXT NOT NULL UNIQUE,
			source      TEXT NOT NULL,
			posted_at   DATETIME NOT NULL,
			fetched_at  DATETIME NOT NULL,
			match_score INTEGER NOT NULL DEFAULT 0,
			match_reason TEXT NOT NULL DEFAULT '',
			matched_tech TEXT NOT NULL DEFAULT '[]',
			status      TEXT NOT NULL DEFAULT 'new'
		)`); err != nil {
		return err
	}

	// Migration: add matched_tech column to existing databases that pre-date it.
	// SQLite raises "duplicate column name" on retry — we ignore that error.
	_, err := s.db.Exec(`ALTER TABLE jobs ADD COLUMN matched_tech TEXT NOT NULL DEFAULT '[]'`)
	if err != nil && !isDuplicateColumnErr(err) {
		return err
	}

	// Indexes that speed up the common queries.
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_jobs_status_score ON jobs(status, match_score DESC)`)
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_jobs_source ON jobs(source)`)

	// Audit table: every job seen in every scrape run, kept or dropped.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS scrape_audit (
			id           TEXT NOT NULL,
			run_id       TEXT NOT NULL,
			url          TEXT NOT NULL,
			title        TEXT NOT NULL,
			company      TEXT NOT NULL,
			location     TEXT NOT NULL,
			source       TEXT NOT NULL,
			posted_at    DATETIME,
			det_score    INTEGER NOT NULL DEFAULT 0,
			matched_tech TEXT NOT NULL DEFAULT '[]',
			missing_tech TEXT NOT NULL DEFAULT '[]',
			was_kept     INTEGER NOT NULL DEFAULT 0,
			used_llm     INTEGER NOT NULL DEFAULT 0,
			final_score  INTEGER NOT NULL DEFAULT 0,
			reason       TEXT NOT NULL DEFAULT '',
			scraped_at   DATETIME NOT NULL,
			PRIMARY KEY (id, run_id)
		)`); err != nil {
		return err
	}
	_, _ = s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_audit_run ON scrape_audit(run_id, det_score DESC)`)
	return nil
}

func isDuplicateColumnErr(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "duplicate column") || contains(msg, "already exists")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexAny(s, sub) >= 0)
}

func indexAny(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// Upsert inserts a job by URL, or updates it if it already exists.
//
// IMPORTANT: this overwrites match_score / match_reason / matched_tech on
// every upsert. The previous behaviour (preserve existing scores) led to
// stale scores from earlier matcher versions sticking around forever. If
// the user wants to keep a manually-rescored result, they should mark the
// job as "pipeline" — pipelined jobs are excluded from the next refresh's
// scrape candidates by URL-dedup at the SSE layer.
func (s *Store) Upsert(j *models.Job) error {
	techJSON, err := encodeTech(j.MatchedTech)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO jobs (
			id, title, company, location, description, url, source,
			posted_at, fetched_at, match_score, match_reason, matched_tech, status
		)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(url) DO UPDATE SET
			title        = excluded.title,
			company      = excluded.company,
			location     = excluded.location,
			description  = excluded.description,
			fetched_at   = excluded.fetched_at,
			match_score  = excluded.match_score,
			match_reason = excluded.match_reason,
			matched_tech = excluded.matched_tech
	`,
		j.ID, j.Title, j.Company, j.Location, j.Description,
		j.URL, j.Source, j.PostedAt.Format(time.RFC3339), j.FetchedAt.Format(time.RFC3339),
		j.MatchScore, j.MatchReason, techJSON, j.Status,
	)
	return err
}

type ListOptions struct {
	Source  string
	Status  string
	NewOnly bool
}

func (s *Store) List(opts ListOptions) ([]*models.Job, error) {
	query := `SELECT id, title, company, location, description, url, source, posted_at, fetched_at, match_score, match_reason, matched_tech, status FROM jobs WHERE 1=1`
	args := []any{}

	if opts.Source != "" {
		query += " AND source = ?"
		args = append(args, opts.Source)
	}
	if opts.Status != "" {
		query += " AND status = ?"
		args = append(args, opts.Status)
	}
	if opts.NewOnly {
		query += " AND status = 'new'"
	}

	query += " ORDER BY match_score DESC, posted_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*models.Job
	for rows.Next() {
		j := &models.Job{}
		var postedAt, fetchedAt, techJSON string
		if err := rows.Scan(&j.ID, &j.Title, &j.Company, &j.Location, &j.Description,
			&j.URL, &j.Source, &postedAt, &fetchedAt,
			&j.MatchScore, &j.MatchReason, &techJSON, &j.Status); err != nil {
			return nil, err
		}
		j.PostedAt = parseTime(postedAt)
		j.FetchedAt = parseTime(fetchedAt)
		j.MatchedTech = decodeTech(techJSON)
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (s *Store) Reset() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM jobs WHERE status NOT IN ('pipeline', 'applied', 'dismissed')`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

func (s *Store) ClearNew() error {
	_, err := s.db.Exec(`DELETE FROM jobs WHERE status = 'new'`)
	return err
}

func (s *Store) UpdateStatus(id string, status models.JobStatus) error {
	_, err := s.db.Exec(`UPDATE jobs SET status = ? WHERE id = ?`, string(status), id)
	return err
}

func (s *Store) UpdateMatch(id string, score int, reason string, matched []string) error {
	techJSON, err := encodeTech(matched)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE jobs SET match_score = ?, match_reason = ?, matched_tech = ? WHERE id = ?`,
		score, reason, techJSON, id)
	return err
}

func (s *Store) Get(id string) (*models.Job, error) {
	j := &models.Job{}
	var postedAt, fetchedAt, techJSON string
	err := s.db.QueryRow(`
		SELECT id, title, company, location, description, url, source, posted_at, fetched_at, match_score, match_reason, matched_tech, status
		FROM jobs WHERE id = ?`, id).
		Scan(&j.ID, &j.Title, &j.Company, &j.Location, &j.Description,
			&j.URL, &j.Source, &postedAt, &fetchedAt,
			&j.MatchScore, &j.MatchReason, &techJSON, &j.Status)
	if err != nil {
		return nil, err
	}
	j.PostedAt = parseTime(postedAt)
	j.FetchedAt = parseTime(fetchedAt)
	j.MatchedTech = decodeTech(techJSON)
	return j, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// ─── Audit ────────────────────────────────────────────────────────────────────

// BulkInsertAudit writes one record per scraped job for this run.
// Called immediately after the pre-filter so all jobs (kept + dropped) are stored.
func (s *Store) BulkInsertAudit(records []*models.AuditRecord) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO scrape_audit
			(id, run_id, url, title, company, location, source, posted_at,
			 det_score, matched_tech, missing_tech, was_kept, final_score, scraped_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range records {
		tech, _ := encodeTech(r.MatchedTech)
		missing, _ := encodeTech(r.MissingTech)
		wasKept := 0
		if r.WasKept {
			wasKept = 1
		}
		if _, err := stmt.Exec(
			r.ID, r.RunID, r.URL, r.Title, r.Company, r.Location, r.Source,
			r.PostedAt.Format(time.RFC3339),
			r.DetScore, tech, missing, wasKept,
			r.DetScore, // final_score starts equal to det_score
			r.ScrapedAt.Format(time.RFC3339),
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpdateAuditFinal sets the LLM-refined score and reason for one kept job.
func (s *Store) UpdateAuditFinal(id, runID string, finalScore int, reason string, usedLLM bool) error {
	used := 0
	if usedLLM {
		used = 1
	}
	_, err := s.db.Exec(
		`UPDATE scrape_audit SET final_score=?, reason=?, used_llm=? WHERE id=? AND run_id=?`,
		finalScore, reason, used, id, runID,
	)
	return err
}

// ListAuditRuns returns a summary of the last 20 scrape runs, newest first.
func (s *Store) ListAuditRuns() ([]models.AuditRunSummary, error) {
	rows, err := s.db.Query(`
		SELECT run_id, COUNT(*) AS total, SUM(was_kept) AS kept, MIN(scraped_at) AS scraped_at
		FROM scrape_audit
		GROUP BY run_id
		ORDER BY scraped_at DESC
		LIMIT 20`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []models.AuditRunSummary
	for rows.Next() {
		var r models.AuditRunSummary
		var ts string
		if err := rows.Scan(&r.RunID, &r.Total, &r.Kept, &ts); err != nil {
			return nil, err
		}
		r.ScrapedAt = parseTime(ts)
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetAuditRun returns all records for one run, sorted by det_score descending.
func (s *Store) GetAuditRun(runID string) ([]*models.AuditRecord, error) {
	rows, err := s.db.Query(`
		SELECT id, run_id, url, title, company, location, source, posted_at,
		       det_score, matched_tech, missing_tech, was_kept, used_llm, final_score, reason, scraped_at
		FROM scrape_audit
		WHERE run_id = ?
		ORDER BY det_score DESC`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*models.AuditRecord
	for rows.Next() {
		r := &models.AuditRecord{}
		var postedAt, scrapedAt, tech, missing string
		var wasKept, usedLLM int
		if err := rows.Scan(
			&r.ID, &r.RunID, &r.URL, &r.Title, &r.Company, &r.Location, &r.Source, &postedAt,
			&r.DetScore, &tech, &missing, &wasKept, &usedLLM, &r.FinalScore, &r.Reason, &scrapedAt,
		); err != nil {
			return nil, err
		}
		r.PostedAt = parseTime(postedAt)
		r.ScrapedAt = parseTime(scrapedAt)
		r.MatchedTech = decodeTech(tech)
		r.MissingTech = decodeTech(missing)
		r.WasKept = wasKept == 1
		r.UsedLLM = usedLLM == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func encodeTech(t []string) (string, error) {
	if t == nil {
		return "[]", nil
	}
	b, err := json.Marshal(t)
	return string(b), err
}

func decodeTech(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil
	}
	return out
}

// parseTime tries multiple datetime formats SQLite may return.
// Returns the zero time on failure (not time.Now() — we don't want to
// silently misrepresent unparseable rows as fresh).
func parseTime(s string) time.Time {
	for _, l := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	} {
		if t, err := time.Parse(l, s); err == nil && t.Year() > 1970 {
			return t
		}
	}
	return time.Time{}
}
