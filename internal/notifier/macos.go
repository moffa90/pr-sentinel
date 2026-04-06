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
	verdict := e.Verdict
	if verdict == "" {
		if e.Posted {
			verdict = "posted"
		} else {
			verdict = "dry-run"
		}
	}

	body := fmt.Sprintf("[%s] %s by %s", verdict, e.PRTitle, e.PRAuthor)
	subtitle := fmt.Sprintf("%s#%d", e.Repo, e.PRNumber)

	script := fmt.Sprintf(
		`display notification %s with title "pr-sentinel" subtitle %s sound name "Glass"`,
		escapeAppleScript(body),
		escapeAppleScript(subtitle),
	)

	cmd := exec.Command("osascript", "-e", script)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("osascript notification failed: %w", err)
	}
	return nil
}

// escapeAppleScript returns a safely quoted AppleScript string literal.
// It wraps the value in double quotes after escaping backslashes and double quotes.
func escapeAppleScript(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
