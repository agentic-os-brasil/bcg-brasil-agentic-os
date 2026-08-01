package agentdispatch

import (
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/selfmodel"
)

func TestMaestroInteractionRecorderRunsWithoutWalterAndKeepsHypothesesProvisional(t *testing.T) {
	recorder := InteractionRecorder{Store: selfmodel.Store{Root: t.TempDir()}}
	observation, err := recorder.AfterInteraction(selfmodel.InteractionInput{
		ObservationID: "obs-loop-1", Signal: selfmodel.InferredHypothesis, SourceEvent: "owner-result",
		SourceEventSHA256: selfmodel.Digest("event"), OccurredAt: time.Now().UTC(), ScopeKind: "global", ScopeID: selfmodel.OwnerScope,
		ClaimSHA256: selfmodel.Digest("minimized-claim"), EvidenceType: "owner_feedback", ProvenanceSHA256: selfmodel.Digest("provenance"),
		Confidence: selfmodel.ConfidenceLow, Sensitivity: "professional", Materiality: selfmodel.MaterialityHigh, OwnerAuthenticated: true,
	})
	if err != nil || observation.Lifecycle != selfmodel.Captured {
		t.Fatalf("interaction observation was not captured independently of Walter: %+v err=%v", observation, err)
	}
	observations, err := recorder.Store.List()
	if err != nil || len(observations) != 1 || observations[0].Signal != selfmodel.InferredHypothesis {
		t.Fatalf("hypothesis was not kept as a provisional observation: %+v err=%v", observations, err)
	}
}

func TestIntentReviewSeparatesLiteralRequestFromEphemeralPurposeHypothesis(t *testing.T) {
	packet := testIntentPacket("sponsor", "Review the recommendation.", "Preserve the defensible thesis.")
	body := testIntentBody(ReviewPacket{Intent: packet}, WalterApproved)
	if err := ValidateIntentReviewBody(body.Intent, packet); err != nil {
		t.Fatal(err)
	}
	if body.Intent.LiteralRequest != packet.LiteralPrompt || body.Intent.IntrinsicIntentHypothesis == body.Intent.LiteralRequest {
		t.Fatal("intent review did not keep literal request and hypothesis distinct")
	}
	packet.UserSelfSnapshot.Digest = strings.Repeat("9", 64)
	if IntentPacketDigest(packet) == body.Intent.IntentPacketSHA256 {
		t.Fatal("self snapshot mutation did not invalidate the intent packet digest")
	}
}

func TestLowConfidenceHighConsequenceReturnsClarifyInsteadOfBlockingOrInventing(t *testing.T) {
	packet := testIntentPacket("executive", "Review the high-consequence recommendation.", "Preserve the thesis.")
	body := testIntentBody(ReviewPacket{Intent: packet}, WalterApproved)
	body.Intent.Confidence = selfmodel.ConfidenceLow
	if err := ValidateIntentReviewBody(body.Intent, packet); err == nil {
		t.Fatal("low-confidence high-consequence approval was accepted")
	}
	body.Intent.Verdict = IntentClarify
	body.Intent.UnresolvedUncertainty = "The intended decision audience is not sufficiently evidenced."
	if err := ValidateIntentReviewBody(body.Intent, packet); err != nil {
		t.Fatalf("clarify should return uncertainty to Maestro: %v", err)
	}
}

func TestIntentRefinementMustBeConstructiveAndBoundToCurrentPacket(t *testing.T) {
	packet := testIntentPacket("sponsor", "Review the trade-off.", "Choose option A.")
	body := testIntentBody(ReviewPacket{Intent: packet}, WalterRefineAndReturn)
	body.Intent.ConstructiveRefinement = ""
	if err := ValidateIntentReviewBody(body.Intent, packet); err == nil {
		t.Fatal("empty intent refinement was accepted")
	}
	body = testIntentBody(ReviewPacket{Intent: packet}, WalterRefineAndReturn)
	body.Intent.IntentPacketSHA256 = strings.Repeat("a", 64)
	if err := ValidateIntentReviewBody(body.Intent, packet); err == nil {
		t.Fatal("cross-packet intent verdict was accepted")
	}
}

func TestIntentReceiptProjectionContainsDigestsButNotSelfOrPromptBodies(t *testing.T) {
	packet := testIntentPacket("sponsor", "Review a private prompt.", "Keep the user thesis.")
	review := ReviewPacket{
		SourcePacketID: strings.Repeat("a", 64), SourcePacketSHA256: strings.Repeat("b", 64),
		SourceScopeKind: "workspace", SourceScopeID: "alpha", Trigger: ReviewMaterialRecommendation,
		Audience: "sponsor", Recommendation: "Keep the user thesis.", DefinitionOfDone: "The sponsor can decide.",
		Intent: packet, Chain: directCaseReviewChain(Receipt{DelegationID: strings.Repeat("a", 64), TargetAgentID: "case-alpha", PacketSHA256: strings.Repeat("b", 64)}),
	}
	summary := reviewSummary(&review, ReviewDispatched)
	if summary.IntentPacketSHA256 == "" || summary.SelfSnapshotSHA256 == "" || summary.PromptSHA256 == "" || summary.OutputSHA256 == "" {
		t.Fatal("review receipt omitted intent/self digest bindings")
	}
	if summary.SelfSnapshotVersion != packet.UserSelfSnapshot.Version {
		t.Fatal("review receipt omitted self snapshot version")
	}
}
