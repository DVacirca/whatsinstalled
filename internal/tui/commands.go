package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"whatsinstalled/internal/enrich"
	"whatsinstalled/internal/nlp"
	"whatsinstalled/internal/scanner"
	"whatsinstalled/internal/search"
	"whatsinstalled/internal/store"
)

// loadData loads the packages for the active tab plus the per-source counts. It
// is a tea.Cmd (returns a tea.Msg) so it can run between renders.
func (m *model) loadData() tea.Msg {
	var pkgs []store.Package
	var err error
	source := m.currentSource()
	switch {
	case source == "results":
		pkgs = m.semanticResults // the Results tab shows the last search results
	case m.filter != "":
		pkgs, err = m.store.Search(m.filter, source, m.hideAuto)
	default:
		pkgs, err = m.store.List(source, m.hideAuto)
	}
	if err != nil {
		return scanErrorMsg{err: err}
	}
	counts, total, err := m.store.CountBySource(m.hideAuto)
	if err != nil {
		return scanErrorMsg{err: err}
	}
	return dataLoadedMsg{packages: pkgs, counts: counts, total: total}
}

// fullInitWithProgress runs the complete init pipeline — scan, enrich, embed —
// in a goroutine, streaming progress over a channel. It returns immediately with
// an envelope scanProgressMsg carrying that channel; the UI then polls it via
// pollScanProgressCmd. Pre-computing descriptions and embeddings here keeps
// search a fast, in-memory operation.
func (m *model) fullInitWithProgress() tea.Msg {
	ch := make(chan scanProgressMsg, 100)

	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				ch <- scanProgressMsg{isDone: true}
			}
		}()
		embedder := m.embedder
		db := m.store

		// ── Phase 0: report any existing cached data immediately ──
		if existingCount, err := db.Count(); err == nil && existingCount > 0 {
			ch <- scanProgressMsg{source: "", count: existingCount}
		}

		// ── Phase 1: scan all managers concurrently ──
		// The cost is subprocess wait (pip/conda/snap dominate), so parallel
		// scans collapse wall-clock to ~max instead of the sum. Scans run in
		// goroutines; DB writes stay sequential (single-writer SQLite). Upsert is
		// incremental; PurgeStale drops packages that have been removed.
		scanners := scanner.DiscoverScanners()
		ch <- scanProgressMsg{source: "scan", count: len(scanners)}
		cutoff := time.Now()
		scanned := make([]struct {
			pkgs []store.Package
			err  error
		}, len(scanners))
		var wg sync.WaitGroup
		for i, sc := range scanners {
			wg.Add(1)
			go func(i int, sc scanner.Scanner) {
				defer wg.Done()
				ch <- scanProgressMsg{source: sc.Name()}
				pkgs, err := sc.Scan()
				scanned[i].pkgs, scanned[i].err = pkgs, err
				ch <- scanProgressMsg{source: sc.Name(), count: len(pkgs)}
			}(i, sc)
		}
		wg.Wait()
		for _, r := range scanned {
			if r.err != nil {
				continue
			}
			for _, pk := range r.pkgs {
				_ = db.Upsert(pk)
			}
		}
		_ = db.PurgeStale(cutoff)

		if embedder == nil {
			ch <- scanProgressMsg{isDone: true}
			return
		}

		// ── Phase 2: enrich missing descriptions ──
		if missing, err := db.ListWithoutDescriptions(""); err == nil && len(missing) > 0 {
			ch <- scanProgressMsg{source: "enrich", count: len(missing)}
			e := enrich.NewEnricher(enrich.NewCache(db.GetEnrichmentCache()))
			lastSource := ""
			e.EnrichPackages(missing, func(total, done int, source, current, desc string) {
				// Throttle: emit on source change or every 5 packages.
				if source != lastSource || done%5 == 0 || done == total {
					ch <- scanProgressMsg{source: "enrich:" + source, count: done}
					lastSource = source
				}
			})
			_ = db.UpdateManyDescriptions(missing)
		}

		// ── Phase 3: compute missing embeddings ──
		if missingEmb, err := db.ListWithoutEmbeddings(); err == nil && len(missingEmb) > 0 {
			ch <- scanProgressMsg{source: "embed", count: len(missingEmb)}
			for i, p := range missingEmb {
				text := nlp.PackageText(p.Name, p.Source, p.Description)
				if vec, err := embedder.Encode(context.Background(), text); err == nil {
					_ = db.UpdateEmbedding(p.ID, nlp.ToJSON(vec))
				}
				ch <- scanProgressMsg{source: "embed", count: i + 1}
			}
		}

		ch <- scanProgressMsg{isDone: true}
	}()

	return scanProgressMsg{ch: ch}
}

