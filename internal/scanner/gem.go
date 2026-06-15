package scanner

import (
	"os/exec"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// GemScanner scans locally installed RubyGems.
type GemScanner struct{}

func (GemScanner) Name() string      { return "gem" }
func (GemScanner) IsAvailable() bool { return commandExists("gem") }
func (s GemScanner) Probe() bool {
	out, _ := exec.Command("gem", "list", "--local").Output()
	return len(out) > 0
}

func (s GemScanner) Scan() ([]store.Package, error) {
	out, err := exec.Command("gem", "list", "--local").Output()
	if err != nil {
		return nil, nil
	}
	location := "system"
	if dir, err := exec.Command("gem", "environment", "gemdir").Output(); err == nil {
		location = strings.TrimSpace(string(dir))
	}
	return parseGemList(string(out), location), nil
}

// parseGemList parses `gem list --local` output. Each line looks like
// "rake (13.0.6)", "csv (default: 3.2.6)", or "json (2.6.3, 2.5.1)".
// We take the gem name and its newest (first-listed) version.
func parseGemList(out, location string) []store.Package {
	var pkgs []store.Package
	for _, line := range strings.Split(out, "\n") {
		openIdx := strings.Index(line, "(")
		closeIdx := strings.LastIndex(line, ")")
		if openIdx < 1 || closeIdx <= openIdx {
			continue
		}
		name := strings.TrimSpace(line[:openIdx])
		versions := line[openIdx+1 : closeIdx]
		versions = strings.TrimPrefix(versions, "default: ")
		version := strings.TrimSpace(strings.SplitN(versions, ",", 2)[0])
		pkgs = append(pkgs, store.Package{
			Name:      name,
			Version:   version,
			Source:    "gem",
			Location:  location,
			UpdatedAt: time.Now(),
			User:      pkg.CurrentUser(),
		})
	}
	return pkgs
}

var _ Scanner = GemScanner{}
