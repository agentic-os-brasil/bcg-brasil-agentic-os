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
	State              ReceiptState
	ProposalCount      int
	ProposalDigest     string
	ProposalArtifactID string
	ReasonCode         ReasonCode
}

type WakeRequest struct {
	WorkspaceID string
	Now         time.Time
	Timezone    string
	Attended    bool
	// Preauthorized is persisted local enrollment authority. It is distinct
	// from Attended, which represents consent for this individual wake.
	Preauthorized bool
	OwnerID       string
}

type WakeReport struct {
	SchemaVersion int                    `json:"schema_version"`
	WorkspaceID   string                 `json:"workspace_id"`
	State         string                 `json:"state"`
	Due           []scheduler.Occurrence `json:"due,omitempty"`
	Receipts      []Receipt              `json:"receipts,omitempty"`
	Reason        string                 `json:"reason,omitempty"`
}

type handlerOutcome struct {
	result HandlerResult
	err    error
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
	ReleaseLease       func(scheduler.Lease) error
}

const leaseQuarantineGrace = time.Second

func (worker Worker) Run(ctx context.Context, request WakeRequest) (WakeReport, error) {
	now := request.Now
	if worker.Now != nil {
		now = worker.Now()
	}
	if now.IsZero() || strings.TrimSpace(request.WorkspaceID) == "" || strings.TrimSpace(request.OwnerID) == "" {
		return WakeReport{}, errors.New("maintenance wake requires workspace, owner and time")
	}
	planningNow := now
	if request.Timezone != "" {
		location, loadErr := time.LoadLocation(request.Timezone)
		if loadErr != nil {
			return WakeReport{}, errors.New("maintenance wake timezone is not a valid IANA location")
		}
		planningNow = now.In(location)
	}
	if worker.Deadline <= 0 || worker.Deadline > 15*time.Minute {
		worker.Deadline = 2 * time.Minute
	}
	if err := worker.Catalog.Validate(); err != nil {
		return WakeReport{}, err
	}
	enrollment, err := worker.Scheduler.EnsureEnrollment(request.WorkspaceID, planningNow)
	if err != nil {
		return WakeReport{}, err
	}
	schedulerReceipts, err := worker.Scheduler.Receipts(request.WorkspaceID)
	if err != nil {
		return WakeReport{}, err
	}
	due, err := scheduler.PlanDue(worker.Jobs, enrollment.EnrolledAt, schedulerReceipts, planningNow)
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
	var authority ExecutionAuthority
	if request.Preauthorized {
		authority, err = NewPreauthorizedLocalExecutionAuthority(worker.Catalog, authorizations, worker.LocalQualification, worker.ActivatedJobs)
	} else {
		authority, err = NewLocalExecutionAuthority(worker.Catalog, authorizations, worker.LocalQualification, worker.ActivatedJobs, request.Attended)
	}
	if err != nil {
		return WakeReport{}, err
	}
	executionNow := now.UTC()
	for index, occurrence := range due {
		if ctx.Err() != nil {
			break
		}
		receipt, runErr := worker.runOccurrence(ctx, request, executionNow.Add(time.Duration(index)*time.Nanosecond), occurrence, authority)
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
		return worker.unavailableReceipt(request.WorkspaceID, occurrence, now, ReasonCatalogUnavailable)
	}
	trigger := triggerForCadence(worker.Jobs, occurrence.JobID)
	command := Command{SchemaVersion: CommandSchemaVersion, CommandID: "wake-" + digestPrefix(occurrence.JobID+occurrence.ScheduledFor.UTC().Format(time.RFC3339Nano)+now.UTC().Format(time.RFC3339Nano)), JobID: occurrence.JobID, WorkspaceID: request.WorkspaceID, Trigger: trigger, ScheduledFor: occurrence.ScheduledFor, RequestedAt: now, Deadline: now.Add(worker.Deadline), ProposalOnly: occurrence.JobID == "darwin-structural-evolution-proposal"}
	if err := command.Validate(now); err != nil {
		return worker.unavailableReceipt(request.WorkspaceID, occurrence, now, ReasonOccurrenceRejected)
	}
	handler, handlerFound := worker.Handlers[occurrence.JobID]
	if !handlerFound || (worker.LocalQualification[occurrence.JobID] == "" && job.Availability != Available) {
		receipt, err := worker.unavailableReceipt(request.WorkspaceID, occurrence, now, ReasonCatalogUnavailable)
		if appendErr := worker.Scheduler.AppendReceipt(request.WorkspaceID, scheduler.Receipt{JobID: occurrence.JobID, ScheduledFor: occurrence.ScheduledFor, AttemptedAt: now, State: scheduler.Unavailable, Error: "qualified local handler is not enrolled"}); appendErr != nil && err == nil {
			err = appendErr
		}
		return receipt, err
	}
	if _, err := authority.Authorize(command, now); err != nil {
		return worker.unavailableReceipt(request.WorkspaceID, occurrence, now, ReasonAuthorityRejected)
	}
	attemptID, err := attemptID()
	if err != nil {
		return Receipt{}, err
	}
	leaseTTL := worker.Deadline
	if leaseTTL < 15*time.Minute {
		// Keep the lease alive past the handler deadline so the timeout path can
		// atomically install the quarantine marker before any successor can
		// observe an expired lease. The marker, not TTL expiry, is the recovery
		// boundary for a still-running handler.
		leaseTTL += leaseQuarantineGrace
	}
	lease, err := worker.Scheduler.TryAcquireLease(request.WorkspaceID, occurrence.JobID, command.OccurrenceKey(), request.OwnerID, now, leaseTTL)
	if err != nil {
		base := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: attemptID, OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: occurrence.JobID, WorkspaceID: request.WorkspaceID, Trigger: trigger, State: ReceiptBusy, RecordedAt: now, Deadline: command.Deadline, ProposalOnly: command.ProposalOnly, ReasonCode: ReasonLeaseBusy}
		return base, err
	}
	base := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: attemptID, OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: occurrence.JobID, WorkspaceID: request.WorkspaceID, Trigger: trigger, State: ReceiptFailed, RecordedAt: now, Deadline: command.Deadline, ProposalOnly: command.ProposalOnly, ReasonCode: ReasonHandlerFailure}
	// Arm the quarantine fence before invoking any handler. This removes the
	// expiry-to-quarantine race entirely: a crashed or stalled worker leaves an
	// explicit operator-recoverable fence instead of an ordinary reclaimable
	// lease.
	if err := worker.Scheduler.ArmLease(lease); err != nil {
		_ = worker.releaseLease(lease)
		return base, err
	}
	workerCtx, cancel := context.WithDeadline(ctx, command.Deadline)
	defer cancel()
	outcomeChannel := make(chan handlerOutcome, 1)
	go func() {
		result, handlerErr := handler.Execute(workerCtx, command)
		outcomeChannel <- handlerOutcome{result: result, err: handlerErr}
	}()
	var executeErr error
	select {
	case outcome := <-outcomeChannel:
		executeErr = outcome.err
		if workerCtx.Err() != nil {
			base.State, base.ReasonCode, executeErr = ReceiptTimedOut, ReasonDeadlineExceeded, workerCtx.Err()
		} else if outcome.err != nil {
			base.State, base.ReasonCode = ReceiptFailed, ReasonHandlerFailure
			if errors.Is(outcome.err, scheduler.ErrCapabilityUnavailable) || outcome.result.State == ReceiptUnavailable {
				base.State, base.ReasonCode = ReceiptUnavailable, ReasonHandlerUnavailable
			}
		} else {
			result := outcome.result
			if !validReasonCode(result.ReasonCode) {
				result.ReasonCode = ReasonHandlerFailure
				if result.State == ReceiptSucceeded {
					result.ReasonCode = ReasonCompleted
				} else if result.State == ReceiptReviewedNoChange {
					result.ReasonCode = ReasonReviewedNoChange
				} else if result.State == ReceiptProposalEmitted {
					result.ReasonCode = ReasonProposalEmitted
				}
			}
			base.State, base.ProposalCount, base.ProposalDigest, base.ProposalArtifactID, base.ReasonCode = result.State, result.ProposalCount, result.ProposalDigest, result.ProposalArtifactID, result.ReasonCode
			if base.State == "" {
				base.State = ReceiptSucceeded
			}
		}
	case <-workerCtx.Done():
		executeErr = workerCtx.Err()
		base.State, base.ReasonCode = ReceiptTimedOut, ReasonDeadlineExceeded
		// Fencing must survive the normal lease TTL while the late handler is
		// still capable of side effects. A successor remains denied until the
		// original handler exits and releases this quarantine marker.
		if quarantineErr := worker.Scheduler.QuarantineLease(lease); quarantineErr != nil {
			return base, quarantineErr
		}
		if err := worker.publishOccurrence(request.WorkspaceID, occurrence, now, lease, base); err != nil {
			worker.boundLateHandler(outcomeChannel, lease, base)
			return base, err
		}
		worker.boundLateHandler(outcomeChannel, lease, base)
		return base, executeErr
	}
	if err := worker.publishOccurrence(request.WorkspaceID, occurrence, now, lease, base); err != nil {
		_ = worker.releaseLease(lease)
		return base, err
	}
	if err := worker.releaseLease(lease); err != nil {
		recovery := worker.recoveryReceipt(base, now)
		_ = worker.Receipts.AppendReceipt(recovery)
		_ = worker.Scheduler.AppendReceipt(request.WorkspaceID, scheduler.Receipt{JobID: occurrence.JobID, ScheduledFor: occurrence.ScheduledFor, AttemptedAt: now.UTC(), State: scheduler.Failed, Error: string(ReasonRecoveryRequired)})
		return recovery, err
	}
	return base, executeErr
}

