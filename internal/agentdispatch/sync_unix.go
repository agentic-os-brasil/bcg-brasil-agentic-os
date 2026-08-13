//go:build !windows

package agentdispatch

import (
	"fmt"
	"os"
)

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("pilot recovery directory open: %w", err)
	}
	defer directory.Close()
	if err := directory.Sync(); err != nil {
		return fmt.Errorf("pilot recovery directory sync: %w", err)
	}
	return nil
}
