package maintenance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

type canonicalWalterHandlerFunc func(context.Context, Command, ExecutionGrant) (Receipt, error)

func (handler canonicalWalterHandlerFunc) ProposeWeekly(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	receipt, err := handler(ctx, command, grant)
	return HandlerResult{State: receipt.State, ProposalCount: receipt.ProposalCount, ProposalDigest: receipt.ProposalDigest, ProposalArtifactID: receipt.ProposalArtifactID, ReasonCode: receipt.ReasonCode}, err
}

type legacyExecuteHandlerFunc func(context.Context, Command) (HandlerResult, error)

func (handler legacyExecuteHandlerFunc) Execute(ctx context.Context, command Command) (HandlerResult, error) {
	return handler(ctx, command)
}

type authorizedHandlerFunc func(context.Context, Command, ExecutionGrant) (HandlerResult, error)

func (handler authorizedHandlerFunc) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	return handler(ctx, command, grant)
}

func TestHandlerExecutorRejectsLegacySeam(t *testing.T) {
	called := false
	legacy := legacyExecuteHandlerFunc(func(context.Context, Command) (HandlerResult, error) {
		called = true
		return HandlerResult{State: ReceiptSucceeded}, nil
	})
	if _, ok := handlerExecutor(legacy); ok {
		t.Fatal("legacy Execute seam remained executable")
	}
	if called {
		t.Fatal("legacy handler executed while being rejected")
	}
}

func TestHandlerExecutorRejectsMissingOrMismatchedGrantBeforeHandler(t *testing.T) {
	command := Command{CommandID: "command-1", JobID: "darwin-housekeeping-daily", WorkspaceID: "maestro-system", Deadline: time.Now().UTC().Add(time.Minute)}
	otherCommand := command
	otherCommand.CommandID = "command-2"
	mismatchedGrant, err := newExecutionGrant(otherCommand)
	if err != nil {
		t.Fatal(err)
	}
	correctGrant, err := newExecutionGrant(command)
	if err != nil {
		t.Fatal(err)
	}
	called := 0
	execute, ok := handlerExecutor(authorizedHandlerFunc(func(_ context.Context, _ Command, _ ExecutionGrant) (HandlerResult, error) {
		called++
		return HandlerResult{State: ReceiptSucceeded}, nil
	}))
	if !ok {
		t.Fatal("grant-aware handler was not accepted")
	}
	if _, err := execute(context.Background(), command, nil); err == nil {
		t.Fatal("missing execution grant was accepted")
	}
	if _, err := execute(context.Background(), command, mismatchedGrant); err == nil {
		t.Fatal("command-mismatched execution grant was accepted")
	}
	if called != 0 {
		t.Fatalf("handler executed before grant validation: called=%d", called)
	}
	if _, err := execute(context.Background(), command, correctGrant); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("handler was not executed with the valid grant: called=%d", called)
	}
}

func TestWorkerUnauthorizedWakeDoesNotCreateEnrollmentOrReceipts(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	schedulerRoot, receiptRoot := t.TempDir(), t.TempDir()
	worker := Worker{Catalog: catalog, Scheduler: scheduler.Store{Root: schedulerRoot}, Receipts: Store{Root: receiptRoot}, Jobs: []scheduler.Job{{ID: "darwin-housekeeping-daily", Cadence: scheduler.Daily, LocalHour: 3, MaxCatchUp: 1}}, ActivatedJobs: []string{"darwin-housekeeping-daily"}, LocalQualification: map[string]string{"darwin-housekeeping-daily": QualificationDigest("darwin-housekeeping-daily")}}
	_, err = worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "unauthorized", Now: time.Now().UTC()})
	if err == nil {
		t.Fatal("unauthorized wake was accepted")
	}
	if _, err := (scheduler.Store{Root: schedulerRoot}).LoadEnrollment("maestro-system"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unauthorized wake changed enrollment: %v", err)
	}
	if entries, readErr := os.ReadDir(receiptRoot); readErr == nil && len(entries) != 0 {
		t.Fatalf("unauthorized wake created receipt state: %v", entries)
	}
}

