package reviewer

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDefaultTimeout(t *testing.T) {
	expected := 5 * time.Minute
	if DefaultTimeout != expected {
		t.Errorf("DefaultTimeout = %v, want %v", DefaultTimeout, expected)
	}
}

func TestBuildReviewPrompt(t *testing.T) {
	p := ReviewParams{
		Repo:     "owner/repo",
		PRNumber: 42,
		PRTitle:  "Add feature X",
		PRAuthor: "johndoe",
		Files:    3,
		Adds:     10,
		Dels:     5,
	}

	result := BuildReviewPrompt(p)

	checks := []struct {
		name     string
		contains string
	}{
		{"PR reference", "owner/repo#42"},
		{"title", "Add feature X"},
		{"author", "@johndoe"},
		{"files stat", "3 files changed"},
		{"additions stat", "10 additions"},
		{"deletions stat", "5 deletions"},
		{"severity HIGH", "HIGH"},
		{"severity MEDIUM", "MEDIUM"},
		{"severity LOW", "LOW"},
		{"verdict instruction", "verdict"},
		{"gh pr diff command", "gh pr diff"},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(result, c.contains) {
				t.Errorf("prompt missing %q", c.contains)
			}
		})
	}
}

func TestBuildClaudeArgs_BothInstructions(t *testing.T) {
	args := BuildClaudeArgs("review this", "global rules", "repo rules")

	if args[0] != "-p" || args[1] != "review this" {
		t.Errorf("expected -p flag with prompt, got %v", args[:2])
	}

	hasOutputFormat := false
	hasJSONSchema := false
	hasAllowedTools := false
	appendCount := 0
	for i, a := range args {
		if a == "--output-format" && i+1 < len(args) && args[i+1] == "json" {
			hasOutputFormat = true
		}
		if a == "--json-schema" {
			hasJSONSchema = true
		}
		if a == "--allowedTools" {
			hasAllowedTools = true
		}
		if a == "--append-system-prompt" {
			appendCount++
		}
	}

	if !hasOutputFormat {
		t.Error("expected --output-format json flag")
	}
	if !hasJSONSchema {
		t.Error("expected --json-schema flag")
	}
	if !hasAllowedTools {
		t.Error("expected --allowedTools flag")
	}
	if appendCount != 2 {
		t.Errorf("expected 2 --append-system-prompt flags, got %d", appendCount)
	}
}

func TestRunReview_CompletesWithoutLeak(t *testing.T) {
	ctx := context.Background()
	// Use a fast command that exits immediately
	result := RunReview(ctx, t.TempDir(), "echo hello", "", "", 10*time.Second)
	if result.Error != nil {
		t.Fatalf("unexpected error: %v", result.Error)
	}
	if result.Duration <= 0 {
		t.Error("duration should be positive")
	}
}

func TestBuildClaudeArgs_EmptyInstructions(t *testing.T) {
	args := BuildClaudeArgs("review this", "", "")

	// Should have: -p, prompt, --output-format, json, --json-schema, <schema>, --allowedTools, <tools>
	if len(args) != 8 {
		t.Errorf("expected 8 args, got %d: %v", len(args), args)
	}

	for _, a := range args {
		if a == "--append-system-prompt" {
			t.Error("unexpected --append-system-prompt flag with empty instructions")
		}
	}
}

func TestBuildFollowUpPrompt(t *testing.T) {
	p := FollowUpParams{
		Repo:           "owner/repo",
		PRNumber:       51,
		PRTitle:        "fix: update auth",
		PRAuthor:       "bob",
		Files:          3,
		Adds:           10,
		Dels:           5,
		PreviousReview: "HIGH: Missing error handling in auth.go:42",
		NewCommitCount: 2,
	}

	result := BuildFollowUpPrompt(p)

	checks := []struct {
		name     string
		contains string
	}{
		{"PR reference", "owner/repo#51"},
		{"title", "fix: update auth"},
		{"author", "@bob"},
		{"follow-up label", "Follow-up review"},
		{"previous review", "Missing error handling"},
		{"new commit count", "2 new commit"},
		{"gh pr diff command", "gh pr diff"},
		{"addresses instruction", "whether the new commits address"},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(result, c.contains) {
				t.Errorf("prompt missing %q", c.contains)
			}
		})
	}
}

