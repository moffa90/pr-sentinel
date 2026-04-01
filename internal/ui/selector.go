package ui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// RepoItem represents a selectable repository entry.
type RepoItem struct {
	Name     string
	Path     string
	Selected bool
}

// SelectorModel is a bubbletea model for multi-select repository picking.
type SelectorModel struct {
	items    []RepoItem
	cursor   int
	done     bool
	quitting bool
}

// NewSelector creates a new SelectorModel with the given items.
func NewSelector(items []RepoItem) SelectorModel {
	return SelectorModel{
		items: items,
	}
}

// Init implements tea.Model.
func (m SelectorModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m SelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(m.items)-1 {
				m.cursor++
			}

		case " ":
			if m.cursor < len(m.items) {
				m.items[m.cursor].Selected = !m.items[m.cursor].Selected
			}

		case "a":
			allSelected := true
			for _, item := range m.items {
				if !item.Selected {
					allSelected = false
					break
				}
			}
			for i := range m.items {
				m.items[i].Selected = !allSelected
			}

		case "enter":
			m.done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

// View implements tea.Model.
func (m SelectorModel) View() string {
	if m.done || m.quitting {
		return ""
	}

	var b strings.Builder
	b.WriteString(BrandStyle.Render("Select repositories:") + "\n\n")

	for i, item := range m.items {
		cursor := "  "
		if i == m.cursor {
			cursor = BrandStyle.Render("> ")
		}

		check := IconCircle
		if item.Selected {
			check = IconCheck
		}

		name := item.Name
		if i == m.cursor {
			name = BoldStyle.Render(name)
		}

		path := MutedStyle.Render(item.Path)
		b.WriteString(fmt.Sprintf("%s%s %s %s\n", cursor, check, name, path))
	}

	b.WriteString("\n" + MutedStyle.Render("↑↓ navigate · space toggle · a select all · enter confirm · q quit") + "\n")
	return b.String()
}

// SelectedItems returns the items that were selected.
func (m SelectorModel) SelectedItems() []RepoItem {
	var selected []RepoItem
	for _, item := range m.items {
		if item.Selected {
			selected = append(selected, item)
		}
	}
	return selected
}

// Quitting returns true if the user quit without confirming.
func (m SelectorModel) Quitting() bool {
	return m.quitting
}

// RunSelector runs the bubbletea selector program and returns the selected items.
func RunSelector(items []RepoItem) ([]RepoItem, error) {
	model := NewSelector(items)
	p := tea.NewProgram(model)

	finalModel, err := p.Run()
	if err != nil {
		return nil, fmt.Errorf("selector error: %w", err)
	}

	m, ok := finalModel.(SelectorModel)
	if !ok {
		return nil, fmt.Errorf("unexpected model type")
	}

	if m.Quitting() {
		return nil, nil
	}

	return m.SelectedItems(), nil
}
