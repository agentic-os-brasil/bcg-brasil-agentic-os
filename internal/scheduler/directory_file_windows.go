//go:build windows

package scheduler

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
	"os"
)

func openDirectoryFile(path string) (*os.File, error) {
	d, e := winprivate.Inspect(path)
	if e != nil {
		return nil, e
	}
	return d.File, nil
}
