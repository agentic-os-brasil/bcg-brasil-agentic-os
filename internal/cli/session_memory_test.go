package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

func TestSessionMemorySourceLoadsOnlyCommittedBoundedLocalMemory(t *testing.T) {
	root := t.TempDir()
	workspaceID := "case-a"
	now := time.Date(2026, 8, 4, 9, 0, 0, 0, time.UTC)
	policy, err := basememory.Policy()
	if err != nil {
		t.Fatal(err)
	}
	config, err := basememory.Runtime()
	if err != nil {
		t.Fatal(err)
	}
	memoryRoot := filepath.Join(root, "memory")
	attestor := memory.CaptureAttestor{Root: memoryRoot}
	capture, err := attestor.Seal(memory.Capture{WorkspaceID: workspaceID, RecordedAt: now, Kind: "skill_route", Text: "bcg-case-kickoff", Sanitized: true, ProducerID: "claude.context-injection", SanitizerID: memory.SkillRouteSanitizerID, SourceDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"})
	if err != nil {
		t.Fatal(err)
	}
	engine := memory.Engine{Root: memoryRoot, Policy: policy, Budgets: config.ContextBudgets(), MaxSourceBytes: config.L1MaxInputBytes, Synthesizer: memory.DeterministicL1Synthesizer{MaxRunes: config.L1MaxRunes, MaxEntries: config.L1MaxEntries, MaxInputBytes: config.L1MaxInputBytes, MaxInputEntries: config.L1MaxInputEntries, Attestor: attestor}, SynthesizerID: memory.DeterministicL1SynthesizerID, Now: func() time.Time { return now }}
	if _, err := engine.Capture(capture); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.DreamDailyAttested(context.Background(), workspaceID, now); err != nil {
		t.Fatal(err)
	}

	source := sessionMemorySource(root, workspaceID)
	if source.State != "available" || len(source.Bundle.Sections) != 1 || source.Bundle.Sections[0].Layer != "L1" || !strings.Contains(source.Bundle.Sections[0].Content, "bcg-case-kickoff") {
		t.Fatalf("session memory source = %#v", source)
	}
}

func TestSessionMemorySourceDistinguishesEmptyFromCorrupt(t *testing.T) {
	root := t.TempDir()
	if source := sessionMemorySource(root, "case-a"); source.State != "empty" || len(source.Bundle.Sections) != 0 {
		t.Fatalf("empty memory source = %#v", source)
	}
	commits := filepath.Join(root, "memory", "workspaces", "case-a", "commits")
	if err := os.MkdirAll(commits, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commits, "corrupt.json"), []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if source := sessionMemorySource(root, "case-a"); source.State != "unavailable" || len(source.Bundle.Sections) != 0 {
		t.Fatalf("corrupt memory source = %#v", source)
	}
}
