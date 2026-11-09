package product

import (
	"encoding/json"
	"errors"
	"io"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/nativeagentflow"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionhook"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacepolicy"
)

// HookExitCode is deliberately event-aware. Exit 2 cannot prevent SessionStart,
// SubagentStart or an already-completed tool; diagnostics never claim otherwise.
func HookExitCode(event string) int {
	switch event {
	case "PreToolUse", "UserPromptSubmit", "Stop", "SubagentStop":
		return 2
	}
	return 1
}

// HandleClaudeHook is the admitted native adapter, never a dispatcher to ZIP hooks.
func (b *Bound) HandleClaudeHook(event string, in io.Reader, out io.Writer) error {
	if err := b.validate(); err != nil {
		return errors.New("hook_binding_unavailable")
	}
	input, err := claudeadapter.ParseReader(in)
	if err != nil || claudeadapter.ValidateEvent(event, input) != nil {
		return errors.New("hook_input_invalid")
	}
	input.Prompt = "" // Private bodies are never used for identity, context or diagnostics.
	root, err := filepath.EvalSymlinks(b.options.WorkspacePath)
	if err != nil {
		return errors.New("hook_scope_unavailable")
	}
	cwd, err := filepath.EvalSymlinks(input.CWD)
	if err != nil || !workspacepolicy.Within(root, cwd) {
		return errors.New("hook_scope_unavailable")
	}
	dataRoot := filepath.Join(b.home, "state")
	flow, err := nativeagentflow.NewBound(dataRoot, b.workspaceID)
	if err != nil {
		return errors.New("hook_store_unavailable")
	}
	emit := func(value any) error {
		body, err := json.Marshal(value)
		if err != nil || len(body) > 24<<10 {
			return errors.New("hook_output_unavailable")
		}
		body = append(body, '\n')
		n, err := out.Write(body)
		if err != nil || n != len(body) {
			return errors.New("hook_output_unavailable")
		}
		return nil
	}
	deny := func() error { return emit(claudeadapter.FailClosedDenial()) }
	context := func(diagnostic string) error {
		inspection, err := workspace.Inspect(b.options.WorkspacePath, dataRoot)
		if err != nil {
			return errors.New("hook_context_unavailable")
		}
		packet := sessionctx.BuildProduct(inspection)
		value, err := sessionhook.BuildClaudeEvent(packet, event)
		if err != nil {
			return errors.New("hook_context_unavailable")
		}
		if diagnostic != "" {
			value.HookSpecificOutput.AdditionalContext += "\n" + diagnostic
		}
		return emit(value)
	}
	canonical, managed := claudeagents.CanonicalAgentID(input.AgentType)
	receipt := func(semantic string) error {
		if managed {
			input.AgentType = canonical
		} else {
			input.AgentType = ""
		}
		r, err := claudeadapter.Receipt(semantic, input)
		if err != nil {
			return errors.New("hook_receipt_unavailable")
		}
		r.IdempotencyKey = lifecycle.IdempotencyKey(r.IdempotencyKey, input.PromptID)
		if _, err := lifecycle.RecordConfined(dataRoot, b.workspaceID, r); err != nil {
			return errors.New("hook_receipt_unavailable")
		}
		return nil
	}
	switch event {
	case "SessionStart":
		return context("")
	case "UserPromptSubmit":
		if input.AgentID != "" || input.AgentType != "" {
			return errors.New("hook_hub_required")
		}
		err := flow.BeginOccurrence(input.SessionID, input.PromptID)
		if errors.Is(err, nativeagentflow.ErrTurnResetUnavailable) {
			return context("turn_reset_unavailable")
		}
		if err != nil {
			return errors.New("hook_transition_unavailable")
		}
		if input.PromptID == "" {
			return context("turn_reset_unavailable")
		}
		return context("")
	case "PreToolUse":
		if input.AgentID != "" || input.AgentType != "" {
			if !managed || input.AgentID == "" || flow.Authorize(input.SessionID, input.PromptID, input.AgentID, canonical) != nil {
				return deny()
			}
			reason, recognized := claudeagents.GuardTool(input.AgentType, input.ToolName, input.ToolInputJSON(), input.CWD, root)
			if !recognized || reason != "" {
				return deny()
			}
		} else if flow.CheckMain(input.SessionID, input.PromptID) != nil {
			return deny()
		}
		if workspacepolicy.Tool(root, input.CWD, input.ToolName, input.ToolInputJSON()) != nil {
			return deny()
		}
		guard, err := claudeadapter.Guard(input)
		if err != nil {
			return deny()
		}
		if guard.HookSpecificOutput != nil {
			return emit(guard)
		}
		if input.ToolName == "Agent" || input.ToolName == "Task" {
			target, ok := claudeagents.CanonicalAgentID(input.ToolInput.SubagentType)
			if !ok || input.AgentID != "" || flow.Admit(input.SessionID, input.PromptID, input.ToolUseID, target) != nil {
				return deny()
			}
		}
		return emit(struct{}{}) // No allow verdict: retain native permission handling.
	case "SubagentStart":
		if !managed {
			return errors.New("hook_correlation_unavailable")
		}
		// Receipt means observation only, including a later-refused correlation.
		// A failed receipt cannot leave a newly authorized child behind.
		if err := receipt(lifecycle.SubagentStart); err != nil {
			return err
		}
		if flow.Correlate(input.SessionID, input.PromptID, input.AgentID, canonical) != nil {
			return errors.New("hook_correlation_unavailable")
		}
		return emit(claudeadapter.ManagedSubagentStartContext(canonical))
	case "PostToolUse":
		if input.AgentID != "" || input.AgentType != "" {
			if !managed || flow.Authorize(input.SessionID, input.PromptID, input.AgentID, canonical) != nil {
				return errors.New("hook_correlation_unavailable")
			}
		} else if flow.CheckMain(input.SessionID, input.PromptID) != nil {
			return errors.New("hook_correlation_unavailable")
		}
		if err := receipt(lifecycle.PostActionObserve); err != nil {
			return err
		}
		return emit(struct{}{})
	case "SubagentStop":
		if !managed {
			if input.AgentType != "" {
				return errors.New("hook_agent_unavailable")
			}
			return emit(struct{}{})
		} // Host internal observations grant no authority.
		if input.StopHookActive {
			return emit(struct{}{})
		}
		// A failed observation keeps the active child fence intact. Recording
		// the observation first never grants a route transition on its own.
		if err := receipt(lifecycle.SubagentStop); err != nil {
			return err
		}
		if err := flow.ObserveStop(input.SessionID, input.PromptID, input.AgentID, canonical); err != nil {
			return errors.New("hook_transition_unavailable")
		}
		return emit(struct{}{})
	case "Stop":
		if input.StopHookActive {
			return emit(struct{}{})
		}
		if input.AgentID != "" || input.AgentType != "" {
			return errors.New("hook_hub_required")
		}
		if err := flow.ObserveFinalize(input.SessionID, input.PromptID); err != nil {
			return emit(claudeadapter.BlockStop("Maestro cannot close this observed route while its admitted specialist sequence is unresolved."))
		}
		if err := receipt(lifecycle.StopFinalize); err != nil {
			return err
		}
		return emit(struct{}{})
	}
	return errors.New("hook_event_unsupported")
}
