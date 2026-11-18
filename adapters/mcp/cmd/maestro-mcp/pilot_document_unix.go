//go:build maestro_experimental_installed_seed && (darwin || linux || freebsd || openbsd || netbsd || dragonfly || aix || solaris)

package main

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/pilotdocument"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"golang.org/x/sys/unix"
	"os"
)

func withPilotDocumentRoot(path string, action func(func(int) (*os.File, error)) error) error {
	held, err := scheduler.OpenDirectory(path)
	if err != nil {
		return err
	}
	defer held.Close()
	if err := verifyPilotDocumentDirectory(path, held); err != nil {
		return err
	}
	err = action(func(flags int) (*os.File, error) {
		fd, err := unix.Openat(int(held.Fd()), pilotdocument.Name, flags|unix.O_CLOEXEC|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0600)
		if err != nil {
			return nil, err
		}
		return os.NewFile(uintptr(fd), pilotdocument.Name), nil
	})
	if err != nil {
		return err
	}
	return verifyPilotDocumentDirectory(path, held)
}
