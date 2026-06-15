package scanner

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// BinScanner scans manually installed binaries in user bin directories.
type BinScanner struct{}

func (BinScanner) Name() string      { return "bin" }
func (BinScanner) IsAvailable() bool { return true }
func (s BinScanner) Probe() bool {
	dirs := s.discoverDirs()
	for _, dir := range dirs {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			info, _ := e.Info()
			if info != nil && info.Mode()&0o111 != 0 {
				return true
			}
		}
	}
	return false
}

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
			mt := info.ModTime()
			p.AddedAt = &mt
			pkgs = append(pkgs, p)
		}
	}

	// Post-process: enrich descriptions
	s.enrichDescriptions(pkgs)

	return pkgs, nil
}

// enrichDescriptions populates descriptions for bin packages.
// Priority: 1) whatis, 2) directory hint, 3) --help first line.
func (s BinScanner) enrichDescriptions(pkgs []store.Package) {
	// Collect unique names for whatis batch
	names := make([]string, 0, len(pkgs))
	nameIdx := make(map[string][]int)
	for i := range pkgs {
		name := pkgs[i].Name
		nameIdx[name] = append(nameIdx[name], i)
		if len(nameIdx[name]) == 1 {
			names = append(names, name)
		}
	}

	// 1. Try whatis batch
	whatisMap := s.whatisBatch(names)
	for name, desc := range whatisMap {
		for _, idx := range nameIdx[name] {
			if pkgs[idx].Description == "" {
				pkgs[idx].Description = desc
			}
		}
	}

	// 2. Directory hints for remaining
	for i := range pkgs {
		if pkgs[i].Description != "" {
			continue
		}
		dir := filepath.Base(pkgs[i].Location)
		switch dir {
		case "go":
			pkgs[i].Description = "Go binary tool"
		case "cargo":
			pkgs[i].Description = "Rust binary tool"
		case "node_modules":
			pkgs[i].Description = "Node.js binary tool"
		case "shims":
			if strings.Contains(pkgs[i].Location, ".pyenv") {
				pkgs[i].Description = "Python version manager shim"
			} else if strings.Contains(pkgs[i].Location, ".rbenv") {
				pkgs[i].Description = "Ruby version manager shim"
			}
		case ".nvm":
			pkgs[i].Description = "Node.js version manager shim"
		}
	}

}

// whatisBatch runs whatis for multiple names and returns name -> description.
func (s BinScanner) whatisBatch(names []string) map[string]string {
	results := make(map[string]string)
	if len(names) == 0 {
		return results
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "whatis", names...)
	out, err := cmd.Output()
	if err != nil {
		return results
	}

	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.SplitN(line, " - ", 2)
		if len(parts) != 2 {
			continue
		}
		namePart := parts[0]
		idx := strings.Index(namePart, " (")
		if idx < 0 {
			continue
		}
		name := strings.TrimSpace(namePart[:idx])
		desc := strings.TrimSpace(parts[1])
		if desc != "" && desc != "nothing appropriate." {
			results[name] = desc
		}
	}
	return results
}

// helpFallback tries --help for a small sample of packages without descriptions.
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

var _ Scanner = BinScanner{}
