package scanner

import (
	"os/exec"
	"strings"

	"whatsinstalled/internal/store"
)

// commandExists reports whether a command is in PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// cmdLine runs a command and returns its trimmed stdout, or "" on error or
// empty output. Handy for "ask the tool where it lives" lookups.
func cmdLine(name string, args ...string) string {
	out, err := exec.Command(name, args...).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Scanner discovers packages from a package manager.
type Scanner interface {
	// Name returns the manager name (apt, snap, npm, pip).
	Name() string
	// Scan returns installed packages. For npm/pip this includes both global and local envs.
	Scan() ([]store.Package, error)
	// IsAvailable returns true if the scanner's tool is present on the system.
	IsAvailable() bool
	// Probe does a lightweight check to see if any packages are actually present.
	// This prevents showing empty tabs for installed-but-unused tools.
	Probe() bool
}
