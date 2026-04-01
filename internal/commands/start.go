package commands

import (
	"fmt"

	"github.com/moffa90/pr-sentinel/internal/ui"
	"github.com/spf13/cobra"
)

// NewStartCmd creates the start command for running the daemon loop.
func NewStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the review daemon",
		Long:  "Polls watched repositories for new PRs and runs Claude Code reviews automatically.",
		RunE:  runStart,
	}

	cmd.Flags().BoolP("daemon", "d", false, "Run as launchd daemon (detached)")

	return cmd
}

// NewStopCmd creates the stop command for stopping the daemon.
func NewStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the review daemon",
		RunE:  runStop,
	}
}

func runStart(cmd *cobra.Command, args []string) error {
	fmt.Println(ui.Banner("Daemon"))
	fmt.Println()
	fmt.Printf("%s Daemon not implemented yet\n", ui.IconDot)
	fmt.Println(ui.MutedStyle.Render("  The daemon loop will be wired in a future task."))
	return nil
}

func runStop(cmd *cobra.Command, args []string) error {
	fmt.Printf("%s Daemon not implemented yet\n", ui.IconDot)
	fmt.Println(ui.MutedStyle.Render("  The stop command will be wired in a future task."))
	return nil
}
