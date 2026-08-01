package darwin

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestHousekeepingExecutorDoesNotExposeRawOccurrenceExecution(t *testing.T) {
	if _, found := reflect.TypeOf(HousekeepingExecutor{}).MethodByName("Execute"); found {
		t.Fatal("raw scheduler occurrence bypass is exposed")
	}
}

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

func authorityForTest(t *testing.T, commands ...maintenance.Command) maintenance.ExecutionAuthority {
	t.Helper()
	catalog, err := maintenance.LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog.CatalogState = maintenance.RuntimeQualified
	selected := make(map[string]bool, len(commands))
	occurrences := make([]maintenance.OccurrenceAuthorization, 0, len(commands))
	seenOccurrences := make(map[string]bool, len(commands))
	activated := make([]string, 0, len(commands))
	for _, command := range commands {
		if !selected[command.JobID] {
			selected[command.JobID] = true
			activated = append(activated, command.JobID)
		}
		if !seenOccurrences[command.OccurrenceDigest()] {
			seenOccurrences[command.OccurrenceDigest()] = true
			occurrences = append(occurrences, maintenance.OccurrenceAuthorization{
				WorkspaceID: command.WorkspaceID, JobID: command.JobID, Trigger: command.Trigger, EventID: command.EventID, ScheduledFor: command.ScheduledFor,
			})
		}
	}
	for index := range catalog.Jobs {
		if selected[catalog.Jobs[index].ID] {
			catalog.Jobs[index].Availability = maintenance.Available
			catalog.Jobs[index].AvailabilityReason = ""
			catalog.Jobs[index].QualificationDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		}
	}
	authority, err := maintenance.NewExecutionAuthority(catalog, occurrences, activated, true)
	if err != nil {
		t.Fatal(err)
	}
	return authority
}

func TestExecuteCommandUsesLeaseAndPersistsBoundedReceipt(t *testing.T) {
	now := time.Now().UTC()
	command := commandForTest(now, HousekeepingJobID, false)
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
		Authority:    authorityForTest(t, command),
		Now:          func() time.Time { return now },
	}
	receipt, err := executor.ExecuteCommand(context.Background(), command)
	if err != nil || receipt.State != maintenance.ReceiptSucceeded {
		t.Fatalf("receipt=%#v err=%v", receipt, err)
	}
	stored, err := executor.CommandStore.Receipts(command.WorkspaceID, command.JobID)
	if err != nil || len(stored) != 1 || stored[0].State != maintenance.ReceiptSucceeded {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
	probe, err := executor.Scheduler.TryAcquireLease(command.WorkspaceID, command.JobID, command.OccurrenceKey(), "probe", now, time.Minute)
	if err != nil {
		t.Fatalf("lease was not released: %v", err)
	}
	if err := executor.Scheduler.ReleaseLease(probe); err != nil {
		t.Fatalf("probe lease cleanup failed: %v", err)
	}
	duplicate, err := executor.ExecuteCommand(context.Background(), command)
	if err != nil || duplicate.CommandID != command.CommandID || builds != 1 {
		t.Fatalf("duplicate receipt=%#v err=%v builds=%d", duplicate, err, builds)
	}
}

func TestExecuteCommandEmitsProposalWithoutInvokingTools(t *testing.T) {
	now := time.Now().UTC()
	command := commandForTest(now, "darwin-structural-evolution-proposal", true)
	command.Trigger = maintenance.TriggerMonthly
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
		Authority:    authorityForTest(t, command),
		Now:          func() time.Time { return now },
	}
	receipt, err := executor.ExecuteCommand(context.Background(), command)
	if err != nil || receipt.State != maintenance.ReceiptProposalEmitted || receipt.ProposalCount != 1 || len(receipt.ProposalDigest) != 64 || called {
		t.Fatalf("receipt=%#v err=%v called=%v", receipt, err, called)
	}
}

