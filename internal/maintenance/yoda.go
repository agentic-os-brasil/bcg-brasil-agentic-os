package maintenance

import (
	"context"
	"errors"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// YodaWeeklyProposalHandler is the integration seam owned by the runtime-
// neutral Yoda/self work. Darwin only dispatches through this typed seam;
// it never reimplements intent or self logic.
type YodaWeeklyProposalHandler interface {
	ProposeWeekly(context.Context, Command, ExecutionGrant) (HandlerResult, error)
}

const YodaWeeklyJobID = YodaSelfReviewWeeklyJobID

type YodaWeeklyAdapter struct {
	Handler YodaWeeklyProposalHandler
}

func (adapter YodaWeeklyAdapter) ExecuteAuthorized(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
	if err := ValidateExecutionGrant(grant, command); err != nil {
		return HandlerResult{}, err
	}
	if adapter.Handler == nil {
		return HandlerResult{}, scheduler.ErrCapabilityUnavailable
	}
	result, err := adapter.Handler.ProposeWeekly(ctx, command, grant)
	if err != nil {
		return result, err
	}
	if result.State != ReceiptProposalEmitted || result.ProposalCount < 1 || result.ProposalDigest == "" || result.ProposalArtifactID == "" {
		return HandlerResult{}, errors.New("Yoda weekly handler must return a non-empty proposal receipt")
	}
	return result, nil
}
