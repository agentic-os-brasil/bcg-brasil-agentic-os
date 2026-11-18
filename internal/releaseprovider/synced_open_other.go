//go:build !windows

package releaseprovider

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"os"
)

func openSyncedFile(path string, _ int64) (*os.File, error) { return scheduler.OpenRegularFile(path) }
