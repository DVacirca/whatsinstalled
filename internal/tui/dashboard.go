package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"installr/internal/enrich"
	"installr/internal/nlp"
	"installr/internal/scanner"
	"installr/internal/search"
	"installr/internal/store"
)

// commandDef describes a command available in the command palette.
type commandDef struct {
	label       string
	desc        string
	key         string
	requiresPkg bool
	action      func(m *model) tea.Cmd
}

// paletteCommands is the full list of commands shown in the command palette.
var paletteCommands = []commandDef{
	{"Details", "Show details", "d", false, func(m *model) tea.Cmd { m.mode = "detail"; return nil }},
	{"Install", "Install a package", "i", false, func(m *model) tea.Cmd {
		m.mode = "install"
		m.installPkg = ""
		if sel := m.tree.selectedPkg(); sel != nil {
			m.installSource = sel.Source
			m.installLocation = sel.Location
		} else if sel := m.tree.selected(); sel != nil && sel.isGroup {
			m.installSource = m.currentSource()
			m.installLocation = sel.label
		} else {
			m.installSource = m.currentSource()
			m.installLocation = "system"
		}
		if m.installLocation == "" {
			m.installLocation = "system"
		}
		return nil
	}},
	{"Uninstall", "Uninstall selected package", "u", true, func(m *model) tea.Cmd {
		if m.tree.selectedPkg() != nil {
			m.mode = "confirm"
		}
		return nil
	}},
	{"Filter", "Filter packages by name", "/", false, func(m *model) tea.Cmd {
		m.filtering = true
		m.filter = ""
		return nil
	}},
	{"Search", "Semantic search with LLM", "?", false, func(m *model) tea.Cmd {
		m.mode = "search"
		m.semanticQuery = ""
		m.semanticResults = nil
		m.searchMsg = ""
		return nil
	}},
	{"Rescan", "Rescan all packages", "r", false, func(m *model) tea.Cmd {
		m.scanning = true
		m.bgUpdating = true
		m.scanSource = ""
		m.scanCount = 0
		m.initStep = "scan"
		return func() tea.Msg { return m.fullInitWithProgress() }
	}},
	{"Quit", "Quit installr", "q", false, func(m *model) tea.Cmd { return tea.Quit }},
	{"Theme", "Switch color theme", "t", false, func(m *model) tea.Cmd {
		m.mode = "theme-picker"
		m.themePickerIndex = 0
		return nil
	}},
}

// filteredPaletteCommands returns commands matching the current query.
func (m *model) filteredPaletteCommands() []commandDef {
	if m.cmdPaletteQuery == "" {
		return paletteCommands
	}
	q := strings.ToLower(m.cmdPaletteQuery)
	var out []commandDef
	for _, c := range paletteCommands {
		if strings.Contains(strings.ToLower(c.label), q) || strings.Contains(strings.ToLower(c.desc), q) {
			out = append(out, c)
		}
	}
	return out
}

// visibleTabSources returns the tab sources that should be shown based on state.
func (m *model) visibleTabSources() []string {
	if m.semanticResults != nil {
		return append([]string{"results"}, m.availableSources...)
	}
	return m.availableSources
}

// visibleTabLabels returns the tab labels that should be shown based on state.
func (m *model) visibleTabLabels() []string {
	if m.semanticResults != nil {
		return append([]string{"Results"}, m.availableLabels...)
	}
	return m.availableLabels
}

// currentSource returns the source key for the currently selected tab.
func (m *model) currentSource() string {
	tabs := m.visibleTabSources()
	if m.tabIndex >= 0 && m.tabIndex < len(tabs) {
		return tabs[m.tabIndex]
	}
	return ""
}

// trace writes a diagnostic message to the debug log.
func trace(msg string) {
	f, err := os.OpenFile("/tmp/installr-trace.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		f.WriteString(time.Now().Format("15:04:05.000") + " " + msg + "\n")
		f.Close()
	}
}

func tracef(format string, args ...interface{}) {
	trace(fmt.Sprintf(format, args...))
}

type model struct {
	store            *store.Store
	embedder         *nlp.Embedder
	width            int
	height           int
	tabIndex         int
	filter           string
	filtering        bool
	tree             *treeView
	packages         []store.Package
	counts           map[string]int
	total            int
	availableSources []string
	availableLabels  []string
	mode             string // "" | "detail" | "confirm" | "install" | "search" | "enriching"
	installPkg       string
	installSource    string
	installLocation  string
	semanticQuery    string
	semanticResults  []store.Package
	searching        bool // true while LLM search is running
	searchVersion    int  // incremented each time a search starts
	scanning         bool
	bgUpdating       bool // background refresh active: cached data shown with corner indicator instead of splash

	// command palette
	cmdPaletteOpen  bool
	cmdPaletteIndex int
	cmdPaletteQuery string

	// theme picker
	themePickerIndex int
	initStep         string               // "scan", "enrich", "embed" — shown during init
	initProgress     string               // current init message for splash screen
	initCh           chan scanProgressMsg // channel for init progress polling
	totalFound       int                  // total packages found so far during init
	scanSource       string               // current scanner name being run
	scanCount        int                  // packages found by current scanner
	scanErr          error
	searchMsg        string // feedback shown in the Ask modal (errors, no-results, blocked)
	err              error

	// enrichment state
	enriching     bool
	enrichTotal   int
	enrichDone    int
	enrichSource  string
	enrichCurrent string
	enrichDesc    string
	enrichCh      chan enrichmentProgressMsg

	// logs for the search modal
	logs []string

	// initLogs stores recent init progress messages for the splash screen.
	initLogs []string

	// cancelEnrich cancels the enrichment goroutine on ESC.
	cancelEnrich context.CancelFunc

	// searchStartTime tracks when search began for timeout detection.
	searchStartTime time.Time
}

type dataLoadedMsg struct {
	packages []store.Package
	counts   map[string]int
	total    int
}

type scanCompleteMsg struct{}
type scanErrorMsg struct{ err error }
type uninstallCompleteMsg struct{ err error }
type installCompleteMsg struct{ err error }
type semanticSearchResult struct {
	results []store.Package
	err     string
	version int
}

// initCompleteMsg is sent when the full init pipeline (scan + enrich + embed) finishes.
type initCompleteMsg struct{}

// enrichmentProgressMsg is sent during enrichment to update the UI.
type enrichmentProgressMsg struct {
	total   int
	done    int
	source  string
	current string
	desc    string
	log     string
	isDone  bool
	err     string                     // non-empty when an error needs reporting
	ch      chan enrichmentProgressMsg // channel to poll from
}

// scanProgressMsg is sent during background scan to update the UI.
type scanProgressMsg struct {
	source string
	count  int
	isDone bool
	ch     chan scanProgressMsg // channel to poll from
}

// enrichmentCompleteMsg is sent when enrichment finishes.
type enrichmentCompleteMsg struct {
	err error
}

// searchTimeoutMsg is sent if search has not completed within the deadline.
type searchTimeoutMsg struct{}

// logMsg is sent to append a log line in the UI.
type logMsg struct {
	line string
}

func NewModel(s *store.Store) *model {
	m := &model{
		store:            s,
		tree:             newTreeView(),
		counts:           make(map[string]int),
		scanning:         true,
		initLogs:         []string{"Checking installed packages..."},
		availableSources: []string{""},
		availableLabels:  []string{"All"},
	}
	return m
}

