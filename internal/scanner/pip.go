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

	"installr/internal/pkg"
	"installr/internal/store"
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

	// Local venvs: find .venv/ venv/ env/ in ~/* depth 1
	home := pkg.HomeDir()
	if home != "" {
		entries, err := os.ReadDir(home)
		if err == nil {
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				path := filepath.Join(home, entry.Name())
				for _, venvName := range []string{".venv", "venv", "env"} {
					venvPath := filepath.Join(path, venvName)
					pipBin := filepath.Join(venvPath, "bin", "pip")
					if _, err := os.Stat(pipBin); err == nil {
						local, err := s.scanWithPip(pipBin, path)
						if err == nil {
							pkgs = append(pkgs, local...)
						}
					}
				}
			}
		}
	}

	// Also scan CWD venv
	cwd := pkg.CWD()
	if cwd != "" {
		for _, venvName := range []string{".venv", "venv", "env"} {
			venvPath := filepath.Join(cwd, venvName)
			pipBin := filepath.Join(venvPath, "bin", "pip")
			if _, err := os.Stat(pipBin); err == nil {
				local, err := s.scanWithPip(pipBin, cwd)
				if err == nil {
					pkgs = append(pkgs, local...)
				}
			}
		}
	}

	return pkgs, nil
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

	// Fetch descriptions in one bulk pip show call
	descMap := s.pipShowDescriptions(pipBin, raw)

	owner := "system"
	if location != "system" {
		owner = pkg.FileOwner(location)
	}
	var pkgs []store.Package
	for _, r := range raw {
		p := store.Package{
			Name:        r.Name,
			Version:     r.Version,
			Source:      "pip",
			Location:    location,
			UpdatedAt:   time.Now(),
			User:        owner,
			Description: descMap[r.Name],
		}
		// Determine package directory for last-used
		var pkgDir string
		if location == "system" {
			pkgDir = filepath.Join("/usr/local/lib", "python3", "dist-packages", r.Name)
		} else {
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
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

// pipShowDescriptions runs pip show for all packages and extracts Summary fields.
func (s PipScanner) pipShowDescriptions(pipBin string, raw []struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}) map[string]string {
	descMap := make(map[string]string)
	if len(raw) == 0 {
		return descMap
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
		return descMap
	}

	var currentName, currentDesc string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Name: ") {
			if currentName != "" && currentDesc != "" {
				descMap[currentName] = currentDesc
			}
			currentName = strings.TrimSpace(strings.TrimPrefix(line, "Name: "))
			currentDesc = ""
		} else if strings.HasPrefix(line, "Summary: ") {
			currentDesc = strings.TrimSpace(strings.TrimPrefix(line, "Summary: "))
		}
	}
	if currentName != "" && currentDesc != "" {
		descMap[currentName] = currentDesc
	}

	return descMap
}

func (s PipScanner) Uninstall(name, location string) error {
	return s.UninstallCmd(name, location).Run()
}

func (s PipScanner) Install(name, location string) error {
	return s.InstallCmd(name, location).Run()
}

func (s PipScanner) UninstallCmd(name, location string) *exec.Cmd {
	pipBin := "pip"
	if location != "system" {
		for _, venvName := range []string{".venv", "venv", "env"} {
			candidate := filepath.Join(location, venvName, "bin", "pip")
			if _, err := os.Stat(candidate); err == nil {
				pipBin = candidate
				break
			}
		}
	}
	cmd := exec.Command(pipBin, "uninstall", "-y", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func (s PipScanner) InstallCmd(name, location string) *exec.Cmd {
	pipBin := "pip"
	if location != "system" {
		for _, venvName := range []string{".venv", "venv", "env"} {
			candidate := filepath.Join(location, venvName, "bin", "pip")
			if _, err := os.Stat(candidate); err == nil {
				pipBin = candidate
				break
			}
		}
	}
	cmd := exec.Command(pipBin, "install", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func dirSize(path string) int64 {
	var size int64
	filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size
}

var _ Scanner = PipScanner{}
