package maintenance

import (
	"context"
	"errors"
)

// MemoryCheckpointHandler closes only the metadata receipt boundary owned by
// the maintenance worker. It never reads or synthesizes memory bodies.
type MemoryCheckpointHandler struct{}

func (MemoryCheckpointHandler) ExecuteAuthorized(_ context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if err := ValidateExecutionGrant(grant, command); err != nil {
		return HandlerResult{}, err
	}
	if command.JobID != MemoryCheckpointJobID || command.WorkspaceID == "" || command.Trigger != TriggerPresence {
		return HandlerResult{}, errors.New("memory checkpoint command is outside its workspace continuity boundary")
	}
	return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
}
