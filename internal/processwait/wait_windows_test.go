//go:build windows

package processwait

import (
	"errors"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestWindowsProcessLookupErrorsRemainUnknownExceptAbsentPID(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		absent bool
	}{
		{"absent PID", os.NewSyscallError("OpenProcess", windows.ERROR_INVALID_PARAMETER), true},
		{"unattributed invalid parameter", windows.ERROR_INVALID_PARAMETER, false},
		{"different operation", os.NewSyscallError("WaitForSingleObject", windows.ERROR_INVALID_PARAMETER), false},
		{"access denied", os.NewSyscallError("OpenProcess", windows.ERROR_ACCESS_DENIED), false},
		{"invalid handle", os.NewSyscallError("OpenProcess", windows.ERROR_INVALID_HANDLE), false},
		{"unknown", errors.New("lookup unavailable"), false},
		{"no error", nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := missingProcessLookup(tc.err); got != tc.absent {
				t.Fatalf("lookup classified absent=%v, want %v", got, tc.absent)
			}
		})
	}
	pid := uint64(1) << 32
	if err := UntilExit(int(pid), time.Millisecond); err == nil {
		t.Fatal("overflowing PID was truncated into an absent native process")
	}
}
