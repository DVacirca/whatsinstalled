package scanner

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"installr/internal/store"
)

// NixScanner scans nix packages.
type NixScanner struct{}

func (NixScanner) Name() string      { return "nix" }
func (NixScanner) IsAvailable() bool { return commandExists("nix-env") }
func (s NixScanner) Probe() bool {
	out, _ := exec.Command("nix-env", "-q").Output()
	return len(out) > 0
}

func (s NixScanner) Scan() ([]store.Package, error) {
	cmd := exec.Command("nix-env", "-q", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("nix-env -q: %w", err)
	}

	var pkgs []store.Package
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 1 {
			continue
		}
		ver := ""
		if len(fields) > 1 {
			ver = fields[1]
		}
		pkgs = append(pkgs, store.Package{
			Name:      fields[0],
			Version:   ver,
			Source:    "nix",
			Location:  "system",
			UpdatedAt: time.Now(),
			User:      "system",
		})
	}
	return pkgs, nil
}

func (s NixScanner) Uninstall(name, _ string) error {
	return s.UninstallCmd(name, "").Run()
}
func (s NixScanner) Install(name, _ string) error {
	return s.InstallCmd(name, "").Run()
}
func (s NixScanner) UninstallCmd(name, _ string) *exec.Cmd {
	return exec.Command("nix-env", "-e", name)
}
func (s NixScanner) InstallCmd(name, _ string) *exec.Cmd {
	return exec.Command("nix-env", "-iA", name)
}

var _ Scanner = NixScanner{}
