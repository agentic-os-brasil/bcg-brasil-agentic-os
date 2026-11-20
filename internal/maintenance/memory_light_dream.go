package maintenance

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
)

type MemoryLightDreamHandler struct {
	Engine                    *memory.Engine
	ExpectedSourceFingerprint string
	// ProductSelection pins the explicitly selected canonical ingestion day.
	// Other callers retain the existing RequestedAt single-day behavior.
	ProductSelection bool
	SelectedInput    *memory.DailyInput
}

func (handler MemoryLightDreamHandler) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if err := ValidateExecutionGrant(grant, command); err != nil {
		return HandlerResult{}, err
	}
	if command.JobID != MemoryLightDreamJobID || command.WorkspaceID == "" || command.Trigger != TriggerPresence || handler.Engine == nil {
		return HandlerResult{}, errors.New("memory light dream command is outside its bounded L1 boundary")
	}
	engine := *handler.Engine
	day := command.RequestedAt
	engine.ExpectedDailyFingerprint = handler.ExpectedSourceFingerprint
	if handler.ProductSelection {
		if handler.SelectedInput == nil {
			return HandlerResult{State: ReceiptReviewedNoChange, ReasonCode: ReasonReviewedNoChange}, nil
		}
		var err error
		day, err = time.Parse("2006-01-02", handler.SelectedInput.Period)
		if err != nil {
			return HandlerResult{}, err
		}
		engine.ExpectedDailyFingerprint = handler.SelectedInput.Fingerprint
	}
	engine.PublicationGuard = func(publish func() error) error { return publishWithGrant(ctx, grant, command, publish) }
	result, err := engine.DreamDailyAttested(ctx, command.WorkspaceID, day)
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
