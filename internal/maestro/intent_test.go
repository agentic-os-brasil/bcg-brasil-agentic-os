package maestro

import (
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

func TestIntentReviewPacketAndReceiptAreBoundToProjectionAndDigests(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ownerctx.ProjectSnapshot(root, []string{"voice", "communication-style"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanFor(caseInput(true))
	if err != nil {
		t.Fatal(err)
	}
	packet, err := BuildIntentReviewPacket("prepare the client-ready answer", plan, "A bounded answer.", []string{"case:summary"}, snapshot, nil, "client sponsor", "medium", "reversible", "")
	if err != nil {
		t.Fatal(err)
	}
	result := IntentReviewResult{
		LiteralRequest:            packet.LiteralRequest,
		IntrinsicIntentHypothesis: "make the next decision easier without expanding scope",
		EvidenceRefs:              []string{"case:summary"},
		Confidence:                0.85,
		PurposeSatisfied:          PurposeSatisfied,
		Verdict:                   IntentApprove,
		Hypothesis: IntentHypothesis{
			ExpressedObjective: "prepare the client-ready answer", LatentIntentHypothesis: "make the next decision easier",
			EvidenceRefs: []string{"case:summary"}, Confidence: 0.85, Materiality: "medium", DisconfirmationCondition: "the sponsor cannot act from the answer",
		},
	}
	receipt, err := NewIntentReviewReceipt(packet, result, time.Unix(10, 0))
	if err != nil {
		t.Fatal(err)
	}
	if receipt.SelfSnapshotVersion != snapshot.Version || receipt.SelfSnapshotDigest != snapshot.CanonicalSourceDigest || receipt.PromptDigest == receipt.OutputDigest || receipt.Verdict != IntentApprove {
		t.Fatalf("receipt did not pin review inputs: %#v", receipt)
	}
	packet.DraftOutput = "mutated"
	if err := packet.Validate(); err == nil {
		t.Fatal("mutated packet passed digest validation")
	}
}

func TestIntentReviewLowConfidenceHighConsequenceReturnsToMaestro(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ownerctx.ProjectSnapshot(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanFor(caseInput(true))
	if err != nil {
		t.Fatal(err)
	}
	packet, err := BuildIntentReviewPacket("make a consequential recommendation", plan, "Recommendation.", nil, snapshot, nil, "executive", "high", "hard_to_reverse", "")
	if err != nil {
		t.Fatal(err)
	}
	result := IntentReviewResult{
		LiteralRequest:            packet.LiteralRequest,
		IntrinsicIntentHypothesis: "unknown",
		Confidence:                0.2,
		PurposeSatisfied:          PurposeUnknown,
		Verdict:                   IntentApprove,
		Hypothesis: IntentHypothesis{
			ExpressedObjective: "make a consequential recommendation", LatentIntentHypothesis: "unknown",
			Confidence: 0.2, Materiality: "high", DisconfirmationCondition: "the owner clarifies the decision context",
		},
	}
	if err := ValidateIntentReview(packet, result); err == nil || !strings.Contains(err.Error(), "clarification") {
		t.Fatalf("low-confidence consequential approval was not returned to Maestro: %v", err)
	}
}

func TestMaestroEvaluatesInteractionWithoutPersistingAnUnauthenticatedLoop(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanFor(caseInput(true))
	if err != nil {
		t.Fatal(err)
	}
	state, err := NewChain(plan, DefaultLoopPolicy)
	if err != nil {
		t.Fatal(err)
	}
	input := ownerctx.ObservationInput{
		SchemaVersion: 1, Signal: ownerctx.SignalObservedPattern, Facet: "voice", Claim: "concise", EvidenceType: "adapter_receipt",
		SourceEvent: "loop-event", SourceDigest: SHA256Hex("loop-event"), EpisodeID: "episode-one", ScopeKind: "workspace", ScopeID: "workspace-one",
		Confidence: 0.4, Sensitivity: "professional", ExpiresAt: time.Now().UTC().Add(time.Hour), Material: true,
	}
	if _, _, err := state.Advance(plan, DefaultLoopPolicy, "maestro", Event{AgentID: "case-agent-transformation", Decision: "return", ContentDigest: digestFor("draft"), Interaction: &input}); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.ListObservations(root); err != nil {
		t.Fatal(err)
	}
}
