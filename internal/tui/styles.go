package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Theme holds the full color palette for a UI theme.
type Theme struct {
	Name      string
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Fg        lipgloss.Color
	FgBright  lipgloss.Color
	FgDim     lipgloss.Color
	Accent    lipgloss.Color
	Purple    lipgloss.Color
	Orange    lipgloss.Color
	Green     lipgloss.Color
	Red       lipgloss.Color
}

// ── Well-known themes ──

var TokyoNight = Theme{
	Name:      "tokyo-night",
	Primary:   lipgloss.Color("#1a1b26"),
	Secondary: lipgloss.Color("#24283b"),
	Fg:        lipgloss.Color("#a9b1d6"),
	FgBright:  lipgloss.Color("#c0caf5"),
	FgDim:     lipgloss.Color("#565f89"),
	Accent:    lipgloss.Color("#7aa2f7"),
	Purple:    lipgloss.Color("#bb9af7"),
	Orange:    lipgloss.Color("#e0af68"),
	Green:     lipgloss.Color("#73daca"),
	Red:       lipgloss.Color("#f7768e"),
}

var Palenight = Theme{
	Name:      "palenight",
	Primary:   lipgloss.Color("#292d3e"),
	Secondary: lipgloss.Color("#2d3246"),
	Fg:        lipgloss.Color("#a6accd"),
	FgBright:  lipgloss.Color("#eeffff"),
	FgDim:     lipgloss.Color("#676e95"),
	Accent:    lipgloss.Color("#82aaff"),
	Purple:    lipgloss.Color("#c792ea"),
	Orange:    lipgloss.Color("#ffcb6b"),
	Green:     lipgloss.Color("#c3e88d"),
	Red:       lipgloss.Color("#f07178"),
}

var Dracula = Theme{
	Name:      "dracula",
	Primary:   lipgloss.Color("#282a36"),
	Secondary: lipgloss.Color("#44475a"),
	Fg:        lipgloss.Color("#f8f8f2"),
	FgBright:  lipgloss.Color("#ffffff"),
	FgDim:     lipgloss.Color("#6272a4"),
	Accent:    lipgloss.Color("#bd93f9"),
	Purple:    lipgloss.Color("#ff79c6"),
	Orange:    lipgloss.Color("#f1fa8c"),
	Green:     lipgloss.Color("#50fa7b"),
	Red:       lipgloss.Color("#ff5555"),
}

var Nord = Theme{
	Name:      "nord",
	Primary:   lipgloss.Color("#2e3440"),
	Secondary: lipgloss.Color("#3b4252"),
	Fg:        lipgloss.Color("#d8dee9"),
	FgBright:  lipgloss.Color("#eceff4"),
	FgDim:     lipgloss.Color("#4c566a"),
	Accent:    lipgloss.Color("#88c0d0"),
	Purple:    lipgloss.Color("#b48ead"),
	Orange:    lipgloss.Color("#ebcb8b"),
	Green:     lipgloss.Color("#a3be8c"),
	Red:       lipgloss.Color("#bf616a"),
}

var Gruvbox = Theme{
	Name:      "gruvbox",
	Primary:   lipgloss.Color("#282828"),
	Secondary: lipgloss.Color("#3c3836"),
	Fg:        lipgloss.Color("#ebdbb2"),
	FgBright:  lipgloss.Color("#fbf1c7"),
	FgDim:     lipgloss.Color("#928374"),
	Accent:    lipgloss.Color("#fabd2f"),
	Purple:    lipgloss.Color("#d3869b"),
	Orange:    lipgloss.Color("#fe8019"),
	Green:     lipgloss.Color("#b8bb26"),
	Red:       lipgloss.Color("#fb4934"),
}

var Catppuccin = Theme{
	Name:      "catppuccin",
	Primary:   lipgloss.Color("#1e1e2e"),
	Secondary: lipgloss.Color("#302d41"),
	Fg:        lipgloss.Color("#cdd6f4"),
	FgBright:  lipgloss.Color("#f5e0dc"),
	FgDim:     lipgloss.Color("#6c7086"),
	Accent:    lipgloss.Color("#89b4fa"),
	Purple:    lipgloss.Color("#cba6f7"),
	Orange:    lipgloss.Color("#f9e2af"),
	Green:     lipgloss.Color("#a6e3a1"),
	Red:       lipgloss.Color("#f38ba8"),
}

var Monokai = Theme{
	Name:      "monokai",
	Primary:   lipgloss.Color("#272822"),
	Secondary: lipgloss.Color("#383830"),
	Fg:        lipgloss.Color("#f8f8f2"),
	FgBright:  lipgloss.Color("#ffffff"),
	FgDim:     lipgloss.Color("#75715e"),
	Accent:    lipgloss.Color("#66d9ef"),
	Purple:    lipgloss.Color("#ae81ff"),
	Orange:    lipgloss.Color("#e6db74"),
	Green:     lipgloss.Color("#a6e22e"),
	Red:       lipgloss.Color("#f92672"),
}

