package scanner

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// CondaScanner scans conda environments and their packages.
type CondaScanner struct{}

func (CondaScanner) Name() string      { return "conda" }
func (CondaScanner) IsAvailable() bool { return commandExists("conda") }
func (s CondaScanner) Probe() bool {
	out, _ := exec.Command("conda", "env", "list", "--json").Output()
	return len(out) > 50 // minimal JSON with envs array
}

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

	// Try to find the python site-packages dir once
	var sitePkgDir string
	libDir := filepath.Join(envPath, "lib")
	if entries, err := os.ReadDir(libDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() && strings.HasPrefix(entry.Name(), "python3") {
				sitePkgDir = filepath.Join(libDir, entry.Name(), "site-packages")
				break
			}
		}
	}

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

		// Try METADATA file for description
		if sitePkgDir != "" {
			p.Description = s.readCondaMetadata(sitePkgDir, r.Name)
		}
		if p.Description == "" && r.Channel != "" {
			p.Description = fmt.Sprintf("channel: %s", r.Channel)
		}

		// Determine package directory for last-used
		var pkgDir string
		if sitePkgDir != "" {
			pkgDir = filepath.Join(sitePkgDir, r.Name)
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

// readCondaMetadata reads the Summary line from a conda package's METADATA file.
func (s CondaScanner) readCondaMetadata(sitePkgDir, pkgName string) string {
	// Look for <name>-<version>.dist-info/METADATA
	entries, err := os.ReadDir(sitePkgDir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, pkgName) || !strings.HasSuffix(name, ".dist-info") {
			continue
		}
		metaPath := filepath.Join(sitePkgDir, name, "METADATA")
		f, err := os.Open(metaPath)
		if err != nil {
			continue
		}
		desc := s.parseMetadataSummary(f)
		f.Close()
		if desc != "" {
			return desc
		}
	}
	return ""
}

// parseMetadataSummary extracts the Summary field from a METADATA file.
func (s CondaScanner) parseMetadataSummary(r io.Reader) string {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "Summary: ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Summary: "))
		}
	}
	return ""
}

var _ Scanner = CondaScanner{}
