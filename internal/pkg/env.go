package pkg

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// HomeDir returns the user's home directory or an empty string.
func HomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}

// CWD returns the current working directory.
func CWD() string {
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	return cwd
}

// IsRoot returns true if running as root (uid 0).
func IsRoot() bool {
	return os.Geteuid() == 0
}

// ShortenPath replaces the home dir with ~.
func ShortenPath(path string) string {
	home := HomeDir()
	if home != "" {
		if rel, err := filepath.Rel(home, path); err == nil && len(rel) < len(path) {
			return "~" + string(filepath.Separator) + rel
		}
	}
	return path
}

// CurrentUser returns the current OS user name.
func CurrentUser() string {
	u, err := user.Current()
	if err != nil {
		return "unknown"
	}
	return u.Username
}

// GetModTime returns the modification time of path, or nil if it can't be
// stat'd. Unlike access time, mtime is not suppressed by noatime/relatime mounts,
// so it is a reliable "installed/last updated" signal.
func GetModTime(path string) *time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	t := info.ModTime()
	return &t
}

// PathSize returns the on-disk size of path in bytes: the file size for a file,
// or the recursive sum of file sizes for a directory. Returns nil if path can't
// be stat'd. Use only where path is the package's own files (a single binary or
// a dedicated dir), not a shared container, or the number will be misleading.
func PathSize(path string) *int64 {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	if !info.IsDir() {
		s := info.Size()
		return &s
	}
	var total int64
	_ = filepath.WalkDir(path, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if !d.IsDir() {
			if fi, err := d.Info(); err == nil {
				total += fi.Size()
			}
		}
		return nil
	})
	return &total
}

// ShellCommandTimes parses shell history files and returns a map of executable
// names to the most recent observed invocation time. Supports zsh extended
// history (": <epoch>:<duration>;<command>") and bash history (with and without
// timestamps). Returns an empty map if no history files can be read.
func ShellCommandTimes() map[string]time.Time {
	times := make(map[string]time.Time)
	home := HomeDir()
	if home == "" {
		return times
	}

	for _, hist := range []string{".zsh_history", ".bash_history"} {
		path := filepath.Join(home, hist)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		parseShellHistory(string(data), times)
	}
	return times
}

// parseShellHistory parses a shell history file into the times map.
func parseShellHistory(raw string, times map[string]time.Time) {
	lines := strings.Split(raw, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var epoch int64
		var cmd string

		// Zsh extended format: ": <epoch>:<duration>;<command>"
		if strings.HasPrefix(line, ": ") {
			rest := strings.TrimPrefix(line, ": ")
			if idx := strings.IndexByte(rest, ':'); idx > 0 {
				if e, err := strconv.ParseInt(rest[:idx], 10, 64); err == nil && e > 0 {
					epoch = e
					if semi := strings.IndexByte(rest, ';'); semi > 0 {
						cmd = strings.TrimSpace(rest[semi+1:])
					}
				}
			}
		}
		// Bash timestamp line: "#<epoch>" followed by command on next line
		if strings.HasPrefix(line, "#") && epoch == 0 && len(line) > 1 {
			if e, err := strconv.ParseInt(line[1:], 10, 64); err == nil && e > 0 {
				if i+1 < len(lines) {
					epoch = e
					cmd = strings.TrimSpace(lines[i+1])
				}
			}
		}

		if cmd == "" {
			continue
		}

		// Extract the executable: first word, strip path prefixes
		firstWord := strings.Fields(cmd)[0]
		exe := filepath.Base(firstWord) // /usr/bin/ls → ls

		if epoch > 0 {
			t := time.Unix(epoch, 0)
			if prev, ok := times[exe]; !ok || t.After(prev) {
				times[exe] = t
			}
		}
	}
}
