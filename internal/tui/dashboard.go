package tui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"installr/internal/enrich"
	"installr/internal/nlp"
	"installr/internal/scanner"
	"installr/internal/store"
)

var tabSources = []string{"", "apt", "snap", "npm", "pip", "conda", "bin"}
var tabLabels = []string{"All", "Apt", "Snap", "Npm", "Pip", "Conda", "Bin"}

type model struct {
	store           *store.Store
	embedder        *nlp.Embedder
	width           int
	height          int
	tabIndex        int
	filter          string
	filtering       bool
	tree            *treeView
	packages        []store.Package
	counts          map[string]int
	total           int
	mode            string // "" | "detail" | "confirm" | "install" | "search" | "enriching"
	installPkg      string
	installSource   string
	installLocation string
	semanticQuery   string
	semanticResults []store.Package
	searching       bool // true while LLM search is running
	scanning        bool
	scanErr         error
	err             error

	// enrichment state
	enriching       bool
	enrichTotal     int
	enrichDone      int
	enrichSource    string
	enrichCurrent   string
	enrichDesc      string
	enrichCh        chan enrichmentProgressMsg

	// logs for the search modal
	logs []string
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
type semanticSearchResult struct{ results []store.Package }

// enrichmentProgressMsg is sent during enrichment to update the UI.
type enrichmentProgressMsg struct {
	total   int
	done    int
	source  string
	current string
	desc    string
	log     string
	isDone  bool
	ch      chan enrichmentProgressMsg // channel to poll from
}

// enrichmentCompleteMsg is sent when enrichment finishes.
type enrichmentCompleteMsg struct {
	err error
}

// logMsg is sent to append a log line in the UI.
type logMsg struct {
	line string
}

func NewModel(s *store.Store) *model {
	m := &model{
		store:  s,
		tree:   newTreeView(),
		counts: make(map[string]int),
	}
	// Load embedder in background — it's fine if it fails, semantic search
	// just won't be available.
	if emb, err := nlp.LoadEmbedder(); err == nil {
		m.embedder = emb
	}
	return m
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.loadData, m.backgroundScan)
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
				m.mode = ""
				m.semanticQuery = ""
				m.searching = false
				m.semanticResults = nil
				cmds = append(cmds, m.loadData)
				return m, tea.Batch(cmds...)
			case "enter":
				m.searching = true
				cmds = append(cmds, m.startSearch())
				return m, tea.Batch(cmds...)
			case "backspace":
				if len(m.semanticQuery) > 0 {
					m.semanticQuery = m.semanticQuery[:len(m.semanticQuery)-1]
				}
				return m, tea.Batch(cmds...)
			case " ":
				m.semanticQuery += " "
				return m, tea.Batch(cmds...)
			default:
				if msg.Type == tea.KeyRunes {
					m.semanticQuery += msg.String()
				}
				return m, tea.Batch(cmds...)
			}
		}

		if m.mode == "detail" {
			switch msg.String() {
			case "esc", "q", "ctrl+c", "d", "enter":
				m.mode = ""
			}
			return m, tea.Batch(cmds...)
		}

		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "tab":
			m.tabIndex = (m.tabIndex + 1) % len(tabSources)
			cmds = append(cmds, m.loadData)
		case "shift+tab":
			m.tabIndex--
			if m.tabIndex < 0 {
				m.tabIndex = len(tabSources) - 1
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
			if m.tree.selectedPkg() != nil {
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
				m.installSource = tabSources[m.tabIndex]
				m.installLocation = sel.label
			} else {
				m.installSource = tabSources[m.tabIndex]
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
		case "r":
			m.scanning = true
			cmds = append(cmds, m.backgroundScan)
		}

	case dataLoadedMsg:
		m.packages = msg.packages
		m.counts = msg.counts
		m.total = msg.total
		m.tree.buildTree(msg.packages)
		m.semanticResults = nil

	case scanCompleteMsg:
		m.scanning = false
		cmds = append(cmds, m.loadData)

	case scanErrorMsg:
		m.scanning = false
		m.scanErr = msg.err
		cmds = append(cmds, m.loadData)

	case uninstallCompleteMsg:
		m.scanning = false
		if msg.err != nil {
			m.scanErr = msg.err
		}
		cmds = append(cmds, m.loadData)

	case installCompleteMsg:
		m.scanning = false
		if msg.err != nil {
			m.scanErr = msg.err
		}
		cmds = append(cmds, m.loadData)

	case semanticSearchResult:
		m.searching = false
		m.enriching = false
		m.semanticResults = msg.results
		m.tree.buildTree(msg.results)
		m.mode = ""
		m.semanticQuery = ""
		if len(msg.results) == 0 {
			m.scanErr = fmt.Errorf("no results found")
		}

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
		}
		if !msg.isDone && m.enrichCh != nil {
			return m, pollProgressCmd(m.enrichCh)
		}
		if msg.isDone {
			m.enriching = false
			m.enrichCh = nil
			// Continue to search after enrichment
			cmds = append(cmds, m.startSearch())
		}
		return m, tea.Batch(cmds...)

	case enrichmentCompleteMsg:
		m.enriching = false
		m.enrichCh = nil
		if msg.err != nil {
			m.scanErr = msg.err
		}
		// Continue to search after enrichment
		cmds = append(cmds, m.startSearch())
	}

	return m, tea.Batch(cmds...)
}

