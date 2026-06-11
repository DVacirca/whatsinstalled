package search

import (
	"testing"

	"installr/internal/nlp"
	"installr/internal/store"
)

func pkg(name string, vec ...float64) store.Package {
	return store.Package{Name: name, Source: "apt", Embedding: nlp.ToJSON(vec)}
}

// TestRankOrdersBySimilarityAndThreshold verifies best-first ordering and that
// results below the threshold are dropped.
func TestRankOrdersBySimilarityAndThreshold(t *testing.T) {
	// query "zzz" contains no domain keyword and is no package's substring, so
	// KeywordScore is 0 and ranking is purely cosine similarity.
	q := []float64{1, 0, 0}
	pkgs := []store.Package{
		pkg("aaa", 1, 0, 0),     // cos 1.0
		pkg("bbb", 0, 1, 0),     // cos 0.0 — below threshold
		pkg("ccc", 0.9, 0.1, 0), // cos ~0.994
	}

	got := Rank(q, "zzz", pkgs, DefaultOptions())
	names := Names(got)
	if len(names) != 2 {
		t.Fatalf("expected 2 results above threshold, got %d (%v)", len(names), names)
	}
	if names[0] != "aaa" || names[1] != "ccc" {
		t.Fatalf("expected [aaa ccc], got %v", names)
	}
}

// TestRankTopKCap verifies the result cap.
func TestRankTopKCap(t *testing.T) {
	q := []float64{1, 0, 0}
	pkgs := []store.Package{
		pkg("a", 1, 0, 0),
		pkg("b", 0.9, 0.1, 0),
		pkg("c", 0.8, 0.2, 0),
	}
	opts := DefaultOptions()
	opts.TopK = 2
	got := Rank(q, "zzz", pkgs, opts)
	if len(got) != 2 {
		t.Fatalf("expected TopK=2 results, got %d", len(got))
	}
}

// TestRankFallbackWhenNothingClearsThreshold verifies that a too-high threshold
// still returns the top few rather than nothing.
func TestRankFallbackWhenNothingClearsThreshold(t *testing.T) {
	q := []float64{1, 0, 0}
	pkgs := []store.Package{
		pkg("a", 0, 1, 0), // cos 0
		pkg("b", 0, 0, 1), // cos 0
	}
	opts := DefaultOptions()
	opts.Threshold = 0.5
	got := Rank(q, "zzz", pkgs, opts)
	if len(got) != 2 {
		t.Fatalf("expected fallback to return all %d, got %d", len(pkgs), len(got))
	}
}

// TestRankKeywordWeight verifies the combined score formula and that the keyword
// boost is applied with the configured weight.
func TestRankKeywordWeight(t *testing.T) {
	q := []float64{1, 0, 0}
	// "curl" matches the package name exactly → non-zero KeywordScore.
	pkgs := []store.Package{pkg("curl", 0, 1, 0)} // cos 0, so score is keyword-only

	weighted := Rank(q, "curl", pkgs, Options{KeywordWeight: 2, Threshold: -1, TopK: 50})
	if len(weighted) != 1 {
		t.Fatalf("expected 1 result, got %d", len(weighted))
	}
	r := weighted[0]
	if r.Keyword <= 0 {
		t.Fatalf("expected positive keyword boost for exact name match, got %.3f", r.Keyword)
	}
	want := r.Semantic + 2*r.Keyword
	if diff := r.Score - want; diff > 1e-9 || diff < -1e-9 {
		t.Fatalf("score formula mismatch: got %.6f want %.6f", r.Score, want)
	}
}

// TestRankSkipsMissingEmbeddings verifies packages without embeddings are ignored.
func TestRankSkipsMissingEmbeddings(t *testing.T) {
	q := []float64{1, 0, 0}
	pkgs := []store.Package{
		{Name: "noemb", Source: "apt"}, // empty Embedding
		pkg("good", 1, 0, 0),
	}
	got := Rank(q, "zzz", pkgs, DefaultOptions())
	if len(got) != 1 || got[0].Pkg.Name != "good" {
		t.Fatalf("expected only [good], got %v", Names(got))
	}
}
