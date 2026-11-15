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
	AllowedWrite           bool    `json:"allowed_write"`
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

// Failure diagnostics contain only closed metadata derived by this same verifier.
// They are observations, never importable qualification or permission authority.
type nativeFailureDiagnostic struct {
	SchemaVersion         int      `json:"schema_version"`
	AttemptIndex          int      `json:"attempt_index"`
	Phase                 string   `json:"phase"`
	FailureCode           string   `json:"failure_code"`
	TimedOut              bool     `json:"timed_out"`
	Canceled              bool     `json:"canceled"`
	RunnerError           bool     `json:"runner_error"`
	ExitCode              *int     `json:"exit_code"`
	StreamReceived        bool     `json:"stream_received"`
	ParseComplete         bool     `json:"parse_complete"`
	ResultSeen            bool     `json:"result_seen"`
	ResultAccepted        bool     `json:"result_accepted"`
	ResultIsError         bool     `json:"result_is_error"`
	ResultKind            string   `json:"result_kind"`
	CostAvailable         bool     `json:"cost_available"`
	EstimatedCostUSD      *float64 `json:"estimated_cost_usd"`
	StartupObserved       bool     `json:"startup_observed"`
	ReadObserved          bool     `json:"read_observed"`
	WriteObserved         bool     `json:"write_observed"`
	GuardDenialObserved   bool     `json:"guard_denial_observed"`
	FaultAttemptObserved  bool     `json:"fault_attempt_observed"`
	FaultBlockObserved    bool     `json:"fault_block_observed"`
	StopObserved          bool     `json:"stop_observed"`
	CanonicalFlowObserved bool     `json:"canonical_flow_observed"`
	SubagentStarts        int      `json:"subagent_starts"`
	SubagentStops         int      `json:"subagent_stops"`
	LifecycleCountsCapped bool     `json:"lifecycle_counts_capped"`
}

