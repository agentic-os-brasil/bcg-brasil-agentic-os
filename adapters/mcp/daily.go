package product

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceagent"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipruntime"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func dailyScopeSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"owner", "case", "workspace"}}
}
func dailyQueries() []any {
	return []any{variant("daily.status", map[string]any{"daily_scope": dailyScopeSchema()}, "daily_scope")}
}
func dailyMutations() []any {
	list := func() map[string]any {
		return map[string]any{"type": "array", "maxItems": 16, "items": strSchema(4096)}
	}
	return []any{
		variant("daily.enqueue", map[string]any{"daily_scope": dailyScopeSchema(), "checkpoint_id": strSchema(64), "session_id": strSchema(64), "captured_at": strSchema(64), "local_date": strSchema(10), "summary": strSchema(4096), "decisions": list(), "next_actions": list()}, "daily_scope", "checkpoint_id", "session_id", "captured_at", "local_date", "summary", "decisions", "next_actions"),
		variant("daily.drain", map[string]any{"daily_scope": dailyScopeSchema(), "checkpoint_ids": map[string]any{"type": "array", "minItems": 1, "maxItems": 32, "uniqueItems": true, "items": strSchema(64)}}, "daily_scope", "checkpoint_ids"),
		variant("daily.ack", map[string]any{"daily_scope": dailyScopeSchema(), "generation": strSchema(64)}, "daily_scope", "generation"),
	}
}

type dailyInput struct {
	Operation     string   `json:"operation"`
	DailyScope    string   `json:"daily_scope"`
	CheckpointID  string   `json:"checkpoint_id"`
	SessionID     string   `json:"session_id"`
	CapturedAt    string   `json:"captured_at"`
	LocalDate     string   `json:"local_date"`
	Summary       string   `json:"summary"`
	Decisions     []string `json:"decisions"`
	NextActions   []string `json:"next_actions"`
	CheckpointIDs []string `json:"checkpoint_ids"`
	Generation    string   `json:"generation"`
}

