//go:build !windows

package processwait

import (
	"context"
	"errors"
	"os"
	"syscall"
	"time"
)

func wait(ctx context.Context, pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		err := process.Signal(syscall.Signal(0))
		if errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