func TestExecuteCommandMonthlyProposalReviewWithNoChangesIsSuccessfulNoChange(t *testing.T) {
	now := time.Now().UTC()
	command := commandForTest(now, "darwin-structural-evolution-proposal", true)
	command.Trigger = maintenance.TriggerMonthly
	executor := HousekeepingExecutor{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-monthly-no-change", Runtime: "runtime-neutral"}, nil
		}),
		Store:        Store{Root: t.TempDir()},
		CommandStore: maintenance.Store{Root: t.TempDir()},
		Scheduler:    scheduler.Store{Root: t.TempDir()},
		Authority:    authorityForTest(t, command),
		Now:          func() time.Time { return now },
	}
	receipt, err := executor.ExecuteCommand(context.Background(), command)
	if err != nil || receipt.State != maintenance.ReceiptReviewedNoChange || receipt.ProposalCount != 0 || receipt.ProposalDigest != "" {
		t.Fatalf("monthly no-change receipt=%#v err=%v", receipt, err)
	}
	stored, err := executor.CommandStore.Receipts(command.WorkspaceID, command.JobID)
	if err != nil || len(stored) != 1 || stored[0].State != maintenance.ReceiptReviewedNoChange {
		t.Fatalf("monthly no-change receipt was not persisted=%#v err=%v", stored, err)
	}
}

func TestExecuteCommandMonthlyProposalRecoversArtifactBeforeRebuilding(t *testing.T) {
	now := time.Now().UTC()
	command := commandForTest(now, "darwin-structural-evolution-proposal", true)
	command.Trigger = maintenance.TriggerMonthly
	proposalRoot := t.TempDir()
	assessment, err := Plan(HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-monthly-crash", Runtime: "runtime-neutral", Mode: DeepReview, Observations: []Observation{{Code: ObservationContractDrift, Severity: SeverityHigh, Count: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	artifact := AssessmentProposalArtifact{SchemaVersion: proposalArtifactSchemaVersion, RecordType: "assessment", AgentID: AgentID, JobID: command.JobID, OccurrenceDigest: command.OccurrenceDigest(), ArtifactID: assessmentArtifactID(command.JobID, command.OccurrenceDigest()), WindowID: assessment.WindowID, ProposalDigest: proposalDigest(command.OccurrenceDigest(), assessment), Assessment: assessment, ScheduledFor: command.ScheduledFor.UTC(), RecordedAt: command.ScheduledFor.UTC()}
	if err := (ProposalStore{Root: proposalRoot}).Append(artifact); err != nil {
		t.Fatal(err)
	}
	builds := 0
	executor := HousekeepingExecutor{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			builds++
			return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-changed-after-crash", Runtime: "runtime-neutral", Mode: DeepReview, Observations: []Observation{{Code: ObservationStateStale, Severity: SeverityHigh, Count: 1}}}, nil
		}),
		CommandStore: maintenance.Store{Root: t.TempDir()}, ProposalStore: ProposalStore{Root: proposalRoot}, Scheduler: scheduler.Store{Root: t.TempDir()}, Store: Store{Root: t.TempDir()}, Authority: authorityForTest(t, command), Now: func() time.Time { return now },
	}
	receipt, err := executor.ExecuteCommand(context.Background(), command)
	if err != nil || receipt.State != maintenance.ReceiptProposalEmitted || receipt.ProposalArtifactID != artifact.ArtifactID || receipt.ProposalDigest != artifact.ProposalDigest || builds != 0 {
		t.Fatalf("monthly artifact recovery receipt=%#v err=%v builds=%d", receipt, err, builds)
	}
	stored, err := executor.CommandStore.Receipts(command.WorkspaceID, command.JobID)
	if err != nil || len(stored) != 1 {
		t.Fatalf("monthly artifact recovery receipts=%#v err=%v", stored, err)
	}
}

