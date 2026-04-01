package main

import (
	"fmt"
	"os"

	"github.com/moffa90/pr-sentinel/internal/commands"
	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "pr-sentinel",
	Short: "Automated PR review powered by Claude Code",
	Long:  "pr-sentinel watches your GitHub repos for new pull requests and runs Claude Code reviews locally, preserving your full .claude/ context.",
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version info",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("pr-sentinel %s\n", version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(commands.NewInitCmd())
	rootCmd.AddCommand(commands.NewStartCmd())
	rootCmd.AddCommand(commands.NewStopCmd())
	rootCmd.AddCommand(commands.NewStatusCmd())
	rootCmd.AddCommand(commands.NewReviewCmd())
	rootCmd.AddCommand(commands.NewReposCmd())
	rootCmd.AddCommand(commands.NewPromoteCmd())
	rootCmd.AddCommand(commands.NewDemoteCmd())
	rootCmd.AddCommand(commands.NewLogsCmd())
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
