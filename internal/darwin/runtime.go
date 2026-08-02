package darwin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const HousekeepingJobID = "darwin-housekeeping-daily"

// HealthPacketBuilder is the only runtime-specific input seam. Claude and
// Codex build the same closed packet; neither runtime gets a second Darwin
// implementation for housekeeping.
type HealthPacketBuilder interface {
	Build(context.Context, scheduler.Occurrence) (HealthPacket, error)
}

type HealthPacketBuilderFunc func(context.Context, scheduler.Occurrence) (HealthPacket, error)

func (function HealthPacketBuilderFunc) Build(ctx context.Context, occurrence scheduler.Occurrence) (HealthPacket, error) {
	return function(ctx, occurrence)
}

// HousekeepingExecutor is the command-gated Darwin worker. It intentionally
// does not implement scheduler.Executor: raw occurrences cannot bypass the
// catalog authority, command deadline or occurrence lease.
type HousekeepingExecutor struct {
	Build         HealthPacketBuilder
	Guard         ToolGuard
	Invoker       ToolInvoker
	Store         Store
	CommandStore  maintenance.Store
	ProposalStore ProposalStore
	Scheduler     scheduler.Store
	Authority     maintenance.ExecutionAuthority
	Now           func() time.Time
	ArmLease      func(scheduler.Lease) error
	ReleaseLease  func(scheduler.Lease) error
}

// ExecuteCommand is the worker-owned Darwin entrypoint. Hooks may construct a
// command, but only this bounded worker acquires a lease and executes it.
func (executor HousekeepingExecutor) ExecuteCommand(ctx context.Context, command maintenance.Command) (maintenance.Receipt, error) {
	now := executor.currentTime()
	attemptID, attemptErr := newAttemptID()
	base := maintenance.Receipt{SchemaVersion: maintenance.CommandSchemaVersion, AttemptID: attemptID, OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: maintenance.ReceiptAccepted, RecordedAt: now, Deadline: command.Deadline, ProposalOnly: command.ProposalOnly, ReasonCode: maintenance.ReasonHandlerFailure}
	if attemptErr != nil {
		base.State = maintenance.ReceiptUnavailable
		base.ReasonCode = maintenance.ReasonHandlerUnavailable
		return base, attemptErr
	}
	if err := command.Validate(now); err != nil {
		base.State = maintenance.ReceiptUnavailable
		base.ReasonCode = maintenance.ReasonOccurrenceRejected
		return base, err
	}
	if executor.Build == nil || executor.Store.Root == "" || executor.Scheduler.Root == "" || executor.CommandStore.Root == "" || !executor.Authority.Ready() {
		base.State = maintenance.ReceiptUnavailable
		base.ReasonCode = maintenance.ReasonHandlerUnavailable
		return base, errors.New("Darwin command executor is not fully configured")
	}
	if err := validateDarwinCommand(command); err != nil {
		base.State = maintenance.ReceiptUnavailable
		base.ReasonCode = maintenance.ReasonAuthorityRejected
		return base, err
	}
	if !command.ProposalOnly && (executor.Guard == nil || executor.Invoker == nil) {
		base.State = maintenance.ReceiptUnavailable
		base.ReasonCode = maintenance.ReasonAuthorityRejected
		return base, errors.New("Darwin housekeeping guard and invoker are required")
	}
	occurrence, err := executor.Authority.Authorize(command, now)
	if err != nil {
		base.State = maintenance.ReceiptUnavailable
		base.ReasonCode = maintenance.ReasonAuthorityRejected
		return base, err
	}
	if occurrence.JobID != command.JobID || !occurrence.ScheduledFor.Equal(command.ScheduledFor) {
		base.State = maintenance.ReceiptUnavailable
		base.ReasonCode = maintenance.ReasonOccurrenceRejected
		return base, errors.New("Darwin command occurrence authority mismatch")
	}
	lease, err := executor.Scheduler.TryAcquireLease(command.WorkspaceID, command.JobID, command.OccurrenceKey(), attemptID, now, command.Deadline.Sub(now))
	if err != nil {
		if errors.Is(err, scheduler.ErrLeaseBusy) {
			base.State = maintenance.ReceiptBusy
			base.ReasonCode = maintenance.ReasonLeaseBusy
		}
		return base, err
	}
	if err := executor.armLease(lease); err != nil {
		if releaseErr := executor.releaseLease(lease); releaseErr != nil {
			return executor.persistRecoveryEvidence(base, errors.Join(err, releaseErr))
		}
		return base, err
	}
	var result maintenance.Receipt
	var executionErr error
	guardErr := executor.Scheduler.WithCurrentLease(lease, now, func() error {
		if existing, found, readErr := executor.existingTerminalReceipt(command); readErr != nil {
			return readErr
		} else if found {
			result = existing
			return nil
		}
		if command.ProposalOnly {
			if recovered, found, recoveryErr := executor.recoverProposalArtifact(command); recoveryErr != nil {
				return recoveryErr
			} else if found {
				result = recovered
				return nil
			}
		}
		result, executionErr = executor.executeLeasedCommand(ctx, command, occurrence, base, now)
		return nil
	})
	if guardErr != nil {
		if releaseErr := executor.releaseLease(lease); releaseErr != nil {
			return executor.persistRecoveryEvidence(base, errors.Join(guardErr, releaseErr))
		}
		return base, guardErr
	}
	if releaseErr := executor.releaseLease(lease); releaseErr != nil {
		return executor.persistRecoveryEvidence(result, releaseErr)
	}
	return result, executionErr
}

