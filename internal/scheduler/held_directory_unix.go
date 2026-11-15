//go:build !windows

package scheduler

import (
	"errors"
	"golang.org/x/sys/unix"
	"os"
	"path/filepath"
)

func heldDirectoryChild(d *secureDirectory, name string, create bool) (*secureDirectory, error) {
	fd, e := unix.Openat(d.fd, name, secureDirectoryFlags, 0)
	if create && errors.Is(e, unix.ENOENT) {
		if e = unix.Mkdirat(d.fd, name, 0700); e != nil && !errors.Is(e, unix.EEXIST) {
			return nil, e
		}
		fd, e = unix.Openat(d.fd, name, secureDirectoryFlags, 0)
	}
	if e != nil {
		return nil, &os.PathError{Op: "openat", Path: filepath.Join(d.path, name), Err: e}
	}
	child := &secureDirectory{fd: fd, path: filepath.Join(d.path, name)}
	if e = heldDirectoryOwned(child); e != nil {
		child.close()
		return nil, e
	}
	if create {
		if e = unix.Fchmod(fd, 0700); e != nil {
			child.close()
			return nil, e
		}
	}
	return child, nil
}
func heldDirectoryStat(d *secureDirectory) (os.FileInfo, error) {
	fd, e := unix.Openat(d.fd, ".", secureDirectoryFlags, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), d.path)
	defer f.Close()
	return f.Stat()
}
func heldDirectoryRead(d *secureDirectory, max int) ([]os.DirEntry, error) {
	fd, e := unix.Openat(d.fd, ".", secureDirectoryFlags, 0)
	if e != nil {
		return nil, e
	}
	f := os.NewFile(uintptr(fd), d.path)
	defer f.Close()
	return f.ReadDir(max)
}
func heldDirectoryMkdir(d *secureDirectory, name string) error { return unix.Mkdirat(d.fd, name, 0700) }
func heldDirectorySync(d *secureDirectory) error               { return unix.Fsync(d.fd) }
func heldDirectoryRename(a *secureDirectory, an string, b *secureDirectory, bn string, replace bool) error {
	if replace {
		return unix.Renameat(a.fd, an, b.fd, bn)
	}
	if a.fd == b.fd {
		return renamePrivateExclusive(a.fd, an, bn)
	}
	ai, e := heldDirectoryStat(a)
	if e != nil {
		return e
	}
	bi, e := heldDirectoryStat(b)
	if e != nil {
		return e
	}
	if os.SameFile(ai, bi) {
		return renamePrivateExclusive(a.fd, an, bn)
	}
	return errors.New("exclusive held rename requires one parent")
}

func heldDirectoryRemove(d *secureDirectory, name string) error {
	var stat unix.Stat_t
	if e := unix.Fstatat(d.fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW); e != nil {
		return e
	}
	flags := 0
	if stat.Mode&unix.S_IFMT == unix.S_IFDIR {
		flags = unix.AT_REMOVEDIR
	}
	return unix.Unlinkat(d.fd, name, flags)
}
func heldDirectoryOwned(d *secureDirectory) error {
	var st unix.Stat_t
	if e := unix.Fstat(d.fd, &st); e != nil {
		return e
	}
	if st.Uid != uint32(os.Getuid()) {
		return errors.New("held directory must belong to current owner")
	}
	return nil
}
func heldDirectoryPrivate(d *secureDirectory) error {
	if e := heldDirectoryOwned(d); e != nil {
		return e
	}
	var st unix.Stat_t
	if e := unix.Fstat(d.fd, &st); e != nil {
		return e
	}
	if st.Mode&0777 != 0700 {
		return errors.New("held publication parent must be private")
	}
	return nil
}

func openRetainedDirectory(path string) (*secureDirectory, error) {
	return openSecureDirectory(path, false)
}
