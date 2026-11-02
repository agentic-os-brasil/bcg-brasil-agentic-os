package claudeadapter

import (
	"encoding/json"
	"errors"
	"path/filepath"
)

func ValidateEvent(event string, input NativeInput) error {
	if input.HookEventName != event || !nativeIdentifierPattern.MatchString(input.SessionID) || !filepath.IsAbs(input.CWD) {
		return errors.New("hook_input_invalid")
	}
	if input.PromptID != "" && !nativeIdentifierPattern.MatchString(input.PromptID) {
		return errors.New("hook_prompt_identity_invalid")
	}
	if input.AgentID != "" && !nativeIdentifierPattern.MatchString(input.AgentID) {
		return errors.New("hook_agent_identity_invalid")
	}
	if input.AgentType != "" && !nativeIdentifierPattern.MatchString(input.AgentType) {
		return errors.New("hook_agent_identity_invalid")
	}
	if (event == "Stop" || event == "SubagentStop") && !input.hasStopHookActive {
		return errors.New("hook_stop_identity_invalid")
	}
	switch event {
	case "SessionStart", "UserPromptSubmit", "Stop":
	case "PreToolUse", "PostToolUse":
		var object map[string]json.RawMessage
		if !nativeIdentifierPattern.MatchString(input.ToolName) || !nativeIdentifierPattern.MatchString(input.ToolUseID) || json.Unmarshal(input.ToolInputJSON(), &object) != nil || object == nil {
			return errors.New("hook_tool_identity_invalid")
		}
	case "SubagentStart":
		if input.AgentID == "" || input.AgentType == "" {
			return errors.New("hook_agent_identity_invalid")
		}
	case "SubagentStop":
		// Internal host agents can have an empty type; this grants no managed authority.
		if input.AgentID == "" {
			return errors.New("hook_agent_identity_invalid")
		}
	default:
		return errors.New("hook_event_unsupported")
	}
	return nil
}
