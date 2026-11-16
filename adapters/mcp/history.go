package product

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/selectedhistory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceagent"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Embedded by the shared decoder; only the finite history variants admit these fields.
type historyArguments struct {
	HistoryManifest        *selectedhistory.Manifest     `json:"history_manifest,omitempty"`
	HistorySelectionDigest string                        `json:"history_selection_digest,omitempty"`
	HistoryCursor          int                           `json:"history_cursor,omitempty"`
	HistoryBatchSize       int                           `json:"history_batch_size,omitempty"`
	HistoryPrompt          string                        `json:"history_prompt,omitempty"`
	HistoryLanguage        string                        `json:"history_language,omitempty"`
	HistorySessionID       string                        `json:"history_session_id,omitempty"`
	HistoryOccurrenceID    string                        `json:"history_occurrence_id,omitempty"`
	HistoryRecordedAt      string                        `json:"history_recorded_at,omitempty"`
	HistoryConfig          *ownerctx.PromptHistoryConfig `json:"history_config,omitempty"`
	HistoryIDs             []string                      `json:"history_ids,omitempty"`
	HistoryRecoveryDigest  string                        `json:"history_recovery_digest,omitempty"`
}
type historyRequest struct {
	Operation string `json:"operation"`
	OwnerID   string `json:"owner_id"`
	Offset    int    `json:"offset"`
	Limit     int    `json:"limit"`
	historyArguments
}

func historyInteger(minimum, maximum int) map[string]any {
	return map[string]any{"type": "integer", "minimum": minimum, "maximum": maximum}
}
func historyArray(item any, maximum int) map[string]any {
	return map[string]any{"type": "array", "minItems": 1, "maxItems": maximum, "items": item}
}
func historyObject(properties map[string]any, required ...string) map[string]any {
	return map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}
}
func historyManifestSchema() map[string]any {
	session := historyObject(map[string]any{"id": strSchema(128), "path": strSchema(4096)}, "id", "path")
	source := historyObject(map[string]any{"kind": map[string]any{"type": "string", "enum": []string{"claude_code", "claude_export"}}, "workspace": strSchema(4096), "root": strSchema(4096), "sessions": historyArray(session, 20)}, "kind", "workspace", "root", "sessions")
	return historyObject(map[string]any{"workspaces": historyArray(strSchema(4096), 4), "sources": historyArray(source, 4)}, "workspaces", "sources")
}
func historyQueries() []any {
	return []any{
		variant("history.status", map[string]any{"offset": historyInteger(0, 10000), "limit": historyInteger(1, 20)}),
		variant("history.selection", map[string]any{"history_manifest": historyManifestSchema()}, "history_manifest"),
	}
}
func historyMutations() []any {
	owner := map[string]any{"owner_id": strSchema(128)}
	record := map[string]any{"owner_id": strSchema(128), "history_prompt": strSchema(32 << 10), "history_language": strSchema(17), "history_session_id": strSchema(128), "history_occurrence_id": strSchema(128), "history_recorded_at": strSchema(64)}
	config := map[string]any{"owner_id": strSchema(128), "history_config": historyObject(map[string]any{"schema_version": map[string]any{"const": 1}, "owner_id": strSchema(128), "max_entries": historyInteger(1, 10000), "max_bytes": historyInteger(1, 8<<20), "max_age_seconds": historyInteger(3600, 10*365*86400)}, "schema_version", "max_entries", "max_bytes", "max_age_seconds")}
	return []any{
		variant("history.read", map[string]any{"history_manifest": historyManifestSchema(), "history_selection_digest": strSchema(64), "history_cursor": historyInteger(0, 20), "history_batch_size": historyInteger(1, 5)}, "history_manifest", "history_selection_digest"),
		variant("history.record", record, "owner_id", "history_prompt", "history_language", "history_session_id", "history_occurrence_id", "history_recorded_at"),
		variant("history.configure", config, "owner_id", "history_config"),
		variant("history.export", map[string]any{"owner_id": strSchema(128), "history_ids": historyArray(strSchema(128), 8)}, "owner_id", "history_ids"),
		variant("history.delete", map[string]any{"owner_id": strSchema(128), "history_ids": historyArray(strSchema(128), 8)}, "owner_id", "history_ids"),
		variant("history.reset", owner, "owner_id"),
		variant("history.recover", map[string]any{"owner_id": strSchema(128), "history_recovery_digest": strSchema(64)}, "owner_id", "history_recovery_digest"),
	}
}

