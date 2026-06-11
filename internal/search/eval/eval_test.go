package eval

import (
	"testing"

	"installr/internal/store"
)

func TestReciprocalRank(t *testing.T) {
	ranked := []string{"a", "b", "c", "d"}
	if rr := ReciprocalRank([]string{"c"}, ranked); rr != 1.0/3.0 {
		t.Fatalf("expected 1/3, got %v", rr)
	}
	if rr := ReciprocalRank([]string{"a"}, ranked); rr != 1.0 {
		t.Fatalf("expected 1.0, got %v", rr)
	}
	// First of multiple expected wins.
	if rr := ReciprocalRank([]string{"d", "b"}, ranked); rr != 0.5 {
		t.Fatalf("expected 1/2 (b at rank 2), got %v", rr)
	}
	if rr := ReciprocalRank([]string{"zzz"}, ranked); rr != 0 {
		t.Fatalf("expected 0 for no match, got %v", rr)
	}
}

func TestHitAtK(t *testing.T) {
	ranked := []string{"a", "b", "c"}
	if HitAtK([]string{"c"}, ranked, 1) {
		t.Fatal("c should not be a hit@1")
	}
	if !HitAtK([]string{"c"}, ranked, 3) {
		t.Fatal("c should be a hit@3")
	}
	if HitAtK([]string{"x"}, ranked, 3) {
		t.Fatal("x should not hit")
	}
}

func TestAggregate(t *testing.T) {
	results := []QueryResult{
		{RR: 1.0, Hit1: true, Hit3: true, Hit10: true},
		{RR: 0.0, Hit1: false, Hit3: false, Hit10: false},
	}
	m := Aggregate(results)
	if m.N != 2 || m.MRR != 0.5 || m.Hit1 != 0.5 {
		t.Fatalf("unexpected aggregate: %+v", m)
	}
	if e := Aggregate(nil); e.N != 0 || e.MRR != 0 {
		t.Fatalf("empty aggregate should be zero, got %+v", e)
	}
}

func TestSyntheticQueriesDeterministicAndLabelled(t *testing.T) {
	pkgs := []store.Package{
		{Name: "a", Description: "alpha tool"},
		{Name: "b", Description: ""}, // skipped (no description)
		{Name: "c", Description: "gamma tool"},
		{Name: "d", Description: "delta tool"},
	}
	got := SyntheticQueries(pkgs, 2)
	if len(got) != 2 {
		t.Fatalf("expected 2 synthetic queries, got %d", len(got))
	}
	for _, q := range got {
		if !q.Synthetic || len(q.Expected) != 1 {
			t.Fatalf("synthetic query malformed: %+v", q)
		}
	}
	// Deterministic for the same input.
	again := SyntheticQueries(pkgs, 2)
	if again[0].Query != got[0].Query || again[1].Query != got[1].Query {
		t.Fatal("synthetic generation is not deterministic")
	}
	// The query is the description; expected is the owning package name.
	if got[0].Query != "alpha tool" || got[0].Expected[0] != "a" {
		t.Fatalf("unexpected first synthetic query: %+v", got[0])
	}
}

func TestDiffFlagsRegressions(t *testing.T) {
	baseline := BuildReport("default", []QueryResult{
		{Query: "q1", RR: 1.0, Hit1: true, Hit3: true, Hit10: true},
		{Query: "q2", RR: 0.5, Hit3: true, Hit10: true},
	})
	current := BuildReport("default", []QueryResult{
		{Query: "q1", RR: 0.5, Hit3: true, Hit10: true}, // RR drop + lost Hit@1
		{Query: "q2", RR: 0.5, Hit3: true, Hit10: true}, // unchanged
	})
	regs := Diff(baseline, current)
	if len(regs) != 1 || regs[0].Query != "q1" {
		t.Fatalf("expected 1 regression on q1, got %+v", regs)
	}
}

func TestLoadCurated(t *testing.T) {
	qs, err := LoadCurated()
	if err != nil {
		t.Fatal(err)
	}
	if len(qs) == 0 {
		t.Fatal("expected at least one curated query in queries.json")
	}
	for _, q := range qs {
		if q.Query == "" || len(q.Expected) == 0 {
			t.Fatalf("curated query missing fields: %+v", q)
		}
	}
}
