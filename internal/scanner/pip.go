package scanner

import (
	"bufio"
	"bytes"
	"context"
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

// PipScanner scans top-level pip packages (global + local venvs).
type PipScanner struct{}

func (PipScanner) Name() string      { return "pip" }
func (PipScanner) IsAvailable() bool { return commandExists("pip") || commandExists("pip3") }
func (s PipScanner) Probe() bool {
	out, _ := exec.Command("pip", "list", "--format=json").Output()
	return len(out) > 10
}

func (s PipScanner) Scan() ([]store.Package, error) {
	var pkgs []store.Package

	// Global (system Python)
	global, err := s.scanWithPip("pip", "system")
	if err == nil {
		pkgs = append(pkgs, global...)
	}

	// Local virtualenvs under ~/* (depth 1-2). These directories may be
	// attacker-controlled (cloned repos, unpacked archives), so we read each
	// venv's installed-package inventory from its on-disk metadata and never
	// execute a `pip` binary found inside them — doing so would be arbitrary
	// code execution. The current working directory is deliberately NOT scanned
	// for the same reason (`cd untrusted-repo && whatsinstalled`).
	home := pkg.HomeDir()
	if home != "" {
		if entries, err := os.ReadDir(home); err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				path := filepath.Join(home, entry.Name())
				src := sourceForProject(path)
				for _, venvName := range []string{".venv", "venv", "env"} {
					pkgs = append(pkgs, s.scanVenvMetadata(filepath.Join(path, venvName), path, src)...)
				}
				// Depth 2: ~/projects/myapp/.venv
				if subEntries, err := os.ReadDir(path); err == nil {
					for _, sub := range subEntries {
						if !sub.IsDir() {
							continue
						}
						subPath := filepath.Join(path, sub.Name())
						src := sourceForProject(subPath)
						for _, venvName := range []string{".venv", "venv", "env"} {
							pkgs = append(pkgs, s.scanVenvMetadata(filepath.Join(subPath, venvName), subPath, src)...)
						}
					}
				}
			}
		}
	}

	return pkgs, nil
}

// scanVenvMetadata reads a virtualenv's installed packages from the
// site-packages *.dist-info/METADATA (and *.egg-info/PKG-INFO) files on disk,
// without executing anything from the venv. Returns nil if the path is not a
// venv. This is the safe replacement for invoking the venv's own `pip`.
func (s PipScanner) scanVenvMetadata(venvPath, location, source string) []store.Package {
	siteDir := venvSitePackages(venvPath)
	if siteDir == "" {
		return nil
	}
	entries, err := os.ReadDir(siteDir)
	if err != nil {
		return nil
	}
	owner := pkg.FileOwner(location)
	var pkgs []store.Package
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		var metaPath string
		switch {
		case strings.HasSuffix(e.Name(), ".dist-info"):
			metaPath = filepath.Join(siteDir, e.Name(), "METADATA")
		case strings.HasSuffix(e.Name(), ".egg-info"):
			metaPath = filepath.Join(siteDir, e.Name(), "PKG-INFO")
		default:
			continue
		}
		name, version, summary := parsePyMetadata(metaPath)
		if name == "" {
			continue
		}
		p := store.Package{
			Name:        name,
			Version:     version,
			Source:      source,
			Location:    location,
			UpdatedAt:   time.Now(),
			User:        owner,
			Description: summary,
			LastUsed:    pkg.GetLastUsed(filepath.Join(siteDir, e.Name())),
		}
		pkgs = append(pkgs, p)
	}
	return pkgs
}

// venvSitePackages returns the site-packages directory inside a venv
// (lib/python3*/site-packages), or "" if venvPath is not a virtualenv.
func venvSitePackages(venvPath string) string {
	libDir := filepath.Join(venvPath, "lib")
	entries, err := os.ReadDir(libDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "python3") {
			continue
		}
		sp := filepath.Join(libDir, e.Name(), "site-packages")
		if info, err := os.Stat(sp); err == nil && info.IsDir() {
			return sp
		}
	}
	return ""
}

// parsePyMetadata extracts Name, Version, and Summary from a Python package
// METADATA / PKG-INFO file (RFC822-style headers). Only the header block is
// read — parsing stops at the first blank line, before the long description.
func parsePyMetadata(path string) (name, version, summary string) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", ""
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			break // end of headers; the description follows
		}
		switch {
		case name == "" && strings.HasPrefix(line, "Name: "):
			name = strings.TrimSpace(line[len("Name: "):])
		case version == "" && strings.HasPrefix(line, "Version: "):
			version = strings.TrimSpace(line[len("Version: "):])
		case summary == "" && strings.HasPrefix(line, "Summary: "):
			summary = strings.TrimSpace(line[len("Summary: "):])
		}
	}
	return name, version, summary
}

