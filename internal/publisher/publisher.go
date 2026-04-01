package publisher

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/moffa90/pr-sentinel/internal/github"
)

// SaveParams holds the parameters needed to save a dry-run review to disk.
type SaveParams struct {
	ReviewsDir string
	Repo       string
	PRNumber   int64
	PRTitle    string
	PRAuthor   string
	Body       string
}

// BuildReviewBody builds the final review body string.
// If aiDisclosure is true and disclosureText is non-empty, it is prepended.
// The review output is appended, followed by a cc mention of the PR author.
func BuildReviewBody(reviewOutput string, aiDisclosure bool, disclosureText string, prAuthor string) string {
	var b strings.Builder

	if aiDisclosure && disclosureText != "" {
		b.WriteString(disclosureText)
		b.WriteString("\n\n")
	}

	b.WriteString(reviewOutput)
	b.WriteString("\n\ncc @")
	b.WriteString(prAuthor)

	return b.String()
}

// PostLiveReview posts a review comment on a pull request via the GitHub CLI.
func PostLiveReview(repo string, prNumber int64, body string) error {
	return github.PostReview(repo, prNumber, body)
}

// SaveDryRunReview writes the review to a markdown file in the given directory
// and returns the file path.
func SaveDryRunReview(params SaveParams) (string, error) {
	if err := os.MkdirAll(params.ReviewsDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create reviews directory: %w", err)
	}

	path := reviewFilePath(params.ReviewsDir, params.Repo, params.PRNumber)

	header := fmt.Sprintf("# Review: %s#%d\n\n"+
		"- **Title:** %s\n"+
		"- **Author:** %s\n"+
		"- **Timestamp:** %s\n"+
		"- **Mode:** dry-run\n\n---\n\n",
		params.Repo, params.PRNumber,
		params.PRTitle,
		params.PRAuthor,
		time.Now().UTC().Format(time.RFC3339),
	)

	content := header + params.Body

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", fmt.Errorf("failed to write review file: %w", err)
	}

	return path, nil
}

// reviewFilePath returns the file path for a dry-run review markdown file.
// Slashes in the repo name are replaced with dashes.
func reviewFilePath(dir string, repo string, prNumber int64) string {
	safeName := strings.ReplaceAll(repo, "/", "-")
	return fmt.Sprintf("%s/%s-%d.md", dir, safeName, prNumber)
}