type historyReviewEnvelope struct {
	Binding historyBinding               `json:"binding"`
	Review  ownerctx.PromptHistoryReview `json:"review"`
}

// Full metadata for10,000 legal retained entries exceeds8MiB when indented.
const maximumHistoryReviewBytes = 16 << 20

var errHistoryScope = errors.New("history scope unavailable")

type historyBinding struct{ Home, Workspace, WorkspaceID, IdentityRevision, ScopeKind, ScopeID, SetupRevision string }

func (b *Bound) historyBinding(ownerID string) (historyBinding, error) {
	binding := historyBinding{Home: b.home, Workspace: b.options.WorkspacePath, WorkspaceID: b.workspaceID, ScopeKind: string(ownerctx.PromptScopeWorkspace), ScopeID: b.workspaceID}
	if err := b.validate(); err != nil {
		return binding, err
	}
	identity, err := agentidentity.ReadProfile(b.identityRoot())
	if err != nil {
		return binding, err
	}
	binding.IdentityRevision = identity.Revision
	if identity.Profile != nil && ownerID != "" && identity.Profile.OwnerID != ownerID {
		return binding, errOwnerReviewChanged
	}
	setup, err := workspaceagent.OpenSetup(filepath.Join(b.home, "state"), b.options.WorkspacePath, b.workspaceID)
	if err != nil {
		return binding, errors.Join(errHistoryScope, err)
	}
	defer setup.Close()
	current, err := setup.Status()
	if err != nil {
		return binding, errors.Join(errHistoryScope, err)
	}
	raw, _ := json.Marshal(current)
	sum := sha256.Sum256(raw)
	binding.SetupRevision = hex.EncodeToString(sum[:])
	if current.State != "absent" && current.State != "active" {
		return binding, errHistoryScope
	}
	if current.Setup != nil && current.Setup.Input.Mode == "client" {
		binding.ScopeKind = string(ownerctx.PromptScopeCase)
		// Registered instance IDs disambiguate equal legacy case slugs across accounts.
		binding.ScopeID = current.Setup.CaseAgentID
	}
	return binding, nil
}
func (b *Bound) historySelection(in historyRequest) (selectedhistory.Plan, error) {
	if in.HistoryManifest == nil {
		return selectedhistory.Plan{}, selectedhistory.ErrSelection
	}
	encoded, _ := json.Marshal(in.HistoryManifest)
	if len(encoded) > 64<<10 {
		return selectedhistory.Plan{}, selectedhistory.ErrSelection
	}
	// Other worktrees require their own bound client invocation and explicit selection.
	for _, path := range in.HistoryManifest.Workspaces {
		if path != b.options.WorkspacePath {
			return selectedhistory.Plan{}, selectedhistory.ErrSelection
		}
	}
	return selectedhistory.Prepare(*in.HistoryManifest)
}
func (b *Bound) handleHistory(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	var in historyRequest
	if req == nil || req.Params == nil || json.Unmarshal(req.Params.Arguments, &in) != nil {
		return result(nil, errors.New("history input invalid"))
	}
	binding, err := b.historyBinding(in.OwnerID)
	if err != nil {
		return historyResult(nil, err)
	}
	revalidate := func() error {
		now, e := b.historyBinding(in.OwnerID)
		if e != nil {
			return e
		}
		if now != binding {
			return errOwnerReviewChanged
		}
		return nil
	}
	switch in.Operation {
	case "history.selection", "history.read":
		plan, e := b.historySelection(in)
		if e != nil {
			return historyResult(nil, e)
		}
		metadata := map[string]any{"selection_digest": plan.Digest, "files": plan.Count, "bytes": plan.Bytes, "source_bodies_read": false, "capture_mode": "attended_only"}
		if in.Operation == "history.selection" {
			return historyResult(metadata, revalidate())
		}
		if plan.Digest != in.HistorySelectionDigest {
			return historyResult(nil, errOwnerReviewChanged)
		}
		size := in.HistoryBatchSize
		if size == 0 {
			size = 5
		}
		review := struct {
			Binding      historyBinding       `json:"binding"`
			Plan         selectedhistory.Plan `json:"plan"`
			Start, Limit int
		}{binding, plan, in.HistoryCursor, size}
		if e = b.confirm(ctx, req, "Autoriza ler somente este lote das conversas selecionadas, sem guardar os textos?", review, review); e != nil {
			return historyResult(nil, e)
		}
		if e = revalidate(); e != nil {
			return historyResult(nil, e)
		}
		batch, e := selectedhistory.Read(plan, in.HistorySelectionDigest, in.HistoryCursor, size)
		if e == nil {
			e = revalidate()
		}
		return historyResult(batch, e)
	case "history.status":
		view, e := ownerctx.PromptHistorySnapshot(b.ownerRoot())
		if errors.Is(e, ownerctx.ErrPromptHistoryRecovery) {
			pending, pendingErr := ownerctx.ReviewPendingPromptHistory(b.ownerRoot())
			if pendingErr != nil {
				return historyResult(nil, pendingErr)
			}
			return historyResult(map[string]any{"state": "recovery_required", "review_digest": pending.Digest, "operation": pending.Operation, "capture_mode": "attended_only", "automatic_capture": false}, revalidate())
		}
		if e != nil {
			return historyResult(nil, e)
		}
		rows := []ownerctx.PromptHistoryReceipt{}
		for _, entry := range view.Entries {
			if string(entry.ScopeKind) == binding.ScopeKind && entry.ScopeID == binding.ScopeID {
				rows = append(rows, ownerctx.PromptHistoryReceipt{ID: entry.ID, RecordedAt: entry.RecordedAt, SHA256: entry.SHA256, Bytes: len(entry.Prompt), ScopeKind: entry.ScopeKind, ScopeID: entry.ScopeID})
			}
		}
		limit := in.Limit
		if limit == 0 {
			limit = 20
		}
		start := min(in.Offset, len(rows))
		end := min(start+limit, len(rows))
		return historyResult(map[string]any{"initialized": view.Initialized, "capture_mode": "attended_only", "automatic_capture": false, "revision": view.Revision, "retention": view.Config, "entries": rows[start:end], "next_offset": end, "complete": end == len(rows)}, revalidate())
	}
	var review ownerctx.PromptHistoryReview
	if in.Operation == "history.recover" {
		review, err = ownerctx.ReviewPendingPromptHistory(b.ownerRoot())
		if err == nil && review.Digest != in.HistoryRecoveryDigest {
			err = errOwnerReviewChanged
		}
	} else {
		view, e := ownerctx.PromptHistorySnapshot(b.ownerRoot())
		if e != nil {
			return historyResult(nil, e)
		}
		if view.Config.OwnerID != "" && view.Config.OwnerID != in.OwnerID {
			return historyResult(nil, errOwnerReviewChanged)
		}
		change := ownerctx.PromptHistoryChange{IDs: in.HistoryIDs}
		switch in.Operation {
		case "history.configure":
			change.Operation = "configure"
			change.Config = in.HistoryConfig
			if change.Config != nil {
				copy := *change.Config
				if copy.OwnerID != "" && copy.OwnerID != in.OwnerID {
					return historyResult(nil, errOwnerReviewChanged)
				}
				copy.OwnerID = in.OwnerID
				change.Config = &copy
			}
		case "history.record":
			if len([]byte(in.HistoryPrompt)) > 32<<10 {
				return historyResult(nil, errors.New("history prompt exceeds reviewed packet bound"))
			}
			timestamp, parseErr := time.Parse(time.RFC3339Nano, in.HistoryRecordedAt)
			if parseErr != nil {
				return historyResult(nil, parseErr)
			}
			change.Operation = "record"
			change.Input = &ownerctx.PromptHistoryInput{OwnerID: in.OwnerID, OccurrenceID: in.HistoryOccurrenceID, Prompt: in.HistoryPrompt, Language: in.HistoryLanguage, RecordedAt: timestamp, Source: "owner", SessionID: in.HistorySessionID, ScopeKind: ownerctx.PromptScopeKind(binding.ScopeKind), ScopeID: binding.ScopeID, ContentKind: "user_prompt"}
		case "history.export", "history.delete":
			change.Operation = in.Operation[len("history."):]
			for _, id := range in.HistoryIDs {
				found := false
				for _, entry := range view.Entries {
					if entry.ID == id && entry.OwnerID == in.OwnerID && string(entry.ScopeKind) == binding.ScopeKind && entry.ScopeID == binding.ScopeID {
						found = true
					}
				}
				if !found {
					return historyResult(nil, errors.New("history entry outside admitted scope"))
				}
			}
		case "history.reset":
			change.Operation = "reset"
		default:
			return historyResult(nil, errors.New("unknown history operation"))
		}
		review, err = ownerctx.ReviewPromptHistory(b.ownerRoot(), change)
	}
	if err != nil {
		return historyResult(nil, err)
	}
	if review.ProposedConfig.OwnerID != "" && review.ProposedConfig.OwnerID != in.OwnerID {
		return historyResult(nil, errOwnerReviewChanged)
	}
	if review.Record != nil && (review.Record.OwnerID != in.OwnerID || string(review.Record.ScopeKind) != binding.ScopeKind || review.Record.ScopeID != binding.ScopeID) {
		return historyResult(nil, errOwnerReviewChanged)
	}
	envelope := historyReviewEnvelope{binding, review}
	if err = b.confirm(ctx, req, "Autoriza esta operação exata no histórico local? Configuração e reset abrangem o histórico deste proprietário. Os arquivos de origem permanecem preservados.", envelope, envelope); err != nil {
		return historyResult(nil, err)
	}
	if err = revalidate(); err != nil {
		return historyResult(nil, err)
	}
	if err = ownerctx.ApplyReviewedPromptHistoryGuarded(b.ownerRoot(), review, true, revalidate); err != nil {
		return historyResult(nil, err)
	}
	if err = revalidate(); err != nil {
		return historyResult(nil, err)
	}
	if review.Operation == "export" {
		return historyResult(map[string]any{"state": "exported", "entries": review.Export, "revision": review.AfterRevision}, nil)
	}
	return historyResult(map[string]any{"state": "applied", "revision": review.AfterRevision, "review_digest": review.Digest, "capture_mode": "attended_only"}, nil)
}
func historyResult(value any, err error) *mcp.CallToolResult {
	if err != nil {
		if errors.Is(err, processidentity.ErrChanged) || errors.Is(err, ErrSourceRecoveryRequired) {
			return result(nil, err)
		}
		code := "history_unavailable"
		var pending *confirmationPending
		switch {
		case errors.As(err, &pending):
			return pending.result
		case errors.Is(err, errConfirmationDeclined):
			code = "confirmation_declined"
		case errors.Is(err, errConfirmationUnavailable):
			code = "confirmation_unavailable"
		case errors.Is(err, errHistoryScope):
			code = "history_scope_unavailable"
		case errors.Is(err, selectedhistory.ErrSelection):
			code = "history_selection_unavailable"
		case errors.Is(err, ownerctx.ErrPromptHistoryRecovery):
			code = "history_recovery_required"
		case errors.Is(err, ownerctx.ErrRevisionConflict):
			code = "revision_conflict"
		}
		metadata := map[string]string{"code": code}
		switch code {
		case "history_scope_unavailable":
			metadata["next_action"] = "Inspect case.setup.status and reconcile the admitted workspace selection before retrying history."
		case "history_selection_unavailable":
			metadata["next_action"] = "Select exact unchanged files in this bound workspace with history.selection, then approve a fresh history.read."
		case "history_recovery_required":
			metadata["next_action"] = "Inspect history.status for the exact pending digest, then explicitly review history.recover. Preserve the private journal."
		}
		body, _ := json.Marshal(metadata)
		return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(body)}}}
	}
	return result(value, nil)
}
