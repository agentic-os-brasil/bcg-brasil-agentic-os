package memory

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func generationEngine(t *testing.T, minimum int) *Engine {
	t.Helper()
	if err := ownedIOAvailable(); err != nil {
		t.Skip(err)
	}
	e := testEngine(t)
	if err := e.PrepareOwnedCaptureAuthority("case-a"); err != nil {
		t.Fatal(err)
	}
	for _, layer := range []string{"L1", "L2", "L3", "lifetime"} {
		e.Budgets[layer] = 12000
	}
	e.Synthesizer = DeterministicWeeklySynthesizer{Daily: DeterministicL1Synthesizer{MaxRunes: 1000, MaxEntries: 8, MaxInputBytes: 1 << 20, MaxInputEntries: 256, Attestor: CaptureAttestor{Root: e.Root}}, MaxRunes: e.Budgets, MaxInputBytes: 1 << 20}
	e.SynthesizerID = DeterministicWeeklySynthesizerID
	e.Eligibility = DeterministicLifetimeEligibility{MinL3Generations: minimum}
	e.EligibilityPolicyID = "deterministic-l3-continuity-v1"
	return e
}

func addGeneration(t *testing.T, e *Engine, day time.Time, text string) (CaptureReceipt, DreamResult) {
	t.Helper()
	r, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, text, day))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
	weekly, err := e.DreamWeekly(context.Background(), "case-a", day)
	if err != nil {
		t.Fatal(err)
	}
	return r, weekly
}
func promoted(result DreamResult) bool {
	for _, layer := range result.ActivatedLayers {
		if layer == "lifetime" {
			return true
		}
	}
	return false
}

func TestL3GenerationEvidenceIgnoresLiteralSourceText(t *testing.T) {
	e := generationEngine(t, 2)
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	_, first := addGeneration(t, e, day, "quoted literals: "+strings.Repeat("# L3 rolling continuity · ", 3))
	if promoted(first) {
		t.Fatalf("literal text forged a committed generation: %#v", first)
	}
	rerun, err := e.DreamWeekly(context.Background(), "case-a", day)
	if err != nil || !rerun.Skipped || promoted(rerun) {
		t.Fatalf("unchanged run counted as generation: %#v %v", rerun, err)
	}
	_, second := addGeneration(t, e, day, "second actual L3 transition in the same calendar week")
	if !promoted(second) {
		t.Fatalf("second actual transition did not qualify: %#v", second)
	}
}

func TestL3GenerationEvidenceHonorsConfiguredMinimumAndVisibility(t *testing.T) {
	e := generationEngine(t, 3)
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	first, r1 := addGeneration(t, e, day, "one")
	_, r2 := addGeneration(t, e, day, "two")
	_, r3 := addGeneration(t, e, day, "three")
	if promoted(r1) || promoted(r2) || !promoted(r3) {
		t.Fatalf("minimum-three transition evidence: %#v %#v %#v", r1, r2, r3)
	}
	if _, err := e.DeleteCapture("case-a", first.ID, first.Revision); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
	reset, err := e.DreamWeekly(context.Background(), "case-a", day)
	if err != nil || promoted(reset) {
		t.Fatalf("revoked generation carried forward: %#v %v", reset, err)
	}
	if _, _, err := e.readArtifactByKey("case-a", "lifetime/current"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lifetime survived visibility reset: %v", err)
	}
}

func TestL3GenerationEvidenceIsPrivateAndCandidateBound(t *testing.T) {
	e := generationEngine(t, 2)
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	var observed Artifact
	e.Eligibility = EligibilityFunc(func(ctx context.Context, candidate Artifact) (bool, string, error) {
		observed = candidate
		return (DeterministicLifetimeEligibility{MinL3Generations: 2}).Evaluate(ctx, candidate)
	})
	addGeneration(t, e, day, "first")
	addGeneration(t, e, day, "second")
	policy := DeterministicLifetimeEligibility{MinL3Generations: 2}
	body, err := json.Marshal(observed)
	if err != nil {
		t.Fatal(err)
	}
	var restored Artifact
	if err := json.Unmarshal(body, &restored); err != nil {
		t.Fatal(err)
	}
	roundTrip, err := json.Marshal(restored)
	if err != nil || string(roundTrip) != string(body) {
		t.Fatalf("private evidence changed artifact JSON/readback: %v", err)
	}
	forged := append(append([]byte{}, body[:len(body)-1]...), []byte(`,"l3_generations":99}`)...)
	var untrusted Artifact
	if err := decodeMemoryJSON(forged, &untrusted); err == nil {
		t.Fatal("serialized generation authority was admitted")
	}
	if eligible, _, err := policy.Evaluate(context.Background(), restored); err != nil || eligible {
		t.Fatalf("serialized payload granted trusted evidence: %v %v", eligible, err)
	}
	changed := observed
	changed.WorkspaceID = "case-b"
	if eligible, _, err := policy.Evaluate(context.Background(), changed); err == nil && eligible {
		t.Fatal("generation evidence retargeted to another candidate")
	}
}

