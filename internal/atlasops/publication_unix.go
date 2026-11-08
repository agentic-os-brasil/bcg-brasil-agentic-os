//go:build !windows

package atlasops

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/unix"
)

func workspaceMutationAvailable() error { return nil }

// Test-only interruption seam at the exclusive publication boundary.
var atlasExclusivePublicationHook func()

// Match the existing ownerctx/memory publication pattern: hold each no-follow
// directory descriptor and publish only inside the final held directory.
func atlasDirectory(path string, privateParent bool) (*os.File, error) {
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
		next, err := unix.Openat(fd, part, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		_ = unix.Close(fd)
		if err != nil {
			return nil, err
		}
		fd = next
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if stat.Uid != uint32(os.Getuid()) || (privateParent && stat.Mode&0777 != 0700) {
		_ = unix.Close(fd)
		return nil, errors.New("atlas publication directory must be owner-private")
	}
	return os.NewFile(uintptr(fd), path), nil
}

func atlasOpenAt(directory *os.File, name string, flags int) (*os.File, error) {
	fd, err := unix.Openat(int(directory.Fd()), name, flags|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = file.Close()
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG || stat.Nlink != 1 || stat.Uid != uint32(os.Getuid()) || stat.Mode&0777 != 0600 {
		_ = file.Close()
		return nil, errors.New("atlas publication target must be an owner-private single-link regular file")
	}
	return file, nil
}

func readAtlasFile(path string, maximum int64, privateParent bool) ([]byte, error) {
	directory, err := atlasDirectory(filepath.Dir(path), privateParent)
	if err != nil {
		return nil, err
	}
	defer directory.Close()
	file, err := atlasOpenAt(directory, filepath.Base(path), os.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() > maximum {
		return nil, errors.New("atlas file exceeds bounded size")
	}
	body, err := io.ReadAll(io.LimitReader(file, maximum+1))
	if err == nil && int64(len(body)) > maximum {
		return nil, errors.New("atlas file exceeds bounded size")
	}
	return body, err
}

func publishAtlasFile(path string, body []byte, exclusive, privateParent bool) error {
	directory, err := atlasDirectory(filepath.Dir(path), privateParent)
	if err != nil {
		return err
	}
	defer directory.Close()
	target, err := atlasOpenAt(directory, filepath.Base(path), os.O_RDONLY)
	if err == nil {
		if err := target.Close(); err != nil {
			return err
		}
		if exclusive {
			return os.ErrExist
		}
	} else if !exclusive || !errors.Is(err, os.ErrNotExist) {
		return err
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	name := ".atlas-publish-" + hex.EncodeToString(random)
	file, err := atlasOpenAt(directory, name, os.O_CREATE|os.O_EXCL|os.O_WRONLY)
	if err != nil {
		return err
	}
	defer unix.Unlinkat(int(directory.Fd()), name, 0)
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
		err = scheduler.RenamePrivateFileExclusive(directory, name, filepath.Base(path))
		if err == nil {
			if atlasExclusivePublicationHook != nil {
				atlasExclusivePublicationHook()
			}
		}
	} else {
		err = unix.Renameat(int(directory.Fd()), name, int(directory.Fd()), filepath.Base(path))
	}
	if err != nil {
		return err
	}
	return directory.Sync()
}

func moveAtlasFile(source, target, expected string) error {
	from, err := atlasDirectory(filepath.Dir(source), true)
	if err != nil {
		return err
	}
	defer from.Close()
	to, err := atlasDirectory(filepath.Dir(target), true)
	if err != nil {
		return err
	}
	defer to.Close()
	file, err := atlasOpenAt(from, filepath.Base(source), os.O_RDONLY)
	if err != nil {
		return err
	}
	defer file.Close()
	selected, err := file.Stat()
	if err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(file, maximumPageBytes+1))
	if err != nil {
		return err
	}
	if digest(string(body)) != expected {
		return ErrLifecycleChanged
	}
	if err = scheduler.RenamePrivateFileExclusiveBetween(from, filepath.Base(source), to, filepath.Base(target)); err != nil {
		return err
	}
	if err = from.Sync(); err != nil {
		return err
	}
	if err = to.Sync(); err != nil {
		return err
	}
	published, err := atlasOpenAt(to, filepath.Base(target), os.O_RDONLY)
	if err != nil {
		return err
	}
	defer published.Close()
	current, err := published.Stat()
	if err != nil {
		return err
	}
	if !os.SameFile(selected, current) {
		return ErrLifecycleChanged
	}
	return nil
}

func sameAtlasFile(a, b string) (bool, error) {
	inspect := func(path string) (os.FileInfo, error) {
		d, err := atlasDirectory(filepath.Dir(path), true)
		if err != nil {
			return nil, err
		}
		defer d.Close()
		f, err := atlasOpenAt(d, filepath.Base(path), os.O_RDONLY)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return f.Stat()
	}
	x, err := inspect(a)
	if err != nil {
		return false, err
	}
	y, err := inspect(b)
	if err != nil {
		return false, err
	}
	return os.SameFile(x, y), nil
}
