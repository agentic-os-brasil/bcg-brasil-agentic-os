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
		SelfSignal struct {
			Ordinary struct {
				Evaluated        bool `json:"evaluated"`
				Persisted        bool `json:"persisted"`
				ObservationCount int  `json:"observation_count"`
			} `json:"ordinary_walter_activity"`
			Valid struct {
				Signal         string  `json:"signal"`
				Facet          string  `json:"facet"`
				Claim          string  `json:"claim"`
				EvidenceType   string  `json:"evidence_type"`
				Confidence     float64 `json:"confidence"`
				Sensitivity    string  `json:"sensitivity"`
				OwnerConfirmed bool    `json:"owner_confirmed"`
			} `json:"valid_explicit_correction"`
			Rejections []string `json:"rejections"`
			Global     string   `json:"global_promotion_requires"`
		} `json:"self_signal_contract"`
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
	if !fixture.SelfSignal.Ordinary.Evaluated || fixture.SelfSignal.Ordinary.Persisted || fixture.SelfSignal.Ordinary.ObservationCount != 0 {
		t.Fatalf("ordinary activity self-signal contract drifted: %#v", fixture.SelfSignal.Ordinary)
	}
	valid := fixture.SelfSignal.Valid
	if valid.Signal != "explicit_correction" || valid.Facet != "communication-style" || valid.Claim != "prefers_concise" || valid.EvidenceType != "owner_correction" || valid.Confidence != 1 || valid.Sensitivity != "professional" || !valid.OwnerConfirmed {
		t.Fatalf("valid explicit self-signal contract drifted: %#v", valid)
	}
	for _, rejection := range []string{"unknown_self_signal_field", "unsupported_signal", "empty_or_unknown_facet", "generic_ok_endorsement", "generated_or_client_evidence"} {
		if !contains(fixture.SelfSignal.Rejections, rejection) {
			t.Fatalf("self-signal rejection missing %q: %#v", rejection, fixture.SelfSignal.Rejections)
		}
	}
	if fixture.SelfSignal.Global != "owner_declassification_and_canonical_cas" {
		t.Fatalf("global self promotion contract drifted: %q", fixture.SelfSignal.Global)
	}
}

func TestWalterIntentReviewFixturePinsSelfAndIntentContract(t *testing.T) {
	body, err := os.ReadFile("../../adapters/conformance/walter-intent-review.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion int      `json:"schema_version"`
		Identity      string   `json:"identity"`
		PacketVersion string   `json:"packet_version"`
		Role          string   `json:"role"`
		Bindings      []string `json:"required_packet_bindings"`
		Results       []string `json:"result_fields"`
		Verdicts      []string `json:"verdicts"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != 1 || fixture.Identity != "owner_self_proxy_inside_maestro_loop" || fixture.PacketVersion != "intent-review-v1" || fixture.Role != "senior_advisor_refiner" {
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
