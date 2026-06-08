package pkg

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"syscall"
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

// FileOwner returns the user name that owns the given path.
// Falls back to CurrentUser() on error.
func FileOwner(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return CurrentUser()
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return CurrentUser()
	}
	u, err := user.LookupId(fmt.Sprintf("%d", stat.Uid))
	if err != nil {
		return CurrentUser()
	}
	return u.Username
}

// GetLastUsed returns the last access time for the given path.
// Falls back to modification time if access time is unavailable.
func GetLastUsed(path string) *time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t := info.ModTime()
		return &t
	}
	t := time.Unix(stat.Atim.Sec, stat.Atim.Nsec)
	if t.IsZero() || t.Before(time.Unix(1, 0)) {
		t = info.ModTime()
	}
	return &t
}
