package daemon

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/moffa90/pr-sentinel/internal/config"
)

// Daemon log rotation limits.
const (
	logMaxSizeMB  = 10
	logMaxBackups = 3
)

// DaemonLogPath returns the path of the daemon's rotating log file.
func DaemonLogPath() string {
	return filepath.Join(config.ConfigDir(), "daemon.log")
}

// launchdLogPaths are the files launchd writes the daemon's stdout/stderr to.
// With logging redirected to DaemonLogPath they only capture crash output.
func launchdLogPaths() []string {
	dir := config.ConfigDir()
	return []string{
		filepath.Join(dir, "daemon.stdout.log"),
		filepath.Join(dir, "daemon.stderr.log"),
	}
}

// restrictLogPermissions sets 0600 on existing log files, which may contain
// review content. lumberjack keeps the mode of a file it didn't create.
func restrictLogPermissions() {
	for _, p := range append(launchdLogPaths(), DaemonLogPath()) {
		if err := os.Chmod(p, 0o600); err != nil && !os.IsNotExist(err) {
			slog.Warn("failed to restrict log file permissions", "path", p, "error", err)
		}
	}
}

// SetupDaemonLogging routes slog to a size-rotated DaemonLogPath (0600,
// logMaxSizeMB per file, logMaxBackups compressed backups). The returned
// closer flushes and closes the log file.
func SetupDaemonLogging(level slog.Level) io.Closer {
	restrictLogPermissions()

	w := &lumberjack.Logger{
		Filename:   DaemonLogPath(),
		MaxSize:    logMaxSizeMB,
		MaxBackups: logMaxBackups,
		Compress:   true,
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})))
	return w
}
