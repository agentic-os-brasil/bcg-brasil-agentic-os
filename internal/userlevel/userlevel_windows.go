//go:build windows

package userlevel

import (
	"errors"

	"golang.org/x/sys/windows"
)

var ErrElevatedProcess = errors.New("Maestro must be installed and initialized from a non-elevated Windows user process; close this window and retry without Run as administrator")

func ensurePlatformUserLevelForPlatform() error {
	if windows.GetCurrentProcessToken().IsElevated() {
		return ErrElevatedProcess
	}
	return nil
}
