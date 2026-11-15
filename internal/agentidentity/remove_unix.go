//go:build !windows

package agentidentity

import (
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/unix"
)

func removeIdentityEntry(path string) error {
	directory, err := scheduler.OpenDirectory(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer directory.Close()
	fd, err := unix.Openat(int(directory.Fd()), filepath.Base(path), unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), filepath.Base(path))
	err = scheduler.ValidatePrivateFile(file, 0600)
	file.Close()
	if err != nil {
		return err
	}
	if err = unix.Unlinkat(int(directory.Fd()), filepath.Base(path), 0); err != nil {
		return err
	}
	return directory.Sync()
}
