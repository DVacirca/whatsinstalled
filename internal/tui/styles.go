package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	// Base colors — superfile-inspired dark palette
	bg       = lipgloss.Color("#1a1b26")
	bgLight  = lipgloss.Color("#24283b")
	bgLighter = lipgloss.Color("#414868")
	fg       = lipgloss.Color("#a9b1d6")
	fgBright = lipgloss.Color("#c0caf5")
	accent   = lipgloss.Color("#7aa2f7")
	accent2  = lipgloss.Color("#bb9af7")
	green    = lipgloss.Color("#73daca")
	red      = lipgloss.Color("#f7768e")
	orange   = lipgloss.Color("#e0af68")
	border   = lipgloss.Color("#565f89")

	// Layout styles
	appStyle = lipgloss.NewStyle().Background(bg)

	titleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(accent).
		Padding(0, 1)

	countStyle = lipgloss.NewStyle().
		Foreground(fg).
		Padding(0, 1)

	countAccentStyle = lipgloss.NewStyle().
		Foreground(accent2).
		Bold(true)

	// Table styles
	tableHeaderStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(fgBright).
		Background(bgLight).
		Padding(0, 1)

	tableCellStyle = lipgloss.NewStyle().
		Foreground(fg).
		Padding(0, 1)

	tableSelectedStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(bg).
		Background(accent).
		Padding(0, 1)

	tableBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Background(bg)

	// Tab bar (at bottom of table)
	tabInactiveStyle = lipgloss.NewStyle().
		Foreground(fg).
		Padding(0, 2)

	tabActiveStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(bg).
		Background(accent).
		Padding(0, 2)

	tabBarStyle = lipgloss.NewStyle().
		Background(bgLight).
		Padding(0, 1)

	// Bottom panel styles
	panelStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(border).
		Background(bg).
		Padding(0, 1)

	panelTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(accent).
		Padding(0, 1)

	panelKeyStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(orange)

	panelValueStyle = lipgloss.NewStyle().
		Foreground(fgBright)

	panelDimStyle = lipgloss.NewStyle().
		Foreground(fg).
		Italic(true)

	// Filter
	filterStyle = lipgloss.NewStyle().
		Foreground(fgBright).
		Background(bgLight).
		Padding(0, 1)

	// Status / confirm bar
	statusBarStyle = lipgloss.NewStyle().
		Background(bgLight).
		Foreground(fg).
		Padding(0, 1)

	confirmStyle = lipgloss.NewStyle().
		Background(red).
		Foreground(bg).
		Bold(true).
		Padding(0, 1)

	confirmKeyStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(green)

	// Modal styles
	modalBgStyle = lipgloss.NewStyle().
		Background(bg)

	modalBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Background(bg).
		Padding(1, 2)

	modalTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(accent)

	modalInputStyle = lipgloss.NewStyle().
		Foreground(fgBright).
		Background(bgLight).
		Padding(0, 1)
)

func formatSize(bytes *int64) string {
	if bytes == nil {
		return "-"
	}
	b := *bytes
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// clipLines truncates a string to at most n lines.
func clipLines(s string, n int) string {
	if n <= 0 {
		return ""
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= n {
		return s
	}
	return strings.Join(lines[:n], "\n")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func formatLastUsed(t *time.Time) string {
	if t == nil {
		return "-"
	}
	d := time.Since(*t)
	switch {
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	case d < 365*24*time.Hour:
		return fmt.Sprintf("%dM", int(d.Hours()/24/30))
	default:
		return fmt.Sprintf("%dY", int(d.Hours()/24/365))
	}
}
