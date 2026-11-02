package product_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func cadenceCall(t *testing.T, cs *mcp.ClientSession, op string, fields map[string]any, fail bool) map[string]any {
	t.Helper()
	args := map[string]any{"operation": "maintenance." + op}
	for k, v := range fields {
		args[k] = v
	}
	tool := "maestro_apply"
	if op == "status" {
		tool = "maestro_query"
	}
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	body := ""
	if len(out.Content) > 0 {
		body = out.Content[0].(*mcp.TextContent).Text
	}
	if out.IsError != fail || len(out.Content) != 1 {
		t.Fatalf("maintenance.%s: %+v content=%s", op, out, body)
	}
	if len(body) > 64<<10 {
		t.Fatal("unbounded maintenance output")
	}
	for _, private := range []string{"private-maintenance-text", "\"binding\"", "\"grant\"", "\"seal\"", "owned-captures/", "source_digest", "product-control/"} {
		if strings.Contains(body, private) {
			t.Fatalf("maintenance output leaked %s: %s", private, body)
		}
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func cadenceJob(t *testing.T, status map[string]any, id string) map[string]any {
	t.Helper()
	for _, raw := range status["jobs"].([]any) {
		job := raw.(map[string]any)
		if job["job_id"] == id {
			return job
		}
	}
	t.Fatalf("missing job %s: %v", id, status)
	return nil
}

func cadenceTree(t *testing.T, f ownerFixture) string {
	t.Helper()
	hash := sha256.New()
	root := filepath.Dir(f.root)
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		fmt.Fprintf(hash, "%s:%v:%d\n", rel, info.Mode(), info.Size())
		if info.Mode().IsRegular() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			hash.Write(body)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return fmt.Sprintf("%x", hash.Sum(nil))
}

func TestMaintenanceFreshStatusReadOnly(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			cs := f.connect(t, nil)
			before := cadenceTree(t, f)
			status := cadenceCall(t, cs, "status", nil, false)
			if len(status["jobs"].([]any)) != 18 {
				t.Fatal(status)
			}
			for _, raw := range status["jobs"].([]any) {
				job := raw.(map[string]any)
				if job["revision"] != float64(0) || job["state"] != "inactive" || job["qualification_state"] != "unqualified" {
					t.Fatal(job)
				}
			}
			if got := cadenceCall(t, cs, "run", nil, false); got["state"] != "no_due_work" {
				t.Fatal(got)
			}
			if cadenceTree(t, f) != before {
				t.Fatal("ordinary status/no-due run initialized state or keys")
			}
		})
	}
}

