package scanner

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// FlatpakScanner scans flatpak packages.
type FlatpakScanner struct{}

func (FlatpakScanner) Name() string      { return "flatpak" }
func (FlatpakScanner) IsAvailable() bool { return commandExists("flatpak") }
func (s FlatpakScanner) Probe() bool {
	out, _ := exec.Command("flatpak", "list", "--app").Output()
	return len(out) > 0
}

func (s FlatpakScanner) Scan() ([]store.Package, error) {
	cmd := exec.Command("flatpak", "list", "--app", "--columns=application,version")
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
		ver := ""
		if len(fields) > 1 {
			ver = fields[1]
		}
		pkgs = append(pkgs, store.Package{
			Name:      fields[0],
			Version:   ver,
			Source:    "flatpak",
			Location:  "/var/lib/flatpak",
			UpdatedAt: time.Now(),
			User:      pkg.FileOwner("/var/lib/flatpak"),
		})
	}
	return pkgs, nil
}

var _ Scanner = FlatpakScanner{}
