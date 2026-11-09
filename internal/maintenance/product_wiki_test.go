//go:build !windows

package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func productWikiCommit(t *testing.T, s *Service, text string) {
	t.Helper()
	productCapture(t, s, text)
	if _, err := s.engine.DreamDailyAttested(context.Background(), s.workspaceID, s.now()); err != nil {
		t.Fatal(err)
	}
}

func TestProductWikiStaleReviewAndClockOnlyNoopCannotQualify(t *testing.T) {
	s := productFixture(t)
	productWikiCommit(t, s, "private-wiki-body: REVIEWED_SOURCE")
	review, err := s.Review(Operation{Kind: "qualify", JobID: WikiReconcileJobID})
	if err != nil {
		t.Fatal(err)
	}
	productWikiCommit(t, s, "private-wiki-body: NEW_SOURCE_AFTER_REVIEW")
	before := productSnapshot(t, s.home)
	if _, err := s.Apply(context.Background(), review, approveProduct); !errors.Is(err, ErrProductConflict) {
		t.Fatal("changed reviewed source was admitted")
	}
	if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
		t.Fatal("stale review changed source or authority")
	}
	qualified := productApply(t, s, "qualify", WikiReconcileJobID, "")
	later := s.now().Add(time.Hour)
	s.now, s.engine.Now = func() time.Time { return later }, func() time.Time { return later }
	review, err = s.Review(Operation{Kind: "qualify", JobID: WikiReconcileJobID, ExpectedRevision: qualified.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(context.Background(), review, approveProduct); !errors.Is(err, ErrQualificationRequired) {
		t.Fatalf("clock-only no-op minted qualification: %v", err)
	}
	status, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range status.Jobs {
		if job.JobID == WikiReconcileJobID && (job.Qualification == nil || job.Qualification.ID != qualified.Qualification.ID) {
			t.Fatal("no-op relabelled historical qualification")
		}
	}
}

func TestProductWikiCadenceEnrollmentAndReplay(t *testing.T) {
	for _, job := range []string{WikiIncrementalJobID, WikiReconcileJobID, WikiIntegrityJobID} {
		t.Run(job, func(t *testing.T) {
			s := productFixture(t)
			productWikiCommit(t, s, "private-wiki-body: INITIAL_COMMITTED_INPUT")
			if job == WikiIntegrityJobID {
				if _, err := s.engine.CompilePrivateWiki(context.Background(), s.workspaceID, ""); err != nil {
					t.Fatal(err)
				}
			}
			q := productApply(t, s, "qualify", job, "")
			if q.Qualification == nil || q.QualificationState != "qualified" || q.State != "inactive" {
				t.Fatal(q)
			}
			active := productApply(t, s, "enroll", job, "UTC")
			if active.Schedule.EventOnly != (job == WikiIncrementalJobID) || active.Schedule.MaxCatchUp != 1 || job == WikiReconcileJobID && active.Schedule.Cadence != scheduler.Weekly {
				t.Fatal("wrong event/calendar contract")
			}
			if job == WikiIncrementalJobID {
				if q.Qualification.EventID == "" || s.receipts.VerifyEventReceipt(q.Qualification.Receipt) != nil {
					t.Fatal("actual durable event receipt absent")
				}
				calendar, err := s.scheduler.Receipts(s.workspaceID)
				if err != nil {
					t.Fatal(err)
				}
				for _, r := range calendar {
					if r.JobID == job {
						t.Fatal("event qualification invented a calendar receipt")
					}
				}
			}
			days := 1
			if job == WikiReconcileJobID {
				days = 8
			}
			later := s.now().AddDate(0, 0, days)
			s.now = func() time.Time { return later }
			s.engine.Now = s.now
			if job == WikiIncrementalJobID {
				if out, err := s.RunDue(context.Background()); err != nil || len(out.Due) != 0 {
					t.Fatalf("clock alone scheduled incremental work: %+v %v", out, err)
				}
			}
			productWikiCommit(t, s, "private-wiki-body: LATER_GENUINE_SOURCE")
			if job == WikiIntegrityJobID {
				snapshot, err := s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := s.engine.CompilePrivateWiki(context.Background(), s.workspaceID, snapshot.Parent); err != nil {
					t.Fatal(err)
				}
			}
			out, err := s.RunDue(context.Background())
			if err != nil || len(out.Receipts) != 1 || out.Receipts[0].State != ReceiptSucceeded {
				t.Fatalf("actual owning due work failed: %+v %v", out, err)
			}
			report, err := s.Report(job)
			if err != nil || report.Wiki == nil || report.Finding != "private_graph_verified" || report.Wiki.Concepts < 1 {
				t.Fatalf("actual current domain report absent: %+v %v", report, err)
			}
			body, _ := json.Marshal(report)
			if strings.Contains(string(body), "private-wiki-body") {
				t.Fatal("private text entered a domain report")
			}
			before := productSnapshot(t, s.home)
			out, err = s.RunDue(context.Background())
			if err != nil || len(out.Due) != 0 || !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
				t.Fatal("replay changed durable output")
			}
		})
	}
}

