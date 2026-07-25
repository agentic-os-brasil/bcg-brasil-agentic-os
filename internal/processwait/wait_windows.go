//go:build windows

package processwait

import (
	"context"
	"os"
)

func wait(ctx context.Context, pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return nil
	}
	done := make(chan error, 1)
	go func() {
		_, waitErr := process.Wait()
		done <- waitErr
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case waitErr := <-done:
		return waitErr
	}
}
