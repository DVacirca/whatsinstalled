package scanner

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"installr/internal/pkg"
	"installr/internal/store"
)

// BinScanner scans manually installed binaries in user bin directories.
type BinScanner struct{}

func (BinScanner) Name() string { return "bin" }

func (s BinScanner) Scan() ([]store.Package, error) {
	var pkgs []store.Package

	dirs := s.discoverDirs()
	seen := make(map[string]bool)

	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			info, err := entry.Info()
			if err != nil {
				continue
			}
			// Skip if not executable
			if info.Mode()&0o111 == 0 {
				continue
			}
			name := entry.Name()
			key := dir + "/" + name
			if seen[key] {
				continue
			}
			seen[key] = true

			owner := pkg.FileOwner(dir)
			if owner == "" {
				owner = pkg.CurrentUser()
			}

			p := store.Package{
				Name:      name,
				Source:    "bin",
				Location:  dir,
				UpdatedAt: time.Now(),
				User:      owner,
			}
			sz := info.Size()
			p.SizeBytes = &sz
			p.LastUsed = pkg.GetLastUsed(filepath.Join(dir, name))
			pkgs = append(pkgs, p)
		}
	}

	return pkgs, nil
}

func (s BinScanner) discoverDirs() []string {
	var dirs []string
	seen := make(map[string]bool)

	// Common user bin directories
	candidates := []string{
		"~/.local/bin",
		"~/bin",
		"~/go/bin",
		"~/.cargo/bin",
		"~/.yarn/bin",
		"~/.npm-global/bin",
		"~/.nvm/versions/node/*/bin",
		"~/.rbenv/shims",
		"~/.pyenv/shims",
		"/usr/local/bin",
		"/usr/bin",
	}

	home := pkg.HomeDir()
	for _, c := range candidates {
		if strings.HasPrefix(c, "~/") {
			c = filepath.Join(home, strings.TrimPrefix(c, "~/"))
		}
		if strings.Contains(c, "*") {
			matches, _ := filepath.Glob(c)
			for _, m := range matches {
				if !seen[m] {
					seen[m] = true
					if info, err := os.Stat(m); err == nil && info.IsDir() {
						dirs = append(dirs, m)
					}
				}
			}
			continue
		}
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			if !seen[c] {
				seen[c] = true
				dirs = append(dirs, c)
			}
		}
	}

	// Also scan PATH directories under home (to catch custom dirs)
	path := os.Getenv("PATH")
	for _, dir := range strings.Split(path, ":") {
		if dir == "" {
			continue
		}
		if !strings.HasPrefix(dir, home) {
			continue // Skip system dirs covered by apt/snap
		}
		if strings.Contains(dir, "apt") || strings.Contains(dir, "snap") {
			continue // Skip package manager dirs
		}
		if !seen[dir] {
			if info, err := os.Stat(dir); err == nil && info.IsDir() {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
		}
	}

	return dirs
}

func (s BinScanner) Uninstall(name, location string) error {
	return os.Remove(filepath.Join(location, name))
}

func (s BinScanner) Install(name, location string) error {
	return fmt.Errorf("installing manual binaries is not supported; use curl or wget to download to %s", location)
}

func (s BinScanner) UninstallCmd(name, location string) *exec.Cmd {
	cmd := exec.Command("rm", filepath.Join(location, name))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func (s BinScanner) InstallCmd(name, location string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", fmt.Sprintf("echo 'Cannot install manual binaries automatically. Download to %s manually.'", location))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

var _ Scanner = BinScanner{}
