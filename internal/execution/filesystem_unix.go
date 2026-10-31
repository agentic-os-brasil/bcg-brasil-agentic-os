//go:build !windows

package execution

import (
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/unix"
)

// Each walk holds the parent descriptor before opening the next component.
// A renamed ancestor therefore cannot redirect an operation through a link.
func openLedgerDirectory(path string, create bool) (*os.File, error) {
	path, err := scheduler.CanonicalPrivatePath(path)
	if err != nil {
		return nil, err
	}
	fd, err := unix.Open("/", unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, "/"), "/") {
		if part == "" {
			continue
		}
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
		if create && errors.Is(e, unix.ENOENT) {
			e = unix.Mkdirat(fd, part, 0700)
			if e == nil || errors.Is(e, unix.EEXIST) {
				next, e = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
			}
		}
		unix.Close(fd)
		if e != nil {
			return nil, &os.PathError{Op: "openat", Path: path, Err: e}
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), path), nil
}

func ledgerOpen(path string, flags int, mode os.FileMode) (*os.File, error) {
	dir, err := openLedgerDirectory(filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return ledgerOpenAt(dir, filepath.Base(path), flags, mode)
}

func ledgerOpenAt(dir *os.File, name string, flags int, mode os.FileMode) (*os.File, error) {
	fd, err := unix.Openat(int(dir.Fd()), name, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, uint32(mode.Perm()))
	if err != nil {
		return nil, &os.PathError{Op: "openat", Path: name, Err: err}
	}
	file := os.NewFile(uintptr(fd), name)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		if err != nil {
			return nil, err
		}
		return nil, errors.New("execution file must be regular")
	}
	return file, nil
}

func ledgerMkdirAll(path string, mode os.FileMode) error {
	dir, err := openLedgerDirectory(path, true)
	if err != nil {
		return err
	}
	return dir.Close()
}
func ledgerMkdir(path string, mode os.FileMode) error {
	dir, err := openLedgerDirectory(filepath.Dir(path), false)
	if err != nil {
		return err
	}
	defer dir.Close()
	return unix.Mkdirat(int(dir.Fd()), filepath.Base(path), uint32(mode.Perm()))
}
func ledgerMkdirTemp(path, prefix string) (string, error) {
	name := filepath.Join(path, prefix+hex.EncodeToString(randomBytes(12)))
	return name, ledgerMkdir(name, 0700)
}
func ledgerReadDir(path string) ([]os.DirEntry, error) {
	dir, err := openLedgerDirectory(path, false)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, err
}
func ledgerStatDirectory(path string) (os.FileInfo, error) {
	dir, err := openLedgerDirectory(path, false)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return dir.Stat()
}
func ledgerRemove(path string) error {
	dir, err := openLedgerDirectory(filepath.Dir(path), false)
	if err != nil {
		return err
	}
	defer dir.Close()
	err = unix.Unlinkat(int(dir.Fd()), filepath.Base(path), 0)
	if errors.Is(err, unix.EISDIR) || errors.Is(err, unix.EPERM) {
		err = unix.Unlinkat(int(dir.Fd()), filepath.Base(path), unix.AT_REMOVEDIR)
	}
	return err
}
func ledgerRemoveAll(path string) error {
	dir, err := openLedgerDirectory(filepath.Dir(path), false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer dir.Close()
	return ledgerRemoveTreeAt(dir, filepath.Base(path))
}
func ledgerRemoveTreeAt(parent *os.File, name string) error {
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if errors.Is(err, unix.ENOTDIR) || errors.Is(err, unix.ELOOP) {
		return unix.Unlinkat(int(parent.Fd()), name, 0)
	}
	if err != nil {
		return err
	}
	dir := os.NewFile(uintptr(fd), name)
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := ledgerRemoveTreeAt(dir, entry.Name()); err != nil {
			return err
		}
	}
	return unix.Unlinkat(int(parent.Fd()), name, unix.AT_REMOVEDIR)
}

// Keep the temporary inode and publication in one held directory, including
// cleanup and fsync; no pathname is re-resolved between these operations.
func ledgerPublishJSON(path string, body []byte, immutable bool) error {
	dir, err := openLedgerDirectory(filepath.Dir(path), !immutable)
	if err != nil {
		return err
	}
	defer dir.Close()
	name := ".publish-" + hex.EncodeToString(randomBytes(12))
	file, err := ledgerOpenAt(dir, name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer unix.Unlinkat(int(dir.Fd()), name, 0)
	if _, err = file.Write(body); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if immutable {
		err = unix.Linkat(int(dir.Fd()), name, int(dir.Fd()), filepath.Base(path), 0)
	} else {
		err = unix.Renameat(int(dir.Fd()), name, int(dir.Fd()), filepath.Base(path))
	}
	if err != nil {
		return err
	}
	return dir.Sync()
}

// OpenConfinedStore admits an existing owner-private root for product use.
// Store operations open each component no-follow and do not retain descriptors.
func OpenConfinedStore(root string) (Store, error) {
	if err := validateStoreRoot(root); err != nil {
		return Store{}, err
	}
	dir, err := openLedgerDirectory(root, false)
	if err != nil {
		return Store{}, err
	}
	defer dir.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(int(dir.Fd()), &stat); err != nil {
		return Store{}, err
	}
	if stat.Uid != uint32(os.Getuid()) || stat.Mode&0777 != 0700 {
		return Store{}, errors.New("execution root must be owner-private")
	}
	return Store{Root: dir.Name()}, nil
}
