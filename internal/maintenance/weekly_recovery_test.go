package maintenance

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Synthetic PRIVATE trusted-store setup exercises recovery, not native
// qualification. No fake qualifier, provider call or test override ships.
func weeklyTrustedRecoveryFixture(t *testing.T) (*Service, productControl, scheduler.Occurrence, ownerctx.WeeklySelfInput, Receipt, *int) {
	t.Helper()
	s := productFixture(t).weeklyModelService()
	now := s.now().UTC()
	brain := filepath.Join(s.home, "brain")
	if _, e := ownerctx.Initialize(brain); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(brain, "owner/self/communication-style.md"), []byte("# Communication style\n\n## Current\n\nprivate-weekly-source-body\n"), 0600); e != nil {
		t.Fatal(e)
	}
	registration := weeklyRegistrationFixture()
	registration.WorkingDirectory = s.options.WorkspacePath
	registration.Configuration = WeeklyConfiguration{{Path: filepath.Join(s.home, "synthetic-settings.json"), Role: "settings", Absent: true}, {Path: filepath.Join(s.home, "synthetic-policy.json"), Role: "managed_policy", Absent: true}}
	sort.Slice(registration.Configuration, func(i, j int) bool { return registration.Configuration[i].Path < registration.Configuration[j].Path })
	registration.Descriptor.ConfigurationSHA256 = registration.Configuration.Digest("settings")
	registration.Descriptor.ManagedPolicySHA256 = registration.Configuration.Digest("managed_policy")
	registration.Descriptor.RuntimeIdentity = &processidentity.WeeklyNativeDeclaration{Topology: processidentity.WeeklyNativeDarwinStandalone, Architecture: "arm64", TeamID: "SYNTHETIC1", SigningIdentifier: "synthetic.runtime", CodeDirectoryHash: strings.Repeat("a", 40)}
	if e := registration.Validate(); e != nil {
		t.Fatal(e)
	}
	c, _ := s.loadControl(YodaSelfReviewWeeklyJobID)
	c.Revision = 1
	c.Weekly = &weeklyModelControl{Registration: registration, RegistrationReviewSHA256: strings.Repeat("b", 64)}
	if e := s.withControl(c.JobID, func() error { return s.saveControl(c) }); e != nil {
		t.Fatal(e)
	}
	at := now.Add(-time.Hour)
	probe := Receipt{SchemaVersion: 1, AttemptID: strings.Repeat("c", 32), CommandID: "synthetic-prior-probe", JobID: c.JobID, WorkspaceID: s.workspaceID, Trigger: TriggerWeekly, State: ReceiptSucceeded, RecordedAt: at, Deadline: at.Add(time.Minute), WeeklyMode: WeeklyNativeProbe, ReasonCode: ReasonCompleted}
	probe.OccurrenceDigest = (Command{JobID: probe.JobID, Trigger: TriggerWeekly, ScheduledFor: at}).OccurrenceDigest()
	evidence := WeeklyNativeEvidence{SchemaVersion: 1, AttemptID: probe.AttemptID, CommandID: probe.CommandID, OccurrenceSHA256: probe.OccurrenceDigest, OwnerScope: s.workspaceID, ImplementationSHA256: s.build, RegistrationSHA256: registration.Digest(), InputSHA256: strings.Repeat("d", 64), PolicySHA256: strings.Repeat("e", 64), ChildIdentitySHA256: strings.Repeat("f", 64), ResponseSHA256: strings.Repeat("a", 64), CompletedAt: at.Add(time.Millisecond)}

	if e := s.writeWeeklyEvidence(evidence); e != nil {
		t.Fatal("evidence", e)
	}
	if e := s.receipts.AppendReceipt(probe); e != nil {
		t.Fatal("maintenance", e)
	}
	if e := s.scheduler.AppendReceipt(s.workspaceID, scheduler.Receipt{JobID: c.JobID, ScheduledFor: at, AttemptedAt: at, State: scheduler.Succeeded}); e != nil {
		t.Fatal("scheduler", e)
	}
	sha, artifact, e := s.weeklyDomainEvidence()
	if e != nil {
		t.Fatal(e)
	}
	q := ProductQualification{WorkspaceID: s.workspaceID, JobID: c.JobID, Binding: s.binding, Implementation: s.build, Configuration: s.configuration(c.JobID), Adapter: s.options.Adapter, Platform: runtime.GOOS + "/" + runtime.GOARCH, Artifact: artifact, ArtifactSHA256: sha, Receipt: probe, ScheduledFor: at, RequestedAt: at, VerifiedAt: at.Add(time.Second)}
	q.ID = productDigest(q)
	c.Qualification = &q
	c.State = "active"
	job, _ := findCatalogJob(s.options.Catalog, c.JobID)
	c.Writes = append([]string(nil), job.Writes...)
	c.Revision = 2
	c.Schedule = scheduleFor(c.JobID)
	c.Schedule.Timezone = "UTC"
	c.Boundary = now.Add(-time.Minute)
	if e := s.withControl(c.JobID, func() error { return s.saveControl(c) }); e != nil {
		t.Fatal(e)
	}
	if e := s.verifyQualification(&q); e != nil {
		t.Fatal("synthetic prior-state fixture invalid", e)
	}
	occ := scheduler.Occurrence{JobID: c.JobID, ScheduledFor: now}
	options := registration.Policy
	options.ScheduledFor = now
	options.OccurrenceID = productDigest([]any{s.workspaceID, c.JobID, now})
	input, e := ownerctx.FreezeWeeklySelf(brain, options, nil, now)
	if e != nil {
		t.Fatal(e)
	}
	original := Receipt{SchemaVersion: 1, AttemptID: strings.Repeat("d", 32), CommandID: "synthetic-original-working", JobID: c.JobID, WorkspaceID: s.workspaceID, Trigger: TriggerWeekly, State: ReceiptSucceeded, RecordedAt: now, Deadline: now.Add(time.Minute), WeeklyMode: WeeklyWorking, ReasonCode: ReasonCompleted}
	original.OccurrenceDigest = (Command{JobID: c.JobID, Trigger: TriggerWeekly, ScheduledFor: now}).OccurrenceDigest()
	calls := new(int)
	s.options.WeeklyNativeExecutor = func(context.Context, WeeklyNativeAttemptRequest, *WeeklyAttemptSession) (ownerctx.WeeklySelfCandidate, error) {
		*calls++
		t.Error("reconciliation called the model")
		return ownerctx.WeeklySelfCandidate{}, ErrQualificationRequired
	}
	return s, c, occ, input, original, calls
}
func TestWeeklyRecoveryCompletesOriginalTerminalWithoutModel(t *testing.T) {
	for _, maintenanceOnly := range []bool{false, true} {
		t.Run(map[bool]string{false: "both-missing", true: "maintenance-only"}[maintenanceOnly], func(t *testing.T) {
			s, c, occ, input, original, calls := weeklyTrustedRecoveryFixture(t)
			candidate := ownerctx.WeeklySelfCandidate{SchemaVersion: 1, InputSHA256: input.InputSHA256, CanonicalSHA256: input.Facets.CanonicalSourceDigest, PromptHistorySHA256: input.PromptHistorySHA256, PolicySHA256: input.PolicySHA256, TranslationSHA256: input.TranslationSHA256, Facets: map[string]string{"communication-style": "private-weekly-working-body"}}
			execution := weeklyFixtureExecution(s, c, original)
			if _, e := ownerctx.PublishWeeklySelfWithExecution(context.Background(), filepath.Join(s.home, "brain"), input.InputSHA256, candidate, input.ExpectedRevision, execution, func(write func() error) error { return write() }, s.now()); e != nil {
				t.Fatal(e)
			}
			if maintenanceOnly {
				if e := s.receipts.AppendReceipt(original); e != nil {
					t.Fatal(e)
				}
			}
			retryAt := s.now().Add(time.Second)
			s.now = func() time.Time { return retryAt }
			got, e := s.execute(context.Background(), &productExecution{service: s, jobID: c.JobID, revision: c.Revision}, occ, retryAt)
			if e != nil {
				t.Fatal("committed occurrence could not close original terminal metadata", e)
			}
			if got != original || *calls != 0 {
				t.Fatal("recovery replaced original receipt or invoked model")
			}
		})
	}
}

