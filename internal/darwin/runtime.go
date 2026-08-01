package darwin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
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
	if existing, readErr := executor.CommandStore.Receipts(command.WorkspaceID, command.JobID); readErr == nil && len(existing) > 0 {
		for _, receipt := range existing {
			if receipt.OccurrenceDigest == command.OccurrenceDigest() && (receipt.State == maintenance.ReceiptSucceeded || receipt.State == maintenance.ReceiptProposalEmitted) {
				return receipt, nil
			}
		}
	} else if readErr != nil {
		return base, readErr
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
	defer func() {
		_ = executor.Scheduler.ReleaseLease(lease)
	}()

	var result maintenance.Receipt
	var executionErr error
	guardErr := executor.Scheduler.WithCurrentLease(lease, now, func() error {
		result, executionErr = executor.executeLeasedCommand(ctx, command, occurrence, base, now)
		return nil
	})
	if guardErr != nil {
		return base, guardErr
	}
	return result, executionErr
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
		base.State = maintenance.ReceiptProposalEmitted
		base.ProposalCount = len(assessment.Proposals)
		base.ProposalDigest = proposalDigest(command.OccurrenceDigest(), assessment)
		proposalStore := executor.ProposalStore
		if proposalStore.Root == "" {
			proposalStore = ProposalStore{Root: executor.CommandStore.Root}
		}
		if len(assessment.Proposals) > 0 {
			artifact := AssessmentProposalArtifact{SchemaVersion: proposalArtifactSchemaVersion, RecordType: "assessment", AgentID: AgentID, JobID: command.JobID, OccurrenceDigest: command.OccurrenceDigest(), WindowID: assessment.WindowID, ProposalDigest: base.ProposalDigest, Assessment: assessment, ScheduledFor: command.ScheduledFor.UTC(), RecordedAt: command.ScheduledFor.UTC()}
			if err := proposalStore.Append(artifact); err != nil {
				return base, err
			}
			base.ProposalArtifactID = base.ProposalDigest
		} else {
			base.State = maintenance.ReceiptUnavailable
			base.ProposalDigest = ""
			base.ReasonCode = maintenance.ReasonHandlerFailure
			base.RecordedAt = executor.currentTime()
			if err := executor.CommandStore.AppendReceipt(base); err != nil {
				return base, err
			}
			return base, errors.New("Darwin structural review produced no proposal")
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
		CommandID  string
		Assessment Assessment
	}{CommandID: occurrenceDigest, Assessment: assessment})
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
