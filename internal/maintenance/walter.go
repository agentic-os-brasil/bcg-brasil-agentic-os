package maintenance

import (
	"context"
	"errors"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// WalterWeeklyProposalHandler is the integration seam owned by the runtime-
// neutral Walter/self work. Darwin only dispatches through this typed seam;
// it never reimplements intent or self logic.
type WalterWeeklyProposalHandler interface {
	ProposeWeekly(context.Context, Command) (HandlerResult, error)
}

type WalterWeeklyAdapter struct {
	Handler WalterWeeklyProposalHandler
}

func (adapter WalterWeeklyAdapter) Execute(ctx context.Context, command Command) (HandlerResult, error) {
	if adapter.Handler == nil {
		return HandlerResult{}, scheduler.ErrCapabilityUnavailable
	}
	result, err := adapter.Handler.ProposeWeekly(ctx, command)
	if err != nil {
		return result, err
	}
	if result.State != ReceiptProposalEmitted || result.ProposalCount < 1 || result.ProposalDigest == "" || result.ProposalArtifactID == "" {
		return HandlerResult{}, errors.New("Walter weekly handler must return a non-empty proposal receipt")
	}
	return result, nil
}