func weeklyFixtureExecution(s *Service, c productControl, r Receipt) ownerctx.WeeklySelfExecution {
	return ownerctx.WeeklySelfExecution{RegistrationSHA256: c.Weekly.Registration.Digest(), QualificationSHA256: c.Qualification.ID, ActivationSHA256: productDigest([]any{s.binding, c.Revision, c.Schedule, c.Boundary, c.Weekly.RegistrationReviewSHA256, c.Qualification.ID}), ImplementationSHA256: s.build, OwnerScope: s.workspaceID, CommandID: r.CommandID, AttemptID: r.AttemptID, OccurrenceSHA256: r.OccurrenceDigest, RequestedAt: r.RecordedAt, Deadline: r.Deadline}
}

func TestWeeklyRecoveryRealWorkerFaultReopenAndReplay(t *testing.T) {
	for _, schedulerFault := range []bool{false, true} {
		t.Run(map[bool]string{false: "maintenance-write-fault", true: "scheduler-write-fault"}[schedulerFault], func(t *testing.T) {
			s, c, occ, input, _, calls := weeklyTrustedRecoveryFixture(t)
			ctx := context.Background()
			p := &productExecution{service: s, jobID: c.JobID, revision: c.Revision, ctx: ctx}
			if e := p.selectWeekly(ctx, occ, s.now()); e != nil {
				t.Fatal(e)
			}
			w := s.worker(p)
			var original Receipt
			var fault string
			// Test-only domain handler seeds a committed effect under the real worker's
			// grant, then obstructs its real terminal store. It is not a native adapter.
			w.Handlers[c.JobID] = HandlerFunc(func(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
				if e := ValidateExecutionGrant(grant, command); e != nil {
					return HandlerResult{}, e
				}
				bounded := grant.(executionGrant)
				original = Receipt{SchemaVersion: 1, AttemptID: bounded.attemptID, OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: ReceiptSucceeded, RecordedAt: command.RequestedAt, Deadline: command.Deadline, WeeklyMode: command.WeeklyMode, ReasonCode: ReasonCompleted}
				candidate := ownerctx.WeeklySelfCandidate{SchemaVersion: 1, InputSHA256: input.InputSHA256, CanonicalSHA256: input.Facets.CanonicalSourceDigest, PromptHistorySHA256: input.PromptHistorySHA256, PolicySHA256: input.PolicySHA256, TranslationSHA256: input.TranslationSHA256, Facets: map[string]string{"communication-style": "private-weekly-working-body"}}
				_, e := ownerctx.PublishWeeklySelfWithExecution(ctx, filepath.Join(s.home, "brain"), input.InputSHA256, candidate, input.ExpectedRevision, weeklyFixtureExecution(s, c, original), func(write func() error) error { return PublishExecutionGrant(ctx, grant, command, write) }, s.now())
				if e != nil {
					return HandlerResult{}, e
				}
				fault = filepath.Join(s.receipts.Root, "workspaces", s.workspaceID, "receipts", c.JobID, original.CommandID+"--"+original.AttemptID+".json")
				if schedulerFault {
					name := original.RecordedAt.UTC().Format("20060102T150405.000000000Z") + "-" + occ.ScheduledFor.UTC().Format("20060102T150405.000000000Z") + ".json"
					fault = filepath.Join(s.scheduler.Root, "workspaces", s.workspaceID, "receipts", c.JobID, name)
				}
				if e := os.MkdirAll(fault, 0700); e != nil {
					return HandlerResult{}, e
				}
				return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
			})
			authority := ExecutionAuthority{catalog: s.options.Catalog, occurrences: map[string]OccurrenceAuthorization{}, activated: map[string]bool{c.JobID: true}, preauthorized: true, ready: true, productBound: true}
			if e := authority.addOccurrence(OccurrenceAuthorization{WorkspaceID: s.workspaceID, JobID: c.JobID, Trigger: TriggerWeekly, ScheduledFor: occ.ScheduledFor}); e != nil {
				t.Fatal(e)
			}
			if _, e := w.runOccurrence(ctx, WakeRequest{WorkspaceID: s.workspaceID, Trigger: TriggerWeekly, OwnerID: "synthetic-worker", Preauthorized: true, IdleState: IdleConfirmed}, s.now(), occ, authority); e == nil {
				t.Fatal("terminal storage fault was not exercised")
			}
			path := filepath.Join(s.home, "brain/owner/self/weekly/current.json")
			before, e := os.ReadFile(path)
			if e != nil {
				t.Fatal(e)
			}
			if e = os.Remove(fault); e != nil {
				t.Fatal(e)
			}
			reopened, e := NewService(s.options)
			if e != nil {
				t.Fatal(e)
			}
			reopened = reopened.weeklyModelService()
			for i := 0; i < 2; i++ {
				got, e := reopened.execute(ctx, &productExecution{service: reopened, jobID: c.JobID, revision: c.Revision}, occ, reopened.now())
				if e != nil || got != original {
					t.Fatalf("reopen/replay lost original effect: %#v %v", got, e)
				}
			}
			after, e := os.ReadFile(path)
			if e != nil || string(before) != string(after) || *calls != 0 {
				t.Fatal("reconciliation selected/rewrote input, effect or model")
			}
			receipts, e := reopened.receipts.Receipts(s.workspaceID, c.JobID)
			if e != nil {
				t.Fatal(e)
			}
			count := 0
			for _, r := range receipts {
				if r == original {
					count++
				}
			}
			if count != 1 {
				t.Fatal("original terminal receipt duplicated", count)
			}
		})
	}
}

