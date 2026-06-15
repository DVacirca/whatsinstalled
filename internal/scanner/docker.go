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
		location := dockerLocation()
		p := store.Package{
			Name:      name,
			Version:   img.Tag,
			Source:    "docker",
			Location:  location,
			UpdatedAt: time.Now(),
			User:      pkg.FileOwner("/var/lib/docker"),
			SizeBytes: parseDockerSize(img.Size),
			AddedAt:   parseDockerCreated(img.CreatedAt),
		}
		if p.User == "" {
			p.User = pkg.FileOwner(pkg.HomeDir() + ".docker")
		}
		pkgs = append(pkgs, p)
	}

	return pkgs, nil
}

func dockerLocation() string {
	candidates := []string{
		pkg.HomeDir() + "/.local/share/docker", // rootless
		"/var/lib/docker",                      // rootful (default)
		"/var/lib/docker-engine",               // some distros
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return "local"
}

var _ Scanner = DockerScanner{}
