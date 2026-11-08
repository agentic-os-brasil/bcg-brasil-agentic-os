//go:build windows

package scheduler

import (
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"os"
)

func writePrivateFileAtomic(path string, body []byte) error {
	return winprivate.Publish(path, body, false)
}
func RenamePrivateFileExclusive(directory *os.File, source, target string) error {
	d := &winprivate.Directory{File: directory}
	return d.Rename(source, d, target, false)
}
func RenamePrivateFileExclusiveBetween(from *os.File, source string, to *os.File, target string) error {
	if from == nil || to == nil {
		return errors.New("exclusive rename requires held directories")
	}
	a, b := &winprivate.Directory{File: from}, &winprivate.Directory{File: to}
	return a.RenamePinnedExclusive(source, b, target, "")
}
