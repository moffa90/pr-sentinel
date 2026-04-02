package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Mode constants
const (
	ModeDryRun = "dry-run"
	ModeLive   = "live"
)

// Default values
const (
	DefaultPollInterval        = 10 * time.Minute
	DefaultMaxReviewsCycle     = 5
	DefaultMaxReviewsDay       = 20
	DefaultReviewTimeout       = 5 * time.Minute
	DefaultDisclosureText      = "> AI-assisted review by [pr-sentinel](https://github.com/moffa90/pr-sentinel)"
	DefaultMaxParallelReviews  = 3
)

// Config is the top-level configuration.
type Config struct {
	PollInterval       time.Duration       `yaml:"poll_interval"`
	MaxReviewsPerCycle int                 `yaml:"max_reviews_per_cycle"`
	MaxReviewsPerDay   int                 `yaml:"max_reviews_per_day"`
	MaxParallelReviews int                 `yaml:"max_parallel_reviews"`
	ReposDir           string              `yaml:"repos_dir"`
	ReviewTimeout      time.Duration       `yaml:"review_timeout"`
	GitHubUser         string              `yaml:"github_user"`
	Review             ReviewConfig        `yaml:"review"`
	Notifications      NotificationConfig  `yaml:"notifications"`
	Repos              []RepoConfig        `yaml:"repos"`
}

// ReviewConfig holds review behaviour settings.
type ReviewConfig struct {
	Instructions   string `yaml:"instructions"`
	AIDisclosure   bool   `yaml:"ai_disclosure"`
	DisclosureText string `yaml:"disclosure_text"`
}

// NotificationConfig holds notification channel settings.
type NotificationConfig struct {
	MacOS   bool          `yaml:"macos"`
	Log     bool          `yaml:"log"`
	Slack   SlackConfig   `yaml:"slack"`
	Teams   TeamsConfig   `yaml:"teams"`
	Webhook WebhookConfig `yaml:"webhook"`
}

// SlackConfig holds Slack notification settings.
type SlackConfig struct {
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url"`
}

// TeamsConfig holds Microsoft Teams notification settings.
type TeamsConfig struct {
	Enabled    bool   `yaml:"enabled"`
	WebhookURL string `yaml:"webhook_url"`
}

// WebhookConfig holds generic webhook notification settings.
type WebhookConfig struct {
	Enabled bool   `yaml:"enabled"`
	URL     string `yaml:"url"`
}

// RepoConfig holds per-repository settings.
type RepoConfig struct {
	Name               string `yaml:"name"`
	Path               string `yaml:"path"`
	Mode               string `yaml:"mode"`
	ReviewInstructions string `yaml:"review_instructions"`
}

// DefaultConfig returns a Config populated with default values.
func DefaultConfig() Config {
	return Config{
		PollInterval:       DefaultPollInterval,
		MaxReviewsPerCycle: DefaultMaxReviewsCycle,
		MaxReviewsPerDay:   DefaultMaxReviewsDay,
		MaxParallelReviews: DefaultMaxParallelReviews,
		ReposDir:           "~/Git",
		ReviewTimeout:      DefaultReviewTimeout,
		GitHubUser:         "",
		Review: ReviewConfig{
			Instructions:   "",
			AIDisclosure:   true,
			DisclosureText: DefaultDisclosureText,
		},
		Notifications: NotificationConfig{
			MacOS: true,
			Log:   true,
			Slack: SlackConfig{
				Enabled:    false,
				WebhookURL: "",
			},
			Teams: TeamsConfig{
				Enabled:    false,
				WebhookURL: "",
			},
			Webhook: WebhookConfig{
				Enabled: false,
				URL:     "",
			},
		},
		Repos: []RepoConfig{},
	}
}

// ConfigDir returns the configuration directory path (~/.config/pr-sentinel).
func ConfigDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".config", "pr-sentinel")
	}
	return filepath.Join(home, ".config", "pr-sentinel")
}

// DefaultConfigPath returns the default configuration file path.
func DefaultConfigPath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

// ExpandPath replaces a leading ~/ with the user's home directory.
func ExpandPath(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	return filepath.Join(home, path[2:])
}

// Load reads a YAML config file from the given path.
// If the file does not exist it returns an error.
func Load(path string) (Config, error) {
	expanded := ExpandPath(path)

	data, err := os.ReadFile(expanded)
	if err != nil {
		return Config{}, fmt.Errorf("reading config file: %w", err)
	}

	cfg := DefaultConfig()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config file: %w", err)
	}

	return cfg, nil
}

// Save writes the config as YAML to the given path, creating directories as needed.
func Save(cfg Config, path string) error {
	expanded := ExpandPath(path)

	dir := filepath.Dir(expanded)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return fmt.Errorf("marshalling config: %w", err)
	}

	if err := os.WriteFile(expanded, data, 0o644); err != nil {
		return fmt.Errorf("writing config file: %w", err)
	}

	return nil
}

// FindRepo returns the RepoConfig for the given name and true if found,
// or a zero value and false otherwise.
func (c *Config) FindRepo(name string) (RepoConfig, bool) {
	for _, r := range c.Repos {
		if r.Name == name {
			return r, true
		}
	}
	return RepoConfig{}, false
}

// SetRepoMode sets the mode for the named repo. Returns an error if the
// repo is not found or the mode is invalid.
func (c *Config) SetRepoMode(name, mode string) error {
	if mode != ModeDryRun && mode != ModeLive {
		return fmt.Errorf("invalid mode %q: must be %q or %q", mode, ModeDryRun, ModeLive)
	}

	for i := range c.Repos {
		if c.Repos[i].Name == name {
			c.Repos[i].Mode = mode
			return nil
		}
	}

	return fmt.Errorf("repo %q not found", name)
}
