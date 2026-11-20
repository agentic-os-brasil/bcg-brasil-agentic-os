package maintenance

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const MemoryModelJobID = memory.ModelWeeklyJobID

func modelJob(id string) bool { return id == YodaSelfReviewWeeklyJobID || id == MemoryModelJobID }

func weeklyMemoryJob(id string) bool { return id == MemoryModelJobID || id == MemoryDeepDreamJobID }
func (s *Service) checkWeeklyMemoryExclusive(job string) error {
	if !weeklyMemoryJob(job) {
		return nil
	}
	other := MemoryModelJobID
	if job == MemoryModelJobID {
		other = MemoryDeepDreamJobID
	}
	c, err := s.loadControl(other)
	if err != nil {
		return err
	}
	if c.State == "active" || c.OneShot != "" {
		return fmt.Errorf("pause the existing %s weekly engine before enrolling or running %s", other, job)
	}
	return nil
}

// MemoryModelPolicy is a closed workspace contract, never owner SELF policy or
// a permission to promote lifetime text.
type MemoryModelPolicy struct {
	SchemaVersion  int    `json:"schema_version"`
	InputContract  string `json:"input_contract"`
	OutputContract string `json:"output_contract"`
	LifetimeMode   string `json:"lifetime_mode"`
}

func DefaultMemoryModelPolicy() MemoryModelPolicy {
	return MemoryModelPolicy{1, "eligible-memory-weekly-v1", "bounded-memory-layers-v1", "independent_exact_review"}
}

func (p MemoryModelPolicy) Validate() error {
	if p != DefaultMemoryModelPolicy() {
		return errors.New("unsupported workspace model memory policy")
	}
	return nil
}

func (p MemoryModelPolicy) Digest() string { return productDigest(p) }

type MemoryNativeAttemptRequest struct {
	Command      Command
	Grant        ExecutionGrant
	Registration WeeklyModelRegistration
	Input        memory.ModelWeeklyInput
}

type MemoryNativeExecutor func(context.Context, MemoryNativeAttemptRequest, *WeeklyAttemptSession) (memory.ModelWeeklyCandidate, error)

type memoryModelSelection struct {
	ScheduledFor       time.Time `json:"scheduled_for"`
	InputSHA256        string    `json:"input_sha256"`
	RegistrationSHA256 string    `json:"registration_sha256"`
}
type memoryExecutionPlan struct {
	registration                      WeeklyModelRegistration
	input                             memory.ModelWeeklyInput
	engine                            *memory.Engine
	at                                time.Time
	mode                              WeeklyModelMode
	qualification, activation, intent string
	committed                         *memory.ModelWeeklyCommit
}

func (p *memoryExecutionPlan) binding(scope string) weeklyModelBinding {
	b := weeklyModelBinding{Mode: p.mode, OwnerScope: scope, DescriptorSHA256: p.registration.Descriptor.Digest(), InputSHA256: p.input.InputSHA256, PolicySHA256: p.registration.Memory.Digest()}
	if p.mode == WeeklyNativeProbe {
		b.TerminalOperation = "qualification_evidence_only"
		b.ProbeIntentSHA256 = p.intent
	} else {
		b.TerminalOperation = "memory_atomic_commit_only"
		b.QualificationSHA256 = p.qualification
		b.ActivationSHA256 = p.activation
	}
	return b
}
func (s *Service) memoryModelEngine(registration WeeklyModelRegistration) *memory.Engine {
	engine := *s.weekly
	engine.SynthesizerID = "weekly-memory-model-v1:" + registration.Digest()
	return &engine
}

