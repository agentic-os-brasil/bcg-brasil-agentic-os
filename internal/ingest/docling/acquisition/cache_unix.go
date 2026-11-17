//go:build darwin || linux

package acquisition

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/unix"
)

func directoryOwner(name string) error {
	file, err := scheduler.OpenDirectory(name)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) || info.Mode().Perm() != 0700 {
		return errors.New("acquisition_cache_owner_unverified")
	}
	return nil
}
func mkdirPrivateNew(name string) error {
	parent, err := scheduler.OpenDirectory(filepath.Dir(name))
	if err != nil {
		return err
	}
	defer parent.Close()
	return unix.Mkdirat(int(parent.Fd()), filepath.Base(name), 0700)
}
func openPrivateNew(name string) (*os.File, error) {
	parent, err := scheduler.OpenDirectory(filepath.Dir(name))
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	info, err := parent.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) || info.Mode().Perm() != 0700 {
		return nil, errors.New("acquisition_cache_owner_unverified")
	}
	fd, err := unix.Openat(int(parent.Fd()), filepath.Base(name), unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}
func holdPrivateDirectory(name string) (io.Closer, error) {
	if err := directoryOwner(name); err != nil {
		return nil, err
	}
	return scheduler.OpenDirectory(name)
}
