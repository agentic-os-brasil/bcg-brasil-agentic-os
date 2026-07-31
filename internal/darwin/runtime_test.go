package darwin

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func commandForTest(now time.Time, jobID string, proposalOnly bool) maintenance.Command {
	return maintenance.Command{
		SchemaVersion: maintenance.CommandSchemaVersion,
		CommandID:     "command-" + jobID,
		JobID:         jobID,
		WorkspaceID:   "workspace-1",
		Trigger:       maintenance.TriggerDaily,
		ScheduledFor:  now,
		RequestedAt:   now,
		Deadline:      now.Add(2 * time.Second),
		ProposalOnly:  proposalOnly,
	}
}

func TestExecuteCommandUsesLeaseAndPersistsBoundedReceipt(t *testing.T) {
	now := time.Now().UTC()
	builds := 0
	executor := HousekeepingExecutor{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			builds++
			return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-command", Runtime: "claude", Observations: []Observation{{Code: ObservationStateStale, Severity: SeverityLow, Count: 1}}}, nil
		}),
		Guard: ToolGuardFunc(func(ToolCall) error { return nil }),
		Invoker: InvokerFunc(func(context.Context, ToolCall, Artifact) (ToolResult, error) {
			return ToolResult{Outcome: OutcomeSucceeded}, nil
		}),
		Store:        Store{Root: t.TempDir()},
		CommandStore: maintenance.Store{Root: t.TempDir()},
		Scheduler:    scheduler.Store{Root: t.TempDir()},
		Now:          func() time.Time { return now },
	}
	command := commandForTest(now, HousekeepingJobID, false)
	receipt, err := executor.ExecuteCommand(context.Background(), command)
	if err != nil || receipt.State != maintenance.ReceiptSucceeded {
		t.Fatalf("receipt=%#v err=%v", receipt, err)
	}
	stored, err := executor.CommandStore.Receipts(command.WorkspaceID, command.JobID)
	if err != nil || len(stored) != 1 || stored[0].State != maintenance.ReceiptSucceeded {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
	if _, err := executor.Scheduler.TryAcquireLease(command.WorkspaceID, command.JobID, command.CommandID, "probe", now, time.Minute); err != nil {
		t.Fatalf("lease was not released: %v", err)
	}
	duplicate, err := executor.ExecuteCommand(context.Background(), command)
	if err != nil || duplicate.CommandID != command.CommandID || builds != 1 {
		t.Fatalf("duplicate receipt=%#v err=%v builds=%d", duplicate, err, builds)
	}
}

func TestExecuteCommandEmitsProposalWithoutInvokingTools(t *testing.T) {
	now := time.Now().UTC()
	called := false
	executor := HousekeepingExecutor{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-proposal", Runtime: "codex", Observations: []Observation{{Code: ObservationContractDrift, Severity: SeverityHigh, Count: 1}}}, nil
		}),
		Guard: ToolGuardFunc(func(ToolCall) error { called = true; return nil }),
		Invoker: InvokerFunc(func(context.Context, ToolCall, Artifact) (ToolResult, error) {
			called = true
			return ToolResult{Outcome: OutcomeSucceeded}, nil
		}),
		Store:        Store{Root: t.TempDir()},
		CommandStore: maintenance.Store{Root: t.TempDir()},
		Scheduler:    scheduler.Store{Root: t.TempDir()},
		Now:          func() time.Time { return now },
	}
	command := commandForTest(now, "darwin-structural-evolution-proposal", true)
	command.Trigger = maintenance.TriggerMonthly
	receipt, err := executor.ExecuteCommand(context.Background(), command)
	if err != nil || receipt.State != maintenance.ReceiptProposalEmitted || receipt.ProposalCount != 1 || len(receipt.ProposalDigest) != 64 || called {
		t.Fatalf("receipt=%#v err=%v called=%v", receipt, err, called)
	}
}

func TestExecuteCommandReturnsBusyWithoutWaiting(t *testing.T) {
	now := time.Now().UTC()
	schedulerRoot := t.TempDir()
	store := scheduler.Store{Root: schedulerRoot}
	command := commandForTest(now, HousekeepingJobID, false)
	if _, err := store.TryAcquireLease(command.WorkspaceID, command.JobID, command.CommandID, "other", now, time.Minute); err != nil {
		t.Fatal(err)
	}
	executor := HousekeepingExecutor{Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
		return HealthPacket{}, errors.New("must not build")
	}), Store: Store{Root: t.TempDir()}, CommandStore: maintenance.Store{Root: t.TempDir()}, Scheduler: store, Now: func() time.Time { return now }}
	receipt, err := executor.ExecuteCommand(context.Background(), command)
	if !errors.Is(err, scheduler.ErrLeaseBusy) || receipt.State != maintenance.ReceiptBusy {
		t.Fatalf("receipt=%#v err=%v", receipt, err)
	}
}

func TestExecuteCommandTimesOutAtExplicitDeadline(t *testing.T) {
	now := time.Now().UTC()
	executor := HousekeepingExecutor{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-timeout", Runtime: "claude", Observations: []Observation{{Code: ObservationStateStale, Severity: SeverityLow, Count: 1}}}, nil
		}),
		Guard: ToolGuardFunc(func(ToolCall) error { return nil }),
		Invoker: InvokerFunc(func(ctx context.Context, _ ToolCall, _ Artifact) (ToolResult, error) {
			<-ctx.Done()
			return ToolResult{}, ctx.Err()
		}),
		Store:        Store{Root: t.TempDir()},
		CommandStore: maintenance.Store{Root: t.TempDir()},
		Scheduler:    scheduler.Store{Root: t.TempDir()},
		Now:          func() time.Time { return now },
	}
	command := commandForTest(now, HousekeepingJobID, false)
	command.Deadline = now.Add(20 * time.Millisecond)
	receipt, err := executor.ExecuteCommand(context.Background(), command)
	if !errors.Is(err, context.DeadlineExceeded) || receipt.State != maintenance.ReceiptTimedOut {
		t.Fatalf("receipt=%#v err=%v", receipt, err)
	}
}
