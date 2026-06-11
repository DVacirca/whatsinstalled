package scanner

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"installr/internal/pkg"
	"installr/internal/store"
)

// pipxVenvsDir returns the directory holding pipx's per-app virtualenvs.
func pipxVenvsDir() string {
	return filepath.Join(pkg.HomeDir(), ".local", "share", "pipx", "venvs")
}

// PipxScanner scans pipx-installed Python CLI applications (one isolated venv each).
type PipxScanner struct{}

func (PipxScanner) Name() string      { return "pipx" }
func (PipxScanner) IsAvailable() bool { return commandExists("pipx") }
func (s PipxScanner) Probe() bool {
	entries, _ := os.ReadDir(pipxVenvsDir())
	return len(entries) > 0
}

func (s PipxScanner) Scan() ([]store.Package, error) {
	out, err := exec.Command("pipx", "list", "--json").Output()
	if err != nil {
		return nil, nil
	}

	var data struct {
		Venvs map[string]struct {
			Metadata struct {
				MainPackage struct {
					Package        string `json:"package"`
					PackageVersion string `json:"package_version"`
				} `json:"main_package"`
			} `json:"metadata"`
		} `json:"venvs"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, nil
	}

	var pkgs []store.Package
	for venv, v := range data.Venvs {
		mp := v.Metadata.MainPackage
		name := mp.Package
		if name == "" {
			name = venv
		}
		location := filepath.Join(pipxVenvsDir(), venv)
		pkgs = append(pkgs, store.Package{
			Name:      name,
			Version:   mp.PackageVersion,
			Source:    "pipx",
			Location:  location,
			UpdatedAt: time.Now(),
			User:      pkg.CurrentUser(),
			LastUsed:  pkg.GetLastUsed(location),
		})
	}
	return pkgs, nil
}

func (s PipxScanner) Uninstall(name, _ string) error {
	return s.UninstallCmd(name, "").Run()
}
func (s PipxScanner) Install(name, _ string) error {
	return s.InstallCmd(name, "").Run()
}
func (s PipxScanner) UninstallCmd(name, _ string) *exec.Cmd {
	return exec.Command("pipx", "uninstall", name)
}
func (s PipxScanner) InstallCmd(name, _ string) *exec.Cmd {
	return exec.Command("pipx", "install", name)
}

var _ Scanner = PipxScanner{}
