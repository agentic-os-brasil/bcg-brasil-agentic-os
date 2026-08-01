package maintenance

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

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
		Handlers: map[string]Handler{"darwin-housekeeping-daily": HandlerFunc(func(context.Context, Command) (HandlerResult, error) {
			called++
			return HandlerResult{State: ReceiptSucceeded, Diagnostic: "bounded health receipt"}, nil
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
	report, err := worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "worker-1", Now: now})
	if err != nil || len(report.Receipts) != 1 || report.Receipts[0].State != ReceiptUnavailable {
		t.Fatalf("report=%#v err=%v", report, err)
	}
	second, err := worker.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", OwnerID: "worker-1", Now: now.Add(time.Minute)})
	if err != nil || len(second.Due) != 1 {
		t.Fatalf("unavailable work was incorrectly completed: report=%#v err=%v", second, err)
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
		return Worker{Catalog: catalog, Scheduler: scheduler.Store{Root: root}, Receipts: Store{Root: receiptRoot}, Jobs: []scheduler.Job{{ID: "darwin-housekeeping-daily", Cadence: scheduler.Daily, LocalHour: 9, MaxCatchUp: 1}}, Handlers: map[string]Handler{"darwin-housekeeping-daily": HandlerFunc(func(ctx context.Context, _ Command) (HandlerResult, error) {
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
