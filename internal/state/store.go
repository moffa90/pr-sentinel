package state

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
	CostUSD         float64
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
	cost_usd         REAL    NOT NULL DEFAULT 0,
	closed_at        TEXT    NOT NULL DEFAULT '',
	reviewed_at      TEXT    NOT NULL
);

CREATE TABLE IF NOT EXISTS daily_counts (
	date  TEXT PRIMARY KEY,
	count INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_reviewed_prs_repo_pr ON reviewed_prs(repo, pr_number);`

	_, err := db.Exec(ddl)
	if err != nil {
		return err
	}

	// Migration: drop UNIQUE constraint from existing databases.
	// SQLite doesn't support ALTER TABLE DROP CONSTRAINT, so we recreate.
	if err := migrateDropUnique(db); err != nil {
		return err
	}

	// Add cost_usd column if missing (existing databases)
	if err := migrateAddCostColumn(db); err != nil {
		return err
	}

	// Add closed_at column if missing
	return migrateAddClosedAtColumn(db)
}

// migrateDropUnique recreates reviewed_prs without the UNIQUE(repo, pr_number)
// constraint if the old schema is detected.
func migrateDropUnique(db *sql.DB) error {
	// Check if old UNIQUE constraint exists by trying to inspect the table SQL
	var tableSql string
	err := db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='reviewed_prs'").Scan(&tableSql)
	if err != nil {
		return nil // table doesn't exist yet, DDL above created it correctly
	}

	if !strings.Contains(tableSql, "UNIQUE") {
		return nil // already migrated
	}

	// Recreate without UNIQUE
	const migration = `
BEGIN;
CREATE TABLE reviewed_prs_new (
	id               INTEGER PRIMARY KEY AUTOINCREMENT,
	repo             TEXT    NOT NULL,
	pr_number        INTEGER NOT NULL,
	pr_title         TEXT    NOT NULL DEFAULT '',
	pr_author        TEXT    NOT NULL DEFAULT '',
	review_output    TEXT    NOT NULL DEFAULT '',
	findings_summary TEXT    NOT NULL DEFAULT '',
	mode             TEXT    NOT NULL DEFAULT '',
	posted           INTEGER NOT NULL DEFAULT 0,
	reviewed_at      TEXT    NOT NULL
);
INSERT INTO reviewed_prs_new SELECT * FROM reviewed_prs;
DROP TABLE reviewed_prs;
ALTER TABLE reviewed_prs_new RENAME TO reviewed_prs;
CREATE INDEX IF NOT EXISTS idx_reviewed_prs_repo_pr ON reviewed_prs(repo, pr_number);
COMMIT;`

	_, err = db.Exec(migration)
	return err
}

// migrateAddCostColumn adds cost_usd column to existing databases.
func migrateAddCostColumn(db *sql.DB) error {
	var tableSql string
	err := db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='reviewed_prs'").Scan(&tableSql)
	if err != nil {
		return nil
	}
	if strings.Contains(tableSql, "cost_usd") {
		return nil // already has the column
	}
	_, err = db.Exec("ALTER TABLE reviewed_prs ADD COLUMN cost_usd REAL NOT NULL DEFAULT 0")
	return err
}

// RecordReview appends a review record. Multiple reviews per PR are preserved.
func (s *Store) RecordReview(r ReviewRecord) error {
	const query = `
INSERT INTO reviewed_prs (repo, pr_number, pr_title, pr_author, review_output, findings_summary, mode, posted, cost_usd, reviewed_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	posted := 0
	if r.Posted {
		posted = 1
	}

	_, err := s.db.Exec(query,
		r.Repo, r.PRNumber, r.PRTitle, r.PRAuthor,
		r.ReviewOutput, r.FindingsSummary, r.Mode,
		posted, r.CostUSD, r.ReviewedAt.UTC().Format(time.RFC3339),
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

// GetReview retrieves the most recent review record by repo and PR number.
func (s *Store) GetReview(repo string, prNumber int64) (ReviewRecord, error) {
	var r ReviewRecord
	var posted int
	var reviewedAt string

	err := s.db.QueryRow(
		`SELECT id, repo, pr_number, pr_title, pr_author, review_output, findings_summary, mode, posted, cost_usd, reviewed_at
		 FROM reviewed_prs WHERE repo = ? AND pr_number = ?
		 ORDER BY reviewed_at DESC LIMIT 1`,
		repo, prNumber,
	).Scan(&r.ID, &r.Repo, &r.PRNumber, &r.PRTitle, &r.PRAuthor,
		&r.ReviewOutput, &r.FindingsSummary, &r.Mode, &posted, &r.CostUSD, &reviewedAt)
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

// DailyCost returns the total cost in USD for reviews on the given date.
func (s *Store) DailyCost(date string) (float64, error) {
	var cost sql.NullFloat64
	err := s.db.QueryRow(
		`SELECT SUM(cost_usd) FROM reviewed_prs WHERE reviewed_at >= ? AND reviewed_at < ?`,
		date+"T00:00:00Z", date+"T23:59:59Z",
	).Scan(&cost)
	if err != nil {
		return 0, err
	}
	if !cost.Valid {
		return 0, nil
	}
	return cost.Float64, nil
}

// RecentReviews returns the most recent review records ordered by reviewed_at descending.
func (s *Store) RecentReviews(limit int) ([]ReviewRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, repo, pr_number, pr_title, pr_author, review_output, findings_summary, mode, posted, cost_usd, reviewed_at
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
			&r.ReviewOutput, &r.FindingsSummary, &r.Mode, &posted, &r.CostUSD, &reviewedAt); err != nil {
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

// TrackedOpenPRNumbers returns distinct PR numbers for a repo that are not marked closed.
func (s *Store) TrackedOpenPRNumbers(repo string) ([]int64, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT pr_number FROM reviewed_prs
		 WHERE repo = ? AND (closed_at = '' OR closed_at IS NULL)`,
		repo,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var numbers []int64
	for rows.Next() {
		var n int64
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		numbers = append(numbers, n)
	}
	return numbers, rows.Err()
}

// MarkPRClosed sets closed_at on all records for the given repo+pr_number.
func (s *Store) MarkPRClosed(repo string, prNumber int64) error {
	_, err := s.db.Exec(
		`UPDATE reviewed_prs SET closed_at = ? WHERE repo = ? AND pr_number = ? AND (closed_at = '' OR closed_at IS NULL)`,
		time.Now().UTC().Format(time.RFC3339), repo, prNumber,
	)
	return err
}

// migrateAddClosedAtColumn adds closed_at column to existing databases.
func migrateAddClosedAtColumn(db *sql.DB) error {
	var tableSql string
	err := db.QueryRow("SELECT sql FROM sqlite_master WHERE type='table' AND name='reviewed_prs'").Scan(&tableSql)
	if err != nil {
		return nil
	}
	if strings.Contains(tableSql, "closed_at") {
		return nil
	}
	_, err = db.Exec("ALTER TABLE reviewed_prs ADD COLUMN closed_at TEXT NOT NULL DEFAULT ''")
	return err
}
