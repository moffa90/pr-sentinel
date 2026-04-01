package reviewer

import (
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
		Diff:     "+ added line\n- removed line",
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
		{"diff content added", "+ added line"},
		{"diff content removed", "- removed line"},
		{"files stat", "3 files changed"},
		{"additions stat", "10 additions"},
		{"deletions stat", "5 deletions"},
		{"severity HIGH", "HIGH"},
		{"severity MEDIUM", "MEDIUM"},
		{"severity LOW", "LOW"},
		{"file:line instruction", "file:line"},
	}

	for _, c := range checks {
		t.Run(c.name, func(t *testing.T) {
			if !strings.Contains(result, c.contains) {
				t.Errorf("prompt missing %q: %s", c.contains, result)
			}
		})
	}
}

func TestBuildClaudeArgs_BothInstructions(t *testing.T) {
	args := BuildClaudeArgs("review this", "global rules", "repo rules")

	if args[0] != "-p" || args[1] != "review this" {
		t.Errorf("expected -p flag with prompt, got %v", args[:2])
	}

	count := 0
	for _, a := range args {
		if a == "--append-system-prompt" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 --append-system-prompt flags, got %d", count)
	}

	// Verify ordering: global then repo
	if args[2] != "--append-system-prompt" || args[3] != "global rules" {
		t.Errorf("expected global instructions at args[2:4], got %v", args[2:4])
	}
	if args[4] != "--append-system-prompt" || args[5] != "repo rules" {
		t.Errorf("expected repo instructions at args[4:6], got %v", args[4:6])
	}
}

func TestBuildClaudeArgs_EmptyInstructions(t *testing.T) {
	args := BuildClaudeArgs("review this", "", "")

	if len(args) != 2 {
		t.Errorf("expected 2 args, got %d: %v", len(args), args)
	}

	for _, a := range args {
		if a == "--append-system-prompt" {
			t.Error("unexpected --append-system-prompt flag with empty instructions")
		}
	}
}
