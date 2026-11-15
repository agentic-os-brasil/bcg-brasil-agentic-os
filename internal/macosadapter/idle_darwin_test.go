//go:build darwin

package macosadapter

import (
	"context"
	"testing"
	"time"
)

func TestObserveIdleReadsNativeHIDCounter(t *testing.T) {
	started := time.Now()
	state, idleFor, err := ObserveIdle(context.Background(), 5*time.Minute)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatalf("shipping native idle probe failed after %s: %v", elapsed, err)
	}
	if idleFor < 0 || state != NativeIdleActive && state != NativeIdleConfirmed {
		t.Fatalf("native idle observation has invalid state %q", state)
	}
	if elapsed >= idleProbeTimeout {
		t.Fatalf("native idle probe exceeded its %s bound: %s", idleProbeTimeout, elapsed)
	}
	t.Logf("shipping native idle probe completed in %s", elapsed)
}
