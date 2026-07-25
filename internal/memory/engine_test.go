package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCaptureRequiresSanitizedInputAndIsolatesWorkspace(t *testing.T) {
	engine := testEngine(t)
	now := time.Date(2026, 7, 20, 10, 0, 0, 0, time.UTC)

	if _, err := engine.Capture(Capture{WorkspaceID: "case-a", RecordedAt: now, Kind: "decision", Text: "unsafe", Sanitized: false}); err == nil {
		t.Fatal("expected unsanitized capture to fail")
	}
	if _, err := engine.Capture(Capture{WorkspaceID: "../case-b", RecordedAt: now, Kind: "decision", Text: "safe", Sanitized: true}); err == nil {
		t.Fatal("expected invalid workspace ID to fail")
	}

	path, err := engine.Capture(Capture{WorkspaceID: "case-a", RecordedAt: now, Kind: "decision", Text: "safe", Sanitized: true})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(engine.Root, "workspaces", "case-a", "l1", "captures", "2026-07-20.jsonl")
	if path != want {
		t.Fatalf("capture path = %q, want %q", path, want)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `"text":"safe"`) {
		t.Fatalf("capture content = %s", content)
	}
}

func TestDailyDreamIsIdempotentAndWritesOnlyL1(t *testing.T) {
	synth := &fakeSynthesizer{outputs: map[string]string{"L1": "daily digest"}}
	engine := testEngine(t)
	engine.Synthesizer = synth
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	if _, err := engine.Capture(Capture{WorkspaceID: "case-a", RecordedAt: day, Kind: "thread", Text: "progress", Sanitized: true}); err != nil {
		t.Fatal(err)
	}

	result, err := engine.DreamDaily(context.Background(), "case-a", day)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped || !reflect.DeepEqual(result.ActivatedLayers, []string{"L1"}) {
		t.Fatalf("daily result = %#v", result)
	}
	if _, _, err := engine.readArtifactByKey("case-a", "lifetime/current"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("daily dream wrote lifetime: %v", err)
	}

	second, err := engine.DreamDaily(context.Background(), "case-a", day)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Skipped || len(second.ActivatedLayers) != 0 || synth.calls["L1"] != 1 {
		t.Fatalf("idempotent daily result = %#v, calls = %#v", second, synth.calls)
	}
}

func TestDreamCycleFailsClosedWhenWorkspaceCycleIsLocked(t *testing.T) {
	synth := &fakeSynthesizer{outputs: map[string]string{"L1": "daily"}}
	engine := testEngine(t)
	engine.Synthesizer = synth
	day := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	if _, err := engine.Capture(Capture{WorkspaceID: "case-a", RecordedAt: day, Kind: "thread", Text: "progress", Sanitized: true}); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(engine.workspaceRoot("case-a"), ".locks", "activation.lock")
	if err := os.MkdirAll(lock, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.DreamDaily(context.Background(), "case-a", day); !errors.Is(err, ErrDreamInProgress) {
		t.Fatalf("locked daily dream error = %v", err)
	}
	if synth.calls["L1"] != 0 {
		t.Fatalf("synthesis ran while lock existed: %#v", synth.calls)
	}
}

func TestWorkspaceActivationLockSerializesDifferentWeeklyPeriods(t *testing.T) {
	engine := testEngine(t)
	engine.Synthesizer = &fakeSynthesizer{outputs: map[string]string{}}
	engine.Eligibility = EligibilityFunc(func(context.Context, Artifact) (bool, string, error) { return false, "not reached", nil })
	release, err := engine.acquireCycleLock("case-a", "activation")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	for _, week := range []time.Time{
		time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC),
		time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC),
	} {
		if _, err := engine.DreamWeekly(context.Background(), "case-a", week); !errors.Is(err, ErrDreamInProgress) {
			t.Fatalf("weekly period %s bypassed workspace lock: %v", week, err)
		}
	}
}

