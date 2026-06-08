package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"installr/internal/store"
)

// treeNode is either a group (location) or a leaf (package).
type treeNode struct {
	isGroup  bool
	label    string       // display label
	count    int          // child count for groups
	pkg      *store.Package // nil for groups
	children []*treeNode
	expanded bool
	depth    int
}

// treeView manages a tree of packages grouped by location.
type treeView struct {
	roots      []*treeNode
	flat       []*treeNode  // visible nodes (respecting expand state)
	cursor     int
	scrollOff  int
	expanded   map[string]bool // persistent expand state by "source:location" key
}

func newTreeView() *treeView {
	return &treeView{
		expanded: make(map[string]bool),
	}
}

// buildTree groups packages by location and builds a 2-level tree.
func (tv *treeView) buildTree(packages []store.Package) {
	// Group by location
	byLoc := make(map[string][]store.Package)
	for _, p := range packages {
		byLoc[p.Location] = append(byLoc[p.Location], p)
	}

	// Sort locations
	var locs []string
	for loc := range byLoc {
		locs = append(locs, loc)
	}
	sort.Strings(locs)

	var roots []*treeNode
	for _, loc := range locs {
		pkgs := byLoc[loc]
		// Sort packages within group by name
		sort.Slice(pkgs, func(i, j int) bool {
			return strings.ToLower(pkgs[i].Name) < strings.ToLower(pkgs[j].Name)
		})

		group := &treeNode{
			isGroup:  true,
			label:    loc,
			count:    len(pkgs),
			expanded: tv.isExpanded(loc),
			depth:    0,
		}
		for i := range pkgs {
			p := pkgs[i]
			group.children = append(group.children, &treeNode{
				isGroup: false,
				label:   p.Name,
				pkg:     &p,
				depth:   1,
			})
		}
		roots = append(roots, group)
	}

	tv.roots = roots
	tv.rebuildFlat()
}

func (tv *treeView) isExpanded(loc string) bool {
	if v, ok := tv.expanded[loc]; ok {
		return v
	}
	// Default: expand groups with few items, collapse large ones
	return false
}

func (tv *treeView) toggle(loc string) {
	tv.expanded[loc] = !tv.isExpanded(loc)
	for _, r := range tv.roots {
		if r.label == loc {
			r.expanded = tv.expanded[loc]
			break
		}
	}
	tv.rebuildFlat()
}

func (tv *treeView) rebuildFlat() {
	tv.flat = nil
	for _, r := range tv.roots {
		tv.appendVisible(r)
	}
	if tv.cursor >= len(tv.flat) {
		tv.cursor = len(tv.flat) - 1
	}
	if tv.cursor < 0 {
		tv.cursor = 0
	}
}

func (tv *treeView) appendVisible(n *treeNode) {
	tv.flat = append(tv.flat, n)
	if n.isGroup && n.expanded {
		for _, c := range n.children {
			tv.flat = append(tv.flat, c)
		}
	}
}

func (tv *treeView) selected() *treeNode {
	if tv.cursor < 0 || tv.cursor >= len(tv.flat) {
		return nil
	}
	return tv.flat[tv.cursor]
}

func (tv *treeView) selectedPkg() *store.Package {
	sel := tv.selected()
	if sel != nil && !sel.isGroup {
		return sel.pkg
	}
	return nil
}

func (tv *treeView) moveCursor(delta int) {
	tv.cursor += delta
	if tv.cursor < 0 {
		tv.cursor = 0
	}
	if tv.cursor >= len(tv.flat) {
		tv.cursor = len(tv.flat) - 1
	}
}

func (tv *treeView) setCursorToPkg(name, source, location string) {
	for i, n := range tv.flat {
		if !n.isGroup && n.pkg != nil {
			p := n.pkg
			if p.Name == name && p.Source == source && p.Location == location {
				tv.cursor = i
				return
			}
		}
	}
}

// render draws the tree, returns lines as a single string.
func (tv *treeView) render(width, height int) string {
	if len(tv.flat) == 0 {
		return tableCellStyle.Render("  No packages found. Press 'r' to scan.")
	}

	// Adjust scroll to keep cursor visible
	if tv.cursor < tv.scrollOff {
		tv.scrollOff = tv.cursor
	}
	if tv.cursor >= tv.scrollOff+height {
		tv.scrollOff = tv.cursor - height + 1
	}
	if tv.scrollOff < 0 {
		tv.scrollOff = 0
	}

	end := tv.scrollOff + height
	if end > len(tv.flat) {
		end = len(tv.flat)
	}

	var lines []string
	for i := tv.scrollOff; i < end; i++ {
		lines = append(lines, tv.renderNode(tv.flat[i], i == tv.cursor, width))
	}

	// Pad remaining height
	for len(lines) < height {
		lines = append(lines, strings.Repeat(" ", width))
	}

	return strings.Join(lines, "\n")
}

func (tv *treeView) renderNode(n *treeNode, selected bool, width int) string {
	if n.isGroup {
		return tv.renderGroupNode(n, selected, width)
	}
	return tv.renderLeafNode(n, selected, width)
}

func (tv *treeView) renderGroupNode(n *treeNode, selected bool, width int) string {
	prefix := "▸ "
	if n.expanded {
		prefix = "▾ "
	}
	label := fmt.Sprintf("%s%s", prefix, n.label)
	countStr := fmt.Sprintf("[%d]", n.count)

	// Pad to full width
	padding := width - lipgloss.Width(label) - lipgloss.Width(countStr) - 1
	if padding < 1 {
		padding = 1
	}
	line := label + strings.Repeat(" ", padding) + countStr

	if selected {
		pad := width - lipgloss.Width(line)
		if pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		return tableSelectedStyle.Render(line)
	}
	return lipgloss.NewStyle().Bold(true).Foreground(accent).Render(line)
}

func (tv *treeView) renderLeafNode(n *treeNode, selected bool, width int) string {
	p := n.pkg
	if p == nil {
		return strings.Repeat(" ", width)
	}

	indent := "  "
	name := truncate(p.Name, 14)
	ver := truncate(p.Version, 8)
	src := truncate(p.Source, 5)
	loc := truncate(p.Location, 12)
	user := truncate(p.User, 6)
	size := formatSize(p.SizeBytes)
	lastUsed := formatLastUsed(p.LastUsed)

	line := fmt.Sprintf("%s%-14s %-8s %-5s %-12s %-6s %-6s %-6s",
		indent, name, ver, src, loc, user, size, lastUsed)

	if selected {
		pad := width - lipgloss.Width(line)
		if pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		return tableSelectedStyle.Render(line)
	}
	return tableCellStyle.Render(line)
}

func renderTreeHeader(width int) string {
	line := fmt.Sprintf("  %-14s %-8s %-5s %-12s %-6s %-6s %-6s",
		"Name", "Version", "Src", "Location", "User", "Size", "Used")
	pad := width - lipgloss.Width(line)
	if pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return tableHeaderStyle.Render(line)
}
