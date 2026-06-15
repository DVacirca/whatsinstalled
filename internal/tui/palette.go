package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// commandDef describes a single entry in the command palette (opened with ':').
type commandDef struct {
	label       string
	desc        string
	key         string // the direct keybinding that also triggers this command
	requiresPkg bool   // when true, the command is a no-op unless a package is selected
	action      func(m *model) tea.Cmd
}

// paletteCommands is the full, ordered list of commands shown in the palette.
// Each command mirrors a top-level keybinding handled in Update.
var paletteCommands = []commandDef{
	{"Details", "Show details", "d", false, func(m *model) tea.Cmd { m.mode = "detail"; return nil }},
	{"Filter", "Filter packages by name", "/", false, func(m *model) tea.Cmd {
		m.filtering = true
		m.filter = ""
		return nil
	}},
	{"Search", "Semantic search with LLM (experimental)", "?", false, func(m *model) tea.Cmd {
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
	{"Deps", "Show/hide apt packages auto-installed as dependencies (apt only)", "D", false, func(m *model) tea.Cmd {
		m.hideAuto = !m.hideAuto
		return m.loadData
	}},
	{"Quit", "Quit whatsinstalled", "q", false, func(m *model) tea.Cmd { return tea.Quit }},
	{"Theme", "Switch color theme", "t", false, func(m *model) tea.Cmd {
		m.mode = "theme-picker"
		m.themePickerIndex = currentThemeIndex()
		return nil
	}},
	{"About", "About whatsinstalled", "a", false, func(m *model) tea.Cmd {
		m.mode = "about"
		return nil
	}},
}

// filteredPaletteCommands returns the commands whose label or description match
// the current palette query (case-insensitive substring). An empty query
// returns every command.
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