func TestMaintenancePublicJourney(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			prepareMemoryFixture(t, f)
			var clock atomic.Int64
			// Only this in-process SDK fixture has the constructor clock. The
			// rebuilt production stdio executable always observes system time.
			connect := func(handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)) *mcp.ClientSession {
				if stdio {
					return f.connect(t, handler)
				}
				bound, err := product.Open(f.options)
				if err != nil {
					t.Fatal(err)
				}
				product.SetMaintenanceClockForTest(bound, func() time.Time { return time.Now().UTC().Add(time.Duration(clock.Load())) })
				a, z := mcp.NewInMemoryTransports()
				server, err := bound.Server().Connect(context.Background(), a, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { server.Close() })
				client := mcp.NewClient(&mcp.Implementation{Name: "maintenance-clock-fixture", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: handler})
				cs, err := client.Connect(context.Background(), z, nil)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { cs.Close() })
				return cs
			}
			var messages []string
			approve := func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				messages = append(messages, req.Params.Message)
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
			cs := connect(approve)
			status := cadenceCall(t, cs, "status", nil, false)
			workspace := status["workspace_id"].(string)
			light := maintenance.MemoryLightDreamJobID
			checkpoint := maintenance.MemoryCheckpointJobID
			beforeDecline := cadenceTree(t, f)
			declining := connect(func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "decline"}, nil
			})
			declined := cadenceCall(t, declining, "qualify", map[string]any{"job_id": light, "expected_revision": 0}, true)
			if declined["code"] != "confirmation_declined" || cadenceTree(t, f) != beforeDecline {
				t.Fatal("declined SDK/stdio review wrote maintenance state")
			}
			// Empty sources cannot become qualification.
			cadenceCall(t, cs, "qualify", map[string]any{"job_id": checkpoint, "expected_revision": 0}, true)
			memoryCall(t, cs, "memory.capture", map[string]any{"text": "private-maintenance-text: retain the next action"}, false)
			qualified := cadenceCall(t, cs, "qualify", map[string]any{"job_id": light, "expected_revision": 0}, false)
			if qualified["qualification_state"] != "qualified" || qualified["state"] != "inactive" {
				t.Fatal(qualified)
			}
			q := qualified["qualification"].(map[string]any)
			if q["adapter"] != "local" || q["artifact_ref"] == "" || q["id"] == "" {
				t.Fatal(q)
			}
			engine, _, err := maintenance.NewProductMemoryEngine(filepath.Dir(f.root))
			if err != nil {
				t.Fatal(err)
			}
			artifact, _, err := engine.ActiveArtifact(workspace, "L1/"+time.Now().UTC().Format("2006-01-02"))
			if err != nil || !strings.Contains(artifact.Content, "private-maintenance-text") || len(artifact.Sources) == 0 {
				t.Fatalf("actual L1 absent: %+v %v", artifact, err)
			}
			qualifiedArtifact := artifact
			sched := scheduler.Store{Root: filepath.Join(filepath.Dir(f.root), "state", "scheduler")}
			receipts, err := sched.Receipts(workspace)
			if err != nil || len(receipts) == 0 {
				t.Fatalf("scheduler receipt missing: %v %v", receipts, err)
			}
			store := maintenance.Store{Root: filepath.Join(filepath.Dir(f.root), "state", "maintenance")}
			durable, err := store.Receipts(workspace, light)
			if err != nil || len(durable) != 1 || durable[0].State != maintenance.ReceiptSucceeded {
				t.Fatalf("maintenance receipt missing: %v %v", durable, err)
			}
			enrolled := cadenceCall(t, cs, "enroll", map[string]any{"job_id": light, "expected_revision": qualified["revision"], "timezone": "America/Sao_Paulo"}, false)
			status = cadenceCall(t, cs, "status", nil, false)
			active := 0
			for _, raw := range status["jobs"].([]any) {
				if raw.(map[string]any)["state"] == "active" {
					active++
				}
			}
			if active != 1 || enrolled["state"] != "active" {
				t.Fatal(status)
			}
			cadenceCall(t, cs, "pause", map[string]any{"job_id": light, "expected_revision": qualified["revision"]}, true)
			// Separate qualification uses the real light-dream receipt. The earlier
			// source-free checkpoint attempt may have advanced its own revision.
			cpState := cadenceJob(t, status, checkpoint)
			cpQualified := cadenceCall(t, cs, "qualify", map[string]any{"job_id": checkpoint, "expected_revision": cpState["revision"]}, false)
			if cpQualified["qualification_state"] != "qualified" || cpQualified["state"] != "inactive" {
				t.Fatal(cpQualified)
			}
			cp, err := (maintenance.ContinuityCheckpointStore{Root: store.Root}).Load(workspace)
			if err != nil || cp.SourceCount < 1 {
				t.Fatalf("checkpoint absent: %+v %v", cp, err)
			}
			// New reviewed work makes the real due run produce a distinct L1.
			captured := memoryCall(t, cs, "memory.capture", map[string]any{"text": "private-maintenance-text: second reviewed action"}, false)
			capture := memoryCall(t, cs, "memory.read", map[string]any{"capture_id": captured["id"]}, false)
			capturedAt, err := time.Parse(time.RFC3339Nano, capture["recorded_at"].(string))
			if err != nil {
				t.Fatal(err)
			}
			if !stdio {
				clock.Add(int64(4 * time.Hour))
			}
			paused := cadenceCall(t, cs, "pause", map[string]any{"job_id": light, "expected_revision": enrolled["revision"]}, false)
			if got := cadenceCall(t, cs, "run", nil, false); got["state"] != "no_due_work" {
				t.Fatal(got)
			}
			resumed := cadenceCall(t, cs, "resume", map[string]any{"job_id": light, "expected_revision": paused["revision"]}, false)
			run := cadenceCall(t, cs, "run", nil, false)
			if !stdio {
				if run["state"] != "processed" || len(run["receipts"].([]any)) != 1 {
					t.Fatal(run)
				}
				receipt := run["receipts"].([]any)[0].(map[string]any)
				executedAt, err := time.Parse(time.RFC3339Nano, receipt["recorded_at"].(string))
				if err != nil {
					t.Fatal(err)
				}
				executionDay := executedAt.UTC().Format("2006-01-02")
				if executionDay == capturedAt.UTC().Format("2006-01-02") {
					if receipt["state"] != "succeeded" {
						t.Fatal(run)
					}
					artifact, _, err = engine.ActiveArtifact(workspace, "L1/"+executionDay)
					if err != nil || !strings.Contains(artifact.Content, "second reviewed action") {
						t.Fatalf("due L1 missing: %+v %v", artifact, err)
					}
				} else {
					// A late-night run may cross UTC midnight. It processes the
					// execution day, preserving the prior artifact rather than
					// inventing synthesis or backfilling yesterday's captures.
					if receipt["state"] != "reviewed_no_change" {
						t.Fatal(run)
					}
					if _, _, err := engine.ActiveArtifact(workspace, "L1/"+executionDay); !os.IsNotExist(err) {
						t.Fatalf("source-free execution day produced L1: %v", err)
					}
					prior, _, err := engine.ActiveArtifact(workspace, "L1/"+qualifiedArtifact.Period)
					if err != nil || prior.Content != qualifiedArtifact.Content {
						t.Fatal("source-free due run changed last-known-good L1")
					}
				}
				t.Logf("controlled SDK due state=%s execution_day=%s capture_day=%s", receipt["state"], executionDay, capturedAt.UTC().Format("2006-01-02"))
			} else if run["state"] != "no_due_work" {
				t.Fatal(run)
			}
			before := cadenceTree(t, f)
			restarted := connect(approve)
			readback := cadenceCall(t, restarted, "status", nil, false)
			if cadenceJob(t, readback, checkpoint)["qualification_state"] != "qualified" {
				t.Fatal(readback)
			}
			if got := cadenceCall(t, restarted, "run", nil, false); got["state"] != "no_due_work" {
				t.Fatal(got)
			}
			if cadenceTree(t, f) != before {
				t.Fatal("restart/replay duplicated output or mutated readback")
			}
			revoked := cadenceCall(t, restarted, "revoke", map[string]any{"job_id": light, "expected_revision": resumed["revision"]}, false)
			if revoked["state"] != "revoked" {
				t.Fatal(revoked)
			}
			cadenceCall(t, restarted, "resume", map[string]any{"job_id": light, "expected_revision": revoked["revision"]}, true)
			if len(messages) < 7 {
				t.Fatal("trusted full reviews not observed")
			}
			found := false
			for _, message := range messages {
				if strings.Contains(message, "maintenance.enroll") {
					found = strings.Contains(message, workspace) && strings.Contains(message, "America/Sao_Paulo") && strings.Contains(message, "max_catch_up") && strings.Contains(message, "memory_l1") && strings.Contains(message, q["id"].(string))
				}
			}
			if !found {
				t.Fatalf("exact enrollment review incomplete: %v", messages)
			}
		})
	}
}
