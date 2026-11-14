package winprivate

import "errors"

// These identities classify native refusals without returning paths, SIDs or
// security descriptors to a diagnostic report. Unknown errors remain unknown.
var (
	ErrOwnerMismatch         = errors.New("owner_mismatch")
	ErrUnsafeACL             = errors.New("unsafe_acl")
	ErrUnsafeAttributes      = errors.New("unsafe_attributes")
	ErrUnsupportedFilesystem = errors.New("unsupported_filesystem")
	ErrInspectionFailed      = errors.New("inspection_failed")
	ErrBackendUnavailable    = errors.New("backend_unavailable")
)

// DiagnosticError retains a native cause and a finite policy reason. Its text
// preserves the existing internal error; share only DiagnosticReason's result.
type DiagnosticError struct {
	reason error
	cause  error
}

func (e *DiagnosticError) Error() string        { return e.cause.Error() }
func (e *DiagnosticError) Unwrap() error        { return e.cause }
func (e *DiagnosticError) Is(target error) bool { return e.reason == target }

func withReason(reason, cause error) error {
	return &DiagnosticError{reason: reason, cause: cause}
}

// DiagnosticReason returns only a known category, "unknown" for unclassified
// errors, or an empty string when no failure occurred. It never parses text.
func DiagnosticReason(err error) string {
	if err == nil {
		return ""
	}
	for _, reason := range []error{ErrOwnerMismatch, ErrUnsafeACL, ErrUnsafeAttributes, ErrUnsupportedFilesystem, ErrInspectionFailed, ErrBackendUnavailable} {
		if errors.Is(err, reason) {
			return reason.Error()
		}
	}
	return "unknown"
}
