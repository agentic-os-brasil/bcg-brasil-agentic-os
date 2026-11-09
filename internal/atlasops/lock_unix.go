//go:build !windows

package atlasops

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
	"os"
)

func (engine *Engine) acquireCorpusLock() (func() error, error) {
	return privatelock.Acquire(engine.journalDirPath("grants.lock"))
}
func boundAtlasDirectory(path string) (os.FileInfo, error) { return os.Lstat(path) }
