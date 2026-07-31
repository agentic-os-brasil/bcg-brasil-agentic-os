//go:build windows

package darwin

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func publishEvolutionFile(temporaryPath, path string) error {
	source, err := windows.UTF16PtrFromString(temporaryPath)
	if err != nil {
		return err
	}
	destination, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	// MOVEFILE_WRITE_THROUGH flushes the move before returning. Omitting
	// MOVEFILE_REPLACE_EXISTING preserves the no-clobber replay fence.
	if err := windows.MoveFileEx(source, destination, windows.MOVEFILE_WRITE_THROUGH); err != nil {
		if errors.Is(err, windows.ERROR_ALREADY_EXISTS) || errors.Is(err, windows.ERROR_FILE_EXISTS) {
			return os.ErrExist
		}
		return err
	}
	return nil
}

// MoveFileEx with MOVEFILE_WRITE_THROUGH already flushes the publication.
func syncEvolutionDirectory(string) error { return nil }
