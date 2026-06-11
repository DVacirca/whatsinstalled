package scanner

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"installr/internal/pkg"
	"installr/internal/store"
)

// DockerScanner scans local Docker images.
type DockerScanner struct{}

func (DockerScanner) Name() string      { return "docker" }
func (DockerScanner) IsAvailable() bool { return commandExists("docker") }
func (s DockerScanner) Probe() bool {
	out, _ := exec.Command("docker", "images", "--format={{.Repository}}").Output()
	return len(out) > 0
}

func (s DockerScanner) Scan() ([]store.Package, error) {
	cmd := exec.Command("docker", "images", "--format", "{{json .}}")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker images: %w", err)
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
			Source:    "docker",
			Location:  "local",
			UpdatedAt: time.Now(),
			User:      pkg.FileOwner("/var/lib/docker"),
		}
		if p.User == "" {
			p.User = pkg.FileOwner(pkg.HomeDir() + ".docker")
		}
		pkgs = append(pkgs, p)
	}

	return pkgs, nil
}

func (s DockerScanner) Uninstall(name, location string) error {
	return s.UninstallCmd(name, location).Run()
}

func (s DockerScanner) Install(name, location string) error {
	return s.InstallCmd(name, location).Run()
}

func (s DockerScanner) UninstallCmd(name, location string) *exec.Cmd {
	cmd := exec.Command("docker", "rmi", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func (s DockerScanner) InstallCmd(name, location string) *exec.Cmd {
	cmd := exec.Command("docker", "pull", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

var _ Scanner = DockerScanner{}
