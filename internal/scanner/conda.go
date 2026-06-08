package scanner

import (
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

// CondaScanner scans conda environments and their packages.
type CondaScanner struct{}

func (CondaScanner) Name() string { return "conda" }

func (s CondaScanner) Scan() ([]store.Package, error) {
	var pkgs []store.Package

	// Use conda env list to discover environments
	cmd := exec.Command("conda", "env", "list", "--json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("conda env list: %w", err)
	}

	var envList struct {
		Envs []string `json:"envs"`
	}
	if err := json.Unmarshal(out, &envList); err != nil {
		return nil, fmt.Errorf("parse conda envs: %w", err)
	}

	for _, envPath := range envList.Envs {
		envPkgs, err := s.scanEnv(envPath)
		if err != nil {
			continue
		}
		pkgs = append(pkgs, envPkgs...)
	}

	return pkgs, nil
}

func (s CondaScanner) scanEnv(envPath string) ([]store.Package, error) {
	condaBin := "conda"
	// Prefer the conda in the base installation if envPath is not base
	baseConda := filepath.Join(pkg.HomeDir(), "miniconda3", "bin", "conda")
	if _, err := os.Stat(baseConda); err == nil {
		condaBin = baseConda
	}

	cmd := exec.Command(condaBin, "list", "--json", "-p", envPath)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("conda list for %s: %w", envPath, err)
	}

	var raw []struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Channel     string `json:"channel"`
		BuildString string `json:"build_string"`
		BaseURL     string `json:"base_url"`
	}
	if err := json.Unmarshal(out, &raw); err != nil {
		return nil, fmt.Errorf("parse conda list: %w", err)
	}

	// Determine location label
	location := envPath
	if envPath == filepath.Join(pkg.HomeDir(), "miniconda3") {
		location = "base"
	} else {
		location = filepath.Base(envPath)
	}

	// Get owner of the env directory
	owner := pkg.FileOwner(envPath)

	var pkgs []store.Package
	for _, r := range raw {
		p := store.Package{
			Name:      r.Name,
			Version:   r.Version,
			Source:    "conda",
			Location:  location,
			UpdatedAt: time.Now(),
			User:      owner,
		}
		if r.Channel != "" {
			p.Description = fmt.Sprintf("channel: %s", r.Channel)
		}
		// Determine package directory for last-used
		var pkgDir string
		libDir := filepath.Join(envPath, "lib")
		if entries, err := os.ReadDir(libDir); err == nil {
			for _, entry := range entries {
				if entry.IsDir() && strings.HasPrefix(entry.Name(), "python3") {
					pkgDir = filepath.Join(libDir, entry.Name(), "site-packages", r.Name)
					break
				}
			}
		}
		if pkgDir == "" {
			pkgDir = filepath.Join(envPath, "pkgs", r.Name+"-"+r.Version)
		}
		if pkgDir != "" {
			p.LastUsed = pkg.GetLastUsed(pkgDir)
		}
		pkgs = append(pkgs, p)
	}
	return pkgs, nil
}

func (s CondaScanner) Uninstall(name, location string) error {
	return s.UninstallCmd(name, location).Run()
}

func (s CondaScanner) Install(name, location string) error {
	return s.InstallCmd(name, location).Run()
}

func (s CondaScanner) UninstallCmd(name, location string) *exec.Cmd {
	condaBin := "conda"
	baseConda := filepath.Join(pkg.HomeDir(), "miniconda3", "bin", "conda")
	if _, err := os.Stat(baseConda); err == nil {
		condaBin = baseConda
	}

	envPath := s.resolveEnvPath(location)
	cmd := exec.Command(condaBin, "remove", "-y", "-p", envPath, name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func (s CondaScanner) InstallCmd(name, location string) *exec.Cmd {
	condaBin := "conda"
	baseConda := filepath.Join(pkg.HomeDir(), "miniconda3", "bin", "conda")
	if _, err := os.Stat(baseConda); err == nil {
		condaBin = baseConda
	}

	envPath := s.resolveEnvPath(location)
	cmd := exec.Command(condaBin, "install", "-y", "-p", envPath, name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func (s CondaScanner) resolveEnvPath(location string) string {
	if location == "base" {
		return filepath.Join(pkg.HomeDir(), "miniconda3")
	}
	if strings.HasPrefix(location, "/") {
		return location
	}
	return filepath.Join(pkg.HomeDir(), "miniconda3", "envs", location)
}

var _ Scanner = CondaScanner{}
