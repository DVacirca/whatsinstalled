//go:build darwin

package pkg

import "syscall"

func atime(stat *syscall.Stat_t) (int64, int64) {
	return stat.Atimespec.Sec, stat.Atimespec.Nsec
}