// boundLateHandler prevents an uncooperative handler from creating an
// unbounded cleanup goroutine. Quarantine keeps the occurrence fenced beyond
// lease TTL; a late result is never published by this goroutine. If the
// handler exits within the bounded cleanup window, releasing the lease is
// safe and allows prompt retry. If it never exits, manual recovery is required
// after the original process is gone.
func (worker Worker) boundLateHandler(outcomeChannel <-chan handlerOutcome, lease scheduler.Lease, base Receipt) {
	cleanup := worker.Deadline
	if cleanup <= 0 || cleanup > 15*time.Minute {
		cleanup = 2 * time.Minute
	}
	go func() {
		timer := time.NewTimer(cleanup)
		defer timer.Stop()
		select {
		case <-outcomeChannel:
			if err := worker.releaseLease(lease); err != nil {
				_ = worker.Receipts.AppendReceipt(worker.recoveryReceipt(base, worker.currentTime()))
			}
		case <-timer.C:
			// Quarantine deliberately survives lease expiry. A stuck handler
			// requires explicit operator recovery after its process is gone.
		}
	}()
}

func (worker Worker) releaseLease(lease scheduler.Lease) error {
	if worker.ReleaseLease != nil {
		return worker.ReleaseLease(lease)
	}
	return worker.Scheduler.ReleaseLease(lease)
}

