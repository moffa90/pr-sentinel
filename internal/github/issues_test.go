package github

import (
	"errors"
	"testing"
)

func TestParseIssueURL(t *testing.T) {
	tests := []struct {
		name string
		out  string
		want string
	}{
		{"single line", "https://github.com/o/r/issues/7\n", "https://github.com/o/r/issues/7"},
		{"with preamble", "\nCreating issue in o/r\n\nhttps://github.com/o/r/issues/12\n", "https://github.com/o/r/issues/12"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseIssueURL(tt.out); got != tt.want {
				t.Errorf("parseIssueURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIssueNumberFromURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    int64
		wantErr bool
	}{
		{"valid", "https://github.com/o/r/issues/42", 42, false},
		{"not an issue url", "https://github.com/o/r/pull/42", 0, true},
		{"non-numeric", "https://github.com/o/r/issues/abc", 0, true},
		{"empty", "", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := issueNumberFromURL(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("got %d, want %d", got, tt.want)
			}
		})
	}
}

func TestIsLabelExistsError(t *testing.T) {
	tests := []struct {
		stderr string
		want   bool
	}{
		{"label with name \"pr-sentinel\" already exists; use `--force` to update its color and description", true},
		{"HTTP 404: Not Found", false},
		{"HTTP 403: Resource not accessible by integration", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isLabelExistsError(tt.stderr); got != tt.want {
			t.Errorf("isLabelExistsError(%q) = %v, want %v", tt.stderr, got, tt.want)
		}
	}
}

func TestIsLabelMissingError(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		{errors.New("gh issue create o/r failed: could not add label: 'pr-sentinel' not found: exit status 1"), true},
		{errors.New("gh issue create o/r failed: HTTP 404: Not Found"), false},
		{errors.New("gh issue create o/r failed: HTTP 502: Bad Gateway"), false},
		{nil, false},
	}
	for _, tt := range tests {
		if got := IsLabelMissingError(tt.err); got != tt.want {
			t.Errorf("IsLabelMissingError(%v) = %v, want %v", tt.err, got, tt.want)
		}
	}
}
