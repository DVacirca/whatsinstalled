// Package search holds the pure ranking logic for "Ask whatsinstalled" semantic
// search. It is deliberately free of UI, DB, and network concerns so the TUI
// and the eval harness score packages with the exact same code.
package search

import (
	"sort"

	"whatsinstalled/internal/nlp"
	"whatsinstalled/internal/store"
)

// Options tunes the hybrid ranking. The query-expansion toggle lives at the
// caller (it changes what text gets embedded), so it is not part of Options.
type Options struct {
	KeywordWeight float64 // multiplier on the keyword-match boost
	Threshold     float64 // minimum combined score to keep a result
	TopK          int     // cap on returned results (0 = unlimited)
}

// DefaultOptions mirrors the original inline ranking in the TUI.
func DefaultOptions() Options {
	return Options{KeywordWeight: 1.0, Threshold: 0.05, TopK: 50}
}

// Result is one scored package with its score broken into components so the
// eval harness (and future graded metrics) can inspect why it ranked where it
// did.
type Result struct {
	Pkg      store.Package
	Score    float64 // combined = Semantic + KeywordWeight*Keyword
	Semantic float64 // cosine similarity
	Keyword  float64 // raw keyword boost
}

// fallbackTopN is how many top results to return when nothing clears Threshold,
// so the user never gets an empty list for a valid query.
const fallbackTopN = 10

// Rank scores packages (which must already carry embeddings) against a query
// embedding and returns them best-first. Pure: no DB, no UI, no network.
func Rank(queryVec []float64, query string, pkgs []store.Package, opts Options) []Result {
	all := make([]Result, 0, len(pkgs))
	for _, p := range pkgs {
		if p.Embedding == "" {
			continue
		}
		vec, err := nlp.FromJSON(p.Embedding)
		if err != nil {
			continue
		}
		sem := nlp.CosineSimilarity(queryVec, vec)
		kw := nlp.KeywordScore(query, p)
		all = append(all, Result{
			Pkg:      p,
			Semantic: sem,
			Keyword:  kw,
			Score:    sem + opts.KeywordWeight*kw,
		})
	}

	sort.SliceStable(all, func(i, j int) bool {
		return all[i].Score > all[j].Score
	})

	results := make([]Result, 0, len(all))
	for _, r := range all {
		if r.Score > opts.Threshold {
			results = append(results, r)
		}
	}

	// Nothing cleared the threshold — fall back to the top few so the caller
	// still gets the most-relevant results rather than an empty list.
	if len(results) == 0 && len(all) > 0 {
		n := fallbackTopN
		if len(all) < n {
			n = len(all)
		}
		results = all[:n]
	}

	if opts.TopK > 0 && len(results) > opts.TopK {
		results = results[:opts.TopK]
	}
	return results
}

// Names returns the ranked package names, useful for metric computation.
func Names(results []Result) []string {
	names := make([]string, len(results))
	for i, r := range results {
		names[i] = r.Pkg.Name
	}
	return names
}