// recoverProposalArtifact closes the monthly crash window where the
// occurrence artifact was published before the command receipt. It is called
// only after authority and the occurrence fence are held, but before health
// construction, so changed health cannot create a second structural
// assessment for the same occurrence.
func (executor HousekeepingExecutor) recoverProposalArtifact(command maintenance.Command) (maintenance.Receipt, bool, error) {
	proposalStore := executor.ProposalStore
	if proposalStore.Root == "" {
		proposalStore = ProposalStore{Root: executor.CommandStore.Root}
	}
	artifact, err := proposalStore.ReadOccurrence(command.JobID, command.OccurrenceDigest())
	if errors.Is(err, os.ErrNotExist) {
		return maintenance.Receipt{}, false, nil
	}
	if err != nil {
		return maintenance.Receipt{}, false, err
	}
	if existing, readErr := executor.CommandStore.Receipts(command.WorkspaceID, command.JobID); readErr != nil {
		return maintenance.Receipt{}, false, readErr
	} else {
		for _, receipt := range existing {
			if receipt.OccurrenceDigest == command.OccurrenceDigest() && receipt.State == maintenance.ReceiptProposalEmitted {
				if err := proposalStore.ValidateReceipt(receipt); err != nil {
					return maintenance.Receipt{}, false, err
				}
				return receipt, true, nil
			}
		}
	}
	attempt, err := newAttemptID()
	if err != nil {
		return maintenance.Receipt{}, false, err
	}
	recovered := maintenance.Receipt{SchemaVersion: maintenance.CommandSchemaVersion, AttemptID: attempt, OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: maintenance.ReceiptProposalEmitted, RecordedAt: executor.currentTime(), Deadline: command.Deadline, ProposalOnly: true, ProposalCount: len(artifact.Assessment.Proposals), ProposalDigest: artifact.ProposalDigest, ProposalArtifactID: artifact.ArtifactID, ReasonCode: maintenance.ReasonProposalEmitted}
	if err := executor.CommandStore.AppendReceipt(recovered); err != nil {
		return maintenance.Receipt{}, false, err
	}
	return recovered, true, nil
}

func (executor HousekeepingExecutor) existingTerminalReceipt(command maintenance.Command) (maintenance.Receipt, bool, error) {
	existing, err := executor.CommandStore.Receipts(command.WorkspaceID, command.JobID)
	if err != nil {
		return maintenance.Receipt{}, false, err
	}
	for _, receipt := range existing {
		if receipt.OccurrenceDigest != command.OccurrenceDigest() || (receipt.State != maintenance.ReceiptSucceeded && receipt.State != maintenance.ReceiptReviewedNoChange && receipt.State != maintenance.ReceiptProposalEmitted) {
			continue
		}
		if receipt.State == maintenance.ReceiptProposalEmitted {
			proposalStore := executor.ProposalStore
			if proposalStore.Root == "" {
				proposalStore = ProposalStore{Root: executor.CommandStore.Root}
			}
			if err := proposalStore.ValidateReceipt(receipt); err != nil {
				return maintenance.Receipt{}, false, err
			}
		}
		return receipt, true, nil
	}
	return maintenance.Receipt{}, false, nil
}

func (executor HousekeepingExecutor) releaseLease(lease scheduler.Lease) error {
	if executor.ReleaseLease != nil {
		return executor.ReleaseLease(lease)
	}
	return executor.Scheduler.ReleaseLease(lease)
}

