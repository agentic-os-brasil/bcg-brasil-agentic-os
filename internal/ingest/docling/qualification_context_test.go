package docling

import (
	"context"
	"testing"
)

func TestQualificationStatusHonorsCallerCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if reason := qualificationStatus(ctx, Options{}, verifiedPack{}); reason != "conversion_cancelled" {
		t.Fatalf("cancelled qualification returned success: %q", reason)
	}
}
