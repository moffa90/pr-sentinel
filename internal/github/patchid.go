package github

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// DiffPatchID fingerprints a PR's diff at an exact head commit: the
// merge-base diff from baseRef to headOID (what the PR shows), hashed with
// `git patch-id --stable`, which ignores line numbers and whitespace.
//
// Two heads with the same patch ID carry the same change. That is what a
// rebase onto a moved base, a restack, or GitHub's "Update branch" merge
// produces, and none of them needs a new paid review.
//
// The compare API is used rather than `gh pr diff`, which only shows the
// current head: the fingerprint must belong to the commit being reviewed.
// Returns "" for an empty diff.
func DiffPatchID(repo, baseRef, headOID string) (string, error) {
	if baseRef == "" || headOID == "" {
		return "", fmt.Errorf("patch id for %s needs a base ref and a head commit", repo)
	}

	diffCmd := exec.Command("gh", "api",
		"-H", "Accept: application/vnd.github.diff",
		fmt.Sprintf("repos/%s/compare/%s...%s", repo, baseRef, headOID),
	)
	var diffErr bytes.Buffer
	diffCmd.Stderr = &diffErr
	diff, err := diffCmd.Output()
	if err != nil {
		if msg := strings.TrimSpace(diffErr.String()); msg != "" {
			return "", fmt.Errorf("fetching diff %s %s...%s: %s: %w", repo, baseRef, shortOID(headOID), msg, err)
		}
		return "", fmt.Errorf("fetching diff %s %s...%s: %w", repo, baseRef, shortOID(headOID), err)
	}

	idCmd := exec.Command("git", "patch-id", "--stable")
	idCmd.Stdin = bytes.NewReader(diff)
	var idErr bytes.Buffer
	idCmd.Stderr = &idErr
	out, err := idCmd.Output()
	if err != nil {
		return "", fmt.Errorf("git patch-id: %s: %w", strings.TrimSpace(idErr.String()), err)
	}
	return parsePatchID(string(out)), nil
}

// parsePatchID takes the patch ID from `git patch-id` output
// ("<patch-id> <commit-id>"); empty output means an empty diff.
func parsePatchID(out string) string {
	fields := strings.Fields(out)
	if len(fields) == 0 {
		return ""
	}
	return fields[0]
}
