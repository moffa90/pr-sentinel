package prerequisites

import (
	"os/exec"
	"regexp"
	"strings"
)

// CheckResult holds the outcome of a single prerequisite check.
type CheckResult struct {
	Name     string
	Found    bool
	Version  string
	Detail   string
	HelpText string
}

// CheckAll runs all prerequisite checks and returns the results.
func CheckAll() []CheckResult {
	return []CheckResult{
		checkClaude(),
		checkGh(),
	}
}

// AllPassed returns true if every check result has Found == true.
func AllPassed(results []CheckResult) bool {
	for _, r := range results {
		if !r.Found {
			return false
		}
	}
	return true
}

func checkClaude() CheckResult {
	result := CheckResult{
		Name:     "claude",
		HelpText: "Install Claude Code: https://docs.anthropic.com/en/docs/claude-code/overview\nThen run `claude` to authenticate.",
	}

	path, err := exec.LookPath("claude")
	if err != nil {
		result.Detail = "claude CLI not found in PATH"
		return result
	}

	result.Found = true
	result.Detail = path

	out, err := exec.Command("claude", "--version").CombinedOutput()
	if err == nil {
		result.Version = strings.TrimSpace(string(out))
	}

	return result
}

func checkGh() CheckResult {
	result := CheckResult{
		Name:     "gh",
		HelpText: "Install with: brew install gh\nThen run: gh auth login",
	}

	path, err := exec.LookPath("gh")
	if err != nil {
		result.Detail = "gh CLI not found in PATH"
		return result
	}

	result.Detail = path

	out, err := exec.Command("gh", "--version").CombinedOutput()
	if err == nil {
		result.Version = strings.TrimSpace(string(out))
	}

	// Check authentication
	authOut, err := exec.Command("gh", "auth", "status").CombinedOutput()
	if err != nil {
		result.Found = false
		result.Detail = "gh CLI found but not authenticated"
		result.HelpText = "gh CLI found but not authenticated. Run: gh auth login"
		return result
	}

	result.Found = true
	if user, ok := parseGhUser(string(authOut)); ok {
		result.Detail = "authenticated as " + user
	}

	return result
}

// parseGhUser extracts the GitHub username from gh auth status output.
func parseGhUser(output string) (string, bool) {
	re := regexp.MustCompile(`Logged in to github\.com account (\S+)`)
	matches := re.FindStringSubmatch(output)
	if len(matches) < 2 {
		return "", false
	}
	return matches[1], true
}

// DetectGitHubUser runs gh auth status and returns the authenticated username.
func DetectGitHubUser() string {
	out, err := exec.Command("gh", "auth", "status").CombinedOutput()
	if err != nil {
		return ""
	}
	user, ok := parseGhUser(string(out))
	if !ok {
		return ""
	}
	return user
}
