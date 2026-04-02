package notifier

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// --- Event JSON roundtrip ---

func TestEventJSONRoundtrip(t *testing.T) {
	original := Event{
		Repo:            "owner/repo",
		PRNumber:        42,
		PRTitle:         "Add feature X",
		PRAuthor:        "alice",
		PRURL:           "https://github.com/owner/repo/pull/42",
		Mode:            "full",
		Posted:          true,
		FindingsSummary: "3 issues found",
		ReviewPath:      "/tmp/review.md",
		Timestamp:       "2026-04-01T12:00:00Z",
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded Event
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded != original {
		t.Errorf("roundtrip mismatch:\n got  %+v\n want %+v", decoded, original)
	}
}

func TestEventJSONOmitEmptyReviewPath(t *testing.T) {
	e := Event{Repo: "r", PRNumber: 1}
	data, _ := json.Marshal(e)

	var m map[string]interface{}
	json.Unmarshal(data, &m)

	if _, ok := m["review_path"]; ok {
		t.Error("review_path should be omitted when empty")
	}
}

// --- Dispatcher ---

type mockNotifier struct {
	called bool
	err    error
}

func (m *mockNotifier) Notify(_ Event) error {
	m.called = true
	return m.err
}

func TestDispatcherCallsAll(t *testing.T) {
	n1 := &mockNotifier{}
	n2 := &mockNotifier{}
	n3 := &mockNotifier{}

	d := NewDispatcher(n1, n2, n3)
	err := d.Notify(Event{Repo: "test"})

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	for i, n := range []*mockNotifier{n1, n2, n3} {
		if !n.called {
			t.Errorf("notifier %d was not called", i)
		}
	}
}

func TestDispatcherCollectsErrors(t *testing.T) {
	n1 := &mockNotifier{err: errors.New("fail-1")}
	n2 := &mockNotifier{}
	n3 := &mockNotifier{err: errors.New("fail-3")}

	d := NewDispatcher(n1, n2, n3)
	err := d.Notify(Event{})

	if err == nil {
		t.Fatal("expected error")
	}
	if !n2.called {
		t.Error("notifier 2 should still be called despite other errors")
	}
	errMsg := err.Error()
	if !(contains(errMsg, "fail-1") && contains(errMsg, "fail-3")) {
		t.Errorf("error should contain both failures, got: %s", errMsg)
	}
}

// --- WebhookNotifier with httptest ---

func TestWebhookNotifier(t *testing.T) {
	var received Event

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
		}
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &received); err != nil {
			t.Errorf("unmarshal: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(srv.URL)
	event := Event{
		Repo:            "owner/repo",
		PRNumber:        99,
		PRTitle:         "Fix bug",
		PRAuthor:        "bob",
		PRURL:           "https://github.com/owner/repo/pull/99",
		Mode:            "quick",
		Posted:          false,
		FindingsSummary: "1 issue",
		Timestamp:       "2026-04-01T12:00:00Z",
	}

	if err := n.Notify(event); err != nil {
		t.Fatalf("notify: %v", err)
	}

	if received.Repo != event.Repo {
		t.Errorf("repo: got %q, want %q", received.Repo, event.Repo)
	}
	if received.PRNumber != event.PRNumber {
		t.Errorf("pr_number: got %d, want %d", received.PRNumber, event.PRNumber)
	}
	if received.PRTitle != event.PRTitle {
		t.Errorf("pr_title: got %q, want %q", received.PRTitle, event.PRTitle)
	}
}

func TestWebhookNotifierErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	n := NewWebhookNotifier(srv.URL)
	err := n.Notify(Event{})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

// --- Slack payload ---

func TestBuildSlackPayload(t *testing.T) {
	e := Event{
		Repo:            "owner/repo",
		PRNumber:        10,
		PRTitle:         "Update docs",
		PRAuthor:        "carol",
		PRURL:           "https://github.com/owner/repo/pull/10",
		Mode:            "full",
		Posted:          true,
		FindingsSummary: "no issues",
	}

	p := buildSlackPayload(e)
	if p.Text == "" {
		t.Fatal("slack payload text should not be empty")
	}
	if !contains(p.Text, "pr-sentinel") {
		t.Error("text should contain pr-sentinel")
	}
	if !contains(p.Text, "owner/repo#10") {
		t.Error("text should contain repo#number")
	}
}

// --- Teams payload ---

func TestBuildTeamsPayload(t *testing.T) {
	e := Event{
		Repo:            "owner/repo",
		PRNumber:        5,
		PRTitle:         "Refactor",
		PRAuthor:        "dave",
		PRURL:           "https://github.com/owner/repo/pull/5",
		Mode:            "quick",
		Posted:          false,
		FindingsSummary: "2 warnings",
	}

	p := buildTeamsPayload(e)
	if p["type"] != "message" {
		t.Errorf("type: got %q, want %q", p["type"], "message")
	}
	attachments, ok := p["attachments"].([]interface{})
	if !ok || len(attachments) == 0 {
		t.Fatal("expected at least one attachment")
	}
	att, ok := attachments[0].(map[string]interface{})
	if !ok {
		t.Fatal("attachment should be a map")
	}
	if att["contentType"] != "application/vnd.microsoft.card.adaptive" {
		t.Errorf("contentType: got %q", att["contentType"])
	}
	content, ok := att["content"].(map[string]interface{})
	if !ok {
		t.Fatal("content should be a map")
	}
	if content["type"] != "AdaptiveCard" {
		t.Errorf("card type: got %q", content["type"])
	}
	body, ok := content["body"].([]interface{})
	if !ok || len(body) == 0 {
		t.Error("card body should not be empty")
	}
	actions, ok := content["actions"].([]interface{})
	if !ok || len(actions) == 0 {
		t.Error("card should have actions")
	}
}

// --- helper ---

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
