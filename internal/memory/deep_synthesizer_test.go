package memory

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDeterministicWeeklySynthesizerPromotesOnlyAfterTwoL3Generations(t *testing.T) {
	// A repeated weekly cycle carries prior continuity plus a fresh L2 that
	// overlaps it. Keep whole observations, not recursively nested summaries.
	oldUnit := "- 09:00:00Z | reviewed | OLD_OBSERVATION " + strings.Repeat("continuidade útil ", 10) + "OLD_COMPLETE_END."
	newUnit := "- 10:00:00Z | reviewed | NEW_OBSERVATION " + strings.Repeat("informação válida ", 10) + "NEW_COMPLETE_LESSON_END."
	source := func(layer, period, body string) SourceDocument {
		return artifactSource(Artifact{SchemaVersion: 1, WorkspaceID: "case-a", Layer: layer, Period: period,
			GeneratedAt: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC), SourceFingerprint: strings.Repeat("a", 64),
			Sources: []SourceRef{{ID: "L1/2026-10-19", SHA256: strings.Repeat("b", 64)}}, SynthesizerID: "deterministic-weekly-v1", Content: body})
	}
	previous := source("L3", "rolling", "# L3 rolling continuity · 2026-W42\n\n## source L2/2026-W42\n# L2 weekly continuity · 2026-W42\n\n## source L1/2026-10-19\n# L1 continuity · 2026-10-19\n\n"+oldUnit+"\n\n- OLD_INCOMPLETE_FRAGMENT clipping…")
	current := source("L2", "2026-W43", "# L2 weekly continuity · 2026-W43\n\n## source L1/2026-10-19\n# L1 continuity · 2026-10-19\n\n"+oldUnit+"\n"+newUnit)
	rollup := DeterministicWeeklySynthesizer{MaxRunes: map[string]int{"L3": 768}, MaxInputBytes: 4096}
	content, err := rollup.Synthesize(context.Background(), SynthesisRequest{Cycle: "weekly", TargetLayer: "L3", WorkspaceID: "case-a", Period: "2026-W43", Sources: []SourceDocument{previous, current}})
	if err != nil || strings.Count(content, "OLD_OBSERVATION") != 1 || !strings.Contains(content, newUnit) || strings.Contains(content, "## source") || strings.Contains(content, "OLD_INCOMPLETE_FRAGMENT") {
		t.Fatalf("weekly continuity recursively duplicated history or clipped new observation: %q error=%v", content, err)
	}
	rollup.MaxBytes = map[string]int{"L3": 768}
	if len(content) > 768 {
		t.Fatal("complete observations exceeded the byte budget")
	}
	ownerText := "# Owner-approved observations\n\nFirst paragraph line.\nContinuation stays with this paragraph.\n\n- Complete bullet.\n  Indented continuation.\n\n```markdown\n# L1 continuity · 2026-10-19\n## source L2/2026-W43\n```"
	ownerSource := source("L2", "2026-W43", "# L2 weekly continuity · 2026-W43\n\n"+ownerText)
	owned, err := rollup.Synthesize(context.Background(), SynthesisRequest{Cycle: "weekly", TargetLayer: "L3", WorkspaceID: "case-a", Period: "2026-W43", Sources: []SourceDocument{ownerSource}})
	if err != nil || !strings.Contains(owned, "# Owner-approved observations\n\nFirst paragraph line.\nContinuation stays with this paragraph.") || !strings.Contains(owned, "- Complete bullet.\n  Indented continuation.") || !strings.Contains(owned, "```markdown\n# L1 continuity · 2026-10-19\n## source L2/2026-W43\n```") {
		t.Fatalf("complete authored units or fenced framing were rewritten: %q error=%v", owned, err)
	}
	for _, fence := range []struct{ opener, literal string }{
		{"````", "```"}, {"````", "````not-a-close"}, {"~~~~", "~~~not-a-close"},
	} {
		block := fence.opener + "markdown\nA complete opening illustration.\n" + fence.literal + "\n\n" + strings.Repeat("complete tail ", 24) + "\n" + fence.opener + "\n"
		fenced := source("L2", "2026-W43", "# L2 weekly continuity · 2026-W43\n\n"+block)
		rollup.MaxBytes["L3"] = 768
		whole, err := rollup.Synthesize(context.Background(), SynthesisRequest{Cycle: "weekly", TargetLayer: "L3", WorkspaceID: "case-a", Period: "2026-W43", Sources: []SourceDocument{fenced}})
		if err != nil || !strings.Contains(whole, strings.TrimSpace(block)) {
			t.Fatalf("valid long fence was split: %q error=%v", whole, err)
		}
		rollup.MaxBytes["L3"] = 180
		if partial, err := rollup.Synthesize(context.Background(), SynthesisRequest{Cycle: "weekly", TargetLayer: "L3", WorkspaceID: "case-a", Period: "2026-W43", Sources: []SourceDocument{fenced}}); err == nil || partial != "" {
			t.Fatalf("tight budget published a partial fenced example after %q: %q error=%v", fence.literal, partial, err)
		}
	}
	rollup.MaxBytes["L3"] = 768
	oversized := source("L2", "2026-W43", "# L2 weekly continuity · 2026-W43\n\n- "+strings.Repeat("é", 600)+" COMPLETE.")
	if partial, err := rollup.Synthesize(context.Background(), SynthesisRequest{Cycle: "weekly", TargetLayer: "L3", WorkspaceID: "case-a", Period: "2026-W43", Sources: []SourceDocument{previous, oversized}}); err == nil || partial != "" {
		t.Fatal("oversized new observation silently became clipped or old-only success")
	}
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
	seed := func(day time.Time) {
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
	seed(first)
	firstResult, err := engine.DreamWeekly(context.Background(), "case-a", first)
	if err != nil || len(firstResult.ActivatedLayers) != 2 || firstResult.LifetimeReason == "" {
		t.Fatalf("first weekly result=%#v err=%v", firstResult, err)
	}
	if _, _, err := engine.readArtifactByKey("case-a", "lifetime/current"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lifetime promoted before continuity was established: %v", err)
	}
	unchanged, err := engine.DreamWeekly(context.Background(), "case-a", first)
	if err != nil || !unchanged.Skipped || len(unchanged.ActivatedLayers) != 0 || unchanged.SourceFingerprint != firstResult.SourceFingerprint || unchanged.LifetimeReason != firstResult.LifetimeReason {
		t.Fatalf("unchanged evidence minted another continuity generation: %#v error=%v", unchanged, err)
	}
	seed(first.AddDate(0, 0, 1))
	secondResult, err := engine.DreamWeekly(context.Background(), "case-a", first.AddDate(0, 0, 1))
	if err != nil || len(secondResult.ActivatedLayers) != 3 || secondResult.ActivatedLayers[2] != "lifetime" {
		t.Fatalf("second weekly result=%#v err=%v", secondResult, err)
	}
}
