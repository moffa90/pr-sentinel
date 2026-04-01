package notifier

import (
	"fmt"
	"os/exec"
	"strings"
)

// MacOSNotifier sends notifications via macOS notification center.
type MacOSNotifier struct{}

// NewMacOSNotifier creates a new MacOSNotifier.
func NewMacOSNotifier() *MacOSNotifier {
	return &MacOSNotifier{}
}

// Notify displays a macOS notification using osascript.
func (m *MacOSNotifier) Notify(e Event) error {
	status := "posted"
	if !e.Posted {
		status = "dry-run"
	}

	body := fmt.Sprintf("[%s] %s by %s (%s)", e.Mode, e.PRTitle, e.PRAuthor, status)
	subtitle := fmt.Sprintf("%s#%d", e.Repo, e.PRNumber)

	// Escape double quotes in body and subtitle for AppleScript.
	body = strings.ReplaceAll(body, `"`, `\"`)
	subtitle = strings.ReplaceAll(subtitle, `"`, `\"`)

	script := fmt.Sprintf(`display notification "%s" with title "pr-sentinel" subtitle "%s"`, body, subtitle)

	cmd := exec.Command("osascript", "-e", script)
	return cmd.Run()
}
