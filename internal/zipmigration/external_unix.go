//go:build !windows

package zipmigration

import (
	"errors"
	"os"
	"runtime"
	"syscall"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/unix"
	"path"
)

func externalMutationAllowed() error {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
		return errors.New("external atomic mutation is unavailable on this platform")
	}
	return nil
}
func externalReadFlags() int { return os.O_RDONLY | syscall.O_NOFOLLOW | syscall.O_NONBLOCK }
func externalOwnedDirectory(name string, private bool) error {
	if err := scheduler.ValidateDirectory(name); err != nil {
		return err
	}
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	return externalDirectoryInfo(info, private)
}
func externalDirectoryInfo(info os.FileInfo, private bool) error {
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || s.Uid != uint32(os.Getuid()) || info.Mode().Perm()&0022 != 0 {
		return errors.New("external directory ownership or privacy unavailable")
	}
	if private && info.Mode().Perm() != 0700 {
		return errors.New("external state directory must be 0700")
	}
	return nil
}
func externalRegular(info os.FileInfo, private bool) error {
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || s.Nlink != 1 {
		return errors.New("external source must be regular and single-linked")
	}
	if private && (s.Uid != uint32(os.Getuid()) || info.Mode().Perm() != 0600) {
		return errors.New("external metadata must be owner-private")
	}
	return nil
}

func externalAcquire(e *externalTree, attempt string) (func(), error) {
	if err := externalProfileControls(e.project, attempt); err != nil {
		return nil, err
	}
	name := path.Join(externalIntentControl, "apply.lock")
	if err := e.check(name); err != nil {
		return nil, err
	}
	if err := e.mkdir(path.Dir(name)); err != nil {
		return nil, err
	}
	f, err := e.root.OpenFile(name, os.O_RDWR|os.O_CREATE|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0600)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err == nil {
		err = externalRegular(info, true)
	}
	if err == nil {
		err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB)
	}
	if err != nil {
		f.Close()
		return nil, err
	}
	return func() { f.Close() }, nil
}

func externalWritableDirectory(name string) error {
	if err := externalOwnedDirectory(name, false); err != nil {
		return err
	}
	info, err := os.Lstat(name)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0200 == 0 {
		return errors.New("migration control parent must be writable")
	}
	return nil
}
