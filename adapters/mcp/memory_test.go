package product_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func memoryCall(t *testing.T, cs *mcp.ClientSession, op string, extra map[string]any, fail bool) map[string]any {
	t.Helper()
	args := map[string]any{"operation": op}
	for k, v := range extra {
		args[k] = v
	}
	tool := "maestro_query"
	if strings.Contains("|memory.capture|memory.correct|memory.delete|memory.retention|memory.consolidate|", "|"+op+"|") {
		tool = "maestro_apply"
	}
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if out.IsError != fail || len(out.Content) == 0 {
		t.Fatalf("%s result: %+v", op, out)
	}
	raw := out.Content[0].(*mcp.TextContent).Text
	if strings.Contains(raw, "owned-captures/") || strings.Contains(raw, ".capture") || strings.Contains(raw, "attestation") {
		t.Fatalf("private path/proof leaked: %s", raw)
	}
	var value map[string]any
	if err = json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func prepareMemoryFixture(t *testing.T, f ownerFixture) {
	t.Helper()
	// The synthetic SDK fixture performs explicit canonical setup; stdio uses init.
	if f.binary == "" {
		ctx, err := f.connect(t, nil).CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_context", Arguments: map[string]any{}})
		if err != nil {
			t.Fatal(err)
		}
		var metadata map[string]any
		if err = json.Unmarshal([]byte(ctx.Content[0].(*mcp.TextContent).Text), &metadata); err != nil {
			t.Fatal(err)
		}
		policy, err := basememory.Policy()
		if err != nil {
			t.Fatal(err)
		}
		e := memory.Engine{Root: filepath.Join(f.root, "memory"), Policy: policy}
		if err = e.PrepareOwnedCaptureAuthority(metadata["workspace_id"].(string)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMemoryReviewedCaptureRestart(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			prepareMemoryFixture(t, f)
			text := "memória sintética completa com ação e revisão"
			var messages []string
			approve := func(_ context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				messages = append(messages, r.Params.Message)
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			cs := f.connect(t, approve)
			got := memoryCall(t, cs, "memory.capture", map[string]any{"text": text}, false)
			if got["eligible"] != true || got["revision"] != float64(1) || len(messages) != 1 || !strings.Contains(messages[0], text) {
				t.Fatalf("capture/review: %v %v", got, messages)
			}
			for _, proof := range []string{"source_digest", "visibility_revision", "review_digest"} {
				if strings.Contains(messages[0], proof) {
					t.Fatalf("proof shown to human: %s", proof)
				}
			}
			restarted := f.connect(t, approve)
			page := memoryCall(t, restarted, "memory.list", map[string]any{"limit": 1}, false)
			rows := page["captures"].([]any)
			if len(rows) != 1 || rows[0].(map[string]any)["id"] != got["id"] {
				t.Fatalf("discovery: %v", page)
			}
			read := memoryCall(t, restarted, "memory.read", map[string]any{"capture_id": got["id"]}, false)
			if read["text"] != text {
				t.Fatalf("read: %v", read)
			}
			search := memoryCall(t, restarted, "memory.search", map[string]any{"query": "ação"}, false)
			if search["total"] != float64(1) {
				t.Fatalf("search: %v", search)
			}
			memoryCall(t, restarted, "memory.consolidate", map[string]any{"cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")}, false)
			context := memoryCall(t, restarted, "memory.context", nil, false)
			body, _ := json.Marshal(context)
			if !strings.Contains(string(body), text) {
				t.Fatalf("context omitted capture: %s", body)
			}
		})
	}
}

func TestMemoryLifecycle(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			prepareMemoryFixture(t, f)
			approve := func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			cs := f.connect(t, approve)
			old := memoryCall(t, cs, "memory.capture", map[string]any{"text": "unique-removed-memory-marker"}, false)
			keep := memoryCall(t, cs, "memory.capture", map[string]any{"text": "surviving-memory-marker"}, false)
			date := time.Now().UTC().Format("2006-01-02")
			memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": date}, false)
			newer := memoryCall(t, cs, "memory.correct", map[string]any{"capture_id": old["id"], "expected_revision": 1, "text": "replacement-memory-marker"}, false)
			if newer["id"] != old["id"] || newer["revision"] != float64(2) || newer["eligible"] != true {
				t.Fatalf("correction receipt: %v", newer)
			}
			memoryCall(t, cs, "memory.delete", map[string]any{"capture_id": old["id"], "expected_revision": 1}, true)
			search := memoryCall(t, cs, "memory.search", map[string]any{"query": "unique-removed-memory-marker"}, false)
			if search["total"] != float64(0) {
				t.Fatal("search recovered corrected text")
			}

			memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": date}, false)
			view := memoryCall(t, f.connect(t, approve), "memory.context", nil, false)
			if strings.Contains(view["text"].(string), "unique-removed-memory-marker") || !strings.Contains(view["text"].(string), "replacement-memory-marker") {
				t.Fatalf("rebuild: %v", view)
			}
			memoryCall(t, cs, "memory.delete", map[string]any{"capture_id": old["id"], "expected_revision": 2}, false)
			memoryCall(t, cs, "memory.read", map[string]any{"capture_id": old["id"]}, true)
			surviving := memoryCall(t, cs, "memory.read", map[string]any{"capture_id": keep["id"]}, false)
			last := memoryCall(t, cs, "memory.capture", map[string]any{"text": "last-live-memory-marker"}, false)
			at, err := time.Parse(time.RFC3339Nano, surviving["recorded_at"].(string))
			if err != nil {
				t.Fatal(err)
			}
			page := memoryCall(t, cs, "memory.list", nil, false)
			retention := map[string]any{"cutoff": at.Add(time.Nanosecond).Format(time.RFC3339Nano), "expected_visibility_revision": page["visibility_revision"]}
			memoryCall(t, cs, "memory.retention", retention, false)
			memoryCall(t, cs, "memory.read", map[string]any{"capture_id": keep["id"]}, true)
			search = memoryCall(t, cs, "memory.search", map[string]any{"query": "surviving-memory-marker"}, false)
			if search["total"] != float64(0) {
				t.Fatal("search recovered retained-away text")
			}

			memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": date}, false)
			view = memoryCall(t, cs, "memory.context", nil, false)
			if strings.Contains(view["text"].(string), "surviving-memory-marker") || strings.Contains(view["text"].(string), "replacement-memory-marker") {
				t.Fatalf("revoked text returned: %v", view)
			}
			memoryCall(t, cs, "memory.delete", map[string]any{"capture_id": last["id"], "expected_revision": 1}, false)
			memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": date}, false)
			view = memoryCall(t, f.connect(t, nil), "memory.context", nil, false)
			if view["text"] != "" {
				t.Fatalf("final empty context: %v", view)
			}
		})
	}
}