func TestWeeklyRecoveryCurrentFencesRefuseInvalidEffects(t *testing.T) {
	for _, kind := range []string{"cancelled", "revoked", "source-corrected", "expired", "registration-changed", "unattested-effect", "lease-busy", "qualification-missing", "failed-original"} {
		t.Run(kind, func(t *testing.T) {
			s, c, occ, input, original, calls := weeklyTrustedRecoveryFixture(t)
			ctx := context.Background()
			candidate := ownerctx.WeeklySelfCandidate{SchemaVersion: 1, InputSHA256: input.InputSHA256, CanonicalSHA256: input.Facets.CanonicalSourceDigest, PromptHistorySHA256: input.PromptHistorySHA256, PolicySHA256: input.PolicySHA256, TranslationSHA256: input.TranslationSHA256, Facets: map[string]string{"communication-style": "private-weekly-working-body"}}
			if kind == "unattested-effect" {
				if _, e := ownerctx.PublishWeeklySelf(ctx, filepath.Join(s.home, "brain"), input.InputSHA256, candidate, input.ExpectedRevision, func(write func() error) error { return write() }, s.now()); e != nil {
					t.Fatal(e)
				}
			} else {
				if _, e := ownerctx.PublishWeeklySelfWithExecution(ctx, filepath.Join(s.home, "brain"), input.InputSHA256, candidate, input.ExpectedRevision, weeklyFixtureExecution(s, c, original), func(write func() error) error { return write() }, s.now()); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "cancelled" {
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			if kind == "revoked" {
				c.State = "revoked"
				c.Revision++
				if e := s.withControl(c.JobID, func() error { return s.saveControl(c) }); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "source-corrected" {
				if e := os.WriteFile(filepath.Join(s.home, "brain/owner/self/communication-style.md"), []byte("# Current\n\nOwner correction.\n"), 0600); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "expired" {
				later := s.now().Add(31 * 24 * time.Hour)
				s.now = func() time.Time { return later }
			}
			if kind == "registration-changed" {
				c.Weekly.Registration.Descriptor.Model = "changed-synthetic-model"
				c.Qualification = nil
				c.Revision++
				if e := s.withControl(c.JobID, func() error { return s.saveControl(c) }); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "qualification-missing" {
				if e := os.Remove(filepath.Join(s.home, "state", c.Qualification.Artifact)); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "failed-original" {
				failed := original
				failed.State = ReceiptFailed
				failed.ReasonCode = ReasonHandlerFailure
				if e := s.receipts.AppendReceipt(failed); e != nil {
					t.Fatal(e)
				}
			}
			if kind == "lease-busy" {
				command := Command{JobID: c.JobID, Trigger: TriggerWeekly, ScheduledFor: occ.ScheduledFor}
				lease, e := s.scheduler.TryAcquireLease(s.workspaceID, c.JobID, command.OccurrenceKey(), "synthetic-existing-owner", s.now(), time.Minute)
				if e != nil {
					t.Fatal(e)
				}
				if e = s.scheduler.ArmLease(lease); e != nil {
					t.Fatal(e)
				}
				defer s.scheduler.ReleaseLease(lease)
			}
			if _, e := s.execute(ctx, &productExecution{service: s, jobID: c.JobID, revision: c.Revision}, occ, s.now()); e == nil {
				t.Fatal("invalid effect/fence reconciled", kind)
			}
			receipts, e := s.receipts.Receipts(s.workspaceID, c.JobID)
			if e != nil {
				t.Fatal(e)
			}
			for _, r := range receipts {
				if r.OccurrenceDigest == original.OccurrenceDigest && r.State == ReceiptSucceeded {
					t.Fatal("invalid recovery published success", kind)
				}
			}
			if *calls != 0 {
				t.Fatal("invalid effect triggered model", kind)
			}
		})
	}
}

func TestWeeklyCurrentReviewsCarryExactAcceptedRegistration(t *testing.T) {
	s, c, _, _, _, _ := weeklyTrustedRecoveryFixture(t)
	status, e := s.Status()
	if e != nil {
		t.Fatal(e)
	}
	if len(status.Jobs) != 1 || status.Jobs[0].Scope != "owner" || status.Jobs[0].ScopeID != s.workspaceID {
		t.Fatal("weekly status hid owner scope", status)
	}

	for _, kind := range []string{"qualify", "pause", "revoke"} {
		r, e := s.Review(Operation{Kind: kind, JobID: c.JobID, ExpectedRevision: c.Revision})
		if e != nil {
			t.Fatal(e)
		}
		if r.WeeklyRegistration == nil || r.WeeklyRegistration.Digest() != c.Weekly.Registration.Digest() || r.Scope != "owner" {
			t.Fatal("current runtime omitted from exact owner review", kind)
		}
	}

	c.State = "inactive"
	c.Revision++
	if e := s.withControl(c.JobID, func() error { return s.saveControl(c) }); e != nil {
		t.Fatal(e)
	}
	r, e := s.Review(Operation{Kind: "enroll", JobID: c.JobID, ExpectedRevision: c.Revision, Timezone: "UTC"})
	if e != nil || r.WeeklyRegistration == nil || r.WeeklyRegistration.Digest() != c.Weekly.Registration.Digest() {
		t.Fatal("enrollment omitted current accepted registration", e)
	}
}

func TestWeeklyReportSeparatesCommittedEffectFromTerminalCompletion(t *testing.T) {
	s, c, occ, input, original, _ := weeklyTrustedRecoveryFixture(t)
	candidate := ownerctx.WeeklySelfCandidate{SchemaVersion: 1, InputSHA256: input.InputSHA256, CanonicalSHA256: input.Facets.CanonicalSourceDigest, PromptHistorySHA256: input.PromptHistorySHA256, PolicySHA256: input.PolicySHA256, TranslationSHA256: input.TranslationSHA256, Facets: map[string]string{"communication-style": "private-weekly-working-body"}}
	if _, e := ownerctx.PublishWeeklySelfWithExecution(context.Background(), filepath.Join(s.home, "brain"), input.InputSHA256, candidate, input.ExpectedRevision, weeklyFixtureExecution(s, c, original), func(write func() error) error { return write() }, s.now()); e != nil {
		t.Fatal(e)
	}
	report, e := s.Report(c.JobID)
	if e != nil || report.Weekly == nil || report.Weekly.TerminalState != "pending" {
		t.Fatal("committed effect hid missing terminal state", report, e)
	}
	if _, e = s.execute(context.Background(), &productExecution{service: s, jobID: c.JobID, revision: c.Revision}, occ, s.now()); e != nil {
		t.Fatal(e)
	}
	report, e = s.Report(c.JobID)
	if e != nil || report.Weekly == nil || report.Weekly.TerminalState != "complete" || report.NativeObservation != "weekly_native_effect_record_verified" {
		t.Fatal("verified completion was not reported", report, e)
	}
}
