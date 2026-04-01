package discovery

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// DiscoveredRepo represents a GitHub repository found on the local filesystem.
type DiscoveredRepo struct {
	Name string // "owner/repo"
	Path string // local filesystem path
}

var githubRemoteRe = regexp.MustCompile(
	`^(?:https://github\.com/|git@github\.com:)([^/]+)/([^/]+?)(?:\.git)?$`,
)

// ScanDirectory reads directory entries, checks for .git subdirectories,
// resolves the origin remote, and returns only GitHub-hosted repositories.
func ScanDirectory(dir string) ([]DiscoveredRepo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %s: %w", dir, err)
	}

	var repos []DiscoveredRepo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		repoPath := filepath.Join(dir, entry.Name())
		gitDir := filepath.Join(repoPath, ".git")

		info, err := os.Stat(gitDir)
		if err != nil || !info.IsDir() {
			continue
		}

		remote := getOriginRemote(repoPath)
		if remote == "" {
			continue
		}

		name := ParseGitHubRemote(remote)
		if name == "" {
			continue
		}

		repos = append(repos, DiscoveredRepo{
			Name: name,
			Path: repoPath,
		})
	}

	return repos, nil
}

// getOriginRemote runs `git remote get-url origin` in the given repo path
// and returns the trimmed URL, or an empty string on failure.
func getOriginRemote(repoPath string) string {
	cmd := exec.Command("git", "remote", "get-url", "origin")
	cmd.Dir = repoPath
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ParseGitHubRemote extracts the "owner/repo" from a GitHub remote URL.
// It supports both HTTPS and SSH formats. Returns an empty string for
// non-GitHub URLs or invalid input.
func ParseGitHubRemote(remote string) string {
	matches := githubRemoteRe.FindStringSubmatch(strings.TrimSpace(remote))
	if matches == nil {
		return ""
	}
	return matches[1] + "/" + matches[2]
}
