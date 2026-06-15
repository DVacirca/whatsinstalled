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

	// "All" first, then source tabs in alphabetical order. Absent sources
	// (snap here) are omitted.
	want := []string{"", "apt", "bin", "conda", "npm", "pip"}
	if !reflect.DeepEqual(want, m.availableSources) {
		t.Fatalf("unexpected tab order: got %v want %v", m.availableSources, want)
	}
}

// TestBuildTabsUnknownSourceStable verifies all sources, including DB-only ones
// not in the registry, are ordered alphabetically and deterministically.
func TestBuildTabsUnknownSourceStable(t *testing.T) {
	m := &model{counts: map[string]int{"apt": 1, "zeta": 1, "alpha": 1}}
	m.buildTabs()
	want := []string{"", "alpha", "apt", "zeta"} // "All" then alphabetical
	if !reflect.DeepEqual(want, m.availableSources) {
		t.Fatalf("got %v want %v", m.availableSources, want)
	}
}
