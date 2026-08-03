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

func TestSchedulerRejectsSymlinkedRootAncestorBeforeMkdir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink setup is host-dependent on Windows")
	}
	parent, outside := t.TempDir(), t.TempDir()
	alias := filepath.Join(parent, "alias")
	if err := os.Symlink(outside, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	root := filepath.Join(alias, "scheduler")
	if _, err := (Store{Root: root}).EnsureEnrollment("workspace-1", time.Now().UTC()); err == nil {
		t.Fatal("scheduler created state through a symlinked root ancestor")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("scheduler modified symlink target: entries=%v err=%v", entries, err)
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

func TestIntervalCadenceAnchorsAtEnrollmentThenLastSuccess(t *testing.T) {
	enrolledAt := time.Date(2026, 8, 2, 9, 10, 0, 0, time.UTC)
	job := Job{ID: "memory-checkpoint", Cadence: Interval, IntervalHours: 3, MaxCatchUp: 1}

	due, err := PlanDue([]Job{job}, enrolledAt, nil, enrolledAt.Add(3*time.Hour))
	if err != nil || !reflect.DeepEqual(due, []Occurrence{{JobID: job.ID, ScheduledFor: enrolledAt.Add(3 * time.Hour)}}) {
		t.Fatalf("enrollment-anchored due=%#v err=%v", due, err)
	}

	successAt := enrolledAt.Add(3*time.Hour + 7*time.Minute)
	receipts := []Receipt{{JobID: job.ID, ScheduledFor: due[0].ScheduledFor, AttemptedAt: successAt, State: Succeeded}}
	if got, err := PlanDue([]Job{job}, enrolledAt, receipts, successAt.Add(3*time.Hour-time.Second)); err != nil || len(got) != 0 {
		t.Fatalf("interval became due before last-success anchor: due=%#v err=%v", got, err)
	}
	want := []Occurrence{{JobID: job.ID, ScheduledFor: successAt.Add(3 * time.Hour)}}
	if got, err := PlanDue([]Job{job}, enrolledAt, receipts, want[0].ScheduledFor); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("last-success-anchored due=%#v want=%#v err=%v", got, want, err)
	}
}

func TestSuppressedIntervalAttemptRemainsDue(t *testing.T) {
	enrolledAt := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
	scheduledFor := enrolledAt.Add(3 * time.Hour)
	job := Job{ID: "memory-checkpoint", Cadence: Interval, IntervalHours: 3, MaxCatchUp: 1}
	receipts := []Receipt{{JobID: job.ID, ScheduledFor: scheduledFor, AttemptedAt: scheduledFor, State: Suppressed, Error: "idle_state_unknown"}}

	due, err := PlanDue([]Job{job}, enrolledAt, receipts, scheduledFor.Add(15*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(due, []Occurrence{{JobID: job.ID, ScheduledFor: scheduledFor}}) {
		t.Fatalf("suppressed occurrence advanced success/due: %#v", due)
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

func TestSecurePathWalkSurvivesAncestorRenameToSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("descriptor-relative nofollow walk is Unix-specific")
	}
	root, outside := t.TempDir(), t.TempDir()
	workspaces := filepath.Join(root, "workspaces")
	if err := os.Mkdir(workspaces, 0o700); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(root, "workspaces-original")
	previous := securePathStepHook
	t.Cleanup(func() { securePathStepHook = previous })
	securePathStepHook = func(component string) {
		if component != "workspaces" {
			return
		}
		if err := os.Rename(workspaces, moved); err != nil {
			t.Fatalf("rename ancestor: %v", err)
		}
		if err := os.Symlink(outside, workspaces); err != nil {
			t.Fatalf("replace ancestor with symlink: %v", err)
		}
	}
	if _, err := ensurePrivateTree(root, "workspaces", "workspace-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(moved, "workspace-1")); err != nil {
		t.Fatalf("descriptor-relative creation missed original ancestor: %v", err)
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("path walk followed renamed symlink: entries=%#v err=%v", entries, err)
	}
}

func TestSecureLeafIOSurvivesAncestorSwapForEnrollmentReceiptAndLease(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix adversarial swap uses descriptor-relative test fixtures")
	}
	cases := []struct {
		name string
		rel  []string
		file string
	}{
		{name: "enrollment", rel: []string{"workspaces", "case-a"}, file: "enrollment.json"},
		{name: "receipt", rel: []string{"workspaces", "case-a", "receipts", "memory-daily"}, file: "receipt.json"},
		{name: "lease", rel: []string{"workspaces", "case-a", "leases", "memory-daily"}, file: "lease.json"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root, outside := t.TempDir(), t.TempDir()
			parent, err := ensurePrivateTree(root, testCase.rel...)
			if err != nil {
				t.Fatal(err)
			}
			workspaces := filepath.Join(root, "workspaces")
			moved := filepath.Join(root, "workspaces-original")
			fired := false
			previous := secureLeafStepHook
			t.Cleanup(func() { secureLeafStepHook = previous })
			secureLeafStepHook = func(string) {
				if fired {
					return
				}
				fired = true
				if err := os.Rename(workspaces, moved); err != nil {
					t.Fatalf("rename ancestor: %v", err)
				}
				if err := os.Symlink(outside, workspaces); err != nil {
					t.Fatalf("replace ancestor with symlink: %v", err)
				}
			}
			path := filepath.Join(parent, testCase.file)
			if err := secureWriteNewFile(path, []byte("metadata-only\n")); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(moved, filepath.Join(append(testCase.rel[1:], testCase.file)...))); err != nil {
				t.Fatalf("secure leaf write did not stay on opened ancestor: %v", err)
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 0 {
				t.Fatalf("secure leaf write followed swapped ancestor: entries=%#v err=%v", entries, err)
			}
		})
	}
}

func TestSecureLeafReadAndRemoveSurviveAncestorSwap(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix adversarial swap uses descriptor-relative test fixtures")
	}
	root, outside := t.TempDir(), t.TempDir()
	parent, err := ensurePrivateTree(root, "workspaces", "case-a", "receipts", "memory-daily")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(parent, "receipt.json")
	if err := secureWriteNewFile(path, []byte("original\n")); err != nil {
		t.Fatal(err)
	}

	swap := func(fired *bool) func(string) {
		return func(string) {
			if *fired {
				return
			}
			*fired = true
			workspaces := filepath.Join(root, "workspaces")
			moved := filepath.Join(root, "workspaces-original")
			if err := os.Rename(workspaces, moved); err != nil {
				t.Fatalf("rename ancestor: %v", err)
			}
			if err := os.Symlink(outside, workspaces); err != nil {
				t.Fatalf("replace ancestor with symlink: %v", err)
			}
		}
	}

	previous := secureLeafStepHook
	t.Cleanup(func() { secureLeafStepHook = previous })
	fired := false
	secureLeafStepHook = swap(&fired)
	body, err := secureReadFile(path)
	if err != nil || string(body) != "original\n" {
		t.Fatalf("secure leaf read followed swapped ancestor: body=%q err=%v", body, err)
	}

	// Recreate the lexical ancestor only after the read assertion; the opened
	// descriptor still identifies the moved tree, while the path now points out.
	if err := os.Remove(filepath.Join(root, "workspaces")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, "workspaces-original"), filepath.Join(root, "workspaces")); err != nil {
		t.Fatal(err)
	}
	fired = false
	secureLeafStepHook = swap(&fired)
	if err := secureRemoveFile(path); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "workspaces-original", "case-a", "receipts", "memory-daily", "receipt.json")); !os.IsNotExist(err) {
		t.Fatalf("secure leaf remove did not remove original file: %v", err)
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("secure leaf remove followed swapped ancestor: entries=%#v err=%v", entries, err)
	}
}

