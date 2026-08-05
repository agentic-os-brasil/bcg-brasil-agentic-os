package cli

import (
	"path/filepath"
	"strings"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionctx"
)

// sessionMemorySource resolves only the newest complete local memory commit.
// Missing memory is an active empty state; malformed or incomplete state is
// unavailable and never falls back to captures or raw history.
func sessionMemorySource(root, workspaceID string) sessionctx.MemorySource {
	if strings.TrimSpace(root) == "" || strings.TrimSpace(workspaceID) == "" {
		return sessionctx.MemorySource{State: "unavailable"}
	}
	policy, err := basememory.Policy()
	if err != nil {
		return sessionctx.MemorySource{State: "unavailable"}
	}
	config, err := basememory.Runtime()
	if err != nil {
		return sessionctx.MemorySource{State: "unavailable"}
	}
	engine := memory.Engine{
		Root:    filepath.Join(root, "memory"),
		Policy:  policy,
		Budgets: config.ContextBudgets(),
	}
	bundle, err := engine.AssembleContext(workspaceID)
	if err != nil {
		return sessionctx.MemorySource{State: "unavailable"}
	}
	if len(bundle.Sections) == 0 {
		return sessionctx.MemorySource{State: "empty"}
	}
	return sessionctx.MemorySource{State: "available", Bundle: bundle}
}
