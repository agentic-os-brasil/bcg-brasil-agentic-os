package agentdispatch

import (
	"strings"
	"testing"
	"time"
)

func testIntentPacket(audience, prompt, output string) IntentReviewPacket {
	return NewIntentReviewPacket(prompt, "maestro -> case -> maestro -> walter", output, audience, IntentConsequenceHigh, IntentReversible, UserSelfSnapshotRef{
		Version: 1, Digest: strings.Repeat("1", 64), Scope: "local_user",
		FacetDigests: map[string]string{"communication-style": strings.Repeat("2", 64)},
	}, []SelfObservationRef{{ObservationID: "obs-1", Signal: "explicit_instruction", SourceEventSHA256: strings.Repeat("3", 64), ClaimSHA256: strings.Repeat("4", 64), OccurredAt: time.Unix(1, 0).UTC(), ScopeKind: "global", ScopeID: "local_user", Confidence: "high", Sensitivity: "professional", EvidenceType: "owner_instruction"}}, []string{"bcgos://workspace/alpha/dossier/evidence.md"})
}

func testIntentBody(review ReviewPacket, verdict WalterVerdict) WalterReviewBody {
	intentVerdict := IntentApprove
	if verdict == WalterRefineAndReturn || verdict == WalterMissingTheMark {
		intentVerdict = IntentRefine
	}
	if verdict == WalterHold {
		intentVerdict = IntentHoldExceptional
	}
	body := WalterReviewBody{Verdict: verdict, Intent: IntentReviewBody{
		IntentPacketSHA256: IntentPacketDigest(review.Intent),
		LiteralRequest:     review.Intent.LiteralPrompt, IntrinsicIntentHypothesis: "Deliver a defensible result aligned to the stated decision.",
		EvidenceRefs: []string{"bcgos://workspace/alpha/dossier/evidence.md"}, Confidence: "high", PurposeSatisfied: PurposeYes,
		Verdict: intentVerdict,
	}}
	if intentVerdict == IntentRefine {
		body.Intent.ConstructiveRefinement = "Address the load-bearing evidence gap before delivery."
		body.Intent.PurposeSatisfied = PurposePartial
	}
	if intentVerdict == IntentHoldExceptional {
		body.Intent.UnresolvedUncertainty = "A material evidence gap remains unresolved."
		body.Intent.PurposeSatisfied = PurposeUnknown
	}
	return body
}

func testIntentBodyForPacket(t *testing.T, packet *ReviewPacket, verdict WalterVerdict) WalterReviewBody {
	t.Helper()
	if packet == nil {
		t.Fatal("review packet is required")
	}
	return testIntentBody(*packet, verdict)
}
