package commands

import (
	"fmt"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/ui"
	"github.com/spf13/cobra"
)

// NewReposCmd creates the repos command that lists watched repositories.
func NewReposCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "repos",
		Short: "List watched repositories and their modes",
		RunE:  runRepos,
	}
}

func runRepos(cmd *cobra.Command, args []string) error {
	cfg, err := config.Load(config.DefaultConfigPath())
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	if len(cfg.Repos) == 0 {
		fmt.Println(ui.MutedStyle.Render("No repositories configured. Run `pr-sentinel init` to get started."))
		return nil
	}

	fmt.Printf("\n%s Watched repositories (%d):\n\n", ui.IconDot, len(cfg.Repos))

	for _, repo := range cfg.Repos {
		badge := ui.ModeBadge(repo.Mode)
		path := ui.MutedStyle.Render(repo.Path)
		fmt.Printf("  %s  %s  %s\n", badge, repo.Name, path)
	}

	fmt.Println()
	return nil
}
