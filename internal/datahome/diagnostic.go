package datahome

import "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"

// DiagnosticReason exposes a finite read-only refusal category. An unclassified
// error is unknown; it must not be interpreted as proof of safe ownership.
func DiagnosticReason(err error) string { return winprivate.DiagnosticReason(err) }
