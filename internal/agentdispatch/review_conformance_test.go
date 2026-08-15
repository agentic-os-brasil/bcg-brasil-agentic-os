package agentdispatch

import (
	"encoding/json"
	"os"
	"testing"
)

func TestYodaReviewConformanceFixtureMatchesSharedContract(t *testing.T) {
	body, err := os.ReadFile("../../adapters/conformance/yoda-review.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		SchemaVersion int      `json:"schema_version"`
		MaxObjections int      `json:"max_objections"`
		Triggers      []string `json:"triggers"`
		Verdicts      []string `json:"verdicts"`
		ReceiptFields []string `json:"receipt_fields"`
	}
	if err := json.Unmarshal(body, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.SchemaVersion != 1 || fixture.MaxObjections != maxYodaObjections {
		t.Fatalf("Yoda fixture header drifted: %#v", fixture)
	}
	expectedTriggers := []YodaReviewTrigger{ReviewMaterialRecommendation, ReviewConsequentialTradeoff, ReviewExternalArtifact}
	if len(fixture.Triggers) != len(expectedTriggers) {
		t.Fatalf("fixture trigger set drifted: %#v", fixture.Triggers)
	}
	for index, trigger := range fixture.Triggers {
		if trigger != string(expectedTriggers[index]) {
			t.Fatalf("fixture trigger order/set drifted: %#v", fixture.Triggers)
		}
		if !RequiresYodaReview(YodaReviewTrigger(trigger)) {
			t.Fatalf("fixture trigger is not executable: %q", trigger)
		}
	}
	expectedVerdicts := []YodaVerdict{YodaApproved, YodaRefineAndReturn, YodaMissingTheMark, YodaHold}
	if len(fixture.Verdicts) != len(expectedVerdicts) {
		t.Fatalf("fixture verdict set drifted: %#v", fixture.Verdicts)
	}
	for index, verdict := range fixture.Verdicts {
		if verdict != string(expectedVerdicts[index]) {
			t.Fatalf("fixture verdict order/set drifted: %#v", fixture.Verdicts)
		}
		body := YodaReviewBody{Verdict: YodaVerdict(verdict), PreservesIntent: true}
		if verdict == string(YodaRefineAndReturn) || verdict == string(YodaMissingTheMark) || verdict == string(YodaHold) {
			body.Objections = []YodaObjection{{Code: "fixture", Fix: "Apply the named correction.", ProposedRefinement: "Preserve the thesis while applying the concrete correction.", ExitCondition: "The correction is evidenced.", Blocking: verdict == string(YodaHold)}}
		}
		if err := validateYodaReviewBody(body, ReviewPacket{
			SourcePacketID:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SourcePacketSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			SourceScopeKind:    "workspace", SourceScopeID: "alpha",
			Trigger: ReviewMaterialRecommendation, Audience: "sponsor",
			Recommendation: "Choose the bounded option.", DefinitionOfDone: "The sponsor can decide.",
		}); err != nil {
			t.Fatalf("fixture verdict %q is not executable: %v", verdict, err)
		}
	}
	expectedReceiptFields := []string{"trigger", "leverage_decision", "posture", "state", "source_packet_id", "source_packet_sha256", "objection_count", "preserves_intent"}
	if len(fixture.ReceiptFields) != len(expectedReceiptFields) {
		t.Fatalf("fixture receipt projection drifted: %#v", fixture.ReceiptFields)
	}
	for index, field := range fixture.ReceiptFields {
		if field != expectedReceiptFields[index] {
			t.Fatalf("fixture receipt projection drifted: %#v", fixture.ReceiptFields)
		}
	}
}

func TestYodaReviewIsConstructiveAndProportional(t *testing.T) {
	review := ReviewPacket{SourcePacketID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SourcePacketSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SourceScopeKind: "workspace", SourceScopeID: "alpha", Trigger: ReviewMaterialRecommendation, Audience: "sponsor", Recommendation: "Choose the bounded option.", DefinitionOfDone: "The sponsor can decide."}
	if err := validateYodaReviewBody(YodaReviewBody{Verdict: YodaApproved, PreservesIntent: true, Objections: []YodaObjection{{Code: "polish", Fix: "Tighten one sentence.", ProposedRefinement: "Keep the thesis and improve clarity.", ExitCondition: "The sentence is clearer.", Blocking: false}}}, review); err != nil {
		t.Fatalf("cosmetic polish blocked approval: %v", err)
	}
	if err := validateYodaReviewBody(YodaReviewBody{Verdict: YodaRefineAndReturn, PreservesIntent: true, Objections: []YodaObjection{{Code: "evidence_gap", Fix: "Add the missing decision evidence.", ProposedRefinement: "Retain the recommendation and add the cited evidence.", ExitCondition: "The evidence pointer supports the claim.", Blocking: true}}}, review); err != nil {
		t.Fatalf("load-bearing refinement rejected: %v", err)
	}
	if err := validateYodaReviewBody(YodaReviewBody{Verdict: YodaHold, PreservesIntent: true, Objections: []YodaObjection{{Code: "material_risk", Fix: "Resolve the material safety issue before delivery.", ExitCondition: "The safety owner confirms the mitigation.", Blocking: true}}}, review); err != nil {
		t.Fatalf("material hold rejected: %v", err)
	}
	if err := validateYodaReviewBody(YodaReviewBody{Verdict: YodaApproved, PreservesIntent: true, Objections: []YodaObjection{{Code: "cosmetic_block", Fix: "Change the color.", ExitCondition: "The color changes.", Blocking: true}}}, review); err == nil {
		t.Fatal("cosmetic blocking objection bypassed calm approval rule")
	}
}

func TestYodaReviewRequiresIntentPreservation(t *testing.T) {
	review := ReviewPacket{SourcePacketID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", SourcePacketSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", SourceScopeKind: "workspace", SourceScopeID: "alpha", Trigger: ReviewMaterialRecommendation, Audience: "sponsor", Recommendation: "Choose the bounded option.", DefinitionOfDone: "The sponsor can decide."}
	if err := validateYodaReviewBody(YodaReviewBody{Verdict: YodaApproved}, review); err == nil {
		t.Fatal("Yoda accepted a verdict without an explicit intent-preservation assertion")
	}
}
