package scheduler

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
	"time"
)

func TestPublishedSchedulerStateSchemaIsRecognized(t *testing.T) {
	if err := ValidateSchemaFile("../../schemas/scheduler-state.schema.json"); err != nil {
		t.Fatal(err)
	}
}

func TestPlanDueRecoversMissedDailyOccurrences(t *testing.T) {
	location := time.FixedZone("pilot", -3*60*60)
	enrolledAt := time.Date(2026, 7, 20, 19, 0, 0, 0, location)
	now := time.Date(2026, 7, 23, 9, 0, 0, 0, location)
	jobs := []Job{{ID: "memory-daily", Cadence: Daily, LocalHour: 18, MaxCatchUp: 2}}

	due, err := PlanDue(jobs, enrolledAt, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	want := []Occurrence{
		{JobID: "memory-daily", ScheduledFor: time.Date(2026, 7, 21, 18, 0, 0, 0, location)},
		{JobID: "memory-daily", ScheduledFor: time.Date(2026, 7, 22, 18, 0, 0, 0, location)},
	}
	if !reflect.DeepEqual(due, want) {
		t.Fatalf("due = %#v, want %#v", due, want)
	}
}

func TestPlanDueWaitsForWeeklyWindow(t *testing.T) {
	location := time.FixedZone("pilot", -3*60*60)
	enrolledAt := time.Date(2026, 7, 20, 9, 0, 0, 0, location)
	job := Job{ID: "memory-weekly", Cadence: Weekly, Weekday: time.Friday, LocalHour: 18, MaxCatchUp: 1}

	before := time.Date(2026, 7, 24, 17, 59, 0, 0, location)
	if due, err := PlanDue([]Job{job}, enrolledAt, nil, before); err != nil || len(due) != 0 {
		t.Fatalf("before weekly window due = %#v, err = %v", due, err)
	}
	after := before.Add(time.Minute)
	if due, err := PlanDue([]Job{job}, enrolledAt, nil, after); err != nil || len(due) != 1 || !due[0].ScheduledFor.Equal(after) {
		t.Fatalf("after weekly window due = %#v, err = %v", due, err)
	}
}

func TestPlanDueSupportsExplicitMonthlyCadence(t *testing.T) {
	location := time.FixedZone("pilot", -3*60*60)
	enrolledAt := time.Date(2026, 7, 1, 9, 0, 0, 0, location)
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, location)
	job := Job{ID: "darwin-monthly", Cadence: Monthly, DayOfMonth: 1, LocalHour: 8, MaxCatchUp: 3}
	due, err := PlanDue([]Job{job}, enrolledAt, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	want := []Occurrence{
		{JobID: "darwin-monthly", ScheduledFor: time.Date(2026, 8, 1, 8, 0, 0, 0, location)},
		{JobID: "darwin-monthly", ScheduledFor: time.Date(2026, 9, 1, 8, 0, 0, 0, location)},
	}
	if !reflect.DeepEqual(due, want) {
		t.Fatalf("monthly due = %#v, want %#v", due, want)
	}
}

func TestSuccessfulReceiptPreventsDuplicateExecution(t *testing.T) {
	location := time.FixedZone("pilot", -3*60*60)
	enrolledAt := time.Date(2026, 7, 20, 9, 0, 0, 0, location)
	scheduled := time.Date(2026, 7, 20, 18, 0, 0, 0, location)
	jobs := []Job{{ID: "memory-daily", Cadence: Daily, LocalHour: 18, MaxCatchUp: 7}}
	receipts := []Receipt{{JobID: "memory-daily", ScheduledFor: scheduled, AttemptedAt: scheduled.Add(time.Minute), State: Succeeded}}

	due, err := PlanDue(jobs, enrolledAt, receipts, scheduled.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 0 {
		t.Fatalf("successful occurrence remained due: %#v", due)
	}
}

func TestLaterSuccessDoesNotHideEarlierGap(t *testing.T) {
	location := time.FixedZone("pilot", -3*60*60)
	enrolledAt := time.Date(2026, 7, 20, 9, 0, 0, 0, location)
	first := time.Date(2026, 7, 20, 18, 0, 0, 0, location)
	second := first.AddDate(0, 0, 1)
	job := Job{ID: "memory-daily", Cadence: Daily, LocalHour: 18, MaxCatchUp: 7}
	receipts := []Receipt{
		{JobID: "memory-daily", ScheduledFor: first, AttemptedAt: first.Add(time.Minute), State: Failed, Error: "temporary failure"},
		{JobID: "memory-daily", ScheduledFor: second, AttemptedAt: second.Add(time.Minute), State: Succeeded},
	}

	due, err := PlanDue([]Job{job}, enrolledAt, receipts, second.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(due, []Occurrence{{JobID: "memory-daily", ScheduledFor: first}}) {
		t.Fatalf("earlier gap was hidden: %#v", due)
	}
}

func TestRunDueLeavesFailedOccurrenceRecoverable(t *testing.T) {
	location := time.FixedZone("pilot", -3*60*60)
	now := time.Date(2026, 7, 20, 18, 0, 0, 0, location)
	occurrence := Occurrence{JobID: "memory-daily", ScheduledFor: now}
	executor := ExecutorFunc(func(context.Context, Occurrence) error {
		return ErrCapabilityUnavailable
	})

	receipts := RunDue(context.Background(), executor, []Occurrence{occurrence}, now)
	if len(receipts) != 1 || receipts[0].State != Unavailable || !errors.Is(receipts[0].Err(), ErrCapabilityUnavailable) {
		t.Fatalf("receipts = %#v", receipts)
	}
	job := Job{ID: "memory-daily", Cadence: Daily, LocalHour: 18, MaxCatchUp: 1}
	due, err := PlanDue([]Job{job}, now.Add(-time.Hour), receipts, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(due, []Occurrence{occurrence}) {
		t.Fatalf("unavailable occurrence was not recoverable: %#v", due)
	}
}

func TestRunDueStopsWhenBoundedContextIsCancelled(t *testing.T) {
	now := time.Date(2026, 7, 20, 18, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	executor := ExecutorFunc(func(context.Context, Occurrence) error {
		calls++
		cancel()
		return nil
	})
	occurrences := []Occurrence{
		{JobID: "memory-daily", ScheduledFor: now},
		{JobID: "memory-daily", ScheduledFor: now.Add(24 * time.Hour)},
	}
	receipts := RunDue(ctx, executor, occurrences, now)
	if calls != 1 || len(receipts) != 1 {
		t.Fatalf("cancelled catch-up calls=%d receipts=%#v", calls, receipts)
	}
}

func TestSchedulerStorePersistsEnrollmentAndReceipts(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}
	enrolledAt := time.Date(2026, 7, 20, 12, 0, 0, 0, time.UTC)
	enrollment, err := store.EnsureEnrollment("case-a", enrolledAt)
	if err != nil {
		t.Fatal(err)
	}
	if !enrollment.EnrolledAt.Equal(enrolledAt) {
		t.Fatalf("enrollment = %#v", enrollment)
	}
	receipt := Receipt{JobID: "memory-daily", ScheduledFor: enrolledAt.Add(time.Hour), AttemptedAt: enrolledAt.Add(2 * time.Hour), State: Succeeded}
	if err := store.AppendReceipt("case-a", receipt); err != nil {
		t.Fatal(err)
	}
	got, err := store.Receipts("case-a")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []Receipt{receipt}) {
		t.Fatalf("receipts = %#v", got)
	}
}

func TestSchedulerLeaseIsNonBlockingAndReclaimsAfterExpiry(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	first, err := store.TryAcquireLease("case-a", "memory-daily", "memory-daily\x002026-07-30", "worker-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.TryAcquireLease("case-a", "memory-daily", "memory-daily\x002026-07-30", "worker-b", now.Add(10*time.Second), time.Minute); !errors.Is(err, ErrLeaseBusy) {
		t.Fatalf("second worker err = %v, want ErrLeaseBusy", err)
	}
	wrongOwner := first
	wrongOwner.OwnerID = "worker-b"
	if err := store.ReleaseLease(wrongOwner); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("wrong owner release err = %v, want ErrLeaseLost", err)
	}
	second, err := store.TryAcquireLease("case-a", "memory-daily", "memory-daily\x002026-07-30", "worker-b", first.ExpiresAt.Add(time.Second), time.Minute)
	if err != nil || second.OwnerID != "worker-b" {
		t.Fatalf("expired lease reclaim = %#v, err=%v", second, err)
	}
	if err := store.ReleaseLease(first); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale worker release err=%v, want ErrLeaseLost", err)
	}
	if _, err := store.TryAcquireLease("case-a", "memory-daily", "memory-daily\x002026-07-30", "worker-c", second.AcquiredAt.Add(time.Second), time.Minute); !errors.Is(err, ErrLeaseBusy) {
		t.Fatalf("stale release removed successor: %v", err)
	}
	if err := store.ReleaseLease(second); err != nil {
		t.Fatal(err)
	}
}

func TestQuarantinedLeaseRequiresExplicitRecoveryAfterExpiry(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	lease, err := store.TryAcquireLease("case-a", "memory-daily", ScheduledOccurrenceKey("memory-daily", now), "worker-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.QuarantineLease(lease); err != nil {
		t.Fatal(err)
	}
	quarantined, err := store.QuarantinedLeases("case-a")
	if err != nil || len(quarantined) != 1 || quarantined[0].FenceToken != lease.FenceToken {
		t.Fatalf("quarantine listing=%#v err=%v", quarantined, err)
	}
	if err := store.RecoverQuarantinedLease(lease, now.Add(30*time.Second)); !errors.Is(err, ErrLeaseBusy) {
		t.Fatalf("live quarantine recovery err=%v, want ErrLeaseBusy", err)
	}
	if _, err := store.TryAcquireLease("case-a", "memory-daily", lease.OccurrenceKey, "worker-b", now.Add(2*time.Minute), time.Minute); !errors.Is(err, ErrLeaseBusy) {
		t.Fatalf("quarantined occurrence was reclaimed: %v", err)
	}
	if err := store.RecoverQuarantinedLease(lease, now.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TryAcquireLease("case-a", "memory-daily", lease.OccurrenceKey, "worker-b", now.Add(2*time.Minute), time.Minute); err != nil {
		t.Fatalf("recovered occurrence was not available: %v", err)
	}
}

func TestReleaseLeaseCannotClearQuarantineWithWrongFenceToken(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	lease, err := store.TryAcquireLease("case-a", "memory-daily", ScheduledOccurrenceKey("memory-daily", now), "worker-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.QuarantineLease(lease); err != nil {
		t.Fatal(err)
	}
	wrong := lease
	wrong.FenceToken = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	if err := store.ReleaseLease(wrong); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("wrong fence release err=%v, want ErrLeaseLost", err)
	}
	if _, err := store.TryAcquireLease("case-a", "memory-daily", lease.OccurrenceKey, "worker-b", now.Add(2*time.Minute), time.Minute); !errors.Is(err, ErrLeaseBusy) {
		t.Fatalf("wrong fence removed quarantine: %v", err)
	}
	if err := store.ReleaseLease(lease); err != nil {
		t.Fatal(err)
	}
}

func TestLeaseNamesDoNotAliasSanitizedOccurrenceKeys(t *testing.T) {
	if safeLeaseName("event/a") == safeLeaseName("event?a") {
		t.Fatal("distinct occurrence keys produced the same lease name")
	}
}

func TestLeaseRejectsPersistedIdentityAndTTLTampering(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	directory, err := ensurePrivateTree(root, "workspaces", "case-a", "leases", "memory-daily")
	if err != nil {
		t.Fatal(err)
	}
	occurrenceKey := "memory-daily\x002026-07-30"
	path := filepath.Join(directory, safeLeaseName(occurrenceKey)+".json")
	tampered := Lease{
		SchemaVersion: 1, WorkspaceID: "case-a", JobID: "memory-daily",
		OccurrenceKey: "different-occurrence", OwnerID: "worker-a",
		FenceToken: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", AcquiredAt: now, ExpiresAt: now.Add(time.Minute),
	}
	if err := writeNewJSON(path, tampered); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TryAcquireLease("case-a", "memory-daily", occurrenceKey, "worker-b", now, time.Minute); err == nil || errors.Is(err, ErrLeaseBusy) {
		t.Fatalf("tampered identity was not rejected: %v", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	tampered.OccurrenceKey = occurrenceKey
	tampered.ExpiresAt = now.Add(16 * time.Minute)
	if err := writeNewJSON(path, tampered); err != nil {
		t.Fatal(err)
	}
	if _, err := store.TryAcquireLease("case-a", "memory-daily", occurrenceKey, "worker-b", now, time.Minute); err == nil || errors.Is(err, ErrLeaseBusy) {
		t.Fatalf("unbounded persisted lease was not rejected: %v", err)
	}
}

func TestSchedulerStoreRejectsSymlinkedWorkspaceAncestor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup is host-dependent on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "workspaces")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	store := Store{Root: root}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	if _, err := store.TryAcquireLease("case-a", "memory-daily", "occurrence", "worker", now, time.Minute); err == nil {
		t.Fatal("scheduler followed a symlinked workspace ancestor")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("scheduler wrote outside its root: entries=%#v err=%v", entries, err)
	}
}
