package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/moffa90/pr-sentinel/internal/config"
	"github.com/moffa90/pr-sentinel/internal/daemon"
	"github.com/moffa90/pr-sentinel/internal/state"
	"github.com/moffa90/pr-sentinel/internal/ui"
	"github.com/spf13/cobra"
)

func NewStartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start watching repos for new PRs",
		Long:  "Polls watched repositories for new PRs and runs Claude Code reviews automatically.",
		RunE:  runStart,
	}
	cmd.Flags().BoolP("daemon", "d", false, "Run as launchd daemon (detached)")
	cmd.Flags().Bool("daemon-mode", false, "Internal flag for launchd invocation")
	_ = cmd.Flags().MarkHidden("daemon-mode")
	return cmd
}

func NewStopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the review daemon",
		RunE:  runStop,
	}
}

func runStart(cmd *cobra.Command, args []string) error {
	daemonFlag, _ := cmd.Flags().GetBool("daemon")
	daemonMode, _ := cmd.Flags().GetBool("daemon-mode")

	if daemonFlag {
		return startDaemon()
	}

	cfg, err := config.Load(config.DefaultConfigPath())
	if err != nil {
		return fmt.Errorf("loading config: %w\nRun `pr-sentinel init` first", err)
	}

	if len(cfg.Repos) == 0 {
		return fmt.Errorf("no repos configured — run `pr-sentinel init` first")
	}

	// Open state store
	dbPath := filepath.Join(config.ConfigDir(), "state.db")
	store, err := state.Open(dbPath)
	if err != nil {
		return fmt.Errorf("opening state store: %w", err)
	}
	defer store.Close()

	// Build notifier
	notify := daemon.BuildNotifier(cfg)

	// Setup context with signal handling
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	if !daemonMode {
		liveCount := 0
		dryCount := 0
		for _, r := range cfg.Repos {
			if r.Mode == config.ModeLive {
				liveCount++
			} else {
				dryCount++
			}
		}

		fmt.Println(ui.Banner("Watching"))
		fmt.Println()
		fmt.Printf("  Tracking %d repos  ·  Poll every %s  ·  %d live · %d dry-run\n\n",
			len(cfg.Repos), cfg.PollInterval, liveCount, dryCount)
	}

	err = daemon.RunDaemon(ctx, cfg, store, notify)
	if err != nil && err == context.Canceled {
		fmt.Println() // newline after ^C
		return nil
	}
	return err
}

func startDaemon() error {
	if err := daemon.InstallPlist(); err != nil {
		return fmt.Errorf("installing plist: %w", err)
	}

	if err := daemon.LoadPlist(); err != nil {
		return fmt.Errorf("loading daemon: %w", err)
	}

	fmt.Printf("\n  %s Daemon started\n", ui.IconCheck)
	fmt.Printf("  Logs: %s\n\n", ui.MutedStyle.Render(filepath.Join(config.ConfigDir(), "daemon.log")))
	return nil
}

func runStop(cmd *cobra.Command, args []string) error {
	if !daemon.IsRunning() {
		fmt.Printf("\n  %s Daemon is not running\n\n", ui.IconCircle)
		return nil
	}

	if err := daemon.UnloadPlist(); err != nil {
		return fmt.Errorf("stopping daemon: %w", err)
	}

	fmt.Printf("\n  %s Daemon stopped\n\n", ui.IconCheck)
	return nil
}
