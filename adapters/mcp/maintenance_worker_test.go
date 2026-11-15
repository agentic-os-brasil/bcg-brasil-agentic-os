package product_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const ownedMaintenanceTestEnv = "MAESTRO_TEST_OWNED_MAINTENANCE"

type ownedMaintenanceTestBinding struct {
	ProductRoot, UserHome, WorkspacePath string
	Clock                                string
	Activity                             []maintenance.IdleState
	BlockObservation                     uint32
}

func testMaintenanceOwnedBackground(t *testing.T) {
	if raw := os.Getenv(ownedMaintenanceTestEnv); raw != "" {
		var in ownedMaintenanceTestBinding
		if err := json.Unmarshal([]byte(raw), &in); err != nil {
			t.Fatal(err)
		}
		bound, err := product.Open(product.Options{ProductRoot: in.ProductRoot, UserHome: in.UserHome, WorkspacePath: in.WorkspacePath, LookupEnv: func(string) (string, bool) { return "", false }})
		if err != nil {
			t.Fatal(err)
		}
		product.SetMaintenanceClockForTest(bound, func() time.Time {
			body, err := os.ReadFile(in.Clock)
			if err != nil {
				panic("owned maintenance test clock unavailable")
			}
			at, err := time.Parse(time.RFC3339Nano, string(body))
			if err != nil {
				panic("owned maintenance test clock invalid")
			}
			return at
		})
		var observations atomic.Uint32
		product.SetMaintenanceActivityForTest(bound, func(ctx context.Context) (maintenance.IdleState, error) {
			n := observations.Add(1)
			if err := os.WriteFile(fmt.Sprintf("%s.%d.observed-%d", in.Clock, os.Getpid(), n), nil, 0600); err != nil {
				panic("owned maintenance observation marker unavailable")
			}
			if n == in.BlockObservation {
				<-ctx.Done()
				if err := os.WriteFile(fmt.Sprintf("%s.%d.cancelled", in.Clock, os.Getpid()), nil, 0600); err != nil {
					panic("owned maintenance cancellation marker unavailable")
				}
				return maintenance.IdleUnknown, ctx.Err()
			}
			if len(in.Activity) == 0 {
				return maintenance.IdleUnknown, nil
			}
			return in.Activity[min(int(n)-1, len(in.Activity)-1)], nil
		})
		if err := bound.Serve(context.Background(), &mcp.StdioTransport{}); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		os.Exit(0)
	}

	f := newOwnerFixture(t, false)
	prepareMemoryFixture(t, f)
	bound, err := product.Open(f.options)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	initial := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var clock atomic.Int64
	clock.Store(initial.UnixNano())
	product.SetMaintenanceClockForTest(bound, func() time.Time { return time.Unix(0, clock.Load()).UTC() })
	a, z := mcp.NewInMemoryTransports()
	server, err := bound.Server().Connect(context.Background(), a, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "owned-maintenance-review", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	}})
	cs, err := client.Connect(context.Background(), z, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	memoryCall(t, cs, "memory.capture", map[string]any{"text": "owned upkeep initial reviewed action"}, false)
	job := maintenance.MemoryLightDreamJobID
	qualified := cadenceCall(t, cs, "qualify", map[string]any{"job_id": job, "expected_revision": 0}, false)
	cadenceCall(t, cs, "enroll", map[string]any{"job_id": job, "expected_revision": qualified["revision"], "timezone": "UTC"}, false)
	wikiJob := maintenance.WikiIncrementalJobID
	wikiQualified := cadenceCall(t, cs, "qualify", map[string]any{"job_id": wikiJob, "expected_revision": 0}, false)
	cadenceCall(t, cs, "enroll", map[string]any{"job_id": wikiJob, "expected_revision": wikiQualified["revision"], "timezone": "UTC"}, false)
	initialGeneration := wikiCall(t, cs, "status", nil, false)["generation"]
	const fresh = "owned upkeep next-session reviewed action"
	memoryCall(t, cs, "memory.capture", map[string]any{"text": fresh}, false)
	clockPath := filepath.Join(physicalTempDir(t), "owned-clock")
	setClock := func(at time.Time) {
		t.Helper()
		next := clockPath + ".next"
		if err := os.WriteFile(next, []byte(at.Format(time.RFC3339Nano)), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(next, clockPath); err != nil {
			t.Fatal(err)
		}
		clock.Store(at.UnixNano())
	}
	setClock(initial.Add(4 * time.Hour))

	connect := func(block uint32, activity ...maintenance.IdleState) (*mcp.ClientSession, *exec.Cmd) {
		t.Helper()
		body, err := json.Marshal(ownedMaintenanceTestBinding{ProductRoot: f.options.ProductRoot, UserHome: f.options.UserHome, WorkspacePath: f.options.WorkspacePath, Clock: clockPath, Activity: activity, BlockObservation: block})
		if err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(os.Args[0], "-test.run=^TestMaintenancePublicJourney$/^owned-background$")
		cmd.Env = append(os.Environ(), ownedMaintenanceTestEnv+"="+string(body))
		cmd.Stderr = os.Stderr
		client := mcp.NewClient(&mcp.Implementation{Name: "owned-maintenance-reader", Version: "1"}, nil)
		out, err := client.Connect(context.Background(), &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { out.Close() })
		return out, cmd
	}
	closeServer := func(session *mcp.ClientSession, cmd *exec.Cmd) {
		t.Helper()
		started := time.Now()
		if err := session.Close(); err != nil {
			t.Fatal(err)
		}
		if cmd.ProcessState == nil || cmd.ProcessState.ExitCode() != 0 || time.Since(started) > 5*time.Second {
			t.Fatal("MCP close did not cancel and join its owned worker")
		}
	}
	for _, state := range []maintenance.IdleState{maintenance.IdleActive, maintenance.IdleUnknown} {
		before := cadenceTree(t, f)
		inactive, process := connect(0, state)
		marker := fmt.Sprintf("%s.%d.observed-1", clockPath, process.Process.Pid)
		observed := false
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			if _, err := os.Stat(marker); err == nil {
				observed = true
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		closeServer(inactive, process)
		if cadenceTree(t, f) != before {
			t.Fatalf("background work changed state without a confirmed native idle observation: %s", state)
		}
		if !observed {
			t.Fatal("background dispatch did not consult native eligibility")
		}
	}
	workspace := cadenceCall(t, cs, "status", nil, false)["workspace_id"].(string)
	engine, _, err := maintenance.NewProductMemoryEngine(filepath.Dir(f.root))
	if err != nil {
		t.Fatal(err)
	}
	store := maintenance.Store{Root: filepath.Join(filepath.Dir(f.root), "state", "maintenance")}
	interrupted, interruptedProcess := connect(2, maintenance.IdleConfirmed, maintenance.IdleConfirmed, maintenance.IdleActive)
	observedActivity := fmt.Sprintf("%s.%d.observed-3", clockPath, interruptedProcess.Process.Pid)
	cancelledProbe := fmt.Sprintf("%s.%d.cancelled", clockPath, interruptedProcess.Process.Pid)
	activityCancelled := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		_, observedErr := os.Stat(observedActivity)
		_, cancelledErr := os.Stat(cancelledProbe)
		if observedErr == nil && cancelledErr == nil {
			activityCancelled = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !activityCancelled {
		t.Fatal("native activity monitor did not cancel running work while MCP remained open")
	}
	schedulerStore := scheduler.Store{Root: filepath.Join(filepath.Dir(f.root), "state", "scheduler")}
	cleaned := false
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); {
		leases, err := schedulerStore.QuarantinedLeases(workspace)
		if err != nil && !errors.Is(err, scheduler.ErrLeaseBusy) {
			t.Fatal(err)
		}
		if err == nil && len(leases) == 0 {
			cleaned = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !cleaned {
		t.Fatal("cancelled cooperative handler did not release its canonical fence")
	}
	closeServer(interrupted, interruptedProcess)
	prior, _, err := engine.ActiveArtifact(workspace, "L1/"+initial.Format("2006-01-02"))
	if err != nil || strings.Contains(prior.Content, fresh) {
		t.Fatal("activity-return cancellation changed the last-good L1")
	}
	if current := wikiCall(t, cs, "status", nil, false); current["generation"] != initialGeneration || current["state"] != "current" {
		t.Fatal("activity-return cancellation changed last-good navigation")
	}
	for _, id := range []string{job, wikiJob} {
		receipts, err := store.Receipts(workspace, id)
		if err != nil {
			t.Fatal(err)
		}
		succeeded := 0
		for _, receipt := range receipts {
			if receipt.State == maintenance.ReceiptSucceeded {
				succeeded++
			}
		}
		if succeeded != 1 {
			t.Fatal("activity-return cancellation published a successful occurrence")
		}
	}
	first, firstProcess := connect(0, maintenance.IdleConfirmed)
	second, secondProcess := connect(0, maintenance.IdleConfirmed)
	awaitCommit := func(marker string, expected int, wait time.Duration) {
		t.Helper()
		deadline := time.Now().Add(wait)
		for time.Now().Before(deadline) {
			artifact, _, artifactErr := engine.ActiveArtifact(workspace, "L1/"+initial.Format("2006-01-02"))
			wiki, wikiErr := engine.InspectPrivateWikiMaintenance(workspace)
			completed := true
			for _, id := range []string{job, wikiJob} {
				receipts, receiptErr := store.Receipts(workspace, id)
				succeeded := 0
				for _, receipt := range receipts {
					if receipt.State == maintenance.ReceiptSucceeded {
						succeeded++
					}
				}
				if succeeded > expected {
					t.Fatal("competing owned servers duplicated a successful occurrence")
				}
				completed = completed && receiptErr == nil && succeeded == expected
			}
			if artifactErr == nil && strings.Contains(artifact.Content, marker) && wikiErr == nil && wiki.State == "current" && wiki.Generation != initialGeneration && completed {
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
		artifact, _, artifactErr := engine.ActiveArtifact(workspace, "L1/"+initial.Format("2006-01-02"))
		wiki, wikiErr := engine.InspectPrivateWikiMaintenance(workspace)
		t.Logf("owned maintenance expiry expected_successes=%d clock=%s artifact_read=%t marker_present=%t wiki_read=%t wiki_state=%s generation_changed=%t event_pending=%t event_missing=%t source_ready=%t retirement_complete=%t", expected, time.Unix(0, clock.Load()).UTC().Format(time.RFC3339Nano), artifactErr == nil, strings.Contains(artifact.Content, marker), wikiErr == nil, wiki.State, wiki.Generation != initialGeneration, wiki.EventPending, wiki.EventMissing, wiki.SourceReady, wiki.RetirementComplete)
		status := cadenceCall(t, cs, "status", nil, false)
		for _, id := range []string{job, wikiJob} {
			state := cadenceJob(t, status, id)
			receipts, err := store.Receipts(workspace, id)
			t.Logf("owned maintenance job=%s state=%v revision=%v qualification=%v reason=%v receipts_read=%t receipts=%d", id, state["state"], state["revision"], state["qualification_state"], state["reason"], err == nil, len(receipts))
			for _, receipt := range receipts[max(0, len(receipts)-4):] {
				t.Logf("owned maintenance receipt job=%s state=%s reason=%s recorded_at=%s", id, receipt.State, receipt.ReasonCode, receipt.RecordedAt.Format(time.RFC3339Nano))
			}
		}
		manual := cadenceCall(t, cs, "run", nil, false)
		t.Fatalf("owned serve did not publish due L1/navigation and receipts; manual diagnosis state=%v due=%v receipts=%v", manual["state"], manual["due"], manual["receipts"])
	}
	awaitCommit(fresh, 2, 5*time.Second)
	const nextPulse = "owned upkeep recurring pulse reviewed action"
	memoryCall(t, cs, "memory.capture", map[string]any{"text": nextPulse}, false)
	setClock(initial.Add(8 * time.Hour))
	awaitCommit(nextPulse, 3, 45*time.Second)
	closeServer(first, firstProcess)
	closeServer(second, secondProcess)
	before := cadenceTree(t, f)
	restarted, restartedProcess := connect(0, maintenance.IdleConfirmed)
	view := memoryCall(t, restarted, "memory.context", nil, false)
	body, err := json.Marshal(view)
	if err != nil || !strings.Contains(string(body), nextPulse) {
		t.Fatal("cold MCP session did not consume the background commit")
	}
	navigation := wikiCall(t, restarted, "status", nil, false)
	if navigation["state"] != "current" || navigation["generation"] == initialGeneration {
		t.Fatal("cold MCP session did not consume background navigation")
	}
	generation := navigation["generation"].(string)
	index := wikiCall(t, restarted, "read", map[string]any{"generation": generation, "reference": "index.md", "max_bytes": 4096}, false)["content"].(string)
	link := regexp.MustCompile(`\((concepts/[a-f0-9]{64}\.md)\)`).FindStringSubmatch(index)
	if len(link) != 2 {
		t.Fatal("background navigation did not publish a concept route")
	}
	concept := wikiCall(t, restarted, "read", map[string]any{"generation": generation, "reference": link[1], "max_bytes": 4096}, false)["content"].(string)
	backlinks := wikiCall(t, restarted, "read", map[string]any{"generation": generation, "reference": "backlinks.json", "max_bytes": 4096}, false)["content"].(string)
	if !strings.Contains(concept, nextPulse) || !strings.Contains(backlinks, link[1]) {
		t.Fatal("background concept/backlinks omitted the recurring source")
	}
	closeServer(restarted, restartedProcess)
	if cadenceTree(t, f) != before {
		t.Fatal("restart or duplicate pulse changed already completed work")
	}
	t.Log("owned_background native_active_refused=true native_unknown_refused=true activity_return_cancelled=true cancelled_handler_fence_released=true occurrences=2 light_successes=3 wiki_successes=3 competing_processes=2 cold_context=true cold_navigation=true graceful_close=true replay_unchanged=true controlled_constructor_clock_and_activity=true")
}
