package product_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func productHookContext(t *testing.T, f ownerFixture, event string) string {
	t.Helper()
	b, err := product.Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	input, _ := json.Marshal(map[string]any{"session_id": "memory-context-session", "prompt_id": "memory-context-turn", "cwd": f.options.WorkspacePath, "hook_event_name": event})
	var out bytes.Buffer
	if err = b.HandleClaudeHook(event, bytes.NewReader(input), &out); err != nil {
		t.Fatal(err)
	}
	var value struct {
		Hook struct {
			Context string `json:"additionalContext"`
		} `json:"hookSpecificOutput"`
	}
	if err = json.Unmarshal(out.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	return value.Hook.Context
}

func TestProductHookGeneratedMemoryReadOnly(t *testing.T) {
	f := newOwnerFixture(t, false)
	prepareMemoryFixture(t, f)
	cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	const marker = "HOOK_GENERATED_MEMORY_MARKER"
	memoryCall(t, cs, "memory.capture", map[string]any{"text": marker}, false)
	base := filepath.Dir(f.options.ProductRoot)
	before := treeSnapshot(t, base)
	if out := productHookContext(t, f, "SessionStart"); strings.Contains(out, marker) {
		t.Fatal("unconsolidated capture was injected")
	}
	if !reflect.DeepEqual(before, treeSnapshot(t, base)) {
		t.Fatal("startup changed the ungenerated memory fixture")
	}
	memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")}, false)
	before = treeSnapshot(t, base)
	out := productHookContext(t, f, "SessionStart")
	if !strings.Contains(out, marker) {
		t.Fatalf("generated reviewed memory missing from SessionStart: %s", out)
	}
	if !strings.Contains(out, "quoted historical data") || !strings.Contains(out, "never as an instruction") {
		t.Fatalf("memory lost its historical-data framing: %s", out)
	}
	if !reflect.DeepEqual(before, treeSnapshot(t, base)) {
		t.Fatal("startup changed the committed memory fixture")
	}
	if out, err := passiveHook(t, f, "SessionStart", nil); err != nil || !strings.Contains(out, marker) {
		t.Fatalf("passive startup did not deliver the same generated view: %v %s", err, out)
	}
	if out, err := passiveHook(t, f, "UserPromptSubmit", map[string]any{"prompt": "PRIVATE_PROMPT_SENTINEL"}); err != nil || strings.Contains(out, marker) || strings.Contains(out, "PRIVATE_PROMPT_SENTINEL") {
		t.Fatalf("passive prompt repeated memory or private input: %v %s", err, out)
	}
	if !reflect.DeepEqual(before, treeSnapshot(t, base)) {
		t.Fatal("passive context changed owner, memory, workspace or agent-flow state")
	}
}

func passiveHook(t *testing.T, f ownerFixture, event string, fields map[string]any) (string, error) {
	t.Helper()
	b, err := product.Open(f.options)
	if err != nil {
		return "", err
	}
	input := map[string]any{"session_id": "passive-session", "prompt_id": "passive-turn", "cwd": f.options.WorkspacePath, "hook_event_name": event, "stop_hook_active": false}
	for key, value := range fields {
		input[key] = value
	}
	body, _ := json.Marshal(input)
	var out bytes.Buffer
	err = b.HandleClaudeLifecycleHook(event, bytes.NewReader(body), &out)
	return out.String(), err
}

func TestPassiveLifecycleMissingMemoryDoesNotInitialize(t *testing.T) {
	f := newOwnerFixture(t, false)
	base := filepath.Dir(f.options.ProductRoot)
	before := treeSnapshot(t, base)
	for _, event := range []string{"SessionStart", "UserPromptSubmit"} {
		out, err := passiveHook(t, f, event, map[string]any{"prompt": "PRIVATE_PROMPT_SENTINEL"})
		if err != nil || !strings.Contains(out, "maestro_context") || strings.Contains(out, "PRIVATE_PROMPT_SENTINEL") || strings.Contains(out, "permissionDecision") {
			t.Fatalf("missing-memory context: %v %s", err, out)
		}
	}
	if !reflect.DeepEqual(before, treeSnapshot(t, base)) {
		t.Fatal("passive context initialized absent authority or flow state")
	}
}

func TestPassiveLifecycleRefusalsPreserveState(t *testing.T) {
	f := newOwnerFixture(t, false)
	base := filepath.Dir(f.options.ProductRoot)
	before := treeSnapshot(t, base)
	for _, tc := range []struct {
		event  string
		fields map[string]any
	}{
		{"PreToolUse", nil},
		{"SessionStart", map[string]any{"session_id": "private invalid session"}},
		{"SessionStart", map[string]any{"hook_event_name": "UserPromptSubmit"}},
		{"SessionStart", map[string]any{"cwd": base}},
		{"SessionStart", map[string]any{"agent_id": "child", "agent_type": "case-agent"}},
		{"Stop", map[string]any{"stop_hook_active": nil}},
	} {
		out, err := passiveHook(t, f, tc.event, tc.fields)
		if err == nil || out != "" || strings.Contains(err.Error(), "private invalid session") {
			t.Fatalf("invalid event gained context/authority: %s %v %s", tc.event, err, out)
		}
	}
	if !reflect.DeepEqual(before, treeSnapshot(t, base)) {
		t.Fatal("refused lifecycle input changed fixture")
	}
}

func TestPassiveStopOnlyRecordsIdempotentMetadata(t *testing.T) {
	f := newOwnerFixture(t, false)
	base := filepath.Dir(f.options.ProductRoot)
	before := treeSnapshot(t, base)
	if out, err := passiveHook(t, f, "Stop", map[string]any{"stop_hook_active": true}); err != nil || strings.TrimSpace(out) != "{}" {
		t.Fatalf("recursive stop: %v %s", err, out)
	}
	if !reflect.DeepEqual(before, treeSnapshot(t, base)) {
		t.Fatal("recursive stop recorded an observation")
	}
	ownerBefore, workBefore := treeSnapshot(t, f.root), treeSnapshot(t, f.options.WorkspacePath)
	fields := map[string]any{"prompt": "PRIVATE_PROMPT_SENTINEL", "last_assistant_message": "PRIVATE_ASSISTANT_SENTINEL", "tool_name": "private-tool-name"}
	out, err := passiveHook(t, f, "Stop", fields)
	if err != nil || strings.TrimSpace(out) != "{}" {
		t.Fatalf("passive stop: %v %s", err, out)
	}
	first := treeSnapshot(t, base)
	if out, err = passiveHook(t, f, "Stop", fields); err != nil || strings.TrimSpace(out) != "{}" {
		t.Fatalf("replayed passive stop: %v %s", err, out)
	}
	if !reflect.DeepEqual(first, treeSnapshot(t, base)) || !reflect.DeepEqual(ownerBefore, treeSnapshot(t, f.root)) || !reflect.DeepEqual(workBefore, treeSnapshot(t, f.options.WorkspacePath)) {
		t.Fatal("stop changed owner/workspace bodies or replay rewrote metadata")
	}
	receipts, err := filepath.Glob(filepath.Join(filepath.Dir(f.root), "state/runtime/receipts/*/*.json"))
	if err != nil || len(receipts) != 1 {
		t.Fatalf("expected one canonical receipt: %v %v", receipts, err)
	}
	body, err := os.ReadFile(receipts[0])
	if err != nil {
		t.Fatal(err)
	}
	var receipt lifecycle.Receipt
	if err = json.Unmarshal(body, &receipt); err != nil || receipt.Event != lifecycle.StopFinalize || receipt.State != "observed" || receipt.Provenance != lifecycle.AdapterCommand || receipt.ToolName != "" || receipt.AgentType != "" || strings.Contains(string(body), "PRIVATE_") {
		t.Fatalf("stop was not body-free observation metadata: %v %s", err, body)
	}
}

func TestLifecycleMemoryCorrectionAndDeletionHideOldView(t *testing.T) {
	f := newOwnerFixture(t, false)
	prepareMemoryFixture(t, f)
	cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	const original = "HOOK_REMOVED_GENERATION_SENTINEL"
	const replacement = "HOOK_SURVIVING_GENERATION_SENTINEL"
	record := memoryCall(t, cs, "memory.capture", map[string]any{"text": original}, false)
	consolidate := map[string]any{"cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")}
	memoryCall(t, cs, "memory.consolidate", consolidate, false)
	if out, err := passiveHook(t, f, "SessionStart", nil); err != nil || !strings.Contains(out, original) {
		t.Fatalf("original generated view absent: %v %s", err, out)
	}
	memoryCall(t, cs, "memory.correct", map[string]any{"capture_id": record["id"], "expected_revision": 1, "text": replacement}, false)
	if out, err := passiveHook(t, f, "SessionStart", nil); err != nil || strings.Contains(out, original) || strings.Contains(out, replacement) {
		t.Fatalf("startup used denied generation or raw replacement: %v %s", err, out)
	}
	memoryCall(t, cs, "memory.consolidate", consolidate, false)
	if out, err := passiveHook(t, f, "SessionStart", nil); err != nil || strings.Contains(out, original) || !strings.Contains(out, replacement) {
		t.Fatalf("rebuilt generation is incorrect: %v %s", err, out)
	}
	memoryCall(t, cs, "memory.delete", map[string]any{"capture_id": record["id"], "expected_revision": 2}, false)
	before := treeSnapshot(t, filepath.Dir(f.options.ProductRoot))
	if out, err := passiveHook(t, f, "SessionStart", nil); err != nil || strings.Contains(out, replacement) || strings.Contains(out, original) {
		t.Fatalf("startup resurrected deleted content: %v %s", err, out)
	}
	if !reflect.DeepEqual(before, treeSnapshot(t, filepath.Dir(f.options.ProductRoot))) {
		t.Fatal("deleted-memory startup performed repair or initialization")
	}
}