func (p *productExecution) selectMemory(ctx context.Context, occ scheduler.Occurrence, now time.Time) error {
	s := p.service
	c, err := s.loadControl(MemoryModelJobID)
	if err != nil {
		return err
	}
	if c.Weekly == nil || c.Weekly.Registration.Descriptor.JobID != MemoryModelJobID || c.Weekly.Registration.Descriptor.RuntimeIdentity == nil || s.options.MemoryNativeExecutor == nil {
		return ErrQualificationRequired
	}
	registration := c.Weekly.Registration
	if err = registration.Validate(); err != nil {
		return err
	}
	plan := &memoryExecutionPlan{registration: registration, engine: s.memoryModelEngine(registration), at: occ.ScheduledFor, mode: WeeklyWorking}
	if p.oneShot != "" {
		plan.mode = WeeklyNativeProbe
		plan.intent = productDigest([]any{p.oneShot, c.Weekly.RegistrationReviewSHA256, registration.Digest(), occ.ScheduledFor})
	} else {
		if s.verifyQualification(c.Qualification) != nil {
			return ErrQualificationRequired
		}
		plan.qualification = c.Qualification.ID
		plan.activation = productDigest([]any{s.binding, c.Revision, c.Schedule, c.Boundary, c.Weekly.RegistrationReviewSHA256, plan.qualification})
		committed, _, e := plan.engine.ReadModelWeeklyCommit(s.workspaceID)
		if e == nil && committed.Execution != nil && committed.Execution.ScheduledFor.Equal(occ.ScheduledFor) {
			plan.committed = &committed
			p.memoryPlan = plan
			return p.guard(func() error { return p.validateMemoryCommitted() })
		}
		if e != nil && !errors.Is(e, memory.ErrModelWeeklySourcesChanged) {
			if errors.Is(e, memory.ErrCaptureConflict) || !errors.Is(e, os.ErrNotExist) {
				return e
			}
		}
	}
	if plan.mode == WeeklyNativeProbe {
		plan.input, err = plan.engine.InspectModelWeeklyProbeInput(s.workspaceID, plan.at)
	} else {
		plan.input, err = plan.engine.InspectModelWeeklyInput(s.workspaceID, plan.at)
	}
	if err != nil {
		return err
	}
	// Freeze only one digest for this real occurrence, under the original memory
	// -> control ordering. A retry cannot select a changed source cohort.
	if plan.mode == WeeklyWorking {
		err = plan.engine.WithCurrentModelWeeklyInput(ctx, plan.input, plan.at, func() error {
			return s.withControl(MemoryModelJobID, func() error {
				current, e := s.loadControl(MemoryModelJobID)
				if e != nil {
					return e
				}
				if current.Revision != p.revision || current.State != "active" || current.Weekly == nil || current.Weekly.Registration.Digest() != registration.Digest() {
					return ErrProductConflict
				}
				selected := current.Weekly.MemorySelection
				if selected != nil && selected.ScheduledFor.Equal(occ.ScheduledFor) {
					if selected.InputSHA256 != plan.input.InputSHA256 || selected.RegistrationSHA256 != registration.Digest() {
						return ErrProductConflict
					}
					return nil
				}
				if selected != nil && !occ.ScheduledFor.After(selected.ScheduledFor) {
					return ErrProductConflict
				}
				current.Weekly.MemorySelection = &memoryModelSelection{occ.ScheduledFor, plan.input.InputSHA256, registration.Digest()}
				// Selection is not a new enrollment or authority revision. It is immutable
				// occurrence data fenced by this already-reviewed control revision.
				return s.saveControl(current)
			})
		})
		if err != nil {
			return err
		}
	}
	p.memoryPlan = plan
	return nil
}

func (p *productExecution) validateMemoryPlan(ctx context.Context, c productControl) error {
	plan := p.memoryPlan
	if plan == nil || c.Weekly == nil || c.Weekly.Registration.Digest() != plan.registration.Digest() {
		return ErrProductConflict
	}
	if plan.mode == WeeklyWorking && (c.Qualification == nil || c.Qualification.ID != plan.qualification || p.service.verifyQualification(c.Qualification) != nil) {
		return ErrQualificationRequired
	}
	if plan.committed != nil {
		return p.validateMemoryCommitted()
	}
	var input memory.ModelWeeklyInput
	var err error
	if plan.mode == WeeklyNativeProbe {
		input, err = plan.engine.InspectModelWeeklyProbeInput(p.service.workspaceID, plan.at)
	} else {
		input, err = plan.engine.InspectModelWeeklyInput(p.service.workspaceID, plan.at)
	}
	if err != nil || input.InputSHA256 != plan.input.InputSHA256 {
		return errors.Join(ErrProductConflict, err)
	}
	if plan.mode == WeeklyWorking {
		selected := c.Weekly.MemorySelection
		if selected == nil || !selected.ScheduledFor.Equal(plan.at) || selected.InputSHA256 != input.InputSHA256 || selected.RegistrationSHA256 != plan.registration.Digest() {
			return ErrProductConflict
		}
	}
	return ctx.Err()
}

type productMemoryModelHandler struct {
	service *Service
	plan    *memoryExecutionPlan
}

