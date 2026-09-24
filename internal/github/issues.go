package github

import (
	"bytes"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// CreateIssue opens an issue via `gh issue create` and returns its number and URL.
// The body is passed on stdin to avoid argv length limits. A non-empty URL
// with an error means the issue was created but its number could not be parsed.
func CreateIssue(repo string, title string, body string, labels []string) (int64, string, error) {
	args := []string{
		"issue", "create",
		"-R", repo,
		"--title", title,
		"--body-file", "-",
	}
	for _, l := range labels {
		args = append(args, "--label", l)
	}

	cmd := exec.Command("gh", args...)
	cmd.Stdin = strings.NewReader(body)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return 0, "", fmt.Errorf("gh issue create %s failed: %s: %w", repo, errMsg, err)
		}
		return 0, "", fmt.Errorf("gh issue create %s failed: %w", repo, err)
	}

	url := parseIssueURL(string(out))
	number, err := issueNumberFromURL(url)
	if err != nil {
		return 0, url, err
	}
	return number, url, nil
}

// CommentOnIssue adds a comment to an existing issue via `gh issue comment`.
// ref is an issue number or URL.
func CommentOnIssue(repo string, ref string, body string) error {
	cmd := exec.Command("gh", "issue", "comment", ref,
		"-R", repo,
		"--body-file", "-",
	)
	cmd.Stdin = strings.NewReader(body)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("gh issue comment %s %s failed: %s: %w", repo, ref, errMsg, err)
		}
		return fmt.Errorf("gh issue comment %s %s failed: %w", repo, ref, err)
	}

	return nil
}

// GetIssueState returns the state of an issue ("OPEN" or "CLOSED").
// ref is an issue number or URL.
func GetIssueState(repo string, ref string) (string, error) {
	cmd := exec.Command("gh", "issue", "view", ref,
		"-R", repo,
		"--json", "state",
		"--jq", ".state",
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	out, err := cmd.Output()
	if err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return "", fmt.Errorf("gh issue view %s %s failed: %s: %w", repo, ref, errMsg, err)
		}
		return "", fmt.Errorf("gh issue view %s %s failed: %w", repo, ref, err)
	}

	return strings.TrimSpace(string(out)), nil
}

// parseIssueURL returns the last non-empty line of gh output, which is the issue URL.
func parseIssueURL(out string) string {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// issueNumberFromURL extracts the issue number from a URL like
// https://github.com/owner/repo/issues/123.
func issueNumberFromURL(url string) (int64, error) {
	idx := strings.LastIndex(url, "/issues/")
	if idx < 0 {
		return 0, fmt.Errorf("unexpected issue URL %q", url)
	}
	n, err := strconv.ParseInt(url[idx+len("/issues/"):], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parsing issue number from %q: %w", url, err)
	}
	return n, nil
}
