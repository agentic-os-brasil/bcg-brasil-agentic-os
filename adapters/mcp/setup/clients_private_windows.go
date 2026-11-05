//go:build windows

package setup

import (
	"errors"
	"os"
)

func validateClientPrivateFile(os.FileInfo, os.FileMode) error {
	return errors.New("Windows private client file ownership inspection unavailable")
}