// pollScanProgressCmd returns a tea.Cmd that reads the next scan-progress
// message, or signals completion when the channel closes.
func pollScanProgressCmd(ch chan scanProgressMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg, ok := <-ch:
			if !ok {
				return scanCompleteMsg{}
			}
			return msg
		case <-time.After(120 * time.Second):
			return scanErrorMsg{err: fmt.Errorf("scan timeout after 120s")}
		}
	}
}

// liveSearch updates m.semanticResults with a fast name/description substring
// match for the current query, backing the Results tab as the user types.
func (m *model) liveSearch() {
	q := strings.TrimSpace(m.semanticQuery)
	if q == "" {
		m.semanticResults = nil
		m.searchMsg = ""
		return
	}
	res, err := m.store.SearchText(q)
	if err != nil {
		m.semanticResults = []store.Package{}
		m.searchMsg = "Search error: " + err.Error()
		return
	}
	m.semanticResults = res
	m.searchMsg = ""
}

// startSearch returns a tea.Cmd that runs the semantic search. All data is
// pre-computed during init, so this is just a query embedding plus in-memory
// scoring — it cannot hang.
func (m *model) startSearch() tea.Cmd {
	query := m.semanticQuery
	embedder := m.embedder
	db := m.store
	version := m.searchVersion

	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				msg = semanticSearchResult{results: []store.Package{}, err: fmt.Sprintf("search panic: %v", r), version: version}
			}
		}()

		if embedder == nil {
			return semanticSearchResult{results: []store.Package{}, err: "search unavailable: embedder not loaded", version: version}
		}
		if query == "" {
			return semanticSearchResult{results: []store.Package{}, version: version}
		}
		return runSearch(query, embedder, db, version)
	}
}

// runSearch performs hybrid semantic + keyword ranking over packages that
// already have embeddings, falling back to a substring match when none do (e.g.
// a fresh DB while init is still running).
func runSearch(query string, embedder *nlp.Embedder, db *store.Store, version int) tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Expand the query with domain synonyms so the model sees richer context.
	queryVec, err := embedder.Encode(ctx, nlp.ExpandQuery(query))
	if err != nil {
		return semanticSearchResult{results: []store.Package{}, err: fmt.Sprintf("embed query: %v", err), version: version}
	}

	pkgs, err := db.ListWithEmbeddings()
	if err != nil {
		return semanticSearchResult{results: []store.Package{}, err: fmt.Sprintf("list packages: %v", err), version: version}
	}

	if len(pkgs) == 0 {
		res, sErr := db.SearchText(query)
		if sErr != nil {
			return semanticSearchResult{results: []store.Package{}, err: sErr.Error(), version: version}
		}
		return semanticSearchResult{results: res, version: version}
	}

	// Rank via the shared (pure) ranker so the TUI and the eval harness agree.
	ranked := search.Rank(queryVec, query, pkgs, search.DefaultOptions())
	out := make([]store.Package, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, r.Pkg)
	}
	return semanticSearchResult{results: out, version: version}
}

// pluralES returns the plural suffix for an "-es" word ("match"/"matches").
func pluralES(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}
