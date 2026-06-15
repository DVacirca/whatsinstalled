package scanner

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// GoScanner scans downloaded Go modules in the module cache.
type GoScanner struct{}

func (GoScanner) Name() string      { return "go" }
func (GoScanner) IsAvailable() bool { return commandExists("go") }
func (s GoScanner) Probe() bool {
	modCache := filepath.Join(pkg.HomeDir(), "go", "pkg", "mod")
	entries, _ := os.ReadDir(modCache)
	for _, e := range entries {
		if e.IsDir() && strings.Contains(e.Name(), "@") {
			return true
		}
	}
	return false
}

func (s GoScanner) Scan() ([]store.Package, error) {
	modCache := filepath.Join(pkg.HomeDir(), "go", "pkg", "mod")
	if _, err := os.Stat(modCache); err != nil {
		return nil, nil // no go module cache
	}

	// Use go list -m all in any go module project, or scan the cache directly.
	// Scanning the cache is more reliable — list every module directory.
	var pkgs []store.Package

	// Walk the module cache. Directories with @ are module versions.
	// e.g. github.com/foo/bar@v1.2.3
	_ = filepath.WalkDir(modCache, func(path string, d os.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.Contains(name, "@") {
			return nil // still inside a domain/org directory
		}

		// path is like .../mod/github.com/foo/bar@v1.2.3
		// Extract module path and version from directory name
		parts := strings.SplitN(name, "@", 2)
		if len(parts) != 2 {
			return nil
		}
		version := parts[1]

		// Reconstruct module path from parent directories
		rel, _ := filepath.Rel(modCache, path)
		modPath := filepath.Dir(rel)
		modPath = strings.ReplaceAll(modPath, string(filepath.Separator), "/")

		p := store.Package{
			Name:        modPath,
			Version:     version,
			Source:      "go",
			Location:    "gomodcache",
			UpdatedAt:   time.Now(),
			User:        pkg.FileOwner(path),
			Description: "",
		}
		pkgs = append(pkgs, p)
		return filepath.SkipDir // don't recurse into module contents
	})

	return pkgs, nil
}

var _ Scanner = GoScanner{}
