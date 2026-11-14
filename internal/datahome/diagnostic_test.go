package datahome_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/winprivate"
)

func TestDiagnosticReasonPreservesFiniteWrappedReasons(t *testing.T) {
	for _, tc := range []struct {
		cause error
		want  string
	}{
		{nil, ""},
		{winprivate.ErrOwnerMismatch, "owner_mismatch"},
		{winprivate.ErrUnsafeACL, "unsafe_acl"},
		{winprivate.ErrUnsafeAttributes, "unsafe_attributes"},
		{winprivate.ErrUnsupportedFilesystem, "unsupported_filesystem"},
		{winprivate.ErrInspectionFailed, "inspection_failed"},
		{winprivate.ErrBackendUnavailable, "backend_unavailable"},
		{errors.New("owner_mismatch"), "unknown"},
	} {
		err := tc.cause
		if err != nil {
			err = fmt.Errorf("outer admission: %w", err)
		}
		if got := datahome.DiagnosticReason(err); got != tc.want {
			t.Errorf("reason=%q, want %q", got, tc.want)
		}
	}
}
