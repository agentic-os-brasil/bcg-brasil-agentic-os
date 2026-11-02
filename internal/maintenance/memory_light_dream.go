package maintenance

import (
	"context"
	"errors"
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

type MemoryLightDreamHandler struct {
	Engine                    *memory.Engine
	ExpectedSourceFingerprint string
}

func (handler MemoryLightDreamHandler) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if err := ValidateExecutionGrant(grant, command); err != nil {
		return HandlerResult{}, err
	}
	if command.JobID != MemoryLightDreamJobID || command.WorkspaceID == "" || command.Trigger != TriggerPresence || handler.Engine == nil {
		return HandlerResult{}, errors.New("memory light dream command is outside its bounded L1 boundary")
	}
	engine := *handler.Engine
	engine.ExpectedDailyFingerprint = handler.ExpectedSourceFingerprint
	engine.PublicationGuard = func(publish func() error) error { return publishWithGrant(ctx, grant, command, publish) }
	result, err := engine.DreamDailyAttested(ctx, command.WorkspaceID, command.RequestedAt)
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, memory.ErrNoLiveCaptures) {
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
