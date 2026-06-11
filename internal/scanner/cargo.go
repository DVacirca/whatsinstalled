package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"installr/internal/pkg"
	"installr/internal/store"
)

// CargoScanner scans Cargo-installed Rust binaries.
type CargoScanner struct{}

func (CargoScanner) Name() string      { return "cargo" }
func (CargoScanner) IsAvailable() bool { return commandExists("cargo") }
func (s CargoScanner) Probe() bool {
	binDir := filepath.Join(pkg.HomeDir(), ".cargo", "bin")
	entries, _ := os.ReadDir(binDir)
	for _, e := range entries {
		if !e.IsDir() {
			return true
		}
	}
	return false
}

func (s CargoScanner) Scan() ([]store.Package, error) {
	binDir := filepath.Join(pkg.HomeDir(), ".cargo", "bin")
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return nil, nil
	}

	var pkgs []store.Package
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.Mode()&0o111 == 0 {
			continue
		}
		pkgs = append(pkgs, store.Package{
			Name:      entry.Name(),
			Version:   "",
			Source:    "cargo",
			Location:  binDir,
			UpdatedAt: time.Now(),
			User:      pkg.CurrentUser(),
		})
	}
	return pkgs, nil
}

func (s CargoScanner) Uninstall(name, _ string) error {
	return s.UninstallCmd(name, "").Run()
}
func (s CargoScanner) Install(name, _ string) error {
	return s.InstallCmd(name, "").Run()
}
func (s CargoScanner) UninstallCmd(name, _ string) *exec.Cmd {
	return exec.Command("cargo", "uninstall", name)
}
func (s CargoScanner) InstallCmd(name, _ string) *exec.Cmd {
	return exec.Command("cargo", "install", name)
}

var _ Scanner = CargoScanner{}