func (worker Worker) currentTime() time.Time {
	if worker.Now != nil {
		return worker.Now().UTC()
	}
	return time.Now().UTC()
}

func (worker Worker) recoveryReceipt(base Receipt, now time.Time) Receipt {
	attempt, err := attemptID()
	if err != nil {
		attempt = base.AttemptID
	}
	base.AttemptID = attempt
	base.State = ReceiptRecoveryRequired
	base.ProposalCount = 0
	base.ProposalDigest = ""
	base.ProposalArtifactID = ""
	base.ReasonCode = ReasonRecoveryRequired
	base.RecordedAt = now.UTC()
	return base
}

func (worker Worker) publishOccurrence(workspaceID string, occurrence scheduler.Occurrence, now time.Time, lease scheduler.Lease, receipt Receipt) error {
	return worker.Scheduler.WithCurrentLease(lease, now, func() error {
		if err := worker.Receipts.AppendReceipt(receipt); err != nil {
			return err
		}
		state := scheduler.Failed
		if receipt.State == ReceiptSucceeded || receipt.State == ReceiptReviewedNoChange || receipt.State == ReceiptProposalEmitted {
			state = scheduler.Succeeded
		}
		return worker.Scheduler.AppendReceipt(workspaceID, scheduler.Receipt{JobID: occurrence.JobID, ScheduledFor: occurrence.ScheduledFor, AttemptedAt: now.UTC(), State: state, Error: schedulerError(receipt)})
	})
}

func (worker Worker) unavailableReceipt(workspaceID string, occurrence scheduler.Occurrence, now time.Time, reason ReasonCode) (Receipt, error) {
	id, err := attemptID()
	if err != nil {
		return Receipt{}, err
	}
	if !validReasonCode(reason) {
		reason = ReasonHandlerUnavailable
	}
	receipt := Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: id, OccurrenceDigest: digestOccurrence(occurrence), CommandID: "unavailable-" + digestPrefix(occurrence.JobID+occurrence.ScheduledFor.String()), JobID: occurrence.JobID, WorkspaceID: workspaceID, Trigger: triggerForCadence(worker.Jobs, occurrence.JobID), State: ReceiptUnavailable, RecordedAt: now, Deadline: now.Add(worker.Deadline), ProposalOnly: occurrence.JobID == "darwin-structural-evolution-proposal", ReasonCode: reason}
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
func schedulerError(receipt Receipt) string {
	if receipt.State == ReceiptSucceeded || receipt.State == ReceiptProposalEmitted {
		return ""
	}
	return string(receipt.ReasonCode)
}