// buildTabs rebuilds tab labels from the currently loaded counts.
func (m *model) buildTabs() {
	sources := []string{""}
	labels := []string{"All"}
	for src, cnt := range m.counts {
		if cnt > 0 && src != "" {
			sources = append(sources, src)
			labels = append(labels, capitalise(src))
		}
	}
	m.availableSources = sources
	m.availableLabels = labels
}

// capitalise returns the string with its first rune uppercased.
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

func (m *model) Init() tea.Cmd {
	m.scanning = true
	m.initStep = "scan"
	// Cached data present → refresh in the background: render the dashboard
	// immediately with a corner indicator instead of a blocking splash.
	if n, err := m.store.Count(); err == nil && n > 0 {
		m.bgUpdating = true
	}
	if emb, err := nlp.LoadEmbedder(); err == nil {
		m.embedder = emb
	}
	return tea.Batch(m.loadData, func() tea.Msg {
		return m.fullInitWithProgress()
	})
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

	case tea.KeyMsg:
		if m.filtering {
			switch msg.String() {
			case "esc":
				m.filtering = false
				m.filter = ""
				cmds = append(cmds, m.loadData)
			case "enter":
				m.filtering = false
				cmds = append(cmds, m.loadData)
			case "backspace":
				if len(m.filter) > 0 {
					m.filter = m.filter[:len(m.filter)-1]
					cmds = append(cmds, m.loadData)
				}
			default:
				if msg.Type == tea.KeyRunes {
					m.filter += msg.String()
					cmds = append(cmds, m.loadData)
				}
			}
			return m, tea.Batch(cmds...)
		}

		if m.mode == "confirm" {
			switch msg.String() {
			case "y", "Y", "enter":
				cmds = append(cmds, m.doUninstall)
				m.mode = ""
			case "n", "N", "esc", "q", "ctrl+c":
				m.mode = ""
			}
			return m, tea.Batch(cmds...)
		}

		if m.mode == "install" {
			switch msg.String() {
			case "esc":
				m.mode = ""
				m.installPkg = ""
			case "enter":
				if m.installPkg != "" {
					cmds = append(cmds, m.doInstall)
				}
				m.mode = ""
			case "backspace":
				if len(m.installPkg) > 0 {
					m.installPkg = m.installPkg[:len(m.installPkg)-1]
				}
			default:
				if msg.Type == tea.KeyRunes {
					m.installPkg += msg.String()
				}
			}
			return m, tea.Batch(cmds...)
		}

		if m.mode == "search" {
			switch msg.String() {
			case "esc":
				if m.cancelEnrich != nil {
					m.cancelEnrich()
					m.cancelEnrich = nil
				}
				m.searchVersion++
				m.mode = ""
				m.semanticQuery = ""
				m.searching = false
				m.enriching = false
				m.enrichCh = nil
				m.logs = nil
				m.semanticResults = nil
				cmds = append(cmds, m.loadData)
				return m, tea.Batch(cmds...)
			case "enter":
				q := strings.TrimSpace(m.semanticQuery)
				if q == "" {
					m.mode = ""
					m.semanticResults = nil
					cmds = append(cmds, m.loadData)
					return m, tea.Batch(cmds...)
				}
				if m.embedder == nil {
					// No model loaded — keep the substring matches that liveSearch
					// already populated so the feature still works.
					m.mode = ""
					if len(m.semanticResults) > 0 {
						m.tabIndex = 0
					} else {
						m.semanticResults = nil
					}
					cmds = append(cmds, m.loadData)
					return m, tea.Batch(cmds...)
				}
				// Run semantic search. Descriptions + embeddings are pre-computed
				// at init, so this is just one query embedding + in-memory scoring:
				// fast and unable to hang (no enrichment/embedding in the hot path).
				m.searchVersion++
				m.searching = true
				m.searchMsg = ""
				m.searchStartTime = time.Now()
				cmds = append(cmds, m.startSearch(), tea.Tick(60*time.Second, func(time.Time) tea.Msg {
					return searchTimeoutMsg{}
				}))
				return m, tea.Batch(cmds...)
			case "backspace":
				if len(m.semanticQuery) > 0 {
					m.semanticQuery = m.semanticQuery[:len(m.semanticQuery)-1]
				}
				m.liveSearch()
				return m, tea.Batch(cmds...)
			case " ":
				m.semanticQuery += " "
				m.liveSearch()
				return m, tea.Batch(cmds...)
			default:
				if msg.Type == tea.KeyRunes {
					m.semanticQuery += msg.String()
					m.liveSearch()
				}
				return m, tea.Batch(cmds...)
			}
		}

		if m.mode == "theme-picker" {
			switch msg.String() {
			case "esc", "q", "ctrl+c":
				m.mode = ""
			case "enter":
				if m.themePickerIndex >= 0 && m.themePickerIndex < len(AllThemes) {
					applyTheme(AllThemes[m.themePickerIndex])
					_ = SaveThemeName(AllThemes[m.themePickerIndex].Name)
				}
				m.mode = ""
			case "up", "k":
				if m.themePickerIndex > 0 {
					m.themePickerIndex--
				}
			case "down", "j":
				if m.themePickerIndex < len(AllThemes)-1 {
					m.themePickerIndex++
				}
			}
			return m, tea.Batch(cmds...)
		}

		if m.mode == "detail" {
			switch msg.String() {
			case "esc", "q", "ctrl+c", "d", "enter":
				m.mode = ""
			}
			return m, tea.Batch(cmds...)
		}

		if m.cmdPaletteOpen {
			switch msg.String() {
			case "esc", "ctrl+c":
				m.cmdPaletteOpen = false
				m.cmdPaletteQuery = ""
				m.cmdPaletteIndex = 0
			case "enter":
				matched := m.filteredPaletteCommands()
				if m.cmdPaletteIndex < len(matched) {
					cmd := matched[m.cmdPaletteIndex]
					if cmd.requiresPkg && m.tree.selectedPkg() == nil {
						// No-op if package required but none selected
					} else {
						c := cmd.action(m)
						if c != nil {
							cmds = append(cmds, c)
						}
					}
				}
				m.cmdPaletteOpen = false
				m.cmdPaletteQuery = ""
				m.cmdPaletteIndex = 0
			case "up", "k":
				if m.cmdPaletteIndex > 0 {
					m.cmdPaletteIndex--
				}
			case "down", "j":
				matched := m.filteredPaletteCommands()
				if m.cmdPaletteIndex < len(matched)-1 {
					m.cmdPaletteIndex++
				}
			case "backspace":
				if len(m.cmdPaletteQuery) > 0 {
					m.cmdPaletteQuery = m.cmdPaletteQuery[:len(m.cmdPaletteQuery)-1]
					m.cmdPaletteIndex = 0
				}
			default:
				if msg.Type == tea.KeyRunes {
					m.cmdPaletteQuery += msg.String()
					m.cmdPaletteIndex = 0
				}
			}
			return m, tea.Batch(cmds...)
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.tabIndex = (m.tabIndex + 1) % len(m.visibleTabSources())
			cmds = append(cmds, m.loadData)
		case "shift+tab":
			m.tabIndex--
			if m.tabIndex < 0 {
				m.tabIndex = len(m.visibleTabSources()) - 1
			}
			cmds = append(cmds, m.loadData)
		case "/":
			m.filtering = true
			m.filter = ""
		case "up", "k":
			m.tree.moveCursor(-1)
		case "down", "j":
			m.tree.moveCursor(1)
		case "right", "l", " ":
			if sel := m.tree.selected(); sel != nil && sel.isGroup {
				m.tree.toggle(sel.label)
			}
		case "left", "h":
			if sel := m.tree.selected(); sel != nil && sel.isGroup {
				if m.tree.isExpanded(sel.label) {
					m.tree.toggle(sel.label)
				}
			}
		case "d", "enter":
			if m.tree.selectedPkg() != nil || (m.tree.selected() != nil && m.tree.selected().isGroup) {
				m.mode = "detail"
			}
		case "u":
			if m.tree.selectedPkg() != nil {
				m.mode = "confirm"
			}
		case "i":
			m.mode = "install"
			m.installPkg = ""
			// Determine install location from selection
			if sel := m.tree.selectedPkg(); sel != nil {
				m.installSource = sel.Source
				m.installLocation = sel.Location
			} else if sel := m.tree.selected(); sel != nil && sel.isGroup {
				m.installSource = m.currentSource()
				m.installLocation = sel.label
			} else {
				m.installSource = m.currentSource()
				m.installLocation = "system"
			}
			// For npm/pip/conda, if no specific location, default to CWD
			if m.installLocation == "" {
				m.installLocation = "system"
			}
		case "?":
			m.mode = "search"
			m.semanticQuery = ""
			m.semanticResults = nil
			m.searchMsg = ""
		case ":":
			m.cmdPaletteOpen = true
			m.cmdPaletteIndex = 0
			m.cmdPaletteQuery = ""
		case "t":
			m.mode = "theme-picker"
			m.themePickerIndex = 0
		case "r":
			m.scanning = true
			m.bgUpdating = true
			m.scanSource = ""
			m.scanCount = 0
			m.initStep = "scan"
			cmds = append(cmds, func() tea.Msg {
				return m.fullInitWithProgress()
			})
		}

	case dataLoadedMsg:
		m.packages = msg.packages
		m.counts = msg.counts
		m.total = msg.total
		m.buildTabs()
		// On the Results tab, always show the last search results.
		// This prevents stale dataLoadedMsg messages from other tabs
		// from overwriting the tree after a search completes.
		if m.currentSource() == "results" {
			m.tree.buildTree(m.semanticResults)
		} else {
			m.tree.buildTree(msg.packages)
		}

	case scanCompleteMsg:
		m.scanning = false
		m.bgUpdating = false
		m.scanSource = ""
		m.scanCount = 0
		m.initCh = nil
		m.totalFound = 0
		cmds = append(cmds, m.loadData)

	case scanProgressMsg:
		m.scanSource = msg.source
		m.scanCount = msg.count
		// Build human-readable progress message for splash screen
		var progressMsg string
		if strings.HasPrefix(msg.source, "enrich:") {
			m.initStep = "enrich"
			src := strings.TrimPrefix(msg.source, "enrich:")
			progressMsg = fmt.Sprintf("Enriching %s... %d done", src, msg.count)
		} else if msg.source == "embed" {
			m.initStep = "embed"
			progressMsg = fmt.Sprintf("Computing embeddings... %d done", msg.count)
		} else if msg.source != "" {
			m.initStep = "scan"
			m.totalFound += msg.count
			progressMsg = fmt.Sprintf("Scanning %s... %d found (%d total)", msg.source, msg.count, m.totalFound)
		} else if msg.count > 0 {
			progressMsg = fmt.Sprintf("Using cached data... %d packages", msg.count)
		} else {
			progressMsg = "Checking installed packages..."
		}
		m.initProgress = progressMsg
		// Deduplicate: replace last log if same source prefix, else append.
		if len(m.initLogs) > 0 {
			last := m.initLogs[len(m.initLogs)-1]
			if strings.HasPrefix(last, "Enriching ") && strings.HasPrefix(progressMsg, "Enriching ") {
				m.initLogs[len(m.initLogs)-1] = progressMsg
			} else if strings.HasPrefix(last, "Scanning ") && strings.HasPrefix(progressMsg, "Scanning ") {
				m.initLogs[len(m.initLogs)-1] = progressMsg
			} else if strings.HasPrefix(last, "Computing ") && strings.HasPrefix(progressMsg, "Computing ") {
				m.initLogs[len(m.initLogs)-1] = progressMsg
			} else {
				m.initLogs = append(m.initLogs, progressMsg)
			}
		} else {
			m.initLogs = append(m.initLogs, progressMsg)
		}
		if len(m.initLogs) > 8 {
			m.initLogs = m.initLogs[len(m.initLogs)-8:]
		}
		// Envelope message carries the channel
		if msg.ch != nil {
			m.initCh = msg.ch
			return m, pollScanProgressCmd(m.initCh)
		}
		// Keep polling if we're still scanning and not done
		if !msg.isDone && m.initCh != nil {
			return m, pollScanProgressCmd(m.initCh)
		}
		if msg.isDone {
			m.scanning = false
			m.bgUpdating = false
			m.scanSource = ""
			m.scanCount = 0
			m.initStep = ""
			m.initProgress = ""
			m.initCh = nil
			m.totalFound = 0
			cmds = append(cmds, m.loadData)
		}
		return m, tea.Batch(cmds...)

	case scanErrorMsg:
		tracef("scanErrorMsg handler: %v", msg.err)
		m.scanning = false
		m.bgUpdating = false
		m.scanErr = msg.err
		m.searching = false
		m.enriching = false
		m.enrichCh = nil
		m.mode = ""
		if m.cancelEnrich != nil {
			m.cancelEnrich()
			m.cancelEnrich = nil
		}
		cmds = append(cmds, m.loadData)

	case searchTimeoutMsg:
		if m.searching && time.Since(m.searchStartTime) > 55*time.Second {
			trace("searchTimeoutMsg: forcing search state cleanup")
			m.searchVersion++
			m.searching = false
			m.enriching = false
			m.enrichCh = nil
			m.searchMsg = "Search timed out after 60s — try again."
			if m.cancelEnrich != nil {
				m.cancelEnrich()
				m.cancelEnrich = nil
			}
			cmds = append(cmds, m.loadData)
		}

	case uninstallCompleteMsg:
		m.scanning = false
		m.bgUpdating = false
		if msg.err != nil {
			m.scanErr = msg.err
		}
		cmds = append(cmds, m.loadData)

	case installCompleteMsg:
		m.scanning = false
		m.bgUpdating = false
		if msg.err != nil {
			m.scanErr = msg.err
		}
		cmds = append(cmds, m.loadData)

	case semanticSearchResult:
		tracef("semanticSearchResult handler: %d results, err=%s, version=%d (current=%d)", len(msg.results), msg.err, msg.version, m.searchVersion)
		if msg.version != m.searchVersion {
			trace("semanticSearchResult handler: ignoring stale result")
			return m, tea.Batch(cmds...)
		}
		m.searching = false
		m.enriching = false
		if msg.err != "" {
			m.searchMsg = "Search failed: " + msg.err
			return m, tea.Batch(cmds...)
		}
		if len(msg.results) == 0 {
			m.searchMsg = "No matches found — try rephrasing your question."
			return m, tea.Batch(cmds...)
		}
		m.semanticResults = msg.results
		m.tree.buildTree(msg.results)
		m.mode = ""
		m.semanticQuery = ""
		m.searchMsg = ""
		m.tabIndex = 0 // Switch to Results tab (first)
		cmds = append(cmds, m.loadData)

	case enrichmentProgressMsg:
		m.enrichTotal = msg.total
		m.enrichDone = msg.done
		m.enrichSource = msg.source
		m.enrichCurrent = msg.current
		m.enrichDesc = msg.desc
		if msg.log != "" {
			m.logs = append(m.logs, msg.log)
			if len(m.logs) > 20 {
				m.logs = m.logs[len(m.logs)-20:]
			}
		}
		if msg.ch != nil {
			m.enrichCh = msg.ch
			m.enriching = true
			m.searching = true
		}
		if !msg.isDone && m.enrichCh != nil {
			return m, pollProgressCmd(m.enrichCh)
		}
		if msg.isDone {
			m.enriching = false
			m.enrichCh = nil
			if msg.err != "" {
				tracef("isDone handler: error=%s", msg.err)
				m.searchMsg = "Search failed: " + msg.err
				m.searching = false
				cmds = append(cmds, m.loadData)
			} else {
				trace("isDone handler: dispatching startSearch")
				m.searching = true
				cmds = append(cmds, m.startSearch())
			}
		}
		return m, tea.Batch(cmds...)

	case enrichmentCompleteMsg:
		tracef("enrichmentCompleteMsg handler: err=%v", msg.err)
		m.enriching = false
		m.enrichCh = nil
		if m.cancelEnrich != nil {
			m.cancelEnrich()
			m.cancelEnrich = nil
		}
		if msg.err != nil {
			m.scanErr = msg.err
			m.searching = false
			m.mode = ""
			cmds = append(cmds, m.loadData)
		} else {
			// Channel closed without isDone; trigger search
			m.searching = true
			cmds = append(cmds, m.startSearch())
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	// Splash screen takes priority — show immediately during any scan.
	if m.scanning && !m.bgUpdating {
		var splashLines []string
		splashLines = append(splashLines, modalTitleStyle.Render("installr"))
		splashLines = append(splashLines, "")
		splashLines = append(splashLines, lipgloss.NewStyle().Foreground(fgBright).Bold(true).Render("⟳  Updating packages"))
		splashLines = append(splashLines, "")
		logsToShow := m.initLogs
		if len(logsToShow) == 0 {
			logsToShow = []string{"Checking installed packages..."}
		}
		if len(logsToShow) > 6 {
			logsToShow = logsToShow[len(logsToShow)-6:]
		}
		for _, logEntry := range logsToShow {
			splashLines = append(splashLines, lipgloss.NewStyle().Foreground(fg).Render(logEntry))
		}
		splashContent := lipgloss.JoinVertical(lipgloss.Left, splashLines...)
		splash := modalBorderStyle.Render(splashContent)
		if m.width > 0 && m.height > 0 {
			return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, splash)
		}
		return splash
	}

	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Fixed-height elements:
	// bottom panel: 8 lines (6 content + 2 border)
	// status bar: 1 line
	// tree internals: title(1) + separator(1) + header(1) + tabBar(1) = 4 lines
	// Total fixed: 8 + 1 + 4 = 13
	fixedH := 13
	treeContentH := m.height - fixedH
	if treeContentH < 4 {
		treeContentH = 4
	}
	sepWidth := m.width
	blankLine := strings.Repeat(" ", sepWidth)

	// ── Title bar ──
	title := shellTitleStyle.Render("installr")
	var countParts []string
	for _, src := range m.availableSources {
		if src == "" {
			continue
		}
		countParts = append(countParts, fmt.Sprintf("%s %d", src, m.counts[src]))
	}
	counts := shellCountStyle.Render(strings.Join(countParts, "  │  "))
	titleContent := lipgloss.JoinHorizontal(lipgloss.Left, title, "  ", counts)
	indicator := ""
	if m.bgUpdating {
		indicator = lipgloss.NewStyle().Foreground(accent).Bold(true).Render("⟳ updating…")
	}
	// Pad to full width so the bg is uniform across the entire line.
	pad := sepWidth - lipgloss.Width(titleContent) - lipgloss.Width(indicator)
	if pad > 0 {
		titleContent += strings.Repeat(" ", pad)
	}
	titleContent += indicator
	// Clamp so a long counts list can't overflow/wrap on narrow terminals.
	titleBar := shellStyle.MaxWidth(sepWidth).Render(titleContent)

	// ── Separator ──
	sep := separatorStyle.Render(strings.Repeat("─", sepWidth))

	// ── Column header ──
	headerRow := renderTreeHeader(sepWidth)

	// ── Tree content ──
	var treeContent string
	if m.scanning && !m.bgUpdating {
		msg := "  Loading..."
		treeContent = bodyCellStyle.Render(msg)
		for i := 0; i < treeContentH-1; i++ {
			treeContent += "\n" + bodyCellStyle.Render(blankLine)
		}
	} else if m.enriching {
		msg := fmt.Sprintf("  ⟳ Enriching %d/%d packages", m.enrichDone, m.enrichTotal)
		if m.enrichCurrent != "" {
			msg += fmt.Sprintf(" (%s: %s)", m.enrichSource, m.enrichCurrent)
		}
		treeContent = bodyCellStyle.Render(msg)
		for i := 0; i < treeContentH-1; i++ {
			treeContent += "\n" + bodyCellStyle.Render(blankLine)
		}
	} else if m.searching {
		treeContent = bodyCellStyle.Render("  ⟳ Searching...")
		for i := 0; i < treeContentH-1; i++ {
			treeContent += "\n" + bodyCellStyle.Render(blankLine)
		}
	} else {
		treeContent = m.tree.render(sepWidth, treeContentH)
	}

	// ── Tab bar ──
	var tabs []string
	for i, label := range m.visibleTabLabels() {
		if i == m.tabIndex {
			tabs = append(tabs, tabActiveStyle.Render(label))
		} else {
			tabs = append(tabs, tabInactiveStyle.Render(label))
		}
	}
	tabLine := lipgloss.JoinHorizontal(lipgloss.Left, tabs...)
	if m.filtering {
		filterText := filterStyle.Render("/" + m.filter + "█")
		tabLine = lipgloss.JoinHorizontal(lipgloss.Left, tabLine, "  ", filterText)
	} else if m.filter != "" {
		filterText := filterStyle.Render("/" + m.filter)
		tabLine = lipgloss.JoinHorizontal(lipgloss.Left, tabLine, "  ", filterText)
	}
	tabBar := tabBarStyle.Width(sepWidth).Render(tabLine)

	// ── Assemble tree area (no outer border) ──
	treePanel := lipgloss.JoinVertical(lipgloss.Left,
		titleBar,
		sep,
		headerRow,
		treeContent,
		tabBar,
	)

	// ── Bottom info area (single unified panel) ──
	// bottomPanelStyle has RoundedBorder + Padding(0,1).
	// Border = 1 char each side, padding = 1 char each side.
	// Inner content width = m.width - 4 (border+padding both sides).
	innerW := m.width - 4
	colW := (innerW - 1) / 2 // 1 vertical divider
	bottomContentH := 6
	leftContent := m.renderDetailPanel(colW, bottomContentH)
	rightContent := m.renderHelpPanel(colW, bottomContentH)
	div := bottomDividerStyle.Render("│")
	bottomRowInner := lipgloss.JoinHorizontal(lipgloss.Top,
		leftContent, div, rightContent,
	)
	bottomRow := bottomPanelStyle.Width(m.width - 2).Render(bottomRowInner)

	// ── Status bar ──
	status := m.renderStatusBar()

	// ── Assemble full layout ──
	mainContent := lipgloss.JoinVertical(lipgloss.Left,
		treePanel,
		bottomRow,
		status,
	)
	result := lipgloss.NewStyle().MaxHeight(m.height).Render(mainContent)

	// ── Theme picker overlay ──
	if m.mode == "theme-picker" {
		modalWidth := min(40, m.width-4)
		var lines []string
		lines = append(lines, modalTitleStyle.Render("Theme"))
		lines = append(lines, "")
		for i, t := range AllThemes {
			label := "  " + t.Name
			if i == m.themePickerIndex {
				label = "▸ " + t.Name
			}
			style := lipgloss.NewStyle().Foreground(fg)
			if i == m.themePickerIndex {
				style = lipgloss.NewStyle().Foreground(orange).Bold(true)
			}
			lines = append(lines, style.Render(label))
		}
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(fgDim).Render("  ↑↓ navigate │ Enter apply │ Esc close"))
		modalContent := lipgloss.JoinVertical(lipgloss.Left, lines...)
		modal := modalBorderStyle.Width(modalWidth).Render(modalContent)
		result = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	}

	// ── Command palette overlay ──
	if m.cmdPaletteOpen {
		modalWidth := min(50, m.width-4)
		var lines []string
		lines = append(lines, modalTitleStyle.Render("Command Palette"))
		lines = append(lines, "")
		lines = append(lines, modalInputStyle.Width(modalWidth-2).Render(m.cmdPaletteQuery+"█"))
		lines = append(lines, "")

		cmds := m.filteredPaletteCommands()
		for i, c := range cmds {
			label := fmt.Sprintf("  %s — %s", c.key, c.label)
			if i == m.cmdPaletteIndex {
				label = fmt.Sprintf("▸ %s — %s", c.key, c.label)
			}
			style := lipgloss.NewStyle().Foreground(fg)
			if i == m.cmdPaletteIndex {
				style = lipgloss.NewStyle().Foreground(orange).Bold(true)
			}
			lines = append(lines, style.Render(label))
		}
		if len(cmds) == 0 {
			lines = append(lines, lipgloss.NewStyle().Foreground(fgDim).Render("  No matching commands"))
		}
		lines = append(lines, "")
		lines = append(lines, lipgloss.NewStyle().Foreground(fgDim).Render("  ↑↓ navigate │ Enter run │ Esc close"))

		modalContent := lipgloss.JoinVertical(lipgloss.Left, lines...)
		modal := modalBorderStyle.Width(modalWidth).Render(modalContent)
		result = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	}

	// ── Detail view overlay ──
	if m.mode == "detail" {
		overlayW := min(64, m.width-4)
		overlayH := min(18, m.height-8)
		innerW := overlayW - 6
		if innerW < 10 {
			innerW = 10
		}
		innerH := overlayH - 4
		if innerH < 4 {
			innerH = 4
		}
		result = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center,
			modalBorderStyle.Width(overlayW).Height(overlayH).Render(m.renderDetailOverlay(innerW, innerH)))
	}

	// ── Modal overlay ──
	if m.mode == "search" {
		modalWidth := min(60, m.width-4)
		var modalContent string
		if m.enriching {
			var logLines []string
			logLines = append(logLines, modalTitleStyle.Render("Ask installr"))
			logLines = append(logLines, "")
			progressText := fmt.Sprintf("⟳  Enriching %d/%d packages...", m.enrichDone, m.enrichTotal)
			logLines = append(logLines, lipgloss.NewStyle().Foreground(fgBright).Render(progressText))
			if m.enrichCurrent != "" {
				logLines = append(logLines, lipgloss.NewStyle().Foreground(fg).Render(fmt.Sprintf("  %s: %s", m.enrichSource, m.enrichCurrent)))
			}
			logLines = append(logLines, "")
			startIdx := 0
			if len(m.logs) > 10 {
				startIdx = len(m.logs) - 10
			}
			for i := startIdx; i < len(m.logs); i++ {
				logLines = append(logLines, lipgloss.NewStyle().Foreground(fg).Render(m.logs[i]))
			}
			logLines = append(logLines, "")
			logLines = append(logLines, lipgloss.NewStyle().Foreground(fg).Render("Press Esc to cancel"))
			modalContent = lipgloss.JoinVertical(lipgloss.Left, logLines...)
		} else if m.searching {
			modalContent = lipgloss.JoinVertical(lipgloss.Left,
				modalTitleStyle.Render("Ask installr"),
				"",
				lipgloss.NewStyle().Foreground(fgBright).Render("⟳  Searching..."),
				"",
				lipgloss.NewStyle().Foreground(fg).Render(m.semanticQuery),
				"",
				lipgloss.NewStyle().Foreground(fg).Render("Press Esc to cancel"),
			)
		} else {
			inputLines := []string{
				modalTitleStyle.Render("Ask installr"),
				"",
				modalInputStyle.Width(modalWidth - 2).Render(m.semanticQuery + "█"),
				"",
			}
			q := strings.TrimSpace(m.semanticQuery)
			switch {
			case m.searchMsg != "":
				inputLines = append(inputLines, lipgloss.NewStyle().Foreground(accent).Bold(true).Width(modalWidth-2).Render(m.searchMsg))
			case q == "":
				inputLines = append(inputLines, lipgloss.NewStyle().Foreground(fgDim).Render("Ask in plain English — e.g. \"tools for editing video\""))
			case len(m.semanticResults) == 0:
				inputLines = append(inputLines, lipgloss.NewStyle().Foreground(fgDim).Render("No name matches — press Enter to search by meaning"))
			default:
				inputLines = append(inputLines, lipgloss.NewStyle().Foreground(fgDim).Render(fmt.Sprintf("%d quick match%s (Enter to search by meaning):", len(m.semanticResults), pluralES(len(m.semanticResults)))))
				const maxShown = 6
				for i, pk := range m.semanticResults {
					if i >= maxShown {
						inputLines = append(inputLines, lipgloss.NewStyle().Foreground(fgDim).Render(fmt.Sprintf("  …and %d more", len(m.semanticResults)-maxShown)))
						break
					}
					inputLines = append(inputLines, lipgloss.NewStyle().Foreground(fg).Render(truncate(fmt.Sprintf("  %s  (%s)", pk.Name, pk.Source), modalWidth-2)))
				}
			}
			inputLines = append(inputLines, "",
				lipgloss.NewStyle().Foreground(fg).Render("Enter: search · Esc: cancel"))
			modalContent = lipgloss.JoinVertical(lipgloss.Left, inputLines...)
		}
		modal := modalBorderStyle.Width(modalWidth).Render(modalContent)
		result = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	}

	return result
}