func memoryCLI(t *testing.T, f ownerFixture, command string, request any, extra []string, fail bool) map[string]any {
	t.Helper()
	body, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	args := append([]string{command, "--product-root", f.options.ProductRoot, "--user-home", f.options.UserHome, "--workspace", f.options.WorkspacePath}, extra...)
	cmd := exec.Command(f.binary, args...)
	cmd.Stdin = strings.NewReader(string(body))
	out, err := cmd.CombinedOutput()
	if (err != nil) != fail {
		t.Fatalf("%s: %s %v", command, out, err)
	}
	if fail {
		return nil
	}
	var value map[string]any
	if err = json.Unmarshal(out, &value); err != nil {
		t.Fatalf("%s: %s %v", command, out, err)
	}
	return value
}
func TestMemoryLocalFallback(t *testing.T) {
	f := newOwnerFixture(t, true)
	request := map[string]any{"operation": "memory.capture", "text": "operator-only synthetic text"}
	review := memoryCLI(t, f, "memory-review", request, nil, false)
	raw, _ := json.Marshal(review)
	if !strings.Contains(string(raw), "operator-only synthetic text") {
		t.Fatal("local review omitted exact private text")
	}
	args := []string{"--review-digest", review["review_digest"].(string)}
	memoryCLI(t, f, "memory-apply", request, args, true)
	wrong := map[string]any{"operation": "memory.capture", "text": "changed operator text"}
	memoryCLI(t, f, "memory-apply", wrong, append(args, "--confirm"), true)
	captured := memoryCLI(t, f, "memory-apply", request, append(args, "--confirm"), false)
	for _, operation := range []string{"memory.correct", "memory.delete", "memory.retention"} {
		request = map[string]any{"operation": operation}
		if operation == "memory.retention" {
			victimRequest := map[string]any{"operation": "memory.capture", "text": "local-retention-victim"}
			victimReview := memoryCLI(t, f, "memory-review", victimRequest, nil, false)
			memoryCLI(t, f, "memory-apply", victimRequest, []string{"--review-digest", victimReview["review_digest"].(string), "--confirm"}, false)
			memoryCLI(t, f, "memory-apply", map[string]any{"operation": "memory.capture", "text": "never reviewed"}, []string{"--review-digest", strings.Repeat("0", 64), "--confirm"}, true)
			page := memoryCall(t, f.connect(t, nil), "memory.list", nil, false)
			request["expected_visibility_revision"] = page["visibility_revision"]
			request["cutoff"] = time.Now().UTC().Add(time.Minute).Format(time.RFC3339Nano)
		} else {
			request["capture_id"] = captured["id"]
			request["expected_revision"] = captured["revision"]
		}
		if operation == "memory.correct" {
			request["text"] = "operator replacement"
		}
		review = memoryCLI(t, f, "memory-review", request, nil, false)
		captured = memoryCLI(t, f, "memory-apply", request, []string{"--review-digest", review["review_digest"].(string), "--confirm"}, false)
		if operation == "memory.retention" {
			if captured["removed"] != float64(1) {
				t.Fatal("local retention did not remove the reviewed victim")
			}
			page := memoryCall(t, f.connect(t, nil), "memory.list", nil, false)
			if page["total"] != float64(0) {
				t.Fatal("local retention did not survive restart")
			}
		}

	}
}

