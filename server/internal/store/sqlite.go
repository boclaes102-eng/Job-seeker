package store

import (
	"database/sql"
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

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
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
			status      TEXT NOT NULL DEFAULT 'new'
		)
	`)
	return err
}

func (s *Store) Upsert(j *models.Job) error {
	_, err := s.db.Exec(`
		INSERT INTO jobs (id, title, company, location, description, url, source, posted_at, fetched_at, match_score, match_reason, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(url) DO UPDATE SET
			title        = excluded.title,
			company      = excluded.company,
			location     = excluded.location,
			description  = excluded.description,
			fetched_at   = excluded.fetched_at,
			match_score  = CASE WHEN jobs.match_score > 0 THEN jobs.match_score ELSE excluded.match_score END,
			match_reason = CASE WHEN jobs.match_reason != '' THEN jobs.match_reason ELSE excluded.match_reason END
	`,
		j.ID, j.Title, j.Company, j.Location, j.Description,
		j.URL, j.Source, j.PostedAt.Format(time.RFC3339), j.FetchedAt.Format(time.RFC3339),
		j.MatchScore, j.MatchReason, j.Status,
	)
	return err
}

type ListOptions struct {
	Source  string
	Status  string
	NewOnly bool
}

func (s *Store) List(opts ListOptions) ([]*models.Job, error) {
	query := `SELECT id, title, company, location, description, url, source, posted_at, fetched_at, match_score, match_reason, status FROM jobs WHERE 1=1`
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
		var postedAt, fetchedAt string
		if err := rows.Scan(&j.ID, &j.Title, &j.Company, &j.Location, &j.Description,
			&j.URL, &j.Source, &postedAt, &fetchedAt,
			&j.MatchScore, &j.MatchReason, &j.Status); err != nil {
			return nil, err
		}
		j.PostedAt = parseTime(postedAt)
		j.FetchedAt = parseTime(fetchedAt)
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
	_, err := s.db.Exec(`UPDATE jobs SET status = ? WHERE id = ?`, status, id)
	return err
}

func (s *Store) UpdateMatch(id string, score int, reason string) error {
	_, err := s.db.Exec(`UPDATE jobs SET match_score = ?, match_reason = ? WHERE id = ?`, score, reason, id)
	return err
}

func (s *Store) Get(id string) (*models.Job, error) {
	j := &models.Job{}
	var postedAt, fetchedAt string
	err := s.db.QueryRow(`
		SELECT id, title, company, location, description, url, source, posted_at, fetched_at, match_score, match_reason, status
		FROM jobs WHERE id = ?`, id).
		Scan(&j.ID, &j.Title, &j.Company, &j.Location, &j.Description,
			&j.URL, &j.Source, &postedAt, &fetchedAt,
			&j.MatchScore, &j.MatchReason, &j.Status)
	if err != nil {
		return nil, err
	}
	j.PostedAt = parseTime(postedAt)
	j.FetchedAt = parseTime(fetchedAt)
	return j, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

// parseTime handles the various datetime formats SQLite may return.
// Falls back to time.Now() so jobs are never hidden by a failed parse.
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
	return time.Now()
}
