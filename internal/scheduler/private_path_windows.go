//go:build windows

package scheduler

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"golang.org/x/sys/windows"
)

var securePathStepHook func(string)
var secureLeafStepHook func(string)

type secureDirectory struct {
	native *winprivate.Directory
	path   string
}

func secureEnsurePrivatePath(path string) error {
	d, e := openSecureDirectory(path, true)
	if e != nil {
		return e
	}
	return d.close()
}
func secureLookupPrivatePath(path string) error {
	d, e := winprivate.Inspect(path)
	if e != nil {
		return e
	}
	return d.Close()
}
func openSecureDirectory(path string, create bool) (*secureDirectory, error) {
	hook := securePathStepHook
	if !create {
		hook = nil
	}
	d, e := winprivate.OpenSteps(path, create, hook)
	if e != nil {
		return nil, e
	}
	return &secureDirectory{native: d, path: path}, nil
}
func (d *secureDirectory) close() error {
	if d == nil {
		return nil
	}
	return d.native.Close()
}
func (d *secureDirectory) openFile(name string, flags int, _ os.FileMode) (*os.File, error) {
	if secureLeafStepHook != nil {
		secureLeafStepHook(filepath.Join(d.path, name))
	}
	return d.native.OpenFile(name, flags)
}
func (d *secureDirectory) readFile(name string) ([]byte, error) {
	f, e := d.openFile(name, os.O_RDONLY, 0)
	if e != nil {
		return nil, e
	}
	defer f.Close()
	return winprivate.Read(f, 1<<20)
}
func (d *secureDirectory) writeNewFile(name string, b []byte) error {
	return d.native.Publish(name, b, true)
}
func (d *secureDirectory) writeFile(name string, b []byte) error {
	return d.native.Publish(name, b, false)
}
func (d *secureDirectory) removeFile(name string) error {
	if secureLeafStepHook != nil {
		secureLeafStepHook(filepath.Join(d.path, name))
	}
	return d.native.Remove(name, false)
}
func (d *secureDirectory) readDir() ([]os.DirEntry, error) { return d.native.ReadDir(4096) }
func secureOpenFile(path string, flags int, mode os.FileMode) (*os.File, error) {
	d, e := openSecureDirectory(filepath.Dir(path), false)
	if e != nil {
		return nil, e
	}
	defer d.close()
	return d.openFile(filepath.Base(path), flags, mode)
}
func secureOpenLock(path string) (*os.File, error) {
	return secureOpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
}
func secureReadFile(path string) ([]byte, error)       { return winprivate.ReadFile(path, 1<<20) }
func secureWriteNewFile(path string, b []byte) error   { return winprivate.Publish(path, b, true) }
func secureWriteFile(path string, b []byte) error      { return winprivate.Publish(path, b, false) }
func secureRemoveFile(path string) error               { return winprivate.Remove(path, false) }
func secureReadDir(path string) ([]os.DirEntry, error) { return winprivate.ReadDir(path) }
func validatePrivateDirectoryIdentity(path string) error {
	d, e := winprivate.Open(path, false)
	if e != nil {
		return e
	}
	return d.Close()
}
func openUnownedRegularFile(path string) (*os.File, error) { return winprivate.OpenRegular(path) }
func isWindowsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist) || errors.Is(err, windows.ERROR_FILE_NOT_FOUND) || errors.Is(err, windows.ERROR_PATH_NOT_FOUND)
}
