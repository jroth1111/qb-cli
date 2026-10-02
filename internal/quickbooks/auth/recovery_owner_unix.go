//go:build !windows

package auth

import (
	"os"
	"syscall"
)

func recoveryFileOwned(info os.FileInfo) bool {
	s, ok := info.Sys().(*syscall.Stat_t)
	return ok && (int(s.Uid) == os.Geteuid() || s.Uid == 0)
}
