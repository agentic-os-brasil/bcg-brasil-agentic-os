package activationpolicy

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBalancedPolicyRoutesDeterministically(t *testing.T) {
	registry := []PXpert{
		{ID: "pxpert-fpa-pricing", Kind: ExpertFPA, Version: "1.2.0", CanonSHA256: digest("pricing"), Lifecycle: Published},
		{ID: "pxpert-ipa-insurance", Kind: ExpertIPA, Version: "2.0.1", CanonSHA256: digest("insurance"), Lifecycle: Published},
	}
	envelope := IntentEnvelope{
		SchemaVersion: 1, EpisodeID: "episode-01", Owner: OwnerCase,
		Posture: Balanced, Consequence: Medium, Reversibility: Reversible,
		Sensitivity: Internal, KnowledgeNeed: Functional,
	}
	first, err := Plan(envelope, registry)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Plan(envelope, append([]PXpert(nil), registry...))
	if err != nil {
		t.Fatal(err)
	}
	if first.Route != D1Targeted || len(first.Experts) != 1 || first.Experts[0].ID != "pxpert-fpa-pricing" {
		t.Fatalf("unexpected route: %#v", first)
	}
	if first.PlanSHA256 != second.PlanSHA256 || first.InputSHA256 != second.InputSHA256 {
		t.Fatal("same normalized input did not produce the same plan")
	}
	if !first.Shadow || first.PolicyVersion != PolicyVersion {
		t.Fatalf("shadow policy metadata missing: %#v", first)
	}
}

func TestOmittedPostureDefaultsToBalanced(t *testing.T) {
	plan, err := Plan(IntentEnvelope{
		SchemaVersion: 1, EpisodeID: "episode-default", Owner: OwnerCase,
		Consequence: Medium, Reversibility: Reversible,
		Sensitivity: Internal, KnowledgeNeed: None,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Posture != Balanced || plan.Route != D1Targeted ||
		!plan.RequiresAssurance || plan.AssuranceAgentID != "walter" {
		t.Fatalf("default posture is not the balanced governed default: %#v", plan)
	}
}

func TestSemanticProposalCannotReduceHardRoute(t *testing.T) {
	plan, err := Plan(IntentEnvelope{
		SchemaVersion: 1, EpisodeID: "episode-02", Owner: OwnerAccount,
		Posture: Direct, Consequence: High, Reversibility: Reversible,
		Sensitivity: Confidential, KnowledgeNeed: None, ExternalEffect: true,
		PlannerProposal: PlannerProposal{RequestedRoute: D0Direct},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Route != D2Governed || !plan.RequiresAssurance {
		t.Fatalf("planner reduced hard route: %#v", plan)
	}
}

func TestRequiredExpertFailsClosedWhenUnavailable(t *testing.T) {
	_, err := Plan(IntentEnvelope{
		SchemaVersion: 1, EpisodeID: "episode-03", Owner: OwnerCase,
		Posture: Balanced, Consequence: Low, Reversibility: Reversible,
		Sensitivity: Internal, KnowledgeNeed: Industry,
	}, []PXpert{{ID: "pxpert-ipa-retail", Kind: ExpertIPA, Version: "1.0.0", CanonSHA256: digest("retail"), Lifecycle: Draft}})
	if err == nil {
		t.Fatal("unavailable required PXpert was silently bypassed")
	}
}

func TestUnknownEnvelopeFieldFailsClosed(t *testing.T) {
	var envelope IntentEnvelope
	if err := DecodeStrict([]byte(`{"schema_version":1,"episode_id":"episode-04","owner":"case_agent","posture":"balanced","consequence":"low","reversibility":"reversible","sensitivity":"internal","knowledge_need":"none","invented_authority":true}`), &envelope); err == nil {
		t.Fatal("unknown authority-bearing field was accepted")
	}
}

func TestPlanJSONDoesNotContainNarrativeTaskContent(t *testing.T) {
	plan, err := Plan(IntentEnvelope{
		SchemaVersion: 1, EpisodeID: "episode-05", Owner: OwnerCase,
		Posture: Balanced, Consequence: Low, Reversibility: Reversible,
		Sensitivity: Public, KnowledgeNeed: None,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(plan)
	for _, forbidden := range []string{"prompt", "objective", "task_text"} {
		if json.Valid(body) && strings.Contains(string(body), forbidden) {
			t.Fatalf("plan leaked narrative field %q: %s", forbidden, body)
		}
	}
}

func TestPostureCalibrationMatrixKeepsHardFloors(t *testing.T) {
	expert := PXpert{
		ID: "pxpert-fpa-pricing", Kind: ExpertFPA, Version: "1.0.0",
		CanonSHA256: digest("canon"), Lifecycle: Published,
	}
	base := IntentEnvelope{
		SchemaVersion: 1, EpisodeID: "episode-matrix", Owner: OwnerCase,
		Consequence: Medium, Reversibility: Reversible,
		Sensitivity: Internal, KnowledgeNeed: None,
	}
	tests := []struct {
		name    string
		posture Posture
		mutate  func(*IntentEnvelope)
		want    Route
	}{
		{"direct collapses safe medium work", Direct, func(*IntentEnvelope) {}, D0Direct},
		{"balanced requests targeted review", Balanced, func(*IntentEnvelope) {}, D1Targeted},
		{"deliberative requests governed review", Deliberative, func(*IntentEnvelope) {}, D2Governed},
		{"direct cannot bypass knowledge need", Direct, func(value *IntentEnvelope) { value.KnowledgeNeed = Functional }, D1Targeted},
		{"direct cannot bypass external effect", Direct, func(value *IntentEnvelope) { value.ExternalEffect = true }, D2Governed},
		{"privileged action blocks every posture", Deliberative, func(value *IntentEnvelope) { value.PrivilegedAction = true }, Blocked},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := base
			input.Posture = test.posture
			test.mutate(&input)
			plan, err := Plan(input, []PXpert{expert})
			if err != nil {
				t.Fatal(err)
			}
			if plan.Route != test.want {
				t.Fatalf("route = %s, want %s", plan.Route, test.want)
			}
			if plan.Route != D0Direct && plan.Route != Blocked &&
				len(plan.Experts) == 0 && !plan.RequiresAssurance {
				t.Fatalf("non-direct route lacks mandatory participant: %#v", plan)
			}
		})
	}
}

func digest(value string) string {
	return SHA256Hex([]byte(value))
}