func (m *model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}

	// Fixed-height elements outside the tree panel:
	// bottom panels: 8 lines total (6 content + 2 borders)
	// status bar: 1 line
	// tree panel borders: 2 lines
	// tree panel internals: title(1) + separator(1) + header(1) + tabBar(1) = 4 lines
	// Total fixed: 8 + 1 + 2 + 4 = 15
	// treeContentH = m.height - 15
	fixedH := 15
	treeContentH := m.height - fixedH
	if treeContentH < 4 {
		treeContentH = 4
	}

	// ── Title bar (inside tree panel top) ──
	title := titleStyle.Render(" installr ")
	var countParts []string
	for _, src := range []string{"apt", "snap", "npm", "pip", "conda", "bin"} {
		countParts = append(countParts, fmt.Sprintf("%s %d", src, m.counts[src]))
	}
	counts := countStyle.Render(strings.Join(countParts, "  │  "))
	titleBar := lipgloss.JoinHorizontal(lipgloss.Center, title, "  ", counts)

	// ── Separator ──
	sepWidth := m.width - 2 // inside border
	if sepWidth < 1 {
		sepWidth = 1
	}
	sep := lipgloss.NewStyle().Foreground(border).Render(strings.Repeat("─", sepWidth))

	// ── Column header ──
	headerRow := renderTreeHeader(sepWidth)

	// ── Tree content ──
	var treeContent string
	if m.enriching {
		progressText := fmt.Sprintf("  ⟳ Enriching %d/%d packages", m.enrichDone, m.enrichTotal)
		if m.enrichCurrent != "" {
			progressText += fmt.Sprintf(" (%s: %s)", m.enrichSource, m.enrichCurrent)
		}
		treeContent = lipgloss.NewStyle().Foreground(accent).Render(progressText)
		linesNeeded := treeContentH - 1
		if linesNeeded > 0 {
			treeContent += strings.Repeat("\n"+strings.Repeat(" ", sepWidth), linesNeeded)
		}
	} else if m.searching {
		treeContent = lipgloss.NewStyle().Foreground(accent).Render("  ⟳ Searching...")
		// Pad remaining height
		linesNeeded := treeContentH - 1
		if linesNeeded > 0 {
			treeContent += strings.Repeat("\n"+strings.Repeat(" ", sepWidth), linesNeeded)
		}
	} else {
		treeContent = m.tree.render(sepWidth, treeContentH)
	}

	// ── Tab bar ──
	var tabs []string
	for i, label := range tabLabels {
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

	// ── Assemble tree panel ──
	treePanelInner := lipgloss.JoinVertical(lipgloss.Left,
		titleBar,
		sep,
		headerRow,
		treeContent,
		tabBar,
	)
	treePanel := tableBorderStyle.Width(m.width).Render(treePanelInner)

	// ── Bottom info panels ──
	colW := (m.width - 6) / 3
	if colW < 10 {
		colW = 10
	}
	bottomContentH := 6 // 6 lines of content inside 8-line bordered panel
	leftPanel := m.renderDetailPanel(colW, bottomContentH)
	centerPanel := m.renderMetaPanel(colW, bottomContentH)
	rightPanel := m.renderHelpPanel(colW, bottomContentH)

	bottomRow := lipgloss.JoinHorizontal(
		lipgloss.Top,
		leftPanel,
		centerPanel,
		rightPanel,
	)

	// ── Status bar ──
	status := m.renderStatusBar()

	// ── Assemble full layout ──
	mainContent := lipgloss.JoinVertical(
		lipgloss.Left,
		treePanel,
		bottomRow,
		status,
	)

	result := lipgloss.NewStyle().MaxHeight(m.height).Render(mainContent)

	// ── Modal overlay ──
	if m.mode == "search" {
		modalWidth := min(60, m.width-4)
		var modalContent string
		if m.enriching {
			// Build log panel
			var logLines []string
			logLines = append(logLines, modalTitleStyle.Render(" Ask installr "))
			logLines = append(logLines, "")
			progressText := fmt.Sprintf("⟳  Enriching %d/%d packages...", m.enrichDone, m.enrichTotal)
			logLines = append(logLines, lipgloss.NewStyle().Foreground(accent).Render(progressText))
			if m.enrichCurrent != "" {
				logLines = append(logLines, lipgloss.NewStyle().Foreground(fg).Render(fmt.Sprintf("  %s: %s", m.enrichSource, m.enrichCurrent)))
			}
			logLines = append(logLines, "")
			// Show recent logs
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
				modalTitleStyle.Render(" Ask installr "),
				"",
				lipgloss.NewStyle().Foreground(accent).Render("⟳  Searching..."),
				"",
				lipgloss.NewStyle().Foreground(fg).Render(m.semanticQuery),
				"",
				lipgloss.NewStyle().Foreground(fg).Render("Press Esc to cancel"),
			)
		} else {
			modalContent = lipgloss.JoinVertical(lipgloss.Left,
				modalTitleStyle.Render(" Ask installr "),
				"",
				modalInputStyle.Width(modalWidth-2).Render(m.semanticQuery+"█"),
				"",
				lipgloss.NewStyle().Foreground(fg).Render("Press Enter to search, Esc to cancel"),
			)
		}
		modal := modalBorderStyle.Width(modalWidth).Render(modalContent)
		result = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, modal)
	}

	return result
}