func fixtureMemoryEngine(t *testing.T, f ownerFixture) (*memory.Engine, string) {
	t.Helper()
	policy, err := basememory.Policy()
	if err != nil {
		t.Fatal(err)
	}
	out, err := f.connect(t, nil).CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_context", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &value); err != nil {
		t.Fatal(err)
	}
	return &memory.Engine{Root: filepath.Join(f.root, "memory"), Policy: policy}, value["workspace_id"].(string)
}
func seedReviewedMemory(t *testing.T, e *memory.Engine, workspace, text, date string) memory.CaptureReceipt {
	t.Helper()
	at, err := time.Parse("2006-01-02", date)
	if err != nil {
		t.Fatal(err)
	}
	input, err := e.SealOwnedCapture(workspace, "", 0, text, at.Add(12*time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := e.CaptureOwned(workspace, input)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}
func TestMemoryWeeklyTrustedPromotion(t *testing.T) {
	for _, tc := range []struct {
		name                               string
		stdio, decline, unavailable, stale bool
	}{
		{name: "SDK"}, {name: "stdio", stdio: true}, {name: "decline", decline: true}, {name: "unsupported", unavailable: true}, {name: "changed-sources", stale: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newOwnerFixture(t, tc.stdio)
			prepareMemoryFixture(t, f)
			e, workspace := fixtureMemoryEngine(t, f)
			firstCapture := seedReviewedMemory(t, e, workspace, "first-week-synthetic-continuity", "2026-10-12")
			seedReviewedMemory(t, e, workspace, "second-week-synthetic-continuity", "2026-10-19")
			calls := 0
			var presentation string
			var approve func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)
			if !tc.unavailable {
				approve = func(_ context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					calls++
					presentation = r.Params.Message
					if tc.stale {
						seedReviewedMemory(t, e, workspace, "changed-after-exact-review", "2026-10-19")
					}
					action := "accept"
					if tc.decline {
						action = "decline"
					}
					return &mcp.ElicitResult{Action: action, Content: map[string]any{"approved": true}}, nil
				}
			}
			cs := f.connect(t, approve)
			memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": "2026-10-12"}, false)
			first := memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "weekly", "date": "2026-10-12"}, false)
			raw, _ := json.Marshal(first)
			if calls != 0 || !strings.Contains(string(raw), "observed 1") {
				t.Fatalf("first generation gate: %s calls=%d", raw, calls)
			}
			memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": "2026-10-19"}, false)
			second := memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "weekly", "date": "2026-10-19"}, tc.stale)
			if !tc.unavailable && calls != 1 {
				t.Fatalf("MRTR review was not stable/exact: calls=%d result=%v", calls, second)
			}
			if !tc.unavailable && (!strings.Contains(presentation, "first-week-synthetic-continuity") || !strings.Contains(presentation, "second-week-synthetic-continuity")) {
				t.Fatalf("candidate body omitted: %s", presentation)
			}
			for _, field := range []string{"source_digest", "source_fingerprint", "visibility_revision", "review_digest", "GeneratedAt"} {
				if strings.Contains(presentation, field) {
					t.Fatalf("opaque proof leaked to human: %s", field)
				}
			}
			view := memoryCall(t, f.connect(t, nil), "memory.context", nil, false)
			raw, _ = json.Marshal(view["layers"])
			want := !tc.decline && !tc.unavailable && !tc.stale
			if strings.Contains(string(raw), "lifetime") != want {
				t.Fatalf("lifetime active=%v want=%v: %v", strings.Contains(string(raw), "lifetime"), want, view)
			}
			if !tc.stale && (!strings.Contains(string(raw), "L2") || !strings.Contains(string(raw), "L3")) {
				t.Fatalf("working weekly layers missing: %v", view)
			}
			if want {
				memoryCall(t, cs, "memory.delete", map[string]any{"capture_id": firstCapture.ID, "expected_revision": 1}, false)
				memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": "2026-10-19"}, false)
				memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "weekly", "date": "2026-10-19"}, false)
				rebuilt := memoryCall(t, f.connect(t, nil), "memory.context", nil, false)
				if strings.Contains(rebuilt["text"].(string), "first-week-synthetic-continuity") {
					t.Fatal("revoked source returned through L3/lifetime carry-forward")
				}
			}

		})
	}
}