func TestWorkerRejectsMalformedEventIDBeforeEnrollment(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	schedulerRoot, receiptRoot := t.TempDir(), t.TempDir()
	worker := Worker{Catalog: catalog, Scheduler: scheduler.Store{Root: schedulerRoot}, Receipts: Store{Root: receiptRoot}, Jobs: []scheduler.Job{{ID: "wiki-incremental-sync", Cadence: scheduler.Daily, LocalHour: 3, MaxCatchUp: 1}}}
	_, err = worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", Trigger: TriggerEvent, EventID: "malformed event", OwnerID: "invalid-event", Now: time.Now().UTC(), Attended: true})
	if err == nil || !strings.Contains(err.Error(), "bounded event ID") {
		t.Fatalf("malformed event wake error=%v", err)
	}
	if _, err := (scheduler.Store{Root: schedulerRoot}).LoadEnrollment("maestro-system"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("malformed event wake changed enrollment: %v", err)
	}
	if entries, readErr := os.ReadDir(receiptRoot); readErr == nil && len(entries) != 0 {
		t.Fatalf("malformed event wake created receipt state: %v", entries)
	}
}

func TestWorkerRunsQualifiedDueOccurrenceAndFencesSuccess(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	if _, err := (scheduler.Store{Root: root}).EnsureEnrollment("maestro-system", now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	called := 0
	worker := Worker{
		Catalog: catalog, Scheduler: scheduler.Store{Root: root}, Receipts: Store{Root: t.TempDir()},
		Jobs: []scheduler.Job{{ID: "darwin-housekeeping-daily", Cadence: scheduler.Daily, LocalHour: 9, MaxCatchUp: 2}},
		Handlers: map[string]any{"darwin-housekeeping-daily": HandlerFunc(func(context.Context, Command, ExecutionGrant) (HandlerResult, error) {
			called++
			return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
		})},
		LocalQualification: map[string]string{"darwin-housekeeping-daily": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
		ActivatedJobs:      []string{"darwin-housekeeping-daily"}, Deadline: time.Minute,
	}
	report, err := worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "worker-1", Now: now, Attended: true})
	if err != nil || report.State != "processed" || len(report.Receipts) != 1 || report.Receipts[0].State != ReceiptSucceeded || called != 1 {
		t.Fatalf("report=%#v called=%d err=%v", report, called, err)
	}
	retry, err := worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "worker-1", Now: now, Attended: true})
	if err != nil || retry.State != "no_due_work" || len(retry.Due) != 0 || called != 1 {
		t.Fatalf("retry=%#v called=%d err=%v", retry, called, err)
	}
}

func TestWorkerPreservesEventIdentityAndSuppressesDuplicateEvent(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	catalog.CatalogState = RuntimeQualified
	for index := range catalog.Jobs {
		if catalog.Jobs[index].ID == "wiki-incremental-sync" {
			catalog.Jobs[index].Availability = Available
			catalog.Jobs[index].AvailabilityReason = ""
			catalog.Jobs[index].QualificationDigest = QualificationDigest(catalog.Jobs[index].ID)
		}
	}
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	receiptRoot := t.TempDir()
	worker := Worker{
		Catalog: catalog, Scheduler: scheduler.Store{Root: t.TempDir()}, Receipts: Store{Root: receiptRoot},
		Handlers: map[string]any{"wiki-incremental-sync": HandlerFunc(func(context.Context, Command, ExecutionGrant) (HandlerResult, error) {
			return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
		})},
		LocalQualification: map[string]string{"wiki-incremental-sync": QualificationDigest("wiki-incremental-sync")},
		ActivatedJobs:      []string{"wiki-incremental-sync"}, Deadline: time.Minute,
	}
	request := WakeRequest{WorkspaceID: "maestro-system", Trigger: TriggerEvent, EventID: "source-change-1", OwnerID: "event-worker", Now: now, Attended: true}
	first, err := worker.Run(context.Background(), request)
	if err != nil || len(first.Receipts) != 1 || first.Receipts[0].State != ReceiptSucceeded || first.Receipts[0].EventID != request.EventID {
		t.Fatalf("first event report=%#v err=%v", first, err)
	}
	second, err := worker.Run(context.Background(), request)
	if err != nil || len(second.Receipts) != 1 || second.Receipts[0].EventID != request.EventID {
		t.Fatalf("duplicate event report=%#v err=%v", second, err)
	}
	stored, err := (Store{Root: receiptRoot}).Receipts("maestro-system", "wiki-incremental-sync")
	if err != nil || len(stored) != 1 {
		t.Fatalf("event receipt count=%d err=%v", len(stored), err)
	}
}

