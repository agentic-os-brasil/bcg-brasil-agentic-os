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

func (b *Bound) handleManagementHandoff(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	if req == nil || req.Session == nil || ctx.Err() != nil {
		return managementHandoffFailure("foreground_required")
	}
	if b.options.ManagementCarrierID == "" || b.options.ManagementInstallerVersion == "" {
		return managementHandoffFailure("qualified_carrier_context_unavailable")
	}
	name := "maestro-install"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	options := installer.ClientOptions{SeededOperationOptions: installer.SeededOperationOptions{Self: filepath.Join(b.options.ProductRoot, name), ManagedRoot: b.options.ProductRoot, UserHome: b.options.UserHome, DataRoot: filepath.Join(b.options.UserHome, ".maestro-install"), Version: b.options.ManagementInstallerVersion, AuthorityRegistrySHA256: b.options.AuthorityRegistrySHA256}, Request: installer.ClientRequest{CarrierID: b.options.ManagementCarrierID, Workspace: b.options.WorkspacePath}}
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
