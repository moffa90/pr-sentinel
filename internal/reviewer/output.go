package reviewer

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Verdict represents the review decision.
type Verdict string

const (
	VerdictApprove        Verdict = "approve"
	VerdictComment        Verdict = "comment"
	VerdictRequestChanges Verdict = "request-changes"
)

// Finding represents a single review finding.
type Finding struct {
	Severity string `json:"severity"`
	File     string `json:"file"`
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
}

// StructuredReview is the parsed review output from Claude.
type StructuredReview struct {
	Verdict  Verdict   `json:"verdict"`
	Summary  string    `json:"summary"`
	Findings []Finding `json:"findings"`
}

// claudeEnvelope is the JSON wrapper that claude CLI returns with --output-format json.
type claudeEnvelope struct {
	Type             string           `json:"type"`
	Subtype          string           `json:"subtype"`
	IsError          bool             `json:"is_error"`
	Result           string           `json:"result"`
	StructuredOutput *StructuredReview `json:"structured_output"`
	CostUSD          float64          `json:"total_cost_usd"`
}

// ReviewJSON schema passed to claude CLI via --json-schema.
const ReviewJSONSchema = `{
  "type": "object",
  "properties": {
    "verdict": {
      "type": "string",
      "enum": ["approve", "comment", "request-changes"],
      "description": "The review decision"
    },
    "summary": {
      "type": "string",
      "description": "A concise summary of the review (1-3 sentences)"
    },
    "findings": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "severity": {
            "type": "string",
            "enum": ["HIGH", "MEDIUM", "LOW"]
          },
          "file": {
            "type": "string",
            "description": "The file path where the issue was found"
          },
          "line": {
            "type": "integer",
            "description": "The line number if applicable"
          },
          "message": {
            "type": "string",
            "description": "Description of the finding"
          }
        },
        "required": ["severity", "file", "message"]
      }
    }
  },
  "required": ["verdict", "summary", "findings"]
}`

// ParseCLIOutput parses the claude CLI JSON envelope and extracts the
// structured review. Returns the parsed review and the raw result string.
// If parsing fails at any stage, returns an error but still provides
// whatever raw output was available.
func ParseCLIOutput(data string) (*StructuredReview, string, error) {
	// Parse the outer envelope
	var env claudeEnvelope
	if err := json.Unmarshal([]byte(data), &env); err != nil {
		// Not a JSON envelope — treat entire output as raw text
		return nil, data, fmt.Errorf("not a claude CLI JSON envelope: %w", err)
	}

	if env.IsError {
		return nil, env.Result, fmt.Errorf("claude returned error (%s): %s", env.Subtype, env.Result)
	}

	// Prefer structured_output field (populated when --json-schema is used)
	if env.StructuredOutput != nil {
		raw, _ := json.Marshal(env.StructuredOutput)
		return env.StructuredOutput, string(raw), nil
	}

	// Fallback: try to parse the result field as JSON
	if env.Result != "" {
		var review StructuredReview
		if err := json.Unmarshal([]byte(env.Result), &review); err != nil {
			return nil, env.Result, fmt.Errorf("failed to parse review JSON: %w", err)
		}
		return &review, env.Result, nil
	}

	return nil, "", fmt.Errorf("claude returned empty result")
}

// FindingsSummary returns a human-readable summary of findings by severity.
func (r *StructuredReview) FindingsSummary() string {
	if len(r.Findings) == 0 {
		return "No issues found"
	}

	counts := map[string]int{}
	for _, f := range r.Findings {
		counts[f.Severity]++
	}

	var parts []string
	for _, sev := range []string{"HIGH", "MEDIUM", "LOW"} {
		if c, ok := counts[sev]; ok {
			parts = append(parts, fmt.Sprintf("%d %s", c, sev))
		}
	}
	return strings.Join(parts, ", ")
}

// FormatMarkdown renders the structured review as a markdown comment
// suitable for posting to GitHub.
func (r *StructuredReview) FormatMarkdown() string {
	var b strings.Builder

	// Verdict badge
	switch r.Verdict {
	case VerdictApprove:
		b.WriteString("**Verdict: Approved** :white_check_mark:\n\n")
	case VerdictRequestChanges:
		b.WriteString("**Verdict: Changes Requested** :x:\n\n")
	default:
		b.WriteString("**Verdict: Comment** :speech_balloon:\n\n")
	}

	b.WriteString(r.Summary)
	b.WriteString("\n")

	if len(r.Findings) == 0 {
		b.WriteString("\nNo issues found.\n")
		return b.String()
	}

	b.WriteString("\n### Findings\n\n")

	for _, f := range r.Findings {
		icon := "🔵"
		switch f.Severity {
		case "HIGH":
			icon = "🔴"
		case "MEDIUM":
			icon = "🟠"
		}

		location := f.File
		if f.Line > 0 {
			location = fmt.Sprintf("%s:%d", f.File, f.Line)
		}

		fmt.Fprintf(&b, "%s **%s** `%s`\n%s\n\n", icon, f.Severity, location, f.Message)
	}

	return b.String()
}
