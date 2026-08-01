package maestro

import (
	"encoding/json"
	"os"
	"testing"
)

func TestMaestroAdapterQualityLoopFixtureCoversFourPathsAndDirectAnswer(t *testing.T) {
	body, err := os.ReadFile("../../adapters/conformance/maestro-quality-loop.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion int `json:"schema_version"`
		Paths         []struct {
			Name                 string      `json:"name"`
			IntentClass          IntentClass `json:"intent_class"`
			ExecutionOnly        bool        `json:"execution_only"`
			StrategicImplication bool        `json:"strategic_implication"`
			Materiality          Materiality `json:"materiality"`
			CaseEntry            CaseEntry   `json:"case_entry"`
			AccountValidation    bool        `json:"account_validation"`
			WalterRequired       bool        `json:"walter_required"`
		} `json:"paths"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != 1 || len(fixture.Paths) != 5 {
		t.Fatalf("quality-loop fixture drifted: %#v", fixture)
	}
	for _, path := range fixture.Paths {
		input := caseInput(path.ExecutionOnly)
		if path.IntentClass != "" {
			input.IntentClass = path.IntentClass
		}
		input.ExecutionOnly = path.ExecutionOnly
		input.StrategicImplication = path.StrategicImplication
		if path.Materiality != "" {
			input.Materiality = path.Materiality
		}
		plan, err := PlanFor(input)
		if err != nil {
			t.Fatalf("%s: %v", path.Name, err)
		}
		if string(plan.CaseEntry) != string(path.CaseEntry) || plan.RequiresAccountValidation != path.AccountValidation || plan.RequiresWalter != path.WalterRequired {
			t.Fatalf("%s: plan=%#v", path.Name, plan)
		}
	}
}

func TestWalterIntentReviewFixturePinsSelfAndIntentContract(t *testing.T) {
	body, err := os.ReadFile("../../adapters/conformance/walter-intent-review.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion int      `json:"schema_version"`
		PacketVersion string   `json:"packet_version"`
		Role          string   `json:"role"`
		Bindings      []string `json:"required_packet_bindings"`
		Results       []string `json:"result_fields"`
		Verdicts      []string `json:"verdicts"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != 1 || fixture.PacketVersion != "intent-review-v1" || fixture.Role != "senior_advisor_refiner" {
		t.Fatalf("Walter intent fixture header drifted: %#v", fixture)
	}
	for _, required := range []string{"self_snapshot_version", "self_snapshot_digest", "draft_output", "observations"} {
		if !contains(fixture.Bindings, required) {
			t.Fatalf("Walter intent packet missing %q: %#v", required, fixture.Bindings)
		}
	}
	for _, required := range []string{"intrinsic_intent_hypothesis", "confidence", "constructive_refinement", "unresolved_uncertainty"} {
		if !contains(fixture.Results, required) {
			t.Fatalf("Walter intent result missing %q: %#v", required, fixture.Results)
		}
	}
	for _, verdict := range []IntentVerdict{IntentApprove, IntentRefine, IntentClarify, IntentHold} {
		if !contains(fixture.Verdicts, string(verdict)) {
			t.Fatalf("Walter intent verdict missing %q: %#v", verdict, fixture.Verdicts)
		}
	}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
