//go:build linux || darwin

package pkg

import (
	"fmt"
	"os"
	"os/user"
	"syscall"
	"time"
)

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
	t := time.Unix(atime(stat))
	if t.IsZero() || t.Before(time.Unix(1, 0)) {
		t = info.ModTime()
	}
	return &t
}
