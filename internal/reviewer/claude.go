package reviewer

import (
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

const DefaultTimeout = 5 * time.Minute

type ReviewParams struct {
	Repo     string
	PRNumber int64
	PRTitle  string
	PRAuthor string
	Diff     string
	Files    int
	Adds     int
	Dels     int
}

type ReviewResult struct {
	Output   string
	Duration time.Duration
	Error    error
}

// BuildReviewPrompt builds a prompt for Claude to review a pull request.
func BuildReviewPrompt(p ReviewParams) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Review pull request %s#%d\n", p.Repo, p.PRNumber)
	fmt.Fprintf(&b, "Title: %s\n", p.PRTitle)
	fmt.Fprintf(&b, "Author: @%s\n", p.PRAuthor)
	fmt.Fprintf(&b, "Stats: %d files changed, %d additions, %d deletions\n\n", p.Files, p.Adds, p.Dels)

	b.WriteString("Instructions:\n")
	b.WriteString("- Focus on correctness, error handling, performance, security, and conventions\n")
	b.WriteString("- Indicate severity for each finding: HIGH, MEDIUM, or LOW\n")
	b.WriteString("- Reference specific locations as file:line when applicable\n\n")

	b.WriteString("Diff:\n```\n")
	b.WriteString(p.Diff)
	b.WriteString("\n```\n")

	return b.String()
}

// BuildClaudeArgs returns the argument list for the claude CLI invocation.
func BuildClaudeArgs(prompt string, globalInstructions string, repoInstructions string) []string {
	args := []string{"-p", prompt}

	if globalInstructions != "" {
		args = append(args, "--append-system-prompt", globalInstructions)
	}
	if repoInstructions != "" {
		args = append(args, "--append-system-prompt", repoInstructions)
	}

	return args
}

// RunReview executes the claude CLI to review a pull request diff.
func RunReview(ctx context.Context, repoPath string, prompt string, globalInstructions string, repoInstructions string, timeout time.Duration) ReviewResult {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	args := BuildClaudeArgs(prompt, globalInstructions, repoInstructions)
	cmd := exec.CommandContext(ctx, "claude", args...)
	cmd.Dir = repoPath

	// Heartbeat goroutine — logs progress every 30s
	heartbeatCtx, heartbeatCancel := context.WithCancel(context.Background())
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				slog.Info("review in progress", "dir", repoPath, "elapsed", time.Since(start).Round(time.Second))
			}
		}
	}()

	output, err := cmd.CombinedOutput()
	heartbeatCancel() // stop heartbeat
	duration := time.Since(start)

	if err != nil {
		if ctx.Err() == context.Canceled {
			return ReviewResult{
				Output:   string(output),
				Duration: duration,
				Error:    fmt.Errorf("review cancelled"),
			}
		}
		if ctx.Err() == context.DeadlineExceeded {
			return ReviewResult{
				Output:   string(output),
				Duration: duration,
				Error:    fmt.Errorf("review timed out after %s", timeout),
			}
		}
		return ReviewResult{
			Output:   string(output),
			Duration: duration,
			Error:    fmt.Errorf("claude exited with error: %w", err),
		}
	}

	return ReviewResult{
		Output:   string(output),
		Duration: duration,
	}
}
