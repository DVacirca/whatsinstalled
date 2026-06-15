package scanner

import (
	"bufio"
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// AptScanner scans all installed dpkg packages (manual + auto dependencies).
type AptScanner struct{}

func (AptScanner) Name() string      { return "apt" }
func (AptScanner) IsAvailable() bool { return commandExists("dpkg-query") }
func (s AptScanner) Probe() bool {
	out, _ := exec.Command("dpkg-query", "-W", "-f=${Package}\n").Output()
	return len(out) > 0
}

func (s AptScanner) Scan() ([]store.Package, error) {
	return s.scanAll()
}

// manualAptPackages returns the set of packages a user explicitly installed
// (not pulled in automatically as a dependency). apt tracks this in
// /var/lib/apt/extended_states, surfaced via `apt-mark showmanual` — the
// dpkg-query ${Auto-Installed} field is unreliable for this.
func manualAptPackages() map[string]bool {
	manual := map[string]bool{}
	out, err := exec.Command("apt-mark", "showmanual").Output()
	if err != nil {
		return manual
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if name := strings.TrimSpace(sc.Text()); name != "" {
			manual[name] = true
		}
	}
	return manual
}

// scanAll is a fallback that returns all installed apt packages.
func (s AptScanner) scanAll() ([]store.Package, error) {
	manual := manualAptPackages()
	cmd := exec.Command("dpkg-query", "-W", "-f=${Package}\t${Version}\t${Installed-Size}\t${Status}\t${Auto-Installed}\t${Description}\n")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("dpkg-query: %s", exitErr.Stderr)
		}
		return nil, err
	}

	var pkgs []store.Package
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 4 {
			continue
		}
		status := fields[3]
		if !strings.Contains(status, "install ok installed") {
			continue
		}

		p := store.Package{
			Name:     fields[0],
			Version:  fields[1],
			Source:   "apt",
			Location: "/var/lib/dpkg",
			User:     pkg.FileOwner("/var/lib/dpkg"),
		}
		// Auto-installed = present in dpkg but not explicitly chosen by the
		// user. Only trust this when apt-mark gave us a manual set.
		if len(manual) > 0 {
			p.AutoInstalled = !manual[fields[0]]
		}
		if len(fields) > 2 && fields[2] != "" {
			var sz int64
			fmt.Sscanf(fields[2], "%d", &sz)
			sz *= 1024
			p.SizeBytes = &sz
		}
		if len(fields) > 5 {
			p.Description = strings.TrimSpace(fields[5])
		}
		p.UpdatedAt = time.Now()
		p.LastUsed = pkg.GetLastUsed(filepath.Join("/var/lib/dpkg/info", fields[0]+".list"))
		pkgs = append(pkgs, p)
	}
	return pkgs, scanner.Err()
}

var _ Scanner = AptScanner{}