func TestWorkerReleaseFailureReturnsRecoveryRequired(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	root, receiptRoot := t.TempDir(), t.TempDir()
	if _, err := (scheduler.Store{Root: root}).EnsureEnrollment("maestro-system", now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	worker := Worker{
		Catalog: catalog, Scheduler: scheduler.Store{Root: root}, Receipts: Store{Root: receiptRoot},
		Jobs: []scheduler.Job{{ID: "darwin-housekeeping-daily", Cadence: scheduler.Daily, LocalHour: 9, MaxCatchUp: 1}},
		Handlers: map[string]any{"darwin-housekeeping-daily": HandlerFunc(func(context.Context, Command, ExecutionGrant) (HandlerResult, error) {
			return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
		})},
		LocalQualification: map[string]string{"darwin-housekeeping-daily": QualificationDigest("darwin-housekeeping-daily")},
		ActivatedJobs:      []string{"darwin-housekeeping-daily"}, Deadline: time.Minute,
		ReleaseLease: func(scheduler.Lease) error { return errors.New("injected release failure") },
	}
	report, err := worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "cleanup-failure", Now: now, Attended: true})
	if err != nil || report.State != "completed_with_failures" || len(report.Receipts) != 1 || report.Receipts[0].State != ReceiptRecoveryRequired {
		t.Fatalf("release failure report=%#v err=%v", report, err)
	}
	stored, err := (Store{Root: receiptRoot}).Receipts("maestro-system", "darwin-housekeeping-daily")
	foundRecovery := false
	for _, receipt := range stored {
		if receipt.State == ReceiptRecoveryRequired {
			foundRecovery = true
		}
	}
	if err != nil || len(stored) != 2 || !foundRecovery {
		t.Fatalf("release failure receipts=%#v err=%v", stored, err)
	}
}

func TestWorkerArmFailureAndReleaseFailurePersistRecoveryEvidence(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	schedulerRoot, receiptRoot := t.TempDir(), t.TempDir()
	schedulerStore := scheduler.Store{Root: schedulerRoot}
	if _, err := schedulerStore.EnsureEnrollment("maestro-system", now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	worker := Worker{
		Catalog: catalog, Scheduler: schedulerStore, Receipts: Store{Root: receiptRoot},
		Jobs: []scheduler.Job{{ID: "darwin-housekeeping-daily", Cadence: scheduler.Daily, LocalHour: 9, MaxCatchUp: 1}},
		Handlers: map[string]any{"darwin-housekeeping-daily": HandlerFunc(func(context.Context, Command, ExecutionGrant) (HandlerResult, error) {
			return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
		})},
		LocalQualification: map[string]string{"darwin-housekeeping-daily": QualificationDigest("darwin-housekeeping-daily")},
		ActivatedJobs:      []string{"darwin-housekeeping-daily"}, Deadline: time.Minute,
		ArmLease:     func(scheduler.Lease) error { return errors.New("injected arm failure") },
		ReleaseLease: func(scheduler.Lease) error { return errors.New("injected arm cleanup failure") },
	}
	report, runErr := worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "arm-cleanup", Now: now, Attended: true})
	if runErr != nil || report.State != "completed_with_failures" || len(report.Receipts) != 1 || report.Receipts[0].State != ReceiptRecoveryRequired {
		t.Fatalf("arm cleanup report=%#v err=%v", report, runErr)
	}
	stored, err := (Store{Root: receiptRoot}).Receipts("maestro-system", "darwin-housekeeping-daily")
	if err != nil || len(stored) != 1 || stored[0].State != ReceiptRecoveryRequired {
		t.Fatalf("arm cleanup receipts=%#v err=%v", stored, err)
	}
}

