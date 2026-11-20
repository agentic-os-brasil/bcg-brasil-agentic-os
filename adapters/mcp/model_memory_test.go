package product_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This controlled source test exercises the real ordinary MCP review/promotion
// route. It supplies no model/native/qualification/enrollment/calendar proof.
func TestMemoryWeeklyReusesExactPendingModelCandidate(t *testing.T) {
	f := newOwnerFixture(t, false)
	prepareMemoryFixture(t, f)
	captureEngine, workspace := fixtureMemoryEngine(t, f)
	e, err := maintenance.NewProductWeeklyMemoryEngine(filepath.Dir(filepath.Dir(captureEngine.Root)))
	if err != nil || e.Root != captureEngine.Root {
		t.Fatal("original weekly engine fixture binding", err)
	}
	e.SynthesizerID = "controlled-model-source-fixture-v1"
	e.EligibilityPolicyID = "deterministic-two-l3-generations+exact-owner-review-v1"
	e.Eligibility = memory.EligibilityFunc(func(ctx context.Context, candidate memory.Artifact) (bool, string, error) {
		eligible, reason, err := (memory.DeterministicLifetimeEligibility{MinL3Generations: 2}).Evaluate(ctx, candidate)
		if err != nil || !eligible {
			return false, reason, err
		}
		return false, "trusted exact-candidate confirmation unavailable", nil
	})
	calls := 0
	cs := f.connect(t, func(_ context.Context, request *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		calls++
		if !strings.Contains(request.Params.Message, "EXACT_STORED_MODEL_CANDIDATE") {
			t.Fatal("normal review lost exact stored candidate")
		}
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	for i, date := range []string{"2026-10-12", "2026-10-13"} {
		seedReviewedMemory(t, e, workspace, "eligible synthetic model source "+date, date)
		memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": date}, false)
		at, err := time.Parse("2006-01-02", date)
		if err != nil {
			t.Fatal(err)
		}
		input, err := e.InspectModelWeeklyInput(workspace, at)
		if err != nil {
			t.Fatal(err)
		}
		text := "first pending candidate"
		if i == 1 {
			text = "EXACT_STORED_MODEL_CANDIDATE"
		}
		candidate := memory.ModelWeeklyCandidate{SchemaVersion: 1, JobID: memory.ModelWeeklyJobID, WorkspaceID: workspace, Period: input.Period, InputSHA256: input.InputSHA256, Content: map[string]string{"L2": "bounded weekly summary " + date, "L3": "bounded continuity " + date, "lifetime": text}}
		if _, err = e.CommitModelWeekly(context.Background(), input, candidate, at); err != nil {
			t.Fatal(err)
		}
	}
	report := memoryCall(t, cs, "maintenance.report", map[string]any{"job_id": memory.ModelWeeklyJobID}, false)
	reportJSON, _ := json.Marshal(report)
	if !strings.Contains(string(reportJSON), "no_report") || strings.Contains(string(reportJSON), "weekly_native_effect_record_verified") {
		t.Fatal("unqualified source storage became native model proof", string(reportJSON))
	}
	l2, _, err := e.ActiveArtifact(workspace, "L2/2026-W42")
	if err != nil {
		t.Fatal(err)
	}
	l3, _, err := e.ActiveArtifact(workspace, "L3/current")
	if err != nil {
		t.Fatal(err)
	}
	before2, _ := json.Marshal(l2)
	before3, _ := json.Marshal(l3)
	out := memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "weekly", "date": "2026-10-13"}, false)
	body, _ := json.Marshal(out)
	if calls != 1 || !strings.Contains(string(body), "stored_model_candidate_no_model_call") {
		t.Fatalf("ordinary path did not reuse pending candidate: calls=%d %s", calls, body)
	}
	for _, item := range []struct {
		key    string
		before []byte
	}{{"L2/2026-W42", before2}, {"L3/current", before3}} {
		after, _, err := e.ActiveArtifact(workspace, item.key)
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(after)
		if string(encoded) != string(item.before) {
			t.Fatal("pending promotion manufactured new L2/L3 output/history")
		}
	}
	lifetime, _, err := e.ActiveArtifact(workspace, "lifetime/current")
	if err != nil || lifetime.Content != "EXACT_STORED_MODEL_CANDIDATE" {
		t.Fatal("exact candidate was not promoted by original MCP review", err)
	}
}