func activeProductWiki(t *testing.T) (*Service, ProductJobStatus) {
	t.Helper()
	s := productFixture(t)
	productWikiCommit(t, s, "private-wiki-body: FIRST_FENCED_SOURCE")
	productApply(t, s, "qualify", WikiIncrementalJobID, "")
	active := productApply(t, s, "enroll", WikiIncrementalJobID, "UTC")
	later := s.now().Add(time.Minute)
	s.now = func() time.Time { return later }
	s.engine.Now = s.now
	productWikiCommit(t, s, "private-wiki-body: PENDING_FENCED_SOURCE")
	return s, active
}

func releaseActualProductLease(s *Service) error {
	root := filepath.Join(s.scheduler.Root, "workspaces", s.workspaceID, "leases", WikiIncrementalJobID)
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".json") {
			body, err := os.ReadFile(filepath.Join(root, entry.Name()))
			if err != nil {
				return err
			}
			var lease scheduler.Lease
			if err := json.Unmarshal(body, &lease); err != nil {
				return err
			}
			return s.scheduler.ReleaseLease(lease)
		}
	}
	return errors.New("actual test lease not found")
}

func TestProductWikiLiveFencesBeforePointer(t *testing.T) {
	for _, action := range []string{"pause", "revoke", "source", "lease"} {
		t.Run(action, func(t *testing.T) {
			s, _ := activeProductWiki(t)
			pending, err := s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
			if err != nil {
				t.Fatal(err)
			}
			blocked, release := make(chan struct{}), make(chan struct{})
			s.engine.FaultPoint = func(point string) error {
				if point == "wiki_after_snapshot" {
					close(blocked)
					<-release
				}
				return nil
			}
			done := make(chan error, 1)
			go func() { _, err := s.RunDue(context.Background()); done <- err }()
			select {
			case <-blocked:
			case <-time.After(45 * time.Second):
				close(release)
				t.Fatal("actual compilation did not reach the prepared boundary")
			}
			switch action {
			case "pause", "revoke":
				productApply(t, s, action, WikiIncrementalJobID, "")
			case "source":
				productWikiCommit(t, s, "private-wiki-body: SUPERSEDING_SOURCE")
			case "lease":
				if err := releaseActualProductLease(s); err != nil {
					t.Fatal(err)
				}
			}
			close(release)
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("lost source/control/lease grant completed successfully")
				}
			case <-time.After(45 * time.Second):
				t.Fatal("publication lock order deadlocked")
			}
			s.engine.FaultPoint = nil
			after, err := s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
			if err != nil || after.Generation != pending.Generation || after.State != "stale" {
				t.Fatalf("denied pointer became current: %+v %v", after, err)
			}
			receipts, err := s.receipts.Receipts(s.workspaceID, WikiIncrementalJobID)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range receipts {
				if r.EventID == pending.EventID && r.State == ReceiptSucceeded {
					t.Fatal("denied occurrence published success")
				}
			}
		})
	}
}

func TestProductWikiTerminalFencesAfterActualEffect(t *testing.T) {
	for _, action := range []string{"pause", "revoke", "source", "lease"} {
		t.Run(action, func(t *testing.T) {
			s, active := activeProductWiki(t)
			pending, err := s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
			if err != nil {
				t.Fatal(err)
			}
			p := &productExecution{service: s, jobID: WikiIncrementalJobID, revision: active.Revision, source: pending.SourceIdentity}
			worker := s.worker(p)
			worker.Jobs = nil // The public Worker receives an event, not a calendar.
			worker.LocalQualification = map[string]string{WikiIncrementalJobID: active.Qualification.ID}
			worker.ActivatedJobs = []string{WikiIncrementalJobID}
			actual := worker.Handlers[WikiIncrementalJobID].(Handler)
			worker.Handlers[WikiIncrementalJobID] = HandlerFunc(func(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
				out, err := actual.ExecuteAuthorized(ctx, command, grant)
				if err != nil || out.State != ReceiptSucceeded {
					return out, err
				}
				switch action {
				case "pause", "revoke":
					r, err := s.Review(Operation{Kind: action, JobID: WikiIncrementalJobID, ExpectedRevision: active.Revision})
					if err != nil {
						return HandlerResult{}, err
					}
					_, err = s.Apply(ctx, r, approveProduct)
					if err != nil {
						return HandlerResult{}, err
					}
				case "source":
					input, err := s.engine.SealOwnedCapture(s.workspaceID, "", 0, "private-wiki-body: AFTER_EFFECT_SOURCE", s.now())
					if err != nil {
						return HandlerResult{}, err
					}
					if _, err := s.engine.CaptureOwned(s.workspaceID, input); err != nil {
						return HandlerResult{}, err
					}
					if _, err := s.engine.DreamDailyAttested(ctx, s.workspaceID, s.now()); err != nil {
						return HandlerResult{}, err
					}
				case "lease":
					if err := releaseActualProductLease(s); err != nil {
						return HandlerResult{}, err
					}
				}
				return out, nil
			})
			out, err := worker.Run(context.Background(), WakeRequest{WorkspaceID: s.workspaceID, Trigger: TriggerEvent, EventID: pending.EventID, Now: s.now(), OwnerID: "wiki-terminal-witness", Preauthorized: true, IdleState: IdleConfirmed})
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range out.Receipts {
				if r.JobID == WikiIncrementalJobID && r.State == ReceiptSucceeded {
					t.Fatal("rejected terminal publication was presented as success")
				}
			}
			receipts, err := s.receipts.Receipts(s.workspaceID, WikiIncrementalJobID)
			if err != nil {
				t.Fatal(err)
			}
			for _, r := range receipts {
				if r.EventID == pending.EventID && r.State == ReceiptSucceeded {
					t.Fatal("lost grant left a durable terminal success")
				}
			}
		})
	}
}