func TestExecuteCommandDoesNotReplayArtifactBeforeAuthorityOrLease(t *testing.T) {
	now := time.Now().UTC()
	command := commandForTest(now, "darwin-structural-evolution-proposal", true)
	command.Trigger = maintenance.TriggerMonthly
	proposalRoot := t.TempDir()
	assessment, err := Plan(HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-unauthorized", Runtime: "runtime-neutral", Mode: DeepReview, Observations: []Observation{{Code: ObservationContractDrift, Severity: SeverityHigh, Count: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	artifact := AssessmentProposalArtifact{SchemaVersion: proposalArtifactSchemaVersion, RecordType: "assessment", AgentID: AgentID, JobID: command.JobID, OccurrenceDigest: command.OccurrenceDigest(), ArtifactID: assessmentArtifactID(command.JobID, command.OccurrenceDigest()), WindowID: assessment.WindowID, ProposalDigest: proposalDigest(command.OccurrenceDigest(), assessment), Assessment: assessment, ScheduledFor: command.ScheduledFor.UTC(), RecordedAt: command.ScheduledFor.UTC()}
	if err := (ProposalStore{Root: proposalRoot}).Append(artifact); err != nil {
		t.Fatal(err)
	}
	authority := authorityForTest(t, commandForTest(now, HousekeepingJobID, false))
	schedulerRoot, commandRoot := t.TempDir(), t.TempDir()
	executor := HousekeepingExecutor{Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
		t.Fatal("unauthorized artifact replay built health")
		return HealthPacket{}, nil
	}), CommandStore: maintenance.Store{Root: commandRoot}, ProposalStore: ProposalStore{Root: proposalRoot}, Scheduler: scheduler.Store{Root: schedulerRoot}, Store: Store{Root: t.TempDir()}, Authority: authority, Now: func() time.Time { return now }}
	receipt, err := executor.ExecuteCommand(context.Background(), command)
	if err == nil || receipt.State != maintenance.ReceiptUnavailable {
		t.Fatalf("unauthorized artifact replay receipt=%#v err=%v", receipt, err)
	}
	stored, readErr := executor.CommandStore.Receipts(command.WorkspaceID, command.JobID)
	if readErr != nil || len(stored) != 0 {
		t.Fatalf("unauthorized artifact replay persisted receipt: receipts=%#v err=%v", stored, readErr)
	}
	if leases, readErr := (scheduler.Store{Root: schedulerRoot}).QuarantinedLeases(command.WorkspaceID); readErr != nil || len(leases) != 0 {
		t.Fatalf("unauthorized artifact replay acquired lease: leases=%#v err=%v", leases, readErr)
	}
}

func TestExecuteCommandArmCleanupFailurePersistsRecoveryEvidence(t *testing.T) {
	now := time.Now().UTC()
	command := commandForTest(now, HousekeepingJobID, false)
	commandRoot, schedulerRoot := t.TempDir(), t.TempDir()
	executor := HousekeepingExecutor{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			return HealthPacket{}, errors.New("must not build after arm failure")
		}), Guard: ToolGuardFunc(func(ToolCall) error { return nil }), Invoker: InvokerFunc(func(context.Context, ToolCall, Artifact) (ToolResult, error) { return ToolResult{}, nil }),
		CommandStore: maintenance.Store{Root: commandRoot}, Scheduler: scheduler.Store{Root: schedulerRoot}, Store: Store{Root: t.TempDir()}, Authority: authorityForTest(t, command), Now: func() time.Time { return now },
		ArmLease: func(scheduler.Lease) error { return errors.New("injected arm failure") }, ReleaseLease: func(scheduler.Lease) error { return errors.New("injected arm cleanup failure") },
	}
	receipt, err := executor.ExecuteCommand(context.Background(), command)
	if err == nil || receipt.State != maintenance.ReceiptRecoveryRequired {
		t.Fatalf("arm cleanup receipt=%#v err=%v", receipt, err)
	}
	stored, readErr := executor.CommandStore.Receipts(command.WorkspaceID, command.JobID)
	if readErr != nil || len(stored) != 1 || stored[0].State != maintenance.ReceiptRecoveryRequired {
		t.Fatalf("arm cleanup durable evidence=%#v err=%v", stored, readErr)
	}
}

func TestExecuteCommandReleaseFailurePersistsRecoveryEvidence(t *testing.T) {
	now := time.Now().UTC()
	command := commandForTest(now, "darwin-structural-evolution-proposal", true)
	command.Trigger = maintenance.TriggerMonthly
	commandRoot, schedulerRoot := t.TempDir(), t.TempDir()
	executor := HousekeepingExecutor{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-release", Runtime: "runtime-neutral", Mode: DeepReview}, nil
		}),
		CommandStore: maintenance.Store{Root: commandRoot}, Scheduler: scheduler.Store{Root: schedulerRoot}, Store: Store{Root: t.TempDir()}, Authority: authorityForTest(t, command), Now: func() time.Time { return now },
		ReleaseLease: func(scheduler.Lease) error { return errors.New("injected release failure") },
	}
	receipt, err := executor.ExecuteCommand(context.Background(), command)
	if err == nil || receipt.State != maintenance.ReceiptRecoveryRequired {
		t.Fatalf("release failure receipt=%#v err=%v", receipt, err)
	}
	stored, readErr := executor.CommandStore.Receipts(command.WorkspaceID, command.JobID)
	if readErr != nil || len(stored) != 2 {
		t.Fatalf("release failure durable evidence=%#v err=%v", stored, readErr)
	}
}

