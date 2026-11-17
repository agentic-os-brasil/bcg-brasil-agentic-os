package packaging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func nativeFixture(t *testing.T) NativeOptions {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("native owner admission unavailable")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prod, owner, ws := filepath.Join(root, "product"), filepath.Join(root, "owner"), filepath.Join(root, "workspace")
	for _, p := range []string{prod, owner} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	lookup := func(string) (string, bool) { return "", false }
	h, err := datahome.InitializeFresh(datahome.Options{ProductRoot: prod, UserHome: owner, LookupEnv: lookup})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: ws, DataRoot: filepath.Join(h.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	server, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	scratch := filepath.Join(owner, "native-scratch")
	if err := os.Mkdir(scratch, 0700); err != nil {
		t.Fatal(err)
	}
	return NativeOptions{Packet: Options{Destination: filepath.Join(root, "plugin"), Server: server, Version: "0.2.0", Binding: product.Options{ProductRoot: prod, UserHome: owner, WorkspacePath: ws, LookupEnv: lookup}}, Executable: server, ScratchRoot: scratch, ChallengeDirectory: filepath.Join(ws, "native-challenge"), MaxBudgetUSD: 0.2}
}
func inspectionRunner(t *testing.T, authenticated bool, print func(context.Context, []string, string) ([]byte, error)) nativeCommand {
	t.Helper()
	profile, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("MAESTRO_CONTROLLED_PROFILE", profile)
	if err := os.WriteFile(filepath.Join(profile, "settings.json"), []byte(`{"autoMemoryEnabled":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	return func(ctx context.Context, path string, args []string, cwd string) ([]byte, error) {
		if len(args) == 1 && args[0] == "--version" {
			return []byte("2.1.295 (Claude Code)\n"), nil
		}
		if len(args) == 2 && args[0] == "auth" && args[1] == "status" {
			body, err := json.Marshal(map[string]any{"loggedIn": authenticated, "authMethod": "claude.ai", "configDirectory": profile})
			return body, err
		}
		if print == nil {
			t.Fatalf("read-only inspection launched a native session: %v", args)
		}
		return print(ctx, args, cwd)
	}
}
func TestNativeReviewRequiresExactConfirmationWithoutWrites(t *testing.T) {
	in := nativeFixture(t)
	run := inspectionRunner(t, true, nil)
	before, err := startupSnapshot(in.Packet.Binding)
	if err != nil {
		t.Fatal(err)
	}
	review, err := reviewNative(context.Background(), in, run)
	if err != nil {
		t.Fatal(err)
	}
	if review.Plan.State != "reviewable" || review.Plan.MaximumCalls != 3 || review.Plan.MaximumTurnsPerCall != 8 || review.Plan.MaximumSecondsPerCall != 120 || review.Plan.AggregateEstimatedLimitUSD != 3*in.MaxBudgetUSD || review.ReviewDigest == "" {
		t.Fatalf("invalid exact plan: %+v", review)
	}
	if !strings.Contains(review.Plan.BillingBoundary, "not hard") || !strings.Contains(review.Plan.ContextBoundary, "additional provider requests") || review.Plan.ChallengeDirectory != in.ChallengeDirectory {
		t.Fatal("plan concealed billing/context/write boundaries")
	}
	after, err := startupSnapshot(in.Packet.Binding)
	if err != nil || before != after {
		t.Fatal("review changed admitted state")
	}
	for _, confirmed := range []bool{false, true} {
		digest := "wrong"
		if !confirmed {
			digest = review.ReviewDigest
		}
		if _, err := qualifyAndBuildNative(context.Background(), in, digest, confirmed, run, "engineering_fixture"); err == nil {
			t.Fatal("unreviewed native effects admitted")
		}
	}
	for _, p := range []string{in.Packet.Destination, in.ChallengeDirectory} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatal("unreviewed output created")
		}
	}
	entries, err := os.ReadDir(in.ScratchRoot)
	if err != nil || len(entries) != 0 {
		t.Fatal("review/decline created scratch")
	}
	for _, budget := range []float64{0, -1, 1.01, math.NaN(), math.Inf(1)} {
		changed := in
		changed.MaxBudgetUSD = budget
		if _, err := reviewNative(context.Background(), changed, run); err == nil {
			t.Fatal("invalid reviewed budget accepted")
		}
	}
	settings := filepath.Join(in.Packet.Binding.WorkspacePath, ".claude", "settings.local.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(settings, []byte(`{"disableAllHooks":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := qualifyAndBuildNative(context.Background(), in, review.ReviewDigest, true, run, "engineering_fixture"); err == nil {
		t.Fatal("changed target configuration accepted old plan")
	}
}
func TestNativeUnavailableProducesMCPOnlyWithoutScratch(t *testing.T) {
	in := nativeFixture(t)
	run := inspectionRunner(t, false, nil)
	review, err := reviewNative(context.Background(), in, run)
	if err != nil {
		t.Fatal(err)
	}
	result, err := qualifyAndBuildNative(context.Background(), in, review.ReviewDigest, true, run, "engineering_fixture")
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "unavailable" || result.Reason != "native_authentication_unavailable" || result.CallsAttempted != 0 {
		t.Fatalf("incorrect unavailable result: %+v", result)
	}
	for _, p := range []string{in.ChallengeDirectory, filepath.Join(in.Packet.Destination, "hooks")} {
		if _, err := os.Lstat(p); !os.IsNotExist(err) {
			t.Fatalf("unavailable capability created %s", p)
		}
	}
	entries, err := os.ReadDir(in.ScratchRoot)
	if err != nil || len(entries) != 0 {
		t.Fatal("unavailable qualification allocated scratch")
	}
	var packet nativePacketManifest
	body, err := os.ReadFile(filepath.Join(in.Packet.Destination, "maestro-packet.json"))
	if err != nil || json.Unmarshal(body, &packet) != nil || packet.Mode != "mcp_only" || packet.Native.Reason != result.Reason {
		t.Fatal("missing useful MCP-only fallback")
	}
}

func argValue(args []string, key string) string {
	for i, value := range args {
		if value == key && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}
func probeFromArguments(t *testing.T, in NativeOptions, args []string, cwd string) nativeProbe {
	t.Helper()
	plugin := argValue(args, "--plugin-dir")
	root := filepath.Dir(plugin)
	var config struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Args []string `json:"args"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	body, err := os.ReadFile(filepath.Join(plugin, "hooks/hooks.json"))
	if err != nil || json.Unmarshal(body, &config) != nil {
		t.Fatal("candidate hooks missing")
	}
	var packet packetInventory
	inventoryBody, err := os.ReadFile(filepath.Join(plugin, InventoryName))
	if err != nil || json.Unmarshal(inventoryBody, &packet) != nil {
		t.Fatal("candidate inventory missing")
	}
	matched := false
	for _, file := range packet.Files {
		if file.Path == "hooks/hooks.json" {
			matched = file.SHA256 == digest(body)
		}
	}
	if !matched {
		t.Fatal("candidate inventory describes pre-qualification hook bytes")
	}
	index := filepath.Base(root)
	fault := map[string]string{"0": "missing", "1": "crash", "2": "timeout"}[index]
	if cwd != in.Packet.Binding.WorkspacePath || argValue(args, "--max-turns") != "8" || argValue(args, "--max-budget-usd") != "0.2" || argValue(args, "--restricted") != "" {
		t.Fatal("candidate changed target/turn/budget/settings authority")
	}
	for _, value := range args {
		if value == "--restricted" || value == "--allowedTools" || value == "--dangerously-skip-permissions" {
			t.Fatal("qualification bypassed effective settings or permissions")
		}
	}
	return nativeProbe{root: root, challenge: filepath.Join(in.ChallengeDirectory, index), outside: in.Packet.Server, plugin: plugin, fault: fault, nonce: argValue(config.Hooks["SessionStart"][0].Hooks[0].Args, "--nonce"), session: argValue(args, "--session-id"), binding: in.Packet.Binding, server: in.Packet.Server}
}

// Build native-shaped envelopes around actual canonical adapter transitions.
// These remain engineering fixtures, including when carried by a real cat
// subprocess; the public producer has no injection callback or fixture token.
func syntheticNativeStream(t *testing.T, p nativeProbe) []byte {
	t.Helper()
	bound, err := product.Open(p.binding)
	if err != nil {
		t.Fatal(err)
	}
	var stream bytes.Buffer
	sequence := 0
	emit := func(m map[string]any) {
		sequence++
		m["uuid"] = fmt.Sprintf("synthetic-%d", sequence)
		m["session_id"] = p.session
		if err := json.NewEncoder(&stream).Encode(m); err != nil {
			t.Fatal(err)
		}
	}
	base := func(event string, extra map[string]any) []byte {
		m := map[string]any{"hook_event_name": event, "session_id": p.session, "cwd": p.binding.WorkspacePath, "stop_hook_active": false}
		for k, v := range extra {
			m[k] = v
		}
		body, _ := json.Marshal(m)
		return body
	}
	hook := func(event string, extra map[string]any) {
		id := fmt.Sprintf("hook-%d", sequence)
		name := "maestro:" + event
		emit(map[string]any{"type": "system", "subtype": "hook_started", "hook_id": id, "hook_name": name, "hook_event": event})
		var out bytes.Buffer
		var err error
		if event == "SessionStart" {
			err = CheckNativeStartup(p.binding, p.nonce, bytes.NewReader(base(event, extra)), &out)
		} else if event == "PreToolUse" {
			var scoped bytes.Buffer
			if err := QualificationScopeGuard(p.binding, p.challenge, p.outside, bytes.NewReader(base(event, extra)), &scoped); err != nil {
				t.Fatal(err)
			}
			if string(bytes.TrimSpace(scoped.Bytes())) != "{}" {
				t.Fatalf("scope instrumentation masked a reviewed control: %s", scoped.Bytes())
			}
			err = QualificationHook(p.binding, p.nonce, bytes.NewReader(base(event, extra)), &out)
		} else {
			err = bound.HandleClaudeHook(event, bytes.NewReader(base(event, extra)), &out)
		}
		if err != nil {
			t.Fatalf("actual %s adapter: %v", event, err)
		}
		emit(map[string]any{"type": "system", "subtype": "hook_response", "hook_id": id, "hook_name": name, "hook_event": event, "outcome": "success", "exit_code": 0, "stdout": out.String()})
	}
	tool := func(id, name string, input map[string]any) {
		emit(map[string]any{"type": "assistant", "message": map[string]any{"content": []any{map[string]any{"type": "tool_use", "id": id, "name": name, "input": input}}}})
	}
	result := func(id string, failed bool, content string) {
		emit(map[string]any{"type": "user", "message": map[string]any{"content": []any{map[string]any{"type": "tool_result", "tool_use_id": id, "is_error": failed, "content": content}}}})
	}
	fields := func(id, name string, input map[string]any) map[string]any {
		return map[string]any{"tool_name": name, "tool_use_id": id, "tool_input": input}
	}
	hook("SessionStart", nil)
	hook("UserPromptSubmit", nil)
	input := map[string]any{"file_path": filepath.Join(p.challenge, "allowed.txt")}
	tool("read", "Read", input)
	hook("PreToolUse", fields("read", "Read", input))
	hook("PostToolUse", fields("read", "Read", input))
	result("read", false, "Synthetic native qualification control.")
	input = map[string]any{"file_path": filepath.Join(p.challenge, "allowed-write.txt"), "content": "Synthetic native allowed write control.\n"}
	tool("allowed-write", "Write", input)
	hook("PreToolUse", fields("allowed-write", "Write", input))
	if err := os.WriteFile(filepath.Join(p.challenge, "allowed-write.txt"), []byte("Synthetic native allowed write control.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	hook("PostToolUse", fields("allowed-write", "Write", input))
	result("allowed-write", false, "Synthetic allowed file written.")
	if p.fault == "missing" {
		input = map[string]any{"file_path": p.outside}
		tool("denied", "Read", input)
		hook("PreToolUse", fields("denied", "Read", input))
		result("denied", true, "Policy denied the outside path.")
		for _, id := range []string{"case-agent", "gamma-guardian"} {
			input = map[string]any{"subagent_type": "maestro-product:" + id}
			tool(id, "Agent", input)
			hook("PreToolUse", fields(id, "Agent", input))
			agent := map[string]any{"agent_id": "native-" + id, "agent_type": "maestro-product:" + id}
			hook("SubagentStart", agent)
			hook("SubagentStop", agent)
			hook("PostToolUse", fields(id, "Agent", input))
			result(id, false, "Synthetic observation only.")
		}
	}
	input = map[string]any{"file_path": filepath.Join(p.challenge, "edit.txt"), "old_string": "original", "new_string": "changed"}
	tool("edit", "Edit", input)
	hook("PreToolUse", fields("edit", "Edit", input))
	marker := "native-fault --fault " + p.fault
	if p.fault == "missing" {
		marker = "absent-native-guard"
	}
	emit(map[string]any{"type": "system", "subtype": "hook_started", "hook_id": "fault", "hook_name": marker, "hook_event": "PreToolUse"})
	emit(map[string]any{"type": "system", "subtype": "hook_response", "hook_id": "fault", "hook_name": marker, "hook_event": "PreToolUse", "outcome": "error", "exit_code": 1, "stderr": "synthetic fixed helper failure"})
	notice := marker + `: failed; blocking because onFailure is "block"`
	if p.fault == "timeout" {
		notice = marker + `: timed out; blocking because onFailure is "block"`
	}
	result("edit", true, notice)
	hook("Stop", nil)
	emit(map[string]any{"type": "result", "subtype": "success", "is_error": false, "total_cost_usd": 0.01})
	return stream.Bytes()
}

type nativeDiagnosticExitError struct{}

func (nativeDiagnosticExitError) Error() string { return "PRIVATE_TYPED_ERROR /private/credential" }
func (nativeDiagnosticExitError) ExitCode() int { return 7 }

func TestNativeControlledSessionsProduceOnlyEngineeringPacket(t *testing.T) {
	t.Run("plain", func(t *testing.T) { testNativeControlledSessions(t, false) })
	t.Run("managed", func(t *testing.T) { testNativeControlledSessions(t, true) })
}
func testNativeControlledSessions(t *testing.T, managed bool) {
	in := nativeFixture(t)
	sessions := 0
	run := inspectionRunner(t, true, func(ctx context.Context, args []string, cwd string) ([]byte, error) {
		sessions++
		p := probeFromArguments(t, in, args, cwd)
		wantSettings := ""
		if managed {
			wantSettings = `{"enabledPlugins":{"maestro-product@managed":false}}`
		}
		if argValue(args, "--settings") != wantSettings {
			t.Fatal("qualification session changed its exact reviewed plugin override")
		}
		stream := syntheticNativeStream(t, p)
		path := filepath.Join(p.root, "synthetic-stream.jsonl")
		if err := os.WriteFile(path, stream, 0600); err != nil {
			t.Fatal(err)
		}
		return runNativeCommand(ctx, "/bin/cat", []string{path}, cwd)
	})
	if managed {
		installManagedPluginFixture(t, in)
	}
	review, err := reviewNative(context.Background(), in, run)
	if err != nil {
		t.Fatal(err)
	}
	result, err := qualifyAndBuildNative(context.Background(), in, review.ReviewDigest, true, run, "engineering_fixture")
	if err != nil {
		t.Fatal(err)
	}
	if sessions != 3 || result.CallsAttempted != 3 || result.State != "engineering_fixture" || result.EvidenceClass != "engineering_fixture" {
		t.Fatalf("synthetic sessions promoted to native or incomplete: %+v", result)
	}
	if managed && (result.NativeConfigurationSHA256 != review.Plan.NativeConfigurationSHA256 || result.SessionSettingsSHA256 == "") {
		t.Fatal("result lost its reviewed source/override binding")
	}
	body, err := os.ReadFile(filepath.Join(in.Packet.Destination, "maestro-packet.json"))
	if err != nil {
		t.Fatal(err)
	}
	var packet nativePacketManifest
	if json.Unmarshal(body, &packet) != nil || packet.Mode != "native_engineering_fixture" {
		t.Fatal("fixture evidence label absent")
	}
	for _, id := range []string{"case-agent", "client-account-agent", "darwin", "gamma-guardian", "pa-expert", "repository-spec-reviewer", "repository-standards-reviewer", "yoda"} {
		if _, err := os.Stat(filepath.Join(in.Packet.Destination, "agents", id+".md")); err != nil {
			t.Fatal(err)
		}
	}
	config, err := os.ReadFile(filepath.Join(in.Packet.Destination, "hooks/hooks.json"))
	if err != nil || !bytes.Contains(config, []byte(`"onFailure": "block"`)) || !bytes.Contains(config, []byte("${user_config.workspace}")) || bytes.Contains(config, []byte(in.Packet.Binding.WorkspacePath)) {
		t.Fatal("qualified packet lost native exec/binding")
	}
	other := in.Packet
	other.Binding.WorkspacePath = filepath.Dir(in.Packet.Binding.WorkspacePath)
	if _, _, err := prepare(other); err == nil {
		t.Fatal("unrelated target admitted")
	}
	if !result.EstimatedSpendComplete || result.EstimatedSpendSessions != 3 {
		t.Fatal("verified native estimates lost their availability boundary")
	}
	if !managed {
		for _, kind := range []string{"transport", "typed_exit", "result", "partial"} {
			failed := nativeFixture(t)
			calls := 0
			run := inspectionRunner(t, true, func(ctx context.Context, args []string, cwd string) ([]byte, error) {
				calls++
				if kind == "transport" {
					return []byte("PRIVATE_STREAM https://private.invalid/prompt"), errors.New("PRIVATE_ERROR auth=/private/credential")
				}
				if kind == "typed_exit" {
					return nil, nativeDiagnosticExitError{}
				}
				p := probeFromArguments(t, failed, args, cwd)
				stream := syntheticNativeStream(t, p)
				if kind == "result" {
					return bytes.Replace(stream, []byte(`"subtype":"success"`), []byte(`"subtype":"PRIVATE_SUBTYPE"`), 1), nil
				}
				if calls == 2 {
					return bytes.ReplaceAll(stream, []byte("Synthetic native qualification control."), []byte("PRIVATE_CONTENT https://private.invalid")), nil
				}
				return stream, nil
			})
			review, err := reviewNative(context.Background(), failed, run)
			if err != nil {
				t.Fatal(err)
			}
			result, err := qualifyAndBuildNative(context.Background(), failed, review.ReviewDigest, true, run, "engineering_fixture")
			wantCalls := 1
			if kind == "partial" {
				wantCalls = 2
			}
			if err != nil || result.State != "unavailable" || result.EvidenceClass != "not_observed" || calls != wantCalls || result.EstimatedSpendComplete || result.EstimatedSpendSessions != wantCalls-1 {
				t.Fatalf("failed %s session changed fallback/cost availability or retried: %+v %v", kind, result, err)
			}
			path := filepath.Join(result.ScratchDirectory, fmt.Sprint(calls-1), "failure.json")
			body, err := scheduler.ReadPrivateFile(path, 4096)
			if err != nil {
				t.Fatal("native session failure has no bounded diagnostic receipt", err)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0600 || len(body) > 4096 || bytes.Contains(body, []byte("PRIVATE_")) || bytes.Contains(body, []byte("https://")) || bytes.Contains(body, []byte("/private/")) {
				t.Fatal("native failure diagnostic lost privacy or bounds")
			}
			var diagnostic map[string]any
			if json.Unmarshal(body, &diagnostic) != nil {
				t.Fatal("invalid diagnostic JSON")
			}
			if _, exists := diagnostic["exit_code"]; !exists {
				t.Fatal("unknown exit field absent")
			}
			if kind == "transport" {
				if diagnostic["exit_code"] != nil || diagnostic["phase"] != "execution" || diagnostic["cost_available"] != false || diagnostic["estimated_cost_usd"] != nil {
					t.Fatal("unknown runner exit/cost became fabricated observations")
				}
			} else if kind == "typed_exit" {
				if diagnostic["exit_code"] != float64(7) || diagnostic["cost_available"] != false {
					t.Fatal("preserved typed exit lost its known code")
				}
			} else if diagnostic["exit_code"] != float64(0) || diagnostic["cost_available"] != true {
				t.Fatal("returned native stream lost known exit/cost availability")
			}
			if kind == "result" && (diagnostic["result_kind"] != "other" || diagnostic["result_seen"] != true || diagnostic["result_accepted"] != false) {
				t.Fatal("rejected result lost safe closed diagnostic kind")
			}
			if kind == "partial" && (diagnostic["phase"] != "controls" || diagnostic["read_observed"] != false || diagnostic["write_observed"] != true || result.EstimatedSpendUSD != 0.01) {
				t.Fatal("partial controls or verified-only aggregate became misleading")
			}
			packet, err := os.ReadFile(filepath.Join(failed.Packet.Destination, "maestro-packet.json"))
			var manifest nativePacketManifest
			if err != nil || json.Unmarshal(packet, &manifest) != nil || manifest.Mode != "mcp_only" {
				t.Fatal("failure diagnostic promoted native authority or removed MCP fallback")
			}
		}
	}
}
func TestNativeProofRejectsAssistantClaimsAndMissingPositiveEvidence(t *testing.T) {
	in := nativeFixture(t)
	o, _, err := prepare(in.Packet)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(in.ScratchRoot, "evidence")
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(in.ChallengeDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	p, err := prepareNativeProbe(o, root, in.ChallengeDirectory, 0, "missing")
	if err != nil {
		t.Fatal(err)
	}
	body := syntheticNativeStream(t, p)
	proof, err := verifyNativeProbe(body, p, true)
	if err != nil {
		t.Fatal(err)
	}
	var diagnostic nativeFailureDiagnostic
	detailed, err := verifyNativeProbeDetailed(body, p, true, &diagnostic)
	if err != nil || detailed != proof || !diagnostic.ResultAccepted || !diagnostic.CostAvailable || diagnostic.EstimatedCostUSD == nil || !diagnostic.CanonicalFlowObserved {
		t.Fatal("diagnostic path changed accepted proof or lost observations")
	}
	zeroCost := bytes.ReplaceAll(body, []byte(`"total_cost_usd":0.01`), []byte(`"total_cost_usd":0`))
	if _, err := verifyNativeProbeDetailed(zeroCost, p, true, &diagnostic); err != nil || !diagnostic.CostAvailable || diagnostic.EstimatedCostUSD == nil || *diagnostic.EstimatedCostUSD != 0 {
		t.Fatal("known zero estimate became an unavailable cost")
	}
	missingCost := bytes.ReplaceAll(body, []byte(`,"total_cost_usd":0.01`), nil)
	if _, err := verifyNativeProbeDetailed(missingCost, p, true, &diagnostic); err == nil || diagnostic.CostAvailable || diagnostic.EstimatedCostUSD != nil || diagnostic.ResultAccepted {
		t.Fatal("missing estimate became accepted or a fabricated zero cost")
	}
	encodedProof, err := json.Marshal(proof)
	if err != nil || !bytes.Contains(encodedProof, []byte(`"allowed_write":true`)) {
		t.Fatal("native qualification accepted without an actual permitted write proof")
	}
	bare := bytes.ReplaceAll(body, []byte(`"subagent_type":"maestro-product:case-agent"`), []byte(`"subagent_type":"case-agent"`))
	if bytes.Equal(body, bare) {
		t.Fatal("closed-plugin fixture has no namespaced dispatch")
	}
	if _, err := verifyNativeProbe(bare, p, true); err == nil {
		t.Fatal("qualification accepted an unclosed bare agent definition")
	}
	for _, tc := range []struct{ name, remove string }{{"no_block", "blocking because onFailure"}, {"no_attempt", `"name":"Edit"`}, {"no_native_startup", "maestro_native_startup_unchanged:"}, {"no_positive_read", "Synthetic native qualification control."}, {"no_positive_write", `"name":"Write"`}, {"wrong_session", p.session}} {
		t.Run(tc.name, func(t *testing.T) {
			changed := bytes.ReplaceAll(body, []byte(tc.remove), []byte("UNAVAILABLE"))
			var d nativeFailureDiagnostic
			if _, err := verifyNativeProbeDetailed(changed, p, true, &d); err == nil || d.FailureCode == "" {
				t.Fatal("incomplete native evidence accepted or lost safe failure code")
			}
			encoded, err := json.Marshal(d)
			if err != nil || bytes.Contains(encoded, []byte(p.session)) || bytes.Contains(encoded, []byte(p.challenge)) || bytes.Contains(encoded, []byte(p.nonce)) {
				t.Fatal("diagnostic retained native identity or payload paths")
			}
		})
	}
	claims := []byte(`{"type":"assistant","message":{"content":[{"type":"text","text":"PASS: all native hooks, controls, blockers and correlations succeeded"}]}}`)
	if _, err := verifyNativeProbeDetailed(claims, p, true, &diagnostic); err == nil || diagnostic.ResultSeen || diagnostic.CostAvailable || diagnostic.EstimatedCostUSD != nil || diagnostic.Phase != "controls" {
		t.Fatal("assistant prose minted admission or fabricated result/cost")
	}
	if _, err := verifyNativeProbeDetailed([]byte("PRIVATE_BAD_JSON"), p, true, &diagnostic); err == nil || diagnostic.Phase != "stream" || diagnostic.ParseComplete || diagnostic.CostAvailable {
		t.Fatal("malformed stream lost its bounded failure phase")
	}
	if err := os.WriteFile(filepath.Join(p.challenge, "allowed-write.txt"), []byte("different\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyNativeProbe(body, p, true); err == nil {
		t.Fatal("positive-looking write events hid a missing physical effect")
	}
	if err := os.WriteFile(filepath.Join(p.challenge, "allowed-write.txt"), []byte("Synthetic native allowed write control.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(p.challenge, "edit.txt"), []byte("changed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyNativeProbe(body, p, true); err == nil {
		t.Fatal("positive-looking events hid an actual refused effect")
	}
}

func TestNativeCommandRunnerBoundsOrdinaryOwnedProcessGroup(t *testing.T) {
	// io.Copy may prefer a promoted ReaderFrom over Write; the same native
	// output cap must hold for streaming readers without a WriterTo method.
	var output boundedNativeOutput
	read := io.LimitReader(strings.NewReader(strings.Repeat("x", (2<<20)+1)), (2<<20)+1)
	if n, err := io.Copy(&output, read); err == nil || n > 2<<20 || len(output.Bytes()) > 2<<20 {
		t.Fatal("native output stream bypassed its bounded writer")
	}
	if runtime.GOOS == "windows" {
		t.Skip("native process group boundary unavailable")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := runNativeCommand(ctx, "/bin/sh", []string{"-c", "sleep 5 & wait"}, "")
	if err == nil || time.Since(start) > 3*time.Second {
		t.Fatal("qualification-owned ordinary process group outlived timeout")
	}
	// A detached host process is deliberately outside this test and the stated
	// guarantee. The producer reports timeout cleanup unavailable, never PASS.
}
