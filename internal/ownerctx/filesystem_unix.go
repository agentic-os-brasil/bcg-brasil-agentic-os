//go:build !windows

package ownerctx

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/unix"
)

// Follow the execution/scheduler no-follow walk, keeping each parent descriptor
// open until the child is admitted. Publication stays in the held directory.
func openOwnerDirectory(path string, create bool) (*os.File, error) {
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
		unix.Close(fd)
		if e != nil {
			return nil, e
		}
		fd = next
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		unix.Close(fd)
		return nil, err
	}
	if stat.Uid != uint32(os.Getuid()) || stat.Mode&0777 != 0700 {
		unix.Close(fd)
		return nil, errors.New("owner directory must be owner-private")
	}
	return os.NewFile(uintptr(fd), path), nil
}

func ensureOwnerDirectory(path string) error {
	dir, err := openOwnerDirectory(path, true)
	if err != nil {
		return err
	}
	return dir.Close()
}

func ownerOpenAt(dir *os.File, name string, flags int) (*os.File, error) {
	fd, err := unix.Openat(int(dir.Fd()), name, flags|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		file.Close()
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Uid != uint32(os.Getuid()) || stat.Mode&0777 != 0600 {
		file.Close()
		return nil, errors.New("owner file must be an owner-private single-link regular file")
	}
	return file, nil
}

func readOwnerFile(path string, maximum int64) ([]byte, error) {
	dir, err := openOwnerDirectory(filepath.Dir(path), false)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	file, err := ownerOpenAt(dir, filepath.Base(path), os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maximum {
		return nil, errors.New("owner file exceeds bounded size")
	}
	body, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maximum {
		return nil, errors.New("owner file exceeds bounded size")
	}
	return body, nil
}

func publishOwnerFile(path string, body []byte, exclusive bool) error {
	dir, err := openOwnerDirectory(filepath.Dir(path), true)
	if err != nil {
		return err
	}
	defer dir.Close()
	if target, err := ownerOpenAt(dir, filepath.Base(path), os.O_RDONLY); err == nil {
		target.Close()
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	random, err := randomCapability()
	if err != nil {
		return err
	}
	name := ".private-" + random
	file, err := ownerOpenAt(dir, name, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
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
	if exclusive {
		err = scheduler.RenamePrivateFileExclusive(dir, name, filepath.Base(path))
	} else {
		err = unix.Renameat(int(dir.Fd()), name, int(dir.Fd()), filepath.Base(path))
	}
	if err != nil {
		return err
	}
	return dir.Sync()
}

func acquireOwnerTransition(path string) (func() error, error) {
	dir, err := openOwnerDirectory(filepath.Dir(path), true)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	file, err := ownerOpenAt(dir, filepath.Base(path), os.O_RDWR|os.O_CREATE)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return func() error { return errors.Join(unix.Flock(int(file.Fd()), unix.LOCK_UN), file.Close()) }, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			file.Close()
			return nil, err
		}
		if time.Now().After(deadline) {
			file.Close()
			return nil, privatelock.ErrBusy
		}
		time.Sleep(5 * time.Millisecond)
	}
}
