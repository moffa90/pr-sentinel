package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.PollInterval != 10*time.Minute {
		t.Errorf("PollInterval = %v, want %v", cfg.PollInterval, 10*time.Minute)
	}
	if cfg.MaxReviewsPerCycle != 5 {
		t.Errorf("MaxReviewsPerCycle = %d, want 5", cfg.MaxReviewsPerCycle)
	}
	if cfg.MaxReviewsPerDay != 20 {
		t.Errorf("MaxReviewsPerDay = %d, want 20", cfg.MaxReviewsPerDay)
	}
	if cfg.ReviewTimeout != 10*time.Minute {
		t.Errorf("ReviewTimeout = %v, want %v", cfg.ReviewTimeout, 10*time.Minute)
	}
	if cfg.ReposDir != "~/Git" {
		t.Errorf("ReposDir = %q, want %q", cfg.ReposDir, "~/Git")
	}
	if cfg.GitHubUser != "" {
		t.Errorf("GitHubUser = %q, want empty", cfg.GitHubUser)
	}
	if !cfg.Review.AIDisclosure {
		t.Error("Review.AIDisclosure should be true by default")
	}
	if cfg.Review.DisclosureText != DefaultDisclosureText {
		t.Errorf("DisclosureText = %q, want %q", cfg.Review.DisclosureText, DefaultDisclosureText)
	}
	if !cfg.Notifications.MacOS {
		t.Error("Notifications.MacOS should be true by default")
	}
	if !cfg.Notifications.Log {
		t.Error("Notifications.Log should be true by default")
	}
	if cfg.Notifications.Slack.Enabled {
		t.Error("Slack should be disabled by default")
	}
	if cfg.Notifications.Teams.Enabled {
		t.Error("Teams should be disabled by default")
	}
	if cfg.Notifications.Webhook.Enabled {
		t.Error("Webhook should be disabled by default")
	}
	if cfg.Repos == nil || len(cfg.Repos) != 0 {
		t.Errorf("Repos should be an empty non-nil slice, got %v", cfg.Repos)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	cfg.GitHubUser = "testuser"
	cfg.Repos = []RepoConfig{
		{
			Name: "owner/repo",
			Path: "/tmp/repo",
			Mode: ModeDryRun,
		},
	}

	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if loaded.GitHubUser != "testuser" {
		t.Errorf("GitHubUser = %q, want %q", loaded.GitHubUser, "testuser")
	}
	if loaded.PollInterval != 10*time.Minute {
		t.Errorf("PollInterval = %v, want %v", loaded.PollInterval, 10*time.Minute)
	}
	if loaded.MaxReviewsPerCycle != 5 {
		t.Errorf("MaxReviewsPerCycle = %d, want 5", loaded.MaxReviewsPerCycle)
	}
	if len(loaded.Repos) != 1 {
		t.Fatalf("Repos length = %d, want 1", len(loaded.Repos))
	}
	if loaded.Repos[0].Name != "owner/repo" {
		t.Errorf("Repos[0].Name = %q, want %q", loaded.Repos[0].Name, "owner/repo")
	}
	if loaded.Repos[0].Mode != ModeDryRun {
		t.Errorf("Repos[0].Mode = %q, want %q", loaded.Repos[0].Mode, ModeDryRun)
	}
}

func TestLoadNonexistent(t *testing.T) {
	_, err := Load("/tmp/pr-sentinel-nonexistent-config-12345.yaml")
	if err == nil {
		t.Fatal("Load should return error for nonexistent file")
	}
}

func TestFindRepo(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Repos = []RepoConfig{
		{Name: "owner/repo-a", Path: "/a", Mode: ModeDryRun},
		{Name: "owner/repo-b", Path: "/b", Mode: ModeLive},
	}

	repo, ok := cfg.FindRepo("owner/repo-a")
	if !ok {
		t.Fatal("FindRepo should find owner/repo-a")
	}
	if repo.Path != "/a" {
		t.Errorf("Path = %q, want %q", repo.Path, "/a")
	}

	repo, ok = cfg.FindRepo("owner/repo-b")
	if !ok {
		t.Fatal("FindRepo should find owner/repo-b")
	}
	if repo.Mode != ModeLive {
		t.Errorf("Mode = %q, want %q", repo.Mode, ModeLive)
	}

	_, ok = cfg.FindRepo("nonexistent")
	if ok {
		t.Error("FindRepo should return false for nonexistent repo")
	}
}

func TestSetRepoMode(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Repos = []RepoConfig{
		{Name: "owner/repo", Path: "/r", Mode: ModeDryRun},
	}

	// Valid mode change
	if err := cfg.SetRepoMode("owner/repo", ModeLive); err != nil {
		t.Fatalf("SetRepoMode: %v", err)
	}
	if cfg.Repos[0].Mode != ModeLive {
		t.Errorf("Mode = %q, want %q", cfg.Repos[0].Mode, ModeLive)
	}

	// Switch back
	if err := cfg.SetRepoMode("owner/repo", ModeDryRun); err != nil {
		t.Fatalf("SetRepoMode: %v", err)
	}
	if cfg.Repos[0].Mode != ModeDryRun {
		t.Errorf("Mode = %q, want %q", cfg.Repos[0].Mode, ModeDryRun)
	}

	// Invalid mode
	if err := cfg.SetRepoMode("owner/repo", "invalid"); err == nil {
		t.Error("SetRepoMode should return error for invalid mode")
	}

	// Nonexistent repo
	if err := cfg.SetRepoMode("nonexistent", ModeLive); err == nil {
		t.Error("SetRepoMode should return error for nonexistent repo")
	}
}

func TestExpandPath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("cannot determine home dir: %v", err)
	}

	tests := []struct {
		input string
		want  string
	}{
		{"~/Git", filepath.Join(home, "Git")},
		{"~/", home},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
		{"~notahome", "~notahome"},
	}

	for _, tc := range tests {
		got := ExpandPath(tc.input)
		if got != tc.want {
			t.Errorf("ExpandPath(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestConfigDir(t *testing.T) {
	dir := ConfigDir()
	if !filepath.IsAbs(dir) {
		t.Errorf("ConfigDir should return absolute path, got %q", dir)
	}
	if filepath.Base(dir) != "pr-sentinel" {
		t.Errorf("ConfigDir base should be 'pr-sentinel', got %q", filepath.Base(dir))
	}
}

func TestDefaultConfigPath(t *testing.T) {
	path := DefaultConfigPath()
	if filepath.Base(path) != "config.yaml" {
		t.Errorf("DefaultConfigPath should end with config.yaml, got %q", path)
	}
}

func TestSaveCreatesDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "deep", "config.yaml")

	cfg := DefaultConfig()
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save should create nested directories: %v", err)
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("config file should exist after Save")
	}
}

func TestDefaultConfig_MaxParallelReviews(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxParallelReviews != 3 {
		t.Errorf("MaxParallelReviews = %d, want 3", cfg.MaxParallelReviews)
	}
}

func TestSaveAndLoad_MaxParallelReviews(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	cfg := DefaultConfig()
	cfg.MaxParallelReviews = 5
	if err := Save(cfg, path); err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.MaxParallelReviews != 5 {
		t.Errorf("MaxParallelReviews = %d, want 5", loaded.MaxParallelReviews)
	}
}
