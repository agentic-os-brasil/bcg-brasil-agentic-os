package atlasops

import (
	"os"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// OpenOwnerExisting is the read-only opening boundary for product adapters.
// Explicit preparation owns creation; queries neither create nor repair roots.
func OpenOwnerExisting(dataRoot string, now func() time.Time, beforeMutation func() error) (*Engine, error) {
	canonical, err := scheduler.CanonicalPrivatePath(dataRoot)
	if err != nil {
		return nil, err
	}
	root, err := atlas.OwnerRoot(canonical)
	if err != nil {
		return nil, err
	}
	if err = scheduler.ValidatePrivateDirectory(root); err != nil {
		return nil, err
	}
	identity, err := boundAtlasDirectory(root)
	if err != nil {
		return nil, err
	}
	if now == nil {
		now = time.Now
	}
	engine := &Engine{root: root, dataRoot: canonical, now: now, boundDirectories: []os.FileInfo{identity}, workspacePolicy: WorkspacePolicy{BeforeMutation: beforeMutation}}
	return engine, engine.validateBinding()
}
