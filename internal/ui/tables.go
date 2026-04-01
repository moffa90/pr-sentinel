package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// StatusTable renders a bordered table with the given headers and rows.
func StatusTable(headers []string, rows [][]string) string {
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(ColorBrand)).
		Align(lipgloss.Center)

	cellStyle := lipgloss.NewStyle().
		Padding(0, 1)

	t := table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(lipgloss.Color(ColorMuted))).
		Headers(headers...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			return cellStyle
		})

	for _, row := range rows {
		t.Row(row...)
	}

	return t.Render()
}

// ModeBadge returns the appropriate mode badge string for the given mode.
func ModeBadge(mode string) string {
	switch strings.ToLower(mode) {
	case "live":
		return ModeLive
	case "dry-run", "dryrun", "dry_run":
		return ModeDryRun
	default:
		return MutedStyle.Render(mode)
	}
}

// PRReference renders a pull request reference in brand color (e.g., "repo#42").
func PRReference(repo string, number int64) string {
	return BrandStyle.Render(fmt.Sprintf("%s#%d", repo, number))
}

// Separator renders a labeled separator line.
func Separator(label string) string {
	return MutedStyle.Render("─── "+label+" ") + MutedStyle.Render(strings.Repeat("─", 40))
}
