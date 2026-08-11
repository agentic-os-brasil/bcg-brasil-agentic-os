package agentorchestration

import "errors"

// ErrDurableStateOwnerMismatch identifies a Windows state file created under
// a different security principal, commonly an elevated Administrator token.
// Callers can provide a safe repair message without weakening the ownership
// boundary or attempting an implicit take-ownership operation.
var ErrDurableStateOwnerMismatch = errors.New("durable orchestration state is not owned by the current Windows user")
