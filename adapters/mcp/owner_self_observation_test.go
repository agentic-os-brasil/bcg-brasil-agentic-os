package product_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func selfObservationCall(t *testing.T, client *mcp.ClientSession, tool, operation string, fields map[string]any, refused bool) map[string]any {
	t.Helper()
	args := map[string]any{"operation": operation}
	for key, value := range fields {
		args[key] = value
	}
	response, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Content) != 1 {
		t.Fatal("self observation returned no single result")
	}
	body := response.Content[0].(*mcp.TextContent).Text
	var value map[string]any
	if err := json.Unmarshal([]byte(body), &value); err != nil {
		t.Fatal(err)
	}
	if response.IsError != refused {
		t.Fatalf("%s: refused=%t result=%s", operation, response.IsError, body)
	}
	if strings.Contains(body, "owner_pref_concise") {
		t.Fatal("private declaration leaked outside trusted review")
	}
	return value
}

func TestOwnerSelfObservationPublicIntake(t *testing.T) {
	for _, tc := range []struct {
		name  string
		stdio bool
	}{{"in_memory", false}, {"compiled_stdio", true}} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newOwnerFixture(t, tc.stdio)
			approved := false
			var review string
			client := fixture.connect(t, func(_ context.Context, request *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				review = request.Params.Message
				if !approved {
					return &mcp.ElicitResult{Action: "decline"}, nil
				}
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			})
			status := selfObservationCall(t, client, "maestro_query", "owner.observation.status", nil, false)
			if status["total"] != float64(0) {
				t.Fatal("fresh intake was not empty", status)
			}
			declaration := map[string]any{"signal": "explicit_instruction", "facet": "communication-style", "claim": "owner_pref_concise", "confidence": 0.95, "expires_at": time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339Nano)}
			selfObservationCall(t, client, "maestro_apply", "owner.observation.intake", map[string]any{"self_observation": declaration}, true)
			status = selfObservationCall(t, client, "maestro_query", "owner.observation.status", nil, false)
			if status["total"] != float64(0) {
				t.Fatal("declined intake persisted", status)
			}
			approved = true
			receipt := selfObservationCall(t, client, "maestro_apply", "owner.observation.intake", map[string]any{"self_observation": declaration}, false)
			if receipt["state"] != "approved" || receipt["revision"] != float64(1) {
				t.Fatal("approval did not create only pending intake", receipt)
			}
			if !strings.Contains(review, "owner_pref_concise") || !strings.Contains(review, "24") && !strings.Contains(review, "Expira") {
				t.Fatal("trusted review omitted declaration fields")
			}
			observations, err := ownerctx.ListObservations(fixture.root)
			if err != nil || len(observations) != 0 {
				t.Fatalf("intake performed canonical capture: %v %v", observations, err)
			}
			status = selfObservationCall(t, client, "maestro_query", "owner.observation.status", nil, false)
			if status["total"] != float64(1) {
				t.Fatal("approved intake is not discoverable", status)
			}
			again := selfObservationCall(t, client, "maestro_apply", "owner.observation.intake", map[string]any{"self_observation": declaration}, false)
			if again["id"] != receipt["id"] || again["revision"] != receipt["revision"] {
				t.Fatal("identical declaration was duplicated", again)
			}
			// A new transport is not an independent owner interaction or a new source.
			reconnected := fixture.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			})
			replay := selfObservationCall(t, reconnected, "maestro_apply", "owner.observation.intake", map[string]any{"self_observation": declaration}, false)
			if replay["id"] != receipt["id"] {
				t.Fatal("reconnect minted a new declaration", replay)
			}
			approved = false
			selfObservationCall(t, client, "maestro_apply", "owner.observation.reject", map[string]any{"intake_id": receipt["id"], "next": "rejected"}, true)
			status = selfObservationCall(t, client, "maestro_query", "owner.observation.status", nil, false)
			first := status["items"].([]any)[0].(map[string]any)
			if first["intake_state"] != "approved" {
				t.Fatal("declined rejection changed intake", first)
			}
			approved = true
			rejected := selfObservationCall(t, client, "maestro_apply", "owner.observation.reject", map[string]any{"intake_id": receipt["id"], "next": "rejected"}, false)
			if rejected["state"] != "rejected" {
				t.Fatal("approved rejection was not durable", rejected)
			}
			selfObservationCall(t, reconnected, "maestro_apply", "owner.observation.intake", map[string]any{"self_observation": declaration}, true)
			redacted := selfObservationCall(t, client, "maestro_apply", "owner.observation.reject", map[string]any{"intake_id": receipt["id"], "next": "redacted"}, false)
			if redacted["state"] != "redacted" {
				t.Fatal("approved redaction was not durable", redacted)
			}
			status = selfObservationCall(t, client, "maestro_query", "owner.observation.status", map[string]any{"offset": 0, "limit": 1}, false)
			first = status["items"].([]any)[0].(map[string]any)
			if first["intake_state"] != "redacted" || first["observation_state"] != "absent" || first["capture_pending"] != false {
				t.Fatal("status invented capture or dropped denial", first)
			}
		})
	}
}

