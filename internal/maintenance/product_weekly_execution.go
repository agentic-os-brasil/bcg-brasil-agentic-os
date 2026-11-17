package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func (p *productExecution) selectWeekly(ctx context.Context, occurrence scheduler.Occurrence, now time.Time) error {
	s := p.service
	c, err := s.loadControl(p.jobID)
	if err != nil {
		return err
	}
	if !s.weeklyScoped || c.Weekly == nil || s.options.WeeklyNativeExecutor == nil || c.Weekly.Registration.Descriptor.RuntimeIdentity == nil {
		return ErrQualificationRequired
	}
	registration := c.Weekly.Registration
	if err := registration.Validate(); err != nil {
		return err
	}
	options := registration.Policy
	if options.TranslatorID != "identity" || options.TranslatorVersion != "1" {
		return errors.New("weekly translator unavailable")
	}
	options.OccurrenceID = productDigest([]any{s.workspaceID, p.jobID, occurrence.ScheduledFor})
	options.ScheduledFor = occurrence.ScheduledFor
	plan := &weeklyExecutionPlan{registration: registration, mode: WeeklyWorking}
	if p.oneShot != "" {
		plan.mode = WeeklyNativeProbe
		plan.intent = productDigest([]any{p.oneShot, c.Weekly.RegistrationReviewSHA256, registration.Digest(), options.OccurrenceID})
	} else {
		if s.verifyQualification(c.Qualification) != nil {
			return ErrQualificationRequired
		}
		plan.qualification = c.Qualification.ID
		plan.activation = productDigest([]any{s.binding, c.Revision, c.Schedule, c.Boundary, c.Weekly.RegistrationReviewSHA256, plan.qualification})
	}
	normalize := func(text, from, to string) (string, error) {
		if from != to {
			return "", errors.New("weekly identity translation requires matching languages")
		}
		return text, nil
	}
	if plan.mode == WeeklyNativeProbe {
		plan.input, err = ownerctx.InspectWeeklySelfInput(ctx, filepath.Join(s.home, "brain"), options, normalize, now)
	} else {
		working, readErr := ownerctx.ReadWeeklySelf(filepath.Join(s.home, "brain"), options, now)
		if readErr == nil && working.Receipt.OccurrenceID == options.OccurrenceID {
			plan.committed = &working
			p.weeklyPlan = plan
			return p.guard(func() error { return p.validateWeeklyCommitted() })
		}
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) && !errors.Is(readErr, ownerctx.ErrWeeklySelfChanged) && !errors.Is(readErr, ownerctx.ErrWeeklySelfExpired) {
			return readErr
		}
		err = p.guard(func() error {
			var e error
			_, e = ownerctx.PruneWeeklySelf(filepath.Join(s.home, "brain"), options, now)
			if e != nil && !errors.Is(e, os.ErrNotExist) {
				return e
			}
			plan.input, e = ownerctx.FreezeWeeklySelf(filepath.Join(s.home, "brain"), options, normalize, now)
			return e
		})
	}
	if err != nil {
		return err
	}
	p.weeklyPlan = plan
	return nil
}

type productWeeklyModelHandler struct {
	service *Service
	plan    *weeklyExecutionPlan
}

func (h productWeeklyModelHandler) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (result HandlerResult, err error) {
	if h.plan == nil || h.service == nil || !h.service.weeklyScoped || h.service.options.WeeklyNativeExecutor == nil {
		return result, ErrQualificationRequired
	}
	if err = ValidateWeeklyModelExecution(grant, command, h.plan.registration.Descriptor, h.plan.input.InputSHA256, h.plan.input.PolicySHA256); err != nil {
		return result, err
	}
	// One owner-wide process lock spans synchronous launch, cancellation and reap.
	// The existing worker quarantine remains authoritative if its caller times out.
	err = scheduler.WithPrivateLockContext(ctx, filepath.Join(filepath.Dir(h.service.controlPath(command.JobID)), "native-supervisor.guard"), func() error {
		if e := ValidateExecutionGrant(grant, command); e != nil {
			return e
		}
		session, e := newWeeklyAttemptSession(ctx, command, grant, h.plan.registration, h.plan.input, h.service.build, filepath.Join(h.service.home, "brain"))
		if e != nil {
			return e
		}
		candidate, e := h.service.options.WeeklyNativeExecutor(ctx, WeeklyNativeAttemptRequest{command, grant, h.plan.registration, h.plan.input}, session)
		if e != nil {
			return e
		}
		evidence, e := session.consume(ctx, candidate)
		if e != nil {
			return e
		}
		if h.plan.mode == WeeklyNativeProbe {
			// Evidence-only runs never prepare or publish a working projection.
			bounded, ok := grant.(executionGrant)
			if !ok || bounded.publication == nil {
				return ErrQualificationRequired
			}
			return bounded.publication(func() error { return h.service.writeWeeklyEvidence(evidence) })
		}
		bounded, ok := grant.(executionGrant)
		if !ok {
			return ErrQualificationRequired
		}
		execution := ownerctx.WeeklySelfExecution{RegistrationSHA256: h.plan.registration.Digest(), QualificationSHA256: h.plan.qualification, ActivationSHA256: h.plan.activation, ImplementationSHA256: h.service.build, OwnerScope: h.service.workspaceID, CommandID: command.CommandID, AttemptID: bounded.attemptID, OccurrenceSHA256: command.OccurrenceDigest(), RequestedAt: command.RequestedAt, Deadline: command.Deadline}
		_, e = ownerctx.PublishWeeklySelfWithExecution(ctx, filepath.Join(h.service.home, "brain"), h.plan.input.InputSHA256, candidate, h.plan.input.ExpectedRevision, execution, func(write func() error) error { return publishWithGrant(ctx, grant, command, write) }, h.service.now().UTC())
		return e
	})
	if err != nil {
		return HandlerResult{}, err
	}
	return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
}

