package scanner

import (
	"os/exec"

	"installr/internal/store"
)

// Scanner discovers packages from a package manager.
type Scanner interface {
	// Name returns the manager name (apt, snap, npm, pip).
	Name() string
	// Scan returns installed packages. For npm/pip this includes both global and local envs.
	Scan() ([]store.Package, error)
	// Uninstall removes a package. For local envs, location is the project/venv path.
	Uninstall(name, location string) error
	// Install installs a package. For local envs, location is the project/venv path.
	Install(name, location string) error
	// UninstallCmd returns the exec.Cmd for uninstalling (used by TUI tea.ExecProcess).
	UninstallCmd(name, location string) *exec.Cmd
	// InstallCmd returns the exec.Cmd for installing (used by TUI tea.ExecProcess).
	InstallCmd(name, location string) *exec.Cmd
}
