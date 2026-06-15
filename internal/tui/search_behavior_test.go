package tui

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"whatsinstalled/internal/store"
)

// contains reports whether s contains substr.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// openTestStore opens a fresh store in a temp dir, closed at test cleanup.
func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// newTestModel returns a model with a usable terminal size for view tests.
func newTestModel(s *store.Store) *model {
	m := NewModel(s)
	m.width = 80
	m.height = 24
	return m
}

// TestSearchEnterNoCrash verifies entering search mode, typing, and pressing
// enter does not crash when no embedder is loaded.
func TestSearchEnterNoCrash(t *testing.T) {
	s := openTestStore(t)

	pkg := store.Package{Name: "testpkg", Version: "1.0.0", Source: "pip", Location: "system", User: "test"}
	if err := s.Upsert(pkg); err != nil {
		t.Fatal(err)
	}

	m := newTestModel(s)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if m.mode != "search" {
		t.Fatalf("expected mode=search, got %s", m.mode)
	}
	for _, r := range "test" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
}

// TestStoreDescriptionMethods verifies the store's description helpers.
func TestStoreDescriptionMethods(t *testing.T) {
	s := openTestStore(t)

	pkgs := []store.Package{
		{Name: "a", Source: "pip", Location: "system", Description: "has desc"},
		{Name: "b", Source: "pip", Location: "system", Description: ""},
		{Name: "c", Source: "apt", Location: "system", Description: ""},
	}
	for _, p := range pkgs {
		if err := s.Upsert(p); err != nil {
			t.Fatal(err)
		}
	}

	missing, err := s.ListWithoutDescriptions("")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 2 {
		t.Fatalf("expected 2 missing, got %d", len(missing))
	}

	count, err := s.CountWithoutDescriptions()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected count=2, got %d", count)
	}

	if err := s.UpdateDescription(missing[0].ID, "new desc"); err != nil {
		t.Fatal(err)
	}
	missing2, err := s.ListWithoutDescriptions("")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing2) != 1 {
		t.Fatalf("expected 1 missing after update, got %d", len(missing2))
	}

	remaining := []store.Package{missing2[0]}
	remaining[0].Description = "batch desc"
	if err := s.UpdateManyDescriptions(remaining); err != nil {
		t.Fatal(err)
	}
	missing3, err := s.ListWithoutDescriptions("")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing3) != 0 {
		t.Fatalf("expected 0 missing after batch update, got %d", len(missing3))
	}
}

// TestSearchWithMissingDescriptions verifies the search-enter flow records the
// query even when packages have no descriptions.
func TestSearchWithMissingDescriptions(t *testing.T) {
	s := openTestStore(t)

	pkgs := []store.Package{
		{Name: "numpy", Source: "pip", Location: "system", Description: "Python arrays"},
		{Name: "find", Source: "bin", Location: "/usr/bin", Description: ""},
		{Name: "grep", Source: "bin", Location: "/usr/bin", Description: ""},
	}
	for _, p := range pkgs {
		if err := s.Upsert(p); err != nil {
			t.Fatal(err)
		}
	}

	m := newTestModel(s)
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	for _, r := range "test" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if m.semanticQuery != "test" {
		t.Fatalf("expected query='test', got %q", m.semanticQuery)
	}
}

// TestCacheTableExists verifies the enrichment_cache table is created on open.
func TestCacheTableExists(t *testing.T) {
	s := openTestStore(t)

	var count int
	var desc string
	err := s.GetEnrichmentCache().QueryRow(
		"SELECT COUNT(*), 'test' FROM enrichment_cache",
	).Scan(&count, &desc)
	if err != nil {
		t.Fatalf("enrichment_cache table should exist: %v", err)
	}
	if desc != "test" {
		t.Fatalf("unexpected result from cache table query")
	}
}

// TestNoDescriptionForExistingData verifies packages that already have a
// description are excluded from the missing list.
func TestNoDescriptionForExistingData(t *testing.T) {
	s := openTestStore(t)

	pkg := store.Package{Name: "numpy", Source: "pip", Location: "system", Description: "Original description"}
	if err := s.Upsert(pkg); err != nil {
		t.Fatal(err)
	}

	missing, err := s.ListWithoutDescriptions("pip")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range missing {
		if p.Name == "numpy" {
			t.Fatal("numpy should not be in missing descriptions list")
		}
	}
}

// TestSearchFlowStateCleanup verifies a successful (non-empty) result closes the
// modal and clears the search state. Empty/failed results intentionally keep the
// modal open with feedback.
func TestSearchFlowStateCleanup(t *testing.T) {
	s := openTestStore(t)

	m := newTestModel(s)
	m.searching = true
	m.mode = "search"
	m.semanticQuery = "test"

	m.Update(semanticSearchResult{results: []store.Package{{Name: "ripgrep", Source: "apt", Location: "system"}}})

	if m.searching {
		t.Fatal("searching should be false after result")
	}
	if m.mode != "" {
		t.Fatalf("mode should be empty after result, got %s", m.mode)
	}
	if m.semanticQuery != "" {
		t.Fatalf("semanticQuery should be empty after result, got %s", m.semanticQuery)
	}
}

