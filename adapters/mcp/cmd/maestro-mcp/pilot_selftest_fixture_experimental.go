//go:build maestro_experimental_installed_seed

package main

import (
	"context"
	"errors"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"
	"path/filepath"
	"time"
)

// The existing fixed-document owner retains its no-follow, single-link,
// exclusive-create and conflict-preservation semantics. No SDK child request is
// made while this short current-installation guard is held.
func installedPilotPrepareFixture(ctx context.Context, o product.Options, id string) error {
	if err := userlevel.EnsureNotElevated(); err != nil {
		return err
	}
	fixed, err := installer.LoadExperimentalInstalledContext(o.ProductRoot, o.AuthorityRegistrySHA256, time.Now)
	if err != nil {
		return err
	}
	if fixed.UserHome != o.UserHome || fixed.Version != o.ManagementInstallerVersion || o.ExperimentalSeed == nil || fixed.ExperimentalSeed.Identity().AdmissionSHA256 != o.ExperimentalSeed.Identity().AdmissionSHA256 {
		return errors.New("synthetic_fixture_binding_changed")
	}
	return installtx.GuardQualifiedCurrent(fixed.ManagedRoot, fixed.DataRoot, installtx.ReleaseBinding{ManifestSHA256: o.InstalledManifestSHA256, ClientInputsSHA256: o.InstalledClientInputsSHA256}, time.Now(), func() error {
		status, err := installer.ReadClientsSeeded(ctx, installer.ClientOptions{SeededOperationOptions: fixed, Request: installer.ClientRequest{CarrierID: o.ManagementCarrierID, Workspace: o.WorkspacePath}})
		if err != nil || status.Record == nil || status.Record.Binding.WorkspaceID != id || status.Record.Binding.Workspace != o.WorkspacePath || status.State != "current_materialization" && (status.State != installer.ClientPartialPreparation || !status.Record.CodePreparationPending()) {
			return errors.New("synthetic_fixture_current_carrier_changed")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := installer.VerifyExperimentalInstalledBinding(fixed); err != nil {
			return err
		}
		_, err = prepareFixedPilotDocument(filepath.Clean(o.WorkspacePath))
		return err
	})
}
