package scanner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// uvToolsDir returns the directory holding uv's installed tools.
func uvToolsDir() string {
	return filepath.Join(pkg.HomeDir(), ".local", "share", "uv", "tools")
}

// UvScanner scans CLI tools installed via `uv tool install`.
type UvScanner struct{}

func (UvScanner) Name() string      { return "uv" }
func (UvScanner) IsAvailable() bool { return commandExists("uv") }
func (s UvScanner) Probe() bool {
	entries, _ := os.ReadDir(uvToolsDir())
	return len(entries) > 0
}

func (s UvScanner) Scan() ([]store.Package, error) {
	out, err := exec.Command("uv", "tool", "list").Output()
	if err != nil {
		return nil, nil
	}
	return parseUvToolList(string(out)), nil
}

// parseUvToolList parses `uv tool list` output. Tool lines look like
// "pytest v9.0.2"; the binaries each tool provides are indented "- name"
// lines that we skip.
func parseUvToolList(out string) []store.Package {
	var pkgs []store.Package
	for _, line := range strings.Split(out, "\n") {
		// Skip blank lines and the indented "- app" binary lines.
		if line == "" || line != strings.TrimLeft(line, " \t-") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		version := strings.TrimPrefix(fields[1], "v")
		location := filepath.Join(uvToolsDir(), name)
		pkgs = append(pkgs, store.Package{
			Name:      name,
			Version:   version,
			Source:    "uv",
			Location:  location,
			UpdatedAt: time.Now(),
			User:      pkg.CurrentUser(),
			LastUsed:  pkg.GetLastUsed(location),
			SizeBytes: pkg.PathSize(location),
			AddedAt:   pkg.GetModTime(location),
		})
	}
	return pkgs
}

func (s UvScanner) Uninstall(name, _ string) error {
	return s.UninstallCmd(name, "").Run()
}
func (s UvScanner) Install(name, _ string) error {
	return s.InstallCmd(name, "").Run()
}
func (s UvScanner) UninstallCmd(name, _ string) *exec.Cmd {
	return exec.Command("uv", "tool", "uninstall", name)
}
func (s UvScanner) InstallCmd(name, _ string) *exec.Cmd {
	return exec.Command("uv", "tool", "install", name)
}

var _ Scanner = UvScanner{}
