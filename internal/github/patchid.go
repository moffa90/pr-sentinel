package github

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// DiffPatchID fingerprints a PR's diff at an exact head commit: the
// merge-base diff from baseRef to headOID (what the PR shows), hashed by
// PatchIDOfDiff.
//
// Two heads with the same patch ID carry the same change. That is what a
// rebase onto a moved base, a restack, or GitHub's "Update branch" merge
// usually produces, and none of them needs a new paid review.
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
		fmt.Sprintf("repos/%s/compare/%s...%s", repo, escapeRef(baseRef), headOID),
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
	return PatchIDOfDiff(diff)
}

// PatchIDOfDiff hashes a diff with `git patch-id --verbatim` (git >= 2.39).
//
// --verbatim, not --stable: --stable also strips whitespace, so an
// indentation-only change, which changes meaning in Python, YAML or a
// Makefile, would match the reviewed fingerprint and never be reviewed.
// Line numbers are still ignored, so moving the change in the file keeps its
// fingerprint; context lines are hashed, so a base edit next to a hunk changes
// it. On an older git this errors and the daemon reviews anyway (fails open).
// Returns "" for an empty diff.
func PatchIDOfDiff(diff []byte) (string, error) {
	cmd := exec.Command("git", "patch-id", "--verbatim")
	cmd.Stdin = bytes.NewReader(diff)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git patch-id --verbatim: %s: %w", strings.TrimSpace(stderr.String()), err)
	}
	return parsePatchID(string(out)), nil
}

// escapeRef escapes the characters git allows in a branch name that would
// break a URL path. "/" stays: GitHub resolves slashed refs in compare paths.
var refEscaper = strings.NewReplacer("%", "%25", "#", "%23", "?", "%3F")

func escapeRef(ref string) string {
	return refEscaper.Replace(ref)
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
