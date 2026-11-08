//go:build windows

package zipmigration

import (
	"errors"
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func externalMutationAllowed() error { return errors.New("external Windows ownership is unavailable") }
func externalReadFlags() int         { return os.O_RDONLY }
func externalOwnedDirectory(name string, private bool) error {
	return scheduler.ValidateDirectory(name)
}
func externalDirectoryInfo(info os.FileInfo, private bool) error {
	if !info.IsDir() {
		return errors.New("external control is not a directory")
	}
	return nil
}
func externalRegular(info os.FileInfo, private bool) error {
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("external source must be regular")
	}
	return nil
}

func externalAcquire(*engine, string) (func(), error) { return nil, externalMutationAllowed() }

func externalWritableDirectory(name string) error { return scheduler.ValidateDirectory(name) }
