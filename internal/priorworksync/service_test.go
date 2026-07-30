package priorworksync

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/priorwork"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestCodexUnavailableReceiptLeavesOccurrenceDue(t *testing.T) {
	location := time.FixedZone("BRT", -3*60*60)
	enrolledAt := time.Date(2026, 7, 28, 5, 0, 0, 0, location)
	now := time.Date(2026, 7, 29, 7, 0, 0, 0, location)
	service := Service{
		Store: scheduler.Store{Root: t.TempDir()},
		Clock: func() time.Time { return now },
	}
	policy := priorwork.SchedulePolicy{
		EnrolledAt: enrolledAt, RefreshHours: 24, StaleHours: 72,
		Timezone: "America/Sao_Paulo",
	}
	report, err := service.RunPresence(context.Background(), "codex", policy)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Due || !report.Attempted || len(report.Receipts) != 1 ||
		report.Receipts[0].State != scheduler.Unavailable ||
		!errors.Is(report.Receipts[0].Err(), scheduler.ErrCapabilityUnavailable) ||
		!strings.Contains(report.Receipts[0].Error, "corporate_policy") {
		t.Fatalf("unexpected Codex report: %#v", report)
	}
	now = now.Add(time.Hour)
	retry, err := service.RunPresence(context.Background(), "codex", policy)
	if err != nil {
		t.Fatal(err)
	}
	if !retry.Due || len(retry.Receipts) != 1 {
		t.Fatalf("unavailable occurrence was incorrectly completed: %#v", retry)
	}
}

func TestQualifiedClaudePublicationCompletesOccurrence(t *testing.T) {
	location := time.FixedZone("BRT", -3*60*60)
	enrolledAt := time.Date(2026, 7, 28, 5, 0, 0, 0, location)
	now := time.Date(2026, 7, 29, 7, 0, 0, 0, location)
	calls := 0
	service := Service{
		Store: scheduler.Store{Root: t.TempDir()},
		Clock: func() time.Time { return now },
		Collector: CollectorFunc(func(_ context.Context, occurrence scheduler.Occurrence) (priorwork.ApplyReport, error) {
			calls++
			if occurrence.JobID != JobID {
				t.Fatalf("job=%s", occurrence.JobID)
			}
			return priorwork.ApplyReport{
				State: "published", Version: "v-" + strings.Repeat("a", 20),
				Fingerprint:        strings.Repeat("a", 64),
				CollectionSequence: 1, Watermark: "watermark-1", Items: 1,
				ReceiptID: "receipt-scheduled-a", TriggerRef: OccurrenceRef(occurrence),
			}, nil
		}),
		Verifier: VerifierFunc(func(report priorwork.ApplyReport) error {
			if report.Version != "v-"+strings.Repeat("a", 20) || report.Items != 1 {
				return errors.New("active manifest mismatch")
			}
			return nil
		}),
	}
	policy := priorwork.SchedulePolicy{
		EnrolledAt: enrolledAt, RefreshHours: 24, StaleHours: 72,
		Timezone: "America/Sao_Paulo",
	}
	report, err := service.RunPresence(context.Background(), "claude", policy)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(report.Receipts) != 1 || report.Receipts[0].State != scheduler.Succeeded {
		t.Fatalf("unexpected Claude report: %#v calls=%d", report, calls)
	}
	retry, err := service.RunPresence(context.Background(), "claude", policy)
	if err != nil {
		t.Fatal(err)
	}
	if retry.Due || retry.Attempted || len(retry.Receipts) != 0 || calls != 1 {
		t.Fatalf("successful occurrence repeated: %#v calls=%d", retry, calls)
	}
}

func TestSchedulerReportContainsNoProfessionalMetadata(t *testing.T) {
	enrolledAt := time.Date(2026, 7, 28, 5, 0, 0, 0, time.UTC)
	service := Service{
		Store: scheduler.Store{Root: t.TempDir()},
		Clock: func() time.Time { return enrolledAt.Add(26 * time.Hour) },
		Collector: CollectorFunc(func(_ context.Context, occurrence scheduler.Occurrence) (priorwork.ApplyReport, error) {
			return priorwork.ApplyReport{}, errors.New("Suzano /private/client https://sharepoint.com deck.pptx")
		}),
	}
	report, err := service.RunPresence(context.Background(), "claude", priorwork.SchedulePolicy{
		EnrolledAt: enrolledAt, RefreshHours: 24, StaleHours: 72, Timezone: "America/Sao_Paulo",
	})
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"Suzano", "Plantio", "sharepoint.com", ".pptx"} {
		if strings.Contains(string(body), forbidden) {
			t.Fatalf("scheduler report leaked %q: %s", forbidden, body)
		}
	}
	if len(report.Receipts) != 1 || report.Receipts[0].Error != ErrCollectorFailed.Error() {
		t.Fatalf("collector error taxonomy was not closed: %#v", report)
	}
}