// TestBatchDescriptionUpdates verifies many descriptions can be written in one
// batch and clear the missing list.
func TestBatchDescriptionUpdates(t *testing.T) {
	s := openTestStore(t)

	for i := 0; i < 10; i++ {
		pkg := store.Package{Name: fmt.Sprintf("pkg%d", i), Source: "pip", Location: "system"}
		if err := s.Upsert(pkg); err != nil {
			t.Fatal(err)
		}
	}

	missing, err := s.ListWithoutDescriptions("pip")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 10 {
		t.Fatalf("expected 10 missing, got %d", len(missing))
	}

	for i := range missing {
		missing[i].Description = fmt.Sprintf("desc %d", i)
	}
	if err := s.UpdateManyDescriptions(missing); err != nil {
		t.Fatal(err)
	}

	missing2, err := s.ListWithoutDescriptions("pip")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing2) != 0 {
		t.Fatalf("expected 0 missing after batch update, got %d", len(missing2))
	}
}

// TestEmptySearchQuery verifies an empty query yields empty results.
func TestEmptySearchQuery(t *testing.T) {
	s := openTestStore(t)

	m := newTestModel(s)
	m.semanticQuery = ""
	if result, ok := m.startSearch()().(semanticSearchResult); ok {
		if len(result.results) != 0 {
			t.Fatal("empty query should return empty results")
		}
	}
}

// TestSearchWithEmbeddings verifies embeddings round-trip through the store.
func TestSearchWithEmbeddings(t *testing.T) {
	s := openTestStore(t)

	pkg := store.Package{Name: "numpy", Source: "pip", Location: "system", Description: "Python arrays"}
	if err := s.Upsert(pkg); err != nil {
		t.Fatal(err)
	}

	pkgs, err := s.List("", false)
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if err := s.UpdateEmbedding(pkgs[0].ID, "[0.1,0.2,0.3]"); err != nil {
		t.Fatal(err)
	}

	pkgsWithEmb, err := s.ListWithEmbeddings()
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgsWithEmb) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgsWithEmb))
	}
	if pkgsWithEmb[0].Embedding != "[0.1,0.2,0.3]" {
		t.Fatalf("embedding mismatch: %s", pkgsWithEmb[0].Embedding)
	}
}

// TestSearchModalRendering verifies the Ask modal shows the typed query.
func TestSearchModalRendering(t *testing.T) {
	s := openTestStore(t)

	m := newTestModel(s)
	m.scanning = false // init complete: the search modal renders over the dashboard
	m.mode = "search"
	m.semanticQuery = "test query"
	m.searching = false

	view := m.View()
	if !contains(view, "test query") {
		t.Fatal("View should show search query")
	}
}

// TestSearchCancellation verifies Esc closes the modal and clears search state.
func TestSearchCancellation(t *testing.T) {
	s := openTestStore(t)

	m := newTestModel(s)
	m.mode = "search"
	m.semanticQuery = "test"
	m.searching = true

	m.Update(tea.KeyMsg{Type: tea.KeyEscape})

	if m.mode != "" {
		t.Fatalf("expected mode=normal after esc, got %s", m.mode)
	}
	if m.semanticQuery != "" {
		t.Fatalf("expected query cleared, got %s", m.semanticQuery)
	}
	if m.searching {
		t.Fatal("searching should be false after cancel")
	}
}

// TestStatusBarWithSemanticResults verifies the status bar reports result count.
func TestStatusBarWithSemanticResults(t *testing.T) {
	s := openTestStore(t)

	m := newTestModel(s)
	m.semanticResults = []store.Package{{Name: "test", Source: "pip"}}
	m.searching = false

	status := m.renderStatusBar()
	if !contains(status, "semantic search") {
		t.Fatal("Status bar should show 'semantic search' when results exist")
	}
}

// TestSearchTimeout verifies a nil embedder yields an error result rather than
// hanging or panicking.
func TestSearchTimeout(t *testing.T) {
	s := openTestStore(t)

	m := newTestModel(s)
	m.semanticQuery = "test"
	if result, ok := m.startSearch()().(semanticSearchResult); ok {
		if len(result.results) != 0 {
			t.Fatal("should return empty results when embedder is nil")
		}
		if result.err == "" {
			t.Fatal("should return error when embedder is nil")
		}
	}
}

// TestConcurrentSearchAndScan verifies the view renders while both scanning and
// searching are active.
func TestConcurrentSearchAndScan(t *testing.T) {
	s := openTestStore(t)

	m := newTestModel(s)
	m.scanning = true
	m.searching = true

	if view := m.View(); view == "" {
		t.Fatal("View returned empty string")
	}
}

// TestSearchResultTreeBuild verifies search results populate the tree.
func TestSearchResultTreeBuild(t *testing.T) {
	s := openTestStore(t)

	m := newTestModel(s)
	results := []store.Package{
		{Name: "numpy", Source: "pip", Location: "system"},
		{Name: "find", Source: "bin", Location: "/usr/bin"},
	}
	m.Update(semanticSearchResult{results: results})

	if len(m.tree.roots) == 0 {
		t.Fatal("Tree should have roots after search results")
	}
}

// TestEmptySearchResults verifies empty results surface modal feedback.
func TestEmptySearchResults(t *testing.T) {
	s := openTestStore(t)

	m := newTestModel(s)
	m.Update(semanticSearchResult{results: []store.Package{}})

	if m.searchMsg == "" {
		t.Fatal("searchMsg should be set for empty results")
	}
}

// TestSearchQueryBuilding verifies typed runes (including spaces) build the query.
func TestSearchQueryBuilding(t *testing.T) {
	s := openTestStore(t)

	m := newTestModel(s)
	m.mode = "search"
	m.semanticQuery = ""

	for _, r := range "filesystem tools" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	if m.semanticQuery != "filesystem tools" {
		t.Fatalf("expected query='filesystem tools', got %s", m.semanticQuery)
	}
}
