package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
)

// WeeklyNativeProcess is borrowed ONLY from the fixed concrete transport's
// successful Start/CreateProcess path. It is not a lookup request or authority.
type WeeklyNativeProcess struct {
	Process                                *os.Process
	ProcessHandle, ThreadHandle, JobHandle uintptr
	ProcessID, ThreadID                    uint32
}

type WeeklyNativeAttemptRequest struct {
	Command      Command
	Grant        ExecutionGrant
	Registration WeeklyModelRegistration
	Input        ownerctx.WeeklySelfInput
}

// WeeklyNativeExecutor is native composition, separate from candidate/model
// factories. Production binds the one fixed reviewed packaging implementation;
// a returned candidate or generic HandlerResult can never complete its session.
type WeeklyNativeExecutor func(context.Context, WeeklyNativeAttemptRequest, *WeeklyAttemptSession) (ownerctx.WeeklySelfCandidate, error)

type WeeklyNativeEvidence struct {
	SchemaVersion        int       `json:"schema_version"`
	AttemptID            string    `json:"attempt_id"`
	CommandID            string    `json:"command_id"`
	OccurrenceSHA256     string    `json:"occurrence_sha256"`
	OwnerScope           string    `json:"owner_scope"`
	JobID                string    `json:"job_id,omitempty"`
	ImplementationSHA256 string    `json:"implementation_sha256"`
	RegistrationSHA256   string    `json:"registration_sha256"`
	InputSHA256          string    `json:"input_sha256"`
	PolicySHA256         string    `json:"policy_sha256"`
	ChildIdentitySHA256  string    `json:"child_identity_sha256"`
	ResponseSHA256       string    `json:"response_sha256"`
	CompletedAt          time.Time `json:"completed_at"`
}

// WeeklyAttemptSession has no public constructor or serializable successful
// state. Service alone owns/consumes one under its existing live grant/lease.
// These events are called only by the fixed compiled native route, never a
// candidate factory, model report, arbitrary verifier or imported PID facts.
type WeeklyAttemptSession struct {
	mu             sync.Mutex
	ctx            context.Context
	command        Command
	grant          executionGrant
	registration   WeeklyModelRegistration
	input          ownerctx.WeeklySelfInput
	memory         *memoryAttemptInput
	created        time.Time
	implementation string
	ownerRoot      string
	phase          string
	process        WeeklyNativeProcess
	child          processidentity.WeeklyNativeChild
	inputClaimed   bool
	responseSHA256 string
	consumed       bool
}

