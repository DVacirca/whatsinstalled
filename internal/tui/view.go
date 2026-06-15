package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"whatsinstalled/internal/version"
)

// View renders the full dashboard for the current state. The base layout is the
// title bar, package tree, bottom info panels, and status bar; an active mode
// (search, detail, theme picker, about, command palette) is drawn as a centered
// overlay on top.
func (m *model) View() string {
	if m.err != nil {
		return fmt.Sprintf("Error: %v\n", m.err)
	}

	// Splash screen takes priority — show immediately during a foreground scan.
	if m.scanning && !m.bgUpdating {
		var splashLines []string
		splashLines = append(splashLines, modalTitleStyle.Render("whatsinstalled"))
		splashLines = append(splashLines, "")
		splashLines = append(splashLines, lipgloss.NewStyle().Foreground(fgBright).Bold(true).Render(spinnerGlyph(m.spinnerFrame)+"  Updating packages"))
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
	title := shellTitleStyle.Render("whatsinstalled")
	var countParts []string
	for _, src := range m.availableSources {
		if src == "" {
			continue
		}
		countParts = append(countParts, fmt.Sprintf("%s %d", src, m.counts[src]))
	}
	counts := shellCountStyle.Render(strings.Join(countParts, "  │  "))
	titleContent := lipgloss.JoinHorizontal(lipgloss.Left, title, "  ", counts)
	// Right corner: version, with the background-refresh indicator to its left.
	right := lipgloss.NewStyle().Foreground(fgDim).Render(version.Version)
	if m.bgUpdating {
		indicator := lipgloss.NewStyle().Foreground(accent).Bold(true).Render(spinnerGlyph(m.spinnerFrame) + " updating…")
		right = lipgloss.JoinHorizontal(lipgloss.Left, indicator, "  ", right)
	}
	// Pad to full width so the bg is uniform and the version sits flush right.
	pad := sepWidth - lipgloss.Width(titleContent) - lipgloss.Width(right)
	if pad > 0 {
		titleContent += strings.Repeat(" ", pad)
	}
	titleContent += right
	// Clamp so a long counts list can't overflow/wrap on narrow terminals.
	titleBar := shellStyle.MaxWidth(sepWidth).Render(titleContent)

	// ── Separator ──
	sep := separatorStyle.Render(strings.Repeat("─", sepWidth))

	// ── Column header ──
	headerRow := renderTreeHeader(sepWidth)

	// ── Tree content ──
	var treeContent string
	if m.scanning && !m.bgUpdating {
		treeContent = bodyCellStyle.Render("  Loading...")
		for i := 0; i < treeContentH-1; i++ {
			treeContent += "\n" + bodyCellStyle.Render(blankLine)
		}
	} else if m.searching {
		treeContent = bodyCellStyle.Render("  " + spinnerGlyph(m.spinnerFrame) + " Searching...")
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
	// bottomPanelStyle has RoundedBorder + Padding(0,1): border and padding each
	// take 1 char per side, so the inner content width is m.width - 4.
	innerW := m.width - 4
	colW := (innerW - 1) / 2 // 1 vertical divider
	bottomContentH := 6
	leftContent := m.renderDetailPanel(colW, bottomContentH)
	rightContent := m.renderHelpPanel(colW, bottomContentH)
	div := bottomDividerStyle.Render("│")
	bottomRowInner := lipgloss.JoinHorizontal(lipgloss.Top, leftContent, div, rightContent)
	bottomRow := bottomPanelStyle.Width(m.width - 2).Render(bottomRowInner)

	// ── Status bar ──
	status := m.renderStatusBar()

	// ── Assemble full layout ──
	mainContent := lipgloss.JoinVertical(lipgloss.Left, treePanel, bottomRow, status)
	result := lipgloss.NewStyle().MaxHeight(m.height).Render(mainContent)

	// Overlays are mutually exclusive and replace the base layout when active.
	switch {
	case m.mode == "theme-picker":
		result = m.overlay(m.viewThemePicker())
	case m.mode == "about":
		modalWidth := min(62, m.width-4)
		result = m.overlay(modalBorderStyle.Width(modalWidth).Render(aboutModalContent(modalWidth)))
	case m.cmdPaletteOpen:
		result = m.overlay(m.viewCommandPalette())
	case m.mode == "detail":
		result = m.overlay(m.viewDetailModal())
	case m.mode == "search":
		result = m.overlay(m.viewSearchModal())
	}

	return result
}

// overlay centers content over the full terminal.
func (m *model) overlay(content string) string {
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

// viewThemePicker renders the theme picker modal.
func (m *model) viewThemePicker() string {
	modalWidth := min(40, m.width-4)
	var lines []string
	lines = append(lines, modalTitleStyle.Render("Theme"))
	lines = append(lines, "")
	for i, t := range AllThemes {
		label := "  " + t.Name
		if i == m.themePickerIndex {
			label = "▸ " + t.Name
		}
		if t.Name == currentTheme.Name {
			label += " ✓"
		}
		style := lipgloss.NewStyle().Foreground(fg)
		if i == m.themePickerIndex {
			style = lipgloss.NewStyle().Foreground(orange).Bold(true)
		}
		lines = append(lines, style.Render(label))
	}
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(fgDim).Render("  ↑↓ navigate │ Enter apply │ Esc close │ ✓ current"))
	return modalBorderStyle.Width(modalWidth).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// viewCommandPalette renders the command palette modal.
func (m *model) viewCommandPalette() string {
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
	return modalBorderStyle.Width(modalWidth).Render(lipgloss.JoinVertical(lipgloss.Left, lines...))
}

// viewDetailModal renders the package-details overlay.
func (m *model) viewDetailModal() string {
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
	return modalBorderStyle.Width(overlayW).Height(overlayH).Render(m.renderDetailOverlay(innerW, innerH))
}

// viewSearchModal renders the "Ask whatsinstalled" modal in either its running
// (searching) or input state.
func (m *model) viewSearchModal() string {
	modalWidth := min(60, m.width-4)
	var modalContent string
	if m.searching {
		modalContent = lipgloss.JoinVertical(lipgloss.Left,
			modalTitleStyle.Render("Ask whatsinstalled"),
			"",
			lipgloss.NewStyle().Foreground(fgBright).Render(spinnerGlyph(m.spinnerFrame)+"  Searching..."),
			"",
			lipgloss.NewStyle().Foreground(fg).Render(m.semanticQuery),
			"",
			lipgloss.NewStyle().Foreground(fg).Render("Press Esc to cancel"),
		)
		return modalBorderStyle.Width(modalWidth).Render(modalContent)
	}

	inputLines := []string{
		modalTitleStyle.Render("Ask whatsinstalled"),
		lipgloss.NewStyle().Foreground(orange).Render("⚗ experimental"),
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
	inputLines = append(inputLines, "", lipgloss.NewStyle().Foreground(fg).Render("Enter: search · Esc: cancel"))
	return modalBorderStyle.Width(modalWidth).Render(lipgloss.JoinVertical(lipgloss.Left, inputLines...))
}
