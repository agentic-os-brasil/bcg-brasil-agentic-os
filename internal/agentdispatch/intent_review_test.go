package agentdispatch

import (
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userintent"
)

func TestWalterReceivesIntentPacketAndReturnsBoundHypothesis(t *testing.T) {
	snapshot := userintent.UserSelfSnapshot{
		SchemaVersion: userintent.SchemaVersion, Version: 1, Digest: strings.Repeat("a", 64), Owner: "user", ProjectionOf: "owner_context",
		PrecedencePolicy: []string{"explicit_instruction", "explicit_correction", "canonical_snapshot", "recent_observation", "walter_hypothesis"},
		Sections:         []userintent.SelfSection{userintent.SectionPreferences, userintent.SectionPrinciples, userintent.SectionDecisionRules, userintent.SectionCommunication, userintent.SectionMotivations, userintent.SectionBoundaries},
	}
	packet, err := userintent.NewIntentReviewPacket("intent-1", "solve this", "case_direct", "plan", "draft", snapshot, []string{"case/case-1"}, nil, userintent.AudienceUser, userintent.ConsequenceMaterial, userintent.Reversible)
	if err != nil {
		t.Fatal(err)
	}
	review := ReviewPacket{SourcePacketID: strings.Repeat("b", 64), SourcePacketSHA256: strings.Repeat("c", 64), SourceScopeKind: "case", SourceScopeID: "case-1", Trigger: ReviewMaterialRecommendation, Audience: "user", Recommendation: "draft", DefinitionOfDone: "answer", SelfReview: &packet}
	if err := validateReviewPacket(&review, strings.Repeat("d", 64), "objective"); err != nil {
		t.Fatal(err)
	}
	body := WalterReviewBody{Verdict: WalterApproved, IntentReview: &userintent.IntentReview{
		SchemaVersion: userintent.SchemaVersion, LiteralRequest: "solve this",
		IntrinsicIntentHypothesis: userintent.IntentHypothesis{Text: "the user wants a useful answer", Confidence: userintent.ConfidenceMedium, Status: "hypothesis"},
		EvidenceRefs:              []string{"case/case-1"}, Confidence: userintent.ConfidenceMedium, Materiality: userintent.ConsequenceMaterial,
		DisconfirmationCondition: "the user says this was only a literal lookup", PurposeSatisfied: userintent.PurposeYes, Verdict: userintent.VerdictApprove,
	}}
	if err := validateWalterReviewBody(body, review); err != nil {
		t.Fatal(err)
	}
	if _, err := userintent.NewIntentReviewReceipt("review-1", packet, *body.IntentReview, 1, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
}
