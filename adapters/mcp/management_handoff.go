package product

import (
	"context"
	"errors"
	"path/filepath"
	"runtime"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/internal/localreview"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// An already-pinned receiver cannot adopt a changed canonical record. A fresh
// connection must repeat the original admission before using its new identity.
var errOwnerReviewRecordChanged = errors.New("owner_review_carrier_changed")

const ownerReviewReconnectAction = "Reconnect only the Maestro extension in Desktop, then retry the request."

// This is the receiving guard, not a mode supplied by a tool or a new source
// admission. Original readers authenticate the current selected carrier on
// every validation, including both sides of trusted foreground confirmation.
// No installation lock is held while waiting for the browser or a domain lock.
func (b *Bound) validateInstalledOwnerReview(home, workspaceID string) error {
	return b.validateInstalledOwnerReviewContext(context.Background(), home, workspaceID)
}

func (b *Bound) validateInstalledOwnerReviewContext(ctx context.Context, home, workspaceID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.options.ManagementCarrierID == "" {
		if b.options.ManagementInstallerVersion != "" || b.ownerReviewClientSHA256 != "" {
			return errors.New("owner_review_carrier_changed")
		}
		if b.options.OwnerReviewChannel != "" {
			owning, err := b.sourceContextWithContext(ctx)
			if err != nil || owning.required || owning.state != nil || b.options.ExperimentalSeed != nil {
				return errors.New("owner_review_carrier_unavailable")
			}
		}
		return nil // Existing unbound pilots and native-first diagnostics retain their contracts.
	}
	if b.options.ManagementInstallerVersion == "" {
		return errors.New("owner_review_carrier_unavailable")
	}
	if role := b.options.ExperimentalSeed; role != nil && b.options.ManagementInstallerVersion != role.Identity().InstallerVersion {
		return errors.New("owner_review_carrier_changed")
	}
	status, err := installer.ReadClientsSeeded(ctx, b.managementClientOptions())
	if cancelled := ctx.Err(); cancelled != nil {
		return cancelled
	}
	if err != nil {
		return errors.New("owner_review_carrier_unavailable")
	}
	return b.validateInstalledOwnerReviewSnapshot(ctx, home, workspaceID, status)
}

// Only the canonical reader above supplies this snapshot. Its source admission
// and material inspection remain mandatory before checking receiver continuity.
func (b *Bound) validateInstalledOwnerReviewSnapshot(ctx context.Context, home, workspaceID string, status installer.ClientStatus) error {
	if status.Record == nil {
		return errors.New("owner_review_carrier_unavailable")
	}
	r := *status.Record
	if status.State != "current_materialization" {
		if status.State != installer.ClientPartialPreparation || !r.CodePreparationPending() || installer.ClientSelectedTarget(r) != "" || !r.Materialization.DesktopOnly() {
			return errors.New("owner_review_carrier_unavailable")
		}
		// This exact schema3 role has a complete Desktop and an unchanged full
		// both-client selection. It supplies no Code packet or enforcement proof.
		if _, err := installer.DesktopPreparationRecord(r); err != nil {
			return errors.New("owner_review_carrier_unavailable")
		}
	}
	if r.Binding.Home != home || r.Binding.WorkspaceID != workspaceID || r.Binding.Workspace != b.options.WorkspacePath || r.Materialization.OwnerReviewChannel != b.options.OwnerReviewChannel {
		return errors.New("owner_review_carrier_changed")
	}
	pin := installer.ClientRecordDigest(r)
	if b.ownerReviewClientSHA256 != "" && b.ownerReviewClientSHA256 != pin {
		return errOwnerReviewRecordChanged
	}
	if b.ownerReviewClientSHA256 == "" {
		b.ownerReviewClientSHA256 = pin
	}
	return ctx.Err()
}

func (b *Bound) managementClientOptions() installer.ClientOptions {
	name := "maestro-install"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{ExperimentalSeed: b.options.ExperimentalSeed, Self: filepath.Join(b.options.ProductRoot, name), ManagedRoot: b.options.ProductRoot, UserHome: b.options.UserHome, DataRoot: filepath.Join(b.options.UserHome, ".maestro-install"), Version: b.options.ManagementInstallerVersion, AuthorityRegistrySHA256: b.options.AuthorityRegistrySHA256}, Request: installer.ClientRequest{CarrierID: b.options.ManagementCarrierID, Workspace: b.options.WorkspacePath}, LookupEnv: b.options.LookupEnv}
}

func (b *Bound) handleManagementHandoff(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	if req == nil || req.Session == nil || ctx.Err() != nil {
		return managementHandoffFailure("foreground_required")
	}
	if b.options.ManagementCarrierID == "" || b.options.ManagementInstallerVersion == "" {
		return managementHandoffFailure("qualified_carrier_context_unavailable")
	}
	options := b.managementClientOptions()
	prepared, err := installer.PrepareClientManagementHandoff(ctx, options)
	if err != nil {
		return managementHandoffFailure("independent_manager_admission_unavailable")
	}
	if err := b.validate(); err != nil {
		return result(nil, err)
	}
	open := b.managementOpen
	if open == nil {
		open = localreview.OpenManagementLauncher
	}
	value, err := requestManagementOpen(ctx, prepared, open)
	if err != nil {
		return managementHandoffFailure("normal_host_handoff_unavailable")
	}
	if b.options.ExperimentalSeed != nil {
		return managementHandoffPostValidation(value, b.validate())
	}
	return result(value, nil)
}

func managementHandoffPostValidation(value any, roleErr error) *mcp.CallToolResult {
	if roleErr != nil {
		// The OS already acknowledged the request. Keep that effect truthful and
		// prevent a generic tool-error retry from opening a second manager.
		pending := make(map[string]string)
		if original, ok := value.(map[string]string); ok {
			for key, field := range original {
				pending[key] = field
			}
		}
		pending["state"] = "handoff_requested"
		pending["role_state"] = "changed_after_request"
		pending["continuation"] = "pending_new_valid_review"
		pending["next_action"] = "The manager was already requested. Inspect that existing window; do not reopen automatically. Activation remains unapproved and requires a new valid role and the original independent review."
		return result(pending, nil)
	}
	return result(value, nil)
}

func managementHandoffFailure(reason string) *mcp.CallToolResult {
	out := result(map[string]string{"state": "handoff_unavailable", "reason": reason, "activation": "not_requested_or_approved", "next_action": "Use the known independently admitted participant manager recovery route for this installation. Preserve the current binding; no alternate source or executable was selected."}, nil)
	out.IsError = true
	return out
}

func requestManagementOpen(ctx context.Context, prepared installer.ClientManagementHandoff, open func(context.Context, string) error) (any, error) {
	if prepared.State != "handoff_prepared" || prepared.Launcher == "" || prepared.Activation != "not_requested_or_approved" {
		return nil, errors.New("qualified_management_handoff_unavailable")
	}
	if err := open(ctx, prepared.Launcher); err != nil {
		return map[string]string{"state": "handoff_unavailable", "activation": "not_requested_or_approved"}, err
	}
	return map[string]string{"state": "handoff_requested", "intent": "review_update_in_independent_participant_manager", "activation": "pending_native_installer_review", "client_loading": "pending", "maintenance_requalification": "pending_actual_per_job_review"}, nil
}