func TestOwnerSelfObservationInputBoundary(t *testing.T) {
	fixture := newOwnerFixture(t, false)
	reviews := 0
	client := fixture.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		reviews++
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	declaration := map[string]any{"signal": "explicit_instruction", "facet": "communication-style", "claim": "owner_pref_concise", "confidence": 0.9, "expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)}
	for _, field := range []string{"source_digest", "episode_id", "evidence_type", "scope_kind", "scope_id", "authenticated_owner", "owner_confirmed", "declassified_global", "material", "root", "review_digest"} {
		extra := map[string]any{}
		for k, v := range declaration {
			extra[k] = v
		}
		extra[field] = true
		response, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "owner.observation.intake", "self_observation": extra}})
		if err == nil && response != nil && !response.IsError {
			t.Fatalf("model-controlled authority accepted: %s", field)
		}
	}
	for field, value := range map[string]any{"signal": "observed_pattern", "facet": "psychological-profile", "claim": "raw sentence from a conversation", "confidence": 2.0} {
		extra := map[string]any{}
		for k, v := range declaration {
			extra[k] = v
		}
		extra[field] = value
		selfObservationCall(t, client, "maestro_apply", "owner.observation.intake", map[string]any{"self_observation": extra}, true)
	}
	for _, fields := range []map[string]any{{"offset": -1}, {"limit": 33}, {"offset": 1025}, {"self_observation": declaration}} {
		selfObservationCall(t, client, "maestro_query", "owner.observation.status", fields, true)
	}
	selfObservationCall(t, client, "maestro_query", "owner.observation.intake", map[string]any{"self_observation": declaration}, true)
	if reviews != 0 {
		t.Fatal("invalid metadata reached trusted review", reviews)
	}
	status := selfObservationCall(t, client, "maestro_query", "owner.observation.status", nil, false)
	if status["total"] != float64(0) {
		t.Fatal("invalid input created intake", status)
	}
}