func (m *model) renderDetailPanel(w, h int) string {
	sel := m.tree.selectedPkg()
	var lines []string
	if sel == nil {
		node := m.tree.selected()
		if node != nil && node.isGroup {
			lines = append(lines, bottomTitleStyle.Render(truncate("Location", w-1)))
			lines = append(lines, sectionRuleStyle.Render(strings.Repeat("─", w-1)))
			lines = append(lines, bottomValueStyle.Render(truncate(node.label, w-2)))
		} else {
			lines = append(lines, bottomTitleStyle.Render(truncate("Description", w-1)))
			lines = append(lines, sectionRuleStyle.Render(strings.Repeat("─", w-1)))
			lines = append(lines, bottomDimStyle.Render(truncate("No package selected", w-2)))
		}
	} else {
		lines = append(lines, bottomTitleStyle.Render(truncate("Description", w-1)))
		lines = append(lines, sectionRuleStyle.Render(strings.Repeat("─", w-1)))
		if sel.Description != "" {
			lines = append(lines, truncate(sel.Description, w-2))
		} else {
			lines = append(lines, bottomDimStyle.Render(truncate("No description available", w-2)))
		}
	}
	// Pad remaining lines
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderDetailOverlay(w, h int) string {
	sel := m.tree.selectedPkg()
	var lines []string
	lines = append(lines, modalTitleStyle.Render("Package Details"))
	lines = append(lines, "")

	if sel == nil {
		node := m.tree.selected()
		if node != nil && node.isGroup {
			lines = append(lines, fmt.Sprintf("%s %s", bottomKeyStyle.Render("Location:"), bottomValueStyle.Render(node.label)))
			lines = append(lines, fmt.Sprintf("%s %s", bottomKeyStyle.Render("Packages:"), bottomValueStyle.Render(fmt.Sprintf("%d", node.count))))
		} else {
			lines = append(lines, bottomDimStyle.Render("No package selected"))
		}
	} else {
		// Description
		lines = append(lines, bottomKeyStyle.Render("Description"))
		lines = append(lines, sectionRuleStyle.Render(strings.Repeat("─", w-1)))
		if sel.Description != "" {
			lines = append(lines, truncate(sel.Description, w-2))
		} else {
			lines = append(lines, bottomDimStyle.Render("No description available"))
		}
		lines = append(lines, "")

		// Metadata
		lines = append(lines, bottomKeyStyle.Render("Metadata"))
		lines = append(lines, sectionRuleStyle.Render(strings.Repeat("─", w-1)))

		fields := []struct{ k, v string }{
			{"Name", sel.Name},
			{"Version", sel.Version},
			{"Source", sel.Source},
			{"Location", sel.Location},
			{"User", sel.User},
			{"Size", formatSize(sel.SizeBytes)},
			{"Last Used", formatLastUsed(sel.LastUsed)},
		}
		for _, f := range fields {
			key := bottomKeyStyle.Render(f.k + ":")
			keyW := lipgloss.Width(key)
			valW := w - keyW - 1
			if valW < 1 {
				valW = 1
			}
			val := bottomValueStyle.Render(truncate(f.v, valW))
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, key, " ", val))
		}
	}
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(fgDim).Render("Press Esc or d to close"))

	// Clip to height
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderMetaPanel(w, h int) string {
	sel := m.tree.selectedPkg()
	var lines []string
	lines = append(lines, bottomTitleStyle.Render(truncate("Metadata", w-1)))
	lines = append(lines, sectionRuleStyle.Render(strings.Repeat("─", w-1)))

	if sel == nil {
		node := m.tree.selected()
		if node != nil && node.isGroup {
			key := bottomKeyStyle.Render("Location:")
			keyW := lipgloss.Width(key)
			valW := w - keyW - 1
			if valW < 1 {
				valW = 1
			}
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, key, " ", bottomValueStyle.Render(truncate(node.label, valW))))
			key = bottomKeyStyle.Render("Packages:")
			keyW = lipgloss.Width(key)
			valW = w - keyW - 1
			if valW < 1 {
				valW = 1
			}
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, key, " ", bottomValueStyle.Render(fmt.Sprintf("%d", node.count))))
		} else {
			lines = append(lines, bottomDimStyle.Render("—"))
		}
	} else {
		fields := []struct{ k, v string }{
			{"Name", sel.Name},
			{"Version", sel.Version},
			{"Source", sel.Source},
			{"Location", sel.Location},
			{"User", sel.User},
			{"Size", formatSize(sel.SizeBytes)},
		}

		if w < 14 {
			// Compact format: "Key: value" with dynamic widths
			for _, f := range fields {
				key := bottomKeyStyle.Render(f.k + ":")
				keyW := lipgloss.Width(key)
				valW := w - keyW - 1
				if valW < 1 {
					valW = 1
				}
				val := bottomValueStyle.Render(truncate(f.v, valW))
				lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, key, " ", val))
			}
		} else {
			// Standard format: fixed-width key
			keyW := 10
			valW := w - keyW - 1
			if valW < 4 {
				valW = 4
			}
			for _, f := range fields {
				lines = append(lines, fmt.Sprintf("%s %s",
					bottomKeyStyle.Render(fmt.Sprintf("%-10s", f.k)),
					bottomValueStyle.Render(truncate(f.v, valW)),
				))
			}
		}
	}
	// Pad remaining lines
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderHelpPanel(w, h int) string {
	keys := []struct{ k, v string }{
		{"↑↓ / jk", "Navigate"},
		{"←→ / hl", "Expand"},
		{"Tab", "Switch source"},
		{"/", "Filter"},
		{"?", "Ask (LLM)"},
		{":", "Command"},
		{"t", "Theme"},
		{"d", "Details"},
		{"i", "Install"},
		{"u", "Uninstall"},
		{"r", "Rescan"},
		{"q", "Quit"},
	}

	lines := []string{bottomTitleStyle.Render(truncate("Keys", w-1))}
	lines = append(lines, sectionRuleStyle.Render(strings.Repeat("─", w-1)))

	if w < 20 {
		// Single column with dynamic widths
		for _, kv := range keys {
			key := bottomKeyStyle.Render(kv.k)
			keyW := lipgloss.Width(key)
			valW := w - keyW - 1
			if valW < 1 {
				valW = 1
			}
			val := bottomValueStyle.Render(truncate(kv.v, valW))
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, key, " ", val))
		}
	} else {
		// Two columns
		gap := 2
		colW := (w - gap) / 2
		keyW := 10
		valW := colW - keyW - 1
		if valW < 4 {
			valW = 4
		}
		renderPair := func(k, v string) string {
			key := bottomKeyStyle.Width(keyW).Render(k)
			val := bottomValueStyle.Width(valW).Render(truncate(v, valW))
			return lipgloss.JoinHorizontal(lipgloss.Left, key, " ", val)
		}
		for i := 0; i < len(keys); i += 2 {
			left := renderPair(keys[i].k, keys[i].v)
			var right string
			if i+1 < len(keys) {
				right = renderPair(keys[i+1].k, keys[i+1].v)
			}
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, left, strings.Repeat(" ", gap), right))
		}
	}
	// Pad remaining lines
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	return strings.Join(lines, "\n")
}