func TestMemoryConfirmationRefusals(t *testing.T) {
	for _, protocol := range []string{"", "2025-11-25"} {
		for _, tc := range []struct {
			name, action                string
			content                     map[string]any
			unsupported, transportError bool
		}{
			{name: "unsupported", unsupported: true}, {name: "decline", action: "decline"}, {name: "cancel", action: "cancel"}, {name: "nil", action: "accept"},
			{name: "false", action: "accept", content: map[string]any{"approved": false}}, {name: "missing", action: "accept", content: map[string]any{}},
			{name: "wrong-type", action: "accept", content: map[string]any{"approved": "true"}}, {name: "transport-error", transportError: true},
		} {
			t.Run(protocol+"/"+tc.name, func(t *testing.T) {
				f := newOwnerFixture(t, false)
				prepareMemoryFixture(t, f)
				var handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)
				if !tc.unsupported {
					handler = func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
						if tc.transportError {
							return nil, errors.New("synthetic host error")
						}
						return &mcp.ElicitResult{Action: tc.action, Content: tc.content}, nil
					}
				}
				cs := f.connectProtocol(t, handler, protocol)
				out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "memory.capture", "text": "declined-private-memory-marker"}})
				if err == nil && (out == nil || !out.IsError) {
					t.Fatal("invalid host response granted authority")
				}
				if out != nil {
					body, _ := json.Marshal(out)
					if strings.Contains(string(body), "declined-private-memory-marker") {
						t.Fatal("refusal exported private text")
					}
				}
				page := memoryCall(t, cs, "memory.list", nil, false)
				if page["total"] != float64(0) {
					t.Fatal("unapproved capture persisted")
				}
			})
		}
	}
}

