package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// ReviewRecord represents a single PR review stored in the database.
type ReviewRecord struct {
	ID              int64
	Repo            string
	PRNumber        int64
	PRTitle         string
	PRAuthor        string
	ReviewOutput    string
	FindingsSummary string
	Mode            string
	Posted          bool
	ReviewedAt      time.Time
}

// DefaultDBPath returns the default path to the SQLite database.
func DefaultDBPath() string {
	return filepath.Join(configDir(), "state.db")
}

// configDir returns the pr-sentinel config directory.
func configDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", "pr-sentinel")
	}
	return filepath.Join(home, ".config", "pr-sentinel")
}

// Store wraps a SQLite database connection for persisting review state.
type Store struct {
	db *sql.DB
}

// Open creates or opens the SQLite database at path and runs migrations.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &Store{db: db}, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

func migrate(db *sql.DB) error {
	const ddl = `
CREATE TABLE IF NOT EXISTS reviewed_prs (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	repo             TEXT    NOT NULL,
	pr_number        INTEGER NOT NULL,
	pr_title         TEXT    NOT NULL DEFAULT '',
	pr_author        TEXT    NOT NULL DEFAULT '',
	review_output    TEXT    NOT NULL DEFAULT '',
	findings_summary TEXT    NOT NULL DEFAULT '',
	mode             TEXT    NOT NULL DEFAULT '',
	posted           INTEGER NOT NULL DEFAULT 0,
	reviewed_at      TEXT    NOT NULL,
	UNIQUE(repo, pr_number)
);

CREATE TABLE IF NOT EXISTS daily_counts (
	date  TEXT PRIMARY KEY,
	count INTEGER NOT NULL DEFAULT 0
);`

	_, err := db.Exec(ddl)
	return err
}

// RecordReview inserts or updates a review record (upsert on repo+pr_number).
func (s *Store) RecordReview(r ReviewRecord) error {
	const query = `
INSERT INTO reviewed_prs (repo, pr_number, pr_title, pr_author, review_output, findings_summary, mode, posted, reviewed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(repo, pr_number) DO UPDATE SET
	pr_title         = excluded.pr_title,
	pr_author        = excluded.pr_author,
	review_output    = excluded.review_output,
	findings_summary = excluded.findings_summary,
	mode             = excluded.mode,
	posted           = excluded.posted,
	reviewed_at      = excluded.reviewed_at`

	posted := 0
	if r.Posted {
		posted = 1
	}

	_, err := s.db.Exec(query,
		r.Repo, r.PRNumber, r.PRTitle, r.PRAuthor,
		r.ReviewOutput, r.FindingsSummary, r.Mode,
		posted, r.ReviewedAt.UTC().Format(time.RFC3339),
	)
	return err
}

// HasReviewed returns true if a review record exists for the given repo and PR number.
func (s *Store) HasReviewed(repo string, prNumber int64) (bool, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM reviewed_prs WHERE repo = ? AND pr_number = ?`,
		repo, prNumber,
	).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetReview retrieves a single review record by repo and PR number.
func (s *Store) GetReview(repo string, prNumber int64) (ReviewRecord, error) {
	var r ReviewRecord
	var posted int
	var reviewedAt string

	err := s.db.QueryRow(
		`SELECT id, repo, pr_number, pr_title, pr_author, review_output, findings_summary, mode, posted, reviewed_at
		 FROM reviewed_prs WHERE repo = ? AND pr_number = ?`,
		repo, prNumber,
	).Scan(&r.ID, &r.Repo, &r.PRNumber, &r.PRTitle, &r.PRAuthor,
		&r.ReviewOutput, &r.FindingsSummary, &r.Mode, &posted, &reviewedAt)
	if err != nil {
		return ReviewRecord{}, err
	}

	r.Posted = posted != 0
	r.ReviewedAt, err = time.Parse(time.RFC3339, reviewedAt)
	if err != nil {
		return ReviewRecord{}, fmt.Errorf("parsing reviewed_at %q: %w", reviewedAt, err)
	}
	return r, nil
}

// GetDailyCount returns the review count for the given date string (e.g. "2026-04-01").
// Returns 0 if no row exists.
func (s *Store) GetDailyCount(date string) (int, error) {
	var count int
	err := s.db.QueryRow(
		`SELECT count FROM daily_counts WHERE date = ?`, date,
	).Scan(&count)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	return count, err
}

// IncrementDailyCount increments the daily count for the given date, inserting if needed.
func (s *Store) IncrementDailyCount(date string) error {
	_, err := s.db.Exec(
		`INSERT INTO daily_counts (date, count) VALUES (?, 1)
		 ON CONFLICT(date) DO UPDATE SET count = count + 1`,
		date,
	)
	return err
}

// RecentReviews returns the most recent review records ordered by reviewed_at descending.
func (s *Store) RecentReviews(limit int) ([]ReviewRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, repo, pr_number, pr_title, pr_author, review_output, findings_summary, mode, posted, reviewed_at
		 FROM reviewed_prs ORDER BY reviewed_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []ReviewRecord
	for rows.Next() {
		var r ReviewRecord
		var posted int
		var reviewedAt string

		if err := rows.Scan(&r.ID, &r.Repo, &r.PRNumber, &r.PRTitle, &r.PRAuthor,
			&r.ReviewOutput, &r.FindingsSummary, &r.Mode, &posted, &reviewedAt); err != nil {
			return nil, err
		}

		r.Posted = posted != 0
		r.ReviewedAt, err = time.Parse(time.RFC3339, reviewedAt)
		if err != nil {
			return nil, fmt.Errorf("parsing reviewed_at %q for %s#%d: %w", reviewedAt, r.Repo, r.PRNumber, err)
		}
		records = append(records, r)
	}
	return records, rows.Err()
}
