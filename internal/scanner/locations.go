package scanner

import (
	"path/filepath"
	"sort"
	"strings"
)

// installBinDirs are checked in priority order for a package's primary command.
var installBinDirs = []string{"/usr/bin", "/bin", "/usr/sbin", "/sbin", "/usr/games"}

// deriveInstallDir picks a representative install directory from a package's
// file manifest (a list of absolute paths): the directory of its primary
// executable, else the directory holding most of its files. Directory entries
// and doc/man/locale noise are ignored. Returns fallback when the manifest has
// no usable install files (e.g. metapackages). Shared by apt and pacman.
func deriveInstallDir(lines []string, fallback string) string {
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
		for _, bd := range installBinDirs {
			if dir == bd {
				binDirs[bd] = true
			}
		}
		if !strings.HasPrefix(p, "/usr/share/doc/") &&
			!strings.HasPrefix(p, "/usr/share/man/") &&
			!strings.HasPrefix(p, "/usr/share/locale/") {
			dirCount[dir]++
		}
	}
	for _, bd := range installBinDirs {
		if binDirs[bd] {
			return bd
		}
	}
	if d := mostCommonDir(dirCount); d != "" {
		return d
	}
	return fallback
}

// mostCommonDir returns the directory with the highest file count, breaking
// ties by lexicographic order for stable output.
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