func (executor HousekeepingExecutor) armLease(lease scheduler.Lease) error {
	if executor.ArmLease != nil {
		return executor.ArmLease(lease)
	}
	return executor.Scheduler.ArmLease(lease)
}

func (executor HousekeepingExecutor) persistRecoveryEvidence(base maintenance.Receipt, cause error) (maintenance.Receipt, error) {
	recovery := recoveryRequiredReceipt(base, executor.currentTime())
	if appendErr := executor.CommandStore.AppendReceipt(recovery); appendErr != nil {
		return recovery, errors.Join(cause, appendErr)
	}
	return recovery, cause
}

func recoveryRequiredReceipt(base maintenance.Receipt, now time.Time) maintenance.Receipt {
	attempt, err := newAttemptID()
	if err == nil {
		base.AttemptID = attempt
	}
	base.State = maintenance.ReceiptRecoveryRequired
	base.ProposalCount = 0
	base.ProposalDigest = ""
	base.ProposalArtifactID = ""
	base.ReasonCode = maintenance.ReasonRecoveryRequired
	base.RecordedAt = now.UTC()
	return base
}

// executeLeasedCommand runs while the scheduler's per-occurrence OS guard is
// held. The guard fences both tool side effects and terminal receipt
// publication; an expired lease may be reclaimed only after the prior process
// releases or loses OS ownership.
func (executor HousekeepingExecutor) executeLeasedCommand(ctx context.Context, command maintenance.Command, occurrence scheduler.Occurrence, base maintenance.Receipt, startedAt time.Time) (maintenance.Receipt, error) {
	workerCtx, cancel := context.WithDeadline(ctx, command.Deadline)
	defer cancel()
	if command.ProposalOnly {
		packet, buildErr := executor.Build.Build(workerCtx, occurrence)
		if buildErr != nil {
			base.State = maintenance.ReceiptFailed
			base.ReasonCode = maintenance.ReasonHandlerFailure
			base.RecordedAt = executor.currentTime()
			if persistErr := executor.CommandStore.AppendReceipt(base); persistErr != nil {
				return base, persistErr
			}
			return base, buildErr
		}
		packet.Mode = DeepReview
		assessment, planErr := Plan(packet)
		if planErr != nil {
			base.State = maintenance.ReceiptFailed
			base.ReasonCode = maintenance.ReasonHandlerFailure
			base.RecordedAt = executor.currentTime()
			if persistErr := executor.CommandStore.AppendReceipt(base); persistErr != nil {
				return base, persistErr
			}
			return base, planErr
		}
		base.ProposalCount = len(assessment.Proposals)
		proposalStore := executor.ProposalStore
		if proposalStore.Root == "" {
			proposalStore = ProposalStore{Root: executor.CommandStore.Root}
		}
		if len(assessment.Proposals) > 0 {
			base.State = maintenance.ReceiptProposalEmitted
			base.ProposalDigest = proposalDigest(command.OccurrenceDigest(), assessment)
			artifact := AssessmentProposalArtifact{SchemaVersion: proposalArtifactSchemaVersion, RecordType: "assessment", AgentID: AgentID, JobID: command.JobID, OccurrenceDigest: command.OccurrenceDigest(), ArtifactID: assessmentArtifactID(command.JobID, command.OccurrenceDigest()), WindowID: assessment.WindowID, ProposalDigest: base.ProposalDigest, Assessment: assessment, ScheduledFor: command.ScheduledFor.UTC(), RecordedAt: command.ScheduledFor.UTC()}
			if err := proposalStore.Append(artifact); err != nil {
				return base, err
			}
			base.ProposalArtifactID = artifact.ArtifactID
		} else {
			base.State = maintenance.ReceiptReviewedNoChange
			base.ProposalDigest = ""
			base.ReasonCode = maintenance.ReasonReviewedNoChange
			base.RecordedAt = executor.currentTime()
			if err := executor.CommandStore.AppendReceipt(base); err != nil {
				return base, err
			}
			return base, nil
		}
		base.ReasonCode = maintenance.ReasonProposalEmitted
		base.RecordedAt = executor.currentTime()
		if err := executor.CommandStore.AppendReceipt(base); err != nil {
			return base, err
		}
		return base, nil
	}

	packet, buildErr := executor.Build.Build(workerCtx, occurrence)
	if buildErr != nil {
		base.State = maintenance.ReceiptFailed
		base.ReasonCode = maintenance.ReasonHandlerFailure
		base.RecordedAt = executor.currentTime()
		if persistErr := executor.CommandStore.AppendReceipt(base); persistErr != nil {
			return base, persistErr
		}
		return base, buildErr
	}
	packet.Mode = HeadlessHousekeeping
	assessment, planErr := Plan(packet)
	if planErr != nil {
		base.State = maintenance.ReceiptFailed
		base.ReasonCode = maintenance.ReasonHandlerFailure
		base.RecordedAt = executor.currentTime()
		if persistErr := executor.CommandStore.AppendReceipt(base); persistErr != nil {
			return base, persistErr
		}
		return base, planErr
	}
	receipt, executeErr := Execute(workerCtx, packet, assessment, executor.Guard, executor.Invoker, func() time.Time { return startedAt })
	if errors.Is(workerCtx.Err(), context.DeadlineExceeded) {
		base.State = maintenance.ReceiptTimedOut
		base.ReasonCode = maintenance.ReasonDeadlineExceeded
		base.RecordedAt = executor.currentTime()
		if persistErr := executor.CommandStore.AppendReceipt(base); persistErr != nil {
			return base, persistErr
		}
		return base, context.DeadlineExceeded
	}
	if storeErr := executor.Store.Append(receipt); storeErr != nil {
		base.State = maintenance.ReceiptFailed
		base.ReasonCode = maintenance.ReasonHandlerFailure
		base.RecordedAt = executor.currentTime()
		if persistErr := executor.CommandStore.AppendReceipt(base); persistErr != nil {
			return base, persistErr
		}
		return base, storeErr
	}
	if executeErr != nil || receipt.Outcome == OutcomeBlocked || receipt.Outcome == OutcomeFailed {
		base.State = maintenance.ReceiptFailed
		base.ReasonCode = maintenance.ReasonHandlerFailure
		base.RecordedAt = executor.currentTime()
		if persistErr := executor.CommandStore.AppendReceipt(base); persistErr != nil {
			return base, persistErr
		}
		if executeErr != nil {
			return base, executeErr
		}
		return base, fmt.Errorf("Darwin housekeeping %s", receipt.Outcome)
	}
	if receipt.Outcome == OutcomeNoAction {
		base.State = maintenance.ReceiptReviewedNoChange
		base.ReasonCode = maintenance.ReasonReviewedNoChange
		base.RecordedAt = executor.currentTime()
		if err := executor.CommandStore.AppendReceipt(base); err != nil {
			return base, err
		}
		return base, nil
	}
	if receipt.Outcome == OutcomePartial {
		base.State = maintenance.ReceiptFailed
		base.ReasonCode = maintenance.ReasonHandlerFailure
		base.RecordedAt = executor.currentTime()
		if err := executor.CommandStore.AppendReceipt(base); err != nil {
			return base, err
		}
		return base, errors.New("Darwin housekeeping did not complete every planned action")
	}
	base.State = maintenance.ReceiptSucceeded
	base.ReasonCode = maintenance.ReasonCompleted
	base.RecordedAt = executor.currentTime()
	if err := executor.CommandStore.AppendReceipt(base); err != nil {
		return base, err
	}
	return base, nil
}

