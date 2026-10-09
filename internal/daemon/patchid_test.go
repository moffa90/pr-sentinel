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

func TestUnchangedDiff(t *testing.T) {
	tests := []struct {
		name        string
		storedPatch string // "-" for no stored review
		fetched     string
		fetchErr    error
		wantErrors  int // a queued review fails by design (missingRepo)
		wantSkipped int
	}{
		{name: "identical change is not re-reviewed", storedPatch: "same", fetched: "same", wantSkipped: 1},
		{name: "real change is reviewed", storedPatch: "old", fetched: "different", wantErrors: 1},
		{name: "no stored fingerprint is reviewed", storedPatch: "", fetched: "x", wantErrors: 1},
		{name: "empty diff is skipped", storedPatch: "-", fetched: "", wantSkipped: 1},
		{name: "fingerprint error fails open", storedPatch: "same", fetchErr: errors.New("HTTP 406: diff too large"), wantErrors: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fingerprintFetcher{mockFetcher: mockFetcher{prs: rebasedPR()}, patches: map[string]string{"h2": tt.fetched}, err: tt.fetchErr}
			_, r := patchCycle(t, tt.storedPatch, f)
			if r.Errors != tt.wantErrors || r.Skipped != tt.wantSkipped {
				t.Errorf("errors=%d skipped=%d, want %d/%d", r.Errors, r.Skipped, tt.wantErrors, tt.wantSkipped)
			}
		})
	}
}

// An identical change records the new head and costs nothing; the next cycle
// doesn't fetch the diff again.
func TestUnchangedDiff_RecordsHeadAndDoesNotRefetch(t *testing.T) {
	f := &fingerprintFetcher{mockFetcher: mockFetcher{prs: rebasedPR()}, patches: map[string]string{"h2": "same"}}
	store, _ := patchCycle(t, "same", f)

	if rec, _ := store.GetReview("o/r", 3); rec.HeadOID != "h2" {
		t.Errorf("head = %q, want h2 recorded as reviewed", rec.HeadOID)
	}
	if n, _ := store.GetDailyCount(time.Now().UTC().Format("2006-01-02")); n != 0 {
		t.Errorf("daily count = %d, want 0", n)
	}
	cfg := testConfig(config.RepoConfig{Name: "o/r", Path: missingRepo, Mode: config.ModeDryRun})
	calls := f.calls
	RunPollCycleWith(context.Background(), cfg, store, nil, f)
	if f.calls != calls {
		t.Errorf("diff fingerprinted again (%d calls), want none once the head is recorded", f.calls-calls)
	}
}

// With the budget spent, no diff is fetched at all.
func TestUnchangedDiff_SpentBudgetFetchesNothing(t *testing.T) {
	store := testStore(t)
	cfg := testConfig(config.RepoConfig{Name: "o/r", Path: missingRepo, Mode: config.ModeDryRun})
	cfg.MaxReviewsPerDay = 1
	if err := store.IncrementDailyCount(time.Now().UTC().Format("2006-01-02")); err != nil {
		t.Fatal(err)
	}
	f := &fingerprintFetcher{mockFetcher: mockFetcher{prs: rebasedPR()}, patches: map[string]string{"h2": "x"}}
	RunPollCycleWith(context.Background(), cfg, store, nil, f)
	if f.calls != 0 {
		t.Errorf("fingerprinted %d times with the budget spent, want 0", f.calls)
	}
}

// The stored fingerprint only counts when that review is the follow-up's
// baseline; a newer review from another host has no fingerprint here.
func TestUnchangedDiff_OnlyAgainstTheBaseline(t *testing.T) {
	store := testStore(t)
	cfg := testConfig(config.RepoConfig{Name: "o/r", Path: missingRepo, Mode: config.ModeLive})
	old := time.Now().Add(-2 * time.Hour).UTC()
	if err := store.RecordReview(state.ReviewRecord{Repo: "o/r", PRNumber: 3, HeadOID: "h1", PatchID: "same", ReviewedAt: old}); err != nil {
		t.Fatal(err)
	}
	f := &fingerprintFetcher{
		mockFetcher: mockFetcher{followUps: map[string][]github.FollowUpCandidate{"o/r": {{
			PullRequest:   github.PullRequest{Repo: "o/r", Number: 3, HeadOID: "h3", BaseRef: "main", CommitOIDs: []string{"h1", "hX", "h3"}},
			LastReviewAt:  time.Now().Add(-time.Hour), // newer: another host reviewed hX
			LastReviewOID: "hX",
		}}}},
		patches: map[string]string{"h3": "same"},
	}
	r := RunPollCycleWith(context.Background(), cfg, store, nil, f)
	if r.Errors != 1 {
		t.Errorf("errors = %d, want 1 (reviewed: the matching fingerprint is not the baseline's)", r.Errors)
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
