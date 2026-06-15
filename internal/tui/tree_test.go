package tui

import (
	"strings"
	"testing"
)

func TestRenderTreeHeaderHasAllColumns(t *testing.T) {
	hdr := renderTreeHeader(160)
	for _, col := range []string{"Name", "Version", "Src", "Location", "User", "Size", "Added", "Used"} {
		if !strings.Contains(hdr, col) {
			t.Fatalf("header missing %q column:\n%s", col, hdr)
		}
	}
}

// TestColumnWidthsFit guards against the 8 columns overflowing the available
// width (which would push the layout and misalign rows).
func TestColumnWidthsFit(t *testing.T) {
	const indent = 2
	const gaps = 7 // single space between 8 columns
	for _, w := range []int{40, 80, 120, 200} {
		avail := w - indent
		c := calcColumnWidths(avail)
		cols := []int{c.name, c.ver, c.src, c.loc, c.user, c.size, c.added, c.used}
		sum := gaps
		for _, v := range cols {
			if v < 1 {
				t.Fatalf("w=%d: non-positive column width in %v", w, cols)
			}
			sum += v
		}
		if sum > avail {
			t.Fatalf("w=%d: columns sum %d exceed available %d (%v)", w, sum, avail, cols)
		}
	}
}
