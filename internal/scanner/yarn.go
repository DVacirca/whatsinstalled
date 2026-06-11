package scanner

import (
	"os/exec"
	"strings"
	"time"

	"installr/internal/pkg"
	"installr/internal/store"
)

// YarnScanner scans globally installed yarn (v1) packages.
type YarnScanner struct{}

func (YarnScanner) Name() string      { return "yarn" }
func (YarnScanner) IsAvailable() bool { return commandExists("yarn") }
func (s YarnScanner) Probe() bool {
	out, _ := exec.Command("yarn", "global", "list").Output()
	return strings.Contains(string(out), "info \"")
}

func (s YarnScanner) Scan() ([]store.Package, error) {
	out, err := exec.Command("yarn", "global", "list").Output()
	if err != nil {
		return nil, nil
	}
	return parseYarnGlobalList(string(out)), nil
}

// parseYarnGlobalList parses `yarn global list` output. Package lines look
// like: info "left-pad@1.3.0" has binaries:
func parseYarnGlobalList(out string) []store.Package {
	var pkgs []store.Package
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		const prefix = `info "`
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		rest := line[len(prefix):]
		end := strings.Index(rest, `"`)
		if end < 0 {
			continue
		}
		spec := rest[:end] // e.g. left-pad@1.3.0 or @scope/pkg@1.0.0
		at := strings.LastIndex(spec, "@")
		if at <= 0 {
			continue
		}
		pkgs = append(pkgs, store.Package{
			Name:      spec[:at],
			Version:   spec[at+1:],
			Source:    "yarn",
			Location:  "global",
			UpdatedAt: time.Now(),
			User:      pkg.CurrentUser(),
		})
	}
	return pkgs
}

func (s YarnScanner) Uninstall(name, _ string) error {
	return s.UninstallCmd(name, "").Run()
}
func (s YarnScanner) Install(name, _ string) error {
	return s.InstallCmd(name, "").Run()
}
func (s YarnScanner) UninstallCmd(name, _ string) *exec.Cmd {
	return exec.Command("yarn", "global", "remove", name)
}
func (s YarnScanner) InstallCmd(name, _ string) *exec.Cmd {
	return exec.Command("yarn", "global", "add", name)
}

var _ Scanner = YarnScanner{}
