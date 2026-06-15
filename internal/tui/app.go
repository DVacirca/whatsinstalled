package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"whatsinstalled/internal/store"
)

// Run starts the Bubble Tea program for the dashboard, backed by the given
// store, and blocks until the user quits.
func Run(s *store.Store) error {
	p := tea.NewProgram(NewModel(s), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("run tui: %w", err)
	}
	return nil
}
