package macosadapter

import (
	"testing"
	"time"
)

func TestParseHIDIdleTime(t *testing.T) {
	output := `"HIDIdleTime" = 420000000000`
	duration, err := ParseHIDIdleTime(output)
	if err != nil {
		t.Fatal(err)
	}
	if duration != 7*time.Minute {
		t.Fatalf("idle duration = %s, want 7m", duration)
	}
}

func TestParseHIDIdleTimeFailsClosed(t *testing.T) {
	for _, output := range []string{`{"Other" = 42}`, `{"HIDIdleTime" = nope}`, `{"HIDIdleTime" = 1, "HIDIdleTime" = 2}`} {
		if _, err := ParseHIDIdleTime(output); err == nil {
			t.Fatalf("unsafe idle output was accepted: %q", output)
		}
	}
}

func TestClassifyIdleRequiresThreshold(t *testing.T) {
	if state, err := ClassifyIdle(5*time.Minute, 5*time.Minute); err != nil || state != NativeIdleConfirmed {
		t.Fatalf("state=%q err=%v, want idle", state, err)
	}
	if state, err := ClassifyIdle(5*time.Minute-time.Nanosecond, 5*time.Minute); err != nil || state != NativeIdleActive {
		t.Fatalf("state=%q err=%v, want active", state, err)
	}
	if _, err := ClassifyIdle(time.Minute, 0); err == nil {
		t.Fatal("zero threshold was accepted")
	}
}
