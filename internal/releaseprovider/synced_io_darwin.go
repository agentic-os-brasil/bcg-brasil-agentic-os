//go:build darwin

package releaseprovider

import (
	"os"
	"syscall"
)

func syncedSingleLink(_ *os.File, info os.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	// SF_DATALESS is the macOS stat flag for an unhydrated cloud object.
	const dataless = 0x40000000
	return ok && stat.Nlink == 1 && stat.Flags&dataless == 0
}