func TestWorkerPublishCleanupFailurePersistsRecoveryEvidence(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	schedulerRoot, receiptRoot := t.TempDir(), t.TempDir()
	schedulerStore := scheduler.Store{Root: schedulerRoot}
	if _, err := schedulerStore.EnsureEnrollment("maestro-system", now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	worker := Worker{
		Catalog: catalog, Scheduler: schedulerStore, Receipts: Store{Root: receiptRoot},
		Jobs: []scheduler.Job{{ID: "darwin-housekeeping-daily", Cadence: scheduler.Daily, LocalHour: 9, MaxCatchUp: 1}},
		Handlers: map[string]any{"darwin-housekeeping-daily": HandlerFunc(func(context.Context, Command, ExecutionGrant) (HandlerResult, error) {
			receiptsPath := filepath.Join(schedulerRoot, "workspaces", "maestro-system", "receipts")
			if err := os.Remove(receiptsPath); err != nil {
				return HandlerResult{}, err
			}
			if err := os.Symlink(outside, receiptsPath); err != nil {
				return HandlerResult{}, err
			}
			return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
		})},
		LocalQualification: map[string]string{"darwin-housekeeping-daily": QualificationDigest("darwin-housekeeping-daily")}, ActivatedJobs: []string{"darwin-housekeeping-daily"}, Deadline: time.Minute,
		ReleaseLease: func(scheduler.Lease) error { return errors.New("injected publish cleanup failure") },
	}
	report, runErr := worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "publish-cleanup", Now: now, Attended: true})
	if runErr != nil || report.State != "completed_with_failures" || len(report.Receipts) != 1 || report.Receipts[0].State != ReceiptRecoveryRequired {
		t.Fatalf("publish cleanup report=%#v err=%v", report, runErr)
	}
	stored, err := (Store{Root: receiptRoot}).Receipts("maestro-system", "darwin-housekeeping-daily")
	if err != nil || len(stored) != 2 {
		t.Fatalf("publish cleanup durable receipts=%#v err=%v", stored, err)
	}
	foundRecovery := false
	for _, receipt := range stored {
		if receipt.State == ReceiptRecoveryRequired {
			foundRecovery = true
		}
	}
	if !foundRecovery {
		t.Fatalf("publish cleanup omitted recovery receipt: %#v", stored)
	}
}

func TestWorkerPlansCalendarInEnrollmentTimezoneAndRecordsUTC(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 2, 6, 0, 0, 0, time.UTC)
	root := t.TempDir()
	schedulerStore := scheduler.Store{Root: root}
	if _, err := schedulerStore.EnsureEnrollment("maestro-system", now.Add(-24*time.Hour).In(location)); err != nil {
		t.Fatal(err)
	}
	receiptRoot := t.TempDir()
	worker := Worker{Catalog: catalog, Scheduler: schedulerStore, Receipts: Store{Root: receiptRoot}, Jobs: []scheduler.Job{{ID: "darwin-housekeeping-daily", Cadence: scheduler.Daily, LocalHour: 3, MaxCatchUp: 1}}, Handlers: map[string]any{"darwin-housekeeping-daily": HandlerFunc(func(context.Context, Command, ExecutionGrant) (HandlerResult, error) {
		return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
	})}, LocalQualification: map[string]string{"darwin-housekeeping-daily": QualificationDigest("darwin-housekeeping-daily")}, ActivatedJobs: []string{"darwin-housekeeping-daily"}, Deadline: time.Minute}
	report, err := worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "worker-tz", Timezone: "America/Sao_Paulo", Now: now, Attended: true})
	if err != nil || len(report.Receipts) != 1 {
		t.Fatalf("report=%#v err=%v", report, err)
	}
	if report.Receipts[0].RecordedAt.Location() != time.UTC || report.Due[0].ScheduledFor.Hour() != 3 {
		t.Fatalf("timezone receipt=%#v", report.Receipts[0])
	}
}

