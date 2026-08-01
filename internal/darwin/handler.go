package darwin

import (
	"context"
	"errors"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// HousekeepingHandler adapts the existing command-gated Darwin implementation
// to the generic maintenance worker. The worker owns the occurrence lease;
// this handler owns only packet construction, scoped repair and Darwin health
// receipt publication. It is deliberately not a Walter/self implementation.
type HousekeepingHandler struct {
	Build        HealthPacketBuilder
	Guard        ToolGuard
	Invoker      ToolInvoker
	Store        Store
	CommandStore maintenance.Store
	Now          func() time.Time
}

func (handler HousekeepingHandler) Execute(ctx context.Context, command maintenance.Command) (maintenance.HandlerResult, error) {
	if command.JobID != HousekeepingJobID || command.ProposalOnly {
		return maintenance.HandlerResult{}, errors.New("Darwin housekeeping handler received an unauthorized job")
	}
	attempt, err := newAttemptID()
	if err != nil {
		return maintenance.HandlerResult{}, err
	}
	now := time.Now().UTC()
	if handler.Now != nil {
		now = handler.Now().UTC()
	}
	base := maintenance.Receipt{SchemaVersion: maintenance.CommandSchemaVersion, AttemptID: attempt, OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: maintenance.ReceiptFailed, RecordedAt: now, Deadline: command.Deadline}
	executor := HousekeepingExecutor{Build: handler.Build, Guard: handler.Guard, Invoker: handler.Invoker, Store: handler.Store, CommandStore: handler.CommandStore, Now: handler.Now}
	receipt, executeErr := executor.executeLeasedCommand(ctx, command, scheduler.Occurrence{JobID: command.JobID, ScheduledFor: command.ScheduledFor}, base, now)
	if executeErr != nil {
		return maintenance.HandlerResult{State: receipt.State, Diagnostic: receipt.Diagnostic}, executeErr
	}
	if receipt.State != maintenance.ReceiptSucceeded {
		return maintenance.HandlerResult{State: receipt.State, Diagnostic: receipt.Diagnostic}, errors.New("Darwin housekeeping did not reach a successful boundary")
	}
	return maintenance.HandlerResult{State: maintenance.ReceiptSucceeded, Diagnostic: receipt.Diagnostic}, nil
}