func (m *model) renderDetailPanel(w, h int) string {
	sel := m.tree.selectedPkg()
	if sel == nil {
		node := m.tree.selected()
		if node != nil && node.isGroup {
			content := panelTitleStyle.Render("Location") + "\n" +
				panelValueStyle.Render(truncate(node.label, w-4))
			content = clipLines(content, h)
			return panelStyle.Width(w).Height(h).Render(content)
		}
		content := panelTitleStyle.Render("Description") + "\n" + panelDimStyle.Render("No package selected")
		content = clipLines(content, h)
		return panelStyle.Width(w).Height(h).Render(content)
	}

	var content string
	if sel.Description != "" {
		content = truncate(sel.Description, w-4)
	} else {
		content = panelDimStyle.Render("No description available")
	}

	title := panelTitleStyle.Render("Description")
	if m.mode == "detail" {
		title = panelTitleStyle.Foreground(accent2).Render("▸ Description")
	}

	panelContent := fmt.Sprintf("%s\n%s", title, content)
	panelContent = clipLines(panelContent, h)
	return panelStyle.Width(w).Height(h).Render(panelContent)
}

func (m *model) renderMetaPanel(w, h int) string {
	sel := m.tree.selectedPkg()
	if sel == nil {
		node := m.tree.selected()
		if node != nil && node.isGroup {
			content := panelTitleStyle.Render("Group") + "\n" +
				fmt.Sprintf("%s %s\n%s %s",
					panelKeyStyle.Render("Location  "), panelValueStyle.Render(node.label),
					panelKeyStyle.Render("Packages  "), panelValueStyle.Render(fmt.Sprintf("%d", node.count)),
				)
			content = clipLines(content, h)
			return panelStyle.Width(w).Height(h).Render(content)
		}
		content := panelTitleStyle.Render("Metadata") + "\n" + panelDimStyle.Render("—")
		content = clipLines(content, h)
		return panelStyle.Width(w).Height(h).Render(content)
	}

	fields := []struct{ k, v string }{
		{"Name", sel.Name},
		{"Version", sel.Version},
		{"Source", sel.Source},
		{"Location", truncate(sel.Location, w-12)},
		{"User", sel.User},
		{"Size", formatSize(sel.SizeBytes)},
	}

	var lines []string
	for _, f := range fields {
		lines = append(lines, fmt.Sprintf("%s %s",
			panelKeyStyle.Render(fmt.Sprintf("%-10s", f.k)),
			panelValueStyle.Render(f.v),
		))
	}

	content := panelTitleStyle.Render("Metadata") + "\n" + strings.Join(lines, "\n")
	content = clipLines(content, h)
	return panelStyle.Width(w).Height(h).Render(content)
}

