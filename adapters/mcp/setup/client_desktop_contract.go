package setup

import (
	"context"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
)

// VerifyRecordedDesktopContract checks the retained compatible archive against
// its independently admitted current record and existing native-contract owner.
// Historical inert assets are not rebuilt against today's embedded methods.
// This read-only projection grants no connection or functional qualification.
func VerifyRecordedDesktopContract(path string, o installer.ClientOptions, targetOS, contract string) error {
	status, err := installer.ReadClientsSeeded(context.Background(), o)
	if err != nil {
		return err
	}
	// Canonical partial preparation already contains the complete Desktop view;
	// pending Code supplies no authority to skip the archive and binding checks.
	if status.Record == nil || status.State != "current_materialization" && (status.State != installer.ClientPartialPreparation || !status.Record.CodePreparationPending()) {
		return errors.New("current admitted client record required")
	}
	r := status.Record
	if path != r.Materialization.DesktopPath || contract != r.Materialization.DesktopNativeContractSHA256 {
		return errors.New("Desktop selection differs from current record")
	}
	state, err := installtx.ReadSourceState(o.DataRoot, o.ManagedRoot)
	if err != nil || state.TargetOS != targetOS {
		return errors.New("Desktop target differs from current installation")
	}
	sha, err := installer.ClientArchiveDigest(path)
	if err != nil || sha != r.Materialization.DesktopSHA256 {
		return errors.New("retained Desktop archive changed")
	}
	return inspectDesktopCandidateBinding(path, o, targetOS, contract)
}