func (m *model) renderStatusBar() string {
	if m.mode == "confirm" {
		sel := m.tree.selectedPkg()
		if sel != nil {
			prompt := fmt.Sprintf(" Uninstall %s (%s) from %s? ", sel.Name, sel.Source, truncate(sel.Location, 20))
			hint := confirmKeyStyle.Render("[y]") + " yes  " + confirmKeyStyle.Render("[n]") + " cancel"
			return confirmStyle.Width(m.width).Render(
				lipgloss.JoinHorizontal(lipgloss.Center, prompt, hint),
			)
		}
	}

	if m.mode == "install" {
		prompt := fmt.Sprintf(" Install package in %s (%s): %s█", m.installLocation, m.installSource, m.installPkg)
		return confirmStyle.Width(m.width).Render(
			lipgloss.JoinHorizontal(lipgloss.Center, prompt, "  "+confirmKeyStyle.Render("[Enter]")+" confirm  "+confirmKeyStyle.Render("[Esc]")+" cancel"),
		)
	}

	var parts []string
	if m.searching {
		parts = append(parts, "⟳ searching...")
	}
	if m.scanning {
		if m.scanSource != "" {
			parts = append(parts, fmt.Sprintf("⟳ scanning %s... %d", m.scanSource, m.scanCount))
		} else {
			parts = append(parts, "⟳ scanning...")
		}
	}
	if m.scanErr != nil {
		parts = append(parts, fmt.Sprintf("error: %v", m.scanErr))
		m.scanErr = nil
	}
	if m.mode == "detail" {
		parts = append(parts, "detail view")
	}
	if m.mode == "theme-picker" {
		parts = append(parts, "theme picker")
	}
	if m.cmdPaletteOpen {
		parts = append(parts, "command palette")
	}
	if m.semanticResults != nil && !m.searching {
		parts = append(parts, fmt.Sprintf("semantic search: %d results", len(m.semanticResults)))
	}
	if sel := m.tree.selectedPkg(); sel != nil {
		parts = append(parts, fmt.Sprintf("%s (%s)", sel.Name, sel.Source))
	} else if node := m.tree.selected(); node != nil && node.isGroup {
		parts = append(parts, fmt.Sprintf("%s [%d]", node.label, node.count))
	}
	if len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("installr — %s", currentTheme.Name))
	}

	return statusBarStyle.Width(m.width).Render(strings.Join(parts, "  │  "))
}

