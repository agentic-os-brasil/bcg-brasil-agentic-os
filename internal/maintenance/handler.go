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

// handlerExecutor admits only the grant-aware worker seam. Validation happens
// here as well as in concrete handlers so a handler cannot execute if the
// worker ever passes a missing or command-mismatched grant.
func handlerExecutor(handler any) (handlerExecuteFunc, bool) {
	typed, ok := handler.(Handler)
	if !ok {
		return nil, false
	}
	return func(ctx context.Context, command Command, grant ExecutionGrant) (HandlerResult, error) {
		if err := ValidateExecutionGrant(grant, command); err != nil {
			return HandlerResult{}, err
		}
		return typed.ExecuteAuthorized(ctx, command, grant)
	}, true
}

// Handler is the only runtime-neutral worker seam. Scheduling and authority
// decide whether a command may run; a handler must receive and honor the
// worker-issued grant before any side effect.
type Handler interface {
	ExecuteAuthorized(context.Context, Command, ExecutionGrant) (HandlerResult, error)
}
