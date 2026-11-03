//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package scheduler

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
)

func validatePrivateUpdateIdentity(file *os.File) error {
	var stat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &stat); err != nil {
		return err
	}
	if stat.Uid != uint32(os.Getuid()) || stat.Nlink != 1 {
		return errors.New("private update ownership or link count")
	}
	return nil
}
func (directory *secureDirectory) publishReplacement(source, target string) error {
	if err := unix.Renameat(directory.fd, source, directory.fd, target); err != nil {
		return err
	}
	return unix.Fsync(directory.fd)
}

func (directory *secureDirectory) removePrivateReplacement(name string) error {
	if err := directory.removeFile(name); err != nil {
		return err
	}
	return unix.Fsync(directory.fd)
}
