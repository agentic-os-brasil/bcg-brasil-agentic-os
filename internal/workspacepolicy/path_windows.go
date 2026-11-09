//go:build windows

package workspacepolicy

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
)

func validatePolicyPath(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	attributes, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return errors.New("native policy path attributes unavailable")
	}
	directory := filepath.Dir(path)
	// Junctions retain the native directory bit but Go reports ModeIrregular.
	if attributes.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 {
		directory = path
	}
	held, err := winprivate.Inspect(directory)
	if err != nil {
		return err
	}
	return held.Close()
}
