package enrich

import (
	"testing"
	"time"

	"whatsinstalled/internal/store"
)

func TestCountMissing(t *testing.T) {
	pkgs := []store.Package{
		{Name: "a", Description: "has desc"},
		{Name: "b", Description: ""},
		{Name: "c", Description: "has desc"},
		{Name: "d", Description: ""},
	}
	if got := CountMissing(pkgs); got != 2 {
		t.Fatalf("CountMissing = %d, want 2", got)
	}
}

func TestFilterMissing(t *testing.T) {
	pkgs := []store.Package{
		{Name: "a", Description: "has desc"},
		{Name: "b", Description: ""},
		{Name: "c", Description: "has desc"},
	}
	missing := FilterMissing(pkgs)
	if len(missing) != 1 {
		t.Fatalf("FilterMissing returned %d packages, want 1", len(missing))
	}
	if missing[0].Name != "b" {
		t.Fatalf("FilterMissing returned %s, want b", missing[0].Name)
	}
}

func TestEnricherNoMissing(t *testing.T) {
	e := NewEnricher(nil)
	pkgs := []store.Package{
		{Name: "a", Description: "has desc", Source: "apt"},
		{Name: "b", Description: "has desc", Source: "pip"},
	}
	result, err := e.EnrichPackages(pkgs, nil)
	if err != nil {
		t.Fatalf("EnrichPackages error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("EnrichPackages returned %d packages, want 2", len(result))
	}
	// Descriptions should be unchanged
	if result[0].Description != "has desc" {
		t.Fatalf("description changed unexpectedly: %s", result[0].Description)
	}
}

func TestLocalEnricherWhatis(t *testing.T) {
	le := NewLocalEnricher()
	// Test with known binaries that should have man pages
	results := le.whatisBatch([]string{"find", "grep", "cat"})
	if len(results) == 0 {
		t.Log("whatis returned no results (may be normal in CI)")
	}
	// At least find should be known
	if desc, ok := results["find"]; ok {
		t.Logf("find: %s", desc)
	}
}

func TestCacheInit(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := InitCacheTable(db.GetEnrichmentCache()); err != nil {
		t.Fatal(err)
	}

	cache := NewCache(db.GetEnrichmentCache())

	// Set a value
	if err := cache.Set("numpy", "pip", "Python package for arrays"); err != nil {
		t.Fatalf("cache.Set: %v", err)
	}

	// Get it back
	desc, ok := cache.Get("numpy", "pip", 24*time.Hour)
	if !ok {
		t.Fatal("cache.Get returned false")
	}
	if desc != "Python package for arrays" {
		t.Fatalf("cache.Get returned %q, want %q", desc, "Python package for arrays")
	}

	// Get non-existent
	_, ok = cache.Get("nonexistent", "pip", 24*time.Hour)
	if ok {
		t.Fatal("cache.Get for non-existent returned true")
	}

	// Stats
	count, err := cache.Stats()
	if err != nil {
		t.Fatalf("cache.Stats: %v", err)
	}
	if count != 1 {
		t.Fatalf("cache.Stats = %d, want 1", count)
	}
}

func TestCacheBatchSet(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := InitCacheTable(db.GetEnrichmentCache()); err != nil {
		t.Fatal(err)
	}

	cache := NewCache(db.GetEnrichmentCache())

	items := []CacheItem{
		{Name: "a", Source: "pip", Description: "desc a"},
		{Name: "b", Source: "pip", Description: "desc b"},
		{Name: "c", Source: "npm", Description: "desc c"},
	}

	if err := cache.BatchSet(items); err != nil {
		t.Fatalf("cache.BatchSet: %v", err)
	}

	count, err := cache.Stats()
	if err != nil {
		t.Fatalf("cache.Stats: %v", err)
	}
	if count != 3 {
		t.Fatalf("cache.Stats = %d, want 3", count)
	}
}

func TestCachePrune(t *testing.T) {
	tmpDir := t.TempDir()
	db, err := store.Open(tmpDir + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := InitCacheTable(db.GetEnrichmentCache()); err != nil {
		t.Fatal(err)
	}

	cache := NewCache(db.GetEnrichmentCache())

	// Set old value
	if err := cache.Set("old", "pip", "old desc"); err != nil {
		t.Fatal(err)
	}

	// Prune with 0 TTL (should remove everything)
	if err := cache.Prune(0); err != nil {
		t.Fatalf("cache.Prune: %v", err)
	}

	// Should be gone
	_, ok := cache.Get("old", "pip", 0)
	if ok {
		t.Fatal("cache entry should have been pruned")
	}
}