func TestWorkerLeavesUnavailableModelJobDue(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	if _, err := (scheduler.Store{Root: root}).EnsureEnrollment("maestro-system", now.Add(-8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	worker := Worker{Catalog: catalog, Scheduler: scheduler.Store{Root: root}, Receipts: Store{Root: t.TempDir()}, Jobs: []scheduler.Job{{ID: "walter-self-review-weekly", Cadence: scheduler.Weekly, Weekday: time.Saturday, LocalHour: 9, MaxCatchUp: 1}}, Deadline: time.Minute}
	report, err := worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "worker-1", Now: now, Preauthorized: true})
	if err != nil || len(report.Receipts) != 1 || report.Receipts[0].State != ReceiptUnavailable {
		t.Fatalf("report=%#v err=%v", report, err)
	}
	second, err := worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "worker-1", Now: now.Add(time.Minute), Preauthorized: true})
	if err != nil || len(second.Due) != 1 {
		t.Fatalf("unavailable work was incorrectly completed: report=%#v err=%v", second, err)
	}
}

func TestWorkerHandlesWalterProposalJobAndPublishesOneTerminalReceipt(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	root, receiptRoot := t.TempDir(), t.TempDir()
	if _, err := (scheduler.Store{Root: root}).EnsureEnrollment("maestro-system", now.Add(-8*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	proposalDigest := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	worker := Worker{
		Catalog: catalog, Scheduler: scheduler.Store{Root: root}, Receipts: Store{Root: receiptRoot},
		Jobs: []scheduler.Job{{ID: WalterSelfReviewWeeklyJobID, Cadence: scheduler.Weekly, Weekday: time.Saturday, LocalHour: 9, MaxCatchUp: 1}},
		Handlers: map[string]any{WalterSelfReviewWeeklyJobID: WalterWeeklyAdapter{Handler: canonicalWalterHandlerFunc(func(_ context.Context, command Command, _ ExecutionGrant) (Receipt, error) {
			return Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: ReceiptProposalEmitted, RecordedAt: command.RequestedAt, Deadline: command.Deadline, ProposalOnly: true, ProposalCount: 1, ProposalDigest: proposalDigest, ProposalArtifactID: proposalDigest, ReasonCode: ReasonProposalEmitted}, nil
		})}},
		LocalQualification: map[string]string{WalterSelfReviewWeeklyJobID: QualificationDigest(WalterSelfReviewWeeklyJobID)},
		ActivatedJobs:      []string{WalterSelfReviewWeeklyJobID}, Deadline: time.Minute,
	}
	report, err := worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "walter-worker", Now: now, Attended: true})
	if err != nil || len(report.Receipts) != 1 || report.Receipts[0].State != ReceiptProposalEmitted {
		t.Fatalf("report=%#v err=%v", report, err)
	}
	stored, err := (Store{Root: receiptRoot}).Receipts("maestro-system", WalterSelfReviewWeeklyJobID)
	if err != nil || len(stored) != 1 || stored[0].State != ReceiptProposalEmitted {
		t.Fatalf("stored=%#v err=%v", stored, err)
	}
}

