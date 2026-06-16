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

// uvToolsDir returns the directory holding uv's installed tools, preferring
// uv's own answer and falling back to UV_TOOL_DIR / the platform data dir.
func uvToolsDir() string {
	if d := cmdLine("uv", "tool", "dir"); d != "" {
		return d
	}
	if d := os.Getenv("UV_TOOL_DIR"); d != "" {
		return d
	}
	return filepath.Join(userDataDir(), "uv", "tools")
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

var _ Scanner = UvScanner{}
