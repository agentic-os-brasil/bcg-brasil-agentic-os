//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package datahome

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

const bootstrapDirFlags = unix.O_RDONLY | unix.O_DIRECTORY | unix.O_CLOEXEC | unix.O_NOFOLLOW

// Keep handles open for the entire transition: renames cannot redirect later
// writes, and no existing leaf is overwritten or removed on failure.
func initializeFresh(options Options, result Result) error {
	user, err := bootstrapDirectory(options.UserHome, false)
	if err != nil {
		return err
	}
	defer unix.Close(user)
	if err = bootstrapOwner(user, false); err != nil {
		return err
	}
	// Recheck the locator on the pinned user directory before creating anything.
	var locator unix.Stat_t
	if err = unix.Fstatat(user, ".maestro-home.json", &locator, unix.AT_SYMLINK_NOFOLLOW); !errors.Is(err, unix.ENOENT) {
		return errors.New("locator appeared during bootstrap")
	}
	home, err := bootstrapDirectory(result.Home, true)
	if err != nil {
		return err
	}
	defer unix.Close(home)
	if err = bootstrapOwner(home, true); err != nil {
		return err
	}
	duplicate, err := unix.Dup(home)
	if err != nil {
		return err
	}
	directory := os.NewFile(uintptr(duplicate), "bootstrap home")
	entries, readErr := directory.ReadDir(1)
	closeErr := directory.Close()
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return readErr
	}
	if closeErr != nil {
		return closeErr
	}
	if len(entries) != 0 {
		return errors.New("home is no longer empty")
	}
	brain, err := bootstrapChild(home, "brain")
	if err != nil {
		return err
	}
	defer unix.Close(brain)
	metadata, err := bootstrapChild(brain, ".maestro")
	if err != nil {
		return err
	}
	defer unix.Close(metadata)
	// The locator is published first. Until origin publication, Resolve refuses
	// this partial pair; origin is the last authority-granting write.
	if !samePath(result.Home, filepath.Join(options.UserHome, ".maestro")) {
		body, _ := json.Marshal(map[string]any{"schema_version": 1, "home": result.Home, "attempt_id": ""})
		if err = bootstrapRecord(user, ".maestro-home.json", body); err != nil {
			return err
		}
	}
	body, _ := json.Marshal(map[string]any{"schema_version": 1, "home": result.Home, "origin": "fresh-confirmed", "attempt_id": "", "plan_sha256": ""})
	return bootstrapRecord(metadata, "origin.json", body)
}

func bootstrapDirectory(path string, create bool) (int, error) {
	fd, err := unix.Open(string(filepath.Separator), bootstrapDirFlags, 0)
	if err != nil {
		return -1, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(path, string(filepath.Separator)), string(filepath.Separator)) {
		if part == "" {
			continue
		}
		child, openErr := unix.Openat(fd, part, bootstrapDirFlags, 0)
		if create && errors.Is(openErr, unix.ENOENT) {
			// EEXIST means a concurrent writer won. Do not adopt its directory.
			if err = unix.Mkdirat(fd, part, 0700); err != nil {
				unix.Close(fd)
				return -1, err
			}
			if err = unix.Fsync(fd); err != nil {
				unix.Close(fd)
				return -1, err
			}
			child, openErr = unix.Openat(fd, part, bootstrapDirFlags, 0)
			if openErr == nil {
				openErr = bootstrapOwner(child, true)
			}
		}
		unix.Close(fd)
		if openErr != nil {
			if child >= 0 {
				unix.Close(child)
			}
			return -1, openErr
		}
		fd = child
	}
	return fd, nil
}

func bootstrapChild(parent int, name string) (int, error) {
	if err := unix.Mkdirat(parent, name, 0700); err != nil {
		return -1, err
	}
	fd, err := unix.Openat(parent, name, bootstrapDirFlags, 0)
	if err != nil {
		return -1, err
	}
	if err = bootstrapOwner(fd, true); err != nil {
		unix.Close(fd)
		return -1, err
	}
	if err = unix.Fsync(parent); err != nil {
		unix.Close(fd)
		return -1, err
	}
	return fd, nil
}

func bootstrapOwner(fd int, private bool) error {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Uid != uint32(os.Getuid()) || stat.Mode&0022 != 0 || (private && stat.Mode&0777 != 0700) {
		return errors.New("bootstrap directory ownership or permissions are unsafe")
	}
	return nil
}

func bootstrapRecord(parent int, name string, body []byte) error {
	fd, err := unix.Openat(parent, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), "bootstrap record")
	defer file.Close()
	var stat unix.Stat_t
	if err = unix.Fstat(fd, &stat); err != nil {
		return err
	}
	if stat.Uid != uint32(os.Getuid()) || stat.Mode&0777 != 0600 || stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("bootstrap record ownership or permissions are unsafe")
	}
	if _, err = file.Write(body); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return unix.Fsync(parent)
}