func (h productMemoryModelHandler) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if h.service == nil || h.plan == nil || h.service.options.MemoryNativeExecutor == nil {
		return HandlerResult{}, ErrQualificationRequired
	}
	p := h.plan
	if err := ValidateWeeklyModelExecution(grant, command, p.registration.Descriptor, p.input.InputSHA256, p.registration.Memory.Digest()); err != nil {
		return HandlerResult{}, err
	}
	err := scheduler.WithPrivateLockContext(ctx, filepath.Join(filepath.Dir(h.service.controlPath(MemoryModelJobID)), "native-supervisor.guard"), func() error {
		session, err := newMemoryAttemptSession(ctx, command, grant, p.registration, p.input, h.service.build, p.engine, p.at)
		if err != nil {
			return err
		}
		candidate, err := h.service.options.MemoryNativeExecutor(ctx, MemoryNativeAttemptRequest{command, grant, p.registration, p.input}, session)
		if err != nil {
			return err
		}
		evidence, err := session.consumeMemory(ctx, candidate)
		if err != nil {
			return err
		}
		bounded, ok := grant.(executionGrant)
		if !ok || bounded.publication == nil {
			return ErrQualificationRequired
		}
		if p.mode == WeeklyNativeProbe {
			return bounded.publication(func() error { return h.service.writeWeeklyEvidence(evidence) })
		}
		execution := &memory.ModelWeeklyExecution{RegistrationSHA256: p.registration.Digest(), QualificationSHA256: p.qualification, ActivationSHA256: p.activation, ImplementationSHA256: h.service.build, WorkspaceID: h.service.workspaceID, CommandID: command.CommandID, AttemptID: bounded.attemptID, OccurrenceSHA256: command.OccurrenceDigest(), ScheduledFor: command.ScheduledFor, RequestedAt: command.RequestedAt, Deadline: command.Deadline}
		engine := *p.engine
		engine.PublicationGuard = func(write func() error) error { return publishWithGrant(ctx, grant, command, write) }
		_, commitErr := engine.CommitModelWeeklyWithExecution(ctx, p.input, candidate, execution, p.at)
		committed, _, readErr := engine.ReadModelWeeklyCommit(h.service.workspaceID)
		if readErr == nil && committed.Execution != nil && *committed.Execution == *execution {
			p.committed = &committed
			// The original atomic effect is authoritative even when a post-commit
			// metadata/fault boundary returned an error. The current grant still
			// fences the worker's separate success receipt and recovery metadata.
			return nil
		}
		return errors.Join(ErrProductConflict, commitErr, readErr)
	})
	if err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
}

func (p *productExecution) validateMemoryCommitted() error {
	plan := p.memoryPlan
	if plan == nil || plan.committed == nil || plan.committed.Execution == nil {
		return ErrQualificationRequired
	}
	current, _, err := plan.engine.ReadModelWeeklyCommit(p.service.workspaceID)
	if err != nil || current.Digest() != plan.committed.Digest() {
		return errors.Join(ErrProductConflict, err)
	}
	e := current.Execution
	if e.Validate(p.service.workspaceID) != nil || e.RegistrationSHA256 != plan.registration.Digest() || e.QualificationSHA256 != plan.qualification || e.ActivationSHA256 != plan.activation || e.ImplementationSHA256 != p.service.build {
		return ErrProductConflict
	}
	command := Command{JobID: MemoryModelJobID, Trigger: TriggerWeekly, ScheduledFor: e.ScheduledFor}
	if command.OccurrenceDigest() != e.OccurrenceSHA256 {
		return ErrProductConflict
	}
	return nil
}
func memoryReceiptFromExecution(e *memory.ModelWeeklyExecution) Receipt {
	return Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: e.AttemptID, OccurrenceDigest: e.OccurrenceSHA256, CommandID: e.CommandID, JobID: MemoryModelJobID, WorkspaceID: e.WorkspaceID, Trigger: TriggerWeekly, State: ReceiptSucceeded, RecordedAt: e.RequestedAt, Deadline: e.Deadline, WeeklyMode: WeeklyWorking, ReasonCode: ReasonCompleted}
}
func (s *Service) reconcileMemoryCommitted(ctx context.Context, p *productExecution, occ scheduler.Occurrence, now time.Time) (Receipt, error) {
	receipt := memoryReceiptFromExecution(p.memoryPlan.committed.Execution)
	if err := receipt.Validate(); err != nil {
		return Receipt{}, err
	}
	if !p.memoryPlan.committed.Execution.ScheduledFor.Equal(occ.ScheduledFor) {
		return Receipt{}, ErrProductConflict
	}
	prior, err := s.receipts.Receipts(s.workspaceID, p.jobID)
	if err != nil {
		return Receipt{}, err
	}
	for _, r := range prior {
		if r.OccurrenceDigest == receipt.OccurrenceDigest && (r.State == ReceiptSucceeded && r != receipt || r.CommandID == receipt.CommandID && r.AttemptID == receipt.AttemptID && r != receipt) {
			return Receipt{}, ErrProductConflict
		}
	}
	worker := s.worker(p)
	lease, err := s.scheduler.TryAcquireLease(s.workspaceID, p.jobID, Command{JobID: p.jobID, Trigger: TriggerWeekly, ScheduledFor: occ.ScheduledFor}.OccurrenceKey(), fmt.Sprintf("product-reconcile-%d", os.Getpid()), now, worker.Deadline+leaseQuarantineGrace)
	if err != nil {
		return Receipt{}, err
	}
	if err = worker.armLease(lease); err != nil {
		return Receipt{}, errors.Join(err, worker.releaseLease(lease))
	}
	err = worker.publishOccurrence(ctx, s.workspaceID, occ, receipt.RecordedAt, lease, receipt)
	releaseErr := worker.releaseLease(lease)
	if err != nil || releaseErr != nil {
		return rejectedTerminalReceipt(receipt), errors.Join(err, releaseErr)
	}
	return receipt, nil
}

