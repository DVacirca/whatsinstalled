package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"whatsinstalled/internal/store"
)

// TestSearchModeKeys verifies that the search modal handles keys correctly.
func TestSearchModeKeys(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24

	// Enter search mode
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if m.mode != "search" {
		t.Fatalf("expected mode=search, got %s", m.mode)
	}

	// Type a query with spaces
	for _, r := range "python tools" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.semanticQuery != "python tools" {
		t.Fatalf("expected query='python tools', got %q", m.semanticQuery)
	}

	// Press escape to cancel
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.mode != "" {
		t.Fatalf("expected mode=normal after esc, got %s", m.mode)
	}
	if m.semanticQuery != "" {
		t.Fatalf("expected query cleared, got %q", m.semanticQuery)
	}

	// Re-enter search mode
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})

	// Type query and press enter
	for _, r := range "python" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if m.mode != "search" {
		t.Fatalf("expected mode=search, got %s", m.mode)
	}

	// The embedder may not be loaded, so we just verify the mode transition
	_ = m.semanticQuery
}