func TestLeaseTransactionPinsGuardAndStateToOneDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("real-directory replacement fixture uses Unix rename semantics")
	}
	root := t.TempDir()
	store := Store{Root: root}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	lease, err := store.TryAcquireLease("case-a", "memory-daily", "occurrence", "worker-a", now, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := store.openLeaseDirectory("case-a", "memory-daily")
	if err != nil {
		t.Fatal(err)
	}
	defer first.close()
	second, _, err := store.openLeaseDirectory("case-a", "memory-daily")
	if err != nil {
		t.Fatal(err)
	}
	defer second.close()
	name := safeLeaseName(lease.OccurrenceKey)
	guard, err := acquireLeaseGuard(first, name+".guard")
	if err != nil {
		t.Fatal(err)
	}
	defer guard.release()
	if _, err := acquireLeaseGuard(second, name+".guard"); !errors.Is(err, errLeaseGuardBusy) {
		t.Fatalf("second pinned worker acquired the shared guard: %v", err)
	}

	workspaces := filepath.Join(root, "workspaces")
	moved := filepath.Join(root, "workspaces-original")
	if err := os.Rename(workspaces, moved); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(workspaces, 0o700); err != nil {
		t.Fatal(err)
	}
	current, err := readLeaseInDirectory(first, name+".json", "pinned")
	if err != nil || !sameLeaseIdentity(current, lease) {
		t.Fatalf("pinned guard/state transaction followed replacement: lease=%#v err=%v", current, err)
	}
	if _, err := os.Stat(filepath.Join(moved, "case-a", "leases", "memory-daily", name+".json")); err != nil {
		t.Fatalf("original pinned state was not retained: %v", err)
	}
	if entries, err := os.ReadDir(workspaces); err != nil || len(entries) != 0 {
		t.Fatalf("replacement directory received state: entries=%#v err=%v", entries, err)
	}
}