func TestExecuteCommandReturnsBusyWithoutWaiting(t *testing.T) {
	now := time.Now().UTC()
	schedulerRoot := t.TempDir()
	store := scheduler.Store{Root: schedulerRoot}
	command := commandForTest(now, HousekeepingJobID, false)
	existing, err := store.TryAcquireLease(command.WorkspaceID, command.JobID, command.OccurrenceKey(), "other", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	executor := HousekeepingExecutor{Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
		return HealthPacket{}, errors.New("must not build")
	}), Guard: ToolGuardFunc(func(ToolCall) error { return nil }), Invoker: InvokerFunc(func(context.Context, ToolCall, Artifact) (ToolResult, error) {
		return ToolResult{}, errors.New("must not invoke")
	}), Store: Store{Root: t.TempDir()}, CommandStore: maintenance.Store{Root: t.TempDir()}, Scheduler: store, Authority: authorityForTest(t, command), Now: func() time.Time { return now }}
	receipt, err := executor.ExecuteCommand(context.Background(), command)
	if !errors.Is(err, scheduler.ErrLeaseBusy) || receipt.State != maintenance.ReceiptBusy {
		t.Fatalf("receipt=%#v err=%v", receipt, err)
	}
	stored, readErr := executor.CommandStore.Receipts(command.WorkspaceID, command.JobID)
	if readErr != nil || len(stored) != 0 {
		t.Fatalf("busy attempt became terminal: stored=%#v err=%v", stored, readErr)
	}
	if err := store.ReleaseLease(existing); err != nil {
		t.Fatal(err)
	}
}

func TestExecuteCommandSerializesDifferentCommandIDsForOneOccurrence(t *testing.T) {
	now := time.Now().UTC()
	first := commandForTest(now, HousekeepingJobID, false)
	second := first
	second.CommandID = "command-darwin-housekeeping-retry"
	entered := make(chan struct{})
	release := make(chan struct{})
	executor := HousekeepingExecutor{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			close(entered)
			<-release
			return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-concurrent", Runtime: "claude", Observations: []Observation{{Code: ObservationStateStale, Severity: SeverityLow, Count: 1}}}, nil
		}),
		Guard: ToolGuardFunc(func(ToolCall) error { return nil }),
		Invoker: InvokerFunc(func(context.Context, ToolCall, Artifact) (ToolResult, error) {
			return ToolResult{Outcome: OutcomeSucceeded}, nil
		}),
		Store:        Store{Root: t.TempDir()},
		CommandStore: maintenance.Store{Root: t.TempDir()},
		Scheduler:    scheduler.Store{Root: t.TempDir()},
		Authority:    authorityForTest(t, first),
		Now:          func() time.Time { return now },
	}
	type result struct {
		receipt maintenance.Receipt
		err     error
	}
	finished := make(chan result, 1)
	go func() {
		receipt, err := executor.ExecuteCommand(context.Background(), first)
		finished <- result{receipt: receipt, err: err}
	}()
	<-entered
	busy, err := executor.ExecuteCommand(context.Background(), second)
	if !errors.Is(err, scheduler.ErrLeaseBusy) || busy.State != maintenance.ReceiptBusy {
		t.Fatalf("second receipt=%#v err=%v", busy, err)
	}
	close(release)
	winner := <-finished
	if winner.err != nil || winner.receipt.State != maintenance.ReceiptSucceeded {
		t.Fatalf("winner receipt=%#v err=%v", winner.receipt, winner.err)
	}
	stored, err := executor.CommandStore.Receipts(first.WorkspaceID, first.JobID)
	if err != nil || len(stored) != 1 || stored[0].CommandID != first.CommandID || stored[0].State != maintenance.ReceiptSucceeded {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
	executor.Authority = authorityForTest(t, second)
	replay, err := executor.ExecuteCommand(context.Background(), second)
	if err != nil || replay.CommandID != first.CommandID || replay.OccurrenceDigest != first.OccurrenceDigest() {
		t.Fatalf("occurrence replay receipt=%#v err=%v", replay, err)
	}
	stored, err = executor.CommandStore.Receipts(first.WorkspaceID, first.JobID)
	if err != nil || len(stored) != 1 {
		t.Fatalf("occurrence replay duplicated work: stored=%#v err=%v", stored, err)
	}
}