func TestWeeklyDreamPromotesEligibleLifetimeAndVersionsPrevious(t *testing.T) {
	synth := &fakeSynthesizer{outputs: map[string]string{
		"L1":       "daily",
		"L2":       "weekly",
		"L3":       "themes",
		"lifetime": "durable memory",
	}}
	engine := testEngine(t)
	engine.Synthesizer = synth
	engine.Eligibility = EligibilityFunc(func(_ context.Context, candidate Artifact) (bool, string, error) {
		return candidate.Content == "durable memory", "repeated evidence", nil
	})
	week := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	seedDailyDreams(t, engine, "case-a", week)

	old := Artifact{SchemaVersion: 1, WorkspaceID: "case-a", Layer: "lifetime", Period: "lifetime", GeneratedAt: week.Add(-7 * 24 * time.Hour), SourceFingerprint: strings.Repeat("a", 64), Sources: []SourceRef{{ID: "old", SHA256: strings.Repeat("b", 64)}}, SynthesizerID: "test-synth-v1", Content: "old lifetime", Promotion: &PromotionRecord{EligibilityPolicy: "test-eligibility-v1", Eligible: true, Reason: "previously eligible", EvaluatedAt: week.Add(-7 * 24 * time.Hour)}}
	if err := engine.activate("case-a", []Artifact{old}); err != nil {
		t.Fatal(err)
	}
	_, oldCommit, err := engine.latestManifest("case-a")
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.DreamWeekly(context.Background(), "case-a", week)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.ActivatedLayers, []string{"L2", "L3", "lifetime"}) || result.LifetimeReason != "repeated evidence" {
		t.Fatalf("weekly result = %#v", result)
	}
	current, _, err := engine.readArtifactByKey("case-a", "lifetime/current")
	if err != nil {
		t.Fatal(err)
	}
	if current.Content != "durable memory" {
		t.Fatalf("lifetime content = %q", current.Content)
	}
	manifest, _, err := engine.latestManifest("case-a")
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ParentCommit != oldCommit {
		t.Fatalf("weekly parent commit = %q, want %q", manifest.ParentCommit, oldCommit)
	}
	oldManifest, err := engine.readManifest(filepath.Join(engine.workspaceRoot("case-a"), "commits", oldCommit))
	if err != nil {
		t.Fatal(err)
	}
	archived, err := engine.readArtifact(filepath.Join(engine.workspaceRoot("case-a"), filepath.FromSlash(oldManifest.Artifacts["lifetime/current"])))
	if err != nil {
		t.Fatal(err)
	}
	if archived.Content != "old lifetime" {
		t.Fatalf("archived lifetime = %q", archived.Content)
	}
	second, err := engine.DreamWeekly(context.Background(), "case-a", week)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Skipped || second.LifetimeReason != "repeated evidence" || synth.calls["L2"] != 1 {
		t.Fatalf("idempotent weekly result = %#v, calls = %#v", second, synth.calls)
	}
}

func TestWeeklyDreamWithoutEligibilityLeavesLifetimeUntouched(t *testing.T) {
	synth := &fakeSynthesizer{outputs: map[string]string{"L1": "daily", "L2": "weekly", "L3": "themes", "lifetime": "candidate"}}
	engine := testEngine(t)
	engine.Synthesizer = synth
	engine.Eligibility = EligibilityFunc(func(context.Context, Artifact) (bool, string, error) {
		return false, "insufficient repetition", nil
	})
	week := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	seedDailyDreams(t, engine, "case-a", week)

	result, err := engine.DreamWeekly(context.Background(), "case-a", week)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.ActivatedLayers, []string{"L2", "L3"}) || result.LifetimeReason != "insufficient repetition" {
		t.Fatalf("weekly result = %#v", result)
	}
	if _, _, err := engine.readArtifactByKey("case-a", "lifetime/current"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ineligible lifetime was activated: %v", err)
	}
}

