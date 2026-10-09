package github

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Defaults for labels pr-sentinel creates.
const (
	labelColor       = "5319E7"
	labelDescription = "Managed by pr-sentinel"
)

// EnsureLabel creates the label in repo if it does not exist. An existing
// label is left as is: no --force, so a color or description a team chose is
// never overwritten.
func EnsureLabel(repo string, name string) error {
	cmd := exec.Command("gh", "label", "create", name,
		"-R", repo,
		"--color", labelColor,
		"--description", labelDescription,
	)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errMsg := strings.TrimSpace(stderr.String())
		if isLabelExistsError(errMsg) {
			return nil
		}
		if errMsg != "" {
			return fmt.Errorf("gh label create %s %q failed: %s: %w", repo, name, errMsg, err)
		}
		return fmt.Errorf("gh label create %s %q failed: %w", repo, name, err)
	}

	return nil
}

// isLabelExistsError reports whether gh refused because the label exists, e.g.
// `label with name "x" already exists; use --force to update its color and description`.
func isLabelExistsError(stderr string) bool {
	return strings.Contains(strings.ToLower(stderr), "already exists")
}
