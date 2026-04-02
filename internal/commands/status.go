package commands

import (
	"fmt"
	"time"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/daemon"
	"github.com/moffa90/pr-sentinel/internal/state"
	"github.com/moffa90/pr-sentinel/internal/ui"
	"github.com/spf13/cobra"
)

// NewStatusCmd creates the status command showing daemon and repo overview.
func NewStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show daemon status, daily review count, and repo overview",
		RunE:  runStatus,
	}
}

func runStatus(cmd *cobra.Command, args []string) error {
	fmt.Println(ui.Banner("Status"))
	fmt.Println()

	// Daemon status
	if daemon.IsRunning() {
		fmt.Printf("%s Daemon: %s\n", ui.IconCheck, ui.SuccessStyle.Render("running"))
	} else {
		fmt.Printf("%s Daemon: %s\n", ui.IconDot, ui.MutedStyle.Render("not running"))
	}

	// Load config
	cfg, err := config.Load(config.DefaultConfigPath())
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Daily review count from state store
	dbPath := state.DefaultDBPath()
	store, err := state.Open(dbPath)
	if err != nil {
		fmt.Printf("%s Could not open state store: %s\n", ui.IconCross, ui.MutedStyle.Render(err.Error()))
	} else {
		defer store.Close()
		today := time.Now().Format("2006-01-02")
		count, countErr := store.GetDailyCount(today)
		if countErr == nil {
			fmt.Printf("%s Reviews today: %s / %d\n",
				ui.IconDot,
				ui.BrandStyle.Render(fmt.Sprintf("%d", count)),
				cfg.MaxReviewsPerDay,
			)
		}
	}

	fmt.Println()

	// Repo table
	if len(cfg.Repos) == 0 {
		fmt.Println(ui.MutedStyle.Render("No repositories configured."))
		return nil
	}

	headers := []string{"Repository", "Mode", "Path"}
	var rows [][]string
	for _, repo := range cfg.Repos {
		rows = append(rows, []string{
			repo.Name,
			ui.ModeBadge(repo.Mode),
			repo.Path,
		})
	}

	fmt.Println(ui.StatusTable(headers, rows))
	fmt.Println()

	return nil
}
