package packaging

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacepolicy"
)

// QualificationHook tags an unchanged canonical pre/post-tool result in the
// documented systemMessage field. It never supplies a decision of its own.
// Active product packets call hook directly and remain marker-free.
func QualificationHook(binding product.Options, nonce string, in io.Reader, out io.Writer) error {
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(nonce) {
		return errors.New("invalid qualification nonce")
	}
	bound, err := product.Open(binding)
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(in, claudeadapter.MaximumNativeInputBytes+1))
	if err != nil {
		return err
	}
	input, err := claudeadapter.ParseReader(bytes.NewReader(raw))
	if err != nil || (input.HookEventName != "PreToolUse" && input.HookEventName != "PostToolUse") || claudeadapter.ValidateEvent(input.HookEventName, input) != nil {
		return errors.New("qualification hook identity unavailable")
	}
	var body bytes.Buffer
	if err := bound.HandleClaudeHook(input.HookEventName, bytes.NewReader(raw), &body); err != nil {
		return err
	}
	var output map[string]any
	if json.Unmarshal(body.Bytes(), &output) != nil {
		return errors.New("canonical hook output unavailable")
	}
	output["systemMessage"] = qualificationHookMessage(input.HookEventName, input.SessionID, input.ToolUseID, input.ToolName, nonce)
	return json.NewEncoder(out).Encode(output)
}

// The canonical JSON array is length-delimited and binds only validated native
// identities. Neither tool content nor a raw identity enters the marker.
func qualificationHookMessage(event, session, tool, name, nonce string) string {
	body, _ := json.Marshal([6]string{"maestro.native.qualification-hook.v1", event, session, tool, name, nonce})
	return fmt.Sprintf("maestro_native_canonical_hook:v1:%x", sha256.Sum256(body))
}

// QualificationScopeGuard limits only the experiment's effects. Its permitted
// controls (including the public outside read and fault edit) return no decision
// and cannot establish the canonical guard's denial or failure-block evidence.
func QualificationScopeGuard(binding product.Options, directory, outside string, in io.Reader, out io.Writer) error {
	if _, err := product.Open(binding); err != nil {
		return err
	}
	if !filepath.IsAbs(directory) || !workspacepolicy.Within(binding.WorkspacePath, directory) || directory == binding.WorkspacePath || !filepath.IsAbs(outside) || workspacepolicy.Within(binding.WorkspacePath, outside) {
		return errors.New("qualification scope unavailable")
	}
	if err := scheduler.ValidatePrivateDirectory(directory); err != nil {
		return err
	}
	input, err := claudeadapter.ParseReader(in)
	if err != nil || claudeadapter.ValidateEvent("PreToolUse", input) != nil {
		return errors.New("qualification input unavailable")
	}
	var fields struct {
		Path         string `json:"file_path"`
		SubagentType string `json:"subagent_type"`
		Background   bool   `json:"run_in_background"`
		Old          string `json:"old_string"`
		New          string `json:"new_string"`
		Content      string `json:"content"`
	}
	if json.Unmarshal(input.ToolInputJSON(), &fields) != nil {
		return errors.New("qualification tool input unavailable")
	}
	allowed := false
	switch input.ToolName {
	case "Read", "Edit", "Write":
		path := fields.Path
		if !filepath.IsAbs(path) {
			path = filepath.Join(input.CWD, path)
		}
		resolved, err := workspacepolicy.ResolveProspective(path)
		if err == nil {
			if input.ToolName == "Read" {
				allowed = resolved == filepath.Join(directory, "allowed.txt") || resolved == outside
			}
			if input.ToolName == "Write" {
				allowed = resolved == filepath.Join(directory, "allowed-write.txt") && fields.Content == nativeAllowedWriteContent
			}
			if input.ToolName == "Edit" {
				allowed = resolved == filepath.Join(directory, "edit.txt") && (fields.Old == "original" || fields.Old == "original\n") && (fields.New == "changed" || fields.New == "changed\n")
			}
		}
	case "Agent":
		id, known := claudeagents.CanonicalAgentID(fields.SubagentType)
		allowed = known && fields.SubagentType == "maestro-product:"+id && (id == "case-agent" || id == "gamma-guardian") && !fields.Background
	}
	if !allowed {
		return json.NewEncoder(out).Encode(claudeadapter.FailClosedDenial())
	}
	return json.NewEncoder(out).Encode(struct{}{})
}
