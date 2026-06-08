package tui

import (
	"testing"

	"installr/internal/store"
)

// TestSemanticSearchFlow tests the full TUI semantic search pipeline.
func TestSemanticSearchFlow(t *testing.T) {
	// Create temp db
	tmpDir := t.TempDir()
	dbPath := tmpDir + "/test.db"
	s, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Seed packages
	pkgs := []store.Package{
		{Name: "pip", Source: "pip", Location: "system", Description: "python package installer"},
		{Name: "uv", Source: "pip", Location: "system", Description: "fast python package manager"},
		{Name: "npm", Source: "npm", Location: "system", Description: "node package manager"},
		{Name: "curl", Source: "apt", Location: "system", Description: "transfer data"},
	}
	for _, p := range pkgs {
		if err := s.Upsert(p); err != nil {
			t.Fatal(err)
		}
	}

	// Create model
	m := NewModel(s)

	// Test that embedder loaded (model may not be cached, so skip if not)
	if m.embedder == nil {
		t.Skip("embedder not loaded (model may not be cached)")
	}

	// Simulate entering search mode
	m.mode = "search"
	m.semanticQuery = "python tools"
	m.searching = true

	// Run semantic search (startSearch returns a tea.Cmd)
	cmd := m.startSearch()
	msg := cmd()
	result, ok := msg.(semanticSearchResult)
	if !ok {
		t.Fatalf("expected semanticSearchResult, got %T", msg)
	}

	m.searching = false
	if len(result.results) == 0 {
		t.Fatal("no results returned")
	}

	// Check top result is a python-related package
	top := result.results[0]
	if top.Name != "pip" && top.Name != "uv" {
		t.Fatalf("expected python tool, got %s", top.Name)
	}

	t.Logf("top result: %s (score implied by order)", top.Name)
	for _, r := range result.results {
		t.Logf("  - %s (%s): %s", r.Name, r.Source, r.Description)
	}
}