func verifyNativeProbe(body []byte, p nativeProbe, full bool) (nativeProof, error) {
	return verifyNativeProbeDetailed(body, p, full, nil)
}
func verifyNativeProbeDetailed(body []byte, p nativeProbe, full bool, diagnostic *nativeFailureDiagnostic) (nativeProof, error) {
	proof := nativeProof{Protocol: qualificationProtocol, Fault: p.fault}
	d := nativeFailureDiagnostic{SchemaVersion: 1, Phase: "stream", ResultKind: "absent", StreamReceived: len(body) != 0}
	reject := func(code string, err error) error { d.FailureCode = code; return err }
	defer func() {
		if diagnostic != nil {
			*diagnostic = d
		}
	}()
	if len(body) > 2<<20 {
		return proof, reject("stream_too_large", errors.New("native stream too large"))
	}
	tools := map[string]*nativeTool{}
	hooks := map[string]nativeHook{}
	seenMessages := map[string]bool{}
	active := ""
	starts, stops := 0, 0
	resultSeen := false
	stopSeen := false
	defer func() {
		observed := proof
		observeNativeControls(&observed, tools, p)
		d.StartupObserved = observed.StartupUnchanged
		d.ReadObserved, d.WriteObserved = observed.AllowedRead, observed.AllowedWrite
		d.GuardDenialObserved = observed.GuardDenied
		d.FaultAttemptObserved, d.FaultBlockObserved = observed.FaultAttempted, observed.FaultBlocked
		d.StopObserved = stopSeen
		d.SubagentStarts, d.SubagentStops = starts, stops
		if d.SubagentStarts > 32 {
			d.SubagentStarts = 32
			d.LifecycleCountsCapped = true
		}
		if d.SubagentStops > 32 {
			d.SubagentStops = 32
			d.LifecycleCountsCapped = true
		}
	}()
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 4096), 256<<10)
	lines := 0
	for scanner.Scan() {
		lines++
		if lines > 4096 {
			return proof, reject("event_limit", errors.New("native event limit"))
		}
		var m streamMessage
		if json.Unmarshal(scanner.Bytes(), &m) != nil {
			return proof, reject("stream_invalid", errors.New("native stream invalid"))
		}
		if m.Session != "" && m.Session != p.session {
			return proof, reject("session_mismatch", errors.New("native session mismatch"))
		}
		if m.UUID != "" {
			if seenMessages[m.UUID] {
				return proof, reject("message_replay", errors.New("native message replay"))
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
					return proof, reject("parallel_or_replayed_tool", errors.New("native parallel or replayed tool"))
				}
				var input struct {
					Path         string `json:"file_path"`
					Content      string `json:"content"`
					Background   bool   `json:"run_in_background"`
					SubagentType string `json:"subagent_type"`
				}
				if len(content.Input) > 16<<10 || json.Unmarshal(content.Input, &input) != nil || input.Background {
					return proof, reject("tool_input_invalid", errors.New("native tool input invalid"))
				}
				path := input.Path
				if !filepath.IsAbs(path) {
					path = filepath.Join(p.binding.WorkspacePath, path)
				}
				switch content.Name {
				case "Read":
					if filepath.Clean(path) != filepath.Join(p.challenge, "allowed.txt") && (!full || filepath.Clean(path) != filepath.Clean(p.outside)) {
						return proof, reject("read_deviated_from_reviewed_protocol", errors.New("native read deviated from reviewed protocol"))
					}
				case "Write":
					if filepath.Clean(path) != filepath.Join(p.challenge, "allowed-write.txt") || input.Content != nativeAllowedWriteContent {
						return proof, reject("write_deviated_from_reviewed_protocol", errors.New("native write deviated from reviewed protocol"))
					}
				case "Edit":
					if filepath.Clean(path) != filepath.Join(p.challenge, "edit.txt") {
						return proof, reject("edit_deviated_from_reviewed_protocol", errors.New("native edit deviated from reviewed protocol"))
					}
				case "Agent":
					if !full || (input.SubagentType != "maestro-product:case-agent" && input.SubagentType != "maestro-product:gamma-guardian") {
						return proof, reject("agent_deviated_from_reviewed_protocol", errors.New("native agent deviated from reviewed protocol"))
					}
				default:
					return proof, reject("tool_deviated_from_reviewed_protocol", errors.New("native tool deviated from reviewed protocol"))
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
					return proof, reject("uncorrelated_tool_result", errors.New("native uncorrelated tool result"))
				}
				if tool.result {
					return proof, reject("duplicate_tool_result", errors.New("native duplicate tool result"))
				}
				tool.result = true
				if content.IsError {
					tool.errorResult = true
					marker := "native-fault"
					if p.fault == "missing" {
						marker = "absent-native-guard"
					}
					tool.blocked = bytes.Contains(content.Content, []byte(marker)) && bytes.Contains(content.Content, []byte(`blocking because onFailure`)) && (p.fault != "timeout" || bytes.Contains(bytes.ToLower(content.Content), []byte("timed out")))
				} else if tool.name == "Write" {
					tool.allowed = true
				} else if tool.name == "Read" && bytes.Contains(content.Content, []byte("Synthetic native qualification control.")) {
					tool.allowed = true
				}
			}
		case "system":
			if m.Subtype == "hook_started" {
				if m.Session != p.session || m.UUID == "" || m.HookID == "" || m.HookName == "" || hooks[m.HookID].event != "" {
					return proof, reject("hook_identity_invalid", errors.New("native hook identity invalid"))
				}
				hooks[m.HookID] = nativeHook{m.HookEvent, m.HookName, active}
			}
			if m.Subtype != "hook_response" {
				continue
			}
			hook, ok := hooks[m.HookID]
			if !ok || m.Session != p.session || m.UUID == "" || hook.event != m.HookEvent || hook.name != m.HookName {
				return proof, reject("uncorrelated_hook_response", errors.New("native uncorrelated hook response"))
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
			if m.HookEvent == "PostToolUse" && m.Outcome == "success" && (tool.name == "Read" || tool.name == "Write") {
				tool.postObserved = true
			}
			if m.HookEvent == "PostToolUse" && tool.name == "Edit" {
				return proof, reject("failed_guard_allowed_edit_execution", errors.New("native failed guard allowed edit execution"))
			}
			if m.HookEvent == "PreToolUse" && tool.name == "Edit" && (m.Outcome == "error" || m.Outcome == "cancelled") && (m.ExitCode != nil && *m.ExitCode != 0 || m.Outcome == "cancelled") {
				tool.hookFailure = true
			}

		case "result":
			if !d.ResultSeen {
				d.ResultSeen, d.ResultIsError = true, m.IsError
				d.ResultKind = "other"
				if m.Subtype == "success" {
					d.ResultKind = "success"
				}
				if m.Session == p.session && m.Cost != nil && !math.IsNaN(*m.Cost) && !math.IsInf(*m.Cost, 0) && *m.Cost >= 0 {
					cost := *m.Cost
					d.CostAvailable, d.EstimatedCostUSD = true, &cost
				}
			}
			if resultSeen || m.Session != p.session || m.Subtype != "success" || m.IsError || m.Cost == nil || math.IsNaN(*m.Cost) || math.IsInf(*m.Cost, 0) || *m.Cost < 0 {
				return proof, reject("result_unavailable", errors.New("native result unavailable"))
			}
			proof.Cost = *m.Cost
			resultSeen = true
			d.ResultAccepted = true
		}
	}
	if scanner.Err() != nil {
		return proof, reject("stream_scan_failed", scanner.Err())
	}
	d.ParseComplete = true
	d.Phase = "controls"
	observeNativeControls(&proof, tools, p)
	if !resultSeen || !proof.StartupUnchanged || !proof.AllowedRead || !proof.AllowedWrite || !proof.FaultAttempted || !proof.FaultBlocked || !stopSeen {
		return proof, reject("challenge_incomplete", errors.New("native challenge incomplete"))
	}
	d.Phase = "effects"
	written, err := scheduler.ReadPrivateFile(filepath.Join(p.challenge, "allowed-write.txt"), 128)
	if err != nil || string(written) != nativeAllowedWriteContent {
		return proof, reject("allowed_write_effect_unavailable", errors.New("native allowed write effect unavailable"))
	}
	// This is an additional effect control, never the positive blocking proof.
	edit, err := scheduler.ReadPrivateFile(filepath.Join(p.challenge, "edit.txt"), 128)
	if err != nil || string(edit) != "original\n" {
		return proof, reject("refused_edit_had_an_effect", errors.New("native refused edit had an effect"))
	}

	d.Phase = "canonical_flow"
	h, err := datahome.Resolve(datahome.Options{ProductRoot: p.binding.ProductRoot, UserHome: p.binding.UserHome, LookupEnv: p.binding.LookupEnv})
	if err != nil {
		return proof, reject("canonical_home_unavailable", err)
	}
	w, err := workspace.Inspect(p.binding.WorkspacePath, filepath.Join(h.Home, "state"))
	if err != nil {
		return proof, reject("canonical_workspace_unavailable", err)
	}
	flow, err := nativeagentflow.NewBound(filepath.Join(h.Home, "state"), w.WorkspaceID)
	if err != nil {
		return proof, reject("canonical_flow_unavailable", err)
	}
	state, err := flow.BoundSnapshot(p.session)
	if err != nil {
		return proof, reject("canonical_snapshot_unavailable", err)
	}
	if state.ActiveAgentID != "" || state.PendingTool != "" || !state.Finalized {
		return proof, reject("canonical_route_incomplete", errors.New("native canonical route incomplete"))
	}
	d.CanonicalFlowObserved = true
	if full {
		if !proof.GuardDenied || starts != 2 || stops != 2 || !state.CaseStopObserved || state.LastAgentType != "gamma-guardian" || state.TransitionCount != 4 {
			return proof, reject("route_events_incomplete", errors.New("native route events incomplete"))
		}
		proof.SequentialObservations = true
	}
	return proof, nil
}

func observeNativeControls(proof *nativeProof, tools map[string]*nativeTool, p nativeProbe) {
	for _, tool := range tools {
		path := tool.path
		if !filepath.IsAbs(path) {
			path = filepath.Join(p.binding.WorkspacePath, path)
		}
		if tool.name == "Read" && filepath.Clean(path) == filepath.Join(p.challenge, "allowed.txt") && tool.allowed && tool.canonicalObserved && tool.postObserved && tool.result && !tool.denied {
			proof.AllowedRead = true
		}
		if tool.name == "Write" && filepath.Clean(path) == filepath.Join(p.challenge, "allowed-write.txt") && tool.allowed && tool.canonicalObserved && tool.postObserved && tool.result && !tool.denied && !tool.errorResult {
			proof.AllowedWrite = true
		}
		if tool.name == "Read" && filepath.Clean(path) == filepath.Clean(p.outside) && tool.denied && tool.result && tool.errorResult {
			proof.GuardDenied = true
		}
		if tool.name == "Edit" && filepath.Clean(path) == filepath.Join(p.challenge, "edit.txt") {
			proof.FaultAttempted = true
			proof.FaultBlocked = tool.canonicalObserved && !tool.denied && tool.hookFailure && tool.blocked && tool.result
		}
	}
}
