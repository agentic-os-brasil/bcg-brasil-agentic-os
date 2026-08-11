package atlas

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// OwnerRoot resolves the canonical owner atlas root from the data root alone.
//
// Initialize deliberately refuses to act without a registered workspace, which
// is right for the workspace root and wrong for this one: the owner atlas holds
// professional trajectory, methods and learnings that span engagements and
// outlive any single workspace. Binding it to a workspace identity would make
// the owner's own corpus unreachable whenever no case is active.
func OwnerRoot(dataRoot string) (string, error) {
	if strings.TrimSpace(dataRoot) == "" {
		return "", errors.New("data root is required")
	}
	return scheduler.CanonicalPrivatePath(filepath.Join(dataRoot, "atlas", "owner"))
}

// InitializeOwner creates the owner atlas scaffold if it is absent and returns
// a pointer to it. It is idempotent and never overwrites a page: the owner owns
// this corpus and may edit it directly, so a repeated bootstrap has to leave
// hand-authored content exactly as it found it.
//
// It creates no workspace content and reads no workspace state.
func InitializeOwner(dataRoot string) (Pointer, error) {
	root, err := OwnerRoot(dataRoot)
	if err != nil {
		return Pointer{}, err
	}
	if err := createFiles(root, ownerFiles); err != nil {
		return Pointer{}, err
	}
	return pointer(root), nil
}