func TestConcurrentPresenceHasSingleCollectorClaim(t *testing.T) {
	location, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		t.Fatal(err)
	}
	enrolledAt := time.Date(2026, 7, 28, 5, 0, 0, 0, location)
	now := enrolledAt.Add(25 * time.Hour)
	entered := make(chan struct{})
	release := make(chan struct{})
	service := Service{
		Store: scheduler.Store{Root: t.TempDir()},
		Clock: func() time.Time { return now },
		Collector: CollectorFunc(func(_ context.Context, occurrence scheduler.Occurrence) (priorwork.ApplyReport, error) {
			close(entered)
			<-release
			return priorwork.ApplyReport{
				State: "published", Version: "v-" + strings.Repeat("b", 20),
				Fingerprint: strings.Repeat("b", 64), CollectionSequence: 1,
				Watermark: "watermark-1", Items: 1,
				ReceiptID: "receipt-scheduled-b", TriggerRef: OccurrenceRef(occurrence),
			}, nil
		}),
		Verifier: VerifierFunc(func(priorwork.ApplyReport) error { return nil }),
	}
	policy := priorwork.SchedulePolicy{
		EnrolledAt: enrolledAt, RefreshHours: 24, StaleHours: 72,
		Timezone: "America/Sao_Paulo",
	}
	firstDone := make(chan error, 1)
	go func() {
		_, err := service.RunPresence(context.Background(), "claude", policy)
		firstDone <- err
	}()
	<-entered
	if _, err := service.RunPresence(context.Background(), "claude", policy); !errors.Is(err, ErrPresenceClaimed) {
		t.Fatalf("expected concurrent claim rejection, got %v", err)
	}
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
}

