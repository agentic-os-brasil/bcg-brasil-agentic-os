package maintenance

import (
	"context"
	"crypto/rand"
	"errors"
	"time"
)

type handlerExecuteFunc func(context.Context, Command, ExecutionGrant) (HandlerResult, error)

// ExecutionGrant is an unforgeable-in-practice handoff from Worker to a
// handler. Its private implementation prevents adapters from manufacturing a
// valid grant outside this package; handlers must validate it before any
// side-effecting work.
type ExecutionGrant interface {
	validate(Command) error
}

type executionGrant struct {
	commandID        string
	occurrenceDigest string
	workspaceID      string
	deadline         time.Time
	nonce            [16]byte
}

func newExecutionGrant(command Command) (ExecutionGrant, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	return executionGrant{
		commandID:        command.CommandID,
		occurrenceDigest: command.OccurrenceDigest(),
		workspaceID:      command.WorkspaceID,
		deadline:         command.Deadline.UTC(),
		nonce:            nonce,
	}, nil
}

func (grant executionGrant) validate(command Command) error {
	if grant.commandID == "" || grant.occurrenceDigest == "" || grant.workspaceID == "" || grant.nonce == ([16]byte{}) {
		return errors.New("maintenance execution grant is empty")
	}
	if grant.commandID != command.CommandID || grant.occurrenceDigest != command.OccurrenceDigest() || grant.workspaceID != command.WorkspaceID || !grant.deadline.Equal(command.Deadline.UTC()) {
		return errors.New("maintenance execution grant does not bind the command")
	}
	return nil
}

// ValidateExecutionGrant is the only validation entrypoint exposed to
// runtime-specific handlers. The grant implementation remains private to the
// maintenance package.
func ValidateExecutionGrant(grant ExecutionGrant, command Command) error {
	if grant == nil {
		return errors.New("maintenance execution grant is required")
	}
	return grant.validate(command)
}

// handlerExecutor bridges the bounded Darwin seam with the canonical Handle
// seam used by Walter/self. The worker is the only place that normalizes the
// two metadata-only result shapes.
func handlerExecutor(handler any) (handlerExecuteFunc, bool) {
	switch typed := handler.(type) {
	case interface {
		ExecuteAuthorized(context.Context, Command, ExecutionGrant) (HandlerResult, error)
	}:
		return typed.ExecuteAuthorized, true
	case interface {
		Execute(context.Context, Command) (HandlerResult, error)
	}:
		return func(ctx context.Context, command Command, _ ExecutionGrant) (HandlerResult, error) {
			return typed.Execute(ctx, command)
		}, true
	case Handler:
		return func(ctx context.Context, command Command, _ ExecutionGrant) (HandlerResult, error) {
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
