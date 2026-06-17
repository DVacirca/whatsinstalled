package scanner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// pacmanPackageLocation derives a representative install directory for a pacman
// package from its local file database, falling back to the pacman DB dir.
// Shared by the pacman and yay (AUR) scanners.
func pacmanPackageLocation(name, version string) string {
	if name == "" || version == "" {
		return "/var/lib/pacman"
	}
	data, err := os.ReadFile(filepath.Join("/var/lib/pacman/local", name+"-"+version, "files"))
	if err != nil {
		return "/var/lib/pacman"
	}
	return deriveInstallDir(parsePacmanFiles(string(data)), "/var/lib/pacman")
}

// parsePacmanFiles extracts absolute file paths from a pacman "files" database
// entry, reading only the %FILES% section (paths are relative, dirs end in /).
func parsePacmanFiles(content string) []string {
	var lines []string
	inFiles := false
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "%") {
			inFiles = line == "%FILES%"
			continue
		}
		if inFiles {
			lines = append(lines, "/"+strings.TrimSuffix(line, "/"))
		}
	}
	return lines
}

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
		location := pacmanPackageLocation(fields[0], ver)
		pkgs = append(pkgs, store.Package{
			Name:      fields[0],
			Version:   ver,
			Source:    "pacman",
			Location:  location,
			UpdatedAt: time.Now(),
			User:      pkg.FileOwner(location),
		})
	}
	return pkgs, nil
}

var _ Scanner = PacmanScanner{}