func TestL3GenerationEvidenceRejectsForgedAndStaleSources(t *testing.T) {
	e := generationEngine(t, 2)
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	addGeneration(t, e, day, "first committed generation")
	current, _, err := e.readArtifactByKey("case-a", "L3/current")
	if err != nil {
		t.Fatal(err)
	}
	sources := []SourceDocument{artifactSource(current)}
	candidate := e.newArtifact("case-a", "L3", "rolling", e.sourceFingerprint("L3-proof", sources), sources, "next L3 candidate")
	if count, err := e.l3GenerationCount("case-a", candidate); err != nil || count != 2 {
		t.Fatalf("real predecessor evidence: count=%d %v", count, err)
	}
	for _, kind := range []string{"digest", "identity", "duplicate", "workspace", "layer", "visibility"} {
		t.Run(kind, func(t *testing.T) {
			changed := candidate
			changed.Sources = append([]SourceRef(nil), candidate.Sources...)
			switch kind {
			case "digest":
				changed.Sources[0].SHA256 = strings.Repeat("0", 64)
			case "identity":
				changed.Sources[0].ID = "L3/unknown"
			case "duplicate":
				changed.Sources = append(changed.Sources, changed.Sources[0])
			case "workspace":
				changed.WorkspaceID = "case-b"
			case "layer":
				changed.Layer = "L2"
			case "visibility":
				changed.VisibilityRevision++
			}
			if _, err := e.l3GenerationCount("case-a", changed); err == nil {
				t.Fatal("forged or stale ancestry granted evidence")
			}
		})
	}
}

func TestL3GenerationEvidenceExcludesUncommittedVersions(t *testing.T) {
	e := generationEngine(t, 2)
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	if _, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "orphan marker # L3 rolling continuity ·", day)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
	e.FaultPoint = func(point string) error {
		if point == "after_publish_version" {
			return errors.New("orphan L3 version")
		}
		return nil
	}
	if _, err := e.DreamWeekly(context.Background(), "case-a", day); err == nil {
		t.Fatal("fixture did not interrupt")
	}
	e.FaultPoint = nil
	result, err := e.DreamWeekly(context.Background(), "case-a", day)
	if err != nil || promoted(result) {
		t.Fatalf("uncommitted version counted as generation: %#v %v", result, err)
	}
}

func TestL3GenerationEvidenceRejectsBrokenCommittedAncestry(t *testing.T) {
	e := generationEngine(t, 3)
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	addGeneration(t, e, day, "first")
	addGeneration(t, e, day, "second")
	current, path, err := e.readArtifactByKey("case-a", "L3/current")
	if err != nil {
		t.Fatal(err)
	}
	for index := range current.Sources {
		if strings.HasPrefix(current.Sources[index].ID, "L3/") {
			current.Sources[index].SHA256 = strings.Repeat("0", 64)
		}
	}
	body, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	if err := memoryPublish(path, body, false); err != nil {
		t.Fatal(err)
	}
	if _, err := e.CaptureOwned("case-a", ownedInput(t, e, "", 0, "third", day)); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DreamDailyAttested(context.Background(), "case-a", day); err != nil {
		t.Fatal(err)
	}
	if _, err := e.DreamWeekly(context.Background(), "case-a", day); err == nil {
		t.Fatal("unknown committed predecessor granted promotion evidence")
	}
	if _, _, err := e.readArtifactByKey("case-a", "lifetime/current"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("broken ancestry activated lifetime: %v", err)
	}
}