func TestProductWikiDiagnosticPointerHonorsDeniedPublication(t *testing.T) {
	s := productFixture(t)
	productWikiCommit(t, s, "private-wiki-body: FIRST_DIAGNOSTIC_SOURCE")
	productApply(t, s, "qualify", WikiReconcileJobID, "")
	productApply(t, s, "qualify", WikiIntegrityJobID, "")
	store := DiagnosticStore{Root: s.receipts.Root}
	old, path, err := store.Load(s.workspaceID, WikiIntegrityJobID, "")
	if err != nil {
		t.Fatal(err)
	}
	productWikiCommit(t, s, "private-wiki-body: NEXT_DIAGNOSTIC_SOURCE")
	snapshot, err := s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.engine.CompilePrivateWiki(context.Background(), s.workspaceID, snapshot.Parent); err != nil {
		t.Fatal(err)
	}
	snapshot, err = s.engine.InspectPrivateWikiMaintenance(s.workspaceID)
	if err != nil {
		t.Fatal(err)
	}
	next := s.wikiReport(WikiIntegrityJobID, snapshot, Command{RequestedAt: s.now()})
	denied := errors.New("actual publication grant denied")
	if changed, err := store.Publish(next, func(func() error) error { return denied }); !errors.Is(err, denied) || changed {
		t.Fatal("denied diagnostic became current")
	}
	after, afterPath, err := store.Load(s.workspaceID, WikiIntegrityJobID, "")
	if err != nil || afterPath != path || after.SourceSHA256 != old.SourceSHA256 {
		t.Fatal("denied report replaced durable evidence")
	}
}

func TestProductWikiEventOnlyControlAndReceiptRefuseTampering(t *testing.T) {
	for _, damage := range []string{"calendar", "receipt-missing", "receipt-hardlink"} {
		t.Run(damage, func(t *testing.T) {
			s := productFixture(t)
			productWikiCommit(t, s, "private-wiki-body: EVENT_PROOF_SOURCE")
			q := productApply(t, s, "qualify", WikiIncrementalJobID, "")
			active := productApply(t, s, "enroll", WikiIncrementalJobID, "UTC")
			if damage == "calendar" {
				c, err := s.loadControl(WikiIncrementalJobID)
				if err != nil {
					t.Fatal(err)
				}
				c.Schedule.EventOnly = false
				if err := s.saveControl(c); err != nil {
					t.Fatal(err)
				}
				before := productSnapshot(t, s.home)
				if _, err := s.Review(Operation{Kind: "pause", JobID: WikiIncrementalJobID, ExpectedRevision: active.Revision}); !errors.Is(err, ErrProductConflict) {
					t.Fatal("event-only record silently became calendar authority")
				}
				if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
					t.Fatal("unsafe control was repaired")
				}
				return
			}
			r := q.Qualification.Receipt
			path := filepath.Join(s.receipts.Root, "workspaces", s.workspaceID, "receipts", WikiIncrementalJobID, r.CommandID+"--"+r.AttemptID+".json")
			if damage == "receipt-missing" {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Link(path, filepath.Join(t.TempDir(), "outside-event-proof.json")); err != nil {
				t.Fatal(err)
			}
			before := productSnapshot(t, s.home)
			if err := s.receipts.VerifyEventReceipt(r); err == nil {
				t.Fatal("unsafe terminal event evidence accepted")
			}
			if _, err := s.Review(Operation{Kind: "resume", JobID: WikiIncrementalJobID, ExpectedRevision: active.Revision}); err == nil {
				t.Fatal("unsafe event evidence granted authority")
			}
			if !reflect.DeepEqual(before, productSnapshot(t, s.home)) {
				t.Fatal("event evidence check mutated private state")
			}
		})
	}
}
