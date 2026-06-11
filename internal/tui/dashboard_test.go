package tui

import (
	"context"
	"testing"

	"installr/internal/nlp"
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

	// Pre-compute embeddings for all packages so search runs immediately
	allPkgs, _ := s.List("")
	for _, p := range allPkgs {
		text := nlp.PackageText(p.Name, p.Source, p.Description)
		vec, err := m.embedder.Encode(context.Background(), text)
		if err != nil {
			t.Fatalf("embed package %s: %v", p.Name, err)
		}
		s.UpdateEmbedding(p.ID, nlp.ToJSON(vec))
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

// TestStaleSearchResultIgnored verifies that a late result from an old search
// does not overwrite the current search state.
func TestStaleSearchResultIgnored(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	pkgs := []store.Package{
		{Name: "pip", Source: "pip", Location: "system", Description: "python package installer"},
		{Name: "npm", Source: "npm", Location: "system", Description: "node package manager"},
	}
	for _, p := range pkgs {
		if err := s.Upsert(p); err != nil {
			t.Fatal(err)
		}
	}

	m := NewModel(s)
	if m.embedder == nil {
		t.Skip("embedder not loaded")
	}

	allPkgs, _ := s.List("")
	for _, p := range allPkgs {
		text := nlp.PackageText(p.Name, p.Source, p.Description)
		vec, _ := m.embedder.Encode(context.Background(), text)
		s.UpdateEmbedding(p.ID, nlp.ToJSON(vec))
	}

	// First search starts
	m.mode = "search"
	m.semanticQuery = "python tools"
	m.searchVersion = 1
	m.searching = true
	oldVersion := m.searchVersion

	// User starts a second search before the first finishes
	m.searchVersion++
	m.semanticQuery = "node tools"
	m.semanticResults = nil

	// Old search finishes with stale version
	oldResult := semanticSearchResult{
		results: []store.Package{{Name: "pip", Source: "pip"}},
		version: oldVersion,
	}
	newM, _ := m.Update(oldResult)
	mm := newM.(*model)

	if !mm.searching {
		t.Fatal("expected searching to remain true after ignoring stale result")
	}
	if len(mm.semanticResults) != 0 {
		t.Fatalf("expected semanticResults empty, got %d", len(mm.semanticResults))
	}
	if mm.mode != "search" {
		t.Fatalf("expected mode to remain search, got %s", mm.mode)
	}

	// New search finishes with current version
	newResult := semanticSearchResult{
		results: []store.Package{{Name: "npm", Source: "npm"}},
		version: mm.searchVersion,
	}
	newM2, _ := mm.Update(newResult)
	mm2 := newM2.(*model)

	if mm2.searching {
		t.Fatal("expected searching false after accepting current result")
	}
	if len(mm2.semanticResults) != 1 || mm2.semanticResults[0].Name != "npm" {
		t.Fatalf("expected npm result, got %v", mm2.semanticResults)
	}
}
