package winprivate

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestDiagnosticErrorPreservesNativeCauseAndReason(t *testing.T) {
	native := &os.PathError{Op: "inspect", Path: "private fixture", Err: os.ErrPermission}
	err := fmt.Errorf("home admission: %w", withReason(ErrInspectionFailed, native))
	var got *os.PathError
	var classified *DiagnosticError
	if !errors.Is(err, os.ErrPermission) || !errors.Is(err, ErrInspectionFailed) || !errors.As(err, &got) || got != native || !errors.As(err, &classified) {
		t.Fatal("wrapped diagnostic lost the native cause or finite reason")
	}
	if DiagnosticReason(err) != "inspection_failed" {
		t.Fatal("wrapped native error did not retain its finite category")
	}
}
