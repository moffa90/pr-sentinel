package commands

import (
	"fmt"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/notifier"
	"github.com/spf13/cobra"
)

func NewNotifyTestCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "notify-test",
		Short: "Send a test notification to verify webhook configuration",
		Long:  "Sends a test notification to Teams. Use --repo to test a specific repo's webhook, otherwise tests the global one.",
		RunE:  runNotifyTest,
	}
	cmd.Flags().StringP("repo", "r", "", "Test a specific repo's Teams webhook (e.g. owner/repo)")
	return cmd
}

func runNotifyTest(cmd *cobra.Command, args []string) error {
	repoFlag, _ := cmd.Flags().GetString("repo")

	cfg, err := config.Load(config.DefaultConfigPath())
	if err != nil {
		return fmt.Errorf("loading config: %w\nRun `pr-sentinel init` first", err)
	}

	evt := notifier.NewEvent(
		"pr-sentinel/test", 0, "Test notification", "pr-sentinel", "",
		"test", false, "This is a test notification from pr-sentinel", "",
	)

	if repoFlag != "" {
		repo, found := cfg.FindRepo(repoFlag)
		if !found {
			return fmt.Errorf("repo %q not found in config", repoFlag)
		}
		if repo.TeamsWebhook == "" {
			return fmt.Errorf("repo %q has no teams_webhook configured", repoFlag)
		}

		evt.Repo = repo.Name
		n := notifier.NewTeamsNotifier(repo.TeamsWebhook)
		if err := n.Notify(evt); err != nil {
			return fmt.Errorf("teams notification failed: %w", err)
		}
		fmt.Printf("  Test notification sent to %s Teams webhook\n", repo.Name)
		return nil
	}

	// Global Teams webhook
	if !cfg.Notifications.Teams.Enabled || cfg.Notifications.Teams.WebhookURL == "" {
		return fmt.Errorf("global Teams webhook is not configured — set notifications.teams in config.yaml")
	}

	n := notifier.NewTeamsNotifier(cfg.Notifications.Teams.WebhookURL)
	if err := n.Notify(evt); err != nil {
		return fmt.Errorf("teams notification failed: %w", err)
	}
	fmt.Println("  Test notification sent to global Teams webhook")
	return nil
}
