package scanner

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// SnapScanner scans snap packages.
type SnapScanner struct{}

func (SnapScanner) Name() string      { return "snap" }
func (SnapScanner) IsAvailable() bool { return commandExists("snap") }
func (s SnapScanner) Probe() bool {
	out, _ := exec.Command("snap", "list").Output()
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return len(lines) > 1 // header + at least one package
}

func (s SnapScanner) Scan() ([]store.Package, error) {
	cmd := exec.Command("snap", "list")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("snap list: %w", err)
	}

	var pkgs []store.Package
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for i, line := range lines {
		if i == 0 {
			continue // skip header
		}
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}

		p := store.Package{
			Name:      fields[0],
			Version:   fields[1],
			Source:    "snap",
			Location:  filepath.Join("/snap", fields[0]),
			User:      pkg.FileOwner(filepath.Join("/snap", fields[0])),
			UpdatedAt: time.Now(),
			LastUsed:  pkg.GetLastUsed(filepath.Join("/snap", fields[0])),
		}

		// Try to get summary from snap info
		infoCmd := exec.Command("snap", "info", p.Name)
		infoOut, _ := infoCmd.Output()
		for _, l := range strings.Split(string(infoOut), "\n") {
			if strings.HasPrefix(l, "summary:") {
				p.Description = strings.TrimSpace(strings.TrimPrefix(l, "summary:"))
				break
			}
		}

		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

var _ Scanner = SnapScanner{}
