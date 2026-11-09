//go:build windows

package scheduler

import "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"

func writePrivateFileAtomic(path string, body []byte) error {
	return winprivate.Publish(path, body, false)
}
