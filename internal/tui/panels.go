package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"whatsinstalled/internal/version"
)

// renderDetailPanel renders the left-hand bottom panel: the description of the
// selected package (or the location of the selected group).
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
	return padLines(lines, h)
}

// renderDetailOverlay renders the full package-details modal: description plus a
// metadata table for the selected package.
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
		lines = append(lines, bottomKeyStyle.Render("Description"))
		lines = append(lines, sectionRuleStyle.Render(strings.Repeat("─", w-1)))
		if sel.Description != "" {
			lines = append(lines, truncate(sel.Description, w-2))
		} else {
			lines = append(lines, bottomDimStyle.Render("No description available"))
		}
		lines = append(lines, "")

		lines = append(lines, bottomKeyStyle.Render("Metadata"))
		lines = append(lines, sectionRuleStyle.Render(strings.Repeat("─", w-1)))

		fields := []struct{ k, v string }{
			{"Name", sel.Name},
			{"Version", sel.Version},
			{"Source", sel.Source},
			{"Location", sel.Location},
			{"User", sel.User},
			{"Size", formatSize(sel.SizeBytes)},
			{"Added", formatRelative(sel.AddedAt)},
			{"Last Used", formatRelative(sel.LastUsed)},
		}
		for _, f := range fields {
			key := bottomKeyStyle.Render(f.k + ":")
			valW := w - lipgloss.Width(key) - 1
			if valW < 1 {
				valW = 1
			}
			val := bottomValueStyle.Render(truncate(f.v, valW))
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, key, " ", val))
		}
	}
	lines = append(lines, "")
	lines = append(lines, lipgloss.NewStyle().Foreground(fgDim).Render("Press Esc or d to close"))
	return padLines(lines, h)
}

// renderHelpPanel renders the right-hand bottom panel: a compact keybinding
// cheatsheet. It uses two columns when wide enough, otherwise a single column.
func (m *model) renderHelpPanel(w, h int) string {
	// Two-column layout fills row-major: even indices land in the left column,
	// odd indices in the right. Left = actions (':' / Ask first), right =
	// navigation. Capped at 8 (4 rows); everything else lives in the palette (':').
	keys := []struct{ k, v string }{
		{":", "Command"}, {"↑↓ / jk", "Navigate"},
		{"?", "Ask LLM"}, {"←→ / hl", "Expand"},
		{"a", "About"}, {"Tab", "Switch tab"},
		{"q", "Quit"}, {"/", "Filter"},
	}

	lines := []string{bottomTitleStyle.Render(truncate("Keys", w-1))}
	lines = append(lines, sectionRuleStyle.Render(strings.Repeat("─", w-1)))

	if w < 20 {
		// Single column with dynamic widths.
		for _, kv := range keys {
			key := bottomKeyStyle.Render(kv.k)
			valW := w - lipgloss.Width(key) - 1
			if valW < 1 {
				valW = 1
			}
			val := bottomValueStyle.Render(truncate(kv.v, valW))
			lines = append(lines, lipgloss.JoinHorizontal(lipgloss.Left, key, " ", val))
		}
	} else {
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
	return padLines(lines, h)
}

// renderStatusBar renders the single-line status bar at the bottom of the
// screen, summarising the current activity and selection.
func (m *model) renderStatusBar() string {
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
		parts = append(parts, fmt.Sprintf("whatsinstalled — %s", currentTheme.Name))
	}

	return statusBarStyle.Width(m.width).Render(strings.Join(parts, "  │  "))
}

// About modal text. Kept as package-level values so tests can assert on them.
const (
	aboutAuthor = "by Dante Vacirca + Claude"
	aboutBlurb  = "One view of everything installed across every package " +
		"manager, language ecosystem, and loose binary — so the " +
		"tools you've accumulated stop being invisible. Built to " +
		"answer \"what do I actually have, and what is it for?\""
)

// aboutModalContent renders the inner content of the About modal. modalWidth is
// the width passed to modalBorderStyle; its Padding(1,2) leaves modalWidth-4 for
// text, so the blurb must wrap to that to avoid the box re-wrapping it raggedly.
func aboutModalContent(modalWidth int) string {
	inner := modalWidth - 4
	if inner < 1 {
		inner = 1
	}
	lines := []string{
		modalTitleStyle.Render("whatsinstalled " + version.Version),
		"",
		lipgloss.NewStyle().Foreground(fgDim).Render(aboutAuthor),
		"",
		lipgloss.NewStyle().Foreground(fg).Width(inner).Render(aboutBlurb),
		"",
		lipgloss.NewStyle().Foreground(fgDim).Render("Esc to close"),
	}
	return lipgloss.JoinVertical(lipgloss.Left, lines...)
}

// padLines joins lines into a fixed-height block of exactly h lines, padding
// with blanks or truncating as needed.
func padLines(lines []string, h int) string {
	if len(lines) > h {
		lines = lines[:h]
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}
