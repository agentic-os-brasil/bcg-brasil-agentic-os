package selfmodel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func validObservation() Observation {
	return Observation{
		SchemaVersion: SchemaVersion, ObservationID: "obs-1", Signal: ExplicitCorrection,
		Lifecycle: Captured, SourceEvent: "owner-feedback", SourceEventSHA256: Digest("event"),
		OccurredAt: time.Unix(1, 0).UTC(), ScopeKind: "global", ScopeID: OwnerScope,
		ClaimSHA256: Digest("minimized-claim"), EvidenceType: "owner_speech",
		ProvenanceSHA256: Digest("provenance"), Confidence: ConfidenceHigh,
		Sensitivity: "professional", Materiality: MaterialityHigh, OwnerAuthenticated: true,
	}
}

func TestEvaluateInteractionIsIndependentOfWalterAndPersistsOnlyMaterialOwnerSignals(t *testing.T) {
	observation, persist, err := EvaluateInteraction(InteractionInput{
		ObservationID: "obs-1", Signal: ExplicitInstruction, SourceEvent: "owner-feedback",
		SourceEventSHA256: Digest("event"), OccurredAt: time.Now().UTC(), ScopeKind: "global",
		ScopeID: OwnerScope, ClaimSHA256: Digest("claim"), EvidenceType: "owner_speech",
		ProvenanceSHA256: Digest("provenance"), Confidence: ConfidenceHigh,
		Sensitivity: "professional", Materiality: MaterialityHigh, OwnerAuthenticated: true,
	})
	if err != nil || !persist || observation.Signal != ExplicitInstruction {
		t.Fatalf("expected an authenticated material signal, observation=%+v persist=%v err=%v", observation, persist, err)
	}
	_, persist, err = EvaluateInteraction(InteractionInput{
		ObservationID: "obs-2", Signal: InferredHypothesis, SourceEvent: "agent-result",
		SourceEventSHA256: Digest("event-2"), OccurredAt: time.Now().UTC(), ScopeKind: "case",
		ScopeID: "case-1", ClaimSHA256: Digest("claim-2"), EvidenceType: "agent_output",
		ProvenanceSHA256: Digest("provenance-2"), Confidence: ConfidenceLow,
		Sensitivity: "internal", Materiality: MaterialityHigh, OwnerAuthenticated: false,
	})
	if err != nil || persist {
		t.Fatalf("unauthenticated agent output must stay ephemeral, persist=%v err=%v", persist, err)
	}
}

func TestStoreIsAppendOnlyAndTombstonesWithoutRawContent(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}
	observation := validObservation()
	if err := store.Append(observation); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(root, "observations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "client secret") || strings.Contains(string(body), "raw prompt") {
		t.Fatal("self log must not contain raw client or prompt content")
	}
	if err := store.Tombstone(observation.ObservationID, Redacted); err != nil {
		t.Fatal(err)
	}
	observations, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(observations) != 0 {
		t.Fatalf("redacted observation should not be visible, got %d", len(observations))
	}
	lines := strings.Count(string(body), "\n")
	updated, err := os.ReadFile(filepath.Join(root, "observations.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(updated), "\n") != lines+1 {
		t.Fatal("tombstone must append rather than rewrite history")
	}
}

func TestExplicitCorrectionContradictsPriorObservationWithoutRewritingHistory(t *testing.T) {
	store := Store{Root: t.TempDir()}
	prior := validObservation()
	if err := store.Append(prior); err != nil {
		t.Fatal(err)
	}
	correction := prior
	correction.ObservationID = "obs-correction"
	correction.Signal = ExplicitCorrection
	correction.SupersedesObservationID = prior.ObservationID
	if err := store.Append(correction); err != nil {
		t.Fatal(err)
	}
	observations, err := store.List()
	if err != nil || len(observations) != 1 || observations[0].ObservationID != correction.ObservationID {
		t.Fatalf("correction did not supersede the prior provisional claim: %+v err=%v", observations, err)
	}
}

func TestPromotionRequiresOwnerControlAndIndependentEpisodes(t *testing.T) {
	observation := validObservation()
	observation.Signal = InferredHypothesis
	if lifecycle, err := PromotionLifecycle(observation, PromotionDecision{ObservationID: observation.ObservationID, Facet: "decision-rules", Episodes: 1}); err != nil || lifecycle != Proposed {
		t.Fatalf("isolated inference must remain a proposal, lifecycle=%s err=%v", lifecycle, err)
	}
	if lifecycle, err := PromotionLifecycle(observation, PromotionDecision{ObservationID: observation.ObservationID, Facet: "decision-rules", Episodes: 2, UserConfirmed: true}); err != nil || lifecycle != Promoted {
		t.Fatalf("owner-confirmed corroborated pattern should be promotable, lifecycle=%s err=%v", lifecycle, err)
	}
	observation.Signal = ExplicitInstruction
	if lifecycle, err := PromotionLifecycle(observation, PromotionDecision{ObservationID: observation.ObservationID, Facet: "working-boundaries"}); err != nil || lifecycle != Proposed {
		t.Fatalf("boundary promotion must require explicit confirmation, lifecycle=%s err=%v", lifecycle, err)
	}
	snapshot, err := NewCanonicalSnapshot(1, map[string]string{"decision-rules": Digest("rules")}, time.Unix(2, 0))
	if err != nil {
		t.Fatal(err)
	}
	if lifecycle, err := AuthorizePromotion(snapshot, observation, PromotionDecision{ObservationID: observation.ObservationID, Facet: "decision-rules", UserConfirmed: true, ExpectedSnapshotDigest: strings.Repeat("f", 64)}); err == nil || lifecycle != "" {
		t.Fatal("promotion with a stale canonical snapshot digest was accepted")
	}
}

func TestSelfSignalPrecedenceKeepsHypothesisBelowOwnerAndCanon(t *testing.T) {
	if AuthorityLevel(ExplicitInstruction) <= CanonicalAuthorityLevel ||
		CanonicalAuthorityLevel <= AuthorityLevel(ObservedPattern) ||
		AuthorityLevel(ObservedPattern) <= AuthorityLevel(InferredHypothesis) {
		t.Fatal("self signal precedence does not preserve owner/canon/observation ordering")
	}
}
