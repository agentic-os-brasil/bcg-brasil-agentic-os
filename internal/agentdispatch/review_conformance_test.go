package agentdispatch

import (
	"encoding/json"
	"os"
	"testing"
)

func TestWalterReviewConformanceFixtureMatchesSharedContract(t *testing.T) {
	body, err := os.ReadFile("../../adapters/conformance/walter-review.json")
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
	if fixture.SchemaVersion != 1 || fixture.MaxObjections != maxWalterObjections {
		t.Fatalf("Walter fixture header drifted: %#v", fixture)
	}
	expectedTriggers := []WalterReviewTrigger{ReviewMaterialRecommendation, ReviewConsequentialTradeoff, ReviewExternalArtifact}
	if len(fixture.Triggers) != len(expectedTriggers) {
		t.Fatalf("fixture trigger set drifted: %#v", fixture.Triggers)
	}
	for index, trigger := range fixture.Triggers {
		if trigger != string(expectedTriggers[index]) {
			t.Fatalf("fixture trigger order/set drifted: %#v", fixture.Triggers)
		}
		if !RequiresWalterReview(WalterReviewTrigger(trigger)) {
			t.Fatalf("fixture trigger is not executable: %q", trigger)
		}
	}
	expectedVerdicts := []WalterVerdict{WalterApproved, WalterRefineAndReturn, WalterMissingTheMark, WalterHold}
	if len(fixture.Verdicts) != len(expectedVerdicts) {
		t.Fatalf("fixture verdict set drifted: %#v", fixture.Verdicts)
	}
	for index, verdict := range fixture.Verdicts {
		if verdict != string(expectedVerdicts[index]) {
			t.Fatalf("fixture verdict order/set drifted: %#v", fixture.Verdicts)
		}
		body := WalterReviewBody{Verdict: WalterVerdict(verdict)}
		if verdict == string(WalterRefineAndReturn) || verdict == string(WalterMissingTheMark) || verdict == string(WalterHold) {
			body.Objections = []WalterObjection{{Code: "fixture", Fix: "Apply the named correction.", ExitCondition: "The correction is evidenced.", Blocking: true}}
		}
		if err := validateWalterReviewBody(body, ReviewPacket{
			SourcePacketID:     "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			SourcePacketSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
			SourceScopeKind:    "workspace", SourceScopeID: "alpha",
			Trigger: ReviewMaterialRecommendation, Audience: "sponsor",
			Recommendation: "Choose the bounded option.", DefinitionOfDone: "The sponsor can decide.",
		}); err != nil {
			t.Fatalf("fixture verdict %q is not executable: %v", verdict, err)
		}
	}
	expectedReceiptFields := []string{"trigger", "state", "source_packet_id", "source_packet_sha256", "chain_mode", "chain_sha256", "account_consultation_required", "post_account_validation_required", "walter_required", "validated_packet_id", "validated_packet_sha256", "direct_case_reason_code", "objection_count"}
	if len(fixture.ReceiptFields) != len(expectedReceiptFields) {
		t.Fatalf("fixture receipt projection drifted: %#v", fixture.ReceiptFields)
	}
	for index, field := range fixture.ReceiptFields {
		if field != expectedReceiptFields[index] {
			t.Fatalf("fixture receipt projection drifted: %#v", fixture.ReceiptFields)
		}
	}
}
