package github

import "testing"

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
