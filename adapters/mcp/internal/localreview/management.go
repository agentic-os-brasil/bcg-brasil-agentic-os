package localreview

import "context"

// OpenManagementLauncher reuses the normal platform handoff owner. Its caller
// supplies only the exact independently admitted participant manager path.
// Successful OS acknowledgement is a handoff request, never installation proof.
func OpenManagementLauncher(ctx context.Context, path string) error { return openBrowser(ctx, path) }