func (m *model) loadData() tea.Msg {
	var pkgs []store.Package
	var err error
	source := m.currentSource()
	if source == "results" {
		// Results tab shows the last semantic search results.
		pkgs = m.semanticResults
	} else if m.filter != "" {
		pkgs, err = m.store.Search(m.filter, source)
	} else {
		pkgs, err = m.store.List(source)
	}
	if err != nil {
		return scanErrorMsg{err: err}
	}
	counts, total, err := m.store.CountBySource()
	if err != nil {
		return scanErrorMsg{err: err}
	}
	return dataLoadedMsg{packages: pkgs, counts: counts, total: total}
}

func (m *model) fullInitWithProgress() tea.Msg {
	ch := make(chan scanProgressMsg, 100)

	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				tracef("init goroutine PANIC: %v", r)
				ch <- scanProgressMsg{isDone: true}
			}
		}()
		embedder := m.embedder
		db := m.store

		// ── Phase 0: Check if we already have data ──
		existingCount, err := db.Count()
		if err == nil && existingCount > 0 {
			ch <- scanProgressMsg{source: "", count: existingCount}
		}

		// ── Phase 1: Scan all managers concurrently. The cost is subprocess wait
		// (pip/conda/snap dominate), so parallel scans collapse wall-clock to ~max
		// instead of the sum. Scans run in goroutines; DB writes stay sequential
		// (single-writer SQLite). Upsert is incremental; PurgeStale drops removed pkgs.
		scanners := scanner.DiscoverScanners()
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

		// ── Phase 2: Enrich descriptions ──
		if embedder != nil {
			missing, err := db.ListWithoutDescriptions("")
			if err == nil && len(missing) > 0 {
				ch <- scanProgressMsg{source: "enrich", count: len(missing)}
				cache := enrich.NewCache(db.GetEnrichmentCache())
				e := enrich.NewEnricher(cache)
				lastSource := ""
				e.EnrichPackages(missing, func(total, done int, source, current, desc string) {
					// Throttle: send message on source change or every 5 packages
					if source != lastSource || done%5 == 0 || done == total {
						ch <- scanProgressMsg{source: "enrich:" + source, count: done}
						lastSource = source
					}
				})
				_ = db.UpdateManyDescriptions(missing)
			}

			// ── Phase 3: Compute embeddings ──
			missingEmb, err := db.ListWithoutEmbeddings()
			if err == nil && len(missingEmb) > 0 {
				ch <- scanProgressMsg{source: "embed", count: len(missingEmb)}
				for i, p := range missingEmb {
					text := nlp.PackageText(p.Name, p.Source, p.Description)
					vec, err := embedder.Encode(context.Background(), text)
					if err == nil {
						_ = db.UpdateEmbedding(p.ID, nlp.ToJSON(vec))
					}
					ch <- scanProgressMsg{source: "embed", count: i + 1}
				}
			}
		}

		ch <- scanProgressMsg{isDone: true}
	}()

	return scanProgressMsg{ch: ch}
}

