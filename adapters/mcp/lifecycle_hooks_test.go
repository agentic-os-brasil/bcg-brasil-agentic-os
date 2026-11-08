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

func TestPassiveLifecycleResumesCanonicalOnboarding(t *testing.T) {
	for _, track := range []string{"quick", "complete"} {
		t.Run(track, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			approved := true
			cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				if !approved {
					return &mcp.ElicitResult{Action: "decline"}, nil
				}
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			})
			ownerCall(t, cs, "owner.track", map[string]any{"track": track}, false)
			interview := ownerCall(t, cs, "owner.interview", map[string]any{"track": track}, false)
			steps := interview["steps"].([]any)
			assertHook := func(state, nextFacet, digest string) {
				t.Helper()
				before := treeSnapshot(t, filepath.Dir(f.options.ProductRoot))
				for _, event := range []string{"SessionStart", "UserPromptSubmit"} {
					out, err := passiveHook(t, f, event, map[string]any{"prompt": "PRIVATE_PROMPT_SENTINEL"})
					if err != nil {
						t.Fatal(err)
					}
					var envelope struct {
						Hook struct {
							Context string `json:"additionalContext"`
						} `json:"hookSpecificOutput"`
					}
					if err = json.Unmarshal([]byte(out), &envelope); err != nil {
						t.Fatal(err)
					}
					text := envelope.Hook.Context
					// JSON field names form the public hook contract, independent of the Go implementation.
					var data map[string]json.RawMessage
					start := strings.Index(text, "\n{")
					if start < 0 || json.Unmarshal([]byte(text[start+1:]), &data) != nil || data["onboarding"] == nil {
						t.Fatalf("hook omitted canonical onboarding metadata: %s", text)
					}
					var onboarding struct {
						State        string   `json:"state"`
						Track        string   `json:"track"`
						NextFacet    string   `json:"next_facet"`
						ReviewDigest string   `json:"review_digest"`
						Remaining    []string `json:"remaining"`
						NextQuestion string   `json:"next_question"`
					}
					if err = json.Unmarshal(data["onboarding"], &onboarding); err != nil || onboarding.State != state || onboarding.Track != track || onboarding.NextFacet != nextFacet || onboarding.ReviewDigest != digest {
						t.Fatalf("hook lost current onboarding: %s %v", text, err)
					}
					expectedQuestion := ""
					for _, raw := range steps {
						step := raw.(map[string]any)
						if step["facet"] == nextFacet {
							expectedQuestion = step["question"].(string)
						}
					}
					if nextFacet != "" && (len(onboarding.Remaining) == 0 || onboarding.Remaining[0] != nextFacet || onboarding.NextQuestion != expectedQuestion) {
						t.Fatalf("hook did not retain exact next pending step: %s", text)
					}
					for _, private := range []string{"private-owner-marker", "PRIVATE_PROMPT_SENTINEL", f.root, "owner/self/", "bcgos owner"} {
						if strings.Contains(out, private) {
							t.Fatalf("hook exposed private text/path or removed command: %s", private)
						}
					}
				}
				if !reflect.DeepEqual(before, treeSnapshot(t, filepath.Dir(f.options.ProductRoot))) {
					t.Fatal("onboarding context wrote state")
				}
			}
			for i, raw := range steps {
				step := raw.(map[string]any)
				facet := step["facet"].(string)
				p := ownerCall(t, cs, "owner.propose", map[string]any{"facet": facet, "evidence": "explicit synthetic interview", "proposed_body": "# Profile\n\n## Current\n\nprivate-owner-marker " + facet}, false)
				ownerCall(t, cs, "owner.apply", map[string]any{"proposal_id": p["id"]}, false)
				if i == 0 {
					assertHook("in_progress", steps[1].(map[string]any)["facet"].(string), "")
					// A new SDK connection and hook binding must resume persisted progress.
					cs = f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
						if !approved {
							return &mcp.ElicitResult{Action: "decline"}, nil
						}
						return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
					})
					status := ownerCall(t, cs, "owner.status", nil, false)["onboarding"].(map[string]any)
					if status["state"] != "in_progress" {
						t.Fatalf("restart reset onboarding: %v", status)
					}
					assertHook("in_progress", steps[1].(map[string]any)["facet"].(string), "")
				}
			}
			review := ownerCall(t, cs, "owner.onboarding_review", nil, false)
			digest := review["review_digest"].(string)
			if len(digest) != 64 {
				t.Fatal("final review digest absent")
			}
			assertHook("review_required", "", digest)
			approved = false
			before := treeSnapshot(t, filepath.Dir(f.options.ProductRoot))
			ownerCall(t, cs, "owner.confirm_onboarding", nil, true)
			if !reflect.DeepEqual(before, treeSnapshot(t, filepath.Dir(f.options.ProductRoot))) {
				t.Fatal("declined final confirmation changed state")
			}
			assertHook("review_required", "", digest)
			approved = true
			ownerCall(t, cs, "owner.confirm_onboarding", nil, false)
			assertHook("complete", "", "")
		})
	}
}

