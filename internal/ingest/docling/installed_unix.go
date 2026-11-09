//go:build darwin || linux

package docling

import (
	"bytes"
	"context"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"path"
)

func verifyInstalled(ctx context.Context, root string, entries map[string]entry) error {
	directory, err := scheduler.OpenDirectory(root)
	if err != nil {
		return err
	}
	defer directory.Close()
	expectedDirs := map[string]bool{".": true}
	for name := range entries {
		for dir := path.Dir(name); dir != "."; dir = path.Dir(dir) {
			expectedDirs[dir] = true
		}
	}
	count := 0
	var walk func(*os.File, string) error
	walk = func(held *os.File, prefix string) error {
		info, err := held.Stat()
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode().Perm() != 0555 || !expectedDirs[prefix] {
			return errors.New("installed_directory_changed")
		}
		for {
			children, readErr := held.ReadDir(256)
			if readErr != nil && !errors.Is(readErr, io.EOF) {
				return readErr
			}
			for _, child := range children {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				name := child.Name()
				if !relativeName(name) || path.Base(name) != name {
					return errors.New("installed_alias")
				}
				relative := name
				if prefix != "." {
					relative = prefix + "/" + name
				}
				expected, fileExpected := entries[relative]
				directoryExpected := expectedDirs[relative]
				if !fileExpected && !directoryExpected {
					return errors.New("extra_entry")
				}
				flags := unix.O_RDONLY | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_CLOEXEC
				if directoryExpected {
					flags |= unix.O_DIRECTORY
				}
				fd, err := unix.Openat(int(held.Fd()), name, flags, 0)
				if err != nil {
					return err
				}
				opened := os.NewFile(uintptr(fd), relative)
				if opened == nil {
					_ = unix.Close(fd)
					return errors.New("installed_handle_unavailable")
				}
				if directoryExpected {
					err = walk(opened, relative)
				} else {
					var body []byte
					var info os.FileInfo
					body, info, err = readOpenedFile(opened, int64(len(expected.body)))
					if err == nil && (info.Mode().Perm() != os.FileMode(expected.mode) || !bytes.Equal(body, expected.body)) {
						err = errors.New("installed_file_changed")
					}
					if err == nil {
						count++
					}
				}
				closeErr := opened.Close()
				if err != nil {
					return err
				}
				if closeErr != nil {
					return closeErr
				}
			}
			if errors.Is(readErr, io.EOF) {
				break
			}
		}
		return nil
	}
	if err := walk(directory, "."); err != nil {
		return err
	}
	if count != len(entries) {
		return errors.New("installed_file_missing")
	}
	return nil
}