func (*WeeklyAttemptSession) MarshalJSON() ([]byte, error) {
	return nil, errors.New("native attempt sessions cannot be serialized")
}
func newWeeklyAttemptSession(ctx context.Context, command Command, grant ExecutionGrant, registration WeeklyModelRegistration, input ownerctx.WeeklySelfInput, implementation, ownerRoot string) (*WeeklyAttemptSession, error) {
	bounded, ok := grant.(executionGrant)
	if !ok || command.JobID != YodaSelfReviewWeeklyJobID || registration.Descriptor.JobID != YodaSelfReviewWeeklyJobID || !attemptIDPattern.MatchString(bounded.attemptID) || !digestPattern.MatchString(implementation) || registration.Descriptor.RuntimeIdentity == nil {
		return nil, ErrQualificationRequired
	}
	if err := registration.Validate(); err != nil {
		return nil, err
	}
	if err := input.Validate(); err != nil {
		return nil, err
	}
	if err := ValidateWeeklyModelExecution(grant, command, registration.Descriptor, input.InputSHA256, input.PolicySHA256); err != nil {
		return nil, err
	}
	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	var owned ownerctx.WeeklySelfInput
	if err := json.Unmarshal(body, &owned); err != nil {
		return nil, err
	}
	ownedRegistration, err := cloneWeeklyRegistration(&registration)
	if err != nil {
		return nil, err
	}
	return &WeeklyAttemptSession{ctx: ctx, command: command, grant: bounded, registration: *ownedRegistration, input: owned, created: time.Now().UTC(), implementation: implementation, ownerRoot: ownerRoot, phase: "created"}, nil
}
func (s *WeeklyAttemptSession) check(ctx context.Context) error {
	if s == nil || s.ctx == nil || ctx == nil {
		return ErrQualificationRequired
	}
	s.mu.Lock()
	invalid := s.consumed || s.phase == "" || s.phase == "failed"
	s.mu.Unlock()
	if invalid {
		return ErrQualificationRequired
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	input, policy := s.inputDigests()
	return ValidateWeeklyModelExecution(s.grant, s.command, s.registration.Descriptor, input, policy)
}
func (s *WeeklyAttemptSession) CheckCurrent(ctx context.Context) error { return s.check(ctx) }
func (s *WeeklyAttemptSession) ObserveStarted(ctx context.Context, actual WeeklyNativeProcess) error {
	if err := s.check(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	if s.phase != "created" {
		s.mu.Unlock()
		return ErrQualificationRequired
	}
	s.phase = "observing"
	s.process = actual
	s.mu.Unlock()
	child, err := observeWeeklyNativeStart(ctx, actual, s.created)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.phase = "failed"
		return err
	}
	if child.PID() != int(actual.ProcessID) {
		s.phase = "failed"
		return ErrQualificationRequired
	}
	s.child = child
	s.phase = "started"
	return nil
}
func (s *WeeklyAttemptSession) AdmitInput(ctx context.Context, actual WeeklyNativeProcess, verified processidentity.WeeklyNativeChild) error {
	if err := s.check(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	if s.phase != "started" || s.process != actual || !s.child.Same(verified) {
		s.mu.Unlock()
		return ErrQualificationRequired
	}
	prior := s.child
	s.mu.Unlock()
	current, err := observeWeeklyNativeStart(ctx, actual, s.created)
	if err != nil || !current.Same(prior) {
		return errors.Join(ErrQualificationRequired, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != "started" || s.process != actual {
		return ErrQualificationRequired
	}
	s.phase = "admitted"
	return nil
}

// WriteInput holds the EXISTING live control/lease fence through bounded stdin
// write AND EOF. It is invocation, not domain publication; probe domain writes
// remain denied. No session mutex is held across blocking pipe operations.
func (s *WeeklyAttemptSession) WriteInput(ctx context.Context, actual WeeklyNativeProcess, writeAndClose func() error) error {
	if err := s.check(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	if s.phase != "admitted" || s.process != actual || s.inputClaimed || writeAndClose == nil {
		s.mu.Unlock()
		return ErrQualificationRequired
	}
	s.inputClaimed = true
	s.mu.Unlock()
	if s.grant.publication == nil {
		return ErrQualificationRequired
	}
	write := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.ctx.Err(); err != nil {
			return err
		}
		return writeAndClose()
	}
	var err error
	if s.memory != nil {
		// Memory takes its original activation lock before control/installation.
		err = s.memory.engine.WithCurrentModelWeeklyInput(ctx, s.memory.input, s.memory.at, func() error { return s.grant.publication(write) })
	} else {
		err = s.grant.publication(func() error {
			return ownerctx.WithCurrentWeeklySelfInput(ctx, s.ownerRoot, s.input, time.Now().UTC(), write)
		})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.phase = "failed"
		return err
	}
	if s.phase != "admitted" {
		return ErrQualificationRequired
	}
	s.phase = "input_released"
	return nil
}
func (s *WeeklyAttemptSession) ObserveTerminal(ctx context.Context, actual WeeklyNativeProcess) error {
	if err := s.check(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	if s.phase != "input_released" || s.process != actual {
		s.mu.Unlock()
		return ErrQualificationRequired
	}
	prior := s.child
	s.mu.Unlock()
	if err := verifyWeeklyNativeTerminal(ctx, actual, prior); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != "input_released" || s.process != actual {
		return ErrQualificationRequired
	}
	s.phase = "terminal"
	return nil
}
func (s *WeeklyAttemptSession) ObserveReaped(ctx context.Context, actual WeeklyNativeProcess, status *os.ProcessState) error {
	if err := s.check(ctx); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != "terminal" || s.process != actual {
		return ErrQualificationRequired
	}
	if err := verifyWeeklyNativeExit(actual, status); err != nil {
		s.phase = "failed"
		return err
	}
	s.phase = "reaped"
	return nil
}
func (s *WeeklyAttemptSession) ObserveResponse(ctx context.Context, candidate ownerctx.WeeklySelfCandidate) error {
	if s == nil || s.memory != nil {
		return ErrQualificationRequired
	}
	if err := s.check(ctx); err != nil {
		return err
	}
	body, err := json.Marshal(candidate)
	if err != nil {
		return err
	}
	if _, err := ownerctx.ParseWeeklySelfCandidate(s.input, body); err != nil {
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
func (s *WeeklyAttemptSession) consume(ctx context.Context, candidate ownerctx.WeeklySelfCandidate) (WeeklyNativeEvidence, error) {
	if s == nil || s.memory != nil {
		return WeeklyNativeEvidence{}, ErrQualificationRequired
	}
	var empty WeeklyNativeEvidence
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
	return WeeklyNativeEvidence{SchemaVersion: 1, AttemptID: s.grant.attemptID, CommandID: s.command.CommandID, OccurrenceSHA256: s.command.OccurrenceDigest(), OwnerScope: s.command.WorkspaceID, ImplementationSHA256: s.implementation, RegistrationSHA256: s.registration.Digest(), InputSHA256: s.input.InputSHA256, PolicySHA256: s.input.PolicySHA256, ChildIdentitySHA256: s.child.Digest(), ResponseSHA256: s.responseSHA256, CompletedAt: time.Now().UTC()}, nil
}
