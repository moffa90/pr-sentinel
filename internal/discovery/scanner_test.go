package discovery

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// createTestRepo initialises a bare-minimum git repo with the given origin remote.
func createTestRepo(t *testing.T, dir, name, remote string) {
	t.Helper()

	repoPath := filepath.Join(dir, name)
	if err := os.MkdirAll(repoPath, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", repoPath, err)
	}

	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v in %s: %v\n%s", args, repoPath, err, out)
		}
	}

	run("init")
	run("remote", "add", "origin", remote)
}

func TestScanDirectory(t *testing.T) {
	dir := t.TempDir()

	// Two GitHub repos (HTTPS and SSH).
	createTestRepo(t, dir, "repo-https", "https://github.com/owner/repo-https.git")
	createTestRepo(t, dir, "repo-ssh", "git@github.com:owner/repo-ssh.git")

	// One plain directory (not a git repo).
	if err := os.MkdirAll(filepath.Join(dir, "not-a-repo"), 0o755); err != nil {
		t.Fatalf("mkdir not-a-repo: %v", err)
	}

	repos, err := ScanDirectory(dir)
	if err != nil {
		t.Fatalf("ScanDirectory: %v", err)
	}

	if got := len(repos); got != 2 {
		t.Fatalf("expected 2 repos, got %d: %+v", got, repos)
	}

	found := map[string]bool{}
	for _, r := range repos {
		found[r.Name] = true
	}

	for _, want := range []string{"owner/repo-https", "owner/repo-ssh"} {
		if !found[want] {
			t.Errorf("expected to find %s in results", want)
		}
	}
}

func TestScanDirectory_Empty(t *testing.T) {
	dir := t.TempDir()

	repos, err := ScanDirectory(dir)
	if err != nil {
		t.Fatalf("ScanDirectory: %v", err)
	}
	if len(repos) != 0 {
		t.Fatalf("expected 0 repos, got %d", len(repos))
	}
}

func TestParseGitHubRemote(t *testing.T) {
	tests := []struct {
		name   string
		remote string
		want   string
	}{
		{"https with .git", "https://github.com/owner/repo.git", "owner/repo"},
		{"https without .git", "https://github.com/owner/repo", "owner/repo"},
		{"ssh with .git", "git@github.com:owner/repo.git", "owner/repo"},
		{"ssh without .git", "git@github.com:owner/repo", "owner/repo"},
		{"non-github https", "https://gitlab.com/owner/repo.git", ""},
		{"non-github ssh", "git@gitlab.com:owner/repo.git", ""},
		{"empty string", "", ""},
		{"random string", "not-a-url", ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ParseGitHubRemote(tc.remote)
			if got != tc.want {
				t.Errorf("ParseGitHubRemote(%q) = %q, want %q", tc.remote, got, tc.want)
			}
		})
	}
}
