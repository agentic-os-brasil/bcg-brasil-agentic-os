//go:build linux

package releaseprovider

import (
	"os"
	"syscall"
)

func syncedSingleLink(_ *os.File, info os.FileInfo) bool {
	s, ok := info.Sys().(*syscall.Stat_t)
	return ok && s.Nlink == 1
}
