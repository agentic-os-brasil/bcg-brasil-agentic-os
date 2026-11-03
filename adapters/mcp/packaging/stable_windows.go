//go:build windows

package packaging

import (
	"errors"
	"os"
)

func stablePrivateFile(os.FileInfo) error {
	return errors.New("stable packet private ownership admission unavailable on Windows")
}