// pollScanProgressCmd returns a tea.Cmd that polls the scan channel.
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

func findScanner(source string) scanner.Scanner {
	for _, sc := range scanner.AllScanners {
		if sc.Name() == source {
			return sc
		}
	}
	return nil
}

func (m *model) doUninstall() tea.Msg {
	sel := m.tree.selectedPkg()
	if sel == nil {
		return nil
	}

	sc := findScanner(sel.Source)
	if sc == nil {
		return uninstallCompleteMsg{err: fmt.Errorf("unknown source: %s", sel.Source)}
	}

	cmd := sc.UninstallCmd(sel.Name, sel.Location)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return uninstallCompleteMsg{err: fmt.Errorf("uninstall failed: %w", err)}
		}
		_ = m.store.Delete(sel.Name, sel.Source, sel.Location)
		return uninstallCompleteMsg{}
	})
}

func (m *model) doInstall() tea.Msg {
	if m.installPkg == "" {
		return nil
	}

	sc := findScanner(m.installSource)
	if sc == nil {
		return installCompleteMsg{err: fmt.Errorf("unknown source: %s", m.installSource)}
	}

	cmd := sc.InstallCmd(m.installPkg, m.installLocation)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		if err != nil {
			return installCompleteMsg{err: fmt.Errorf("install failed: %w", err)}
		}
		return installCompleteMsg{}
	})
}

