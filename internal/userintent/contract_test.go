package userintent

import (
	"strings"
	"testing"
	"time"
)

var testIntentTime = time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)

func testSnapshot() UserSelfSnapshot {
	return UserSelfSnapshot{
		SchemaVersion: SchemaVersion, Version: 7, Digest: strings.Repeat("a", 64), Owner: "user",
		ProjectionOf:     "owner_context",
		PrecedencePolicy: []string{"explicit_instruction", "explicit_correction", "canonical_snapshot", "recent_observation", "walter_hypothesis"},
		Sections:         []SelfSection{SectionPreferences, SectionPrinciples, SectionDecisionRules, SectionCommunication, SectionMotivations, SectionBoundaries},
	}
}

func TestIntentReviewReceiptBindsPromptDraftAndOwnerContextProjection(t *testing.T) {
	packet, err := NewIntentReviewPacket("packet-1", "prepare the answer", "account_first", "plan-digest-input", "draft answer", testSnapshot(), []string{"case/case-1"}, nil, AudienceUser, ConsequenceMaterial, Reversible)
	if err != nil {
		t.Fatal(err)
	}
	review := IntentReview{
		SchemaVersion: SchemaVersion, LiteralRequest: "prepare the answer",
		IntrinsicIntentHypothesis: IntentHypothesis{Text: "the user needs a decision-ready answer", Confidence: ConfidenceMedium, Status: "hypothesis"},
		EvidenceRefs:              []string{"case/case-1"}, Confidence: ConfidenceMedium, Alternatives: []string{"literal-only answer"},
		Materiality: ConsequenceMaterial, DisconfirmationCondition: "user states the answer is only a transcription",
		PurposeSatisfied: PurposeYes, Verdict: VerdictApprove,
	}
	receipt, err := NewIntentReviewReceipt("review-1", packet, review, 2, testIntentTime)
	if err != nil {
		t.Fatal(err)
	}
	if err := receipt.Validate(packet); err != nil {
		t.Fatal(err)
	}
	packet.DraftOutput = "changed"
	if err := receipt.Validate(packet); err == nil {
		t.Fatal("receipt accepted a changed draft")
	}
}

func TestIntentReviewLowConfidenceClarifiesButDoesNotHold(t *testing.T) {
	packet, err := NewIntentReviewPacket("packet-2", "make a call", "case_direct", "plan", "draft", testSnapshot(), nil, nil, AudienceUser, ConsequenceHigh, HardToReverse)
	if err != nil {
		t.Fatal(err)
	}
	review := IntentReview{
		SchemaVersion: SchemaVersion, LiteralRequest: "make a call",
		IntrinsicIntentHypothesis: IntentHypothesis{Text: "the user may want a recommendation", Confidence: ConfidenceLow, Status: "hypothesis"},
		EvidenceRefs:              []string{"case/case-2"}, Confidence: ConfidenceLow, Materiality: ConsequenceHigh,
		DisconfirmationCondition: "the user says no recommendation is wanted", PurposeSatisfied: PurposeUnknown,
		UnresolvedUncertainty: "decision criteria are not explicit", Verdict: VerdictClarify,
	}
	if err := review.Validate(packet); err != nil {
		t.Fatal(err)
	}
	review.Verdict = VerdictHoldExceptional
	if err := review.Validate(packet); err == nil {
		t.Fatal("low-confidence uncertainty became an exceptional hold")
	}
}

func testObservation(id, claim string, kind SignalKind, facet SelfSection, scope ObservationScope, episode string) InteractionObservation {
	return InteractionObservation{
		SchemaVersion: SchemaVersion, ObservationID: id, SourceEventSHA256: strings.Repeat("b", 64), Kind: kind,
		Facet: facet, SignalKey: string(facet), ClaimDigest: claim, EpisodeSHA256: episode,
		EvidenceType: EvidenceOwnerSpeech, OwnerAuthenticated: true, Material: true, Declassified: scope == ScopeGlobal, Scope: scope,
		ConfidenceBasisPts: 8000, Sensitivity: SensitivityNormal, Lifecycle: LifecycleEligible,
		RecordedAt: testIntentTime.Add(-2 * time.Hour), ExpiresAt: testIntentTime.Add(90 * 24 * time.Hour), RecheckAt: testIntentTime.Add(24 * time.Hour),
		UserConfirmed: kind == ExplicitInstruction || kind == ExplicitCorrection,
	}
}

