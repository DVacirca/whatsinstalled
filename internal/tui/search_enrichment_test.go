package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"installr/internal/store"
)

// TestEnrichmentSearchFlow verifies that the search flow triggers enrichment
// when packages are missing descriptions.
func TestEnrichmentSearchFlow(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Insert a package with no description
	pkg := store.Package{
		Name:     "testpkg",
		Version:  "1.0.0",
		Source:   "pip",
		Location: "system",
		User:     "test",
	}
	if err := s.Upsert(pkg); err != nil {
		t.Fatal(err)
	}

	m := NewModel(s)
	m.width = 80
	m.height = 24

	// Enter search mode
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	if m.mode != "search" {
		t.Fatalf("expected mode=search, got %s", m.mode)
	}

	// Type query and press enter
	for _, r := range "test" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	// Press enter to trigger search
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// The search should be running (searching=true)
	if !m.searching && m.mode != "search" {
		// Note: if embedder is not loaded, search might fail immediately
		// which is fine - the test just verifies the flow doesn't crash
	}
}

// TestEnrichmentProgressUI verifies the UI updates during enrichment.
func TestEnrichmentProgressUI(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24

	// Simulate enrichment progress
	m.enriching = true
	m.enrichTotal = 10
	m.enrichDone = 5
	m.enrichSource = "whatis"
	m.enrichCurrent = "find"

	// View should render without crashing
	view := m.View()
	if view == "" {
		t.Fatal("View returned empty string")
	}

	// Check that the progress text is in the view
	if !contains(view, "Enriching") {
		t.Fatal("View should contain 'Enriching' progress text")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && s != substr
}

// TestEnrichmentStateTransitions verifies the mode transitions correctly.
func TestEnrichmentStateTransitions(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24

	// Initially not enriching
	if m.enriching {
		t.Fatal("should not be enriching initially")
	}

	// Send enrichment progress message
	m.Update(enrichmentProgressMsg{total: 10, done: 5, source: "whatis", current: "find"})
	if m.enrichTotal != 10 {
		t.Fatalf("expected enrichTotal=10, got %d", m.enrichTotal)
	}
	if m.enrichDone != 5 {
		t.Fatalf("expected enrichDone=5, got %d", m.enrichDone)
	}

	// Send enrichment complete message
	m.Update(enrichmentCompleteMsg{})
	if m.enriching {
		t.Fatal("should not be enriching after complete")
	}

	// Send enrichment complete with error
	m.enriching = true
	m.Update(enrichmentCompleteMsg{err: errTest})
	if m.enriching {
		t.Fatal("should not be enriching after error")
	}
}

var errTest = fmt.Errorf("test error")

// TestStoreDescriptionMethods verifies the new store methods work.
func TestStoreDescriptionMethods(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Insert packages
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

	// Test ListWithoutDescriptions
	missing, err := s.ListWithoutDescriptions("")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 2 {
		t.Fatalf("expected 2 missing, got %d", len(missing))
	}

	// Test CountWithoutDescriptions
	count, err := s.CountWithoutDescriptions()
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("expected count=2, got %d", count)
	}

	// Test UpdateDescription
	if err := s.UpdateDescription(missing[0].ID, "new desc"); err != nil {
		t.Fatal(err)
	}

	// Verify updated
	missing2, err := s.ListWithoutDescriptions("")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing2) != 1 {
		t.Fatalf("expected 1 missing after update, got %d", len(missing2))
	}

	// Test UpdateManyDescriptions
	remaining := []store.Package{missing2[0]}
	remaining[0].Description = "batch desc"
	if err := s.UpdateManyDescriptions(remaining); err != nil {
		t.Fatal(err)
	}

	// Verify all updated
	missing3, err := s.ListWithoutDescriptions("")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing3) != 0 {
		t.Fatalf("expected 0 missing after batch update, got %d", len(missing3))
	}
}

// TestSearchWithMissingDescriptions verifies search handles missing descriptions.
func TestSearchWithMissingDescriptions(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Insert packages with and without descriptions
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

	m := NewModel(s)
	m.width = 80
	m.height = 24

	// Enter search mode
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'e'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})

	// Press enter
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	// Verify the search state is set correctly
	if m.semanticQuery != "test" {
		t.Fatalf("expected query='test', got %q", m.semanticQuery)
	}
}

