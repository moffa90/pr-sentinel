package prerequisites

import (
	"testing"
)

func TestParseGhUser_Authenticated(t *testing.T) {
	output := "github.com\n  \u2713 Logged in to github.com account jmoffa (keyring)\n  - Active account: true\n"

	user, ok := parseGhUser(output)
	if !ok {
		t.Fatal("expected ok to be true for authenticated output")
	}
	if user != "jmoffa" {
		t.Fatalf("expected user %q, got %q", "jmoffa", user)
	}
}

func TestParseGhUser_NotAuthenticated(t *testing.T) {
	output := "You are not logged in to any GitHub hosts. Run gh auth login to authenticate.\n"

	user, ok := parseGhUser(output)
	if ok {
		t.Fatal("expected ok to be false for unauthenticated output")
	}
	if user != "" {
		t.Fatalf("expected empty user, got %q", user)
	}
}

func TestCheckResult_Fields(t *testing.T) {
	r := CheckResult{
		Name:     "test-tool",
		Found:    true,
		Version:  "1.0.0",
		Detail:   "/usr/bin/test-tool",
		HelpText: "Install test-tool",
	}

	if r.Name != "test-tool" {
		t.Fatalf("expected Name %q, got %q", "test-tool", r.Name)
	}
	if !r.Found {
		t.Fatal("expected Found to be true")
	}
	if r.Version != "1.0.0" {
		t.Fatalf("expected Version %q, got %q", "1.0.0", r.Version)
	}
	if r.Detail != "/usr/bin/test-tool" {
		t.Fatalf("expected Detail %q, got %q", "/usr/bin/test-tool", r.Detail)
	}
	if r.HelpText != "Install test-tool" {
		t.Fatalf("expected HelpText %q, got %q", "Install test-tool", r.HelpText)
	}
}
