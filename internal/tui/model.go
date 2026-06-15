package tui

import (
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"
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
	hideAuto bool   // hide apt+ pip packages auto-installed as dependencies

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
	enrichTotal  int                  // total packages to enrich (set by phase 2 header)
	embedTotal   int                  // total packages to embed (set by phase 3 header)
	spinnerFrame int                  // braille spinner animation frame
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

// spinnerTickMsg is a self-perpetuating tick that animates the braille spinner.
type spinnerTickMsg struct{}

// spinnerFrames are braille characters that cycle to suggest activity.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// spinnerGlyph returns the current spinner frame.
func spinnerGlyph(frame int) string {
	return spinnerFrames[frame%len(spinnerFrames)]
}

// spinTick returns a tea.Cmd that fires spinnerTickMsg after ~80ms.
func spinTick() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(t time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// NewModel returns a dashboard model bound to the given store.
func NewModel(s *store.Store) *model {
	return &model{
		store:            s,
		tree:             newTreeView(),
		counts:           make(map[string]int),
		hideAuto:         false,
		scanning:         true,
		initLogs:         []string{"Initializing..."},
		availableSources: []string{""},
		availableLabels:  []string{"All"},
	}
}

// buildTabs rebuilds the tab labels from the currently loaded counts. "All" is
// always first; the remaining source tabs follow in alphabetical order.
func (m *model) buildTabs() {
	sources := []string{""}
	labels := []string{fmt.Sprintf("All (%d)", m.total)}

	var names []string
	for src, cnt := range m.counts {
		if cnt > 0 && src != "" {
			names = append(names, src)
		}
	}
	sort.Strings(names)

	for _, src := range names {
		sources = append(sources, src)
		labels = append(labels, fmt.Sprintf("%s (%d)", capitalise(src), m.counts[src]))
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
		return append([]string{fmt.Sprintf("Results (%d)", len(m.semanticResults))}, m.availableLabels...)
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

// ── Mode-entry helpers ────────────────────────────────────────────────
// Each helper encapsulates the state transition for entering a UI mode.
// Both the key handler (update.go) and the command palette (palette.go)
// call these so the transition stays in one place.

func (m *model) enterSearchMode() tea.Cmd {
	m.mode = "search"
	m.semanticQuery = ""
	m.semanticResults = nil
	m.searchMsg = ""
	return nil
}

func (m *model) enterFilterMode() tea.Cmd {
	m.filtering = true
	m.filter = ""
	return nil
}

func (m *model) enterDetailMode() tea.Cmd {
	m.mode = "detail"
	return nil
}

func (m *model) enterThemePicker() tea.Cmd {
	m.mode = "theme-picker"
	m.themePickerIndex = currentThemeIndex()
	return nil
}

func (m *model) enterAbout() tea.Cmd {
	m.mode = "about"
	return nil
}

func (m *model) enterCommandPalette() tea.Cmd {
	m.cmdPaletteOpen = true
	m.cmdPaletteIndex = 0
	m.cmdPaletteQuery = ""
	return nil
}

func (m *model) triggerRescan() tea.Cmd {
	m.scanning = true
	m.bgUpdating = true
	m.scanSource = ""
	m.scanCount = 0
	m.enrichTotal = 0
	m.embedTotal = 0
	m.initStep = "scan"
	return func() tea.Msg { return m.fullInitWithProgress() }
}
