package scanner

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// BrewScanner scans Homebrew packages.
type BrewScanner struct{}

func (BrewScanner) Name() string      { return "brew" }
func (BrewScanner) IsAvailable() bool { return commandExists("brew") }
func (s BrewScanner) Probe() bool {
	out, _ := exec.Command("brew", "list", "--formula").Output()
	return len(out) > 0
}

func (s BrewScanner) Scan() ([]store.Package, error) {
	cmd := exec.Command("brew", "list", "--formula")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("brew list: %w", err)
	}

	// Determine brew prefix for location and owner
	brewPrefix := cmdLine("brew", "--prefix")
	if brewPrefix == "" {
		brewPrefix = "/home/linuxbrew/.linuxbrew"
	}
	owner := pkg.FileOwner(brewPrefix)

	var pkgs []store.Package
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		for _, name := range fields {
			pkgs = append(pkgs, store.Package{
				Name:      name,
				Version:   "",
				Source:    "brew",
				Location:  brewPrefix,
				UpdatedAt: time.Now(),
				User:      owner,
			})
		}
	}
	return pkgs, nil
}

var _ Scanner = BrewScanner{}
