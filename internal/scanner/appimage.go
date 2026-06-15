package scanner

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"whatsinstalled/internal/pkg"
	"whatsinstalled/internal/store"
)

// appImageDirs returns the directories searched for *.AppImage files.
func appImageDirs() []string {
	home := pkg.HomeDir()
	return []string{
		filepath.Join(home, "Applications"),
		filepath.Join(home, "Downloads"),
		filepath.Join(home, ".local", "bin"),
		"/opt",
	}
}

// AppImageScanner scans portable *.AppImage applications, which no package
// manager tracks. Always "available"; its tab shows only when one is found.
type AppImageScanner struct{}

func (AppImageScanner) Name() string      { return "appimage" }
func (AppImageScanner) IsAvailable() bool { return true }
func (s AppImageScanner) Probe() bool {
	for _, dir := range appImageDirs() {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".appimage") {
				return true
			}
		}
	}
	return false
}

func (s AppImageScanner) Scan() ([]store.Package, error) {
	var pkgs []store.Package
	seen := map[string]bool{}
	for _, dir := range appImageDirs() {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".appimage") {
				continue
			}
			path := filepath.Join(dir, e.Name())
			if seen[path] {
				continue
			}
			seen[path] = true
			name, version := splitAppImageName(e.Name())
			pkgs = append(pkgs, store.Package{
				Name:      name,
				Version:   version,
				Source:    "appimage",
				Location:  dir,
				UpdatedAt: time.Now(),
				User:      pkg.CurrentUser(),
				LastUsed:  pkg.GetLastUsed(path),
				SizeBytes: pkg.PathSize(path),
				AddedAt:   pkg.GetModTime(path),
			})
		}
	}
	return pkgs, nil
}

var appImageVersionRE = regexp.MustCompile(`[-_]v?\d[\w.]*$`)

// splitAppImageName turns a filename like "GitKraken-9.10.0.AppImage" into
// ("GitKraken", "9.10.0"). If no trailing version is present the whole stem is
// the name and the version is empty.
func splitAppImageName(filename string) (name, version string) {
	stem := filename
	for _, suf := range []string{".AppImage", ".appimage", ".AppImage.AppImage"} {
		if strings.HasSuffix(stem, suf) {
			stem = stem[:len(stem)-len(suf)]
			break
		}
	}
	if loc := appImageVersionRE.FindStringIndex(stem); loc != nil {
		version = strings.TrimLeft(stem[loc[0]:], "-_v")
		return stem[:loc[0]], version
	}
	return stem, ""
}

var _ Scanner = AppImageScanner{}
