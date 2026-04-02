package state

import (
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q): %v", path, err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestOpenClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oc.db")
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestRecordReviewAndHasReviewed(t *testing.T) {
	s := newTestStore(t)

	repo := "owner/repo"
	var prNum int64 = 42

	reviewed, err := s.HasReviewed(repo, prNum)
	if err != nil {
		t.Fatalf("HasReviewed: %v", err)
	}
	if reviewed {
		t.Fatal("expected HasReviewed=false before insert")
	}

	rec := ReviewRecord{
		Repo:            repo,
		PRNumber:        prNum,
		PRTitle:         "Add feature",
		PRAuthor:        "alice",
		ReviewOutput:    "looks good",
		FindingsSummary: "no issues",
		Mode:            "full",
		Posted:          true,
		ReviewedAt:      time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC),
	}
	if err := s.RecordReview(rec); err != nil {
		t.Fatalf("RecordReview: %v", err)
	}

	reviewed, err = s.HasReviewed(repo, prNum)
	if err != nil {
		t.Fatalf("HasReviewed after insert: %v", err)
	}
	if !reviewed {
		t.Fatal("expected HasReviewed=true after insert")
	}

	got, err := s.GetReview(repo, prNum)
	if err != nil {
		t.Fatalf("GetReview: %v", err)
	}
	if got.PRTitle != "Add feature" {
		t.Errorf("PRTitle = %q, want %q", got.PRTitle, "Add feature")
	}
	if got.PRAuthor != "alice" {
		t.Errorf("PRAuthor = %q, want %q", got.PRAuthor, "alice")
	}
	if !got.Posted {
		t.Error("expected Posted=true")
	}
}

func TestMultipleReviewsPreserved(t *testing.T) {
	s := newTestStore(t)

	first := ReviewRecord{
		Repo:         "owner/repo",
		PRNumber:     10,
		PRTitle:      "first",
		ReviewOutput: "v1",
		ReviewedAt:   time.Now().UTC().Add(-time.Hour),
	}
	if err := s.RecordReview(first); err != nil {
		t.Fatalf("first insert: %v", err)
	}

	second := ReviewRecord{
		Repo:         "owner/repo",
		PRNumber:     10,
		PRTitle:      "updated",
		ReviewOutput: "v2",
		Posted:       true,
		ReviewedAt:   time.Now().UTC(),
	}
	if err := s.RecordReview(second); err != nil {
		t.Fatalf("second insert: %v", err)
	}

	// GetReview returns the latest
	got, err := s.GetReview("owner/repo", 10)
	if err != nil {
		t.Fatalf("GetReview: %v", err)
	}
	if got.PRTitle != "updated" {
		t.Errorf("PRTitle = %q, want %q", got.PRTitle, "updated")
	}
	if got.ReviewOutput != "v2" {
		t.Errorf("ReviewOutput = %q, want %q", got.ReviewOutput, "v2")
	}
	if !got.Posted {
		t.Error("expected Posted=true for latest review")
	}

	// HasReviewed still works
	reviewed, err := s.HasReviewed("owner/repo", 10)
	if err != nil {
		t.Fatalf("HasReviewed: %v", err)
	}
	if !reviewed {
		t.Error("expected HasReviewed=true")
	}
}

func TestDailyCount(t *testing.T) {
	s := newTestStore(t)
	date := "2026-04-01"

	count, err := s.GetDailyCount(date)
	if err != nil {
		t.Fatalf("GetDailyCount: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}

	for i := 0; i < 3; i++ {
		if err := s.IncrementDailyCount(date); err != nil {
			t.Fatalf("IncrementDailyCount #%d: %v", i+1, err)
		}
	}

	count, err = s.GetDailyCount(date)
	if err != nil {
		t.Fatalf("GetDailyCount after increments: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3, got %d", count)
	}
}

func TestRecentReviews(t *testing.T) {
	s := newTestStore(t)

	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		rec := ReviewRecord{
			Repo:       "owner/repo",
			PRNumber:   int64(i + 1),
			PRTitle:    "PR " + time.Duration(i).String(),
			ReviewedAt: base.Add(time.Duration(i) * time.Hour),
		}
		if err := s.RecordReview(rec); err != nil {
			t.Fatalf("RecordReview #%d: %v", i+1, err)
		}
	}

	// Fetch top 3 — should be PRs 5, 4, 3 (most recent first)
	recent, err := s.RecentReviews(3)
	if err != nil {
		t.Fatalf("RecentReviews: %v", err)
	}
	if len(recent) != 3 {
		t.Fatalf("expected 3 records, got %d", len(recent))
	}
	if recent[0].PRNumber != 5 {
		t.Errorf("first record PRNumber = %d, want 5", recent[0].PRNumber)
	}
	if recent[1].PRNumber != 4 {
		t.Errorf("second record PRNumber = %d, want 4", recent[1].PRNumber)
	}
	if recent[2].PRNumber != 3 {
		t.Errorf("third record PRNumber = %d, want 3", recent[2].PRNumber)
	}
}