func TestOwnerSelfObservationStaleReview(t *testing.T) {
	fixture := newOwnerFixture(t, false)
	changed := false
	client := fixture.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		if !changed {
			changed = true
			submission, err := ownerctx.NewSelfObservationDeclaration(fixture.root, ownerctx.SelfObservationDeclaration{Signal: ownerctx.SignalExplicitInstruction, Facet: "preferences", Claim: "other_reviewed_preference", Confidence: 0.8, ExpiresAt: time.Now().UTC().Add(time.Hour)})
			if err != nil {
				return nil, err
			}
			review, err := ownerctx.ReviewSelfObservation(fixture.root, submission)
			if err != nil {
				return nil, err
			}
			if _, err = ownerctx.ApproveSelfObservation(context.Background(), fixture.root, review, true); err != nil {
				return nil, err
			}
		}
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	declaration := map[string]any{"signal": "explicit_instruction", "facet": "communication-style", "claim": "owner_pref_concise", "confidence": 0.9, "expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)}
	selfObservationCall(t, client, "maestro_apply", "owner.observation.intake", map[string]any{"self_observation": declaration}, true)
	status := selfObservationCall(t, client, "maestro_query", "owner.observation.status", nil, false)
	if status["total"] != float64(1) {
		t.Fatal("stale approval was applied or newer intake lost", status)
	}
	observations, err := ownerctx.ListObservations(fixture.root)
	if err != nil || len(observations) != 0 {
		t.Fatal("stale intake wrote canonical observations", err)
	}
}

func TestOwnerSelfObservationMethodDiscovery(t *testing.T) {
	fixture := newOwnerFixture(t, false)
	client := fixture.connect(t, nil)
	plan := selfObservationCall(t, client, "maestro_query", "methods.plan", map[string]any{"method_id": "interaction-profile"}, false)
	found := false
	for _, raw := range plan["dependencies"].([]any) {
		dependency := raw.(map[string]any)
		if dependency["id"] == "domain.owner_self_declaration" {
			found = dependency["state"] == "available"
		}
	}
	if !found || plan["execution_state"] != "not_executed" {
		t.Fatal("declaration method is undiscoverable or claims execution", plan)
	}
	resource := selfObservationCall(t, client, "maestro_query", "methods.read", map[string]any{"resource_uri": "maestro://methods/interaction-profile/resources/self-observation.md"}, false)
	for _, operation := range []string{"owner.observation.status", "owner.observation.intake", "owner.observation.reject", "self-observation-capture"} {
		if !strings.Contains(resource["text"].(string), operation) {
			t.Fatal("packaged recipe is missing owning operation", operation)
		}
	}
	methods := selfObservationCall(t, client, "maestro_query", "methods.list", map[string]any{"limit": 1}, false)
	if methods["total"] != float64(92) {
		t.Fatal("method wiring changed canonical method count", methods["total"])
	}
}

func TestOwnerSelfObservationQueryIsReadOnly(t *testing.T) {
	fixture := newOwnerFixture(t, false)
	client := fixture.connect(t, nil)
	before := methodTreeSnapshot(t, fixture.root)
	for i := 0; i < 2; i++ {
		selfObservationCall(t, client, "maestro_query", "owner.observation.status", nil, false)
	}
	if !reflect.DeepEqual(before, methodTreeSnapshot(t, fixture.root)) {
		t.Fatal("observation query created, repaired or changed owner state")
	}
}

func selfObservationJob(t *testing.T, client *mcp.ClientSession) map[string]any {
	t.Helper()
	status := selfObservationCall(t, client, "maestro_query", "maintenance.status", nil, false)
	for _, raw := range status["jobs"].([]any) {
		job := raw.(map[string]any)
		if job["job_id"] == "self-observation-capture" {
			return job
		}
	}
	t.Fatal("self observation job is absent from public status")
	return nil
}

func TestOwnerSelfObservationMaintenanceSDK(t *testing.T) {
	fixture := newOwnerFixture(t, true)
	allowJob := false
	declarations, jobs := 0, 0
	client := fixture.connect(t, func(_ context.Context, request *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		if strings.Contains(request.Params.Message, "maintenance.qualify") {
			jobs++
			if !strings.Contains(request.Params.Message, "one shared owner control") {
				t.Error("job review omitted shared owner scope")
			}
			if !allowJob {
				return &mcp.ElicitResult{Action: "decline"}, nil
			}
		} else {
			declarations++
		}
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	facet := filepath.Join(fixture.root, "owner", "self", "voice.md")
	before, err := os.ReadFile(facet)
	if err != nil {
		t.Fatal(err)
	}
	declaration := map[string]any{"signal": "explicit_instruction", "facet": "voice", "claim": "owner_pref_concise", "confidence": 0.9, "expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)}
	intake := selfObservationCall(t, client, "maestro_apply", "owner.observation.intake", map[string]any{"self_observation": declaration}, false)
	observations, err := ownerctx.ListObservations(fixture.root)
	if err != nil || len(observations) != 0 || declarations != 1 || jobs != 0 {
		t.Fatal("intake captured or shared job approval", declarations, jobs, err)
	}
	current := selfObservationJob(t, client)
	if current["scope"] != "owner" || !strings.HasPrefix(current["scope_id"].(string), "owner-") {
		t.Fatal("public status did not identify owner scope", current)
	}
	qualify := map[string]any{"job_id": "self-observation-capture", "expected_revision": current["revision"]}
	selfObservationCall(t, client, "maestro_apply", "maintenance.qualify", qualify, true)
	observations, err = ownerctx.ListObservations(fixture.root)
	if err != nil || len(observations) != 0 {
		t.Fatal("declined job review captured", err)
	}
	allowJob = true
	qualified := selfObservationCall(t, client, "maestro_apply", "maintenance.qualify", qualify, false)
	if qualified["qualification_state"] != "qualified" || qualified["qualification"] == nil || qualified["scope_id"] != current["scope_id"] {
		t.Fatal("new actual capture was not qualified", qualified)
	}
	if declarations != 1 || jobs != 2 {
		t.Fatal("intake/job confirmations were conflated", declarations, jobs)
	}
	canonical, err := ownerctx.GetObservation(fixture.root, intake["observation_id"].(string))
	if err != nil || canonical.State != ownerctx.ObservationCaptured || !canonical.Persisted || canonical.Revision == "" {
		t.Fatal("qualified observation was not committed/readable", err)
	}
	report := selfObservationCall(t, client, "maestro_query", "maintenance.report", map[string]any{"job_id": "self-observation-capture"}, false)
	if report["workspace_id"] != current["scope_id"] || report["finding"] != "reviewed_owner_declarations_captured_as_proposal_input" || report["native_observation"] != "not_observed" || report["counts"].(map[string]any)["observations"] != float64(1) {
		t.Fatal("public report lost actual domain owner evidence", report)
	}
	status := selfObservationCall(t, client, "maestro_query", "owner.observation.status", nil, false)
	item := status["items"].([]any)[0].(map[string]any)
	if item["id"] != intake["id"] || item["observation_state"] != "captured" || item["observation_revision"] != canonical.Revision || item["capture_pending"] != false {
		t.Fatal("intake status did not read canonical capture", item)
	}
	// The same owner is observed through a different registered workspace/client.
	other := fixture
	other.options.WorkspacePath = filepath.Join(filepath.Dir(fixture.options.WorkspacePath), "workspace-other")
	if _, err = workspace.Initialize(workspace.Options{WorkspacePath: other.options.WorkspacePath, DataRoot: filepath.Join(filepath.Dir(fixture.root), "state")}); err != nil {
		t.Fatal(err)
	}
	otherClient := other.connect(t, nil)
	seen := selfObservationJob(t, otherClient)
	if seen["scope_id"] != qualified["scope_id"] || seen["revision"] != qualified["revision"] || seen["qualification"].(map[string]any)["id"] != qualified["qualification"].(map[string]any)["id"] {
		t.Fatal("second public workspace did not share exact owner qualification", seen)
	}
	refused := selfObservationCall(t, client, "maestro_apply", "maintenance.qualify", map[string]any{"job_id": "self-observation-capture", "expected_revision": qualified["revision"]}, true)
	if refused["code"] != "qualification_required" {
		t.Fatal("replay was not refused by the actual no-new-result boundary", refused)
	}
	after, err := ownerctx.GetObservation(fixture.root, canonical.ID)
	if err != nil || after.Revision != canonical.Revision {
		t.Fatal("replay qualification duplicated or changed observation", err)
	}
	observations, err = ownerctx.ListObservations(fixture.root)
	if err != nil || len(observations) != 1 {
		t.Fatal("replay qualified or duplicated canonical source", err)
	}
	afterFacet, err := os.ReadFile(facet)
	if err != nil || string(afterFacet) != string(before) {
		t.Fatal("self capture altered active canonical SELF", err)
	}
}

// Match the existing synced-publication constructor synchronization: observe the
// real boundary without adding a shipping callback or changing its authority.
func selfObservationPublicationGuardInStack() bool {
	pcs := make([]uintptr, 48)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if strings.Contains(frame.Function, ".(*Bound).guardPublication") {
			return true
		}
		if !more {
			return false
		}
	}
}

func TestOwnerSelfObservationCancelledOwningPublication(t *testing.T) {
	for _, operation := range []string{"owner.observation.intake", "owner.observation.reject"} {
		t.Run(operation, func(t *testing.T) {
			fixture := newOwnerFixture(t, false)
			declaration := ownerctx.SelfObservationDeclaration{Signal: ownerctx.SignalExplicitInstruction, Facet: "voice", Claim: "cancelled_owner_declaration", Confidence: 1, ExpiresAt: time.Now().UTC().Add(time.Hour)}
			fields := map[string]any{"self_observation": declaration}
			if operation == "owner.observation.reject" {
				submission, err := ownerctx.NewSelfObservationDeclaration(fixture.root, declaration)
				if err != nil {
					t.Fatal(err)
				}
				review, err := ownerctx.ReviewSelfObservation(fixture.root, submission)
				if err != nil {
					t.Fatal(err)
				}
				pending, err := ownerctx.ApproveSelfObservation(context.Background(), fixture.root, review, true)
				if err != nil {
					t.Fatal(err)
				}
				fields = map[string]any{"intake_id": pending.ID, "next": "rejected"}
			}
			before := methodTreeSnapshot(t, fixture.root)
			canonical, err := ownerctx.ListObservations(fixture.root)
			if err != nil {
				t.Fatal(err)
			}
			var approved, cancelled atomic.Bool
			var mutex sync.Mutex
			var cancelRequest context.CancelFunc
			options := fixture.options
			options.LookupEnv = func(key string) (string, bool) {
				if key == "MAESTRO_HOME" && approved.Load() && selfObservationPublicationGuardInStack() && cancelled.CompareAndSwap(false, true) {
					mutex.Lock()
					cancel := cancelRequest
					mutex.Unlock()
					if cancel == nil {
						t.Error("publication boundary had no live request context")
					} else {
						cancel()
					}
				}
				return "", false
			}
			bound, err := product.Open(options)
			if err != nil {
				t.Fatal(err)
			}
			server := bound.Server()
			server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
				return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
					if method != "tools/call" {
						return next(ctx, method, request)
					}
					scoped, cancel := context.WithCancel(ctx)
					mutex.Lock()
					cancelRequest = cancel
					mutex.Unlock()
					defer func() { mutex.Lock(); cancelRequest = nil; mutex.Unlock(); cancel() }()
					return next(scoped, method, request)
				}
			})
			a, z := mcp.NewInMemoryTransports()
			session, err := server.Connect(context.Background(), a, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { session.Close() })
			client := mcp.NewClient(&mcp.Implementation{Name: "self-owner-cancelled-publication-test", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: func(_ context.Context, request *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				if !strings.Contains(request.Params.Message, declaration.Claim) {
					t.Error("trusted review omitted the complete declaration")
				}
				approved.Store(true)
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}})
			connected, err := client.Connect(context.Background(), z, &mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { connected.Close() })
			args := map[string]any{"operation": operation}
			for name, value := range fields {
				args[name] = value
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			response, err := connected.CallTool(ctx, &mcp.CallToolParams{Name: "maestro_apply", Arguments: args})
			if !approved.Load() || !cancelled.Load() {
				t.Fatalf("test did not traverse approved review then actual publication boundary: approved=%t cancelled=%t response=%v err=%v", approved.Load(), cancelled.Load(), response, err)
			}
			if err == nil && response != nil && !response.IsError {
				t.Fatal("cancelled owning publication succeeded", response)
			}
			if !reflect.DeepEqual(before, methodTreeSnapshot(t, fixture.root)) {
				t.Fatal("cancelled owning publication changed pending/canonical owner bytes or modes")
			}
			after, readErr := ownerctx.ListObservations(fixture.root)
			if readErr != nil || !reflect.DeepEqual(canonical, after) {
				t.Fatal("cancelled owning publication changed canonical observations", readErr)
			}
		})
	}
}

func TestOwnerSelfObservationLegacyUpgradeSDK(t *testing.T) {
	fixture := newOwnerFixture(t, true)
	old, _, err := ownerctx.AppendObservation(fixture.root, ownerctx.ObservationInput{SchemaVersion: 1, Signal: ownerctx.SignalExplicitCorrection, Facet: "voice", Claim: "prefer.legacy", EvidenceType: "owner_correction", SourceEvent: "interaction.completed", SourceDigest: strings.Repeat("a", 64), EpisodeID: "legacy-episode", ScopeKind: "global", ScopeID: "owner", Confidence: 1, Sensitivity: "professional", ExpiresAt: time.Now().UTC().Add(time.Hour), AuthenticatedOwner: true, Material: true, OwnerConfirmed: true})
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(fixture.root, "owner", "observations", "observations.jsonl")
	before, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(fixture.root, "owner", "observations", ".lock")
	if err := os.Remove(lock); err != nil {
		t.Fatal(err)
	}
	client := fixture.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	snapshot := methodTreeSnapshot(t, filepath.Dir(fixture.root))
	selfObservationCall(t, client, "maestro_query", "owner.observation.status", nil, true)
	selfObservationCall(t, client, "maestro_apply", "owner.observation.intake", map[string]any{"self_observation": map[string]any{"signal": "explicit_instruction", "facet": "voice", "claim": "prefer.after-upgrade", "confidence": 1, "expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)}}, true)
	if !reflect.DeepEqual(snapshot, methodTreeSnapshot(t, filepath.Dir(fixture.root))) {
		t.Fatal("legacy query/refusal repaired or changed owner state")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	// Exercise the actual existing operator command again for an initialized owner.
	fixture.cli(t, "init", nil, false)
	after, err := os.ReadFile(log)
	if err != nil || string(before) != string(after) {
		t.Fatal("explicit init changed canonical legacy bytes", err)
	}
	client = fixture.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	selfObservationCall(t, client, "maestro_query", "owner.observation.status", nil, false)
	intake := selfObservationCall(t, client, "maestro_apply", "owner.observation.intake", map[string]any{"self_observation": map[string]any{"signal": "explicit_instruction", "facet": "voice", "claim": "prefer.after-upgrade", "confidence": 1, "expires_at": time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano)}}, false)
	job := selfObservationJob(t, client)
	result := selfObservationCall(t, client, "maestro_apply", "maintenance.qualify", map[string]any{"job_id": "self-observation-capture", "expected_revision": job["revision"]}, false)
	if result["qualification_state"] != "qualified" {
		t.Fatal("legacy upgrade could not qualify new canonical capture", result)
	}
	captured, err := ownerctx.GetObservation(fixture.root, intake["observation_id"].(string))
	if err != nil || captured.State != ownerctx.ObservationCaptured {
		t.Fatal("post-upgrade canonical capture missing", err)
	}
	preserved, err := ownerctx.GetObservation(fixture.root, old.ID)
	if err != nil || preserved.Revision != old.Revision {
		t.Fatal("upgrade altered legacy observation", err)
	}
	final, err := os.ReadFile(log)
	if err != nil || !strings.HasPrefix(string(final), string(before)) {
		t.Fatal("new capture failed to preserve legacy log prefix", err)
	}
}

func selfObservationInnerValidationInStack() bool {
	pcs := make([]uintptr, 64)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	closure, validation := false, false
	for {
		frame, more := frames.Next()
		closure = closure || strings.Contains(frame.Function, ".ownerSelfObservation.func")
		validation = validation || strings.HasSuffix(frame.Function, ".(*Bound).validate")
		if !more {
			break
		}
	}
	return closure && validation
}

func TestOwnerSelfObservationLateCancelledOwningPublication(t *testing.T) {
	for _, next := range []string{"", "rejected", "redacted"} {
		operation := "owner.observation.intake"
		if next != "" {
			operation = "owner.observation.reject"
		}
		t.Run(operation+"/"+next, func(t *testing.T) {
			fixture := newOwnerFixture(t, false)
			declaration := ownerctx.SelfObservationDeclaration{Signal: ownerctx.SignalExplicitInstruction, Facet: "voice", Claim: "cancelled_owner_declaration", Confidence: 1, ExpiresAt: time.Now().UTC().Add(time.Hour)}
			fields := map[string]any{"self_observation": declaration}
			if operation == "owner.observation.reject" {
				submission, err := ownerctx.NewSelfObservationDeclaration(fixture.root, declaration)
				if err != nil {
					t.Fatal(err)
				}
				review, err := ownerctx.ReviewSelfObservation(fixture.root, submission)
				if err != nil {
					t.Fatal(err)
				}
				pending, err := ownerctx.ApproveSelfObservation(context.Background(), fixture.root, review, true)
				if err != nil {
					t.Fatal(err)
				}
				source, err := ownerctx.InspectSelfObservationSource(fixture.root, time.Now().UTC())
				if err != nil {
					t.Fatal(err)
				}
				if _, err = ownerctx.CaptureSelfObservation(context.Background(), fixture.root, pending.ID, source.Fingerprint, func(write func() error) error { return write() }); err != nil {
					t.Fatal(err)
				}
				fields = map[string]any{"intake_id": pending.ID, "next": next}
			}
			before := methodTreeSnapshot(t, fixture.root)
			canonical, err := ownerctx.ListObservations(fixture.root)
			if err != nil {
				t.Fatal(err)
			}
			var approved, cancelled atomic.Bool
			var mutex sync.Mutex
			var cancelRequest context.CancelFunc
			options := fixture.options
			options.LookupEnv = func(key string) (string, bool) {
				if key == "MAESTRO_HOME" && approved.Load() && selfObservationInnerValidationInStack() && cancelled.CompareAndSwap(false, true) {
					mutex.Lock()
					cancel := cancelRequest
					mutex.Unlock()
					if cancel == nil {
						t.Error("publication boundary had no live request context")
					} else {
						cancel()
					}
				}
				return "", false
			}
			bound, err := product.Open(options)
			if err != nil {
				t.Fatal(err)
			}
			server := bound.Server()
			server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
				return func(ctx context.Context, method string, request mcp.Request) (mcp.Result, error) {
					if method != "tools/call" {
						return next(ctx, method, request)
					}
					scoped, cancel := context.WithCancel(ctx)
					mutex.Lock()
					cancelRequest = cancel
					mutex.Unlock()
					defer func() { mutex.Lock(); cancelRequest = nil; mutex.Unlock(); cancel() }()
					return next(scoped, method, request)
				}
			})
			a, z := mcp.NewInMemoryTransports()
			session, err := server.Connect(context.Background(), a, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { session.Close() })
			client := mcp.NewClient(&mcp.Implementation{Name: "self-owner-cancelled-publication-test", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: func(_ context.Context, request *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				if !strings.Contains(request.Params.Message, declaration.Claim) {
					t.Error("trusted review omitted the complete declaration")
				}
				approved.Store(true)
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}})
			connected, err := client.Connect(context.Background(), z, &mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { connected.Close() })
			args := map[string]any{"operation": operation}
			for name, value := range fields {
				args[name] = value
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			response, err := connected.CallTool(ctx, &mcp.CallToolParams{Name: "maestro_apply", Arguments: args})
			if !approved.Load() || !cancelled.Load() {
				t.Fatalf("test did not traverse approved review then actual publication boundary: approved=%t cancelled=%t response=%v err=%v", approved.Load(), cancelled.Load(), response, err)
			}
			if err == nil && response != nil && !response.IsError {
				t.Fatal("cancelled owning publication succeeded", response)
			}
			if !reflect.DeepEqual(before, methodTreeSnapshot(t, fixture.root)) {
				t.Fatal("cancelled owning publication changed pending/canonical owner bytes or modes")
			}
			after, readErr := ownerctx.ListObservations(fixture.root)
			if readErr != nil || !reflect.DeepEqual(canonical, after) {
				t.Fatal("cancelled owning publication changed canonical observations", readErr)
			}
		})
	}
}