func (s *Service) weeklyEvidenceRoot() string {
	return filepath.Join(s.receipts.Root, "workspaces", s.workspaceID, "weekly-native")
}
func (s *Service) writeWeeklyEvidence(evidence WeeklyNativeEvidence) error {
	body, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	id := productDigest(evidence)
	path := filepath.Join(s.weeklyEvidenceRoot(), "versions", id+".json")
	if err = scheduler.EnsurePrivateDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	if err = scheduler.PublishNewPrivateFile(path, body); err != nil {
		return err
	}
	pointer, _ := json.Marshal(struct {
		Generation string `json:"generation"`
	}{id})
	return scheduler.WritePrivateFileAtomic(filepath.Join(s.weeklyEvidenceRoot(), "current.json"), pointer)
}
func (s *Service) readWeeklyEvidence(id string) (WeeklyNativeEvidence, string, error) {
	var evidence WeeklyNativeEvidence
	if id == "" {
		body, e := scheduler.ReadPrivateStateFile(filepath.Join(s.weeklyEvidenceRoot(), "current.json"), 1024)
		if e != nil {
			return evidence, "", e
		}
		var pointer struct {
			Generation string `json:"generation"`
		}
		if decodeProduct(body, &pointer) != nil {
			return evidence, "", ErrQualificationRequired
		}
		id = pointer.Generation
	}
	if !digestPattern.MatchString(id) {
		return evidence, "", ErrQualificationRequired
	}
	path := filepath.Join(s.weeklyEvidenceRoot(), "versions", id+".json")
	body, err := scheduler.ReadPrivateStateFile(path, 16384)
	if err != nil {
		return evidence, "", err
	}
	if decodeProduct(body, &evidence) != nil || productDigest(evidence) != id || evidence.SchemaVersion != 1 || evidence.OwnerScope != s.workspaceID || !attemptIDPattern.MatchString(evidence.AttemptID) || evidence.CommandID == "" || evidence.CompletedAt.IsZero() {
		return evidence, "", ErrQualificationRequired
	}
	for _, v := range []string{evidence.OccurrenceSHA256, evidence.ImplementationSHA256, evidence.RegistrationSHA256, evidence.InputSHA256, evidence.PolicySHA256, evidence.ChildIdentitySHA256, evidence.ResponseSHA256} {
		if !digestPattern.MatchString(v) {
			return evidence, "", ErrQualificationRequired
		}
	}
	return evidence, path, nil
}
func (s *Service) weeklyDomainEvidence() (string, string, error) {
	_, path, err := s.readWeeklyEvidence("")
	if err != nil {
		return "", "", err
	}
	body, err := scheduler.ReadPrivateStateFile(path, 16384)
	if err != nil {
		return "", "", err
	}
	rel, err := filepath.Rel(s.receipts.Root, path)
	return digest(string(body)), filepath.ToSlash(filepath.Join("maintenance", rel)), err
}
func (s *Service) verifyWeeklyQualification(q *ProductQualification) (string, error) {
	prefix := "maintenance/workspaces/" + s.workspaceID + "/weekly-native/versions/"
	if !s.weeklyScoped || !strings.HasPrefix(q.Artifact, prefix) || !strings.HasSuffix(q.Artifact, ".json") || q.Receipt.WeeklyMode != WeeklyNativeProbe {
		return "", ErrQualificationRequired
	}
	id := strings.TrimSuffix(strings.TrimPrefix(q.Artifact, prefix), ".json")
	evidence, path, err := s.readWeeklyEvidence(id)
	if err != nil {
		return "", err
	}
	c, err := s.loadControl(q.JobID)
	if err != nil {
		return "", err
	}
	if c.Weekly == nil || c.Weekly.Registration.Digest() != evidence.RegistrationSHA256 || evidence.ImplementationSHA256 != s.build || evidence.AttemptID != q.Receipt.AttemptID || evidence.CommandID != q.Receipt.CommandID || evidence.OccurrenceSHA256 != q.Receipt.OccurrenceDigest || evidence.CompletedAt.Before(q.RequestedAt) || evidence.CompletedAt.After(q.VerifiedAt) {
		return "", ErrQualificationRequired
	}
	return path, nil
}
