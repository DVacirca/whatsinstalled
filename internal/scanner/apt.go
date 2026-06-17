package scanner

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// aptBinDirs are checked in priority order for a package's primary command.
var aptBinDirs = []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin", "/usr/games"}

// dpkgManifestPath returns the path to a package's installed-files manifest,
// preferring the architecture-qualified name (e.g. libssl3:amd64.list).
func dpkgManifestPath(name, arch string) string {
	base := "/var/lib/dpkg/info"
	if arch != "" {
		if p := filepath.Join(base, name+":"+arch+".list"); fileExists(p) {
			return p
		}
	}
	return filepath.Join(base, name+".list")
}

// aptPackageLocation derives a representative install directory for a dpkg
// package from its file manifest: the directory of its primary executable, or
// failing that the directory holding the most of its files. Falls back to the
// dpkg metadata dir when the manifest is unreadable (e.g. permission denied).
func aptPackageLocation(name, arch string) string {
	f, err := os.Open(dpkgManifestPath(name, arch))
	if err != nil {
		return "/var/lib/dpkg"
	}
	defer f.Close()

	var lines []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if p := strings.TrimSpace(sc.Text()); p != "" {
			lines = append(lines, p)
		}
	}
	return deriveAptLocation(lines)
}

// deriveAptLocation picks a representative install directory from a dpkg file
// manifest: the package's primary executable directory, else the directory
// holding most of its files. Returns the dpkg metadata dir when the manifest
// has no usable install files (metapackages, doc-only packages).
func deriveAptLocation(lines []string) string {
	sort.Strings(lines)

	binDirs := make(map[string]bool)
	dirCount := make(map[string]int)
	for i, p := range lines {
		// Skip directory entries — a path that is a prefix of the next line.
		// These would otherwise inflate generic parents like /usr.
		if i+1 < len(lines) && strings.HasPrefix(lines[i+1], p+"/") {
			continue
		}
		dir := filepath.Dir(p)
		for _, bd := range aptBinDirs {
			if dir == bd {
				binDirs[bd] = true
			}
		}
		// Tally real install dirs for the fallback, skipping doc/man/locale noise.
		if !strings.HasPrefix(p, "/usr/share/doc/") &&
			!strings.HasPrefix(p, "/usr/share/man/") &&
			!strings.HasPrefix(p, "/usr/share/locale/") {
			dirCount[dir]++
		}
	}
	for _, bd := range aptBinDirs {
		if binDirs[bd] {
			return bd
		}
	}
	if d := mostCommonDir(dirCount); d != "" {
		return d
	}
	return "/var/lib/dpkg"
}

// mostCommonDir returns the directory with the highest file count, breaking
// ties by shortest then lexicographic order for stable output.
func mostCommonDir(counts map[string]int) string {
	best, bestN := "", -1
	for d, n := range counts {
		if d == "/" || d == "." {
			continue
		}
		if n > bestN || (n == bestN && d < best) {
			best, bestN = d, n
		}
	}
	return best
}

// AptScanner scans all installed dpkg packages (manual + auto dependencies).
type AptScanner struct{}

func (AptScanner) Name() string      { return "apt" }
func (AptScanner) IsAvailable() bool { return commandExists("dpkg-query") }
func (s AptScanner) Probe() bool {
	out, _ := exec.Command("dpkg-query", "-W", "-f=${Package}\n").Output()
	return len(out) > 0
}

func (s AptScanner) Scan() ([]store.Package, error) {
	return s.scanAll()
}

// manualAptPackages returns the set of packages a user explicitly installed
// (not pulled in automatically as a dependency). apt tracks this in
// /var/lib/apt/extended_states, surfaced via `apt-mark showmanual` — the
// dpkg-query ${Auto-Installed} field is unreliable for this.
func manualAptPackages() map[string]bool {
	manual := map[string]bool{}
	out, err := exec.Command("apt-mark", "showmanual").Output()
	if err != nil {
		return manual
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	for sc.Scan() {
		if name := strings.TrimSpace(sc.Text()); name != "" {
			manual[name] = true
		}
	}
	return manual
}

// scanAll is a fallback that returns all installed apt packages.
func (s AptScanner) scanAll() ([]store.Package, error) {
	manual := manualAptPackages()
	cmd := exec.Command("dpkg-query", "-W", "-f=${Package}\t${Version}\t${Installed-Size}\t${Status}\t${Auto-Installed}\t${Architecture}\t${Description}\n")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("dpkg-query: %s", exitErr.Stderr)
		}
		return nil, err
	}

	var pkgs []store.Package
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) < 4 {
			continue
		}
		status := fields[3]
		if !strings.Contains(status, "install ok installed") {
			continue
		}

		arch := ""
		if len(fields) > 5 {
			arch = fields[5]
		}
		location := aptPackageLocation(fields[0], arch)

		p := store.Package{
			Name:     fields[0],
			Version:  fields[1],
			Source:   "apt",
			Location: location,
			User:     pkg.FileOwner(location),
		}
		// Auto-installed = present in dpkg but not explicitly chosen by the
		// user. Only trust this when apt-mark gave us a manual set.
		if len(manual) > 0 {
			p.AutoInstalled = !manual[fields[0]]
		}
		if len(fields) > 2 && fields[2] != "" {
			var sz int64
			fmt.Sscanf(fields[2], "%d", &sz)
			sz *= 1024
			p.SizeBytes = &sz
		}
		if len(fields) > 6 {
			p.Description = strings.TrimSpace(fields[6])
		}
		p.UpdatedAt = time.Now()
		p.LastUsed = pkg.GetLastUsed(dpkgManifestPath(fields[0], arch))
		pkgs = append(pkgs, p)
	}
	return pkgs, scanner.Err()
}

var _ Scanner = AptScanner{}
