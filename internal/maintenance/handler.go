package maintenance

import "context"

// Handler is the runtime-neutral worker seam. Scheduling and authority decide
// whether a command may run; a handler only executes the already-authorized
// bounded occurrence and returns a metadata-only receipt.
type Handler interface {
	Handle(context.Context, Command) (Receipt, error)
}
