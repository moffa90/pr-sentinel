package github

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// EnableAutoMerge enables GitHub's auto-merge for a PR using `gh pr merge --auto`.
// GitHub will merge the PR once all required status checks and reviews pass.
// strategy must be "merge", "squash", or "rebase".
func EnableAutoMerge(repo string, number int64, strategy string, deleteBranch bool) error {
	validStrategies := map[string]bool{"merge": true, "squash": true, "rebase": true}
	if !validStrategies[strategy] {
		return fmt.Errorf("invalid merge strategy %q: must be merge, squash, or rebase", strategy)
	}

	args := []string{
		"pr", "merge",
		fmt.Sprintf("%d", number),
		"-R", repo,
		"--auto",
		"--" + strategy,
	}

	if deleteBranch {
		args = append(args, "--delete-branch")
	}

	cmd := exec.Command("gh", args...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if errMsg != "" {
			return fmt.Errorf("gh pr merge --auto %s#%d failed: %s: %w", repo, number, errMsg, err)
		}
		return fmt.Errorf("gh pr merge --auto %s#%d failed: %w", repo, number, err)
	}

	return nil
}
