//go:build !windows

package windowsadapter

import (
	"context"
	"time"
)

func ObserveIdle(ctx context.Context) (time.Duration, error) {
	if ctx.Err() != nil {
		return 0, ctx.Err()
	}
	return 0, ErrIdleUnavailable
}
