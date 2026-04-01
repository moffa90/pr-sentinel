package ui

import "github.com/charmbracelet/lipgloss"

// Banner renders a centered box with rounded border in brand color
// containing the pr-sentinel title and subtitle.
func Banner(subtitle string) string {
	content := IconShield + "  pr-sentinel — " + subtitle

	style := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(ColorBrand)).
		Padding(0, 2).
		Align(lipgloss.Center)

	return style.Render(content)
}
