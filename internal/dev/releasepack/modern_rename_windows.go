//go:build windows

package releasepack

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

// Newly executed images may briefly retain Windows sharing locks after their
// identity process exits. Retry only those native errors, never replace a
// destination, and retain the complete staged candidate when publication fails.
func renameModernCandidate(ctx context.Context, stage, output string) error {
	source, err := windows.UTF16PtrFromString(stage)
	if err != nil {
		return err
	}
	target, err := windows.UTF16PtrFromString(output)
	if err != nil {
		return err
	}
	deadline, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	for {
		if err := deadline.Err(); err != nil {
			return err
		}
		if _, err := os.Lstat(output); err == nil {
			return fmt.Errorf("modern output already exists: %w", os.ErrExist)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		// No MOVEFILE_REPLACE_EXISTING: a concurrent winner stays untouched.
		err = windows.MoveFileEx(source, target, 0)
		if err == nil || (!errors.Is(err, windows.ERROR_ACCESS_DENIED) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION)) {
			return err
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-deadline.Done():
			timer.Stop()
			return errors.Join(err, deadline.Err())
		case <-timer.C:
		}
	}
}
