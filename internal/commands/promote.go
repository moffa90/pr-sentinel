package commands

import (
	"fmt"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/daemon"
	"github.com/moffa90/pr-sentinel/internal/ui"
	"github.com/spf13/cobra"
)

// NewPromoteCmd creates the promote command to switch a repo to live mode.
func NewPromoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "promote <repo>",
		Short: "Switch a repository to live mode (reviews are posted to GitHub)",
		Args:  cobra.ExactArgs(1),
		RunE:  runPromote,
	}
}

// NewDemoteCmd creates the demote command to switch a repo to dry-run mode.
func NewDemoteCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "demote <repo>",
		Short: "Switch a repository to dry-run mode (reviews saved locally)",
		Args:  cobra.ExactArgs(1),
		RunE:  runDemote,
	}
}

func runPromote(cmd *cobra.Command, args []string) error {
	return setMode(args[0], config.ModeLive)
}

func runDemote(cmd *cobra.Command, args []string) error {
	return setMode(args[0], config.ModeDryRun)
}

func setMode(repoName, mode string) error {
	cfgPath := config.DefaultConfigPath()
	cfg, err := config.Load(cfgPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if err := cfg.SetRepoMode(repoName, mode); err != nil {
		return fmt.Errorf("setting mode: %w", err)
	}

	if err := config.Save(cfg, cfgPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	badge := ui.ModeBadge(mode)
	fmt.Printf("%s %s is now %s\n", ui.IconCheck, ui.BrandStyle.Render(repoName), badge)
	if daemon.IsRunning() {
		fmt.Printf("  %s Restart the daemon for changes to take effect\n", ui.MutedStyle.Render("hint:"))
	}
	return nil
}
