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
		EvidenceRefs:              []string{"current_prompt", "case:summary"},
		Confidence:                0.85,
		PurposeSatisfied:          PurposeSatisfied,
		Verdict:                   IntentApprove,
		Hypothesis: IntentHypothesis{
			ExpressedObjective: "prepare the client-ready answer", LatentIntentHypothesis: "make the next decision easier",
			EvidenceRefs: []string{"current_prompt", "case:summary"}, Confidence: 0.85, Materiality: "medium", DisconfirmationCondition: "the sponsor cannot act from the answer", WorkingPrompt: packet.WorkingCurrentPrompt,
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
		EvidenceRefs:              []string{"current_prompt"},
		Confidence:                0.2,
		PurposeSatisfied:          PurposeUnknown,
		Verdict:                   IntentApprove,
		Hypothesis: IntentHypothesis{
			ExpressedObjective: "make a consequential recommendation", LatentIntentHypothesis: "unknown",
			EvidenceRefs: []string{"current_prompt"}, Confidence: 0.2, Materiality: "high", DisconfirmationCondition: "the owner clarifies the decision context", WorkingPrompt: packet.WorkingCurrentPrompt,
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

func TestPromptHistoryIsSelectedNormalizedAndQuotedInEphemeralWalterPacket(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.RecordUserPrompt(root, ownerctx.PromptHistoryInput{OwnerID: "owner", Prompt: "  historico  ", Language: "pt-BR", Source: "owner", SessionID: "session-a", ScopeKind: ownerctx.PromptScopeCase, ScopeID: "case-a", RecordedAt: time.Now().UTC(), ContentKind: "user_prompt"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ownerctx.ProjectSnapshot(root, []string{"voice"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanFor(caseInput(true))
	if err != nil {
		t.Fatal(err)
	}
	packet, err := BuildIntentReviewPacketWithPromptHistory("current user prompt", plan, "draft", nil, snapshot, nil, "owner", "low", "reversible", "", root, ownerctx.PromptHistorySelectionLimits{OwnerID: "owner", MaxCount: 4, MaxBytes: 1024, MaxAge: time.Hour, ScopeKind: ownerctx.PromptScopeCase, ScopeID: "case-a", CurrentLanguage: "pt-BR"}, "pt-BR", nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if packet.CurrentPrompt != "current user prompt" || len(packet.PriorPrompts) != 1 || packet.PriorPrompts[0].OriginalText != "  historico  " || packet.PriorPrompts[0].NormalizedText != "historico" || !packet.PriorPrompts[0].QuotedData {
		t.Fatalf("prompt packet = %#v", packet)
	}
	hypothesis, err := DeriveIntentHypothesis(packet, "answer current request", "maintain continuity without losing current intent", []string{"current_prompt", packet.PriorPrompts[0].ID}, 0.8, []string{"history is irrelevant"}, "medium", "the owner rejects the continuity assumption")
	if err != nil || hypothesis.Confidence != 0.8 {
		t.Fatalf("hypothesis = %#v, err = %v", hypothesis, err)
	}
}

func TestPromptHistoryTranslationStageFailsClosedWithoutTranslator(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.RecordUserPrompt(root, ownerctx.PromptHistoryInput{OwnerID: "owner", Prompt: "prior", Language: "en-US", Source: "owner", SessionID: "session-a", ScopeKind: ownerctx.PromptScopeGlobal, ScopeID: "owner", RecordedAt: time.Now().UTC(), ContentKind: "user_prompt"}); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ownerctx.ProjectSnapshot(root, []string{"voice"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanFor(caseInput(true))
	if err != nil {
		t.Fatal(err)
	}
	limits := ownerctx.PromptHistorySelectionLimits{OwnerID: "owner", MaxCount: 1, MaxBytes: 100, MaxAge: time.Hour, ScopeKind: ownerctx.PromptScopeGlobal, ScopeID: "owner", CurrentLanguage: "en-US"}
	if _, err := BuildIntentReviewPacketWithPromptHistory("current", plan, "draft", nil, snapshot, nil, "owner", "low", "reversible", "", root, limits, "pt-BR", nil, time.Now().UTC()); err == nil {
		t.Fatal("missing translator was accepted")
	}
	packet, err := BuildIntentReviewPacketWithPromptHistory("current", plan, "draft", nil, snapshot, nil, "owner", "low", "reversible", "", root, limits, "pt-BR", func(original, source, target string) (string, error) { return "traduzido", nil }, time.Now().UTC())
	if err != nil || len(packet.PriorPrompts) != 1 || packet.PriorPrompts[0].OriginalText != "prior" || packet.PriorPrompts[0].NormalizedText != "traduzido" {
		t.Fatalf("translated packet = %#v, err = %v", packet, err)
	}
}

func TestCurrentPromptWorkingStageRunsBeforeHypothesisAndPreservesOriginal(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	snapshot, err := ownerctx.ProjectSnapshot(root, []string{"voice"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := PlanFor(caseInput(true))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	limits := ownerctx.PromptHistorySelectionLimits{OwnerID: "owner", MaxCount: 1, MaxBytes: 1024, MaxAge: time.Hour, ScopeKind: ownerctx.PromptScopeGlobal, ScopeID: "owner", CurrentLanguage: "en-US"}
	packet, err := BuildIntentReviewPacketWithPromptHistory("  preserve this exact instruction  ", plan, "draft", nil, snapshot, nil, "owner", "low", "reversible", "", root, limits, "pt-BR", func(original, source, target string) (string, error) {
		called = true
		return "texto de trabalho", nil
	}, time.Now().UTC())
	if err != nil || !called || packet.LiteralRequest != "  preserve this exact instruction  " || packet.WorkingCurrentPrompt != "texto de trabalho" {
		t.Fatalf("current working stage = %#v, err=%v, called=%v", packet, err, called)
	}
	hypothesis, err := DeriveIntentHypothesis(packet, "preserve", "honor current instruction", []string{"current_prompt"}, 0.9, nil, "low", "the owner changes the instruction")
	if err != nil || hypothesis.WorkingPrompt != "texto de trabalho" {
		t.Fatalf("working hypothesis = %#v, err=%v", hypothesis, err)
	}
	if packet.LiteralRequest == packet.WorkingCurrentPrompt {
		t.Fatal("working prompt unexpectedly replaced the original instruction")
	}
	limits.CurrentLanguage = "en-US"
	if _, err := BuildIntentReviewPacketWithPromptHistory("current", plan, "draft", nil, snapshot, nil, "owner", "low", "reversible", "", root, limits, "pt-BR", nil, time.Now().UTC()); err == nil {
		t.Fatal("current prompt language divergence without translator was accepted")
	}
}

func TestIntentEvidenceRequiresCurrentAndRejectsInventedOrHistoryOnlyRefs(t *testing.T) {
	root := t.TempDir()
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	if _, err := ownerctx.RecordUserPrompt(root, ownerctx.PromptHistoryInput{OwnerID: "owner", Prompt: "prior", Language: "en-US", Source: "owner", SessionID: "session-a", ScopeKind: ownerctx.PromptScopeGlobal, ScopeID: "owner", RecordedAt: time.Now().UTC(), ContentKind: "user_prompt"}); err != nil {
		t.Fatal(err)
	}
	snapshot, _ := ownerctx.ProjectSnapshot(root, []string{"voice"})
	plan, _ := PlanFor(caseInput(true))
	packet, err := BuildIntentReviewPacketWithPromptHistory("current", plan, "draft", nil, snapshot, nil, "owner", "low", "reversible", "", root, ownerctx.PromptHistorySelectionLimits{OwnerID: "owner", MaxCount: 1, MaxBytes: 1024, MaxAge: time.Hour, ScopeKind: ownerctx.PromptScopeGlobal, ScopeID: "owner", CurrentLanguage: "en-US"}, "en-US", nil, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DeriveIntentHypothesis(packet, "objective", "latent", []string{packet.PriorPrompts[0].ID}, 0.8, nil, "low", "owner corrects the prompt"); err == nil {
		t.Fatal("history-only hypothesis evidence was accepted")
	}
	if _, err := DeriveIntentHypothesis(packet, "objective", "latent", []string{"forged-ref", "current_prompt"}, 0.8, nil, "low", "owner corrects the prompt"); err == nil {
		t.Fatal("invented hypothesis evidence was accepted")
	}
}
