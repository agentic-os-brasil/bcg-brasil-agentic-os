package product

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// ForegroundRuntime is an actual observed transport tuple, never a maintenance grant.
type ForegroundRuntime struct {
	Executable         string                                  `json:"executable"`
	ExecutableSHA256   string                                  `json:"executable_sha256"`
	RuntimeVersion     string                                  `json:"runtime_version"`
	ProfileDirectory   string                                  `json:"profile_directory"`
	WorkingDirectory   string                                  `json:"working_directory"`
	ClientRecordSHA256 string                                  `json:"client_record_sha256"`
	Configuration      maintenance.WeeklyConfiguration         `json:"configuration"`
	Environment        []string                                `json:"environment"`
	Identity           processidentity.WeeklyNativeDeclaration `json:"identity"`
	Check              func(context.Context) error             `json:"-"`
}

type ForegroundPacket struct {
	SchemaVersion        int            `json:"schema_version"`
	Role                 string         `json:"role"`
	RoleSHA256           string         `json:"role_sha256"`
	Scope                historyBinding `json:"scope"`
	SourceManifestSHA256 string         `json:"source_manifest_sha256"`
	RuntimeSHA256        string         `json:"runtime_sha256"`
	Provider             string         `json:"provider"`
	Model                string         `json:"model"`
	Task                 string         `json:"task"`
	Context              string         `json:"context"`
}
type ForegroundResult struct {
	SchemaVersion int    `json:"schema_version"`
	PacketSHA256  string `json:"packet_sha256"`
	Role          string `json:"role"`
	Text          string `json:"text"`
}
type ForegroundExecution struct {
	Result           ForegroundResult `json:"result"`
	SessionID        string           `json:"session_id"`
	RuntimeVersion   string           `json:"runtime_version"`
	Model            string           `json:"model"`
	Usage            json.RawMessage  `json:"usage,omitempty"`
	CostUSD          *float64         `json:"cost_usd,omitempty"`
	ObservedProvider string           `json:"observed_provider,omitempty"`
	ProviderEvidence string           `json:"provider_evidence"`
}
type foregroundArguments struct {
	AgentRole        string `json:"agent_role"`
	AgentTask        string `json:"agent_task"`
	AgentContext     string `json:"agent_context"`
	AgentWorkspaceID string `json:"agent_workspace_id"`
	AgentScopeID     string `json:"agent_scope_id"`
	AgentProvider    string `json:"agent_provider"`
	AgentModel       string `json:"agent_model"`
}

// Scope and output are never persisted here. The coordinator uses the existing
// separately reviewed filesystem operations for an approved durable artifact.

