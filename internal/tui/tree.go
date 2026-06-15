package tui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"whatsinstalled/internal/store"
)

// treeNode is either a group (location) or a leaf (package).
type treeNode struct {
	isGroup  bool
	label    string         // display label
	count    int            // child count for groups
	pkg      *store.Package // nil for groups
	children []*treeNode
	expanded bool
	depth    int
}

// treeView manages a tree of packages grouped by location.
type treeView struct {
	roots     []*treeNode
	flat      []*treeNode // visible nodes (respecting expand state)
	cursor    int
	scrollOff int
	expanded  map[string]bool // persistent expand state by "source:location" key
}

func newTreeView() *treeView {
	return &treeView{
		expanded: make(map[string]bool),
	}
}

// buildTree groups packages by location and builds a 2-level tree.
func (tv *treeView) buildTree(packages []store.Package) {
	byLoc := make(map[string][]store.Package)
	for _, p := range packages {
		byLoc[p.Location] = append(byLoc[p.Location], p)
	}

	var locs []string
	for loc := range byLoc {
		locs = append(locs, loc)
	}
	sort.Strings(locs)

	var roots []*treeNode
	for _, loc := range locs {
		pkgs := byLoc[loc]
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

// render draws the tree, returns lines as a single string.
func (tv *treeView) render(width, height int) string {
	if len(tv.flat) == 0 {
		msg := "  No packages found. Press 'r' to scan."
		pad := width - lipgloss.Width(msg)
		if pad > 0 {
			msg += strings.Repeat(" ", pad)
		}
		// Pad to full height so the body doesn't collapse when empty.
		lines := []string{bodyCellStyle.Render(msg)}
		for len(lines) < height {
			lines = append(lines, bodyCellStyle.Render(strings.Repeat(" ", width)))
		}
		return strings.Join(lines, "\n")
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

	// Pad remaining height with blank bg-filled lines
	for len(lines) < height {
		blank := strings.Repeat(" ", width)
		lines = append(lines, bodyCellStyle.Render(blank))
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
		return bodySelectedStyle.Render(line)
	}
	return bodyGroupStyle.Render(line)
}

func (tv *treeView) renderLeafNode(n *treeNode, selected bool, width int) string {
	p := n.pkg
	if p == nil {
		blank := strings.Repeat(" ", width)
		return bodyCellStyle.Render(blank)
	}

	indent := "  "
	name := p.Name
	if p.AutoInstalled {
		indent = "    \u21b3 "
	}
	availWidth := width - lipgloss.Width(indent)
	cols := calcColumnWidths(availWidth)

	name = truncate(name, cols.name)
	ver := truncate(p.Version, cols.ver)
	src := truncate(p.Source, cols.src)
	loc := truncate(p.Location, cols.loc)
	user := truncate(p.User, cols.user)
	size := formatSize(p.SizeBytes)
	sizeStr := truncate(size, cols.size)
	added := truncate(formatRelative(p.AddedAt), cols.added)
	lastUsed := formatRelative(p.LastUsed)

	line := fmt.Sprintf("%s%-*s %-*s %-*s %-*s %-*s %-*s %-*s %-*s",
		indent,
		cols.name, name,
		cols.ver, ver,
		cols.src, src,
		cols.loc, loc,
		cols.user, user,
		cols.size, sizeStr,
		cols.added, added,
		cols.used, lastUsed)

	if selected {
		pad := width - lipgloss.Width(line)
		if pad > 0 {
			line += strings.Repeat(" ", pad)
		}
		return bodySelectedStyle.Render(line)
	}
	return bodyCellStyle.Render(line)
}

type colWidths struct {
	name, ver, src, loc, user, size, added, used int
}

func calcColumnWidths(availWidth int) colWidths {
	const spaces = 7 // spaces between 8 columns
	const (
		minName  = 6
		minVer   = 3
		minSrc   = 3
		minLoc   = 4
		minUser  = 3
		minSize  = 3
		minAdded = 3
		minUsed  = 3
	)
	minContent := minName + minVer + minSrc + minLoc + minUser + minSize + minAdded + minUsed
	target := colWidths{14, 40, 10, 10, 6, 12, 6, 6}
	targetContent := target.name + target.ver + target.src + target.loc + target.user + target.size + target.added + target.used

	contentWidth := availWidth - spaces
	if contentWidth <= 0 {
		return colWidths{1, 1, 1, 1, 1, 1, 1, 1}
	}

	// Helper to distribute remaining space proportionally.
	scale := func(min, target int) int {
		if contentWidth <= minContent {
			// Scale down from minimums
			s := float64(contentWidth) / float64(minContent)
			w := int(float64(min) * s)
			if w < 1 {
				w = 1
			}
			return w
		}
		if contentWidth >= targetContent {
			return target
		}
		s := float64(contentWidth-minContent) / float64(targetContent-minContent)
		return min + int(float64(target-min)*s)
	}

	c := colWidths{
		scale(minName, target.name),
		scale(minVer, target.ver),
		scale(minSrc, target.src),
		scale(minLoc, target.loc),
		scale(minUser, target.user),
		scale(minSize, target.size),
		scale(minAdded, target.added),
		scale(minUsed, target.used),
	}

	// Ensure total content fits exactly; subtract overflow from location.
	total := c.name + c.ver + c.src + c.loc + c.user + c.size + c.added + c.used
	if total > contentWidth {
		c.loc -= total - contentWidth
		if c.loc < 1 {
			c.loc = 1
		}
	}
	// Below target, scale() interpolates and rounding can leave a small
	// remainder — give it to location. At/above target the slack is handled
	// by the name/location split below, so don't add it here too (doing both
	// double-counted the slack and made rows ~2x too wide).
	if total < contentWidth && contentWidth < targetContent {
		c.loc += contentWidth - total
	}

	// If we have extra space (target or above), grow name modestly (capped) and
	// give the bulk to location, which holds the longest values (paths). Splitting
	// 50/50 made name balloon on wide terminals.
	if contentWidth >= targetContent {
		extra := contentWidth - targetContent
		const maxName = 28
		nameExtra := extra / 4
		if c.name+nameExtra > maxName {
			nameExtra = maxName - c.name
		}
		if nameExtra < 0 {
			nameExtra = 0
		}
		c.name += nameExtra
		c.loc += extra - nameExtra
	}

	return c
}

func renderTreeHeader(width int) string {
	indent := "  "
	availWidth := width - lipgloss.Width(indent)
	cols := calcColumnWidths(availWidth)

	line := fmt.Sprintf("%s%-*s %-*s %-*s %-*s %-*s %-*s %-*s %-*s",
		indent,
		cols.name, "Name",
		cols.ver, "Version",
		cols.src, "Src",
		cols.loc, "Location",
		cols.user, "User",
		cols.size, "Size",
		cols.added, "Added",
		cols.used, "Used")
	pad := width - lipgloss.Width(line)
	if pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return shellHeaderStyle.Render(line)
}
