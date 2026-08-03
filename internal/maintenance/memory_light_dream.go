package maintenance

import (
	"context"
	"errors"
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

type MemoryLightDreamHandler struct {
	Engine *memory.Engine
}

func (handler MemoryLightDreamHandler) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if err := ValidateExecutionGrant(grant, command); err != nil {
		return HandlerResult{}, err
	}
	if command.JobID != MemoryLightDreamJobID || command.WorkspaceID == "" || command.Trigger != TriggerPresence || handler.Engine == nil {
		return HandlerResult{}, errors.New("memory light dream command is outside its bounded L1 boundary")
	}
	result, err := handler.Engine.DreamDailyAttested(ctx, command.WorkspaceID, command.RequestedAt)
	if errors.Is(err, os.ErrNotExist) {
		return HandlerResult{State: ReceiptReviewedNoChange, ReasonCode: ReasonReviewedNoChange}, nil
	}
	if err != nil {
		return HandlerResult{}, err
	}
	if result.Skipped {
		return HandlerResult{State: ReceiptReviewedNoChange, ReasonCode: ReasonReviewedNoChange}, nil
	}
	return HandlerResult{State: ReceiptSucceeded, ReasonCode: ReasonCompleted}, nil
}
