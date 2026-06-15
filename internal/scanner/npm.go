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

// npmGlobalDir returns the global npm packages directory.
func npmGlobalDir() string {
	cmd := exec.Command("npm", "root", "-g")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// NpmScanner scans top-level npm packages only (global + local projects).
type NpmScanner struct{}

func (NpmScanner) Name() string      { return "npm" }
func (NpmScanner) IsAvailable() bool { return commandExists("npm") }
func (s NpmScanner) Probe() bool {
	out, _ := exec.Command("npm", "list", "-g", "--depth=0", "--json").Output()
	return len(out) > 10 // minimal JSON object
}

func (s NpmScanner) Scan() ([]store.Package, error) {
	var pkgs []store.Package

	// Global
	global, err := s.scanLocation("global", "", true)
	if err == nil {
		pkgs = append(pkgs, global...)
	}

	// Local envs: find package.json in ~/* depth 1
	home := pkg.HomeDir()
	if home != "" {
		entries, err := os.ReadDir(home)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				path := filepath.Join(home, entry.Name())
				if _, err := os.Stat(filepath.Join(path, "package.json")); err == nil {
					local, err := s.scanLocation(path, path, false)
					if err == nil {
						pkgs = append(pkgs, local...)
					}
				}
			}
		}
	}

	// Also scan CWD if it has a package.json
	cwd := pkg.CWD()
	if cwd != "" {
		if _, err := os.Stat(filepath.Join(cwd, "package.json")); err == nil {
			local, err := s.scanLocation(cwd, cwd, false)
			if err == nil {
				pkgs = append(pkgs, local...)
			}
		}
	}

	return pkgs, nil
}

func (s NpmScanner) scanLocation(location, dir string, global bool) ([]store.Package, error) {
	args := []string{"list", "--depth=0", "--json"}
	if global {
		args = append(args, "-g")
		location = "system"
	}
	cmd := exec.Command("npm", args...)
	if !global && dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.Output()
	if err != nil {
		if len(out) == 0 {
			return nil, fmt.Errorf("npm list: %w", err)
		}
	}

	var root npmListRoot
	if err := json.Unmarshal(out, &root); err != nil {
		return nil, fmt.Errorf("parse npm list: %w", err)
	}

	var pkgs []store.Package
	deps := root.Dependencies
	if deps == nil {
		deps = root.DevDependencies
	}
	owner := pkg.CurrentUser()
	if !global && dir != "" {
		owner = pkg.FileOwner(dir)
	}
	for name, info := range deps {
		p := store.Package{
			Name:      name,
			Version:   strings.TrimPrefix(info.Version, "^"),
			Source:    "npm",
			Location:  location,
			UpdatedAt: time.Now(),
			User:      owner,
		}
		if info.Description != "" {
			p.Description = info.Description
		}
		// Fallback: read package.json from node_modules
		if p.Description == "" {
			var pkgDir string
			if global {
				pkgDir = filepath.Join(npmGlobalDir(), name)
			} else if dir != "" {
				pkgDir = filepath.Join(dir, "node_modules", name)
			}
			if pkgDir != "" {
				p.Description = s.readPackageJSONDesc(pkgDir)
			}
		}
		// Determine package directory for last-used
		var pkgDir string
		if global {
			pkgDir = filepath.Join(npmGlobalDir(), name)
		} else if dir != "" {
			pkgDir = filepath.Join(dir, "node_modules", name)
		}
		if pkgDir != "" {
			p.LastUsed = pkg.GetLastUsed(pkgDir)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

func (s NpmScanner) Uninstall(name, location string) error {
	return s.UninstallCmd(name, location).Run()
}

func (s NpmScanner) Install(name, location string) error {
	return s.InstallCmd(name, location).Run()
}

func (s NpmScanner) UninstallCmd(name, location string) *exec.Cmd {
	args := []string{"uninstall", name}
	if location == "system" || location == "global" {
		args = append(args, "-g")
	}
	cmd := exec.Command("npm", args...)
	if location != "system" && location != "global" {
		cmd.Dir = location
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func (s NpmScanner) InstallCmd(name, location string) *exec.Cmd {
	args := []string{"install", name}
	if location == "system" || location == "global" {
		args = append(args, "-g")
	}
	cmd := exec.Command("npm", args...)
	if location != "system" && location != "global" {
		cmd.Dir = location
	}
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

// readPackageJSONDesc reads the "description" field from a package's package.json.
func (s NpmScanner) readPackageJSONDesc(pkgDir string) string {
	path := filepath.Join(pkgDir, "package.json")
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	var data struct {
		Description string `json:"description"`
	}
	if err := json.NewDecoder(f).Decode(&data); err != nil {
		return ""
	}
	return data.Description
}

type npmListRoot struct {
	Name            string                `json:"name"`
	Version         string                `json:"version"`
	Dependencies    map[string]npmDepInfo `json:"dependencies"`
	DevDependencies map[string]npmDepInfo `json:"devDependencies"`
}

type npmDepInfo struct {
	Version     string `json:"version"`
	Description string `json:"_description,omitempty"`
}

var _ Scanner = NpmScanner{}
