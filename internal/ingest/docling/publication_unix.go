//go:build !windows

package docling

import (
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/unix"
)

func openPlacementDirectory(path string) (*os.File, error) {
	return scheduler.OpenDirectory(path)
}

func openPlacementLeaf(parent *os.File, name string) (*os.File, error) {
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func syncPlacementDirectory(directory *os.File) error { return directory.Sync() }
