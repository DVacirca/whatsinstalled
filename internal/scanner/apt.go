package scanner

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"installr/internal/pkg"
	"installr/internal/store"
)

// AptScanner scans manually installed dpkg packages only.
type AptScanner struct{}

func (AptScanner) Name() string { return "apt" }

func (s AptScanner) Scan() ([]store.Package, error) {
	// Get list of manually installed packages from apt-mark
	manualCmd := exec.Command("apt-mark", "showmanual")
	manualOut, err := manualCmd.Output()
	if err != nil {
		// Fallback: if apt-mark fails, include all packages
		return s.scanAll()
	}

	manual := make(map[string]bool)
	scanner := bufio.NewScanner(bytes.NewReader(manualOut))
	for scanner.Scan() {
		manual[strings.TrimSpace(scanner.Text())] = true
	}

	// Get package details from dpkg-query
	cmd := exec.Command("dpkg-query", "-W", "-f=${Package}\t${Version}\t${Installed-Size}\t${Status}\t${Auto-Installed}\t${Description}\n")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("dpkg-query: %s", exitErr.Stderr)
		}
		return nil, err
	}

	var pkgs []store.Package
	scanner = bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 4 {
			continue
		}
		status := fields[3]
		if !strings.Contains(status, "install ok installed") {
			continue
		}
		name := fields[0]
		if !manual[name] {
			continue
		}

		p := store.Package{
			Name:     name,
			Version:  fields[1],
			Source:   "apt",
			Location: "system",
			User:     "system",
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
		p.LastUsed = pkg.GetLastUsed(filepath.Join("/var/lib/dpkg/info", name+".list"))
		pkgs = append(pkgs, p)
	}
	return pkgs, scanner.Err()
}

// scanAll is a fallback that returns all installed apt packages.
func (s AptScanner) scanAll() ([]store.Package, error) {
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
			Location: "system",
			User:     "system",
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

func (s AptScanner) Uninstall(name, _ string) error {
	return s.UninstallCmd(name, "").Run()
}

func (s AptScanner) Install(name, _ string) error {
	return s.InstallCmd(name, "").Run()
}

func (s AptScanner) UninstallCmd(name, _ string) *exec.Cmd {
	cmd := exec.Command("sudo", "apt-get", "remove", "-y", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func (s AptScanner) InstallCmd(name, _ string) *exec.Cmd {
	cmd := exec.Command("sudo", "apt-get", "install", "-y", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

var _ Scanner = AptScanner{}