func TestMemoryLegacyEligibility(t *testing.T) {
	f := newOwnerFixture(t, false)
	prepareMemoryFixture(t, f)
	e, workspace := fixtureMemoryEngine(t, f)
	if _, err := e.Capture(memory.Capture{WorkspaceID: workspace, RecordedAt: time.Now().UTC(), Kind: "legacy", Text: "unreviewed-legacy-private-marker", Sanitized: true}); err != nil {
		t.Fatal(err)
	}
	cs := f.connect(t, func(_ context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		if !strings.Contains(r.Params.Message, "unreviewed-legacy-private-marker") {
			t.Fatal("trusted correction did not show legacy source")
		}
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	page := memoryCall(t, cs, "memory.list", nil, false)
	record := page["captures"].([]any)[0].(map[string]any)
	if record["eligible"] != false {
		t.Fatal("legacy attestation invented")
	}
	for _, op := range []string{"memory.read", "memory.context"} {
		args := map[string]any{}
		if op == "memory.read" {
			args["capture_id"] = record["id"]
		}
		out := memoryCall(t, cs, op, args, true)
		raw, _ := json.Marshal(out)
		if strings.Contains(string(raw), "unreviewed-legacy-private-marker") {
			t.Fatal("legacy plaintext disclosed")
		}
	}
	found := memoryCall(t, cs, "memory.search", map[string]any{"query": "unreviewed-legacy-private-marker"}, false)
	if found["total"] != float64(0) {
		t.Fatal("search exposed legacy text")
	}
	reviewed := memoryCall(t, cs, "memory.correct", map[string]any{"capture_id": record["id"], "expected_revision": 1, "text": "explicitly-reviewed-replacement"}, false)
	if reviewed["eligible"] != true {
		t.Fatal("reviewed correction not eligible")
	}
	read := memoryCall(t, cs, "memory.read", map[string]any{"capture_id": record["id"]}, false)
	if read["text"] != "explicitly-reviewed-replacement" {
		t.Fatal("reviewed legacy correction missing")
	}
}

func TestMemoryQueryNeverInitializes(t *testing.T) {
	f := newOwnerFixture(t, false)
	cs := f.connect(t, nil)
	memoryCall(t, cs, "memory.list", nil, false)
	memoryCall(t, cs, "memory.search", map[string]any{"query": "synthetic"}, false)
	memoryCall(t, cs, "memory.context", nil, false)
	if _, err := os.Lstat(filepath.Join(f.root, "memory")); !os.IsNotExist(err) {
		t.Fatalf("ordinary query created memory authority: %v", err)
	}
	approve := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	memoryCall(t, approve, "memory.capture", map[string]any{"text": "unprepared capture"}, true)
	if _, err := os.Lstat(filepath.Join(f.root, "memory")); !os.IsNotExist(err) {
		t.Fatal("approved capture bootstrapped absent authority")
	}
}

func TestMemoryRejectsAuthorityAndUnsafeBinding(t *testing.T) {
	f := newOwnerFixture(t, false)
	prepareMemoryFixture(t, f)
	cs := f.connect(t, nil)
	for _, field := range []string{"root", "brain_root", "workspace", "workspace_id", "producer_id", "sanitizer_id", "capability", "key", "sanitized", "confirmed", "approved", "recorded_at", "review_digest"} {
		memoryCall(t, cs, "memory.capture", map[string]any{"text": "forged authority", field: true}, true)
	}
	page := memoryCall(t, cs, "memory.list", nil, false)
	if page["total"] != float64(0) {
		t.Fatal("forged authority persisted")
	}
	for _, kind := range []string{"workspace", "home"} {
		t.Run(kind, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			prepareMemoryFixture(t, f)
			called := false
			cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				called = true
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			})
			path := f.options.WorkspacePath
			if kind == "home" {
				path = filepath.Dir(f.root)
			}
			if err := os.Rename(path, path+"-moved"); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(path+"-moved", path); err != nil {
				t.Fatal(err)
			}
			memoryCall(t, cs, "memory.capture", map[string]any{"text": "unsafe binding marker"}, true)
			if called {
				t.Fatal("unsafe binding reached trusted review")
			}
		})
	}
}

