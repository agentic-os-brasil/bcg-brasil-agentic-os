package memory

import (
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Bootstrap materializes the private, workspace-scoped memory directories
// that are safe to create before the first trusted capture. It creates no
// memory content, commit, lock or execution evidence.
func Bootstrap(root, workspaceID string) error {
	if err := validateWorkspaceID(workspaceID); err != nil {
		return err
	}
	root, err := scheduler.CanonicalPrivatePath(root)
	if err != nil {
		return err
	}
	if err := scheduler.EnsurePrivateDirectory(root); err != nil {
		return err
	}
	for _, relative := range []string{
		filepath.Join("workspaces", workspaceID, "l1", "captures"),
		filepath.Join("workspaces", workspaceID, "l1", "attested-captures"),
		filepath.Join("workspaces", workspaceID, "commits"),
		filepath.Join("workspaces", workspaceID, "versions"),
		filepath.Join("workspaces", workspaceID, ".transactions"),
		filepath.Join("workspaces", workspaceID, ".locks"),
	} {
		if err := scheduler.EnsurePrivateDirectory(filepath.Join(root, relative)); err != nil {
			return err
		}
	}
	return nil
}
