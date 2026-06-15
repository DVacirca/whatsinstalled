package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"whatsinstalled/internal/nlp"
)

// Init loads cached data immediately and kicks off the full init pipeline.
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
	}, spinTick())
}

// Update is the Bubble Tea event loop: it routes key presses by active mode and
// handles the messages produced by the background commands.
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

		if m.mode == "search" {
			switch msg.String() {
			case "esc":
				m.searchVersion++
				m.mode = ""
				m.semanticQuery = ""
				m.searching = false
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

		if m.mode == "about" {
			switch msg.String() {
			case "esc", "q", "ctrl+c", "enter", "a":
				m.mode = ""
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
					// Skip commands that need a package when none is selected.
					if !cmd.requiresPkg || m.tree.selectedPkg() != nil {
						if c := cmd.action(m); c != nil {
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
			if c := m.enterFilterMode(); c != nil {
				cmds = append(cmds, c)
			}
		case "D":
			m.hideAuto = !m.hideAuto
			cmds = append(cmds, m.loadData)
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
				if c := m.enterDetailMode(); c != nil {
					cmds = append(cmds, c)
				}
			}
		case "?":
			if c := m.enterSearchMode(); c != nil {
				cmds = append(cmds, c)
			}
		case ":":
			if c := m.enterCommandPalette(); c != nil {
				cmds = append(cmds, c)
			}
		case "t":
			if c := m.enterThemePicker(); c != nil {
				cmds = append(cmds, c)
			}
		case "a":
			if c := m.enterAbout(); c != nil {
				cmds = append(cmds, c)
			}
		case "r":
			if c := m.triggerRescan(); c != nil {
				cmds = append(cmds, c)
			}
		}

	case dataLoadedMsg:
		m.packages = msg.packages
		m.counts = msg.counts
		m.total = msg.total
		m.buildTabs()
		// On the Results tab, always show the last search results. This prevents
		// stale dataLoadedMsg messages from other tabs from overwriting the tree
		// after a search completes.
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
		m.enrichTotal = 0
		m.embedTotal = 0
		m.initCh = nil
		m.totalFound = 0
		cmds = append(cmds, m.loadData)

	case scanProgressMsg:
		m.scanSource = msg.source
		m.scanCount = msg.count
		var progressMsg string

		switch {
		case msg.source == "enrich":
			// Phase 2 header — the total we'll enrich (line 105 in commands.go).
			m.enrichTotal = msg.count
			m.initStep = "enrich"
			progressMsg = fmt.Sprintf("Enriching descriptions... %d packages", msg.count)
		case strings.HasPrefix(msg.source, "enrich:"):
			// Per-source enrichment progress.  Count carries the number done.
			m.initStep = "enrich"
			src := strings.TrimPrefix(msg.source, "enrich:")
			if m.enrichTotal > 0 {
				progressMsg = fmt.Sprintf("Enriching %s... %d/%d packages", src, msg.count, m.enrichTotal)
			} else {
				progressMsg = fmt.Sprintf("Enriching %s... %d done", src, msg.count)
			}
		case msg.source == "embed":
			// Both the phase-3 header (carries total) and per-package progress
			// (carries index+1) use source "embed".  Distinguish by whether we
			// already have a stored total.
			m.initStep = "embed"
			if m.embedTotal == 0 {
				m.embedTotal = msg.count
				progressMsg = fmt.Sprintf("Computing embeddings... %d packages", msg.count)
			} else {
				pct := float64(msg.count) / float64(m.embedTotal) * 100
				progressMsg = fmt.Sprintf("Computing embeddings... %d/%d (%.0f%%)", msg.count, m.embedTotal, pct)
			}
		case msg.source != "":
			m.initStep = "scan"
			m.totalFound += msg.count
			progressMsg = fmt.Sprintf("Scanning %s... %d found (%d total)", msg.source, msg.count, m.totalFound)
		case msg.count > 0:
			progressMsg = fmt.Sprintf("Using cached data... %d packages", msg.count)
		default:
			progressMsg = "Initializing..."
		}
		m.initProgress = progressMsg
		// Deduplicate: replace the last log line if it shares a prefix, else append.
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
		// The envelope message carries the channel to poll.
		if msg.ch != nil {
			m.initCh = msg.ch
			return m, pollScanProgressCmd(m.initCh)
		}
		// Keep polling while the scan runs.
		if !msg.isDone && m.initCh != nil {
			return m, pollScanProgressCmd(m.initCh)
		}
		if msg.isDone {
			m.scanning = false
			m.bgUpdating = false
			m.scanSource = ""
			m.scanCount = 0
			m.enrichTotal = 0
			m.embedTotal = 0
			m.initStep = ""
			m.initProgress = ""
			m.initCh = nil
			m.totalFound = 0
			cmds = append(cmds, m.loadData)
		}
		return m, tea.Batch(cmds...)

	case scanErrorMsg:
		m.scanning = false
		m.bgUpdating = false
		m.scanErr = msg.err
		m.searching = false
		m.mode = ""
		cmds = append(cmds, m.loadData)

	case searchTimeoutMsg:
		if m.searching && time.Since(m.searchStartTime) > 55*time.Second {
			m.searchVersion++
			m.searching = false
			m.searchMsg = "Search timed out after 60s — try again."
			cmds = append(cmds, m.loadData)
		}

	case semanticSearchResult:
		if msg.version != m.searchVersion {
			return m, tea.Batch(cmds...) // discard a superseded search
		}
		m.searching = false
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
		m.tabIndex = 0 // switch to the Results tab (first)
		cmds = append(cmds, m.loadData)

	case spinnerTickMsg:
		m.spinnerFrame++
		if m.scanning || m.searching || m.bgUpdating {
			cmds = append(cmds, spinTick())
		}
	}

	return m, tea.Batch(cmds...)
}