func TestWeeklyDreamFailureDoesNotPartiallyActivate(t *testing.T) {
	synth := &fakeSynthesizer{outputs: map[string]string{"L1": "daily", "L2": "weekly"}, failLayer: "L3"}
	engine := testEngine(t)
	engine.Synthesizer = synth
	engine.Eligibility = EligibilityFunc(func(context.Context, Artifact) (bool, string, error) { return true, "eligible", nil })
	week := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	seedDailyDreams(t, engine, "case-a", week)

	if _, err := engine.DreamWeekly(context.Background(), "case-a", week); err == nil {
		t.Fatal("expected weekly synthesis failure")
	}
	if _, _, err := engine.readArtifactByKey("case-a", "L2/2026-W30"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial L2 activation: %v", err)
	}
	if _, _, err := engine.readArtifactByKey("case-a", "L3/current"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial L3 activation: %v", err)
	}
}

func TestWeeklyEmptyLifetimeCandidateDoesNotPartiallyActivate(t *testing.T) {
	synth := &fakeSynthesizer{outputs: map[string]string{"L1": "daily", "L2": "weekly", "L3": "themes", "lifetime": ""}}
	engine := testEngine(t)
	engine.Synthesizer = synth
	engine.Eligibility = EligibilityFunc(func(context.Context, Artifact) (bool, string, error) { return false, "empty", nil })
	week := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	seedDailyDreams(t, engine, "case-a", week)

	if _, err := engine.DreamWeekly(context.Background(), "case-a", week); err == nil || !strings.Contains(err.Error(), "candidate") {
		t.Fatalf("weekly dream error = %v", err)
	}
	if _, _, err := engine.readArtifactByKey("case-a", "L2/2026-W30"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("partial L2 activation: %v", err)
	}
}

func TestWeeklyDreamFailsClosedWithoutEligibilityPolicy(t *testing.T) {
	synth := &fakeSynthesizer{outputs: map[string]string{"L1": "daily", "L2": "weekly", "L3": "themes", "lifetime": "candidate"}}
	engine := testEngine(t)
	engine.Synthesizer = synth
	week := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	seedDailyDreams(t, engine, "case-a", week)
	engine.Eligibility = nil

	if _, err := engine.DreamWeekly(context.Background(), "case-a", week); err == nil || !strings.Contains(err.Error(), "eligibility policy") {
		t.Fatalf("weekly dream error = %v", err)
	}
	if synth.calls["L2"] != 0 {
		t.Fatalf("synthesis ran before eligibility validation: %#v", synth.calls)
	}
}

func TestActivationValidatesEveryArtifactBeforeMutation(t *testing.T) {
	engine := testEngine(t)
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	valid := Artifact{SchemaVersion: 1, WorkspaceID: "case-a", Layer: "L2", Period: "2026-W30", GeneratedAt: now, SourceFingerprint: strings.Repeat("1", 64), Sources: []SourceRef{{ID: "a", SHA256: strings.Repeat("a", 64)}}, SynthesizerID: "test-synth-v1", Content: "weekly"}
	invalid := Artifact{SchemaVersion: 1, WorkspaceID: "case-a", Layer: "L3", Period: "rolling", GeneratedAt: now, SourceFingerprint: strings.Repeat("2", 64), Sources: []SourceRef{{ID: "b", SHA256: strings.Repeat("b", 64)}}, SynthesizerID: "test-synth-v1", Content: ""}

	if err := engine.activate("case-a", []Artifact{valid, invalid}); err == nil {
		t.Fatal("expected staged validation failure")
	}
	if _, _, err := engine.readArtifactByKey("case-a", "L2/2026-W30"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("first artifact activated before full validation: %v", err)
	}
}

