package scanner

import (
	"encoding/json"
	"os/exec"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// pnpmGlobalDir returns the global pnpm packages directory.
func pnpmGlobalDir() string {
	out, err := exec.Command("pnpm", "root", "-g").Output()
	if err != nil {
		return "global"
	}
	if dir := strings.TrimSpace(string(out)); dir != "" {
		return dir
	}
	return "global"
}

// PnpmScanner scans globally installed pnpm packages.
type PnpmScanner struct{}

func (PnpmScanner) Name() string      { return "pnpm" }
func (PnpmScanner) IsAvailable() bool { return commandExists("pnpm") }
func (s PnpmScanner) Probe() bool {
	out, _ := exec.Command("pnpm", "ls", "-g", "--depth=0", "--json").Output()
	return len(out) > 10 // non-empty JSON with at least one dependency
}

func (s PnpmScanner) Scan() ([]store.Package, error) {
	out, err := exec.Command("pnpm", "ls", "-g", "--depth=0", "--json").Output()
	if err != nil {
		return nil, nil
	}
	return parsePnpmList(out, pnpmGlobalDir()), nil
}

// parsePnpmList parses `pnpm ls -g --depth=0 --json` output, an array of
// project objects each with a "dependencies" map of name -> {version}.
func parsePnpmList(out []byte, location string) []store.Package {
	var projects []struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal(out, &projects); err != nil {
		return nil
	}

	var pkgs []store.Package
	for _, proj := range projects {
		for name, dep := range proj.Dependencies {
			pkgs = append(pkgs, store.Package{
				Name:      name,
				Version:   dep.Version,
				Source:    "pnpm",
				Location:  location,
				UpdatedAt: time.Now(),
				User:      pkg.CurrentUser(),
			})
		}
	}
	return pkgs
}

func (s PnpmScanner) Uninstall(name, _ string) error {
	return s.UninstallCmd(name, "").Run()
}
func (s PnpmScanner) Install(name, _ string) error {
	return s.InstallCmd(name, "").Run()
}
func (s PnpmScanner) UninstallCmd(name, _ string) *exec.Cmd {
	return exec.Command("pnpm", "remove", "-g", name)
}
func (s PnpmScanner) InstallCmd(name, _ string) *exec.Cmd {
	return exec.Command("pnpm", "add", "-g", name)
}

var _ Scanner = PnpmScanner{}
