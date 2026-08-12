package userlevel

import (
	"errors"
	"testing"
)

func TestEnsureNotElevatedDelegatesToPlatformBoundary(t *testing.T) {
	want := errors.New("elevated")
	original := ensurePlatformUserLevel
	ensurePlatformUserLevel = func() error { return want }
	t.Cleanup(func() { ensurePlatformUserLevel = original })

	if err := EnsureNotElevated(); !errors.Is(err, want) {
		t.Fatalf("EnsureNotElevated() error = %v, want %v", err, want)
	}
}
