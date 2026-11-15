//go:build windows

package agentidentity

import (
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
)

func removeIdentityEntry(path string) error {
	directory, err := winprivate.Open(filepath.Dir(path), false)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Remove(filepath.Base(path), false)
}
