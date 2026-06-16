package scanner

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"whatsinstalled/internal/pkg"
)

// userDataDir returns the base data directory for the current platform,
// honouring XDG_DATA_HOME when set. macOS uses ~/Library/Application Support.
func userDataDir() string {
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		return v
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(pkg.HomeDir(), "Library", "Application Support")
	}
	return filepath.Join(pkg.HomeDir(), ".local", "share")
}

// binDirCandidates returns the ordered set of directory patterns to scan for
// loose binaries, resolved against home and the given env lookup. Patterns may
// contain a single "*" glob segment (e.g. version-manager shims). Filesystem
// existence and package-manager ownership are checked later, not here.
func binDirCandidates(home string, getenv func(string) string) []string {
	var out []string
	add := func(parts ...string) {
		p := filepath.Join(parts...)
		if p != "" && p != "." {
			out = append(out, p)
		}
	}

	// 1. Environment-variable driven (honoured even when unusual).
	if v := getenv("GOBIN"); v != "" {
		add(v)
	}
	for _, g := range filepath.SplitList(getenv("GOPATH")) {
		add(g, "bin")
	}
	if v := getenv("CARGO_HOME"); v != "" {
		add(v, "bin")
	}
	if v := getenv("PNPM_HOME"); v != "" {
		add(v)
	}
	if v := getenv("BUN_INSTALL"); v != "" {
		add(v, "bin")
	}
	if v := getenv("DENO_INSTALL"); v != "" {
		add(v, "bin")
	}
	if v := getenv("VOLTA_HOME"); v != "" {
		add(v, "bin")
	}

	// 2. PATH entries — the ownership filter removes manager-owned binaries,
	//    so scanning the whole PATH is safe and catches custom locations.
	for _, d := range filepath.SplitList(getenv("PATH")) {
		add(d)
	}

	// 3. User / home directories, including language and version-manager dirs.
	userDirs := []string{
		".local/bin",
		"bin",
		"go/bin",
		".cargo/bin",
		".yarn/bin",
		".npm-global/bin",
		".npm-packages/bin",
		".deno/bin",
		".bun/bin",
		".nix-profile/bin",
		".dotnet/tools",
		".composer/vendor/bin",
		".luarocks/bin",
		".cabal/bin",
		".local/share/flatpak/exports/bin",
		".nvm/versions/node/*/bin",
		".rbenv/shims",
		".pyenv/shims",
		".asdf/shims",
		".rvm/bin",
		".volta/bin",
		".sdkman/candidates/*/current/bin",
		".nodenv/shims",
		".goenv/shims",
	}
	for _, d := range userDirs {
		add(home, d)
	}

	// 4. System, /opt and Homebrew directories.
	out = append(out,
		"/usr/local/bin", "/usr/local/sbin",
		"/opt/bin", "/opt/local/bin",
		"/opt/homebrew/bin", "/opt/homebrew/sbin",
		"/home/linuxbrew/.linuxbrew/bin",
		"/snap/bin", "/var/lib/flatpak/exports/bin",
	)

	// 5. Core OS directories. Package-manager binaries here are dropped by the
	//    ownership filter, leaving only genuinely manual installs.
	out = append(out,
		"/usr/bin", "/usr/sbin", "/bin", "/sbin",
		"/usr/games", "/usr/local/games",
	)

	return out
}

// isLibraryFile reports whether a filename is a shared library or object file
// rather than an invokable command. These carry the executable bit in some lib
// directories (e.g. WSL's /usr/lib/wsl/lib, injected into PATH) but are not
// "binaries" a user installed.
func isLibraryFile(name string) bool {
	if strings.Contains(name, ".so.") {
		return true
	}
	for _, ext := range []string{".so", ".dylib", ".dll", ".a", ".la", ".o"} {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}

// excludedBinDir reports whether a directory should never be scanned for loose
// binaries. WSL mounts Windows drives under /mnt, where every file is marked
// executable; those are not part of the Linux package inventory.
func excludedBinDir(p string) bool {
	return p == "/mnt" || strings.HasPrefix(p, "/mnt/")
}

// discoverBinDirs resolves binDirCandidates against the real filesystem:
// globs are expanded, symlinks collapsed, non-directories dropped, duplicates
// removed. Order is preserved (earliest candidate wins).
func discoverBinDirs() []string {
	var dirs []string
	seen := make(map[string]bool)
	addDir := func(p string) {
		if rp, err := filepath.EvalSymlinks(p); err == nil {
			p = rp
		}
		if seen[p] || excludedBinDir(p) {
			return
		}
		if info, err := os.Stat(p); err == nil && info.IsDir() {
			seen[p] = true
			dirs = append(dirs, p)
		}
	}
	for _, c := range binDirCandidates(pkg.HomeDir(), os.Getenv) {
		if strings.Contains(c, "*") {
			matches, _ := filepath.Glob(c)
			for _, m := range matches {
				addDir(m)
			}
			continue
		}
		addDir(c)
	}
	return dirs
}
