package product

import (
	"encoding/json"
	"errors"
	"io"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionhook"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacepolicy"
)

func (b *Bound) claudeLifecycleContext(event string) (sessionhook.ClaudeOutput, error) {
	inspection, err := workspace.Inspect(b.options.WorkspacePath, filepath.Join(b.home, "state"))
	if err != nil {
		return sessionhook.ClaudeOutput{}, err
	}
	owner, ownerErr := ownerctx.Inspect(b.ownerRoot())
	if ownerErr != nil || !owner.Initialized {
		// Read availability is a projection, never a persisted owner transition.
		owner = ownerctx.Status{Onboarding: ownerctx.OnboardingStatus{State: "unavailable", Track: "unavailable"}}
	}
	packet := sessionctx.BuildProduct(inspection, owner)
	if event == "SessionStart" {
		packet.Memory = sessionctx.Memory{State: "unavailable", Message: "committed reviewed memory unavailable; use memory.context for explicit diagnosis"}
		e, cfg, memoryErr := b.memoryEngine()
		if memoryErr == nil {
			var snapshot memoryContextSnapshot
			snapshot, memoryErr = b.readMemoryContext(e, cfg)
			if memoryErr == nil {
				packet.Memory = sessionctx.Memory{State: "empty", Message: "no eligible generated memory is available"}
				if len(snapshot.Sections) > 0 {
					packet.Memory = sessionctx.Memory{State: "available", Message: "bounded reviewed generated memory", Sections: snapshot.Sections}
					for _, section := range snapshot.Sections {
						packet.Memory.Layers = append(packet.Memory.Layers, sessionctx.MemoryLayer{Layer: section.Layer, Pointer: "bcgos://memory/" + section.Layer, Truncated: section.Truncated})
					}
				}
			}
		}
	}
	if err = b.validate(); err != nil {
		return sessionhook.ClaudeOutput{}, err
	}
	return sessionhook.BuildClaudeEvent(packet, event)
}

// HandleClaudeLifecycleHook delivers passive context and observation only.
// It never enters the native agent-flow or permission-enforcement profile.
func (b *Bound) HandleClaudeLifecycleHook(event string, in io.Reader, out io.Writer) error {
	if event != "SessionStart" && event != "UserPromptSubmit" && event != "Stop" {
		return errors.New("hook_event_unsupported")
	}
	if err := b.validate(); err != nil {
		return errors.New("hook_binding_unavailable")
	}
	input, err := claudeadapter.ParseReader(in)
	if err != nil || claudeadapter.ValidateEvent(event, input) != nil {
		return errors.New("hook_input_invalid")
	}
	input.Prompt = ""
	if input.AgentID != "" || input.AgentType != "" {
		return errors.New("hook_hub_required")
	}
	root, err := filepath.EvalSymlinks(b.options.WorkspacePath)
	if err != nil {
		return errors.New("hook_scope_unavailable")
	}
	cwd, err := filepath.EvalSymlinks(input.CWD)
	if err != nil || !workspacepolicy.Within(root, cwd) {
		return errors.New("hook_scope_unavailable")
	}
	var value any = struct{}{}
	if event == "Stop" {
		if !input.StopHookActive {
			receipt, err := claudeadapter.Receipt(lifecycle.StopFinalize, claudeadapter.NativeInput{SessionID: input.SessionID})
			if err != nil {
				return errors.New("hook_receipt_unavailable")
			}
			receipt.IdempotencyKey = lifecycle.IdempotencyKey(receipt.IdempotencyKey, input.PromptID)
			if _, err = lifecycle.RecordConfined(filepath.Join(b.home, "state"), b.workspaceID, receipt); err != nil {
				return errors.New("hook_receipt_unavailable")
			}
		}
	} else {
		value, err = b.claudeLifecycleContext(event)
		if err != nil {
			return errors.New("hook_context_unavailable")
		}
	}
	body, err := json.Marshal(value)
	if err != nil || len(body) > 24<<10 {
		return errors.New("hook_output_unavailable")
	}
	body = append(body, '\n')
	if n, err := out.Write(body); err != nil || n != len(body) {
		return errors.New("hook_output_unavailable")
	}
	return nil
}
