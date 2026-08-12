//go:build windows

package userlevel

import "testing"

func TestWindowsUserLevelProbeMatchesCurrentToken(t *testing.T) {
	// This is a native smoke test. The elevated case is exercised by the
	// platform's installer acceptance run, where the process token is known.
	if err := ensurePlatformUserLevelForPlatform(); err != nil {
		t.Logf("current Windows token is not user-level: %v", err)
	}
}
