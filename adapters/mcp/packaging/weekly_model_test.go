package packaging

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/yodaselfreview"
)

func TestWeeklyCLIEnvelopeRejectsExecutionAndUnsupportedShapes(t *testing.T) {
	d := maintenance.WeeklyModelDescriptor{RuntimeVersion: "synthetic-version", Model: "synthetic-model"}
	for _, body := range []string{
		`{"type":"system","subtype":"hook_started"}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Agent"}]}}`,
		`{"type":"result","subtype":"success","is_error":false,"num_turns":1,"structured_output":{}}`,
		`{"type":"result","type":"system"}`,
		`{"type":"unknown"}`,
		`plain text`,
	} {
		if _, err := parseWeeklyCLIEnvelope([]byte(body), d, yodaselfreview.WeeklyCompactionInput{}); err == nil {
			t.Fatal("unsafe/unbound envelope accepted")
		}
	}
	if _, err := NewWeeklyCLIAdapter(WeeklyCLIOptions{}); err == nil {
		t.Fatal("missing exact descriptor/authority accepted")
	}
	adapter := &WeeklyCLIAdapter{}
	if _, err := adapter.Compact(context.Background(), yodaselfreview.WeeklyCompactionInput{}); err == nil {
		t.Fatal("empty adapter invoked")
	}
}

func TestWeeklyCLIClosedEnvironmentAndBounds(t *testing.T) {
	for _, env := range [][]string{{"ANTHROPIC_API_KEY=synthetic"}, {"AWS_SECRET_ACCESS_KEY=synthetic"}, {"CLAUDE_CODE_MAX_RETRIES=15"}, {"PATH=/one", "PATH=/two"}, {"UNKNOWN=value"}} {
		if _, err := weeklyCLIEnvironment(env, 4096); err == nil {
			t.Fatal("ambient/secret/override environment accepted")
		}
	}
	env, err := weeklyCLIEnvironment([]string{"HOME=/synthetic/owner", "PATH=/synthetic/bin"}, 4096)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(env, "\n")
	for _, v := range []string{"CLAUDE_CODE_MAX_RETRIES=0", "CLAUDE_CODE_MAX_OUTPUT_TOKENS=4096", "CLAUDE_CODE_RETRY_WATCHDOG=0", "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1", "DISABLE_AUTO_COMPACT=1", "ENABLE_CLAUDEAI_MCP_SERVERS=false"} {
		if !strings.Contains(joined, v) {
			t.Fatal("missing hard finite-attempt/token setting")
		}
	}
	var b weeklyModelOutput
	b.limit = 4
	if _, err := b.Write([]byte("1234")); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Write([]byte("5")); err == nil {
		t.Fatal("output bound bypassed")
	}
	if string(b.Bytes()) != "1234" {
		t.Fatal("overflow retained private bytes")
	}
	var _ yodaselfreview.WeeklyCompactionAdapter = (*WeeklyCLIAdapter)(nil)
}

func weeklyCLIInputFixture(t *testing.T) (yodaselfreview.WeeklyCompactionInput, []byte) {
	t.Helper()
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "owner/self/communication-style.md"), []byte("# Communication style\n\n## Current\n\nUse concise synthetic text.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	options := ownerctx.WeeklySelfOptions{OccurrenceID: strings.Repeat("a", 64), OwnerID: "synthetic-owner", ScheduledFor: now, Facets: []string{"communication-style"}, Limits: ownerctx.DefaultWeeklySelfLimits(), WorkingLanguage: "en", TranslatorID: "synthetic-identity", TranslatorVersion: "1"}
	config := ownerctx.DefaultPromptHistoryConfig()
	config.OwnerID = options.OwnerID
	if err := ownerctx.ConfigurePromptHistory(root, config); err != nil {
		t.Fatal(err)
	}
	frozen, err := ownerctx.FreezeWeeklySelf(root, options, func(text, _, _ string) (string, error) { return text, nil }, now)
	if err != nil {
		t.Fatal(err)
	}
	input, err := yodaselfreview.BuildWeeklyCompactionInput(frozen)
	if err != nil {
		t.Fatal(err)
	}
	candidate := ownerctx.WeeklySelfCandidate{SchemaVersion: 1, InputSHA256: input.InputSHA256, CanonicalSHA256: input.CanonicalSHA256, PromptHistorySHA256: input.PromptHistorySHA256, PolicySHA256: input.PolicySHA256, TranslationSHA256: input.TranslationSHA256, Facets: map[string]string{"communication-style": "Bounded synthetic working text."}}
	body, _ := json.Marshal(candidate)
	return input, body
}

