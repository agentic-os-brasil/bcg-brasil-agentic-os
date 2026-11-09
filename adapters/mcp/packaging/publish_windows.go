//go:build windows

package packaging

import (
	"errors"
	"golang.org/x/sys/windows"
	"path/filepath"
)

func publishPacket(source, target string) error {
	if filepath.Dir(source) != filepath.Dir(target) {
		return errors.New("packet publication must stay in its selected parent")
	}
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	// No REPLACE_EXISTING: an existing or concurrently newer destination survives.
	return windows.MoveFileEx(from, to, windows.MOVEFILE_WRITE_THROUGH)
}
