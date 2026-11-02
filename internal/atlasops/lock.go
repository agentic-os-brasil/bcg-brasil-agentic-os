package atlasops

import (
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// One corpus lock orders grant transitions with page reads, comparisons and
// effects, so a completed revocation cannot race an already-checked writer.
func (engine *Engine) beginMutation() (func() error, error) {
	if engine.workspace != nil {
		if err := workspaceMutationAvailable(); err != nil {
			return nil, err
		}
	}
	if err := engine.validateBinding(); err != nil {
		return nil, err
	}
	if err := scheduler.EnsurePrivateDirectory(engine.journalDirPath("")); err != nil {
		return nil, err
	}
	release, err := privatelock.Acquire(engine.journalDirPath("grants.lock"))
	if err != nil {
		return nil, err
	}
	if err := engine.validateBinding(); err != nil {
		_ = release()
		return nil, err
	}
	if err := scheduler.ValidatePrivateDirectory(engine.journalDirPath("")); err != nil {
		_ = release()
		return nil, err
	}
	return release, nil
}
