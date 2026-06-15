package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// PodmanScanner scans local Podman images.
type PodmanScanner struct{}

func (PodmanScanner) Name() string      { return "podman" }
func (PodmanScanner) IsAvailable() bool { return commandExists("podman") }
func (s PodmanScanner) Probe() bool {
	out, _ := exec.Command("podman", "images", "--format={{.Repository}}").Output()
	return len(out) > 0
}

func (s PodmanScanner) Scan() ([]store.Package, error) {
	cmd := exec.Command("podman", "images", "--format", "{{json .}}")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("podman images: %w", err)
	}

	var pkgs []store.Package
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		var img struct {
			Repository string `json:"Repository"`
			Tag        string `json:"Tag"`
			ID         string `json:"ID"`
			Size       string `json:"Size"`
			CreatedAt  string `json:"CreatedAt"`
		}
		if err := json.Unmarshal([]byte(line), &img); err != nil {
			continue
		}
		if img.Repository == "<none>" || img.Tag == "<none>" {
			continue
		}
		name := img.Repository
		if img.Tag != "latest" && img.Tag != "" {
			name += ":" + img.Tag
		}
		p := store.Package{
			Name:      name,
			Version:   img.Tag,
			Source:    "podman",
			Location:  "local",
			UpdatedAt: time.Now(),
			User:      pkg.CurrentUser(),
			SizeBytes: parseDockerSize(img.Size),
			AddedAt:   parseDockerCreated(img.CreatedAt),
		}
		pkgs = append(pkgs, p)
	}

	return pkgs, nil
}

func (s PodmanScanner) Uninstall(name, location string) error {
	return s.UninstallCmd(name, location).Run()
}

func (s PodmanScanner) Install(name, location string) error {
	return s.InstallCmd(name, location).Run()
}

func (s PodmanScanner) UninstallCmd(name, location string) *exec.Cmd {
	cmd := exec.Command("podman", "rmi", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func (s PodmanScanner) InstallCmd(name, location string) *exec.Cmd {
	cmd := exec.Command("podman", "pull", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

var _ Scanner = PodmanScanner{}
