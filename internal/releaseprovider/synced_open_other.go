//go:build !windows

package releaseprovider

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"os"
)

func openSyncedFile(path string) (*os.File, error) { return scheduler.OpenRegularFile(path) }
