//go:build windows

package scheduler

import (
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