func TestWorkerDoesNotPersistBusyReceipt(t *testing.T) {
	if !errors.Is(scheduler.ErrLeaseBusy, scheduler.ErrLeaseBusy) {
		t.Fatal("lease sentinel changed")
	}
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	root := t.TempDir()
	if _, err := (scheduler.Store{Root: root}).EnsureEnrollment("maestro-system", now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	started, release := make(chan struct{}), make(chan struct{})
	makeWorker := func(receiptRoot string) Worker {
		return Worker{Catalog: catalog, Scheduler: scheduler.Store{Root: root}, Receipts: Store{Root: receiptRoot}, Jobs: []scheduler.Job{{ID: "darwin-housekeeping-daily", Cadence: scheduler.Daily, LocalHour: 9, MaxCatchUp: 1}}, Handlers: map[string]any{"darwin-housekeeping-daily": HandlerFunc(func(ctx context.Context, _ Command, _ ExecutionGrant) (HandlerResult, error) {
			close(started)
			select {
			case <-release:
				return HandlerResult{State: ReceiptSucceeded}, nil
			case <-ctx.Done():
				return HandlerResult{}, ctx.Err()
			}
		})}, LocalQualification: map[string]string{"darwin-housekeeping-daily": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, ActivatedJobs: []string{"darwin-housekeeping-daily"}, Deadline: time.Minute}
	}
	firstDone := make(chan error, 1)
	go func() {
		_, runErr := makeWorker(t.TempDir()).Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "worker-1", Now: now, Attended: true})
		firstDone <- runErr
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first worker did not acquire lease")
	}
	second, runErr := makeWorker(t.TempDir()).Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "worker-2", Now: now, Attended: true})
	if runErr != nil || len(second.Receipts) != 1 || second.Receipts[0].State != ReceiptBusy {
		t.Fatalf("busy report=%#v err=%v", second, runErr)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestWorkerDeadlineReturnsFromNonCooperativeHandlerAndNeverMarksSuccess(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	schedulerRoot, receiptRoot := t.TempDir(), t.TempDir()
	if _, err := (scheduler.Store{Root: schedulerRoot}).EnsureEnrollment("maestro-system", now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	worker := Worker{Catalog: catalog, Scheduler: scheduler.Store{Root: schedulerRoot}, Receipts: Store{Root: receiptRoot}, Jobs: []scheduler.Job{{ID: "darwin-housekeeping-daily", Cadence: scheduler.Daily, LocalHour: (now.Hour() + 23) % 24, MaxCatchUp: 1}}, Handlers: map[string]any{"darwin-housekeeping-daily": HandlerFunc(func(context.Context, Command, ExecutionGrant) (HandlerResult, error) {
		<-release
		return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
	})}, LocalQualification: map[string]string{"darwin-housekeeping-daily": QualificationDigest("darwin-housekeeping-daily")}, ActivatedJobs: []string{"darwin-housekeeping-daily"}, Deadline: 20 * time.Millisecond}
	started := time.Now()
	report, err := worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "worker-timeout", Now: now, Attended: true})
	if time.Since(started) > time.Second || len(report.Receipts) != 1 || report.Receipts[0].State != ReceiptTimedOut {
		t.Fatalf("report=%#v err=%v elapsed=%s", report, err, time.Since(started))
	}
	close(release)
	time.Sleep(10 * time.Millisecond)
	second, err := worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "worker-timeout", Now: now.Add(time.Minute), Attended: true})
	if err != nil || len(second.Due) != 1 {
		t.Fatalf("timed-out occurrence was completed: %#v err=%v", second, err)
	}
}

