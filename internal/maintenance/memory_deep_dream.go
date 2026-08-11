package maintenance

import (
	"context"
	"errors"
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

// MemoryDeepDreamHandler runs the bundled deterministic L2/L3 rollup and the
// named lifetime eligibility policy through the same atomic memory engine.
type MemoryDeepDreamHandler struct {
	Engine *memory.Engine
}

func (handler MemoryDeepDreamHandler) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if err := ValidateExecutionGrant(grant, command); err != nil {
		return HandlerResult{}, err
	}
	if command.JobID != MemoryDeepDreamJobID || command.WorkspaceID == "" || command.Trigger != TriggerPresence || handler.Engine == nil {
		return HandlerResult{}, errors.New("memory deep dream command is outside its bounded weekly boundary")
	}
	result, err := handler.Engine.DreamWeekly(ctx, command.WorkspaceID, command.RequestedAt)
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
