package darwin

import (
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userintent"
)

func TestMaintainSelfLoopRemainsMetadataOnly(t *testing.T) {
	log := userintent.AbsorptionLog{}
	observation := userintent.InteractionObservation{
		SchemaVersion: userintent.SchemaVersion, ObservationID: "obs-darwin", SourceEventSHA256: strings.Repeat("a", 64),
		Kind: userintent.ExplicitEndorsement, Facet: userintent.SectionCommunication, SignalKey: "communication_style",
		ClaimDigest: strings.Repeat("b", 64), EpisodeSHA256: strings.Repeat("c", 64), EvidenceType: userintent.EvidenceOwnerSpeech,
		OwnerAuthenticated: true, Material: true, Declassified: false, Scope: userintent.ScopeWorkspace, ConfidenceBasisPts: 7000,
		Sensitivity: userintent.SensitivityNormal, Lifecycle: userintent.LifecycleEligible,
		RecordedAt: testTime.Add(-2 * time.Hour), ExpiresAt: testTime.Add(24 * 90 * time.Hour), RecheckAt: testTime.Add(-time.Hour), UserConfirmed: true,
	}
	if _, err := log.Append(observation); err != nil {
		t.Fatal(err)
	}
	receipt, err := MaintainSelfLoop(testSelfSnapshot(), log, testTime)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.AgentID != AgentID || receipt.ReevaluationProposals == 0 || receipt.CanonicalMutations != 0 || receipt.SnapshotSHA256 != testSelfSnapshot().Digest {
		t.Fatalf("receipt = %#v", receipt)
	}
}

func testSelfSnapshot() userintent.UserSelfSnapshot {
	return userintent.UserSelfSnapshot{
		SchemaVersion: userintent.SchemaVersion, Version: 2, Digest: strings.Repeat("d", 64), Owner: "user", ProjectionOf: "owner_context",
		PrecedencePolicy: []string{"explicit_instruction", "explicit_correction", "canonical_snapshot", "recent_observation", "walter_hypothesis"},
		Sections:         []userintent.SelfSection{userintent.SectionPreferences, userintent.SectionPrinciples, userintent.SectionDecisionRules, userintent.SectionCommunication, userintent.SectionMotivations, userintent.SectionBoundaries},
	}
}
