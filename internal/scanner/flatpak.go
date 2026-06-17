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

// flatpakAppDir returns where a flatpak app's files live: the per-app directory
// under the system or per-user flatpak installation.
func flatpakAppDir(appID, installation string) string {
	if installation == "user" {
		return filepath.Join(pkg.HomeDir(), ".local", "share", "flatpak", "app", appID)
	}
	return filepath.Join("/var/lib/flatpak", "app", appID)
}

// FlatpakScanner scans flatpak packages.
type FlatpakScanner struct{}

func (FlatpakScanner) Name() string      { return "flatpak" }
func (FlatpakScanner) IsAvailable() bool { return commandExists("flatpak") }
func (s FlatpakScanner) Probe() bool {
	out, _ := exec.Command("flatpak", "list", "--app").Output()
	return len(out) > 0
}

func (s FlatpakScanner) Scan() ([]store.Package, error) {
	cmd := exec.Command("flatpak", "list", "--app", "--columns=application,version,installation")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("flatpak list: %w", err)
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
		ver, installation := "", ""
		if len(fields) > 1 {
			ver = fields[1]
		}
		if len(fields) > 2 {
			installation = fields[2]
		}
		location := flatpakAppDir(fields[0], installation)
		pkgs = append(pkgs, store.Package{
			Name:      fields[0],
			Version:   ver,
			Source:    "flatpak",
			Location:  location,
			UpdatedAt: time.Now(),
			User:      pkg.FileOwner(location),
		})
	}
	return pkgs, nil
}

var _ Scanner = FlatpakScanner{}