func TestWorkerQuarantinesOccurrenceWhileLateHandlerCanStillSideEffect(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	schedulerRoot, receiptRoot := t.TempDir(), t.TempDir()
	if _, err := (scheduler.Store{Root: schedulerRoot}).EnsureEnrollment("maestro-system", now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	makeWorker := func(owner string) Worker {
		return Worker{Catalog: catalog, Scheduler: scheduler.Store{Root: schedulerRoot}, Receipts: Store{Root: receiptRoot}, Jobs: []scheduler.Job{{ID: "darwin-housekeeping-daily", Cadence: scheduler.Daily, LocalHour: (now.Hour() + 23) % 24, MaxCatchUp: 1}}, Handlers: map[string]any{"darwin-housekeeping-daily": HandlerFunc(func(context.Context, Command, ExecutionGrant) (HandlerResult, error) {
			<-release
			return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
		})}, LocalQualification: map[string]string{"darwin-housekeeping-daily": QualificationDigest("darwin-housekeeping-daily")}, ActivatedJobs: []string{"darwin-housekeeping-daily"}, Deadline: 10 * time.Millisecond, Now: func() time.Time { return now }}
	}
	first, err := makeWorker("late-owner").Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "late-owner", Now: now, Attended: true})
	if err != nil || len(first.Receipts) != 1 || first.Receipts[0].State != ReceiptTimedOut {
		t.Fatalf("first report=%#v err=%v", first, err)
	}
	second, err := makeWorker("successor").Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "successor", Now: now.Add(2 * time.Minute), Attended: true})
	if err != nil || len(second.Receipts) != 1 || second.Receipts[0].State != ReceiptBusy {
		t.Fatalf("successor was allowed past TTL: report=%#v err=%v", second, err)
	}
	close(release)
	time.Sleep(20 * time.Millisecond)
}

func TestWorkerNeverPersistsHandlerErrorOrSecret(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	schedulerRoot, receiptRoot := t.TempDir(), t.TempDir()
	if _, err := (scheduler.Store{Root: schedulerRoot}).EnsureEnrollment("maestro-system", now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	worker := Worker{Catalog: catalog, Scheduler: scheduler.Store{Root: schedulerRoot}, Receipts: Store{Root: receiptRoot}, Jobs: []scheduler.Job{{ID: "darwin-housekeeping-daily", Cadence: scheduler.Daily, LocalHour: 9, MaxCatchUp: 1}}, Handlers: map[string]any{"darwin-housekeeping-daily": HandlerFunc(func(context.Context, Command, ExecutionGrant) (HandlerResult, error) {
		return HandlerResult{}, errors.New("secret-client-prompt-body")
	})}, LocalQualification: map[string]string{"darwin-housekeeping-daily": QualificationDigest("darwin-housekeeping-daily")}, ActivatedJobs: []string{"darwin-housekeeping-daily"}, Deadline: time.Minute}
	report, err := worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "worker-secret", Now: now, Attended: true})
	if len(report.Receipts) != 1 {
		t.Fatalf("report=%#v err=%v", report, err)
	}
	body, marshalErr := json.Marshal(report)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	if bytes.Contains(body, []byte("secret-client-prompt-body")) {
		t.Fatalf("secret leaked in receipt: %#v", report.Receipts[0])
	}
}

func TestUnavailableReceiptUsesCanonicalOccurrenceDigest(t *testing.T) {
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	worker := Worker{
		Receipts: Store{Root: t.TempDir()},
		Jobs:     []scheduler.Job{{ID: "darwin-housekeeping-daily", Cadence: scheduler.Daily}},
		Deadline: time.Minute,
	}
	occurrence := scheduler.Occurrence{JobID: "darwin-housekeeping-daily", ScheduledFor: now}
	receipt, err := worker.unavailableReceipt("maestro-system", occurrence, now, ReasonHandlerUnavailable)
	if err != nil {
		t.Fatal(err)
	}
	expected := (Command{JobID: occurrence.JobID, Trigger: TriggerDaily, ScheduledFor: occurrence.ScheduledFor}).OccurrenceDigest()
	if receipt.OccurrenceDigest != expected {
		t.Fatalf("unavailable receipt digest=%q, want canonical command digest %q", receipt.OccurrenceDigest, expected)
	}
}