func TestActivationCrashPointsExposeOldOrCompleteCommitNeverPartial(t *testing.T) {
	points := []string{
		"after_stage_artifact:L2",
		"after_stage_artifact:L3",
		"after_stage_artifact:lifetime",
		"after_publish_version",
		"after_stage_manifest",
		"after_commit",
	}
	for _, point := range points {
		t.Run(point, func(t *testing.T) {
			engine := testEngine(t)
			workspace := "case-a"
			now := engine.Now()
			base := Artifact{SchemaVersion: 1, WorkspaceID: workspace, Layer: "L3", Period: "rolling", GeneratedAt: now.Add(-time.Hour), SourceFingerprint: strings.Repeat("0", 64), Sources: []SourceRef{{ID: "base", SHA256: strings.Repeat("a", 64)}}, SynthesizerID: "test-synth-v1", Content: "old L3"}
			if err := engine.activate(workspace, []Artifact{base}); err != nil {
				t.Fatal(err)
			}
			promotion := &PromotionRecord{EligibilityPolicy: "test-eligibility-v1", Eligible: true, Reason: "durable", EvaluatedAt: now}
			artifacts := []Artifact{
				{SchemaVersion: 1, WorkspaceID: workspace, Layer: "L2", Period: "2026-W30", GeneratedAt: now, SourceFingerprint: strings.Repeat("1", 64), Sources: []SourceRef{{ID: "l1", SHA256: strings.Repeat("b", 64)}}, SynthesizerID: "test-synth-v1", Content: "new L2", Promotion: promotion},
				{SchemaVersion: 1, WorkspaceID: workspace, Layer: "L3", Period: "rolling", GeneratedAt: now, SourceFingerprint: strings.Repeat("2", 64), Sources: []SourceRef{{ID: "l2", SHA256: strings.Repeat("c", 64)}}, SynthesizerID: "test-synth-v1", Content: "new L3"},
				{SchemaVersion: 1, WorkspaceID: workspace, Layer: "lifetime", Period: "lifetime", GeneratedAt: now, SourceFingerprint: strings.Repeat("3", 64), Sources: []SourceRef{{ID: "l3", SHA256: strings.Repeat("d", 64)}}, SynthesizerID: "test-synth-v1", Content: "new lifetime", Promotion: promotion},
			}
			engine.FaultPoint = func(candidate string) error {
				if candidate == point {
					return errors.New("simulated process stop")
				}
				return nil
			}
			if err := engine.activate(workspace, artifacts); err == nil {
				t.Fatal("expected injected activation failure")
			}
			engine.FaultPoint = nil

			l3, _, err := engine.readArtifactByKey(workspace, "L3/current")
			if err != nil {
				t.Fatal(err)
			}
			if point == "after_commit" {
				if l3.Content != "new L3" {
					t.Fatalf("committed L3 = %q", l3.Content)
				}
				if _, _, err := engine.readArtifactByKey(workspace, "L2/2026-W30"); err != nil {
					t.Fatalf("committed L2 missing: %v", err)
				}
				if _, _, err := engine.readArtifactByKey(workspace, "lifetime/current"); err != nil {
					t.Fatalf("committed lifetime missing: %v", err)
				}
				return
			}
			if l3.Content != "old L3" {
				t.Fatalf("pre-commit failure exposed L3 = %q", l3.Content)
			}
			if _, _, err := engine.readArtifactByKey(workspace, "L2/2026-W30"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("pre-commit failure exposed L2: %v", err)
			}
			if _, _, err := engine.readArtifactByKey(workspace, "lifetime/current"); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("pre-commit failure exposed lifetime: %v", err)
			}
		})
	}
}

