package scanner

import (
	"os/exec"
	"strings"
)

// storeMarkers are path fragments that mark a binary as managed by a package
// manager whose payloads live in a content-addressed store. The loose-binary
// scan skips these because their own scanner already reports them.
var storeMarkers = []string{"/Cellar/", "/nix/store/", "/snap/"}

// isStoreManaged reports whether realPath lives under a package-manager store
// (Homebrew Cellar, Nix store, or a snap mount).
func isStoreManaged(realPath string) bool {
	for _, m := range storeMarkers {
		if strings.Contains(realPath, m) {
			return true
		}
	}
	return false
}

// pathAliases returns usr-merge spelling variants of an absolute path, e.g.
// /usr/sbin/bridge <-> /sbin/bridge, so package-manager lookups match whichever
// form the database recorded.
func pathAliases(p string) []string {
	for _, d := range []string{"/bin/", "/sbin/", "/lib/"} {
		if strings.HasPrefix(p, "/usr"+d) {
			return []string{strings.TrimPrefix(p, "/usr")}
		}
		if strings.HasPrefix(p, d) {
			return []string{"/usr" + p}
		}
	}
	return nil
}

// ownedAny reports whether a path or any of its usr-merge aliases is in the set.
func ownedAny(set map[string]bool, p string) bool {
	if set[p] {
		return true
	}
	for _, a := range pathAliases(p) {
		if set[a] {
			return true
		}
	}
	return false
}

// managedPaths returns the set of package-manager-owned path spellings derived
// from the given absolute paths (dpkg, pacman, or rpm). dpkg is queried for the
// candidate paths plus their usr-merge aliases; pacman and rpm dump their full
// file list once. Test membership with ownedAny. Empty when no manager present.
func managedPaths(paths []string) map[string]bool {
	owned := make(map[string]bool)
	if len(paths) == 0 {
		return owned
	}
	switch {
	case commandExists("dpkg-query"):
		// Query both usr-merge spellings; dpkg only reports what it is asked.
		query := make([]string, 0, len(paths)*2)
		for _, p := range paths {
			query = append(query, p)
			query = append(query, pathAliases(p)...)
		}
		for _, batch := range chunkStrings(query, 400) {
			args := append([]string{"-S"}, batch...)
			out, _ := exec.Command("dpkg-query", args...).Output()
			for _, p := range parseDpkgSearch(string(out)) {
				owned[p] = true
			}
		}
	case commandExists("pacman"):
		out, _ := exec.Command("pacman", "-Qlq").Output()
		set := linesToSet(string(out))
		for _, p := range paths {
			if set[p] {
				owned[p] = true
			}
		}
	case commandExists("rpm"):
		out, _ := exec.Command("rpm", "-qa", "--qf", "[%{FILENAMES}\n]").Output()
		set := linesToSet(string(out))
		for _, p := range paths {
			if set[p] {
				owned[p] = true
			}
		}
	}
	return owned
}

// parseDpkgSearch extracts the owned absolute paths from `dpkg-query -S` output.
// Lines look like "coreutils: /usr/bin/ls" or "a, b: /usr/sbin/foo"; the path
// follows the final ": ".
func parseDpkgSearch(out string) []string {
	var paths []string
	for _, line := range strings.Split(out, "\n") {
		idx := strings.LastIndex(line, ": ")
		if idx < 0 {
			continue
		}
		if p := strings.TrimSpace(line[idx+2:]); strings.HasPrefix(p, "/") {
			paths = append(paths, p)
		}
	}
	return paths
}

// linesToSet collects non-empty, slash-prefixed lines (trailing slashes
// trimmed) into a set — used for pacman/rpm whole-system file listings.
func linesToSet(out string) map[string]bool {
	set := make(map[string]bool)
	for _, line := range strings.Split(out, "\n") {
		p := strings.TrimRight(strings.TrimSpace(line), "/")
		if strings.HasPrefix(p, "/") {
			set[p] = true
		}
	}
	return set
}

// chunkStrings splits s into consecutive slices of at most n elements.
func chunkStrings(s []string, n int) [][]string {
	var out [][]string
	for i := 0; i < len(s); i += n {
		end := i + n
		if end > len(s) {
			end = len(s)
		}
		out = append(out, s[i:end])
	}
	return out
}
