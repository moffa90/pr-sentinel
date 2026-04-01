package commands

import (
	"fmt"
	"path/filepath"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/state"
	"github.com/moffa90/pr-sentinel/internal/ui"
	"github.com/spf13/cobra"
)

// NewLogsCmd creates the logs command showing recent review history.
func NewLogsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logs",
		Short: "Show recent review history",
		RunE:  runLogs,
	}

	cmd.Flags().IntP("count", "n", 20, "Number of recent reviews to show")

	return cmd
}

func runLogs(cmd *cobra.Command, args []string) error {
	count, _ := cmd.Flags().GetInt("count")

	dbPath := filepath.Join(config.ConfigDir(), "state.db")
	store, err := state.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening state store: %w", err)
	}
	defer store.Close()

	records, err := store.RecentReviews(count)
	if err != nil {
		return fmt.Errorf("fetching reviews: %w", err)
	}

	if len(records) == 0 {
		fmt.Println(ui.MutedStyle.Render("No reviews recorded yet."))
		return nil
	}

	fmt.Printf("\n%s Recent reviews (%d):\n\n", ui.IconDot, len(records))

	headers := []string{"Timestamp", "Action", "PR", "Title"}
	var rows [][]string

	for _, r := range records {
		action := ui.MutedStyle.Render("saved")
		if r.Posted {
			action = ui.SuccessStyle.Render("posted")
		}

		ts := r.ReviewedAt.Format("2006-01-02 15:04")
		pr := ui.PRReference(r.Repo, r.PRNumber)

		title := r.PRTitle
		if len(title) > 50 {
			title = title[:47] + "..."
		}

		rows = append(rows, []string{ts, action, pr, title})
	}

	fmt.Println(ui.StatusTable(headers, rows))
	fmt.Println()

	return nil
}
