package tui

import (
	"sort"
	"time"

	"whatsinstalled/internal/nlp"
	"whatsinstalled/internal/store"
)

// model is the Bubble Tea model backing the dashboard. It owns all UI state;
// the package manager scan, description enrichment, and embedding are driven by
// the init pipeline (see fullInitWithProgress) so that search stays a fast,
// in-memory operation.
type model struct {
	store    *store.Store
	embedder *nlp.Embedder
	width    int
	height   int

	// package list / tabs
	tabIndex         int
	filter           string
	filtering        bool
	tree             *treeView
	packages         []store.Package
	counts           map[string]int
	total            int
	availableSources []string
	availableLabels  []string

	mode     string // "" | "detail" | "search" | "theme-picker" | "about"
	hideAuto bool   // hide apt packages auto-installed as dependencies

	// semantic search
	semanticQuery   string
	semanticResults []store.Package
	searching       bool      // true while a search is running
	searchVersion   int       // bumped per search; stale results are discarded
	searchMsg       string    // feedback shown in the Ask modal (errors, no-results)
	searchStartTime time.Time // when the current search began, for timeout detection

	// command palette
	cmdPaletteOpen  bool
	cmdPaletteIndex int
	cmdPaletteQuery string

	// theme picker
	themePickerIndex int

	// init / scan progress
	scanning     bool
	bgUpdating   bool                 // refreshing in the background: show the dashboard with a corner indicator instead of the splash
	initStep     string               // "scan" | "enrich" | "embed" — shown during init
	initProgress string               // current init message for the splash screen
	initLogs     []string             // recent init progress lines for the splash screen
	initCh       chan scanProgressMsg // channel polled for init progress
	totalFound   int                  // packages found so far during the scan phase
	scanSource   string               // scanner currently running
	scanCount    int                  // packages found by the current scanner
	scanErr      error

	err error
}

// dataLoadedMsg carries the result of loadData: the packages for the active
// tab plus per-source counts for the title bar.
type dataLoadedMsg struct {
	packages []store.Package
	counts   map[string]int
	total    int
}

// scanCompleteMsg signals that the init/scan pipeline finished.
type scanCompleteMsg struct{}

// scanErrorMsg reports a failure from a background command.
type scanErrorMsg struct{ err error }

// semanticSearchResult carries the outcome of a semantic search. version lets
// the handler discard results from a search the user has already superseded.
type semanticSearchResult struct {
	results []store.Package
	err     string
	version int
}

// scanProgressMsg streams progress from the init pipeline. ch carries the
// channel to keep polling; isDone marks the final message.
type scanProgressMsg struct {
	source string
	count  int
	isDone bool
	ch     chan scanProgressMsg
}

// searchTimeoutMsg fires if a search has not completed within the deadline.
type searchTimeoutMsg struct{}

// NewModel returns a dashboard model bound to the given store.
func NewModel(s *store.Store) *model {
	return &model{
		store:            s,
		tree:             newTreeView(),
		counts:           make(map[string]int),
		hideAuto:         true, // dependency packages are noise by default
		scanning:         true,
		initLogs:         []string{"Checking installed packages..."},
		availableSources: []string{""},
		availableLabels:  []string{"All"},
	}
}

// buildTabs rebuilds the tab labels from the currently loaded counts. "All" is
// always first; the remaining source tabs follow in alphabetical order. A STABLE
// order matters because m.tabIndex is positional — iterating the m.counts map
// directly randomized the order on every rebuild, desyncing the highlighted
// label from the loaded data.
func (m *model) buildTabs() {
	sources := []string{""}
	labels := []string{"All"}

	var names []string
	for src, cnt := range m.counts {
		if cnt > 0 && src != "" {
			names = append(names, src)
		}
	}
	sort.Strings(names)

	for _, src := range names {
		sources = append(sources, src)
		labels = append(labels, capitalise(src))
	}

	m.availableSources = sources
	m.availableLabels = labels
}

// capitalise returns s with its first rune uppercased.
func capitalise(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	if r[0] >= 'a' && r[0] <= 'z' {
		r[0] -= 'a' - 'A'
	}
	return string(r)
}

// visibleTabSources returns the source keys for the currently shown tabs,
// prepending a synthetic "results" tab while semantic results are displayed.
func (m *model) visibleTabSources() []string {
	if m.semanticResults != nil {
		return append([]string{"results"}, m.availableSources...)
	}
	return m.availableSources
}

// visibleTabLabels returns the labels matching visibleTabSources.
func (m *model) visibleTabLabels() []string {
	if m.semanticResults != nil {
		return append([]string{"Results"}, m.availableLabels...)
	}
	return m.availableLabels
}

// currentSource returns the source key for the selected tab ("" for All,
// "results" for the search-results tab).
func (m *model) currentSource() string {
	tabs := m.visibleTabSources()
	if m.tabIndex >= 0 && m.tabIndex < len(tabs) {
		return tabs[m.tabIndex]
	}
	return ""
}
