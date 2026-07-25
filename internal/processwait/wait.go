// Package processwait lets the stable bootstrapper wait for the CLI process
// that launched it to exit before replacing the active executable.
package processwait

import (
	"context"
	"errors"
	"fmt"
	"time"
)

func UntilExit(pid int, timeout time.Duration) error {
	if pid <= 0 {
		return errors.New("wait PID must be positive")
	}
	if timeout <= 0 {
		return errors.New("wait timeout must be positive")
	}
	contextWithTimeout, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := wait(contextWithTimeout, pid); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return fmt.Errorf("process %d did not exit within %s", pid, timeout)
		}
		return err
	}
	return nil
}
