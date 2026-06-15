package enrich

import (
	"testing"

	"whatsinstalled/internal/store"
)

// TestIntegrationEnrichRealPackages tests enrichment with actual system packages.
// This test requires the real database to be present.
func TestIntegrationEnrichRealPackages(t *testing.T) {
	// Skip if no real database
	db, err := store.Open("/home/dv/.whatsinstalled.db")
	if err != nil {
		t.Skipf("No real database available: %v", err)
	}
	defer db.Close()

	missing, err := db.ListWithoutDescriptions("")
	if err != nil {
		t.Fatal(err)
	}

	if len(missing) == 0 {
		t.Skip("No packages missing descriptions")
	}

	t.Logf("Found %d packages without descriptions", len(missing))

	// Test bin enrichment
	binMissing := filterBySource(missing, "bin")
	if len(binMissing) > 0 {
		le := NewLocalEnricher()
		batch := binMissing
		if len(batch) > 20 {
			batch = batch[:20]
		}
		names := make([]string, len(batch))
		for i, p := range batch {
			names[i] = p.Name
		}
		results := le.EnrichBin(names)
		t.Logf("Bin enrichment: %d/%d found", len(results), len(names))
		for name, desc := range results {
			if len(desc) > 50 {
				desc = desc[:50] + "..."
			}
			t.Logf("  %s: %s", name, desc)
		}
	}

	// Test pip enrichment
	pipMissing := filterBySource(missing, "pip")
	if len(pipMissing) > 0 {
		le := NewLocalEnricher()
		batch := pipMissing
		if len(batch) > 5 {
			batch = batch[:5]
		}
		names := make([]string, len(batch))
		for i, p := range batch {
			names[i] = p.Name
		}
		results := le.EnrichPip(names)
		t.Logf("Pip enrichment: %d/%d found", len(results), len(names))
		for name, desc := range results {
			if len(desc) > 50 {
				desc = desc[:50] + "..."
			}
			t.Logf("  %s: %s", name, desc)
		}
	}

	// Test npm enrichment
	npmMissing := filterBySource(missing, "npm")
	if len(npmMissing) > 0 {
		le := NewLocalEnricher()
		batch := npmMissing
		if len(batch) > 5 {
			batch = batch[:5]
		}
		names := make([]string, len(batch))
		for i, p := range batch {
			names[i] = p.Name
		}
		results := le.EnrichNpm(names)
		t.Logf("Npm enrichment: %d/%d found", len(results), len(names))
		for name, desc := range results {
			if len(desc) > 50 {
				desc = desc[:50] + "..."
			}
			t.Logf("  %s: %s", name, desc)
		}
	}
}

func filterBySource(pkgs []store.Package, source string) []store.Package {
	var filtered []store.Package
	for _, p := range pkgs {
		if p.Source == source {
			filtered = append(filtered, p)
		}
	}
	return filtered
}

// TestIntegrationPyPIFetch tests fetching from PyPI.
func TestIntegrationPyPIFetch(t *testing.T) {
	re := NewRemoteEnricher(nil, false)
	desc := re.fetchPyPI("numpy")
	if desc == "" {
		t.Skip("PyPI fetch returned empty (no internet?)")
	}
	t.Logf("PyPI numpy: %s", desc)
}

// TestIntegrationNpmFetch tests fetching from npm registry.
func TestIntegrationNpmFetch(t *testing.T) {
	re := NewRemoteEnricher(nil, false)
	desc := re.fetchNpm("lodash")
	if desc == "" {
		t.Skip("npm fetch returned empty (no internet?)")
	}
	t.Logf("npm lodash: %s", desc)
}
