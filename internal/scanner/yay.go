package scanner

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// YayScanner scans AUR packages via yay.
type YayScanner struct{}

func (YayScanner) Name() string      { return "yay" }
func (YayScanner) IsAvailable() bool { return commandExists("yay") }
func (s YayScanner) Probe() bool {
	out, _ := exec.Command("yay", "-Q").Output()
	return len(out) > 0
}

func (s YayScanner) Scan() ([]store.Package, error) {
	cmd := exec.Command("yay", "-Q")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("yay -Q: %w", err)
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
			Source:    "yay",
			Location:  location,
			UpdatedAt: time.Now(),
			User:      pkg.FileOwner(location),
		})
	}
	return pkgs, nil
}

var _ Scanner = YayScanner{}
