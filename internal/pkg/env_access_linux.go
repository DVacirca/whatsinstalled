//go:build linux

package pkg

import "syscall"

func atime(stat *syscall.Stat_t) (int64, int64) {
	return stat.Atim.Sec, stat.Atim.Nsec
}