func TestWeeklyCLICompleteEnvelopeUsesOwnerValidation(t *testing.T) {
	input, candidate := weeklyCLIInputFixture(t)
	d := maintenance.WeeklyModelDescriptor{RuntimeVersion: "synthetic-version", Model: "synthetic-model"}
	init := `{"type":"system","subtype":"init","model":"synthetic-model","claude_code_version":"synthetic-version","tools":[],"mcp_servers":[],"plugins":[],"skills":[],"agents":[]}` + "\n"
	text, _ := json.Marshal(string(candidate))
	result := `{"type":"result","subtype":"success","is_error":false,"num_turns":1,"result":` + string(text) + `}`
	if _, err := parseWeeklyCLIEnvelope([]byte(init+`{"type":"assistant","message":{"role":"assistant","model":"synthetic-model","content":[{"type":"text","text":`+string(text)+`}]}}`+"\n"+result), d, input); err != nil {
		t.Fatal(err)
	}
	cases := []string{result, init, init + result + "\n" + result, init + `{"type":"system","subtype":"hook_started"}` + "\n" + result, init + `{"type":"assistant","message":{"role":"assistant","model":"synthetic-model","content":[{"type":"tool_use","name":"Agent"}]}}` + "\n" + result, strings.Replace(init+result, `"tools":[]`, `"tools":["Bash"]`, 1), strings.Replace(init+result, `"num_turns":1`, `"num_turns":2`, 1), strings.Replace(init+result, "Bounded synthetic working text.", strings.Repeat("x", 2001), 1), strings.Replace(init+result, input.InputSHA256, strings.Repeat("f", 64), 1), strings.Replace(init+result, `"type":"result"`, `"type":"result","type":"result"`, 1)}
	for i, body := range cases {
		if _, err := parseWeeklyCLIEnvelope([]byte(body), d, input); err == nil {
			t.Fatal("unsafe vendor envelope accepted", i)
		}
	}
	args, err := weeklyCLIArguments(d, input)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, "\n")
	for _, flag := range []string{"--safe-mode", "--restricted", "--tools\n\n--strict-mcp-config", "--permission-prompts\nnone", "--max-turns\n1", "--no-session-persistence"} {
		if !strings.Contains(joined, flag) {
			t.Fatal("missing fixed tool-free control", flag)
		}
	}
	if strings.Contains(joined, "Use concise synthetic text.") {
		t.Fatal("private owner input leaked to process argv")
	}
}

func TestWeeklyCLIMissingNativeSessionBlocksBeforeInvocation(t *testing.T) {
	input, _ := weeklyCLIInputFixture(t)
	// This incomplete native constructor intentionally has no valid grant or
	// executable. The specific readiness error proves missing supported declared-topology
	// cancellation is denied BEFORE authority, executable access or child start.
	adapter := &WeeklyCLIAdapter{options: WeeklyCLIOptions{Descriptor: maintenance.WeeklyModelDescriptor{Executable: "/must-never-be-opened"}}}
	_, err := adapter.Compact(context.Background(), input)
	if !errors.Is(err, maintenance.ErrQualificationRequired) {
		t.Fatalf("unsupported supervisor did not deny first: %v", err)
	}
}

