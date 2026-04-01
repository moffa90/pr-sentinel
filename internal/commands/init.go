package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/discovery"
	"github.com/moffa90/pr-sentinel/internal/prerequisites"
	"github.com/moffa90/pr-sentinel/internal/ui"
	"github.com/spf13/cobra"
)

// NewInitCmd creates the init command for first-time setup.
func NewInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init [directory]",
		Short: "Interactive setup — scan repos, pick which to watch",
		Long:  "Scans a directory for GitHub repositories, lets you pick which ones to watch, and writes the initial configuration file.",
		Args:  cobra.MaximumNArgs(1),
		RunE:  runInit,
	}
}

func runInit(cmd *cobra.Command, args []string) error {
	fmt.Println(ui.Banner("Setup"))
	fmt.Println()

	// Step 1: Check prerequisites
	fmt.Printf("%s Checking prerequisites...\n", ui.IconDot)
	results := prerequisites.CheckAll()
	for _, r := range results {
		if r.Found {
			fmt.Printf("  %s %s %s\n", ui.IconCheck, r.Name, ui.MutedStyle.Render(r.Version))
		} else {
			fmt.Printf("  %s %s — %s\n", ui.IconCross, r.Name, r.Detail)
			fmt.Printf("    %s\n", ui.MutedStyle.Render(r.HelpText))
		}
	}
	fmt.Println()

	if !prerequisites.AllPassed(results) {
		return fmt.Errorf("prerequisites not met — install missing tools and try again")
	}

	// Step 2: Determine scan directory
	scanDir := "~/Git"
	if len(args) > 0 {
		scanDir = args[0]
	}
	expandedDir := config.ExpandPath(scanDir)

	fmt.Printf("%s Scanning %s for GitHub repos...\n", ui.IconDot, ui.BrandStyle.Render(expandedDir))

	repos, err := discovery.ScanDirectory(expandedDir)
	if err != nil {
		return fmt.Errorf("scanning directory: %w", err)
	}

	if len(repos) == 0 {
		return fmt.Errorf("no GitHub repositories found in %s", expandedDir)
	}

	fmt.Printf("  %s Found %d repositories\n\n", ui.IconCheck, len(repos))

	// Step 3: Interactive selector
	items := make([]ui.RepoItem, len(repos))
	for i, r := range repos {
		items[i] = ui.RepoItem{
			Name:     r.Name,
			Path:     r.Path,
			Selected: true,
		}
	}

	selected, err := ui.RunSelector(items)
	if err != nil {
		return fmt.Errorf("selector: %w", err)
	}
	if len(selected) == 0 {
		fmt.Println(ui.WarningStyle.Render("No repos selected — exiting."))
		return nil
	}

	fmt.Printf("\n%s Selected %d repositories\n", ui.IconCheck, len(selected))

	// Step 4: Detect GitHub user
	fmt.Printf("%s Detecting GitHub user...\n", ui.IconDot)
	ghUser := prerequisites.DetectGitHubUser()
	if ghUser != "" {
		fmt.Printf("  %s Authenticated as %s\n", ui.IconCheck, ui.BrandStyle.Render(ghUser))
	} else {
		fmt.Printf("  %s Could not detect GitHub user — set manually in config\n", ui.IconCross)
	}
	fmt.Println()

	// Step 5: Build config
	cfg := config.DefaultConfig()
	cfg.ReposDir = scanDir
	cfg.GitHubUser = ghUser

	for _, item := range selected {
		cfg.Repos = append(cfg.Repos, config.RepoConfig{
			Name: item.Name,
			Path: item.Path,
			Mode: config.ModeDryRun,
		})
	}

	// Step 6: Save config
	cfgPath := config.DefaultConfigPath()
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	if err := config.Save(cfg, cfgPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("%s Config saved to %s\n", ui.IconCheck, ui.MutedStyle.Render(cfgPath))
	fmt.Printf("%s All repos start in %s mode — use %s to enable posting\n",
		ui.IconDot,
		ui.ModeBadge(config.ModeDryRun),
		ui.BrandStyle.Render("pr-sentinel promote <repo>"),
	)

	return nil
}
