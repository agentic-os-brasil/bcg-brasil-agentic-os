//go:build windows

package atlasops

import (
	"errors"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Preserve the existing owner constructor and behavior; new workspace effects
// require a separately qualified native ownership/confinement publisher.
func workspaceMutationAvailable() error {
	return errors.New("workspace atlas mutation is unavailable on Windows pending native confinement qualification")
}

func publishAtlasFile(path string, body []byte, exclusive, _ bool) error {
	if exclusive {
		return scheduler.WriteNewPrivateFile(path, body)
	}
	return scheduler.ReplacePrivateFile(path, body)
}

func readAtlasFile(path string, maximum int64, _ bool) ([]byte, error) {
	return scheduler.ReadPrivateFile(path, maximum)
}
