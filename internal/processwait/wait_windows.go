//go:build windows

package processwait

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

func wait(ctx context.Context, pid int) error {
	if pid <= 0 || uint64(pid) > uint64(^uint32(0)) {
		return errors.New("process PID exceeds the native DWORD range")
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		if missingProcessLookup(err) {
			return nil
		}
		return err
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

func missingProcessLookup(err error) bool {
	var lookup *os.SyscallError
	return errors.As(err, &lookup) && lookup.Syscall == "OpenProcess" && errors.Is(lookup.Err, windows.ERROR_INVALID_PARAMETER)
}
