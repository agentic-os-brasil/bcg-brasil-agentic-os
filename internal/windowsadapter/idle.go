package windowsadapter

import (
	"errors"
	"time"
)

var ErrIdleUnavailable = errors.New("native Windows idle observation unavailable")

func idleDuration(last uint32, ticks uint64) (time.Duration, error) {
	if last == 0 || ticks == 0 || ticks < 1<<32 && uint64(last) > ticks {
		return 0, ErrIdleUnavailable
	}
	delta := uint32(ticks) - last
	// The half-range rejects future/ambiguous DWORD counters without mistaking wrap for idle.
	if delta > 1<<31-1 {
		return 0, ErrIdleUnavailable
	}
	return time.Duration(delta) * time.Millisecond, nil
}
