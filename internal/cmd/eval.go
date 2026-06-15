package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"whatsinstalled/internal/nlp"
	"whatsinstalled/internal/search"
	"whatsinstalled/internal/search/eval"
	"whatsinstalled/internal/store"
)

var (
	evalBaseline  string
	evalVariantFl string
	evalSynthetic int
	evalOut       string
)

// evalVariant is a named ranking configuration the harness can sweep.
type evalVariant struct {
	name   string
	expand bool // apply nlp.ExpandQuery before embedding the query
	opts   search.Options
}

func evalVariants() []evalVariant {
	d := search.DefaultOptions()
	return []evalVariant{
		{"default", true, d},
		{"no-expand", false, d},
		{"semantic-only", true, search.Options{KeywordWeight: 0, Threshold: d.Threshold, TopK: d.TopK}},
		{"keyword-2x", true, search.Options{KeywordWeight: 2, Threshold: d.Threshold, TopK: d.TopK}},
		{"thr-0", true, search.Options{KeywordWeight: d.KeywordWeight, Threshold: 0, TopK: d.TopK}},
	}
}

// selectVariants resolves the --variant flag: "" → default only, "all" → every
// preset, otherwise a comma-separated list of preset names.
func selectVariants(flag string) ([]evalVariant, error) {
	all := evalVariants()
	if flag == "" {
		return all[:1], nil // default
	}
	if flag == "all" {
		return all, nil
	}
	byName := map[string]evalVariant{}
	for _, v := range all {
		byName[v.name] = v
	}
	var out []evalVariant
	for _, name := range strings.Split(flag, ",") {
		name = strings.TrimSpace(name)
		v, ok := byName[name]
		if !ok {
			return nil, fmt.Errorf("unknown variant %q (known: default, no-expand, semantic-only, keyword-2x, thr-0, all)", name)
		}
		out = append(out, v)
	}
	return out, nil
}

var evalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Evaluate semantic-search ranking quality (MRR / Hit@k)",
	Long: `Scores the "Ask whatsinstalled" ranker against a labelled query set.

Combines a hand-curated set (internal/search/eval/queries.json) with synthetic
known-item queries generated from package descriptions, runs them through the
same search.Rank used by the TUI, and reports MRR and Hit@1/3/10. Use --variant
to compare configurations and --baseline to diff against a prior report.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := store.Open(dbPath)
		if err != nil {
			return err
		}
		defer s.Close()

		pkgs, err := s.ListWithEmbeddings()
		if err != nil {
			return err
		}
		if len(pkgs) == 0 {
			return fmt.Errorf("no embedded packages in %s — run the app once so it scans and computes embeddings", dbPath)
		}

		emb, err := nlp.LoadEmbedder()
		if err != nil {
			return fmt.Errorf("load embedder: %w", err)
		}

		curated, err := eval.LoadCurated()
		if err != nil {
			return err
		}
		queries := append([]eval.Query(nil), curated...)
		queries = append(queries, eval.SyntheticQueries(pkgs, evalSynthetic)...)
		fmt.Printf("Corpus: %d embedded packages · queries: %d curated + %d synthetic\n\n",
			len(pkgs), len(curated), len(queries)-len(curated))

		variants, err := selectVariants(evalVariantFl)
		if err != nil {
			return err
		}

		var first eval.Report
		for i, v := range variants {
			report := runVariant(emb, pkgs, queries, v)
			if i == 0 {
				first = report
			}
			printReport(report)
		}

		// Always show the curated queries in detail — they're the ones you care
		// about, and there are few of them.
		printCuratedDetail(first)

		if evalOut != "" {
			if err := writeReport(evalOut, first); err != nil {
				return err
			}
			fmt.Printf("\nWrote %s (variant %q)\n", evalOut, first.Variant)
		}

		if evalBaseline != "" {
			base, err := readReport(evalBaseline)
			if err != nil {
				return fmt.Errorf("read baseline: %w", err)
			}
			printDiff(base, first)
		}
		return nil
	},
}

func runVariant(emb *nlp.Embedder, pkgs []store.Package, queries []eval.Query, v evalVariant) eval.Report {
	results := make([]eval.QueryResult, 0, len(queries))
	for _, q := range queries {
		text := q.Query
		if v.expand {
			text = nlp.ExpandQuery(text)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		vec, err := emb.Encode(ctx, text)
		cancel()
		if err != nil {
			// Treat an embed failure as a zero-score miss rather than aborting.
			results = append(results, eval.ScoreQuery(q, nil))
			continue
		}
		ranked := search.Rank(vec, q.Query, pkgs, v.opts)
		results = append(results, eval.ScoreQuery(q, search.Names(ranked)))
	}
	return eval.BuildReport(v.name, results)
}

func printReport(r eval.Report) {
	fmt.Printf("== variant: %s ==\n", r.Variant)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "set\tN\tMRR\tHit@1\tHit@3\tHit@10")
	row := func(name string, m eval.Metrics) {
		fmt.Fprintf(w, "%s\t%d\t%.3f\t%.2f\t%.2f\t%.2f\n",
			name, m.N, m.MRR, m.Hit1, m.Hit3, m.Hit10)
	}
	row("overall", r.Overall)
	row("curated", r.Curated)
	row("synthetic", r.Synthetic)
	w.Flush()
	fmt.Println()
}

func printCuratedDetail(r eval.Report) {
	var curated []eval.QueryResult
	for _, q := range r.Queries {
		if !q.Synthetic {
			curated = append(curated, q)
		}
	}
	if len(curated) == 0 {
		return
	}
	fmt.Printf("Curated queries (variant %q):\n", r.Variant)
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "query\tRR\ttop-3")
	for _, q := range curated {
		top := q.Top
		if len(top) > 3 {
			top = top[:3]
		}
		fmt.Fprintf(w, "%s\t%.3f\t%s\n", truncateStr(q.Query, 32), q.RR, strings.Join(top, ", "))
	}
	w.Flush()
	fmt.Println()
}

func printDiff(base, cur eval.Report) {
	regs := eval.Diff(base, cur)
	fmt.Printf("Baseline diff (%q → %q): ΔMRR %+.3f overall\n",
		base.Variant, cur.Variant, cur.Overall.MRR-base.Overall.MRR)
	if len(regs) == 0 {
		fmt.Println("No per-query regressions.")
		return
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "query\tbaseRR\tcurRR\tnote")
	for _, r := range regs {
		fmt.Fprintf(w, "%s\t%.3f\t%.3f\t%s\n", truncateStr(r.Query, 32), r.BaseRR, r.CurRR, r.Note)
	}
	w.Flush()
}

func writeReport(path string, r eval.Report) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func readReport(path string) (eval.Report, error) {
	var r eval.Report
	b, err := os.ReadFile(path)
	if err != nil {
		return r, err
	}
	err = json.Unmarshal(b, &r)
	return r, err
}

func truncateStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func init() {
	rootCmd.AddCommand(evalCmd)
	evalCmd.Flags().StringVar(&evalBaseline, "baseline", "", "Path to a prior eval-report.json to diff against")
	evalCmd.Flags().StringVar(&evalVariantFl, "variant", "", `Variant(s) to run: "" (default), "all", or comma list (default,no-expand,semantic-only,keyword-2x,thr-0)`)
	evalCmd.Flags().IntVar(&evalSynthetic, "synthetic", 50, "Number of synthetic known-item queries to generate (0 to disable)")
	evalCmd.Flags().StringVar(&evalOut, "out", "eval-report.json", "Write the first variant's report to this path (empty to skip)")
}