func TestParseCLIOutput_StructuredOutput(t *testing.T) {
	// When --json-schema is used, the result goes in structured_output
	input := `{"type":"result","subtype":"success","is_error":false,"result":"","structured_output":{"verdict":"comment","summary":"Minor issues found","findings":[{"severity":"LOW","file":"main.go","line":10,"message":"unused import"}]},"total_cost_usd":0.05}`

	review, raw, err := ParseCLIOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if review == nil {
		t.Fatal("expected non-nil review")
	}
	if review.Verdict != VerdictComment {
		t.Errorf("verdict = %q, want %q", review.Verdict, VerdictComment)
	}
	if review.Summary != "Minor issues found" {
		t.Errorf("summary = %q", review.Summary)
	}
	if len(review.Findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(review.Findings))
	}
	if review.Findings[0].Severity != "LOW" {
		t.Errorf("severity = %q", review.Findings[0].Severity)
	}
	if raw == "" {
		t.Error("raw should not be empty")
	}
}

func TestParseCLIOutput_FallbackResultField(t *testing.T) {
	// Fallback: structured data in result field (no --json-schema)
	input := `{"type":"result","subtype":"success","is_error":false,"result":"{\"verdict\":\"approve\",\"summary\":\"Looks good\",\"findings\":[]}","total_cost_usd":0.05}`

	review, _, err := ParseCLIOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if review == nil {
		t.Fatal("expected non-nil review")
	}
	if review.Verdict != VerdictApprove {
		t.Errorf("verdict = %q, want %q", review.Verdict, VerdictApprove)
	}
}

func TestParseCLIOutput_NotJSON(t *testing.T) {
	review, raw, err := ParseCLIOutput("just some text")
	if err == nil {
		t.Error("expected error for non-JSON input")
	}
	if review != nil {
		t.Error("expected nil review")
	}
	if raw != "just some text" {
		t.Errorf("raw = %q, want original text", raw)
	}
}

func TestParseCLIOutput_EnvelopeError(t *testing.T) {
	input := `{"type":"result","subtype":"error","is_error":true,"result":"something went wrong","total_cost_usd":0}`

	review, _, err := ParseCLIOutput(input)
	if err == nil {
		t.Error("expected error for error envelope")
	}
	if review != nil {
		t.Error("expected nil review on error")
	}
}

func TestStructuredReview_FindingsSummary(t *testing.T) {
	r := StructuredReview{
		Findings: []Finding{
			{Severity: "HIGH", File: "a.go", Message: "bad"},
			{Severity: "HIGH", File: "b.go", Message: "bad"},
			{Severity: "LOW", File: "c.go", Message: "minor"},
		},
	}
	got := r.FindingsSummary()
	if got != "2 HIGH, 1 LOW" {
		t.Errorf("FindingsSummary = %q", got)
	}
}

func TestStructuredReview_FindingsSummary_Empty(t *testing.T) {
	r := StructuredReview{}
	got := r.FindingsSummary()
	if got != "No issues found" {
		t.Errorf("FindingsSummary = %q", got)
	}
}

func TestStructuredReview_FormatMarkdown(t *testing.T) {
	r := StructuredReview{
		Verdict: VerdictRequestChanges,
		Summary: "Two issues need fixing",
		Findings: []Finding{
			{Severity: "HIGH", File: "auth.go", Line: 42, Message: "Missing nil check"},
			{Severity: "LOW", File: "util.go", Message: "Consider renaming"},
		},
	}
	md := r.FormatMarkdown()

	checks := []string{
		"Changes Requested",
		"Two issues need fixing",
		"HIGH",
		"`auth.go:42`",
		"Missing nil check",
		"LOW",
		"`util.go`",
	}
	for _, c := range checks {
		if !strings.Contains(md, c) {
			t.Errorf("markdown missing %q", c)
		}
	}
}
