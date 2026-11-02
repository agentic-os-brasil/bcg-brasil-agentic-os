//go:build !windows

package memory

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/unix"
)

func ownedIOAvailable() error { return nil }

func memoryDirectory(path string, create bool) (*os.File, error) {
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
		next, e := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if create && errors.Is(e, unix.ENOENT) {
			e = unix.Mkdirat(fd, part, 0700)
			if e == nil || errors.Is(e, unix.EEXIST) {
				next, e = unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
			}
		}
		_ = unix.Close(fd)
		if e != nil {
			return nil, e
		}
		fd = next
	}
	return os.NewFile(uintptr(fd), path), nil
}

func memoryOpenAt(dir *os.File, name string, flags int) (*os.File, error) {
	fd, err := unix.Openat(int(dir.Fd()), name, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC|unix.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), name)
	var st unix.Stat_t
	if err = unix.Fstat(fd, &st); err != nil {
		_ = f.Close()
		return nil, err
	}
	if st.Mode&unix.S_IFMT != unix.S_IFREG || st.Nlink != 1 || st.Uid != uint32(os.Getuid()) || st.Mode&0077 != 0 {
		_ = f.Close()
		return nil, errors.New("memory file must be owner-private, regular and single-link")
	}
	return f, nil
}

func memoryOpen(path string, flags int) (*os.File, error) {
	d, err := memoryDirectory(filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	return memoryOpenAt(d, filepath.Base(path), flags)
}

func memoryReadFile(path string, maximum int64) ([]byte, error) {
	f, err := memoryOpen(path, os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if st.Size() > maximum {
		return nil, errors.New("memory file exceeds read bound")
	}
	b, err := io.ReadAll(io.LimitReader(f, maximum+1))
	if err == nil && int64(len(b)) > maximum {
		err = errors.New("memory file exceeds read bound")
	}
	return b, err
}

func memoryMkdirAll(path string, _ os.FileMode) error {
	d, err := memoryDirectory(path, true)
	if err != nil {
		return err
	}
	return d.Close()
}
func memoryMkdir(path string, mode os.FileMode) error {
	d, err := memoryDirectory(filepath.Dir(path), false)
	if err != nil {
		return err
	}
	defer d.Close()
	return unix.Mkdirat(int(d.Fd()), filepath.Base(path), uint32(mode.Perm()))
}
func memoryMkdirTemp(path, prefix string) (string, error) {
	name := filepath.Join(path, prefix+memoryRandomID())
	return name, memoryMkdir(name, 0700)
}
func memoryRandomID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func memoryReadDir(path string) ([]os.DirEntry, error) {
	d, err := memoryDirectory(path, false)
	if err != nil {
		return nil, err
	}
	defer d.Close()
	entries, err := d.ReadDir(4097)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if len(entries) > 4096 {
		return nil, errors.New("memory directory exceeds entry bound")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	return entries, nil
}

// Publish and cleanup use the same held directory; no ancestor is resolved again.
func memoryPublish(path string, body []byte, immutable bool) error {
	d, err := memoryDirectory(filepath.Dir(path), false)
	if err != nil {
		return err
	}
	defer d.Close()
	if !immutable {
		old, e := memoryOpenAt(d, filepath.Base(path), os.O_RDONLY)
		if e == nil {
			_ = old.Close()
		} else if !errors.Is(e, os.ErrNotExist) {
			return e
		}
	}
	name := ".publish-" + memoryRandomID()
	f, err := memoryOpenAt(d, name, os.O_WRONLY|os.O_CREATE|os.O_EXCL)
	if err != nil {
		return err
	}
	defer unix.Unlinkat(int(d.Fd()), name, 0)
	if _, err = f.Write(body); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	if immutable {
		err = unix.Linkat(int(d.Fd()), name, int(d.Fd()), filepath.Base(path), 0)
		if err == nil {
			err = unix.Unlinkat(int(d.Fd()), name, 0)
		}
	} else {
		err = unix.Renameat(int(d.Fd()), name, int(d.Fd()), filepath.Base(path))
	}
	if err != nil {
		return err
	}
	return d.Sync()
}

func memoryRemoveAll(path string) error {
	d, err := memoryDirectory(filepath.Dir(path), false)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer d.Close()
	if err := memoryRemoveAt(d, filepath.Base(path)); err != nil {
		return err
	}
	return d.Sync()
}

func memoryPruneVisibilityHistory(path string, through uint64, fault func() error) error {
	directory, err := memoryDirectory(path, false)
	if err != nil {
		return err
	}
	defer directory.Close()
	for revision := uint64(1); revision <= through; revision++ {
		name := fmt.Sprintf("%020d.json", revision)
		file, err := memoryOpenAt(directory, name, os.O_RDONLY)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
		if err := unix.Unlinkat(int(directory.Fd()), name, 0); err != nil {
			return err
		}
		if err := fault(); err != nil {
			if syncErr := directory.Sync(); syncErr != nil {
				return syncErr
			}
			return err
		}
	}
	return directory.Sync()
}
func memoryRemoveAt(parent *os.File, name string) error {
	fd, err := unix.Openat(int(parent.Fd()), name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if errors.Is(err, unix.ENOENT) {
		return nil
	}
	if errors.Is(err, unix.ENOTDIR) {
		f, e := memoryOpenAt(parent, name, os.O_RDONLY)
		if e != nil {
			return e
		}
		_ = f.Close()
		return unix.Unlinkat(int(parent.Fd()), name, 0)
	}
	if err != nil {
		return err
	}
	d := os.NewFile(uintptr(fd), name)
	defer d.Close()
	entries, err := d.ReadDir(4097)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) > 4096 {
		return errors.New("memory cleanup exceeds entry bound")
	}
	for _, e := range entries {
		if err := memoryRemoveAt(d, e.Name()); err != nil {
			return err
		}
	}
	return unix.Unlinkat(int(parent.Fd()), name, unix.AT_REMOVEDIR)
}

func memoryRename(source, target string) error {
	s, err := memoryDirectory(filepath.Dir(source), false)
	if err != nil {
		return err
	}
	defer s.Close()
	d, err := memoryDirectory(filepath.Dir(target), false)
	if err != nil {
		return err
	}
	defer d.Close()
	if err = unix.Renameat(int(s.Fd()), filepath.Base(source), int(d.Fd()), filepath.Base(target)); err != nil {
		return err
	}
	if err = d.Sync(); err != nil {
		return err
	}
	return s.Sync()
}
