//go:build !darwin

package processidentity

import (
	"os"
	"time"
)

// Other systems cannot reinterpret a reported PID as Darwin Start custody.
func ObserveWeeklyNativeChild(*os.Process, int, time.Time) (WeeklyNativeChild, error) {
	return WeeklyNativeChild{}, ErrUnavailable
}
func VerifyWeeklyNativeTerminal(*os.Process, WeeklyNativeChild) error { return ErrUnavailable }
