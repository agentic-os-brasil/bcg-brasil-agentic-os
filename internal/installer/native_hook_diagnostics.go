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

// The wrapper retains the original error text and errors.Is/As chain. With
// diagnostics off even its error identity is unchanged; it grants no recovery.
type nativeHookAdmissionError struct {
	cause string
	err   error
}

func (e *nativeHookAdmissionError) Error() string { return e.err.Error() }
func (e *nativeHookAdmissionError) Unwrap() error { return e.err }

func closedNativeHookAdmissionCause(cause string) string {
	switch cause {
	case "selection", "seed_admission", "activation_busy", "current_source",
		"client_binding", "native_selection", "launch_binding", "binding_inspection",
		"record_recheck", "current_recheck", "role_recheck":
		return cause
	}
	return ""
}

func withNativeHookAdmissionCause(cause string, err error) error {
	if err == nil || os.Getenv("MAESTRO_HOOK_DIAGNOSTICS") != "1" || closedNativeHookAdmissionCause(cause) == "" {
		return err
	}
	var existing *nativeHookAdmissionError
	if errors.As(err, &existing) {
		return err
	}
	return &nativeHookAdmissionError{cause: cause, err: err}
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
		cause := ""
		var admission *nativeHookAdmissionError
		if errors.As(err, &admission) {
			if closed := closedNativeHookAdmissionCause(admission.cause); closed != "" {
				cause = " cause=" + closed
			}
		}
		_, _ = fmt.Fprintf(output, "maestro-hook phase=%s state=end elapsed_ms=%d outcome=%s%s\n", phase, now().Sub(started).Milliseconds(), outcome, cause)
	}
}
