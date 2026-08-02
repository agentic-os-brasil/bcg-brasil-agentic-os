package darwin

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestManagedStateRepairChangesAndValidatesActualState(t *testing.T) {
	root := t.TempDir()
	seedManagedState(t, root, managedState{SchemaVersion: SchemaVersion, State: "stale", Generation: 4})
	invoker := ManagedStateRepairInvoker{Root: root}
	artifact := Artifact{SchemaVersion: SchemaVersion, AgentID: AgentID, WindowID: "window-repair", ProposalID: "proposal-repair", Finding: ObservationStateStale, Action: ActionRefreshDerivedState}

	result, err := invoker.Invoke(context.Background(), managedStateRepairCall(), artifact)
	if err != nil || result.Outcome != OutcomeSucceeded {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	state, err := readManagedState(root)
	if err != nil || state.State != "healthy" || state.Generation != 5 || state.WindowID != artifact.WindowID || state.ProposalID != artifact.ProposalID {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

func TestManagedStateRepairRollsBackWhenPostValidationFails(t *testing.T) {
	root := t.TempDir()
	original := managedState{SchemaVersion: SchemaVersion, State: "stale", Generation: 2}
	seedManagedState(t, root, original)
	invoker := ManagedStateRepairInvoker{Root: root, PostValidate: func(managedState) error { return errors.New("validation failed") }}
	artifact := Artifact{SchemaVersion: SchemaVersion, AgentID: AgentID, WindowID: "window-rollback", ProposalID: "proposal-rollback", Finding: ObservationStateStale, Action: ActionRefreshDerivedState}

	if _, err := invoker.Invoke(context.Background(), managedStateRepairCall(), artifact); err == nil {
		t.Fatal("post-validation failure must fail the repair")
	}
	state, err := readManagedState(root)
	if err != nil || state != original {
		t.Fatalf("rollback state=%#v err=%v, want %#v", state, err, original)
	}
}

func TestDiagnosticArtifactCannotClaimAppliedRepair(t *testing.T) {
	root := t.TempDir()
	invoker := FilesystemInvoker{Root: root}
	artifact := Artifact{SchemaVersion: SchemaVersion, AgentID: AgentID, WindowID: "window-diagnostic", ProposalID: "proposal-diagnostic", Finding: ObservationSchedulerMissed, Action: ActionReconcileScheduler}
	result, err := invoker.Invoke(context.Background(), ToolCall{Tool: "filesystem", Operation: "write", Resource: "bcgos://health/maestro-system/derived/diagnostic.json"}, artifact)
	if err != nil || result.Outcome != OutcomeNoAction {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	result, err = invoker.Invoke(context.Background(), ToolCall{Tool: "filesystem", Operation: "write", Resource: "bcgos://health/maestro-system/derived/diagnostic.json"}, artifact)
	if err != nil || result.Outcome != OutcomeNoAction {
		t.Fatalf("replayed result=%#v err=%v", result, err)
	}
}

func TestOperationalRepairAuthorizesTheMutatedResource(t *testing.T) {
	artifact := Artifact{SchemaVersion: SchemaVersion, AgentID: AgentID, WindowID: "window-guard", ProposalID: "proposal-guard", Finding: ObservationStateStale, Action: ActionRefreshDerivedState}
	invoker := OperationalInvoker{Repairs: ManagedStateRepairInvoker{Root: t.TempDir()}, Guard: ToolGuardFunc(func(call ToolCall) error {
		if call == managedStateRepairCall() {
			return errors.New("managed state denied")
		}
		return nil
	})}
	call := ToolCall{Tool: "filesystem", Operation: "write", Resource: "bcgos://health/maestro-system/derived/refresh_derived_state-proposal-guard.json"}
	if _, err := invoker.Invoke(context.Background(), call, artifact); err == nil {
		t.Fatal("operational repair bypassed authorization for the mutated resource")
	}
}

func TestManagedStateRepairSerializesConcurrentJobs(t *testing.T) {
	root := t.TempDir()
	seedManagedState(t, root, managedState{SchemaVersion: SchemaVersion, State: "stale", Generation: 1})
	lockPath := filepath.Join(root, "managed-state", "repair.lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := tryLockManagedStateFile(file); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = unlockManagedStateFile(file)
		_ = file.Close()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	artifact := Artifact{SchemaVersion: SchemaVersion, AgentID: AgentID, WindowID: "window-serialized", ProposalID: "proposal-serialized", Finding: ObservationStateStale, Action: ActionRefreshDerivedState}
	if _, err := (ManagedStateRepairInvoker{Root: root}).Invoke(ctx, managedStateRepairCall(), artifact); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("repair did not respect the shared state lock: %v", err)
	}
}

func TestDailyDiagnosticOnlyReturnsReviewedNoChange(t *testing.T) {
	now := time.Now().UTC()
	root := t.TempDir()
	handler := HousekeepingHandler{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-daily-diagnostic", Runtime: "runtime-neutral", Observations: []Observation{{Code: ObservationSchedulerMissed, Severity: SeverityHigh, Count: 1, State: "warning"}}}, nil
		}),
		Guard: ToolGuardFunc(func(ToolCall) error { return nil }), Invoker: FilesystemInvoker{Root: root},
		Store: Store{Root: root}, CommandStore: maintenance.Store{Root: t.TempDir()}, Now: func() time.Time { return now },
	}
	command := maintenance.Command{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: "daily-diagnostic", JobID: HousekeepingJobID, WorkspaceID: MaintenanceScope, Trigger: maintenance.TriggerDaily, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute)}

	result, err := handler.Execute(context.Background(), command)
	if err != nil || result.State != maintenance.ReceiptReviewedNoChange {
		t.Fatalf("result=%#v err=%v", result, err)
	}
}

func TestDeepReviewExecutesValidatedOperationalRepair(t *testing.T) {
	now := time.Now().UTC()
	root := t.TempDir()
	seedManagedState(t, root, managedState{SchemaVersion: SchemaVersion, State: "stale", Generation: 1})
	commandStore := maintenance.Store{Root: t.TempDir()}
	handler := DeepReviewHandler{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-weekly-repair", Runtime: "runtime-neutral", Mode: DeepReview, Observations: []Observation{{Code: ObservationStateStale, Severity: SeverityMedium, Count: 1, State: "stale"}}}, nil
		}),
		Guard: ToolGuardFunc(func(ToolCall) error { return nil }),
		Invoker: OperationalInvoker{
			Diagnostics: FilesystemInvoker{Root: root},
			Repairs:     ManagedStateRepairInvoker{Root: root},
			Guard:       ToolGuardFunc(func(ToolCall) error { return nil }),
		},
		Store: Store{Root: root}, CommandStore: commandStore, ProposalStore: ProposalStore{Root: t.TempDir()}, Now: func() time.Time { return now },
	}
	command := maintenance.Command{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: "weekly-repair", JobID: "darwin-deep-weekly", WorkspaceID: MaintenanceScope, Trigger: maintenance.TriggerWeekly, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute)}

	result, err := handler.Execute(context.Background(), command)
	if err != nil || result.State != maintenance.ReceiptSucceeded {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	state, err := readManagedState(root)
	if err != nil || state.State != "healthy" {
		t.Fatalf("state=%#v err=%v", state, err)
	}
}

func seedManagedState(t *testing.T, root string, state managedState) {
	t.Helper()
	directory := filepath.Join(root, "managed-state")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	body, err := encodeManagedState(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "current.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
}
