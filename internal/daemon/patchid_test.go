package daemon

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/github"
	"github.com/moffa90/pr-sentinel/internal/state"
)

// fingerprintFetcher adds DiffFingerprinter to mockFetcher.
type fingerprintFetcher struct {
	mockFetcher
	patches map[string]string // head → patch ID
	err     error
	calls   int
}

func (f *fingerprintFetcher) PatchID(_, _, head string) (string, error) {
	f.calls++
	if f.err != nil {
		return "", f.err
	}
	return f.patches[head], nil
}

func patchCycle(t *testing.T, recPatch string, f *fingerprintFetcher) (*state.Store, PollResult) {
	t.Helper()
	store := testStore(t)
	cfg := testConfig(config.RepoConfig{Name: "o/r", Path: missingRepo, Mode: config.ModeDryRun})
	if recPatch != "-" {
		if err := store.RecordReview(state.ReviewRecord{Repo: "o/r", PRNumber: 3, HeadOID: "h1", PatchID: recPatch, ReviewedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	return store, RunPollCycleWith(context.Background(), cfg, store, nil, f)
}

func rebasedPR() map[string][]github.PullRequest {
	return map[string][]github.PullRequest{
		"o/r": {{Repo: "o/r", Number: 3, HeadOID: "h2", BaseRef: "main", CommitOIDs: []string{"r1", "h2"}}},
	}
}

func TestUnchangedDiff_RebaseIsNotReReviewed(t *testing.T) {
	f := &fingerprintFetcher{mockFetcher: mockFetcher{prs: rebasedPR()}, patches: map[string]string{"h2": "same"}}
	store, r := patchCycle(t, "same", f)

	if r.Errors != 0 || r.Skipped != 1 {
		t.Fatalf("errors=%d skipped=%d, want 0/1 (no paid run for an identical change)", r.Errors, r.Skipped)
	}
	if rec, _ := store.GetReview("o/r", 3); rec.HeadOID != "h2" {
		t.Errorf("head = %q, want h2 recorded as reviewed", rec.HeadOID)
	}
	if n, _ := store.GetDailyCount(time.Now().UTC().Format("2006-01-02")); n != 0 {
		t.Errorf("daily count = %d, want 0", n)
	}

	// Next cycle: the head is recorded, so the diff isn't fetched again.
	cfg := testConfig(config.RepoConfig{Name: "o/r", Path: missingRepo, Mode: config.ModeDryRun})
	calls := f.calls
	RunPollCycleWith(context.Background(), cfg, store, nil, f)
	if f.calls != calls {
		t.Errorf("diff fingerprinted again (%d calls), want none once the head is recorded", f.calls-calls)
	}
}

func TestUnchangedDiff_RealChangeIsReviewed(t *testing.T) {
	f := &fingerprintFetcher{mockFetcher: mockFetcher{prs: rebasedPR()}, patches: map[string]string{"h2": "different"}}
	_, r := patchCycle(t, "old", f)
	if r.Errors != 1 {
		t.Errorf("errors = %d, want 1 (review queued, fails by design)", r.Errors)
	}
}

func TestUnchangedDiff_UnknownStoredPatchIsReviewed(t *testing.T) {
	f := &fingerprintFetcher{mockFetcher: mockFetcher{prs: rebasedPR()}, patches: map[string]string{"h2": "x"}}
	_, r := patchCycle(t, "", f)
	if r.Errors != 1 {
		t.Errorf("errors = %d, want 1 (no stored fingerprint, so review)", r.Errors)
	}
}

func TestUnchangedDiff_EmptyDiffIsSkipped(t *testing.T) {
	f := &fingerprintFetcher{mockFetcher: mockFetcher{prs: rebasedPR()}, patches: map[string]string{"h2": ""}}
	_, r := patchCycle(t, "-", f)
	if r.Errors != 0 || r.Skipped != 1 {
		t.Errorf("errors=%d skipped=%d, want 0/1 (nothing to review)", r.Errors, r.Skipped)
	}
}

func TestUnchangedDiff_FingerprintErrorFailsOpen(t *testing.T) {
	f := &fingerprintFetcher{mockFetcher: mockFetcher{prs: rebasedPR()}, err: errors.New("HTTP 406: diff too large")}
	_, r := patchCycle(t, "same", f)
	if r.Errors != 1 {
		t.Errorf("errors = %d, want 1 (reviewed despite the fingerprint error)", r.Errors)
	}
}

func TestProcessReviewWith_RecordsPatchID(t *testing.T) {
	store, gh := testStore(t), &mockGitHub{}
	repo := config.RepoConfig{Name: "o/r", Mode: config.ModeLive}
	if _, err := ProcessReviewWith(store, nil, PollOptions{}, repo, github.PullRequest{Number: 1, HeadOID: "h", PatchID: "p1"}, structuredResult(), gh); err != nil {
		t.Fatal(err)
	}
	if rec, _ := store.GetReview("o/r", 1); rec.PatchID != "p1" {
		t.Errorf("PatchID = %q, want p1", rec.PatchID)
	}
}
