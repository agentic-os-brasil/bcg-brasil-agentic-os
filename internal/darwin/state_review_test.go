package darwin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestReviewStateDocumentsReportsOnlyBoundedAggregate(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "maestro", "states.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("- retained control-plane item\n", maxStateDocumentLines+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	surface := reviewStateDocuments(root)
	if surface != (ProductSurface{State: "warning", Count: 1}) {
		t.Fatalf("state document surface = %#v", surface)
	}
	packet, err := BuildHealthPacket(HealthRequest{SchemaVersion: SchemaVersion, WindowID: "state-documents", Runtime: "runtime-neutral", Mode: DeepReview, Surfaces: HealthSurfaces{StateDocuments: surface}})
	if err != nil || len(packet.Observations) != 1 || packet.Observations[0].Code != ObservationStateDocumentsOversized || packet.Observations[0].Count != 1 {
		t.Fatalf("packet=%#v err=%v", packet, err)
	}
	assessment, err := Plan(packet)
	if err != nil || len(assessment.Proposals) != 1 || assessment.Proposals[0].Action != ActionReviewStateDocuments || isExecutableRepair(assessment.Proposals[0]) {
		t.Fatalf("assessment=%#v err=%v", assessment, err)
	}
}

func TestReviewStateDocumentsRejectsUnsafeOrUnboundedRoot(t *testing.T) {
	root := t.TempDir()
	for index := 0; index <= maxReviewedStateDocuments; index++ {
		if err := os.Mkdir(filepath.Join(root, "agent-"+string(rune('a'+index))), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if surface := reviewStateDocuments(root); surface != (ProductSurface{State: "failed", Count: 1}) {
		t.Fatalf("unbounded root surface=%#v", surface)
	}
	if surface := reviewStateDocuments(filepath.Join(root, "missing")); surface != (ProductSurface{State: "healthy"}) {
		t.Fatalf("missing root surface=%#v", surface)
	}
}

func TestLocalProductHealthBuilderReviewsStateDocumentsOnlyWeekly(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "maestro", "states.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Repeat("x\n", maxStateDocumentLines+1)), 0o600); err != nil {
		t.Fatal(err)
	}
	builder := LocalProductHealthBuilder{Scheduler: scheduler.Store{Root: t.TempDir()}, Workspace: MaintenanceScope, Runtime: "runtime-neutral", StateDocumentsRoot: root}
	now := time.Date(2026, 8, 2, 4, 0, 0, 0, time.UTC)
	weekly, err := builder.Build(context.Background(), scheduler.Occurrence{JobID: "darwin-deep-weekly", ScheduledFor: now})
	if err != nil || len(weekly.Observations) != 1 || weekly.Observations[0].Code != ObservationStateDocumentsOversized {
		t.Fatalf("weekly=%#v err=%v", weekly, err)
	}
	daily, err := builder.Build(context.Background(), scheduler.Occurrence{JobID: HousekeepingJobID, ScheduledFor: now})
	if err != nil || len(daily.Observations) != 0 {
		t.Fatalf("daily=%#v err=%v", daily, err)
	}
}
