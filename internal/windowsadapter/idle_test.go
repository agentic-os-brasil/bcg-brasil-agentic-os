package windowsadapter

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"
)

func TestNativeIdleCounterDuration(t *testing.T) {
	for _, tc := range []struct {
		last    uint32
		ticks   uint64
		want    time.Duration
		invalid bool
	}{
		{last: 100, ticks: 1000, want: 900 * time.Millisecond},
		{last: 1<<32 - 50, ticks: 1<<32 + 50, want: 100 * time.Millisecond},
		{last: 300000, ticks: 3*(1<<32) + 600000, want: 5 * time.Minute},
		{last: 0, ticks: 1000, invalid: true},
		{last: 1001, ticks: 1000, invalid: true},
		{last: 1, ticks: 1<<31 + 1, invalid: true},
	} {
		got, err := idleDuration(tc.last, tc.ticks)
		if (err != nil) != tc.invalid || !tc.invalid && got != tc.want {
			t.Fatalf("idle counter last=%d ticks=%d duration=%s error=%v", tc.last, tc.ticks, got, err)
		}
	}
	if runtime.GOOS != "windows" {
		if _, err := ObserveIdle(context.Background()); !errors.Is(err, ErrIdleUnavailable) {
			t.Fatal("unsupported native platform fabricated an idle observation")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ObserveIdle(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled native observation did not stop")
	}
}
