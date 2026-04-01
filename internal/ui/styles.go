package ui

import "github.com/charmbracelet/lipgloss"

// Color constants.
const (
	ColorBrand   = "#00CED1"
	ColorSuccess = "#2ECC71"
	ColorWarning = "#F1C40F"
	ColorError   = "#E74C3C"
	ColorMuted   = "#95A5A6"
)

// Lipgloss styles.
var (
	BrandStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorBrand))
	SuccessStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorSuccess))
	WarningStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorWarning))
	ErrorStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorError))
	MutedStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorMuted))
	BoldStyle    = lipgloss.NewStyle().Bold(true)
)

// Icon constants.
var (
	IconCheck  = SuccessStyle.Render("✓")
	IconCross  = ErrorStyle.Render("✗")
	IconDot    = BrandStyle.Render("●")
	IconCircle = MutedStyle.Render("○")
	IconShield = "🛡"
)

// Severity icons.
var (
	SeverityHigh   = ErrorStyle.Render("●")
	SeverityMedium = WarningStyle.Render("●")
	SeverityLow    = SuccessStyle.Render("●")
)

// Mode badges.
var (
	ModeLive   = SuccessStyle.Render("🟢 live")
	ModeDryRun = WarningStyle.Render("🟡 dry-run")
)
