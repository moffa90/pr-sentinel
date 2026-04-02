package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/moffa90/pr-sentinel/internal/config"
)

const plistLabel = "com.moffa90.pr-sentinel"

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{.Label}}</string>
    <key>ProgramArguments</key>
    <array>
        <string>{{.BinaryPath}}</string>
        <string>start</string>
        <string>--daemon-mode</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>{{.LogDir}}/daemon.stdout.log</string>
    <key>StandardErrorPath</key>
    <string>{{.LogDir}}/daemon.stderr.log</string>
    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>/usr/local/bin:/opt/homebrew/bin:/usr/bin:/bin</string>
        <key>HOME</key>
        <string>{{.Home}}</string>
    </dict>
</dict>
</plist>
`

type plistData struct {
	Label      string
	BinaryPath string
	LogDir     string
	Home       string
}

// PlistPath returns the path to the launchd plist file.
func PlistPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, "Library", "LaunchAgents", plistLabel+".plist")
}

// InstallPlist writes the launchd plist file for the daemon.
func InstallPlist() error {
	binPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to find executable path: %w", err)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	logDir := config.ConfigDir()
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		return fmt.Errorf("failed to create log directory: %w", err)
	}

	plistDir := filepath.Dir(PlistPath())
	if err := os.MkdirAll(plistDir, 0o755); err != nil {
		return fmt.Errorf("failed to create LaunchAgents directory: %w", err)
	}

	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse plist template: %w", err)
	}

	f, err := os.OpenFile(PlistPath(), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("failed to create plist file: %w", err)
	}
	defer f.Close()

	data := plistData{
		Label:      plistLabel,
		BinaryPath: binPath,
		LogDir:     logDir,
		Home:       home,
	}

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("failed to write plist: %w", err)
	}

	return nil
}

// LoadPlist loads the launchd plist via launchctl.
func LoadPlist() error {
	cmd := exec.Command("launchctl", "load", PlistPath())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl load failed: %s: %w", string(out), err)
	}
	return nil
}

// UnloadPlist unloads the launchd plist via launchctl.
func UnloadPlist() error {
	cmd := exec.Command("launchctl", "unload", PlistPath())
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launchctl unload failed: %s: %w", string(out), err)
	}
	return nil
}

// IsRunning returns true if the daemon is currently loaded in launchctl.
func IsRunning() bool {
	cmd := exec.Command("launchctl", "list", plistLabel)
	return cmd.Run() == nil
}