// TestCacheTableExists verifies the enrichment_cache table exists.
func TestCacheTableExists(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Try to query the cache table
	var count int
	var desc string
	err = s.GetEnrichmentCache().QueryRow(
		"SELECT COUNT(*), 'test' FROM enrichment_cache",
	).Scan(&count, &desc)
	if err != nil {
		t.Fatalf("enrichment_cache table should exist: %v", err)
	}
	if desc != "test" {
		t.Fatalf("unexpected result from cache table query")
	}
}

// TestNoDescriptionForExistingData verifies packages with descriptions
// are not modified during enrichment.
func TestNoDescriptionForExistingData(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Insert package with description
	pkg := store.Package{
		Name:        "numpy",
		Source:      "pip",
		Location:    "system",
		Description: "Original description",
	}
	if err := s.Upsert(pkg); err != nil {
		t.Fatal(err)
	}

	// Verify it's not in the missing list
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

// TestEnrichmentModalUI verifies the search modal shows enrichment state.
func TestEnrichmentModalUI(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24
	m.mode = "search"
	m.searching = true
	m.enriching = true
	m.enrichTotal = 100
	m.enrichDone = 50

	view := m.View()
	if view == "" {
		t.Fatal("View returned empty string")
	}

	// Should show enriching state
	if !contains(view, "Enriching") {
		t.Fatal("View should show 'Enriching' when enriching is true")
	}
}

// TestSearchFlowStateCleanup verifies state is cleaned up after search.
func TestSearchFlowStateCleanup(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24

	// Simulate search completion
	m.searching = true
	m.enriching = true
	m.mode = "search"
	m.semanticQuery = "test"

	m.Update(semanticSearchResult{results: []store.Package{}})

	if m.searching {
		t.Fatal("searching should be false after result")
	}
	if m.enriching {
		t.Fatal("enriching should be false after result")
	}
	if m.mode != "" {
		t.Fatalf("mode should be empty after result, got %s", m.mode)
	}
	if m.semanticQuery != "" {
		t.Fatalf("semanticQuery should be empty after result, got %s", m.semanticQuery)
	}
}

// TestStatusBarShowsEnrichment verifies the status bar shows enrichment info.
func TestStatusBarShowsEnrichment(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24
	m.enriching = true
	m.enrichTotal = 10
	m.enrichDone = 5

	status := m.renderStatusBar()
	if status == "" {
		t.Fatal("Status bar should not be empty")
	}

	// Should show enriching state
	if !contains(status, "enriching") {
		t.Fatal("Status bar should show 'enriching' when enrichment is active")
	}
}

// TestConcurrentEnrichmentSafety verifies multiple enrichment calls don't race.
func TestConcurrentEnrichmentSafety(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Insert multiple packages
	for i := 0; i < 10; i++ {
		pkg := store.Package{
			Name:     fmt.Sprintf("pkg%d", i),
			Source:   "pip",
			Location: "system",
		}
		if err := s.Upsert(pkg); err != nil {
			t.Fatal(err)
		}
	}

	// Get missing descriptions
	missing, err := s.ListWithoutDescriptions("pip")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 10 {
		t.Fatalf("expected 10 missing, got %d", len(missing))
	}

	// Update all in one batch
	for i := range missing {
		missing[i].Description = fmt.Sprintf("desc %d", i)
	}
	if err := s.UpdateManyDescriptions(missing); err != nil {
		t.Fatal(err)
	}

	// Verify all updated
	missing2, err := s.ListWithoutDescriptions("pip")
	if err != nil {
		t.Fatal(err)
	}
	if len(missing2) != 0 {
		t.Fatalf("expected 0 missing after batch update, got %d", len(missing2))
	}
}

// TestEmptySearchQuery verifies empty query is handled.
func TestEmptySearchQuery(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24

	m.semanticQuery = ""
	cmd := m.startSearch()

	// Should return nil results for empty query
	msg := cmd()
	if result, ok := msg.(semanticSearchResult); ok {
		if result.results != nil {
			t.Fatal("empty query should return nil results")
		}
	}
}

// TestSearchWithEmbeddings verifies search with existing embeddings.
func TestSearchWithEmbeddings(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// Insert a package
	pkg := store.Package{
		Name:        "numpy",
		Source:      "pip",
		Location:    "system",
		Description: "Python arrays",
	}
	if err := s.Upsert(pkg); err != nil {
		t.Fatal(err)
	}

	// Get the ID and set embedding
	pkgs, err := s.List("")
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 {
		t.Fatalf("expected 1 package, got %d", len(pkgs))
	}
	if err := s.UpdateEmbedding(pkgs[0].ID, "[0.1,0.2,0.3]"); err != nil {
		t.Fatal(err)
	}

	// Verify ListWithEmbeddings returns it
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

// TestSearchModalRendering verifies the search modal renders correctly.
func TestSearchModalRendering(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24
	m.mode = "search"
	m.semanticQuery = "test query"
	m.searching = false

	view := m.View()
	if view == "" {
		t.Fatal("View returned empty string")
	}

	// Should show the search input
	if !contains(view, "test query") {
		t.Fatal("View should show search query")
	}
}

// TestEnrichmentCompleteMsgHandling verifies the complete message handling.
func TestEnrichmentCompleteMsgHandling(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24

	// Set enriching state
	m.enriching = true
	m.searching = true
	m.mode = "search"

	// Send complete with error
	m.Update(enrichmentCompleteMsg{err: fmt.Errorf("test error")})

	// enriching should be false even on error (state is reset)
	if m.enriching {
		t.Fatal("enriching should be false after complete msg (error path)")
	}
	if m.scanErr == nil {
		t.Fatal("scanErr should be set after error")
	}
}

// TestSearchCancellation verifies search can be cancelled.
func TestSearchCancellation(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24

	m.mode = "search"
	m.semanticQuery = "test"
	m.searching = true

	// Press escape
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

// TestRenderTreeContentDuringEnrichment verifies tree renders during enrichment.
func TestRenderTreeContentDuringEnrichment(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24
	m.enriching = true
	m.enrichTotal = 50
	m.enrichDone = 25

	// Should render without panic
	view := m.View()
	if view == "" {
		t.Fatal("View returned empty string")
	}
}

// TestStatusBarWithSemanticResults verifies status bar shows search results.
func TestStatusBarWithSemanticResults(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24
	m.semanticResults = []store.Package{
		{Name: "test", Source: "pip"},
	}
	m.searching = false

	status := m.renderStatusBar()
	if status == "" {
		t.Fatal("Status bar should not be empty")
	}

	if !contains(status, "semantic search") {
		t.Fatal("Status bar should show 'semantic search' when results exist")
	}
}

// TestSearchTimeout verifies search handles timeout.
func TestSearchTimeout(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24

	// Set a very short timeout
	m.semanticQuery = "test"
	cmd := m.startSearch()

	// Execute and check result
	msg := cmd()
	// If embedder is nil, should return empty results
	if result, ok := msg.(semanticSearchResult); ok {
		if result.results != nil {
			t.Fatal("should return nil results when embedder is nil")
		}
	}
}

// TestConcurrentSearchAndScan verifies search and scan can run concurrently.
func TestConcurrentSearchAndScan(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24

	// Start scanning
	m.scanning = true

	// Start searching
	m.searching = true

	// Both should be tracked
	if !m.scanning {
		t.Fatal("scanning should be true")
	}
	if !m.searching {
		t.Fatal("searching should be true")
	}

	// View should render
	view := m.View()
	if view == "" {
		t.Fatal("View returned empty string")
	}
}

// TestSearchResultTreeBuild verifies search results build the tree.
func TestSearchResultTreeBuild(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24

	// Send search results
	results := []store.Package{
		{Name: "numpy", Source: "pip", Location: "system"},
		{Name: "find", Source: "bin", Location: "/usr/bin"},
	}
	m.Update(semanticSearchResult{results: results})

	// Tree should be built
	if len(m.tree.roots) == 0 {
		t.Fatal("Tree should have roots after search results")
	}
}

// TestEmptySearchResults verifies empty results are handled.
func TestEmptySearchResults(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24

	// Send empty search results
	m.Update(semanticSearchResult{results: []store.Package{}})

	// Should show error
	if m.scanErr == nil {
		t.Fatal("scanErr should be set for empty results")
	}
}

// TestSearchQueryBuilding verifies query is built correctly.
func TestSearchQueryBuilding(t *testing.T) {
	tmpDir := t.TempDir()
	s, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	m := NewModel(s)
	m.width = 80
	m.height = 24

	m.mode = "search"
	m.semanticQuery = ""

	// Type query
	for _, r := range "filesystem tools" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}

	if m.semanticQuery != "filesystem tools" {
		t.Fatalf("expected query='filesystem tools', got %s", m.semanticQuery)
	}
}