func TestAbsorptionLogDeduplicatesAndDarwinOnlyProposesReevaluation(t *testing.T) {
	log := AbsorptionLog{}
	first := testObservation("obs-1", strings.Repeat("c", 64), ExplicitInstruction, SectionCommunication, ScopeGlobal, strings.Repeat("d", 64))
	if _, err := log.Append(first); err != nil {
		t.Fatal(err)
	}
	duplicate, err := log.Append(first)
	if err != nil || !duplicate.Duplicate || log.DuplicateCount != 1 {
		t.Fatalf("duplicate = %#v, err = %v, count = %d", duplicate, err, log.DuplicateCount)
	}
	second := testObservation("obs-2", strings.Repeat("e", 64), ExplicitCorrection, SectionCommunication, ScopeGlobal, strings.Repeat("f", 64))
	second.SupersedesIDs = []string{"obs-1"}
	contradiction, err := log.Append(second)
	if err != nil || !contradiction.Contradiction {
		t.Fatalf("contradiction = %#v, err = %v", contradiction, err)
	}
	log.Observations[0].RecheckAt = testIntentTime.Add(-time.Hour)
	report, err := log.Analyze(testSnapshot(), testIntentTime)
	if err != nil {
		t.Fatal(err)
	}
	if report.DuplicateCount != 1 || report.ContradictionCount != 1 || report.ReevaluationProposals == 0 || report.CanonicalMutationsByDarwin != 0 {
		t.Fatalf("report = %#v", report)
	}
	for _, proposal := range report.ProposalReceipts {
		if proposal.Kind != "reevaluate_facet" || proposal.MayPromoteCanonical || proposal.Status != "proposal_only" || proposal.BaseSnapshotSHA256 != testSnapshot().Digest {
			t.Fatalf("unsafe proposal = %#v", proposal)
		}
	}
}

func TestAbsorptionLogRejectsUnauthenticatedOrNonOwnerEvidence(t *testing.T) {
	observation := testObservation("obs-3", strings.Repeat("1", 64), ObservedPattern, SectionPreferences, ScopeCase, strings.Repeat("2", 64))
	observation.OwnerAuthenticated = false
	if err := observation.Validate(); err == nil {
		t.Fatal("unauthenticated observation was accepted")
	}
	observation = testObservation("obs-4", strings.Repeat("1", 64), ExplicitCorrection, SectionPreferences, ScopeAccount, strings.Repeat("2", 64))
	if err := observation.Validate(); err == nil {
		t.Fatal("correction without supersession was accepted")
	}
	observation = testObservation("obs-6", strings.Repeat("5", 64), ObservedPattern, SectionPreferences, ScopeWorkspace, strings.Repeat("6", 64))
	if _, err := (&AbsorptionLog{}).Append(observation); err == nil {
		t.Fatal("observed pattern was persisted as owner fact")
	}
}

func TestEveryInteractionIsEvaluatedButOnlyAuthenticatedOwnerSignalsPersist(t *testing.T) {
	observation := testObservation("obs-5", strings.Repeat("3", 64), ObservedPattern, SectionDecisionRules, ScopeInteraction, strings.Repeat("4", 64))
	decision := EvaluateInteraction(observation)
	if decision.Persistable || decision.ReasonCode != "hypothesis_or_pattern_is_ephemeral" {
		t.Fatalf("pattern evaluation = %#v", decision)
	}
	observation.Kind = ExplicitEndorsement
	decision = EvaluateInteraction(observation)
	if !decision.Persistable || decision.ReasonCode != "authenticated_owner_signal" {
		t.Fatalf("owner evaluation = %#v", decision)
	}
}
