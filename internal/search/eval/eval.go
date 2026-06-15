// Package eval is the data-driven evaluation harness for "Ask whatsinstalled"
// semantic search. It is deliberately model-free and deterministic: it loads a
// labelled query set, builds synthetic known-item queries from the corpus, and
// computes retrieval metrics (MRR, Hit@k) from a ranking the caller produces.
//
// The caller (cmd/eval) owns the embedder + DB + search.Rank; this package only
// scores the resulting ranked names against the expected answers, so every
// function here is unit-testable without the embedding model.
package eval

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"whatsinstalled/internal/store"
)

// curatedJSON is the hand-curated query set. Users replace/expand the entries
// in queries.json; the committed file holds a few worked examples as a template.
//
//go:embed queries.json
var curatedJSON []byte

// Query is one labelled test: a natural-language query and the package name(s)
// that should rank at the top.
type Query struct {
	Query     string   `json:"query"`
	Expected  []string `json:"expected"`
	Synthetic bool     `json:"synthetic,omitempty"`
}

// LoadCurated returns the hand-curated query set embedded from queries.json.
func LoadCurated() ([]Query, error) {
	var qs []Query
	if err := json.Unmarshal(curatedJSON, &qs); err != nil {
		return nil, fmt.Errorf("parse curated queries: %w", err)
	}
	return qs, nil
}

// SyntheticQueries builds up to n known-item queries by sampling packages that
// have a description: the description becomes the query and the package itself
// is the single correct answer. Sampling is an even stride over the (stable)
// input order, so the set is deterministic for a given corpus.
func SyntheticQueries(pkgs []store.Package, n int) []Query {
	withDesc := make([]store.Package, 0, len(pkgs))
	for _, p := range pkgs {
		if strings.TrimSpace(p.Description) != "" {
			withDesc = append(withDesc, p)
		}
	}
	if n <= 0 || len(withDesc) == 0 {
		return nil
	}
	if n > len(withDesc) {
		n = len(withDesc)
	}
	stride := len(withDesc) / n
	if stride < 1 {
		stride = 1
	}
	out := make([]Query, 0, n)
	for i := 0; i < len(withDesc) && len(out) < n; i += stride {
		p := withDesc[i]
		out = append(out, Query{
			Query:     p.Description,
			Expected:  []string{p.Name},
			Synthetic: true,
		})
	}
	return out
}

// ReciprocalRank returns 1/rank of the first expected name in the ranked list,
// or 0 if no expected name appears.
func ReciprocalRank(expected, ranked []string) float64 {
	exp := toSet(expected)
	for i, name := range ranked {
		if exp[name] {
			return 1.0 / float64(i+1)
		}
	}
	return 0
}

// HitAtK reports whether any expected name appears in the top k of ranked.
func HitAtK(expected, ranked []string, k int) bool {
	exp := toSet(expected)
	for i := 0; i < k && i < len(ranked); i++ {
		if exp[ranked[i]] {
			return true
		}
	}
	return false
}

// QueryResult is the scored outcome for a single query.
type QueryResult struct {
	Query     string   `json:"query"`
	Synthetic bool     `json:"synthetic,omitempty"`
	Expected  []string `json:"expected"`
	RR        float64  `json:"rr"`
	Hit1      bool     `json:"hit1"`
	Hit3      bool     `json:"hit3"`
	Hit10     bool     `json:"hit10"`
	Top       []string `json:"top"` // top-10 ranked names, for inspection
}

// ScoreQuery scores one query against a best-first ranked list of package names.
func ScoreQuery(q Query, ranked []string) QueryResult {
	top := ranked
	if len(top) > 10 {
		top = top[:10]
	}
	return QueryResult{
		Query:     q.Query,
		Synthetic: q.Synthetic,
		Expected:  q.Expected,
		RR:        ReciprocalRank(q.Expected, ranked),
		Hit1:      HitAtK(q.Expected, ranked, 1),
		Hit3:      HitAtK(q.Expected, ranked, 3),
		Hit10:     HitAtK(q.Expected, ranked, 10),
		Top:       append([]string(nil), top...),
	}
}

// Metrics is the aggregate over a set of QueryResults.
type Metrics struct {
	N     int     `json:"n"`
	MRR   float64 `json:"mrr"`
	Hit1  float64 `json:"hit1"`
	Hit3  float64 `json:"hit3"`
	Hit10 float64 `json:"hit10"`
}

// Aggregate averages the per-query scores. Returns a zero Metrics for an empty
// slice.
func Aggregate(results []QueryResult) Metrics {
	m := Metrics{N: len(results)}
	if len(results) == 0 {
		return m
	}
	var rr, h1, h3, h10 float64
	for _, r := range results {
		rr += r.RR
		h1 += b2f(r.Hit1)
		h3 += b2f(r.Hit3)
		h10 += b2f(r.Hit10)
	}
	n := float64(len(results))
	m.MRR = rr / n
	m.Hit1 = h1 / n
	m.Hit3 = h3 / n
	m.Hit10 = h10 / n
	return m
}

// Report is the full output of one variant run.
type Report struct {
	Variant   string        `json:"variant"`
	Overall   Metrics       `json:"overall"`
	Curated   Metrics       `json:"curated"`
	Synthetic Metrics       `json:"synthetic"`
	Queries   []QueryResult `json:"queries"`
}

// BuildReport partitions the results into curated/synthetic and aggregates each.
func BuildReport(variant string, results []QueryResult) Report {
	var curated, synthetic []QueryResult
	for _, r := range results {
		if r.Synthetic {
			synthetic = append(synthetic, r)
		} else {
			curated = append(curated, r)
		}
	}
	return Report{
		Variant:   variant,
		Overall:   Aggregate(results),
		Curated:   Aggregate(curated),
		Synthetic: Aggregate(synthetic),
		Queries:   results,
	}
}

// Regression is a single query that got worse versus a baseline.
type Regression struct {
	Query  string
	BaseRR float64
	CurRR  float64
	Note   string
}

// Diff returns the queries whose reciprocal rank dropped, or that lost a Hit@k,
// in current versus baseline. Matched by query text.
func Diff(baseline, current Report) []Regression {
	const eps = 1e-9
	base := make(map[string]QueryResult, len(baseline.Queries))
	for _, q := range baseline.Queries {
		base[q.Query] = q
	}
	var regs []Regression
	for _, cur := range current.Queries {
		b, ok := base[cur.Query]
		if !ok {
			continue
		}
		var notes []string
		if cur.RR < b.RR-eps {
			notes = append(notes, "RR dropped")
		}
		if b.Hit1 && !cur.Hit1 {
			notes = append(notes, "lost Hit@1")
		}
		if b.Hit3 && !cur.Hit3 {
			notes = append(notes, "lost Hit@3")
		}
		if b.Hit10 && !cur.Hit10 {
			notes = append(notes, "lost Hit@10")
		}
		if len(notes) > 0 {
			regs = append(regs, Regression{
				Query:  cur.Query,
				BaseRR: b.RR,
				CurRR:  cur.RR,
				Note:   strings.Join(notes, ", "),
			})
		}
	}
	return regs
}

func toSet(xs []string) map[string]bool {
	s := make(map[string]bool, len(xs))
	for _, x := range xs {
		s[x] = true
	}
	return s
}

func b2f(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
