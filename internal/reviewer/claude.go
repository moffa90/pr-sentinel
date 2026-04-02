package reviewer

import (
	"bytes"
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
	Files    int
	Adds     int
	Dels     int
}

type ReviewResult struct {
	Output   string            // raw result text from Claude
	Review   *StructuredReview // parsed structured review (nil if parsing failed)
	Duration time.Duration
	Error    error
}

// BuildReviewPrompt builds a prompt for Claude to review a pull request.
// The output must be valid JSON matching the StructuredReview schema.
func BuildReviewPrompt(p ReviewParams) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Review pull request %s#%d\n", p.Repo, p.PRNumber)
	fmt.Fprintf(&b, "Title: %s\n", p.PRTitle)
	fmt.Fprintf(&b, "Author: @%s\n", p.PRAuthor)
	fmt.Fprintf(&b, "Stats: %d files changed, %d additions, %d deletions\n\n", p.Files, p.Adds, p.Dels)

	b.WriteString("Instructions:\n")
	b.WriteString("- Focus on correctness, error handling, performance, security, and conventions\n")
	b.WriteString("- For each finding, specify the severity (HIGH, MEDIUM, or LOW), the file path, line number if applicable, and a clear description\n")
	b.WriteString("- Set verdict to \"approve\" if no issues found, \"comment\" for minor observations, or \"request-changes\" for issues that must be fixed\n")
	b.WriteString("- Provide a concise summary (1-3 sentences) of the overall review\n\n")

	b.WriteString("Use `gh pr diff " + fmt.Sprintf("%d", p.PRNumber) + " -R " + p.Repo + "` to fetch the diff and review the changes.\n")
	b.WriteString("You are running inside the repo directory with full access to the codebase.\n")

	return b.String()
}

// FollowUpParams holds parameters for building a follow-up review prompt.
type FollowUpParams struct {
	Repo           string
	PRNumber       int64
	PRTitle        string
	PRAuthor       string
	Files          int
	Adds           int
	Dels           int
	PreviousReview string
	NewCommitCount int
}

// BuildFollowUpPrompt builds a prompt for Claude to do a follow-up review.
// It includes the previous review output and asks Claude to check if the
// new commits address the previously raised issues.
func BuildFollowUpPrompt(p FollowUpParams) string {
	var b strings.Builder

	fmt.Fprintf(&b, "Follow-up review for pull request %s#%d\n", p.Repo, p.PRNumber)
	fmt.Fprintf(&b, "Title: %s\n", p.PRTitle)
	fmt.Fprintf(&b, "Author: @%s\n", p.PRAuthor)
	fmt.Fprintf(&b, "Stats: %d files changed, %d additions, %d deletions\n", p.Files, p.Adds, p.Dels)
	fmt.Fprintf(&b, "New activity: %d new commit(s) since last review\n\n", p.NewCommitCount)

	b.WriteString("## Previous Review\n\n")
	b.WriteString(p.PreviousReview)
	b.WriteString("\n\n")

	b.WriteString("## Instructions\n\n")
	b.WriteString("This is a follow-up review. A previous review was already posted (shown above).\n")
	b.WriteString("The PR author has pushed new commits since that review.\n\n")
	b.WriteString("Your task:\n")
	b.WriteString("1. Determine whether the new commits address the issues raised in the previous review\n")
	b.WriteString("2. For each previous finding, state whether it was RESOLVED or UNRESOLVED\n")
	b.WriteString("3. Note any NEW issues introduced by the new commits\n")
	b.WriteString("4. If all previous issues are resolved and no new issues found, approve the changes\n\n")

	b.WriteString("Use `gh pr diff " + fmt.Sprintf("%d", p.PRNumber) + " -R " + p.Repo + "` to fetch the diff and review the changes.\n")
	b.WriteString("You are running inside the repo directory with full access to the codebase.\n")

	return b.String()
}

// BuildClaudeArgs returns the argument list for the claude CLI invocation.
func BuildClaudeArgs(prompt string, globalInstructions string, repoInstructions string) []string {
	args := []string{
		"-p", prompt,
		"--output-format", "json",
		"--json-schema", ReviewJSONSchema,
	}

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
	slog.Debug("starting claude", "dir", repoPath, "timeout", timeout, "prompt_bytes", len(prompt), "arg_count", len(args))

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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

	err := cmd.Run()
	heartbeatCancel() // stop heartbeat
	duration := time.Since(start)

	outStr := stdout.String()
	errStr := strings.TrimSpace(stderr.String())

	slog.Debug("claude finished", "stdout_bytes", len(outStr), "stderr_bytes", len(errStr), "duration", duration.Round(time.Second))

	if err != nil {
		// Log stderr for visibility on any failure
		if errStr != "" {
			slog.Error("claude stderr", "output", truncate(errStr, 500))
		}

		if ctx.Err() == context.Canceled {
			return ReviewResult{
				Output:   outStr,
				Duration: duration,
				Error:    fmt.Errorf("review cancelled"),
			}
		}
		if ctx.Err() == context.DeadlineExceeded {
			detail := "no output captured"
			if errStr != "" {
				detail = truncate(errStr, 200)
			} else if outStr != "" {
				detail = truncate(outStr, 200)
			}
			return ReviewResult{
				Output:   outStr,
				Duration: duration,
				Error:    fmt.Errorf("review timed out after %s (%s)", timeout, detail),
			}
		}

		detail := err.Error()
		if errStr != "" {
			detail = fmt.Sprintf("%s: %s", err.Error(), truncate(errStr, 200))
		}
		return ReviewResult{
			Output:   outStr,
			Duration: duration,
			Error:    fmt.Errorf("claude exited with error: %s", detail),
		}
	}

	// Parse the structured output from claude CLI JSON envelope
	review, rawResult, parseErr := ParseCLIOutput(outStr)
	if parseErr != nil {
		slog.Warn("failed to parse structured review, using raw output", "error", parseErr)
	}

	return ReviewResult{
		Output:   rawResult,
		Review:   review,
		Duration: duration,
	}
}

// truncate returns the first n bytes of s, appending "..." if truncated.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