func TestExecuteCommandFencesSideEffectsAcrossExpiredTTL(t *testing.T) {
	now := time.Now().UTC()
	command := commandForTest(now, HousekeepingJobID, false)
	command.Deadline = now.Add(20 * time.Millisecond)
	entered := make(chan struct{})
	release := make(chan struct{})
	schedulerStore := scheduler.Store{Root: t.TempDir()}
	executor := HousekeepingExecutor{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			close(entered)
			<-release
			return HealthPacket{SchemaVersion: SchemaVersion, WindowID: "window-expired-fence", Runtime: "claude", Observations: []Observation{{Code: ObservationStateStale, Severity: SeverityLow, Count: 1}}}, nil
		}),
		Guard: ToolGuardFunc(func(ToolCall) error { return nil }),
		Invoker: InvokerFunc(func(context.Context, ToolCall, Artifact) (ToolResult, error) {
			return ToolResult{Outcome: OutcomeSucceeded}, nil
		}),
		Store:        Store{Root: t.TempDir()},
		CommandStore: maintenance.Store{Root: t.TempDir()},
		Scheduler:    schedulerStore,
		Authority:    authorityForTest(t, command),
		Now:          func() time.Time { return now },
	}
	finished := make(chan error, 1)
	go func() {
		_, err := executor.ExecuteCommand(context.Background(), command)
		finished <- err
	}()
	<-entered
	<-time.After(30 * time.Millisecond)
	if _, err := schedulerStore.TryAcquireLease(command.WorkspaceID, command.JobID, command.OccurrenceKey(), "successor", command.Deadline.Add(time.Second), time.Minute); !errors.Is(err, scheduler.ErrLeaseBusy) {
		t.Fatalf("successor entered while stale worker still owned side-effect guard: %v", err)
	}
	close(release)
	if err := <-finished; !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expired worker err=%v, want deadline exceeded", err)
	}
}

func TestExecuteCommandTimesOutAtExplicitDeadline(t *testing.T) {
	now := time.Now().UTC()
	command := commandForTest(now, HousekeepingJobID, false)
	command.Deadline = now.Add(20 * time.Millisecond)
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
		Authority:    authorityForTest(t, command),
		Now:          func() time.Time { return now },
	}
	receipt, err := executor.ExecuteCommand(context.Background(), command)
	if !errors.Is(err, context.DeadlineExceeded) || receipt.State != maintenance.ReceiptTimedOut {
		t.Fatalf("receipt=%#v err=%v", receipt, err)
	}
}

func TestExecuteCommandRejectsUnownedJobAndTrigger(t *testing.T) {
	now := time.Now().UTC()
	authorized := commandForTest(now, HousekeepingJobID, false)
	executor := HousekeepingExecutor{
		Build: HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) {
			return HealthPacket{}, errors.New("must not build")
		}),
		Store:        Store{Root: t.TempDir()},
		CommandStore: maintenance.Store{Root: t.TempDir()},
		Scheduler:    scheduler.Store{Root: t.TempDir()},
		Authority:    authorityForTest(t, authorized),
		Now:          func() time.Time { return now },
	}
	command := commandForTest(now, "memory-daily", false)
	if receipt, err := executor.ExecuteCommand(context.Background(), command); err == nil || receipt.State != maintenance.ReceiptUnavailable {
		t.Fatalf("unowned command receipt=%#v err=%v", receipt, err)
	}
	command = commandForTest(now, "darwin-structural-evolution-proposal", true)
	if receipt, err := executor.ExecuteCommand(context.Background(), command); err == nil || receipt.State != maintenance.ReceiptUnavailable {
		t.Fatalf("non-monthly proposal receipt=%#v err=%v", receipt, err)
	}
}

func TestExecuteCommandRejectsAuthorityOccurrenceMismatch(t *testing.T) {
	now := time.Now().UTC()
	command := commandForTest(now, HousekeepingJobID, false)
	mismatched := command
	mismatched.ScheduledFor = now.Add(-time.Hour)
	executor := HousekeepingExecutor{
		Build:        HealthPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (HealthPacket, error) { return HealthPacket{}, nil }),
		Guard:        ToolGuardFunc(func(ToolCall) error { return nil }),
		Invoker:      InvokerFunc(func(context.Context, ToolCall, Artifact) (ToolResult, error) { return ToolResult{}, nil }),
		Store:        Store{Root: t.TempDir()},
		CommandStore: maintenance.Store{Root: t.TempDir()},
		Scheduler:    scheduler.Store{Root: t.TempDir()},
		Authority:    authorityForTest(t, mismatched),
		Now:          func() time.Time { return now },
	}
	if receipt, err := executor.ExecuteCommand(context.Background(), command); err == nil || receipt.State != maintenance.ReceiptUnavailable {
		t.Fatalf("mismatched authority receipt=%#v err=%v", receipt, err)
	}
}