func TestWeeklyCLIPlainJSONContractAvoidsStructuredOutputTool(t *testing.T) {
	input, candidate := weeklyCLIInputFixture(t)
	descriptor := maintenance.WeeklyModelDescriptor{RuntimeVersion: "synthetic-version", Model: "synthetic-model"}
	args, err := weeklyCLIArguments(descriptor, input)
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range args {
		if arg == "--json-schema" {
			t.Fatal("provider structured output introduces a tool and second turn")
		}
	}
	text, _ := json.Marshal(string(candidate))
	init := `{"type":"system","subtype":"init","session_id":"synthetic-session","model":"synthetic-model","claude_code_version":"synthetic-version","tools":[],"mcp_servers":[],"plugins":[],"skills":[],"agents":[]}`
	assistant := `{"type":"assistant","message":{"role":"assistant","model":"synthetic-model","content":[{"type":"text","text":` + string(text) + `}]}}`
	rate := `{"type":"rate_limit_event","uuid":"synthetic-event","session_id":"synthetic-session","rate_limit_info":{"status":"allowed","resetsAt":2000000000,"rateLimitType":"overage","overageStatus":"allowed","overageResetsAt":2000000000,"isUsingOverage":false,"overageInUse":true}}`
	result := `{"type":"result","subtype":"success","is_error":false,"num_turns":1,"result":` + string(text) + `}`
	envelope := strings.Join([]string{init, assistant, rate, result}, "\n")
	if _, err := parseWeeklyCLIEnvelope([]byte(envelope), descriptor, input); err != nil {
		t.Fatal("one-turn plain JSON envelope denied", err)
	}
	builtinNames := []string{"cc-plugin-sec-default", "cc-plugin-agents-md", "cc-plugin-telemetry", "cc-plugin-privacy", "cc-plugin-plugin-authoring"}
	builtinPlugins := []map[string]string{}
	for _, name := range builtinNames {
		builtinPlugins = append(builtinPlugins, map[string]string{"name": name, "path": "builtin", "source": name + "@builtin"})
	}
	builtinJSON, _ := json.Marshal(builtinPlugins)
	actualInventory := strings.Replace(envelope, `"plugins":[]`, `"plugins":`+string(builtinJSON), 1)
	actualInventory = strings.Replace(actualInventory, `"agents":[]`, `"agents":["claude","Explore","general-purpose","Plan"]`, 1)
	if _, err := parseWeeklyCLIEnvelope([]byte(actualInventory), descriptor, input); err != nil {
		t.Fatal("observed informational builtin inventory denied", err)
	}
	if _, err := parseWeeklyCLIEnvelope([]byte(strings.Replace(actualInventory, `"tools":[]`, `"tools":["Agent"]`, 1)), descriptor, input); err == nil {
		t.Fatal("builtin inventory added delegation authority")
	}

	for _, bad := range []string{strings.Replace(envelope, `"status":"allowed"`, `"status":"unknown"`, 1), strings.Replace(envelope, `"overageInUse":true`, `"overageInUse":true,"command":"synthetic"`, 1), strings.Replace(envelope, `"rate_limit_event"`, `"execution_event"`, 1), strings.Replace(envelope, `"resetsAt":2000000000`, `"resetsAt":"not-a-time"`, 1), strings.Replace(envelope, `"uuid":"synthetic-event","session_id":"synthetic-session"`, `"uuid":"synthetic-event","session_id":"another-session"`, 1), strings.Replace(envelope, `"num_turns":1`, `"num_turns":2`, 1)} {
		if _, err := parseWeeklyCLIEnvelope([]byte(bad), descriptor, input); err == nil {
			t.Fatal("unsupported metadata/execution envelope accepted")
		}
	}
}

func TestWeeklyCLIBuiltinInventoryIsNotToolAuthority(t *testing.T) {
	names := []string{"cc-plugin-sec-default", "cc-plugin-agents-md", "cc-plugin-telemetry", "cc-plugin-privacy", "cc-plugin-plugin-authoring"}
	plugins := []map[string]string{}
	for _, name := range names {
		plugins = append(plugins, map[string]string{"name": name, "path": "builtin", "source": name + "@builtin"})
	}
	p, _ := json.Marshal(plugins)
	a := []byte(`["claude","Explore","general-purpose","Plan"]`)
	if validateWeeklyBuiltinInventory(p, a) != nil {
		t.Fatal("exact builtin inventory denied")
	}
	for _, bad := range [][]byte{[]byte(`null`), []byte(`[]`), []byte(`["claude","Explore","general-purpose","Agent"]`), []byte(`["claude","Explore","Explore","Plan"]`)} {
		if validateWeeklyBuiltinInventory(p, bad) == nil {
			t.Fatal("partial/custom/duplicate inventory accepted")
		}
	}
	for _, bad := range [][]byte{[]byte(strings.Replace(string(p), `"path":"builtin"`, `"path":"/custom"`, 1)), []byte(strings.Replace(string(p), `"path":"builtin"`, `"path":"builtin","command":"synthetic"`, 1)), []byte(strings.Replace(string(p), `cc-plugin-privacy`, `cc-plugin-other`, 1))} {
		if validateWeeklyBuiltinInventory(bad, a) == nil {
			t.Fatal("unknown/custom builtin record accepted")
		}
	}
}
