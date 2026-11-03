//go:build windows

package setup

import (
	"errors"
	"os"
)

func validateDesktopBackupFile(os.FileInfo) error {
	return errors.New("Windows private Desktop backup ownership inspection unavailable")
}