func (m *model) renderHelpPanel(w, h int) string {
	keys := []struct{ k, v string }{
		{"↑↓ / jk", "Navigate"},
		{"←→ / hl", "Expand"},
		{"Tab", "Switch source"},
		{"/", "Filter"},
		{"?", "Ask (LLM)"},
		{"d", "Details"},
		{"i", "Install"},
		{"u", "Uninstall"},
		{"r", "Rescan"},
		{"q", "Quit"},
	}

	var lines []string
	for i := 0; i < len(keys); i += 2 {
		left := fmt.Sprintf("%s %s",
			panelKeyStyle.Render(fmt.Sprintf("%-10s", keys[i].k)),
			panelValueStyle.Render(keys[i].v),
		)
		var right string
		if i+1 < len(keys) {
			right = fmt.Sprintf("%s %s",
				panelKeyStyle.Render(fmt.Sprintf("%-10s", keys[i+1].k)),
				panelValueStyle.Render(keys[i+1].v),
			)
		}
		lines = append(lines, left+"  "+right)
	}

	content := panelTitleStyle.Render("Keys") + "\n" + strings.Join(lines, "\n")
	content = clipLines(content, h)
	return panelStyle.Width(w).Height(h).Render(content)
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
		parts = append(parts, "⟳ scanning...")
	}
	if m.scanErr != nil {
		parts = append(parts, fmt.Sprintf("error: %v", m.scanErr))
		m.scanErr = nil
	}
	if m.mode == "detail" {
		parts = append(parts, "detail view — press Esc to close")
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
		parts = append(parts, "installr — package tracker")
	}

	return statusBarStyle.Width(m.width).Render(strings.Join(parts, "  │  "))
}

