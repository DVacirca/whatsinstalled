package tui

import (
	"reflect"
	"testing"
)

// TestBuildTabsStableOrder guards against the map-iteration bug where tabs
// re-ordered randomly on every rebuild (making tabs jump and the highlighted
// label desync from the loaded data).
func TestBuildTabsStableOrder(t *testing.T) {
	m := &model{counts: map[string]int{
		"pip": 5, "apt": 100, "bin": 3, "conda": 2, "npm": 1,
	}}

	m.buildTabs()
	first := append([]string(nil), m.availableSources...)

	// Many rebuilds must yield byte-identical order.
	for i := 0; i < 50; i++ {
		m.buildTabs()
		if !reflect.DeepEqual(first, m.availableSources) {
			t.Fatalf("tab order not stable on rebuild %d: %v vs %v", i, first, m.availableSources)
		}
	}

	// Order follows the scanner registry (apt, snap, npm, pip, conda, bin, …),
	// with "All" first and absent sources (snap here) omitted.
	want := []string{"", "apt", "npm", "pip", "conda", "bin"}
	if !reflect.DeepEqual(want, m.availableSources) {
		t.Fatalf("unexpected tab order: got %v want %v", m.availableSources, want)
	}
}

// TestBuildTabsUnknownSourceStable verifies DB-only sources (not in the
// registry) are appended deterministically rather than randomly.
func TestBuildTabsUnknownSourceStable(t *testing.T) {
	m := &model{counts: map[string]int{"apt": 1, "zeta": 1, "alpha": 1}}
	m.buildTabs()
	want := []string{"", "apt", "alpha", "zeta"} // registry (apt) then sorted extras
	if !reflect.DeepEqual(want, m.availableSources) {
		t.Fatalf("got %v want %v", m.availableSources, want)
	}
}
