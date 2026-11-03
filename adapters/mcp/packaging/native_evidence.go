package packaging

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/nativeagentflow"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

// Only native envelopes and the independently read canonical flow supply proof.
// Assistant text, sentinel absence and a self-reported summary are ignored.
type nativeProof struct {
	Protocol               string  `json:"protocol"`
	Fault                  string  `json:"fault"`
	StartupUnchanged       bool    `json:"startup_unchanged"`
	AllowedRead            bool    `json:"allowed_read"`
	GuardDenied            bool    `json:"guard_denied"`
	FaultAttempted         bool    `json:"fault_attempted"`
	FaultBlocked           bool    `json:"fault_blocked"`
	SequentialObservations bool    `json:"sequential_observations"`
	Cost                   float64 `json:"estimated_cost_usd"`
}
type nativeTool struct {
	name, path                                                                                  string
	result, denied, allowed, postObserved, hookFailure, blocked, errorResult, canonicalObserved bool
}
type nativeHook struct{ event, name, tool string }
type streamMessage struct {
	Type      string   `json:"type"`
	Subtype   string   `json:"subtype"`
	Session   string   `json:"session_id"`
	UUID      string   `json:"uuid"`
	HookID    string   `json:"hook_id"`
	HookName  string   `json:"hook_name"`
	HookEvent string   `json:"hook_event"`
	Stdout    string   `json:"stdout"`
	Stderr    string   `json:"stderr"`
	Output    string   `json:"output"`
	Outcome   string   `json:"outcome"`
	ExitCode  *int     `json:"exit_code"`
	Cost      *float64 `json:"total_cost_usd"`
	IsError   bool     `json:"is_error"`
	Message   struct {
		Content []struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Input     json.RawMessage `json:"input"`
			ToolUseID string          `json:"tool_use_id"`
			IsError   bool            `json:"is_error"`
			Content   json.RawMessage `json:"content"`
		} `json:"content"`
	} `json:"message"`
}

