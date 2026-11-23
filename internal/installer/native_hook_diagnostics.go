package installer

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"
)

// BeginNativeHookDiagnostic emits only closed phase/timing/outcome metadata to
// stderr when explicitly selected. It never changes admission or returns an
// error, reads event bodies, or accepts a log destination from the environment.
func BeginNativeHookDiagnostic(phase string) func(error) {
	return nativeHookDiagnostic(phase, os.Getenv("MAESTRO_HOOK_DIAGNOSTICS") == "1", os.Stderr, time.Now)
}

func nativeHookDiagnostic(phase string, enabled bool, output io.Writer, now func() time.Time) func(error) {
	switch phase {
	case "parent_admission", "parent_child", "child_bootstrap", "child_admission", "handler":
	default:
		enabled = false
	}
	if !enabled {
		return func(error) {}
	}
	started, done := now(), false
	_, _ = fmt.Fprintf(output, "maestro-hook phase=%s state=start\n", phase)
	return func(err error) {
		if done {
			return
		}
		done = true
		outcome := "ok"
		switch {
		case errors.Is(err, context.Canceled):
			outcome = "cancelled"
		case errors.Is(err, context.DeadlineExceeded):
			outcome = "deadline"
		case err != nil:
			outcome = "unavailable"
		}
		_, _ = fmt.Fprintf(output, "maestro-hook phase=%s state=end elapsed_ms=%d outcome=%s\n", phase, now().Sub(started).Milliseconds(), outcome)
	}
}