func TestMemoryChangedDuringReviewAndRetarget(t *testing.T) {
	for _, operation := range []string{"memory.correct", "memory.delete", "memory.retention"} {
		t.Run(operation, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			prepareMemoryFixture(t, f)
			e, workspace := fixtureMemoryEngine(t, f)
			original := seedReviewedMemory(t, e, workspace, "reviewed original", "2026-10-12")
			handler := func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				replacement, err := e.SealOwnedCapture(workspace, original.ID, original.Revision, "newer canonical record", time.Now().UTC())
				if err != nil {
					t.Fatal(err)
				}
				if _, err = e.CorrectCapture(workspace, original.ID, original.Revision, replacement); err != nil {
					t.Fatal(err)
				}
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			cs := f.connect(t, handler)
			args := map[string]any{"capture_id": original.ID, "expected_revision": 1}
			if operation == "memory.correct" {
				args["text"] = "stale model replacement"
			}
			if operation == "memory.retention" {
				rev, err := e.VisibilityRevision(workspace)
				if err != nil {
					t.Fatal(err)
				}
				args = map[string]any{"cutoff": time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), "expected_visibility_revision": rev}
			}
			memoryCall(t, cs, operation, args, true)
			current := memoryCall(t, cs, "memory.read", map[string]any{"capture_id": original.ID}, false)
			if current["text"] != "newer canonical record" {
				t.Fatal("stale approval overwrote latest record")
			}
		})
	}
	f := newOwnerFixture(t, false)
	prepareMemoryFixture(t, f)
	e, workspace := fixtureMemoryEngine(t, f)
	first := seedReviewedMemory(t, e, workspace, "first target", "2026-10-12")
	second := seedReviewedMemory(t, e, workspace, "second target", "2026-10-12")
	bound, err := product.Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	request := product.MemoryMutation{Operation: "memory.correct", CaptureID: first.ID, ExpectedRevision: 1, Text: "reviewed replacement"}
	review, err := bound.MemoryReview(request)
	if err != nil {
		t.Fatal(err)
	}
	request.CaptureID = second.ID
	if _, err = bound.ApplyMemoryReview(request, review.ReviewDigest, true); !errors.Is(err, memory.ErrCaptureConflict) {
		t.Fatalf("retargeted review accepted: %v", err)
	}
}

func TestMemoryUTF8Pagination(t *testing.T) {
	f := newOwnerFixture(t, false)
	prepareMemoryFixture(t, f)
	text := strings.Repeat("ação🙂", 600)
	cs := f.connectProtocol(t, func(_ context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		if !strings.Contains(r.Params.Message, text) {
			t.Fatal("exact full capture omitted")
		}
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	}, "2025-11-25")
	receipt := memoryCall(t, cs, "memory.capture", map[string]any{"text": text}, false)
	rebuilt := ""
	offset := 0
	for {
		page := memoryCall(t, cs, "memory.read", map[string]any{"capture_id": receipt["id"], "expected_revision": 1, "offset": offset, "max_bytes": 127}, false)
		part := page["text"].(string)
		if len(part) > 127 || !utf8.ValidString(part) {
			t.Fatal("page split UTF-8")
		}
		rebuilt += part
		next := int(page["next_offset"].(float64))
		if next < 0 {
			break
		}
		if next <= offset {
			t.Fatal("pagination did not progress")
		}
		offset = next
	}
	if rebuilt != text {
		t.Fatal("paged read lost original text")
	}
	memoryCall(t, cs, "memory.read", map[string]any{"capture_id": receipt["id"], "offset": 2, "max_bytes": 4}, true)
}

