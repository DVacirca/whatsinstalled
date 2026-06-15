package scanner

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"whatsinstalled/internal/store"
)

// PacmanScanner scans Arch pacman packages.
type PacmanScanner struct{}

func (PacmanScanner) Name() string      { return "pacman" }
func (PacmanScanner) IsAvailable() bool { return commandExists("pacman") }
func (s PacmanScanner) Probe() bool {
	out, _ := exec.Command("pacman", "-Q").Output()
	return len(out) > 0
}

func (s PacmanScanner) Scan() ([]store.Package, error) {
	cmd := exec.Command("pacman", "-Q")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pacman -Q: %w", err)
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
			Source:    "pacman",
			Location:  "system",
			UpdatedAt: time.Now(),
			User:      "system",
		})
	}
	return pkgs, nil
}

var _ Scanner = PacmanScanner{}
