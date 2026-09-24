package daemon

import (
	"errors"
	"strings"
	"testing"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/github"
	"github.com/moffa90/pr-sentinel/internal/reviewer"
)

type mockIssueClient struct {
	created   int
	commented []int64
	lastBody  string
	lastLabel []string
	err       error
}

func (m *mockIssueClient) CreateIssue(repo, title, body string, labels []string) (int64, string, error) {
	if m.err != nil {
		return 0, "", m.err
	}
	m.created++
	m.lastBody = body
	m.lastLabel = labels
	return 100, "https://github.com/" + repo + "/issues/100", nil
}

func (m *mockIssueClient) CommentOnIssue(_ string, number int64, body string) error {
	if m.err != nil {
		return m.err
	}
	m.commented = append(m.commented, number)
	m.lastBody = body
	return nil
}

func TestQualifyingFindings(t *testing.T) {
	findings := []reviewer.Finding{
		{Severity: "HIGH", File: "a.go"},
		{Severity: "MEDIUM", File: "b.go"},
		{Severity: "LOW", File: "c.go"},
		{Severity: "bogus", File: "d.go"},
	}
	tests := []struct {
		min  string
		want int
	}{
		{"", 1}, // defaults to HIGH
		{"HIGH", 1},
		{"medium", 2},
		{"LOW", 3},
	}
	for _, tt := range tests {
		t.Run("min="+tt.min, func(t *testing.T) {
			got := qualifyingFindings(config.IssuesConfig{MinSeverity: tt.min}, findings)
			if len(got) != tt.want {
				t.Errorf("got %d findings, want %d", len(got), tt.want)
			}
		})
	}
}

func TestBuildIssueBody(t *testing.T) {
	pr := github.PullRequest{Number: 7, Author: "alice"}
	findings := []reviewer.Finding{
		{Severity: "HIGH", File: "main.go", Line: 12, Message: "nil deref"},
		{Severity: "HIGH", File: "util.go", Message: "leak"},
	}

	body := buildIssueBody(pr, findings, false)
	for _, want := range []string{"#7", "@alice", "- [ ] **HIGH** `main.go:12` — nil deref", "`util.go` — leak"} {
		if !strings.Contains(body, want) {
			t.Errorf("body missing %q:\n%s", want, body)
		}
	}

	followUp := buildIssueBody(pr, findings, true)
	if !strings.Contains(followUp, "Follow-up review of #7") {
		t.Errorf("follow-up body missing header:\n%s", followUp)
	}
}

func TestHandleIssues(t *testing.T) {
	pr := github.PullRequest{Number: 5, Title: "t", Author: "bob"}
	high := &reviewer.StructuredReview{Findings: []reviewer.Finding{{Severity: "HIGH", File: "x.go", Message: "bad"}}}
	lowOnly := &reviewer.StructuredReview{Findings: []reviewer.Finding{{Severity: "LOW", File: "x.go", Message: "nit"}}}
	liveRepo := config.RepoConfig{Name: "o/r", Mode: config.ModeLive, Issues: config.IssuesConfig{Enabled: true, Labels: []string{"pr-sentinel"}}}

	t.Run("disabled does nothing", func(t *testing.T) {
		store, client := testStore(t), &mockIssueClient{}
		repo := liveRepo
		repo.Issues.Enabled = false
		if got := handleIssues(store, client, repo, pr, high); got != "" || client.created != 0 {
			t.Errorf("status=%q created=%d", got, client.created)
		}
	})

	t.Run("nil review does nothing", func(t *testing.T) {
		store, client := testStore(t), &mockIssueClient{}
		if got := handleIssues(store, client, liveRepo, pr, nil); got != "" {
			t.Errorf("status=%q", got)
		}
	})

	t.Run("below threshold does nothing", func(t *testing.T) {
		store, client := testStore(t), &mockIssueClient{}
		if got := handleIssues(store, client, liveRepo, pr, lowOnly); got != "" || client.created != 0 {
			t.Errorf("status=%q created=%d", got, client.created)
		}
	})

	t.Run("creates then comments on follow-up", func(t *testing.T) {
		store, client := testStore(t), &mockIssueClient{}

		if got := handleIssues(store, client, liveRepo, pr, high); got != "Created #100" {
			t.Fatalf("first status = %q", got)
		}
		if client.created != 1 || len(client.lastLabel) != 1 || client.lastLabel[0] != "pr-sentinel" {
			t.Fatalf("created=%d labels=%v", client.created, client.lastLabel)
		}

		if got := handleIssues(store, client, liveRepo, pr, high); got != "Commented on #100" {
			t.Fatalf("second status = %q", got)
		}
		if client.created != 1 || len(client.commented) != 1 || client.commented[0] != 100 {
			t.Errorf("created=%d commented=%v", client.created, client.commented)
		}
	})

	t.Run("dry-run does not call client", func(t *testing.T) {
		store, client := testStore(t), &mockIssueClient{}
		repo := liveRepo
		repo.Mode = config.ModeDryRun
		if got := handleIssues(store, client, repo, pr, high); got != "Would create issue" {
			t.Errorf("status = %q", got)
		}
		if client.created != 0 {
			t.Errorf("created=%d, want 0", client.created)
		}
		if _, found, _ := store.GetIssue("o/r", 5); found {
			t.Error("dry-run should not record an issue")
		}
	})

	t.Run("create failure is not recorded", func(t *testing.T) {
		store, client := testStore(t), &mockIssueClient{err: errors.New("label not found")}
		if got := handleIssues(store, client, liveRepo, pr, high); !strings.HasPrefix(got, "Failed:") {
			t.Errorf("status = %q", got)
		}
		if _, found, _ := store.GetIssue("o/r", 5); found {
			t.Error("failed create should not record an issue")
		}
	})
}
