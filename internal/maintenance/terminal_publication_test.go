package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestWorkerCalendarTerminalFailureNeverReportsSuccess(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 2, 10, 0, 0, 0, time.UTC)
	store := scheduler.Store{Root: t.TempDir()}
	if _, err := store.EnsureEnrollment("maestro-system", now.Add(-24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	const job = "darwin-housekeeping-daily"
	effect := filepath.Join(t.TempDir(), "known-applied-effect.txt")
	var lease scheduler.Lease
	w := Worker{Catalog: catalog, Scheduler: store, Receipts: Store{Root: t.TempDir()}, Jobs: []scheduler.Job{{ID: job, Cadence: scheduler.Daily, LocalHour: 9, MaxCatchUp: 1}}, LocalQualification: map[string]string{job: QualificationDigest(job)}, ActivatedJobs: []string{job}, Deadline: time.Minute}
	w.ArmLease = func(actual scheduler.Lease) error { lease = actual; return store.ArmLease(actual) }
	w.Handlers = map[string]any{job: HandlerFunc(func(_ context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
		if err := ValidateExecutionGrant(grant, command); err != nil {
			return HandlerResult{}, err
		}
		if err := os.WriteFile(effect, []byte("known harmless fixture effect"), 0600); err != nil {
			return HandlerResult{}, err
		}
		if err := store.ReleaseLease(lease); err != nil {
			return HandlerResult{}, err
		}
		return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
	})}
	out, err := w.Run(context.Background(), WakeRequest{WorkspaceID: "maestro-system", Trigger: TriggerDaily, OwnerID: "calendar-publication-witness", Now: now, Attended: true})
	if err != nil || out.State != "completed_with_failures" || len(out.Receipts) != 1 || out.Receipts[0].State != ReceiptFailed {
		t.Fatalf("candidate success escaped failed terminal publication: %+v %v", out, err)
	}
	if body, err := os.ReadFile(effect); err != nil || string(body) != "known harmless fixture effect" {
		t.Fatal("failed terminal reporting erased a known applied effect")
	}
	calendar, err := store.Receipts("maestro-system")
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range calendar {
		if r.JobID == job && r.State == scheduler.Succeeded {
			t.Fatal("lost lease published calendar success")
		}
	}
}