func (b *Bound) dailyOwner(choice string) (*zipruntime.DailyQueue, func() error, func(), error) {
	if e := b.validate(); e != nil {
		return nil, nil, nil, e
	}
	path, scope := filepath.Join(b.home, "brain"), "owner"
	guard := b.validate
	closeOwner := func() {}
	if choice != "owner" {
		if choice != "case" && choice != "workspace" {
			return nil, nil, nil, errors.New("daily_scope_unavailable")
		}
		owner, e := workspaceagent.OpenSetup(filepath.Join(b.home, "state"), b.options.WorkspacePath, b.workspaceID)
		if e != nil {
			return nil, nil, nil, e
		}
		closeOwner = owner.Close
		status, e := owner.Status()
		if e != nil || status.Setup == nil || status.State != "active" || status.Setup.State != "active" || status.Setup.WorkspaceID != b.workspaceID {
			closeOwner()
			return nil, nil, nil, errors.New("daily_scope_unavailable")
		}
		if choice == "case" {
			if status.Setup.Input.Mode != "client" {
				closeOwner()
				return nil, nil, nil, errors.New("daily_scope_unavailable")
			}
			a, c := status.Setup.Input.AccountID, status.Setup.CaseID
			scope = "account/" + a + "/case/" + c
			path = filepath.Join(b.options.WorkspacePath, "brain", "accounts", a, "cases", c)
		} else {
			if status.Setup.Input.Mode != "internal" {
				closeOwner()
				return nil, nil, nil, errors.New("daily_scope_unavailable")
			}
			// Active internal setup already owns workspace-scoped authored pages.
			scope = "workspace/" + b.workspaceID
			path = filepath.Join(b.options.WorkspacePath, "brain")
		}
		revision := status.SetupRevision
		guard = func() error {
			if e := b.validate(); e != nil {
				return e
			}
			current, e := owner.Status()
			if e != nil {
				return e
			}
			if current.State != "active" || current.SetupRevision != revision {
				return errors.New("daily_scope_changed")
			}
			return nil
		}
	}
	queue, e := zipruntime.OpenDailyQueue(path, b.options.WorkspacePath, scope)
	if e != nil {
		closeOwner()
		return nil, nil, nil, e
	}
	return queue, guard, func() { queue.Close(); closeOwner() }, nil
}
func (b *Bound) handleDaily(ctx context.Context, req *mcp.CallToolRequest) *mcp.CallToolResult {
	failure := func(e error) *mcp.CallToolResult {
		var pending *confirmationPending
		if errors.As(e, &pending) {
			return pending.result
		}
		code := "daily_queue_rejected"
		if errors.Is(e, errConfirmationUnavailable) {
			code = "confirmation_unavailable"
		} else if errors.Is(e, errConfirmationDeclined) {
			code = "confirmation_declined"
		}
		next := "Preserve pending checkpoints; inspect the selected scope and review the exact operation again after resolving its reported prerequisite."
		switch e.Error() {
		case "daily_scope_unavailable", "daily_scope_changed", "daily_writer_busy", "daily_id_conflict", "daily_review_changed", "daily_pending_changed", "daily_generation_changed", "daily_transition_invalid", "daily_recovery_required", "daily_recovery_selection_required", "daily_selection_invalid", "daily_request_changed", "daily_page_changed", "daily_review_too_large":
			code = e.Error()
		}
		if code == "daily_review_too_large" {
			next = "Keep the queue unchanged. Select fewer checkpoints or a smaller explicitly reviewed daily page before retrying; no hidden batching or synthesis is authorized."
		}
		out := result(map[string]any{"code": code, "retryable": false, "next_action": next}, nil)
		out.IsError = true
		return out
	}
	var in dailyInput
	if e := json.Unmarshal(req.Params.Arguments, &in); e != nil {
		return failure(e)
	}
	queue, guard, closeQueue, e := b.dailyOwner(in.DailyScope)
	if e != nil {
		return failure(e)
	}
	defer closeQueue()
	if in.Operation == "daily.status" {
		value, e := queue.Status()
		if e == nil {
			e = guard()
		}
		if e != nil {
			return failure(e)
		}
		return result(value, nil)
	}
	var review zipruntime.DailyReview
	switch in.Operation {
	case "daily.enqueue":
		review, e = queue.ReviewEnqueue(zipruntime.DailyCheckpoint{SchemaVersion: 1, ID: in.CheckpointID, SessionID: in.SessionID, Workspace: b.options.WorkspacePath, Scope: queue.Scope(), CapturedAt: in.CapturedAt, LocalDate: in.LocalDate, Summary: in.Summary, Decisions: in.Decisions, NextActions: in.NextActions, Provenance: "agent-authored"})
	case "daily.drain":
		review, e = queue.ReviewDrain(in.CheckpointIDs)
	case "daily.ack":
		review, e = queue.ReviewAck(in.Generation)
	default:
		e = errors.New("daily_operation_invalid")
	}
	if e != nil {
		return failure(e)
	}
	if e = guard(); e != nil {
		return failure(e)
	}
	if e = b.confirm(ctx, req, "Authorize this exact selected daily continuity operation? Enqueue stages agent-authored context; drain publishes only the displayed checkpoints. A dream acknowledgement asserts agent success and does not perform synthesis.", review, review); e != nil {
		return failure(e)
	}
	if e = guard(); e != nil {
		return failure(e)
	}
	var out zipruntime.DailyResult
	switch in.Operation {
	case "daily.enqueue":
		out, e = queue.Enqueue(review, guard)
	case "daily.drain":
		out, e = queue.Drain(review, guard)
	case "daily.ack":
		out, e = queue.Ack(review, guard)
	}
	if e == nil {
		e = guard()
	}
	if e != nil {
		return failure(e)
	}
	return result(out, nil)
}
