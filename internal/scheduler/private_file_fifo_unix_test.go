//go:build !windows

package scheduler

import (
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestReadPrivateFileRejectsFIFOWithoutWaitingForWriter(t *testing.T) {
	path := filepath.Join(privateTestDir(t), "locator.json")
	if err := unix.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := ReadPrivateFile(path, 16<<10)
		done <- err
	}()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("FIFO accepted as a regular private record")
		}
	case <-time.After(time.Second):
		// Release the blocked reader so the red test leaves no leaked goroutine.
		fd, err := unix.Open(path, unix.O_WRONLY|unix.O_NONBLOCK, 0)
		if err == nil {
			unix.Close(fd)
		}
		t.Fatal("private record read waited for a FIFO writer")
	}
}
