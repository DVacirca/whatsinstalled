package tui

import (
	"strings"
	"testing"

	"installr/internal/version"
)

// TestAboutModalContent verifies the About modal shows the name, version, author
// and a non-empty description.
func TestAboutModalContent(t *testing.T) {
	out := aboutModalContent(54)

	for _, want := range []string{
		"installr",
		version.Version,
		"Dante Vacirca",
		"Claude",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("about modal missing %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "everything installed") {
		t.Fatalf("about modal missing the purpose statement:\n%s", out)
	}
}