func TestDeclarativeReportWithoutActiveManifestDoesNotCompleteOccurrence(t *testing.T) {
	enrolledAt := time.Now().UTC().Add(-25 * time.Hour)
	service := Service{
		Store: scheduler.Store{Root: t.TempDir()},
		Clock: func() time.Time { return enrolledAt.Add(25 * time.Hour) },
		Collector: CollectorFunc(func(_ context.Context, occurrence scheduler.Occurrence) (priorwork.ApplyReport, error) {
			return priorwork.ApplyReport{
				State: "published", Version: "v-" + strings.Repeat("c", 20),
				Fingerprint: strings.Repeat("c", 64), CollectionSequence: 1,
				Watermark: "watermark-1", Items: 1,
				ReceiptID: "receipt-scheduled-c", TriggerRef: OccurrenceRef(occurrence),
			}, nil
		}),
		Verifier: StoreVerifier{
			Store: priorwork.Store{Root: t.TempDir()},
			Access: priorwork.AccessContext{
				ActorRef: "actor-local-test", Purpose: "prior_work_retrieval",
			},
		},
	}
	report, err := service.RunPresence(context.Background(), "claude", priorwork.SchedulePolicy{
		EnrolledAt: enrolledAt, RefreshHours: 24, StaleHours: 72, Timezone: "America/Sao_Paulo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Receipts) != 1 || report.Receipts[0].State != scheduler.Failed ||
		report.Receipts[0].Error != ErrPublicationUnverified.Error() {
		t.Fatalf("declarative publication incorrectly succeeded: %#v", report)
	}
}

func TestStalePresenceLeaseIsRecoverable(t *testing.T) {
	root := t.TempDir()
	claimDirectory := filepath.Join(root, "claims")
	if err := os.MkdirAll(claimDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	claimPath := filepath.Join(claimDirectory, JobID+".lock")
	body, err := json.Marshal(claimRecord{
		SchemaVersion: 1, PID: 2147483647, Token: strings.Repeat("d", 32),
		CreatedAt: time.Now().UTC().Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(claimPath, append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	enrolledAt := time.Now().UTC().Add(-25 * time.Hour)
	service := Service{
		Store: scheduler.Store{Root: root},
		Clock: func() time.Time { return enrolledAt.Add(25 * time.Hour) },
	}
	report, err := service.RunPresence(context.Background(), "codex", priorwork.SchedulePolicy{
		EnrolledAt: enrolledAt, RefreshHours: 24, StaleHours: 72, Timezone: "America/Sao_Paulo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Receipts) != 1 || report.Receipts[0].State != scheduler.Unavailable {
		t.Fatalf("stale lease did not recover: %#v", report)
	}
}

func TestConcurrentDeadOwnerRecoveryHasSingleClaim(t *testing.T) {
	root := t.TempDir()
	claimDirectory := filepath.Join(root, "claims")
	if err := os.MkdirAll(claimDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(claimRecord{
		SchemaVersion: 1, PID: 2147483647, Token: strings.Repeat("f", 32),
		CreatedAt: time.Now().UTC().Add(-time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(claimDirectory, JobID+".lock"),
		append(body, '\n'),
		0o600,
	); err != nil {
		t.Fatal(err)
	}

	type result struct {
		claim *presenceClaim
		err   error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for range 2 {
		go func() {
			<-start
			claim, err := acquirePresenceClaim(root)
			results <- result{claim: claim, err: err}
		}()
	}
	close(start)

	var winner *presenceClaim
	busy := 0
	for range 2 {
		got := <-results
		switch {
		case got.err == nil:
			if winner != nil {
				t.Fatal("two concurrent recoverers acquired the same occurrence")
			}
			winner = got.claim
		case errors.Is(got.err, ErrPresenceClaimed):
			busy++
		default:
			t.Fatalf("unexpected recovery result: %v", got.err)
		}
	}
	if winner == nil || busy != 1 {
		t.Fatalf("winner=%v busy=%d", winner != nil, busy)
	}
	winner.Release()
}

func TestDeadClaimSymlinkCannotEscapeSchedulerRoot(t *testing.T) {
	root := t.TempDir()
	claimDirectory := filepath.Join(root, "claims")
	if err := os.MkdirAll(claimDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	const sentinel = "must not be changed"
	if err := os.WriteFile(outside, []byte(sentinel), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(claimDirectory, JobID+".lock")); err != nil {
		t.Fatal(err)
	}

	claim, err := acquirePresenceClaim(root)
	if err != nil {
		t.Fatal(err)
	}
	defer claim.Release()
	body, err := os.ReadFile(outside)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != sentinel {
		t.Fatalf("claim recovery modified file outside scheduler root: %q", body)
	}
	info, err := os.Lstat(filepath.Join(claimDirectory, JobID+".lock"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		t.Fatalf("recovered claim is not a regular file: %s", info.Mode())
	}
}

func TestClaimsDirectorySymlinkCannotEscapeSchedulerRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	sentinel := filepath.Join(outside, "sentinel.txt")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "claims")); err != nil {
		t.Fatal(err)
	}

	if claim, err := acquirePresenceClaim(root); err == nil {
		claim.Release()
		t.Fatal("scheduler followed claims directory symlink")
	}
	entries, err := os.ReadDir(outside)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "sentinel.txt" {
		t.Fatalf("scheduler wrote outside root through claims symlink: %#v", entries)
	}
	body, err := os.ReadFile(sentinel)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "unchanged" {
		t.Fatalf("outside sentinel changed: %q", body)
	}
}

func TestLiveClaimOwnerIsNeverTakenOverByAge(t *testing.T) {
	root := t.TempDir()
	owner, err := acquirePresenceClaim(root)
	if err != nil {
		t.Fatal(err)
	}
	defer owner.Release()
	body, err := json.Marshal(claimRecord{
		SchemaVersion: 1, PID: os.Getpid(), Token: strings.Repeat("e", 32),
		CreatedAt: time.Now().UTC().AddDate(-1, 0, 0),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "claims", JobID+".lock"), append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := acquirePresenceClaim(root); !errors.Is(err, ErrPresenceClaimed) {
		t.Fatalf("live owner was taken over: %v", err)
	}
}
