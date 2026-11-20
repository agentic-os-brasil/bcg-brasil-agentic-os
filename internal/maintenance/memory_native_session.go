package maintenance

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

// The workspace branch borrows only the original native process lifecycle. Its
// input, validator, publication owner, policy and proof remain job-specific.
type memoryAttemptInput struct {
	input  memory.ModelWeeklyInput
	engine *memory.Engine
	at     time.Time
}

func (s *WeeklyAttemptSession) inputDigests() (string, string) {
	if s.memory != nil {
		return s.memory.input.InputSHA256, productDigest(*s.registration.Memory)
	}
	return s.input.InputSHA256, s.input.PolicySHA256
}

func newMemoryAttemptSession(ctx context.Context, command Command, grant ExecutionGrant, registration WeeklyModelRegistration, input memory.ModelWeeklyInput, implementation string, engine *memory.Engine, at time.Time) (*WeeklyAttemptSession, error) {
	bounded, ok := grant.(executionGrant)
	if !ok || engine == nil || command.JobID != MemoryModelJobID || registration.Descriptor.JobID != MemoryModelJobID || !attemptIDPattern.MatchString(bounded.attemptID) || !digestPattern.MatchString(implementation) || registration.Descriptor.RuntimeIdentity == nil {
		return nil, ErrQualificationRequired
	}
	if err := registration.Validate(); err != nil {
		return nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if err := ValidateWeeklyModelExecution(grant, command, registration.Descriptor, input.InputSHA256, productDigest(*registration.Memory)); err != nil {
		return nil, err
	}
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var owned memory.ModelWeeklyInput
	if err := json.Unmarshal(body, &owned); err != nil {
		return nil, err
	}
	r, err := cloneWeeklyRegistration(&registration)
	if err != nil {
		return nil, err
	}
	return &WeeklyAttemptSession{ctx: ctx, command: command, grant: bounded, registration: *r, memory: &memoryAttemptInput{owned, engine, at}, created: time.Now().UTC(), implementation: implementation, phase: "created"}, nil
}

func (s *WeeklyAttemptSession) ObserveMemoryResponse(ctx context.Context, candidate memory.ModelWeeklyCandidate) error {
	if s == nil || s.memory == nil {
		return ErrQualificationRequired
	}
	if err := s.check(ctx); err != nil {
		return err
	}
	body, err := json.Marshal(candidate)
	if err != nil {
		return err
	}
	if _, err := memory.ParseModelWeeklyCandidate(s.memory.input, body); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != "reaped" || s.consumed {
		return ErrQualificationRequired
	}
	s.responseSHA256 = productDigest(candidate)
	s.phase = "responded"
	return nil
}

func (s *WeeklyAttemptSession) consumeMemory(ctx context.Context, candidate memory.ModelWeeklyCandidate) (WeeklyNativeEvidence, error) {
	var empty WeeklyNativeEvidence
	if s == nil || s.memory == nil {
		return empty, ErrQualificationRequired
	}
	if err := s.check(ctx); err != nil {
		return empty, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != "responded" || s.consumed || s.child.PID() <= 1 || s.responseSHA256 != productDigest(candidate) {
		return empty, ErrQualificationRequired
	}
	s.consumed = true
	s.phase = "consumed"
	input, policy := s.inputDigests()
	return WeeklyNativeEvidence{SchemaVersion: 1, JobID: MemoryModelJobID, AttemptID: s.grant.attemptID, CommandID: s.command.CommandID, OccurrenceSHA256: s.command.OccurrenceDigest(), OwnerScope: s.command.WorkspaceID, ImplementationSHA256: s.implementation, RegistrationSHA256: s.registration.Digest(), InputSHA256: input, PolicySHA256: policy, ChildIdentitySHA256: s.child.Digest(), ResponseSHA256: s.responseSHA256, CompletedAt: time.Now().UTC()}, nil
}
