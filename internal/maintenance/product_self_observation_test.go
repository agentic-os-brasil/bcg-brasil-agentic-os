package maintenance

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestProductSelfObservationDeclineStaleAndNoOpCannotQualify(t *testing.T) {
	s := productFixture(t)
	if _, err := ownerctx.Initialize(filepath.Join(s.home, "brain")); err != nil {
		t.Fatal(err)
	}
	before := productSnapshot(t, s.home)
	review, err := s.Review(Operation{Kind: "qualify", JobID: SelfObservationCaptureJobID, ExpectedRevision: 0})
	if err != nil {
		t.Fatal(err)
	}
	declined := errors.New("synthetic owner declined")
	if _, err := s.Apply(context.Background(), review, func(context.Context, ProductReview) error { return declined }); !errors.Is(err, declined) {
		t.Fatalf("decline=%v", err)
	}
	after := productSnapshot(t, s.home)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("declined qualification changed private state")
	}
	if _, err := s.Apply(context.Background(), review, func(context.Context, ProductReview) error {
		queueSelfDeclaration(t, s, "prefer.changed-source")
		return nil
	}); !errors.Is(err, ErrProductConflict) {
		t.Fatalf("stale source review=%v", err)
	}
	observations, err := ownerctx.ListObservations(filepath.Join(s.home, "brain"))
	if err != nil || len(observations) != 0 {
		t.Fatalf("stale source captured: %#v %v", observations, err)
	}
	applySelfJob(t, s, "qualify")
	status := selfJobStatus(t, s)
	if status.QualificationState != "qualified" {
		t.Fatalf("qualification=%#v", status)
	}
	review, err = s.Review(Operation{Kind: "qualify", JobID: SelfObservationCaptureJobID, ExpectedRevision: status.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(context.Background(), review, approveProduct); !errors.Is(err, ErrQualificationRequired) {
		t.Fatalf("no-op qualification=%v", err)
	}
}

func TestProductSelfObservationPartialDomainCommitCannotQualifyOnReplay(t *testing.T) {
	s := productFixture(t)
	queued := queueSelfDeclaration(t, s, "prefer.partial")
	root := filepath.Join(s.home, "brain")
	s.options.InstalledGuard = func(write func() error) error {
		observations, err := ownerctx.ListObservations(root)
		if err != nil {
			return err
		}
		if len(observations) > 0 {
			return errors.New("synthetic post-domain publication interruption")
		}
		return write()
	}
	review, err := s.Review(Operation{Kind: "qualify", JobID: SelfObservationCaptureJobID, ExpectedRevision: 0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(context.Background(), review, approveProduct); err == nil {
		t.Fatal("partial commit qualified")
	}
	observation, err := ownerctx.GetObservation(root, queued.ObservationID)
	if err != nil || observation.State != ownerctx.ObservationCaptured {
		t.Fatalf("real partial domain result missing: %#v %v", observation, err)
	}
	s.options.InstalledGuard = nil
	status := selfJobStatus(t, s)
	if status.Qualification != nil {
		t.Fatal("failed worker minted qualification")
	}
	review, err = s.Review(Operation{Kind: "qualify", JobID: SelfObservationCaptureJobID, ExpectedRevision: status.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(context.Background(), review, approveProduct); !errors.Is(err, ErrQualificationRequired) {
		t.Fatalf("replay qualified partial work: %v", err)
	}
	observations, err := ownerctx.ListObservations(root)
	if err != nil || len(observations) != 1 || observations[0].Revision != observation.Revision {
		t.Fatalf("partial replay duplicated/changed canonical result: %#v %v", observations, err)
	}
}

func TestProductSelfObservationSharedOwnerConcurrentWakeAndRevoke(t *testing.T) {
	for _, testcase := range []struct {
		name       string
		forcedBusy bool
	}{{"native_concurrency", false}, {"controlled_publication_busy", true}} {
		t.Run(testcase.name, func(t *testing.T) {
			s := productFixture(t)
			queueSelfDeclaration(t, s, "prefer.initial")
			applySelfJob(t, s, "qualify")
			applySelfJob(t, s, "enroll")
			otherPath := filepath.Join(filepath.Dir(s.options.WorkspacePath), "workspace-concurrent")
			if _, err := workspace.Initialize(workspace.Options{WorkspacePath: otherPath, DataRoot: filepath.Join(s.home, "state")}); err != nil {
				t.Fatal(err)
			}
			options := s.options
			options.WorkspacePath = otherPath
			options.Now = s.now
			other, err := NewService(options)
			if err != nil {
				t.Fatal(err)
			}
			queued := queueSelfDeclaration(t, s, "prefer.concurrent")
			beforeSource, err := ownerctx.InspectSelfObservationSource(filepath.Join(s.home, "brain"), s.now())
			if err != nil {
				t.Fatal(err)
			}
			beforeCanonical, err := ownerctx.ListObservations(filepath.Join(s.home, "brain"))
			if err != nil {
				t.Fatal(err)
			}
			if testcase.forcedBusy {
				busy := func(func() error) error { return scheduler.ErrLeaseBusy }
				s.options.InstalledGuard = busy
				other.options.InstalledGuard = busy
			}
			now := s.now().Add(25 * time.Hour)
			s.now = func() time.Time { return now }
			other.now = s.now
			var wait sync.WaitGroup
			type wakeResult struct {
				report WakeReport
				err    error
			}
			results := make(chan wakeResult, 2)
			for _, service := range []*Service{s, other} {
				wait.Add(1)
				go func(service *Service) {
					defer wait.Done()
					report, err := service.RunDue(context.Background())
					results <- wakeResult{report, err}
				}(service)
			}
			wait.Wait()
			close(results)
			var evidence []wakeResult
			for result := range results {
				evidence = append(evidence, result)
				err := result.err
				if err != nil && !errors.Is(err, ErrProductConflict) && !errors.Is(err, scheduler.ErrLeaseBusy) {
					t.Fatal(err)
				}
			}
			if testcase.forcedBusy {
				s.options.InstalledGuard = nil
				other.options.InstalledGuard = nil
			}
			observations, err := ownerctx.ListObservations(filepath.Join(s.home, "brain"))
			if err != nil || len(observations) < 1 || len(observations) > 2 {
				t.Fatalf("owner concurrent capture count: %#v %v reports=%#v", observations, err, evidence)
			}
			// A later wake may replay the exact stored terminal receipt without
			// executing again. Count distinct full receipts, not response copies.
			successes := map[Receipt]bool{}
			for _, result := range evidence {
				for _, receipt := range result.report.Receipts {
					if receipt.State == ReceiptSucceeded {
						successes[receipt] = true
					}
				}
			}
			if len(observations) == 1 {
				// Independent nonblocking guard contention can refuse both clients.
				// This is pending work, never success; one later ordinary wake must
				// recover that SAME occurrence without advancing time or retry loops.
				for _, result := range evidence {
					if !errors.Is(result.err, scheduler.ErrLeaseBusy) || len(result.report.Due) != 1 {
						t.Fatalf("uncaptured work was not truthfully busy/pending: %#v", evidence)
					}
				}
				afterSource, err := ownerctx.InspectSelfObservationSource(filepath.Join(s.home, "brain"), s.now())
				if err != nil || !reflect.DeepEqual(beforeSource, afterSource) || !reflect.DeepEqual(beforeCanonical, observations) {
					t.Fatal("dual refusal changed authoritative owner source", err)
				}
				if len(successes) != 0 {
					t.Fatal("concurrent wake claimed success without canonical capture", evidence)
				}
				recovered, err := s.RunDue(context.Background())
				if err != nil || len(recovered.Due) != 1 || recovered.Due[0] != evidence[0].report.Due[0] || len(recovered.Receipts) != 1 || recovered.Receipts[0].State != ReceiptSucceeded {
					t.Fatalf("single ordinary wake did not recover same pending occurrence: %#v %v", recovered, err)
				}
				expectedOccurrence := (Command{JobID: SelfObservationCaptureJobID, Trigger: TriggerPresence, ScheduledFor: evidence[0].report.Due[0].ScheduledFor}).OccurrenceDigest()
				if recovered.Receipts[0].OccurrenceDigest != expectedOccurrence {
					t.Fatal("recovery minted a different occurrence", recovered)
				}
				for _, result := range evidence {
					for _, receipt := range result.report.Receipts {
						if receipt.CommandID != "" && receipt.OccurrenceDigest != expectedOccurrence {
							t.Fatal("concurrent clients disagreed about occurrence", evidence)
						}
					}
				}
				successes[recovered.Receipts[0]] = true
				t.Log("one ordinary wake recovered the same contended owner occurrence")
				observations, err = ownerctx.ListObservations(filepath.Join(s.home, "brain"))
			}
			if len(successes) != 1 {
				t.Fatal("concurrent capture lacks exactly one truthful successful receipt", evidence)
			}
			if err != nil || len(observations) != 2 {
				t.Fatalf("owner occurrence did not commit exactly once: %#v %v", observations, err)
			}
			owner := s.selfObservationService()
			var succeeded Receipt
			for receipt := range successes {
				succeeded = receipt
			}
			durable, err := owner.receipts.Receipts(owner.workspaceID, SelfObservationCaptureJobID)
			if err != nil {
				t.Fatal(err)
			}
			committed := 0
			for _, receipt := range durable {
				if receipt.OccurrenceDigest == succeeded.OccurrenceDigest && receipt.State == ReceiptSucceeded {
					committed++
					if receipt != succeeded {
						t.Fatal("successful response differs from the original durable receipt", receipt, succeeded)
					}
				}
			}
			calendar, err := owner.scheduler.Receipts(owner.workspaceID)
			if err != nil {
				t.Fatal(err)
			}
			calendarCommitted := 0
			for _, receipt := range calendar {
				occurrence := (Command{JobID: receipt.JobID, Trigger: TriggerPresence, ScheduledFor: receipt.ScheduledFor}).OccurrenceDigest()
				if occurrence == succeeded.OccurrenceDigest && receipt.State == scheduler.Succeeded {
					calendarCommitted++
					if !receipt.AttemptedAt.Equal(succeeded.RecordedAt) {
						t.Fatal("calendar completion belongs to a different attempt", receipt, succeeded)
					}
				}
			}
			if committed != 1 || calendarCommitted != 1 {
				t.Fatalf("owner occurrence has distinct completions: command=%d calendar=%d", committed, calendarCommitted)
			}
			captured, err := ownerctx.GetObservation(filepath.Join(s.home, "brain"), queued.ObservationID)
			if err != nil {
				t.Fatal(err)
			}
			capture, _, err := owner.readSelfObservationReport("")
			if err != nil || !reflect.DeepEqual(capture.Observations, []selfObservationVersion{{queued.ID, queued.ObservationID, captured.Revision}}) {
				t.Fatal("completion does not read back the exact reviewed canonical version", capture, err)
			}
			if _, err := other.RunDue(context.Background()); err != nil {
				t.Fatal(err)
			}
			again, err := ownerctx.ListObservations(filepath.Join(s.home, "brain"))
			if err != nil || !reflect.DeepEqual(observations, again) {
				t.Fatal("same occurrence duplicated after successful wake", err)
			}
			applySelfJob(t, other, "revoke")
			queueSelfDeclaration(t, s, "prefer.after-revoke")
			now = now.Add(25 * time.Hour)
			if _, err := s.RunDue(context.Background()); err != nil {
				t.Fatal(err)
			}
			observations, err = ownerctx.ListObservations(filepath.Join(s.home, "brain"))
			if err != nil || len(observations) != 2 {
				t.Fatalf("other workspace escaped revoke: %#v %v", observations, err)
			}
		})
	}
}

func queueSelfDeclaration(t *testing.T, s *Service, claim string) ownerctx.SelfObservationIntakeReceipt {
	t.Helper()
	root := filepath.Join(s.home, "brain")
	if _, err := ownerctx.Initialize(root); err != nil {
		t.Fatal(err)
	}
	input, err := ownerctx.NewSelfObservationDeclaration(root, ownerctx.SelfObservationDeclaration{Signal: ownerctx.SignalExplicitInstruction, Facet: "voice", Claim: claim, Confidence: 1, ExpiresAt: s.now().Add(7 * 24 * time.Hour)})
	if err != nil {
		t.Fatal(err)
	}
	review, err := ownerctx.ReviewSelfObservation(root, input)
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := ownerctx.ApproveSelfObservation(context.Background(), root, review, true)
	if err != nil {
		t.Fatal(err)
	}
	return receipt
}

func selfJobStatus(t *testing.T, s *Service) ProductJobStatus {
	t.Helper()
	status, err := s.Status()
	if err != nil {
		t.Fatal(err)
	}
	for _, job := range status.Jobs {
		if job.JobID == "self-observation-capture" {
			return job
		}
	}
	t.Fatal("self job absent")
	return ProductJobStatus{}
}
func applySelfJob(t *testing.T, s *Service, kind string) ProductJobStatus {
	t.Helper()
	status := selfJobStatus(t, s)
	zone := ""
	if kind == "enroll" {
		zone = "UTC"
	}
	review, err := s.Review(Operation{Kind: kind, JobID: "self-observation-capture", ExpectedRevision: status.Revision, Timezone: zone})
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Apply(context.Background(), review, approveProduct)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestProductSelfObservationCaptureHasActualOwnerDomainEvidence(t *testing.T) {
	s := productFixture(t)
	queued := queueSelfDeclaration(t, s, "prefer.concise")
	root := filepath.Join(s.home, "brain")
	before, err := ownerctx.ListObservations(root)
	if err != nil || len(before) != 0 {
		t.Fatalf("intake captured before job: %#v %v", before, err)
	}
	facet := filepath.Join(root, "owner/self/voice.md")
	facetBefore, err := os.ReadFile(facet)
	if err != nil {
		t.Fatal(err)
	}
	review, err := s.Review(Operation{Kind: "qualify", JobID: "self-observation-capture", ExpectedRevision: 0})
	if err != nil {
		t.Fatalf("reviewed owner capture binding missing: %v", err)
	}
	qualified, err := s.Apply(context.Background(), review, approveProduct)
	if err != nil {
		t.Fatal(err)
	}
	if qualified.Qualification == nil || qualified.QualificationState != "qualified" || qualified.Qualification.WorkspaceID == s.workspaceID || !strings.HasPrefix(qualified.Qualification.WorkspaceID, "owner-") {
		t.Fatalf("owner qualification=%#v", qualified)
	}
	after, err := ownerctx.GetObservation(root, queued.ObservationID)
	if err != nil || after.State != ownerctx.ObservationCaptured {
		t.Fatalf("canonical=%#v %v", after, err)
	}
	if err := ownerctx.VerifySelfObservationVersion(root, queued.ID, after.ID, after.Revision); err != nil {
		t.Fatal(err)
	}
	facetAfter, err := os.ReadFile(facet)
	if err != nil || string(facetAfter) != string(facetBefore) {
		t.Fatalf("capture changed active SELF: %v", err)
	}
	status := selfJobStatus(t, s)
	replay, err := s.Review(Operation{Kind: "qualify", JobID: "self-observation-capture", ExpectedRevision: status.Revision})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Apply(context.Background(), replay, approveProduct); !errors.Is(err, ErrQualificationRequired) {
		t.Fatalf("replay qualified without new result: %v", err)
	}
}

func TestProductSelfObservationUsesOneOwnerControlAcrossWorkspaces(t *testing.T) {
	s := productFixture(t)
	queueSelfDeclaration(t, s, "prefer.first")
	qualified := applySelfJob(t, s, "qualify")
	otherPath := filepath.Join(filepath.Dir(s.options.WorkspacePath), "workspace-other")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: otherPath, DataRoot: filepath.Join(s.home, "state")}); err != nil {
		t.Fatal(err)
	}
	options := s.options
	options.WorkspacePath = otherPath
	options.Now = s.now
	other, err := NewService(options)
	if err != nil {
		t.Fatal(err)
	}
	seen := selfJobStatus(t, other)
	if seen.Revision != qualified.Revision || seen.Qualification == nil || seen.Qualification.ID != qualified.Qualification.ID {
		t.Fatalf("owner state diverged: %#v %#v", qualified, seen)
	}
	stale, err := s.Review(Operation{Kind: "enroll", JobID: "self-observation-capture", ExpectedRevision: seen.Revision, Timezone: "UTC"})
	if err != nil {
		t.Fatal(err)
	}
	applySelfJob(t, other, "enroll")
	if _, err := s.Apply(context.Background(), stale, approveProduct); !errors.Is(err, ErrProductConflict) {
		t.Fatalf("second workspace enrolled same owner twice: %v", err)
	}
	applySelfJob(t, other, "pause")
	queueSelfDeclaration(t, s, "prefer.second")
	now := s.now().Add(25 * time.Hour)
	s.now = func() time.Time { return now }
	other.now = s.now
	if _, err := s.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	observations, err := ownerctx.ListObservations(filepath.Join(s.home, "brain"))
	if err != nil || len(observations) != 1 {
		t.Fatalf("other workspace escaped pause: %#v %v", observations, err)
	}
	applySelfJob(t, s, "resume")
	if _, err := other.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	observations, err = ownerctx.ListObservations(filepath.Join(s.home, "brain"))
	if err != nil || len(observations) != 2 {
		t.Fatalf("owner resume did not capture pending input: %#v %v", observations, err)
	}
	if _, err := s.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	observations, err = ownerctx.ListObservations(filepath.Join(s.home, "brain"))
	if err != nil || len(observations) != 2 {
		t.Fatalf("owner occurrence duplicated: %#v %v", observations, err)
	}
}
