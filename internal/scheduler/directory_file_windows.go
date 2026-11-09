//go:build windows

package scheduler

import (
	"errors"
	"golang.org/x/sys/windows"
	"os"
)

func openDirectoryFile(path string) (*os.File, error) {
	handle, err := walkWindowsDirectory(path, false, nil)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, errors.New("directory_handle_unavailable")
	}
	return file, nil
}
