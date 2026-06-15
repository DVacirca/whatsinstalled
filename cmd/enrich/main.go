// Command enrich is a one-off helper that fills in missing package
// descriptions in the whatsinstalled database and reports what remains.
package main

import (
	"fmt"
	"os"

	"whatsinstalled/internal/enrich"
	"whatsinstalled/internal/store"
)

func main() {
	db, err := store.Open(store.DBPath())
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Get missing descriptions
	missing, err := db.ListWithoutDescriptions("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing missing: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Found %d packages missing descriptions\n", len(missing))

	if len(missing) == 0 {
		fmt.Println("All packages already have descriptions!")
		return
	}

	// Show breakdown by source
	bySource := make(map[string]int)
	for _, p := range missing {
		bySource[p.Source]++
	}
	for source, count := range bySource {
		fmt.Printf("  %s: %d\n", source, count)
	}

	// Run enrichment
	cache := enrich.NewCache(db.GetEnrichmentCache())
	e := enrich.NewEnricher(cache)

	enriched, err := e.EnrichPackages(missing, func(total, done int, source, current, desc string) {
		status := "✗"
		if desc != "" {
			status = "✓"
		}
		fmt.Printf("  [%d/%d] %s %s: %s -> %s\n", done, total, status, source, current, desc[:min(60, len(desc))])
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "Enrichment error: %v\n", err)
		os.Exit(1)
	}

	// Count how many got descriptions
	enrichedCount := 0
	for _, p := range enriched {
		if p.Description != "" {
			enrichedCount++
		}
	}
	fmt.Printf("\nEnriched %d/%d packages with descriptions\n", enrichedCount, len(missing))

	// Update database
	err = db.UpdateManyDescriptions(enriched)
	if err != nil {
		fmt.Fprintf(os.Stderr, "DB update error: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Database updated successfully")

	// Check remaining
	remaining, err := db.ListWithoutDescriptions("")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error checking remaining: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nRemaining packages without descriptions: %d\n", len(remaining))
	if len(remaining) > 0 {
		for _, p := range remaining[:min(10, len(remaining))] {
			fmt.Printf("  - %s (%s)\n", p.Name, p.Source)
		}
		if len(remaining) > 10 {
			fmt.Printf("  ... and %d more\n", len(remaining)-10)
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
