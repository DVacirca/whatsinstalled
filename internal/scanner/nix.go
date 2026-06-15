package scanner

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
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
			Location:  "/nix",
			UpdatedAt: time.Now(),
			User:      pkg.FileOwner("/nix"),
		})
	}
	return pkgs, nil
}

var _ Scanner = NixScanner{}
