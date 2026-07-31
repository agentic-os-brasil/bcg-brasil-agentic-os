package darwin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const HousekeepingJobID = "darwin-housekeeping"

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

// HousekeepingExecutor is the scheduler-facing Darwin seam. Headless mode is
// explicit in the packet, but uses the same Plan, grants, invoker and receipt
// path as interactive Darwin execution.
type HousekeepingExecutor struct {
	Build        HealthPacketBuilder
	Guard        ToolGuard
	Invoker      ToolInvoker
	Store        Store
	CommandStore maintenance.Store
	Scheduler    scheduler.Store
	Now          func() time.Time
}

func (executor HousekeepingExecutor) Execute(ctx context.Context, occurrence scheduler.Occurrence) error {
	if occurrence.JobID != HousekeepingJobID || executor.Build == nil || executor.Guard == nil || executor.Invoker == nil {
		return errors.New("invalid Darwin housekeeping executor")
	}
	packet, err := executor.Build.Build(ctx, occurrence)
	if err != nil {
		return err
	}
	if packet.Mode != "" && packet.Mode != Interactive && packet.Mode != HeadlessHousekeeping {
		return errors.New("Darwin housekeeping packet uses an incompatible mode")
	}
	packet.Mode = HeadlessHousekeeping
	assessment, err := Plan(packet)
	if err != nil {
		return err
	}
	recordedAt := time.Now
	if executor.Now != nil {
		recordedAt = executor.Now
	}
	receipt, executeErr := Execute(ctx, packet, assessment, executor.Guard, executor.Invoker, recordedAt)
	if storeErr := executor.Store.Append(receipt); storeErr != nil {
		return storeErr
	}
	if executeErr != nil {
		return executeErr
	}
	if receipt.Outcome == OutcomeBlocked || receipt.Outcome == OutcomeFailed {
		return fmt.Errorf("Darwin housekeeping %s", receipt.Outcome)
	}
	return nil
}

// ExecuteCommand is the worker-owned Darwin entrypoint. Hooks may construct a
// command, but only this bounded worker acquires a lease and executes it.
func (executor HousekeepingExecutor) ExecuteCommand(ctx context.Context, command maintenance.Command) (maintenance.Receipt, error) {
	now := time.Now().UTC()
	if executor.Now != nil {
		now = executor.Now().UTC()
	}
	base := maintenance.Receipt{SchemaVersion: maintenance.CommandSchemaVersion, CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: maintenance.ReceiptAccepted, RecordedAt: now, Deadline: command.Deadline, ProposalOnly: command.ProposalOnly}
	if err := command.Validate(now); err != nil {
		base.State = maintenance.ReceiptUnavailable
		base.Diagnostic = "command rejected by bounded validation"
		return base, err
	}
	if executor.Build == nil || executor.Store.Root == "" || executor.Scheduler.Root == "" || executor.CommandStore.Root == "" {
		base.State = maintenance.ReceiptUnavailable
		base.Diagnostic = "Darwin worker dependencies are unavailable"
		return base, errors.New("Darwin command executor is not fully configured")
	}
	if existing, readErr := executor.CommandStore.Receipts(command.WorkspaceID, command.JobID); readErr == nil && len(existing) > 0 {
		for _, receipt := range existing {
			if receipt.CommandID == command.CommandID {
				return receipt, nil
			}
		}
	} else if readErr != nil {
		return base, readErr
	}
	lease, err := executor.Scheduler.TryAcquireLease(command.WorkspaceID, command.JobID, command.CommandID, command.CommandID, now, command.Deadline.Sub(now))
	if err != nil {
		if errors.Is(err, scheduler.ErrLeaseBusy) {
			base.State = maintenance.ReceiptBusy
			base.Diagnostic = "another bounded worker already owns this command"
			_ = executor.CommandStore.AppendReceipt(base)
		}
		return base, err
	}
	defer func() {
		_ = executor.Scheduler.ReleaseLease(command.WorkspaceID, command.JobID, command.CommandID, lease.OwnerID)
	}()

	workerCtx, cancel := context.WithDeadline(ctx, command.Deadline)
	defer cancel()
	occurrence := scheduler.Occurrence{JobID: command.JobID, ScheduledFor: command.ScheduledFor}
	if command.ProposalOnly {
		packet, buildErr := executor.Build.Build(workerCtx, occurrence)
		if buildErr != nil {
			base.State = maintenance.ReceiptFailed
			base.Diagnostic = "Darwin proposal packet was not built"
			_ = executor.CommandStore.AppendReceipt(base)
			return base, buildErr
		}
		packet.Mode = DeepReview
		assessment, planErr := Plan(packet)
		if planErr != nil {
			base.State = maintenance.ReceiptFailed
			base.Diagnostic = "Darwin proposal plan was rejected"
			_ = executor.CommandStore.AppendReceipt(base)
			return base, planErr
		}
		base.State = maintenance.ReceiptProposalEmitted
		base.ProposalCount = len(assessment.Proposals)
		base.ProposalDigest = proposalDigest(command.CommandID, assessment)
		base.Diagnostic = "proposal emitted; approval and application remain separate"
		if err := executor.CommandStore.AppendReceipt(base); err != nil {
			return base, err
		}
		return base, nil
	}

	packet, buildErr := executor.Build.Build(workerCtx, occurrence)
	if buildErr != nil {
		base.State = maintenance.ReceiptFailed
		base.Diagnostic = "Darwin housekeeping packet was not built"
		_ = executor.CommandStore.AppendReceipt(base)
		return base, buildErr
	}
	packet.Mode = HeadlessHousekeeping
	assessment, planErr := Plan(packet)
	if planErr != nil {
		base.State = maintenance.ReceiptFailed
		base.Diagnostic = "Darwin housekeeping plan was rejected"
		_ = executor.CommandStore.AppendReceipt(base)
		return base, planErr
	}
	receipt, executeErr := Execute(workerCtx, packet, assessment, executor.Guard, executor.Invoker, func() time.Time { return now })
	if errors.Is(workerCtx.Err(), context.DeadlineExceeded) {
		base.State = maintenance.ReceiptTimedOut
		base.Diagnostic = "Darwin housekeeping exceeded its explicit deadline"
		_ = executor.CommandStore.AppendReceipt(base)
		return base, context.DeadlineExceeded
	}
	if executeErr != nil || receipt.Outcome == OutcomeBlocked || receipt.Outcome == OutcomeFailed {
		base.State = maintenance.ReceiptFailed
		base.Diagnostic = "Darwin housekeeping completed without a successful repair boundary"
		_ = executor.CommandStore.AppendReceipt(base)
		if executeErr != nil {
			return base, executeErr
		}
		return base, fmt.Errorf("Darwin housekeeping %s", receipt.Outcome)
	}
	base.State = maintenance.ReceiptSucceeded
	base.Diagnostic = "Darwin housekeeping completed within its explicit deadline"
	if err := executor.CommandStore.AppendReceipt(base); err != nil {
		return base, err
	}
	return base, nil
}

func proposalDigest(commandID string, assessment Assessment) string {
	material := commandID
	for _, proposal := range assessment.Proposals {
		material += "\x00" + proposal.ID + "\x00" + string(proposal.Action)
	}
	digest := sha256.Sum256([]byte(material))
	return hex.EncodeToString(digest[:])
}
