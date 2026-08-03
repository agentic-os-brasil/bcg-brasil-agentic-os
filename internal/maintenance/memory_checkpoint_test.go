package maintenance

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestContinuityCheckpointCrashPreservesLastKnownGood(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	root := t.TempDir()
	store := ContinuityCheckpointStore{Root: root}
	firstSources := []scheduler.Receipt{{JobID: "darwin-housekeeping-daily", ScheduledFor: now.Add(-time.Hour), AttemptedAt: now.Add(-30 * time.Minute), State: scheduler.Succeeded}}
	first, changed, err := store.CommitSchedulerReceipts("maestro-system", firstSources, now)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("initial checkpoint was reported unchanged")
	}
	store.FaultPoint = func(point string) error {
		if point == "after_version" {
			return errors.New("simulated crash")
		}
		return nil
	}
	secondSources := append(firstSources, scheduler.Receipt{JobID: "darwin-deep-weekly", ScheduledFor: now, AttemptedAt: now.Add(time.Minute), State: scheduler.Failed, Error: "must-not-enter-watermark"})
	if _, _, err := store.CommitSchedulerReceipts("maestro-system", secondSources, now.Add(2*time.Minute)); err == nil {
		t.Fatal("checkpoint commit crossed the injected crash")
	}
	current, err := store.Load("maestro-system")
	if err != nil || !reflect.DeepEqual(current, first) {
		t.Fatalf("last-known-good changed: current=%#v first=%#v err=%v", current, first, err)
	}
}

func TestContinuityCheckpointRejectsSymlinkedWorkspacePath(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("symlink setup is host-dependent on Windows")
	}
	root, outside := t.TempDir(), t.TempDir()
	workspaces := filepath.Join(root, "workspaces")
	if err := os.Mkdir(workspaces, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(workspaces, "maestro-system")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	_, _, err := (ContinuityCheckpointStore{Root: root}).CommitSchedulerReceipts("maestro-system", []scheduler.Receipt{{JobID: "darwin-housekeeping-daily", ScheduledFor: now.Add(-time.Hour), AttemptedAt: now, State: scheduler.Succeeded}}, now)
	if err == nil {
		t.Fatal("checkpoint traversed a symlinked workspace path")
	}
	if entries, readErr := os.ReadDir(outside); readErr != nil || len(entries) != 0 {
		t.Fatalf("checkpoint modified symlink target: entries=%v err=%v", entries, readErr)
	}
}

func TestPublishedMemoryCheckpointSchemaMatchesCommittedState(t *testing.T) {
	now := time.Date(2026, 8, 2, 12, 0, 0, 0, time.UTC)
	checkpoint, _, err := (ContinuityCheckpointStore{Root: t.TempDir()}).CommitSchedulerReceipts("maestro-system", []scheduler.Receipt{{JobID: "darwin-housekeeping-daily", ScheduledFor: now.Add(-time.Hour), AttemptedAt: now, State: scheduler.Succeeded}}, now)
	if err != nil {
		t.Fatal(err)
	}
	if err := validatePublishedSchemaDocument(t, "memory-checkpoint.schema.json", checkpoint); err != nil {
		t.Fatalf("memory checkpoint does not satisfy published schema: %v", err)
	}
}
