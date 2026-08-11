package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeterministicWeeklySynthesizerPromotesOnlyAfterTwoL3Generations(t *testing.T) {
	policy := validPolicyForTest()
	root := filepath.Join(t.TempDir(), "memory")
	attestor := CaptureAttestor{Root: root}
	daily := DeterministicL1Synthesizer{MaxRunes: 1000, MaxEntries: 8, MaxInputBytes: 4096, MaxInputEntries: 16, Attestor: attestor}
	engine := Engine{
		Root: root, Policy: policy, Budgets: map[string]int{"L1": 1000, "L2": 768, "L3": 768, "lifetime": 768}, MaxSourceBytes: 4096,
		Synthesizer: DeterministicWeeklySynthesizer{Daily: daily, MaxRunes: map[string]int{"L2": 768, "L3": 768, "lifetime": 768}, MaxInputBytes: 4096}, SynthesizerID: DeterministicWeeklySynthesizerID,
		Eligibility: DeterministicLifetimeEligibility{MinL3Generations: 2}, EligibilityPolicyID: "deterministic-l3-continuity-v1",
	}
	first := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	for _, day := range []time.Time{first, first.AddDate(0, 7, 0)} {
		capture, err := attestor.Seal(Capture{WorkspaceID: "case-a", RecordedAt: day, Kind: "skill_route", Text: "meeting-close", Sanitized: true, ProducerID: "claude.context-injection", SanitizerID: SkillRouteSanitizerID, SourceDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := engine.Capture(capture); err != nil {
			t.Fatal(err)
		}
		if _, err := engine.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
			t.Fatal(err)
		}
	}
	firstResult, err := engine.DreamWeekly(context.Background(), "case-a", first)
	if err != nil || len(firstResult.ActivatedLayers) != 2 || firstResult.LifetimeReason == "" {
		t.Fatalf("first weekly result=%#v err=%v", firstResult, err)
	}
	if _, _, err := engine.readArtifactByKey("case-a", "lifetime/current"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lifetime promoted before continuity was established: %v", err)
	}
	secondResult, err := engine.DreamWeekly(context.Background(), "case-a", first.AddDate(0, 7, 0))
	if err != nil || len(secondResult.ActivatedLayers) != 3 || secondResult.ActivatedLayers[2] != "lifetime" {
		t.Fatalf("second weekly result=%#v err=%v", secondResult, err)
	}
}