func TestMemoryCapacityDoesNotBlockExistingRecords(t *testing.T) {
	f := newOwnerFixture(t, false)
	prepareMemoryFixture(t, f)
	e, workspace := fixtureMemoryEngine(t, f)
	live := seedReviewedMemory(t, e, workspace, "eligible existing record", "2026-10-12")
	var journal strings.Builder
	for i := 0; i < 4095; i++ {
		body, err := json.Marshal(memory.Capture{WorkspaceID: workspace, RecordedAt: time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC), Kind: "synthetic-legacy", Text: fmt.Sprintf("capacity-source-%d", i), Sanitized: true})
		if err != nil {
			t.Fatal(err)
		}
		journal.Write(body)
		journal.WriteByte('\n')
	}
	path := filepath.Join(e.Root, "workspaces", workspace, "l1", "captures", "2026-10-12.jsonl")
	if err := os.WriteFile(path, []byte(journal.String()), 0600); err != nil {
		t.Fatal(err)
	}
	cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	refusal := memoryCall(t, cs, "memory.capture", map[string]any{"text": "over lifetime capacity"}, true)
	if refusal["code"] != "memory_capacity" || refusal["retryable"] != false {
		t.Fatalf("capacity outcome: %v", refusal)
	}
	read := memoryCall(t, cs, "memory.read", map[string]any{"capture_id": live.ID}, false)
	if read["text"] != "eligible existing record" {
		t.Fatal("capacity blocked read")
	}
	corrected := memoryCall(t, cs, "memory.correct", map[string]any{"capture_id": live.ID, "expected_revision": 1, "text": "corrected at capacity"}, false)
	if corrected["revision"] != float64(2) {
		t.Fatal("capacity blocked correction")
	}
	memoryCall(t, cs, "memory.delete", map[string]any{"capture_id": live.ID, "expected_revision": 2}, false)
}

func TestMemoryHistoricalFallbackCannotRestoreRemovedText(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			prepareMemoryFixture(t, f)
			e, workspace := fixtureMemoryEngine(t, f)
			approve := func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			cs := f.connect(t, approve)
			original := memoryCall(t, cs, "memory.capture", map[string]any{"text": "never-restore-this-unique-marker"}, false)
			date := time.Now().UTC().Format("2006-01-02")
			memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": date}, false)
			root := filepath.Join(e.Root, "workspaces", workspace)
			saved := map[string][]byte{}
			for _, directory := range []string{"commits", "versions"} {
				err := filepath.WalkDir(filepath.Join(root, directory), func(path string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					if d.IsDir() {
						return nil
					}
					body, err := os.ReadFile(path)
					if err != nil {
						return err
					}
					saved[path] = body
					return nil
				})
				if err != nil {
					t.Fatal(err)
				}
			}
			memoryCall(t, cs, "memory.correct", map[string]any{"capture_id": original["id"], "expected_revision": 1, "text": "only-surviving-replacement-marker"}, false)
			for _, directory := range []string{"commits", "versions"} {
				if err := os.RemoveAll(filepath.Join(root, directory)); err != nil {
					t.Fatal(err)
				}
			}
			for path, body := range saved {
				if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, body, 0600); err != nil {
					t.Fatal(err)
				}
			}
			out, err := f.connect(t, nil).CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_query", Arguments: map[string]any{"operation": "memory.context"}})
			if err != nil {
				t.Fatal(err)
			}
			raw, _ := json.Marshal(out)
			if strings.Contains(string(raw), "never-restore-this-unique-marker") {
				t.Fatal("historical fallback restored revoked text")
			}
			memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": date}, false)
			rebuilt := memoryCall(t, f.connect(t, nil), "memory.context", nil, false)
			if strings.Contains(rebuilt["text"].(string), "never-restore-this-unique-marker") || !strings.Contains(rebuilt["text"].(string), "only-surviving-replacement-marker") {
				t.Fatalf("canonical rebuild: %v", rebuilt)
			}
		})
	}
}

