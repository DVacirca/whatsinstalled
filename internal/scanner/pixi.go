package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// PixiScanner scans pixi global environments.
type PixiScanner struct{}

func (PixiScanner) Name() string      { return "pixi" }
func (PixiScanner) IsAvailable() bool { return commandExists("pixi") }
func (s PixiScanner) Probe() bool {
	envDir := filepath.Join(pkg.HomeDir(), ".pixi", "envs")
	entries, _ := os.ReadDir(envDir)
	for _, e := range entries {
		if e.IsDir() {
			return true
		}
	}
	return false
}

func (s PixiScanner) Scan() ([]store.Package, error) {
	var pkgs []store.Package

	// Global envs under ~/.pixi/envs
	envDir := filepath.Join(pkg.HomeDir(), ".pixi", "envs")
	entries, err := os.ReadDir(envDir)
	if err != nil {
		// No pixi envs is not an error
		return pkgs, nil
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		envPath := filepath.Join(envDir, entry.Name())
		envPkgs, err := s.scanEnv(envPath, entry.Name())
		if err != nil {
			continue
		}
		pkgs = append(pkgs, envPkgs...)
	}

	// Also scan local project pixi.toml files in ~/projects depth 2
	home := pkg.HomeDir()
	if home != "" {
		_ = filepath.WalkDir(home, func(path string, d os.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			if d.Name() == "pixi.toml" {
				projectDir := filepath.Dir(path)
				rel, _ := filepath.Rel(home, projectDir)
				if rel == "" {
					rel = projectDir
				}
				// Only go depth 2 from home to avoid crawling everything
				depth := strings.Count(rel, string(filepath.Separator))
				if depth <= 2 {
					if envPkgs, err := s.scanEnv(projectDir, rel); err == nil {
						pkgs = append(pkgs, envPkgs...)
					}
				}
			}
			return nil
		})
	}

	return pkgs, nil
}

func (s PixiScanner) scanEnv(envPath, envName string) ([]store.Package, error) {
	cmd := exec.Command("pixi", "list", "--manifest-path", filepath.Join(envPath, "pixi.toml"), "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pixi list: %w", err)
	}

	var raw []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse pixi list: %w", err)
	}

	owner := pkg.FileOwner(envPath)
	var pkgs []store.Package
	for _, r := range raw {
		p := store.Package{
			Name:        r.Name,
			Version:     r.Version,
			Source:      "pixi",
			Location:    envName,
			UpdatedAt:   time.Now(),
			User:        owner,
			Description: r.Channel,
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

var _ Scanner = PixiScanner{}
