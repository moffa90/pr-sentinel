package ui

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SpinnerDoneMsg signals that the spinner operation has completed.
type SpinnerDoneMsg struct {
	Result string
}

// SpinnerModel is a bubbletea model that shows a spinner with elapsed time.
type SpinnerModel struct {
	spinner   spinner.Model
	message   string
	startTime time.Time
	done      bool
	result    string
}

// NewSpinner creates a new SpinnerModel with the given message.
func NewSpinner(message string) SpinnerModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color(ColorBrand))

	return SpinnerModel{
		spinner:   s,
		message:   message,
		startTime: time.Now(),
	}
}

// Init implements tea.Model.
func (m SpinnerModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update implements tea.Model.
func (m SpinnerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case SpinnerDoneMsg:
		m.done = true
		m.result = msg.Result
		return m, tea.Quit

	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View implements tea.Model.
func (m SpinnerModel) View() string {
	if m.done {
		return m.result + "\n"
	}

	elapsed := time.Since(m.startTime).Truncate(time.Second)
	return fmt.Sprintf("%s %s %s\n",
		m.spinner.View(),
		m.message,
		MutedStyle.Render(fmt.Sprintf("(elapsed: %s)", elapsed)),
	)
}
