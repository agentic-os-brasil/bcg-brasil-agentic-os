//go:build !windows

package processidentity

import (
	"os"
	"path/filepath"
)

func executableFilePath(file *os.File) (string, error) {
	return filepath.EvalSymlinks(file.Name())
}