func verifyNativeProbe(body []byte, p nativeProbe, full bool) (nativeProof, error) {
	proof := nativeProof{Protocol: qualificationProtocol, Fault: p.fault}
	if len(body) > 2<<20 {
		return proof, errors.New("native stream too large")
	}
	tools := map[string]*nativeTool{}
	hooks := map[string]nativeHook{}
	seenMessages := map[string]bool{}
	active := ""
	starts, stops := 0, 0
	resultSeen := false
	stopSeen := false
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 4096), 256<<10)
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > 4096 {
			return proof, errors.New("native event limit")
		}
		var m streamMessage
		if json.Unmarshal(scanner.Bytes(), &m) != nil {
			return proof, errors.New("native stream invalid")
		}
		if m.Session != "" && m.Session != p.session {
			return proof, errors.New("native session mismatch")
		}
		if m.UUID != "" {
			if seenMessages[m.UUID] {
				return proof, errors.New("native message replay")
			}
			seenMessages[m.UUID] = true
		}
		switch m.Type {
		case "assistant":
			count := 0
			for _, content := range m.Message.Content {
				if content.Type != "tool_use" {
					continue
				}
				count++
				if count > 1 || content.ID == "" || tools[content.ID] != nil {
					return proof, errors.New("native parallel or replayed tool")
				}
				var input struct {
					Path         string `json:"file_path"`
					Background   bool   `json:"run_in_background"`
					SubagentType string `json:"subagent_type"`
				}
				if len(content.Input) > 16<<10 || json.Unmarshal(content.Input, &input) != nil || input.Background {
					return proof, errors.New("native tool input invalid")
				}
				path := input.Path
				if !filepath.IsAbs(path) {
					path = filepath.Join(p.binding.WorkspacePath, path)
				}
				switch content.Name {
				case "Read":
					if filepath.Clean(path) != filepath.Join(p.challenge, "allowed.txt") && (!full || filepath.Clean(path) != filepath.Clean(p.outside)) {
						return proof, errors.New("native read deviated from reviewed protocol")
					}
				case "Edit":
					if filepath.Clean(path) != filepath.Join(p.challenge, "edit.txt") {
						return proof, errors.New("native edit deviated from reviewed protocol")
					}
				case "Agent":
					if !full || (input.SubagentType != "maestro-product:case-agent" && input.SubagentType != "case-agent" && input.SubagentType != "maestro-product:gamma-guardian" && input.SubagentType != "gamma-guardian") {
						return proof, errors.New("native agent deviated from reviewed protocol")
					}
				default:
					return proof, errors.New("native tool deviated from reviewed protocol")
				}
				tools[content.ID] = &nativeTool{name: content.Name, path: input.Path}
				active = content.ID
			}
		case "user":
			for _, content := range m.Message.Content {
				if content.Type != "tool_result" {
					continue
				}
				tool := tools[content.ToolUseID]
				if tool == nil {
					return proof, errors.New("native uncorrelated tool result")
				}
				if tool.result {
					return proof, errors.New("native duplicate tool result")
				}
				tool.result = true
				if content.IsError {
					tool.errorResult = true
					marker := "native-fault"
					if p.fault == "missing" {
						marker = "absent-native-guard"
					}
					tool.blocked = bytes.Contains(content.Content, []byte(marker)) && bytes.Contains(content.Content, []byte(`blocking because onFailure`)) && (p.fault != "timeout" || bytes.Contains(bytes.ToLower(content.Content), []byte("timed out")))
				} else if tool.name == "Read" && bytes.Contains(content.Content, []byte("Synthetic native qualification control.")) {
					tool.allowed = true
				}
			}
		case "system":
			if m.Subtype == "hook_started" {
				if m.Session != p.session || m.UUID == "" || m.HookID == "" || m.HookName == "" || hooks[m.HookID].event != "" {
					return proof, errors.New("native hook identity invalid")
				}
				hooks[m.HookID] = nativeHook{m.HookEvent, m.HookName, active}
			}
			if m.Subtype != "hook_response" {
				continue
			}
			hook, ok := hooks[m.HookID]
			if !ok || m.Session != p.session || m.UUID == "" || hook.event != m.HookEvent || hook.name != m.HookName {
				return proof, errors.New("native uncorrelated hook response")
			}
			delete(hooks, m.HookID)
			if m.HookEvent == "SessionStart" && m.Outcome == "success" {
				var output struct {
					Message string `json:"systemMessage"`
				}
				if json.Unmarshal([]byte(m.Stdout), &output) == nil && output.Message == "maestro_native_startup_unchanged:"+p.nonce {
					proof.StartupUnchanged = true
				}
			}
			if m.HookEvent == "SubagentStart" && m.Outcome == "success" {
				starts++
			}
			if m.HookEvent == "SubagentStop" && m.Outcome == "success" {
				stops++
			}
			if m.HookEvent == "Stop" && m.Outcome == "success" {
				stopSeen = true
			}
			tool := tools[hook.tool]
			if tool == nil {
				continue
			}
			if m.HookEvent == "PreToolUse" && m.Outcome == "success" {
				var output struct {
					Message  string `json:"systemMessage"`
					Specific struct {
						Decision string `json:"permissionDecision"`
					} `json:"hookSpecificOutput"`
				}
				if json.Unmarshal([]byte(m.Stdout), &output) == nil && output.Message == "maestro_native_canonical_hook:"+p.nonce {
					tool.canonicalObserved = true
					if output.Specific.Decision == "deny" {
						tool.denied = true
					}
				}
			}
			if m.HookEvent == "PostToolUse" && m.Outcome == "success" && tool.name == "Read" {
				tool.postObserved = true
			}
			if m.HookEvent == "PostToolUse" && tool.name == "Edit" {
				return proof, errors.New("native failed guard allowed edit execution")
			}
			if m.HookEvent == "PreToolUse" && tool.name == "Edit" && (m.Outcome == "error" || m.Outcome == "cancelled") && (m.ExitCode != nil && *m.ExitCode != 0 || m.Outcome == "cancelled") {
				tool.hookFailure = true
			}

		case "result":
			if resultSeen || m.Session != p.session || m.Subtype != "success" || m.IsError || m.Cost == nil || math.IsNaN(*m.Cost) || math.IsInf(*m.Cost, 0) || *m.Cost < 0 {
				return proof, errors.New("native result unavailable")
			}
			proof.Cost = *m.Cost
			resultSeen = true
		}
	}
	if scanner.Err() != nil {
		return proof, scanner.Err()
	}
	for _, tool := range tools {
		path := tool.path
		if !filepath.IsAbs(path) {
			path = filepath.Join(p.binding.WorkspacePath, path)
		}
		if tool.name == "Read" && filepath.Clean(path) == filepath.Join(p.challenge, "allowed.txt") && tool.allowed && tool.canonicalObserved && tool.postObserved && tool.result && !tool.denied {
			proof.AllowedRead = true
		}
		if tool.name == "Read" && filepath.Clean(path) == filepath.Clean(p.outside) && tool.denied && tool.result && tool.errorResult {
			proof.GuardDenied = true
		}
		if tool.name == "Edit" && filepath.Clean(path) == filepath.Join(p.challenge, "edit.txt") {
			proof.FaultAttempted = true
			proof.FaultBlocked = tool.canonicalObserved && !tool.denied && tool.hookFailure && tool.blocked && tool.result
		}
	}
	if !resultSeen || !proof.StartupUnchanged || !proof.AllowedRead || !proof.FaultAttempted || !proof.FaultBlocked || !stopSeen {
		return proof, errors.New("native challenge incomplete")
	}
	// This is an additional effect control, never the positive blocking proof.
	edit, err := scheduler.ReadPrivateFile(filepath.Join(p.challenge, "edit.txt"), 128)
	if err != nil || string(edit) != "original\n" {
		return proof, errors.New("native refused edit had an effect")
	}

	h, err := datahome.Resolve(datahome.Options{ProductRoot: p.binding.ProductRoot, UserHome: p.binding.UserHome, LookupEnv: p.binding.LookupEnv})
	if err != nil {
		return proof, err
	}
	w, err := workspace.Inspect(p.binding.WorkspacePath, filepath.Join(h.Home, "state"))
	if err != nil {
		return proof, err
	}
	flow, err := nativeagentflow.NewBound(filepath.Join(h.Home, "state"), w.WorkspaceID)
	if err != nil {
		return proof, err
	}
	state, err := flow.BoundSnapshot(p.session)
	if err != nil {
		return proof, err
	}
	if state.ActiveAgentID != "" || state.PendingTool != "" || !state.Finalized {
		return proof, errors.New("native canonical route incomplete")
	}
	if full {
		if !proof.GuardDenied || starts != 2 || stops != 2 || !state.CaseStopObserved || state.LastAgentType != "gamma-guardian" || state.TransitionCount != 4 {
			return proof, errors.New("native route events incomplete")
		}
		proof.SequentialObservations = true
	}
	return proof, nil
}
