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
	dirs := discoverBinDirs()
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

// binCandidate is one executable found during the directory walk, kept until
// package-manager ownership has been resolved.
type binCandidate struct {
	dir, name, path string
	info            os.FileInfo
}

func (s BinScanner) Scan() ([]store.Package, error) {
	cands := collectBinaries(discoverBinDirs())

	paths := make([]string, len(cands))
	for i, c := range cands {
		paths[i] = c.path
	}
	managed := managedSet(paths)

	var pkgs []store.Package
	for _, c := range cands {
		if managed[c.path] {
			continue // owned by a package manager
		}
		pkgs = append(pkgs, c.toPackage())
	}

	s.enrichDescriptions(pkgs) // whatis / directory hints
	s.enrichLastUsed(pkgs)     // shell-history invocation times
	return pkgs, nil
}

// collectBinaries walks the given directories and returns every executable that
// is not a shared library, de-duplicated by path.
func collectBinaries(dirs []string) []binCandidate {
	var cands []binCandidate
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
			if err != nil || info.Mode()&0o111 == 0 {
				continue // not executable
			}
			if isLibraryFile(entry.Name()) {
				continue // shared library / object file, not a command
			}
			path := filepath.Join(dir, entry.Name())
			if seen[path] {
				continue
			}
			seen[path] = true
			cands = append(cands, binCandidate{dir: dir, name: entry.Name(), path: path, info: info})
		}
	}
	return cands
}

// toPackage builds the store.Package for an unmanaged binary.
func (c binCandidate) toPackage() store.Package {
	owner := pkg.FileOwner(c.dir)
	if owner == "" {
		owner = pkg.CurrentUser()
	}
	source := "bin"
	if strings.Contains(c.dir, ".nvm") {
		source = "npm"
	}
	sz := c.info.Size()
	mt := c.info.ModTime()
	return store.Package{
		Name:      c.name,
		Source:    source,
		Location:  c.dir,
		UpdatedAt: time.Now(),
		User:      owner,
		SizeBytes: &sz,
		LastUsed:  pkg.GetLastUsed(c.path),
		AddedAt:   &mt,
	}
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

// enrichLastUsed updates LastUsed for bin packages using shell history, which
// gives reliable command-invocation timestamps unlike filesystem atime.
func (s BinScanner) enrichLastUsed(pkgs []store.Package) {
	cmdTimes := pkg.ShellCommandTimes()
	if len(cmdTimes) == 0 {
		return
	}
	for i := range pkgs {
		if t, ok := cmdTimes[pkgs[i].Name]; ok {
			if pkgs[i].LastUsed == nil || t.After(*pkgs[i].LastUsed) {
				pkgs[i].LastUsed = &t
			}
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

var _ Scanner = BinScanner{}
