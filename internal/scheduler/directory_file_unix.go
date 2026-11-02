//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package scheduler

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func openDirectoryFile(path string) (*os.File, error) {
	fd, err := walkUnixDirectory(path, false, nil)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("directory_handle_unavailable")
	}
	return file, nil
}
