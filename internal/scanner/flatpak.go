package scanner

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"installr/internal/store"
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
			Location:  "system",
			UpdatedAt: time.Now(),
			User:      "system",
		})
	}
	return pkgs, nil
}

func (s FlatpakScanner) Uninstall(name, _ string) error {
	return s.UninstallCmd(name, "").Run()
}
func (s FlatpakScanner) Install(name, _ string) error {
	return s.InstallCmd(name, "").Run()
}
func (s FlatpakScanner) UninstallCmd(name, _ string) *exec.Cmd {
	return exec.Command("flatpak", "uninstall", "-y", name)
}
func (s FlatpakScanner) InstallCmd(name, _ string) *exec.Cmd {
	return exec.Command("flatpak", "install", "-y", name)
}

var _ Scanner = FlatpakScanner{}
