package maintenance

import "context"

type handlerExecuteFunc func(context.Context, Command) (HandlerResult, error)

// handlerExecutor bridges the bounded Darwin seam with the canonical Handle
// seam used by Walter/self. The worker is the only place that normalizes the
// two metadata-only result shapes.
func handlerExecutor(handler any) (handlerExecuteFunc, bool) {
	switch typed := handler.(type) {
	case interface {
		Execute(context.Context, Command) (HandlerResult, error)
	}:
		return typed.Execute, true
	case Handler:
		return func(ctx context.Context, command Command) (HandlerResult, error) {
			receipt, err := typed.Handle(ctx, command)
			return HandlerResult{
				State:              receipt.State,
				ProposalCount:      receipt.ProposalCount,
				ProposalDigest:     receipt.ProposalDigest,
				ProposalArtifactID: receipt.ProposalArtifactID,
				ReasonCode:         receipt.ReasonCode,
			}, err
		}, true
	default:
		return nil, false
	}
}

// Handler is the runtime-neutral worker seam. Scheduling and authority decide
// whether a command may run; a handler only executes the already-authorized
// bounded occurrence and returns a metadata-only receipt.
type Handler interface {
	Handle(context.Context, Command) (Receipt, error)
}