func (executor HousekeepingExecutor) currentTime() time.Time {
	if executor.Now != nil {
		return executor.Now().UTC()
	}
	return time.Now().UTC()
}

func validateDarwinCommand(command maintenance.Command) error {
	switch command.JobID {
	case HousekeepingJobID:
		if command.ProposalOnly {
			return errors.New("Darwin housekeeping cannot be proposal-only")
		}
		switch command.Trigger {
		case maintenance.TriggerContinuous, maintenance.TriggerEvent, maintenance.TriggerPresence, maintenance.TriggerDaily, maintenance.TriggerWeekly:
			return nil
		default:
			return errors.New("Darwin housekeeping trigger is not authorized")
		}
	case "darwin-structural-evolution-proposal":
		if !command.ProposalOnly || command.Trigger != maintenance.TriggerMonthly {
			return errors.New("Darwin structural evolution requires an attended monthly proposal command")
		}
		return nil
	default:
		return errors.New("maintenance job is not owned by the Darwin worker")
	}
}

func newAttemptID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func proposalDigest(occurrenceDigest string, assessment Assessment) string {
	body, _ := json.Marshal(struct {
		OccurrenceDigest string
		Assessment       Assessment
	}{OccurrenceDigest: occurrenceDigest, Assessment: assessment})
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