// liveSearch filters all packages by the current query (name or description
// substring) into m.semanticResults, which backs the Results tab.
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

func pluralES(n int) string {
	if n == 1 {
		return ""
	}
	return "es"
}

// startSearch returns a tea.Cmd that performs the semantic search.
// All data is pre-computed during init; search is instant.
func (m *model) startSearch() tea.Cmd {
	query := m.semanticQuery
	embedder := m.embedder
	db := m.store
	version := m.searchVersion

	return func() (msg tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				tracef("startSearch PANIC: %v", r)
				msg = semanticSearchResult{results: []store.Package{}, err: fmt.Sprintf("search panic: %v", r), version: version}
			}
		}()

		if embedder == nil {
			msg = semanticSearchResult{results: []store.Package{}, err: "search unavailable: embedder not loaded", version: version}
			return
		}
		if query == "" {
			msg = semanticSearchResult{results: []store.Package{}, version: version}
			return
		}
		msg = m.search(query, embedder, db, version)
		return
	}
}

// enrich runs description enrichment in a background goroutine and sends
// progress messages through the returned channel.
func (m *model) enrich(query string, embedder *nlp.Embedder, db *store.Store, missing []store.Package) tea.Msg {
	ch := make(chan enrichmentProgressMsg, 100)
	ctx, cancel := context.WithCancel(context.Background())
	m.cancelEnrich = cancel

	go func(totalMissing int) {
		defer close(ch)
		defer cancel()
		defer func() {
			if r := recover(); r != nil {
				tracef("enrich goroutine PANIC: %v", r)
				ch <- enrichmentProgressMsg{
					isDone: true,
					log:    fmt.Sprintf("Enrichment panic: %v", r),
				}
			}
		}()

		cache := enrich.NewCache(db.GetEnrichmentCache())
		e := enrich.NewEnricher(cache)

		ch <- enrichmentProgressMsg{
			total: totalMissing,
			log:   fmt.Sprintf("Found %d packages missing descriptions", totalMissing),
		}

		select {
		case <-ctx.Done():
			ch <- enrichmentProgressMsg{
				isDone: true,
				log:    "Enrichment cancelled",
			}
			return
		default:
		}

		totalDone := 0
		_, err := e.EnrichPackages(missing, func(total, done int, source, current, desc string) {
			ch <- enrichmentProgressMsg{
				total:   total,
				done:    done,
				source:  source,
				current: current,
				desc:    desc,
				log:     fmt.Sprintf("[%s] %s", source, current),
			}
			totalDone = done
		})

		select {
		case <-ctx.Done():
			ch <- enrichmentProgressMsg{
				isDone: true,
				log:    "Enrichment cancelled",
			}
			return
		default:
		}

		if err != nil {
			ch <- enrichmentProgressMsg{
				isDone: true,
				log:    fmt.Sprintf("Enrichment error: %v", err),
			}
			return
		}

		// Update descriptions in DB before signaling completion
		err = db.UpdateManyDescriptions(missing)
		if err != nil {
			ch <- enrichmentProgressMsg{
				isDone: true,
				log:    fmt.Sprintf("DB update error: %v", err),
			}
			return
		}

		tracef("enrich goroutine: sending isDone after DB update, totalDone=%d", totalDone)
		ch <- enrichmentProgressMsg{
			done:   totalDone,
			log:    "Descriptions updated. Starting search...",
			isDone: true,
		}
	}(len(missing))

	return enrichmentProgressMsg{
		isDone: false,
		log:    "Starting enrichment...",
		ch:     ch,
	}
}

