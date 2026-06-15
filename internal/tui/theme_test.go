package tui

import "testing"

// TestCurrentThemeIndex verifies the theme picker can open on the active theme
// (the fix for the picker always reopening at index 0).
func TestCurrentThemeIndex(t *testing.T) {
	orig := currentTheme
	t.Cleanup(func() { applyTheme(orig) })

	for want, th := range AllThemes {
		applyTheme(th)
		if got := currentThemeIndex(); got != want {
			t.Fatalf("after applyTheme(%s): currentThemeIndex() = %d, want %d", th.Name, got, want)
		}
	}
}

func TestCurrentThemeIndexUnknownDefaultsToZero(t *testing.T) {
	orig := currentTheme
	t.Cleanup(func() { applyTheme(orig) })

	currentTheme = Theme{Name: "does-not-exist"}
	if got := currentThemeIndex(); got != 0 {
		t.Fatalf("unknown theme: currentThemeIndex() = %d, want 0", got)
	}
}