func (m *model) loadData() tea.Msg {
	var pkgs []store.Package
	var err error
	source := tabSources[m.tabIndex]
	if m.filter != "" {
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

func (m *model) backgroundScan() tea.Msg {
	scanners := []scanner.Scanner{
		scanner.AptScanner{},
		scanner.SnapScanner{},
		scanner.NpmScanner{},
		scanner.PipScanner{},
		scanner.CondaScanner{},
		scanner.BinScanner{},
	}
	cutoff := time.Now()
	for _, sc := range scanners {
		pkgs, err := sc.Scan()
		if err != nil {
			continue
		}
		for _, p := range pkgs {
			_ = m.store.Upsert(p)
		}
	}
	_ = m.store.PurgeStale(cutoff)
	return scanCompleteMsg{}
}

func (m *model) doUninstall() tea.Msg {
	sel := m.tree.selectedPkg()
	if sel == nil {
		return nil
	}

	var sc scanner.Scanner
	switch sel.Source {
	case "apt":
		sc = scanner.AptScanner{}
	case "snap":
		sc = scanner.SnapScanner{}
	case "npm":
		sc = scanner.NpmScanner{}
	case "pip":
		sc = scanner.PipScanner{}
	case "conda":
		sc = scanner.CondaScanner{}
	case "bin":
		sc = scanner.BinScanner{}
	}
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

	var sc scanner.Scanner
	switch m.installSource {
	case "apt":
		sc = scanner.AptScanner{}
	case "snap":
		sc = scanner.SnapScanner{}
	case "npm":
		sc = scanner.NpmScanner{}
	case "pip":
		sc = scanner.PipScanner{}
	case "conda":
		sc = scanner.CondaScanner{}
	case "bin":
		sc = scanner.BinScanner{}
	}
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

// startSearch returns a tea.Cmd that checks for missing descriptions, enriches them, then runs the search.
func (m *model) startSearch() tea.Cmd {
	query := m.semanticQuery
	embedder := m.embedder
	db := m.store

	return func() tea.Msg {
		if embedder == nil || query == "" {
			return semanticSearchResult{results: nil}
		}

		// Step 1: Check for missing descriptions
		missing, err := db.ListWithoutDescriptions("")
		if err != nil {
			return scanErrorMsg{err: fmt.Errorf("list missing descriptions: %w", err)}
		}

		// Step 2: Enrich if needed
		if len(missing) > 0 {
			// Create a channel for progress updates
			ch := make(chan enrichmentProgressMsg, 100)

			// Start enrichment in a goroutine
			go func(totalMissing int) {
				defer close(ch)

				cache := enrich.NewCache(db.GetEnrichmentCache())
				e := enrich.NewEnricher(cache)

				// Send initial progress
				ch <- enrichmentProgressMsg{
					total: totalMissing,
					log:   fmt.Sprintf("Found %d packages missing descriptions", totalMissing),
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

				if err != nil {
					ch <- enrichmentProgressMsg{
						isDone: true,
						log:    fmt.Sprintf("Enrichment error: %v", err),
					}
					return
				}

				ch <- enrichmentProgressMsg{
					done:   totalDone,
					log:    "Updating descriptions in database...",
					isDone: true,
				}

				// Update descriptions in DB
				err = db.UpdateManyDescriptions(missing)
				if err != nil {
					ch <- enrichmentProgressMsg{
						isDone: true,
						log:    fmt.Sprintf("DB update error: %v", err),
					}
					return
				}

				ch <- enrichmentProgressMsg{
					isDone: true,
					log:    "Descriptions updated. Starting search...",
				}
			}(len(missing))

			// Return a progress message to start polling
			return enrichmentProgressMsg{
				isDone: false,
				log:    "Starting enrichment...",
				ch:     ch,
			}
		}

		// Step 3: Run semantic search (no enrichment needed)
		return m.runSemanticSearch(query, embedder, db)
	}
}

// runSemanticSearch performs the actual semantic search.
func (m *model) runSemanticSearch(query string, embedder *nlp.Embedder, db *store.Store) tea.Msg {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	queryVec, err := embedder.Encode(ctx, query)
	if err != nil {
		return scanErrorMsg{err: fmt.Errorf("embed query: %w", err)}
	}

	pkgs, err := db.ListWithEmbeddings()
	if err != nil {
		return scanErrorMsg{err: fmt.Errorf("list packages: %w", err)}
	}

	// Compute embeddings for any newly enriched packages
	for i, p := range pkgs {
		if p.Embedding == "" {
			text := nlp.PackageText(p.Name, p.Source, p.Description)
			vec, err := embedder.Encode(ctx, text)
			if err != nil {
				continue
			}
			jsonStr := nlp.ToJSON(vec)
			_ = db.UpdateEmbedding(p.ID, jsonStr)
			pkgs[i].Embedding = jsonStr
		}
	}

	// Score and rank
	type scored struct {
		pkg   store.Package
		score float64
	}
	var results []scored
	for _, p := range pkgs {
		vec, err := nlp.FromJSON(p.Embedding)
		if err != nil {
			continue
		}
		score := nlp.CosineSimilarity(queryVec, vec)
		if score > 0.3 {
			results = append(results, scored{pkg: p, score: score})
		}
	}

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].score > results[j].score
	})

	// Take top 20
	maxResults := 20
	if len(results) > maxResults {
		results = results[:maxResults]
	}

	var pkgsResult []store.Package
	for _, r := range results {
		pkgsResult = append(pkgsResult, r.pkg)
	}

	return semanticSearchResult{results: pkgsResult}
}

// pollProgressCmd returns a tea.Cmd that polls the channel for progress.
func pollProgressCmd(ch chan enrichmentProgressMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			// Channel closed, enrichment is done
			return enrichmentCompleteMsg{}
		}
		return msg
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