func (s PipScanner) scanWithPip(pipBin, location string) ([]store.Package, error) {
	cmd := exec.Command(pipBin, "list", "--format=json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("pip list: %w", err)
	}

	var raw []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse pip list: %w", err)
	}

	// Fetch descriptions and dependency info in one bulk pip show call
	showResult := s.pipShowDescriptions(pipBin, raw)

	owner := "system"
	if location != "system" {
		owner = pkg.FileOwner(location)
	}
	var pkgs []store.Package
	for _, r := range raw {
		info := showResult[r.Name]
		p := store.Package{
			Name:          r.Name,
			Version:       r.Version,
			Source:        "pip",
			Location:      location,
			UpdatedAt:     time.Now(),
			User:          owner,
			Description:   info.Desc,
			AutoInstalled: info.IsDependency,
		}
		// Use the actual install path from pip show when available
		if info.Location != "" {
			p.Location = info.Location
		}
		// Determine package directory for last-used and size
		var pkgDir string
		if info.Location != "" {
			// Package might be name/ (directory) or name.py (single file)
			pkgDir = filepath.Join(info.Location, r.Name)
			if _, err := os.Stat(pkgDir); os.IsNotExist(err) {
				pyPath := filepath.Join(info.Location, r.Name+".py")
				if _, err := os.Stat(pyPath); err == nil {
					pkgDir = pyPath
				} else {
					pkgDir = ""
				}
			}
		} else if location != "system" {
			for _, venvName := range []string{".venv", "venv", "env"} {
				venvPath := filepath.Join(location, venvName)
				candidate := filepath.Join(venvPath, "lib")
				if entries, err := os.ReadDir(candidate); err == nil {
					for _, entry := range entries {
						if entry.IsDir() && strings.HasPrefix(entry.Name(), "python3") {
							pkgDir = filepath.Join(candidate, entry.Name(), "site-packages", r.Name)
							break
						}
					}
				}
				if pkgDir != "" {
					break
				}
			}
		}
		if pkgDir != "" {
			p.LastUsed = pkg.GetLastUsed(pkgDir)
			p.SizeBytes = pkg.PathSize(pkgDir)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// pipShowResult holds the description, dependency status, and install path of a pip package.
type pipShowResult struct {
	Desc         string
	IsDependency bool
	Location     string // e.g. /usr/lib/python3/dist-packages
}

// pipShowDescriptions runs pip show for all packages and extracts summary and dependency info.
func (s PipScanner) pipShowDescriptions(pipBin string, raw []struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}) map[string]pipShowResult {
	result := make(map[string]pipShowResult)
	if len(raw) == 0 {
		return result
	}

	names := make([]string, len(raw))
	for i, r := range raw {
		names[i] = r.Name
	}

	args := append([]string{"show"}, names...)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, pipBin, args...)
	out, err := cmd.Output()
	if err != nil {
		return result
	}

	var currentName, currentDesc, currentLocation string
	currentIsDep := false
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Name: ") {
			if currentName != "" {
				r := result[currentName]
				r.Desc = currentDesc
				r.IsDependency = currentIsDep
				r.Location = currentLocation
				result[currentName] = r
			}
			currentName = strings.TrimSpace(strings.TrimPrefix(line, "Name: "))
			currentDesc = ""
			currentIsDep = false
			currentLocation = ""
		} else if strings.HasPrefix(line, "Summary: ") {
			currentDesc = strings.TrimSpace(strings.TrimPrefix(line, "Summary: "))
		} else if strings.HasPrefix(line, "Required-by: ") {
			requiredBy := strings.TrimSpace(strings.TrimPrefix(line, "Required-by: "))
			currentIsDep = requiredBy != "" && requiredBy != "N/A"
		} else if strings.HasPrefix(line, "Location: ") {
			currentLocation = strings.TrimSpace(strings.TrimPrefix(line, "Location: "))
		}
	}
	if currentName != "" {
		r := result[currentName]
		r.Desc = currentDesc
		r.IsDependency = currentIsDep
		r.Location = currentLocation
		result[currentName] = r
	}

	return result
}

var _ Scanner = PipScanner{}

// sourceForProject returns "uv" if the project directory has a uv.lock file,
// indicating it's a uv-managed project. Otherwise returns "pip".
func sourceForProject(dir string) string {
	if _, err := os.Stat(filepath.Join(dir, "uv.lock")); err == nil {
		return "uv"
	}
	return "pip"
}