func TestPassiveLifecycleUnavailableOwnerDoesNotRestartOrRepair(t *testing.T) {
	for _, invalid := range []string{"missing", "corrupt", "alias"} {
		t.Run(invalid, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			registry := filepath.Join(f.root, "owner/registry.json")
			if err := os.Remove(registry); err != nil {
				t.Fatal(err)
			}
			switch invalid {
			case "corrupt":
				if err := os.WriteFile(registry, []byte("PRIVATE_CORRUPT_PROFILE"), 0600); err != nil {
					t.Fatal(err)
				}
			case "alias":
				if err := os.Symlink(filepath.Join(f.options.WorkspacePath, "private-profile"), registry); err != nil {
					t.Fatal(err)
				}
			}
			before := treeSnapshot(t, filepath.Dir(f.options.ProductRoot))
			for _, event := range []string{"SessionStart", "UserPromptSubmit"} {
				out, err := passiveHook(t, f, event, nil)
				if invalid == "alias" {
					if err == nil || out != "" {
						t.Fatalf("unsafe owner alias was admitted: %v %s", err, out)
					}
					continue
				}
				var envelope struct {
					Hook struct {
						Context string `json:"additionalContext"`
					} `json:"hookSpecificOutput"`
				}
				decodeErr := json.Unmarshal([]byte(out), &envelope)
				if err != nil || decodeErr != nil || !strings.Contains(envelope.Hook.Context, `"onboarding":{"state":"unavailable","track":"unavailable"}`) || strings.Contains(out, "PRIVATE_CORRUPT_PROFILE") {
					t.Fatalf("invalid owner became a fresh interview: %v %s", err, out)
				}
			}
			if !reflect.DeepEqual(before, treeSnapshot(t, filepath.Dir(f.options.ProductRoot))) {
				t.Fatal("hook repaired or initialized unavailable owner")
			}
		})
	}
}

func TestPassiveLifecycleStaleBindingNeverEmitsContext(t *testing.T) {
	f := newOwnerFixture(t, false)
	bound, err := product.Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(f.options.WorkspacePath, f.options.WorkspacePath+"-moved"); err != nil {
		t.Fatal(err)
	}
	before := treeSnapshot(t, filepath.Dir(f.options.ProductRoot))
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "Stop"} {
		input, _ := json.Marshal(map[string]any{"session_id": "stale-session", "prompt_id": "stale-turn", "cwd": f.options.WorkspacePath, "hook_event_name": event, "stop_hook_active": false})
		var output bytes.Buffer
		if err = bound.HandleClaudeLifecycleHook(event, bytes.NewReader(input), &output); err == nil || output.Len() != 0 {
			t.Fatalf("stale %s emitted context or observation: %v %s", event, err, output.String())
		}
	}
	if !reflect.DeepEqual(before, treeSnapshot(t, filepath.Dir(f.options.ProductRoot))) {
		t.Fatal("stale binding changed state")
	}
}
