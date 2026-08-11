package cli

import "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/userlevel"

// Kept injectable so the setup/init contract can be tested without requiring
// an elevated Windows test runner.
var ensureUserLevelProcess = userlevel.EnsureNotElevated
