package atlasops

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// OpenAccountExisting binds one authenticated account's sole authored narrative.
// The host admits the account; callers cannot supply a corpus root or page.
// Queries never create or repair the already admitted owner's brain.
func OpenAccountExisting(home, accountID string, beforeMutation func() error) (*Engine, error) {
	if !agentcatalog.ValidAgentID(accountID) {
		return nil, errors.New("account_scope_invalid")
	}
	canonical, err := scheduler.CanonicalPrivatePath(home)
	if err != nil {
		return nil, err
	}
	root := filepath.Join(canonical, "brain")
	if err = scheduler.ValidatePrivateDirectory(root); err != nil {
		return nil, err
	}
	identity, err := boundAtlasDirectory(root)
	if err != nil {
		return nil, err
	}
	page := "accounts/" + accountID + "/" + accountID + ".md"
	engine := &Engine{root: root, dataRoot: canonical, now: time.Now, accountID: accountID, boundDirectories: []os.FileInfo{identity}, workspacePolicy: WorkspacePolicy{BeforeMutation: beforeMutation, AdmitPage: func(requested string) error {
		if requested != page {
			return errors.New("account_page_unavailable")
		}
		return nil
	}}}
	return engine, engine.validateBinding()
}
