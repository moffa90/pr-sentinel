package daemon

import (
	"bytes"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

func TestSetupDaemonLogging(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	// A pre-existing launchd log with loose permissions gets tightened.
	dir := filepath.Join(home, ".config", "pr-sentinel")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	stderrLog := filepath.Join(dir, "daemon.stderr.log")
	if err := os.WriteFile(stderrLog, []byte("old\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	closer := SetupDaemonLogging(slog.LevelInfo)
	slog.Info("hello from daemon")
	slog.Debug("hidden at info level")
	if err := closer.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	if DaemonLogPath() != filepath.Join(dir, "daemon.log") {
		t.Errorf("DaemonLogPath = %q", DaemonLogPath())
	}
	data, err := os.ReadFile(DaemonLogPath())
	if err != nil {
		t.Fatalf("reading daemon log: %v", err)
	}
	if !strings.Contains(string(data), "hello from daemon") {
		t.Errorf("daemon log missing message: %s", data)
	}
	if strings.Contains(string(data), "hidden at info level") {
		t.Errorf("debug message written at info level: %s", data)
	}

	for _, p := range []string{DaemonLogPath(), stderrLog} {
		info, err := os.Stat(p)
		if err != nil {
			t.Fatalf("stat %s: %v", p, err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("%s perm = %o, want 600", filepath.Base(p), perm)
		}
	}
}

func TestPlistTemplateSetsUmask(t *testing.T) {
	tmpl := template.Must(template.New("plist").Parse(plistTemplate))
	var b bytes.Buffer
	if err := tmpl.Execute(&b, plistData{Label: plistLabel, BinaryPath: "/bin/x", LogDir: "/tmp", Home: "/home"}); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(b.String(), "<key>Umask</key>\n    <integer>63</integer>") {
		t.Errorf("plist missing Umask 63:\n%s", b.String())
	}
}
