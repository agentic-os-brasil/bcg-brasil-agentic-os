package maintenance

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// Handler is the typed runtime-neutral seam for a governed job. Walter/self
// implementations register here after their own PR is integrated; this
// package never reimplements their semantics.
type Handler interface {
	Execute(context.Context, Command) (HandlerResult, error)
}

type HandlerFunc func(context.Context, Command) (HandlerResult, error)

func (function HandlerFunc) Execute(ctx context.Context, command Command) (HandlerResult, error) {
	return function(ctx, command)
}

type HandlerResult struct {
	State          ReceiptState
	ProposalCount  int
	ProposalDigest string
	Diagnostic     string
}

type WakeRequest struct {
	WorkspaceID string
	Now         time.Time
	Attended    bool
	OwnerID     string
}

type WakeReport struct {
	SchemaVersion int                    `json:"schema_version"`
	WorkspaceID   string                 `json:"workspace_id"`
	State         string                 `json:"state"`
	Due           []scheduler.Occurrence `json:"due,omitempty"`
	Receipts      []Receipt              `json:"receipts,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
}

// Worker is the local, bounded executor behind a presence wake. A wake only
// derives due work; only a qualified handler, occurrence lease and terminal
// receipt can make scheduler work complete.
type Worker struct {
	Catalog            Catalog
	Scheduler          scheduler.Store
	Receipts           Store
	Jobs               []scheduler.Job
	Handlers           map[string]Handler
	LocalQualification map[string]string
	ActivatedJobs      []string
	Deadline           time.Duration
	Now                func() time.Time
}

func (worker Worker) Run(ctx context.Context, request WakeRequest) (WakeReport, error) {
	now := request.Now
	if worker.Now != nil {
		now = worker.Now()
	}
	if now.IsZero() || strings.TrimSpace(request.WorkspaceID) == "" || strings.TrimSpace(request.OwnerID) == "" {
		return WakeReport{}, errors.New("maintenance wake requires workspace, owner and time")
	}
	if worker.Deadline <= 0 || worker.Deadline > 15*time.Minute {
		worker.Deadline = 2 * time.Minute
	}
	if err := worker.Catalog.Validate(); err != nil {
		return WakeReport{}, err
	}
	enrollment, err := worker.Scheduler.EnsureEnrollment(request.WorkspaceID, now)
	if err != nil {
		return WakeReport{}, err
	}
	schedulerReceipts, err := worker.Scheduler.Receipts(request.WorkspaceID)
	if err != nil {
		return WakeReport{}, err
	}
	due, err := scheduler.PlanDue(worker.Jobs, enrollment.EnrolledAt, schedulerReceipts, now)
	if err != nil {
		return WakeReport{}, err
	}
	report := WakeReport{SchemaVersion: 1, WorkspaceID: request.WorkspaceID, State: "no_due_work", Due: due}
	if len(due) == 0 {
		return report, nil
	}
	authorizations := make([]OccurrenceAuthorization, 0, len(due))
	for _, occurrence := range due {
		authorizations = append(authorizations, OccurrenceAuthorization{WorkspaceID: request.WorkspaceID, JobID: occurrence.JobID, Trigger: triggerForCadence(worker.Jobs, occurrence.JobID), ScheduledFor: occurrence.ScheduledFor})
	}
	authority, err := NewLocalExecutionAuthority(worker.Catalog, authorizations, worker.LocalQualification, worker.ActivatedJobs, request.Attended)
	if err != nil {
		return WakeReport{}, err
	}
	for index, occurrence := range due {
		if ctx.Err() != nil {
			break
		}
		receipt, runErr := worker.runOccurrence(ctx, request, now.Add(time.Duration(index)*time.Nanosecond), occurrence, authority)
		report.Receipts = append(report.Receipts, receipt)
		if runErr != nil && !errors.Is(runErr, scheduler.ErrLeaseBusy) {
			report.State = "completed_with_failures"
		}
	}
	if report.State == "no_due_work" {
		report.State = "processed"
	}
	return report, nil
}

func (worker Worker) runOccurrence(ctx context.Context, request WakeRequest, now time.Time, occurrence scheduler.Occurrence, authority ExecutionAuthority) (Receipt, error) {
	job, found := findCatalogJob(worker.Catalog, occurrence.JobID)
	if !found {
		return worker.unavailableReceipt(request.WorkspaceID, occurrence, now, "job is absent from maintenance catalog")
	}
	trigger := triggerForCadence(worker.Jobs, occurrence.JobID)
	command := Command{SchemaVersion: CommandSchemaVersion, CommandID: "wake-" + digestPrefix(occurrence.JobID+occurrence.ScheduledFor.UTC().Format(time.RFC3339Nano)+now.UTC().Format(time.RFC3339Nano)), JobID: occurrence.JobID, WorkspaceID: request.WorkspaceID, Trigger: trigger, ScheduledFor: occurrence.ScheduledFor, RequestedAt: now, Deadline: now.Add(worker.Deadline), ProposalOnly: occurrence.JobID == "darwin-structural-evolution-proposal"}
	if err := command.Validate(now); err != nil {
		return worker.unavailableReceipt(request.WorkspaceID, occurrence, now, "bounded wake command was rejected")
	}
	handler, handlerFound := worker.Handlers[occurrence.JobID]
	if !handlerFound || (worker.LocalQualification[occurrence.JobID] == "" && job.Availability != Available) {
		receipt, err := worker.unavailableReceipt(request.WorkspaceID, occurrence, now, job.AvailabilityReason)
		if appendErr := worker.Scheduler.AppendReceipt(request.WorkspaceID, scheduler.Receipt{JobID: occurrence.JobID, ScheduledFor: occurrence.ScheduledFor, AttemptedAt: now, State: scheduler.Unavailable, Error: "qualified local handler is not enrolled"}); appendErr != nil && err == nil {
			err = appendErr
		}
		return receipt, err
	}
	if _, err := authority.Authorize(command, now); err != nil {
		return worker.unavailableReceipt(request.WorkspaceID, occurrence, now, "local occurrence authority rejected command")
	}
	attemptID, err := attemptID()
	if err != nil {
		return Receipt{}, err
	}
	lease, err := worker.Scheduler.TryAcquireLease(request.WorkspaceID, occurrence.JobID, command.OccurrenceKey(), request.OwnerID, now, worker.Deadline)
	if err != nil {
		base := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: attemptID, OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: occurrence.JobID, WorkspaceID: request.WorkspaceID, Trigger: trigger, State: ReceiptBusy, RecordedAt: now, Deadline: command.Deadline, ProposalOnly: command.ProposalOnly, Diagnostic: "another bounded worker owns this occurrence"}
		return base, err
	}
	defer func() { _ = worker.Scheduler.ReleaseLease(lease) }()
	base := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: attemptID, OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: occurrence.JobID, WorkspaceID: request.WorkspaceID, Trigger: trigger, State: ReceiptFailed, RecordedAt: now, Deadline: command.Deadline, ProposalOnly: command.ProposalOnly}
	workerCtx, cancel := context.WithDeadline(ctx, command.Deadline)
	defer cancel()
	var executeErr error
	if err := worker.Scheduler.WithCurrentLease(lease, now, func() error {
		result, handlerErr := handler.Execute(workerCtx, command)
		executeErr = handlerErr
		if workerCtx.Err() != nil {
			base.State, base.Diagnostic = ReceiptTimedOut, "qualified handler exceeded its explicit deadline"
			executeErr = workerCtx.Err()
		} else if handlerErr != nil {
			base.State = ReceiptFailed
			base.Diagnostic = "qualified handler returned a recoverable failure"
			if errors.Is(handlerErr, scheduler.ErrCapabilityUnavailable) {
				base.State = ReceiptUnavailable
				base.Diagnostic = "qualified handler capability became unavailable"
			}
		} else {
			base.State, base.ProposalCount, base.ProposalDigest, base.Diagnostic = result.State, result.ProposalCount, result.ProposalDigest, boundedDiagnostic(result.Diagnostic)
			if base.State == "" {
				base.State = ReceiptSucceeded
			}
		}
		if err := worker.Receipts.AppendReceipt(base); err != nil {
			return err
		}
		state := scheduler.Failed
		if base.State == ReceiptSucceeded || base.State == ReceiptProposalEmitted {
			state = scheduler.Succeeded
		}
		return worker.Scheduler.AppendReceipt(request.WorkspaceID, scheduler.Receipt{JobID: occurrence.JobID, ScheduledFor: occurrence.ScheduledFor, AttemptedAt: now, State: state, Error: schedulerError(base)})
	}); err != nil {
		return base, err
	}
	return base, executeErr
}

func (worker Worker) unavailableReceipt(workspaceID string, occurrence scheduler.Occurrence, now time.Time, diagnostic string) (Receipt, error) {
	id, err := attemptID()
	if err != nil {
		return Receipt{}, err
	}
	if diagnostic == "" {
		diagnostic = "qualified local handler is unavailable"
	}
	receipt := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: id, OccurrenceDigest: digestOccurrence(occurrence), CommandID: "unavailable-" + digestPrefix(occurrence.JobID+occurrence.ScheduledFor.String()), JobID: occurrence.JobID, WorkspaceID: workspaceID, Trigger: triggerForCadence(worker.Jobs, occurrence.JobID), State: ReceiptUnavailable, RecordedAt: now, Deadline: now.Add(worker.Deadline), ProposalOnly: occurrence.JobID == "darwin-structural-evolution-proposal", Diagnostic: boundedDiagnostic(diagnostic)}
	return receipt, worker.Receipts.AppendReceipt(receipt)
}

func triggerForCadence(jobs []scheduler.Job, jobID string) Trigger {
	for _, job := range jobs {
		if job.ID == jobID {
			switch job.Cadence {
			case scheduler.Daily:
				return TriggerDaily
			case scheduler.Weekly:
				return TriggerWeekly
			case scheduler.Monthly:
				return TriggerMonthly
			}
		}
	}
	return TriggerPresence
}

func digestOccurrence(occurrence scheduler.Occurrence) string {
	return digest(occurrence.JobID + occurrence.ScheduledFor.UTC().Format(time.RFC3339Nano))
}
func digestPrefix(value string) string { return digest(value)[:16] }
func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return fmt.Sprintf("%x", sum[:])
}
func attemptID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}
func boundedDiagnostic(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 256 {
		return value[:256]
	}
	return value
}
func schedulerError(receipt Receipt) string {
	if receipt.State == ReceiptSucceeded || receipt.State == ReceiptProposalEmitted {
		return ""
	}
	return receipt.Diagnostic
}