func ForegroundDigest(value any) string {
	body, err := json.Marshal(value)
	if err != nil {
		return ""
	}
	h := sha256.Sum256(body)
	return hex.EncodeToString(h[:])
}
func foregroundText(s string, max int) bool {
	return s != "" && len(s) <= max && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
func ValidateForegroundSelection(provider, model string) error {
	for _, v := range []string{provider, model} {
		if !foregroundText(v, 128) || strings.TrimSpace(v) != v || strings.HasPrefix(v, "-") || strings.ContainsAny(v, "\r\n") {
			return errors.New("foreground_selection_invalid")
		}
	}
	return nil
}
func foregroundRoles() ([]string, error) {
	files, err := baseagents.Files()
	if err != nil {
		return nil, err
	}
	var p struct {
		Agents []struct {
			ID, Layer, Status string
			Dispatchable      bool
		}
	}
	if json.Unmarshal(files["activation-policy.json"], &p) != nil {
		return nil, errors.New("foreground_policy_invalid")
	}
	roles := []string{}
	for _, r := range p.Agents {
		if r.Layer == "spoke" && r.Status == "active" && r.Dispatchable {
			roles = append(roles, r.ID)
		}
	}
	return roles, nil
}

// ForegroundRole returns only currently active dispatchable canonical spokes.
func ForegroundRole(id string) ([]byte, error) {
	roles, err := foregroundRoles()
	if err != nil {
		return nil, err
	}
	for _, r := range roles {
		if r == id {
			return baseagents.Definition(id)
		}
	}
	return nil, errors.New("foreground_role_unavailable")
}
func foregroundQueries() []any { return []any{variant("agents.status", map[string]any{})} }
func foregroundMutations() []any {
	return []any{variant("agents.run", map[string]any{
		"agent_role": strSchema(64), "agent_task": strSchema(4096), "agent_context": strSchema(8192),
		"agent_workspace_id": strSchema(64), "agent_scope_id": strSchema(128),
		"agent_provider": strSchema(128), "agent_model": strSchema(128),
	}, "agent_role", "agent_task", "agent_context", "agent_workspace_id", "agent_scope_id", "agent_provider", "agent_model")}
}
func (b *Bound) foregroundRuntime(ctx context.Context) (ForegroundRuntime, error) {
	if b.options.ForegroundInspect == nil || b.options.ForegroundExecute == nil {
		return ForegroundRuntime{}, errors.New("foreground_runtime_unavailable")
	}
	return b.options.ForegroundInspect(ctx, b.managementClientOptions())
}
func (b *Bound) handleForeground(ctx context.Context, req *mcp.CallToolRequest, in input) *mcp.CallToolResult {
	fail := func(code string) *mcp.CallToolResult {
		out := result(map[string]any{"state": "unavailable", "code": code, "effects": "none", "next_action": "Prepare or inspect the dedicated foreground profile in the participant manager; use its official login. Preserve existing state."}, nil)
		out.IsError = true
		return out
	}
	scope, err := b.historyBinding("")
	if err != nil {
		return fail("foreground_scope_unavailable")
	}
	roles, err := foregroundRoles()
	if err != nil {
		return fail("foreground_policy_unavailable")
	}
	if in.Operation == "agents.status" {
		r, e := b.foregroundRuntime(ctx)
		state, reason := "configured", ""
		if e != nil {
			state, reason = "unavailable", "foreground_profile_preparation_or_login_required"
		}
		if b.foregroundActive.Load() > 0 {
			state = "running"
		}
		return result(map[string]any{"state": state, "reason": reason, "next_action": "When unavailable, open the participant management launcher and choose foreground-profile, then complete its official login. A configured runtime still requires one exact review per call.", "workspace_id": b.workspaceID, "scope_kind": scope.ScopeKind, "scope_id": scope.ScopeID, "dispatchable_roles": roles, "runtime_version": r.RuntimeVersion, "execution": "not_started", "tools": []string{}, "effects": "derived_draft_or_review_only", "owner_review": "per_call", "code_native_agents": "host_path_preserved", "desktop_native_hooks": "not_provided"}, nil)
	}
	if in.AgentWorkspaceID != b.workspaceID || in.AgentScopeID != scope.ScopeID {
		return fail("foreground_scope_changed")
	}
	definition, err := ForegroundRole(in.AgentRole)
	if err != nil {
		return fail("foreground_role_unavailable")
	}
	if scope.ScopeKind == "case" && in.AgentRole != "yoda" {
		return fail("foreground_role_scope_denied")
	}
	if !foregroundText(in.AgentTask, 4096) || !foregroundText(in.AgentContext, 8192) || ValidateForegroundSelection(in.AgentProvider, in.AgentModel) != nil {
		return fail("foreground_input_invalid")
	}
	runtime, err := b.foregroundRuntime(ctx)
	if err != nil {
		return fail("foreground_runtime_unavailable")
	}
	roleDigest := sha256.Sum256(definition)
	packet := ForegroundPacket{1, in.AgentRole, hex.EncodeToString(roleDigest[:]), scope, b.options.InstalledManifestSHA256, ForegroundDigest(runtime), in.AgentProvider, in.AgentModel, in.AgentTask, in.AgentContext}
	digest := ForegroundDigest(packet)
	encoded, _ := json.Marshal(struct {
		Packet ForegroundPacket `json:"packet"`
		Digest string           `json:"packet_sha256"`
	}{packet, digest})
	if len(encoded) > 16<<10 {
		return fail("foreground_input_exceeds_bound")
	}
	check := func(c context.Context) error {
		if c == nil || c.Err() != nil {
			return errors.New("foreground_cancelled")
		}
		now, e := b.historyBinding("")
		if e != nil || now != scope {
			return errors.New("foreground_scope_changed")
		}
		if runtime.Check == nil {
			return errors.New("foreground_profile_unavailable")
		}
		return runtime.Check(c)
	}
	review := struct {
		Packet  ForegroundPacket  `json:"packet"`
		Runtime ForegroundRuntime `json:"runtime"`
		Digest  string            `json:"packet_sha256"`
		Effects []string          `json:"effects"`
	}{packet, runtime, digest, []string{"one_independent_native_model_turn", "only_this_closed_task_and_context", "no_tools_or_direct_filesystem_effects", "return_derived_output_to_Maestro", "no_maintenance_or_unattended_authority"}}
	if err = b.confirm(ctx, req, "Authorize this one bounded independent agent call? It may consume model usage. Review the exact selected context, role, runtime and provider/model.", review, review); err != nil {
		var pending *confirmationPending
		if errors.As(err, &pending) {
			return pending.result
		}
		return fail(err.Error())
	}
	if err = check(ctx); err != nil {
		return fail("foreground_review_changed")
	}
	b.foregroundActive.Add(1)
	defer b.foregroundActive.Add(-1)
	execution, err := b.options.ForegroundExecute(ctx, runtime, packet, check)
	if err != nil {
		return fail("foreground_execution_failed_or_cancelled")
	}
	if err = check(ctx); err != nil {
		return fail("foreground_changed_before_return")
	}
	if execution.Result.SchemaVersion != 1 || execution.Result.PacketSHA256 != digest || execution.Result.Role != packet.Role || !foregroundText(execution.Result.Text, 8192) || execution.SessionID == "" || len(execution.SessionID) > 128 || execution.Model != packet.Model || execution.RuntimeVersion != runtime.RuntimeVersion {
		return fail("foreground_output_invalid")
	}
	return result(map[string]any{"state": "completed", "evidence": "actual_runtime_response", "workspace_id": b.workspaceID, "scope_id": scope.ScopeID, "execution": execution, "effects": "derived_output_only", "filesystem": "use_separately_reviewed_canonical_operations"}, nil)
}