func (s *Service) memoryModelReport() (ProductDomainReport, error) {
	out := ProductDomainReport{JobID: MemoryModelJobID, WorkspaceID: s.workspaceID, Scope: "workspace", Finding: "no_report", NativeObservation: "not_observed"}
	if err := s.validate(); err != nil {
		return out, err
	}
	c, err := s.loadControl(MemoryModelJobID)
	if err != nil {
		return out, err
	}
	if c.Weekly == nil {
		return out, nil
	}
	out.Weekly = &WeeklyReportMetadata{RegistrationSHA256: c.Weekly.Registration.Digest()}
	qualified := c.Qualification != nil && s.verifyQualification(c.Qualification) == nil
	if qualified {
		out.Weekly.QualificationID = c.Qualification.ID
		out.Finding = "historical_native_probe_only"
		out.NativeObservation = "verified_historical_probe"
		out.ObservedAt = c.Qualification.VerifiedAt
	}
	engine := s.memoryModelEngine(c.Weekly.Registration)
	r, _, err := engine.ReadModelWeeklyCommit(s.workspaceID)
	if errors.Is(err, memory.ErrModelWeeklySourcesChanged) {
		out.Finding = "model_memory_sources_changed"
		return out, nil
	}
	if errors.Is(err, memory.ErrCaptureConflict) {
		return out, err
	}
	if errors.Is(err, os.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.Period = r.Input.Period
	out.Counts = map[string]int{"layers": 2}
	out.LifetimeStatus = "not_promoted"
	if r.PendingLifetime != nil {
		out.LifetimeStatus = "exact_candidate_review_pending"
		out.Counts["pending_lifetime_candidates"] = 1
	} else if a, _, e := engine.ActiveArtifact(s.workspaceID, "lifetime/current"); e == nil && a.SynthesizerID == r.Input.SynthesizerID && len(a.Sources) > 0 && a.Sources[0].SHA256 == r.L3SHA256 {
		out.LifetimeStatus = "promoted"
		out.Counts["layers"] = 3
	}
	out.Finding = "model_memory_projection_unattested"
	out.NativeObservation = "not_observed"
	out.ObservedAt = time.Time{}
	if execution := r.Execution; execution != nil {
		out.Finding = "historical_model_memory_projection"
		out.NativeObservation = "historical_execution_record"
		out.ObservedAt = execution.RequestedAt
		out.Weekly.ScheduledFor = execution.ScheduledFor
		out.Weekly.TerminalState = "pending"
		receipt := memoryReceiptFromExecution(execution)
		if e := receipt.Validate(); e != nil {
			return out, e
		}
		if (Command{JobID: MemoryModelJobID, Trigger: TriggerWeekly, ScheduledFor: execution.ScheduledFor}).OccurrenceDigest() != execution.OccurrenceSHA256 {
			return out, ErrProductConflict
		}
		complete, e := s.weeklyTerminalComplete(receipt, execution.ScheduledFor)
		if e != nil {
			return out, e
		}
		if complete {
			out.Weekly.TerminalState = "complete"
		}
		if execution.RegistrationSHA256 == c.Weekly.Registration.Digest() && qualified && execution.QualificationSHA256 == c.Qualification.ID && execution.ImplementationSHA256 == s.build {
			out.Finding = "model_memory_projection_committed"
			out.NativeObservation = "weekly_native_effect_record_verified"
		}
	}
	return out, nil
}