var AllThemes = []Theme{TokyoNight, Palenight, Dracula, Nord, Gruvbox, Catppuccin, Monokai}

func findTheme(name string) Theme {
	for _, t := range AllThemes {
		if t.Name == name || strings.EqualFold(t.Name, name) {
			return t
		}
	}
	return TokyoNight
}

// currentThemeIndex returns the index in AllThemes of the active theme, or 0 if
// it can't be found. Used to open the theme picker on the current selection.
func currentThemeIndex() int {
	for i, t := range AllThemes {
		if t.Name == currentTheme.Name {
			return i
		}
	}
	return 0
}

// themeDir returns the config directory for whatsinstalled.
func themeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".config", "whatsinstalled")
}

// LoadThemeName reads the persisted theme name from config.
func LoadThemeName() string {
	d := themeDir()
	if d == "" {
		return TokyoNight.Name
	}
	b, err := os.ReadFile(filepath.Join(d, "theme"))
	if err != nil {
		return TokyoNight.Name
	}
	name := strings.TrimSpace(string(b))
	if name == "" {
		return TokyoNight.Name
	}
	return name
}

// SaveThemeName writes the theme name to config.
func SaveThemeName(name string) error {
	d := themeDir()
	if d == "" {
		return nil
	}
	if err := os.MkdirAll(d, 0755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(d, "theme"), []byte(name+"\n"), 0644)
}

// currentTheme holds the active theme.  Set by applyTheme.
var currentTheme Theme

// ── Color palette vars (set by applyTheme) ──

var (
	secondary lipgloss.Color
	fg        lipgloss.Color
	fgBright  lipgloss.Color
	fgDim     lipgloss.Color
	accent    lipgloss.Color
	purple    lipgloss.Color
	orange    lipgloss.Color
)

// ── Style vars (set by applyTheme) ──

var (
	shellStyle         lipgloss.Style
	shellTitleStyle    lipgloss.Style
	shellHeaderStyle   lipgloss.Style
	separatorStyle     lipgloss.Style
	tabInactiveStyle   lipgloss.Style
	tabActiveStyle     lipgloss.Style
	tabBarStyle        lipgloss.Style
	bottomPanelStyle   lipgloss.Style
	bottomTitleStyle   lipgloss.Style
	bottomKeyStyle     lipgloss.Style
	bottomValueStyle   lipgloss.Style
	bottomDimStyle     lipgloss.Style
	bottomDividerStyle lipgloss.Style
	sectionRuleStyle   lipgloss.Style
	filterStyle        lipgloss.Style
	statusBarStyle     lipgloss.Style
	modalBorderStyle   lipgloss.Style
	modalTitleStyle    lipgloss.Style
	modalInputStyle    lipgloss.Style
	bodyCellStyle      lipgloss.Style
	bodyGroupStyle     lipgloss.Style
	bodySelectedStyle  lipgloss.Style
)

// applyTheme rebuilds all colour and style variables for the given theme.
func applyTheme(t Theme) {
	currentTheme = t
	secondary = t.Secondary
	fg = t.Fg
	fgBright = t.FgBright
	fgDim = t.FgDim
	accent = t.Accent
	purple = t.Purple
	orange = t.Orange

	shellStyle = lipgloss.NewStyle()

	shellTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(fgBright)

	shellHeaderStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(fgBright)

	separatorStyle = lipgloss.NewStyle().
		Foreground(fgDim)

	tabInactiveStyle = lipgloss.NewStyle().
		Foreground(fgDim).
		Padding(0, 2)

	tabActiveStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(orange).
		Padding(0, 2)

	tabBarStyle = lipgloss.NewStyle().
		Padding(0, 1)

	bottomPanelStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(0, 1)

	bottomTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(purple)

	bottomKeyStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(orange)

	bottomValueStyle = lipgloss.NewStyle().
		Foreground(fgBright)

	bottomDimStyle = lipgloss.NewStyle().
		Foreground(fgDim).
		Italic(true)

	bottomDividerStyle = lipgloss.NewStyle().
		Foreground(accent)

	sectionRuleStyle = lipgloss.NewStyle().
		Foreground(accent)

	filterStyle = lipgloss.NewStyle().
		Foreground(fgBright).
		Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
		Foreground(fgBright).
		Padding(0, 1)

	modalBorderStyle = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(accent).
		Padding(1, 2)

	modalTitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(purple)

	modalInputStyle = lipgloss.NewStyle().
		Foreground(fgBright).
		Padding(0, 1)

	bodyCellStyle = lipgloss.NewStyle().
		Foreground(fg).
		Background(secondary)

	bodyGroupStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(purple).
		Background(secondary)

	bodySelectedStyle = lipgloss.NewStyle().
		Bold(true).
		Reverse(true)
}

func init() {
	applyTheme(findTheme(LoadThemeName()))
}

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
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// formatRelative renders a *time.Time as a compact age ("3d", "2M"), or "-" if
// nil. Shared by the Added (mtime) and Used (atime) columns.
func formatRelative(t *time.Time) string {
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