func TestInvalidNewestManifestIsIgnoredAndCannotBecomeParent(t *testing.T) {
	engine := testEngine(t)
	workspace := "case-a"
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	first := Artifact{SchemaVersion: 1, WorkspaceID: workspace, Layer: "L3", Period: "rolling", GeneratedAt: now, SourceFingerprint: strings.Repeat("3", 64), Sources: []SourceRef{{ID: "c", SHA256: strings.Repeat("c", 64)}}, SynthesizerID: "test-synth-v1", Content: "first"}

	if err := engine.activate(workspace, []Artifact{first}); err != nil {
		t.Fatal(err)
	}
	_, validCommit, err := engine.latestManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	commitsRoot := filepath.Join(engine.workspaceRoot(workspace), "commits")
	invalid := CommitManifest{SchemaVersion: 1, WorkspaceID: workspace, TransactionID: "corrupt", CommittedAt: now.Add(time.Hour), Artifacts: map[string]string{"L3/current": "versions/missing/l3.json"}}
	if err := writeJSONFile(filepath.Join(commitsRoot, "99999999T999999.999999999Z-corrupt.json"), invalid); err != nil {
		t.Fatal(err)
	}
	second := first
	second.Content = "second"
	second.SourceFingerprint = strings.Repeat("4", 64)
	if err := engine.activate(workspace, []Artifact{second}); err != nil {
		t.Fatal(err)
	}
	manifest, _, err := engine.latestManifest(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.ParentCommit != validCommit {
		t.Fatalf("parent = %q, want last valid commit %q", manifest.ParentCommit, validCommit)
	}
	current, _, err := engine.readArtifactByKey(workspace, "L3/current")
	if err != nil || current.Content != "second" {
		t.Fatalf("current = %#v, err = %v", current, err)
	}
}

func TestAssembleContextUsesBroadToRecentOrderAndBudgets(t *testing.T) {
	engine := testEngine(t)
	workspace := "case-a"
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	artifacts := []Artifact{
		{SchemaVersion: 1, WorkspaceID: workspace, Layer: "L1", Period: "2026-07-20", GeneratedAt: now, SourceFingerprint: strings.Repeat("1", 64), Sources: []SourceRef{{ID: "a", SHA256: strings.Repeat("a", 64)}}, SynthesizerID: "test-synth-v1", Content: "recent-detail"},
		{SchemaVersion: 1, WorkspaceID: workspace, Layer: "L2", Period: "2026-W30", GeneratedAt: now, SourceFingerprint: strings.Repeat("2", 64), Sources: []SourceRef{{ID: "b", SHA256: strings.Repeat("b", 64)}}, SynthesizerID: "test-synth-v1", Content: "weekly-detail"},
		{SchemaVersion: 1, WorkspaceID: workspace, Layer: "L3", Period: "rolling", GeneratedAt: now, SourceFingerprint: strings.Repeat("3", 64), Sources: []SourceRef{{ID: "c", SHA256: strings.Repeat("c", 64)}}, SynthesizerID: "test-synth-v1", Content: "medium-detail"},
		{SchemaVersion: 1, WorkspaceID: workspace, Layer: "lifetime", Period: "lifetime", GeneratedAt: now, SourceFingerprint: strings.Repeat("4", 64), Sources: []SourceRef{{ID: "d", SHA256: strings.Repeat("d", 64)}}, SynthesizerID: "test-synth-v1", Content: "durable-detail", Promotion: &PromotionRecord{EligibilityPolicy: "test-eligibility-v1", Eligible: true, Reason: "eligible", EvaluatedAt: now}},
	}
	if err := engine.activate(workspace, artifacts); err != nil {
		t.Fatal(err)
	}
	engine.Budgets["L1"] = 6

	bundle, err := engine.AssembleContext(workspace)
	if err != nil {
		t.Fatal(err)
	}
	gotLayers := make([]string, 0, len(bundle.Sections))
	for _, section := range bundle.Sections {
		gotLayers = append(gotLayers, section.Layer)
	}
	if !reflect.DeepEqual(gotLayers, []string{"lifetime", "L3", "L2", "L1"}) {
		t.Fatalf("context order = %v", gotLayers)
	}
	if bundle.Sections[3].Content != "recent" || !bundle.Sections[3].Truncated {
		t.Fatalf("bounded L1 section = %#v", bundle.Sections[3])
	}
}

func TestAssembleContextPinsOneManifestAcrossConcurrentCommit(t *testing.T) {
	engine := testEngine(t)
	workspace := "case-a"
	oldArtifacts := contextSnapshotArtifacts(workspace, engine.Now(), "old", "5")
	newArtifacts := contextSnapshotArtifacts(workspace, engine.Now().Add(time.Minute), "new", "6")
	if err := engine.activate(workspace, oldArtifacts); err != nil {
		t.Fatal(err)
	}
	engine.contextReadPoint = func(point string) error {
		if point != "after_snapshot" {
			return nil
		}
		engine.contextReadPoint = nil
		return engine.activate(workspace, newArtifacts)
	}

	bundle, err := engine.AssembleContext(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Sections) != 4 {
		t.Fatalf("snapshot sections = %#v", bundle.Sections)
	}
	for _, section := range bundle.Sections {
		if !strings.HasPrefix(section.Content, "old-") {
			t.Fatalf("mixed context snapshot contains %s=%q", section.Layer, section.Content)
		}
	}

	latest, err := engine.AssembleContext(workspace)
	if err != nil {
		t.Fatal(err)
	}
	for _, section := range latest.Sections {
		if !strings.HasPrefix(section.Content, "new-") {
			t.Fatalf("latest context contains %s=%q", section.Layer, section.Content)
		}
	}
}

func TestAssembleContextSkipsConfiguredStaleLayer(t *testing.T) {
	engine := testEngine(t)
	workspace := "case-a"
	now := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	artifact := Artifact{SchemaVersion: 1, WorkspaceID: workspace, Layer: "L1", Period: "2026-07-18", GeneratedAt: now.Add(-48 * time.Hour), SourceFingerprint: strings.Repeat("1", 64), Sources: []SourceRef{{ID: "a", SHA256: strings.Repeat("a", 64)}}, SynthesizerID: "test-synth-v1", Content: "stale-detail"}
	if err := engine.activate(workspace, []Artifact{artifact}); err != nil {
		t.Fatal(err)
	}
	engine.Freshness = map[string]time.Duration{"L1": 24 * time.Hour}

	bundle, err := engine.AssembleContext(workspace)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundle.Sections) != 0 || !containsString(bundle.Diagnostics, "L1: stale; skipped") {
		t.Fatalf("context bundle = %#v", bundle)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func contextSnapshotArtifacts(workspace string, generatedAt time.Time, label, fingerprintDigit string) []Artifact {
	promotion := &PromotionRecord{EligibilityPolicy: "test-eligibility-v1", Eligible: true, Reason: "eligible", EvaluatedAt: generatedAt}
	layers := []struct {
		layer  string
		period string
	}{
		{layer: "L1", period: "2026-07-20"},
		{layer: "L2", period: "2026-W30"},
		{layer: "L3", period: "rolling"},
		{layer: "lifetime", period: "lifetime"},
	}
	artifacts := make([]Artifact, 0, len(layers))
	for index, layer := range layers {
		artifact := Artifact{
			SchemaVersion:     1,
			WorkspaceID:       workspace,
			Layer:             layer.layer,
			Period:            layer.period,
			GeneratedAt:       generatedAt,
			SourceFingerprint: strings.Repeat(fingerprintDigit, 63) + string(rune('0'+index)),
			Sources:           []SourceRef{{ID: label + "-source", SHA256: strings.Repeat(fingerprintDigit, 64)}},
			SynthesizerID:     "test-synth-v1",
			Content:           label + "-" + layer.layer,
		}
		if layer.layer == "lifetime" {
			artifact.Promotion = promotion
		}
		artifacts = append(artifacts, artifact)
	}
	return artifacts
}

func testEngine(t *testing.T) *Engine {
	t.Helper()
	policy := validPolicyForTest()
	return &Engine{
		Root:                t.TempDir(),
		Policy:              policy,
		SynthesizerID:       "test-synth-v1",
		EligibilityPolicyID: "test-eligibility-v1",
		Budgets: map[string]int{
			"L1": 1024, "L2": 1024, "L3": 1024, "lifetime": 1024,
		},
		Now: func() time.Time { return time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC) },
	}
}

func seedDailyDreams(t *testing.T, engine *Engine, workspace string, week time.Time) {
	t.Helper()
	monday := startOfISOWeek(week)
	for offset := 0; offset < 2; offset++ {
		day := monday.AddDate(0, 0, offset)
		if _, err := engine.Capture(Capture{WorkspaceID: workspace, RecordedAt: day, Kind: "thread", Text: "evidence", Sanitized: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := engine.DreamDaily(context.Background(), workspace, day); err != nil {
			t.Fatal(err)
		}
	}
}

type fakeSynthesizer struct {
	outputs   map[string]string
	failLayer string
	calls     map[string]int
}

func (fake *fakeSynthesizer) Synthesize(_ context.Context, request SynthesisRequest) (string, error) {
	if fake.calls == nil {
		fake.calls = make(map[string]int)
	}
	fake.calls[request.TargetLayer]++
	if request.TargetLayer == fake.failLayer {
		return "", errors.New("synthetic failure")
	}
	return fake.outputs[request.TargetLayer], nil
}
