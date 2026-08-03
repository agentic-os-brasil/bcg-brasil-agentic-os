package maintenance

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

func TestMemoryLightDreamHandlerCommitsOnlyDailyL1(t *testing.T) {
	policy, err := basememory.Policy()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "memory")
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	attestor := memory.CaptureAttestor{Root: root}
	engine := &memory.Engine{Root: root, Policy: policy, Budgets: map[string]int{"L1": 1000, "L2": 1, "L3": 1, "lifetime": 1}, MaxSourceBytes: 4096, Synthesizer: memory.DeterministicL1Synthesizer{MaxRunes: 1000, MaxEntries: 8, MaxInputBytes: 4096, MaxInputEntries: 16, Attestor: attestor}, SynthesizerID: memory.DeterministicL1SynthesizerID, Now: func() time.Time { return now }}
	capture, err := attestor.Seal(memory.Capture{WorkspaceID: "maestro-system", RecordedAt: now, Kind: "skill_route", Text: "meeting-close", Sanitized: true, ProducerID: "claude.context-injection", SanitizerID: memory.SkillRouteSanitizerID, SourceDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := engine.Capture(capture); err != nil {
		t.Fatal(err)
	}
	command := Command{SchemaVersion: CommandSchemaVersion, CommandID: "memory-light-dream-1", JobID: MemoryLightDreamJobID, WorkspaceID: "maestro-system", Trigger: TriggerPresence, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute)}
	grant, err := newExecutionGrant(command)
	if err != nil {
		t.Fatal(err)
	}
	result, err := (MemoryLightDreamHandler{Engine: engine}).ExecuteAuthorized(context.Background(), command, grant)
	if err != nil || result.State != ReceiptSucceeded {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	status, err := engine.Status("maestro-system")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != "ready" || len(status.Layers) != 1 || status.Layers["L1"] != "2026-08-03" {
		t.Fatalf("unexpected memory status: %#v", status)
	}
}

func TestMemoryLightDreamHandlerTreatsMissingCaptureAsNoChange(t *testing.T) {
	policy, err := basememory.Policy()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 3, 12, 0, 0, 0, time.UTC)
	root := filepath.Join(t.TempDir(), "memory")
	engine := &memory.Engine{Root: root, Policy: policy, Budgets: map[string]int{"L1": 1000, "L2": 1, "L3": 1, "lifetime": 1}, MaxSourceBytes: 4096, Synthesizer: memory.DeterministicL1Synthesizer{MaxRunes: 1000, MaxEntries: 8, MaxInputBytes: 4096, MaxInputEntries: 16, Attestor: memory.CaptureAttestor{Root: root}}, SynthesizerID: memory.DeterministicL1SynthesizerID, Now: func() time.Time { return now }}
	command := Command{SchemaVersion: CommandSchemaVersion, CommandID: "memory-light-dream-2", JobID: MemoryLightDreamJobID, WorkspaceID: "maestro-system", Trigger: TriggerPresence, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute)}
	grant, _ := newExecutionGrant(command)
	result, err := (MemoryLightDreamHandler{Engine: engine}).ExecuteAuthorized(context.Background(), command, grant)
	if err != nil || result.State != ReceiptReviewedNoChange {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}