func TestMemoryLocalInputRequiresExactClosedRequest(t *testing.T) {
	f := newOwnerFixture(t, true)
	memoryCLI(t, f, "memory-review", map[string]any{"operation": "memory.retention", "cutoff": time.Now().UTC().Format(time.RFC3339Nano)}, nil, true)
	memoryCLI(t, f, "memory-review", map[string]any{"operation": "memory.capture", "text": "synthetic", "confirmed": true}, nil, true)
}

func TestMemoryLiteralGenerationMarkerDoesNotGrantPromotion(t *testing.T) {
	f := newOwnerFixture(t, false)
	prepareMemoryFixture(t, f)
	promotions := 0
	cs := f.connect(t, func(_ context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		if strings.Contains(r.Params.Message, "Promover memória vitalícia") {
			promotions++
		}
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	memoryCall(t, cs, "memory.capture", map[string]any{"text": "Synthetic quoted literal: # L3 rolling continuity · this sentence is capture text, not a canonical L3 generation."}, false)
	date := time.Now().UTC().Format("2006-01-02")
	memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": date}, false)
	memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "weekly", "date": date}, false)
	view := memoryCall(t, cs, "memory.context", nil, false)
	layers, _ := json.Marshal(view["layers"])
	if promotions != 0 || strings.Contains(string(layers), "lifetime") {
		t.Fatalf("captured literal forged canonical L3 generation evidence: promotion_reviews=%d layers=%s", promotions, layers)
	}
}

func TestMemoryReviewedCaptureConcurrentReplay(t *testing.T) {
	f := newOwnerFixture(t, false)
	prepareMemoryFixture(t, f)
	second, err := product.Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	request := product.MemoryMutation{Operation: "memory.capture", Text: "single approved capture occurrence"}
	review, err := second.MemoryReview(request)
	if err != nil {
		t.Fatal(err)
	}
	armed, calls, injected := false, 0, false
	var competing any
	f.options.LookupEnv = func(key string) (string, bool) {
		if armed && key == "MAESTRO_HOME" {
			calls++
			// Interleave at final binding validation, after the review digest matched.
			if calls == 7 {
				injected = true
				competing, err = second.ApplyMemoryReview(request, review.ReviewDigest, true)
				if err != nil {
					t.Fatalf("competing approved create: %v", err)
				}
			}
		}
		return "", false
	}
	first, err := product.Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	armed = true
	got, applyErr := first.ApplyMemoryReview(request, review.ReviewDigest, true)
	if !injected {
		t.Fatalf("did not interleave after digest validation, lookups=%d", calls)
	}
	if !errors.Is(applyErr, memory.ErrCaptureConflict) {
		t.Fatalf("stale approved create succeeded: %#v %v", got, applyErr)
	}
	if rejected, ok := got.(memory.CaptureReceipt); ok && rejected.ID != "" {
		t.Fatalf("stale create returned a capture identity: %#v", rejected)
	}
	e, workspace := fixtureMemoryEngine(t, f)
	page, err := e.ListCaptures(workspace, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	winner, ok := competing.(memory.CaptureReceipt)
	if !ok || winner.VisibilityRevision != 1 || winner.Revision != 1 || page.Total != 1 || page.VisibilityRevision != 1 || page.Captures[0].ID != winner.ID {
		t.Fatalf("one review produced multiple effects: winner=%#v page=%#v", competing, page)
	}
}
