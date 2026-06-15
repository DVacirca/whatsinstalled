package scanner

import (
	"os/exec"

	"whatsinstalled/internal/store"
)

// commandExists reports whether a command is in PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
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
