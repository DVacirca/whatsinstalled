//go:build windows

package pkg

import (
	"os"
	"time"
)

func FileOwner(path string) string {
	return CurrentUser()
}

func GetLastUsed(path string) *time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	t := info.ModTime()
	return &t
}