// computeEmbeddings runs embedding computation in a background goroutine and
// sends progress messages. When done it automatically transitions to search.
func (m *model) computeEmbeddings(query string, embedder *nlp.Embedder, db *store.Store) tea.Msg {
	trace("computeEmbeddings: entered")
	missingEmbeddings, err := db.ListWithoutEmbeddings()
	if err != nil {
		tracef("computeEmbeddings: ListWithoutEmbeddings error: %v", err)
		return scanErrorMsg{err: fmt.Errorf("list missing embeddings: %w", err)}
	}

	tracef("computeEmbeddings: %d missing embeddings", len(missingEmbeddings))

	if len(missingEmbeddings) == 0 {
		trace("computeEmbeddings: no missing embeddings, calling search directly")
		return m.search(query, embedder, db, m.searchVersion)
	}

	trace("computeEmbeddings: spawning goroutine for embedding computation")

	ch := make(chan enrichmentProgressMsg, 100)

	go func() {
		defer close(ch)
		defer func() {
			if r := recover(); r != nil {
				tracef("computeEmbeddings goroutine PANIC: %v", r)
				ch <- enrichmentProgressMsg{
					isDone: true,
					log:    fmt.Sprintf("Embedding panic: %v", r),
					err:    fmt.Sprintf("embedding panic: %v", r),
				}
			}
		}()

		ch <- enrichmentProgressMsg{
			isDone: false,
			log:    fmt.Sprintf("Computing embeddings for %d packages...", len(missingEmbeddings)),
			total:  len(missingEmbeddings),
		}

		anySuccess := false
		for i, p := range missingEmbeddings {
			text := nlp.PackageText(p.Name, p.Source, p.Description)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			vec, err := embedder.Encode(ctx, text)
			cancel()
			if err != nil {
				ch <- enrichmentProgressMsg{
					isDone: false,
					log:    fmt.Sprintf("Error embedding %s: %v", p.Name, err),
					total:  len(missingEmbeddings),
					done:   i,
				}
				continue
			}
			anySuccess = true
			jsonStr := nlp.ToJSON(vec)
			_ = db.UpdateEmbedding(p.ID, jsonStr)

			ch <- enrichmentProgressMsg{
				isDone: false,
				log:    fmt.Sprintf("Computed embedding %d/%d", i+1, len(missingEmbeddings)),
				total:  len(missingEmbeddings),
				done:   i + 1,
			}
		}

		if !anySuccess {
			trace("computeEmbeddings goroutine: all embeddings failed")
			ch <- enrichmentProgressMsg{
				isDone: true,
				log:    "All embedding operations failed",
				err:    "all embedding operations failed - embeddings may not be cached",
			}
			return
		}

		tracef("computeEmbeddings goroutine: sending isDone, %d/%d embeddings computed", len(missingEmbeddings), len(missingEmbeddings))
		ch <- enrichmentProgressMsg{
			isDone: true,
			log:    "Embeddings computed. Starting search...",
			total:  len(missingEmbeddings),
			done:   len(missingEmbeddings),
		}
	}()

	return enrichmentProgressMsg{
		isDone: false,
		log:    "Starting embedding computation...",
		ch:     ch,
	}
}

// search performs hybrid semantic + keyword search on packages that already have embeddings.
func (m *model) search(query string, embedder *nlp.Embedder, db *store.Store, version int) tea.Msg {
	trace("search: entered")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Expand the query with domain synonyms so the embedding model sees a richer context.
	expandedQuery := nlp.ExpandQuery(query)
	tracef("search: expanded query from %q to %q", query, expandedQuery)

	queryVec, err := embedder.Encode(ctx, expandedQuery)
	if err != nil {
		tracef("search: embed query error: %v", err)
		return semanticSearchResult{results: []store.Package{}, err: fmt.Sprintf("embed query: %v", err), version: version}
	}
	tracef("search: query encoded, vector len=%d", len(queryVec))

	pkgs, err := db.ListWithEmbeddings()
	if err != nil {
		tracef("search: ListWithEmbeddings error: %v", err)
		return semanticSearchResult{results: []store.Package{}, err: fmt.Sprintf("list packages: %v", err), version: version}
	}
	tracef("search: %d packages with embeddings", len(pkgs))

	// No embeddings computed yet (fresh DB, init still running). Fall back to a
	// substring match so the user still gets results instead of an empty modal.
	if len(pkgs) == 0 {
		res, sErr := db.SearchText(query)
		if sErr != nil {
			return semanticSearchResult{results: []store.Package{}, err: sErr.Error(), version: version}
		}
		tracef("search: no embeddings, substring fallback returned %d", len(res))
		return semanticSearchResult{results: res, version: version}
	}

	// Score and rank via the shared (pure) ranker so the TUI and the eval
	// harness use identical logic.
	ranked := search.Rank(queryVec, query, pkgs, search.DefaultOptions())
	pkgsResult := make([]store.Package, 0, len(ranked))
	for _, r := range ranked {
		pkgsResult = append(pkgsResult, r.Pkg)
	}

	tracef("search: returning %d results", len(pkgsResult))
	return semanticSearchResult{results: pkgsResult, version: version}
}

// pollProgressCmd returns a tea.Cmd that polls the channel for progress.
func pollProgressCmd(ch chan enrichmentProgressMsg) tea.Cmd {
	return func() tea.Msg {
		select {
		case msg, ok := <-ch:
			if !ok {
				// Channel closed, enrichment is done
				return enrichmentCompleteMsg{}
			}
			return msg
		case <-time.After(15 * time.Second):
			return enrichmentCompleteMsg{err: fmt.Errorf("enrichment timeout - process may be stuck")}
		}
	}
}

// Run starts the Bubble Tea program.
func Run(s *store.Store) error {
	m := NewModel(s)
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("run tui: %w", err)
	}
	return nil
}
