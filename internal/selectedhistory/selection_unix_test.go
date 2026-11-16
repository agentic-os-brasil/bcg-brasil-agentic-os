//go:build !windows

package selectedhistory

import (
	"golang.org/x/sys/unix"
	"os"
	"testing"
	"time"
)

func TestSelectedHistoryFIFOSelectionFailsWithoutWaiting(t *testing.T) {
	manifest, path := selectionFixture(t)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := unix.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if _, err := Prepare(manifest); err == nil {
		t.Fatal("FIFO admitted")
	}
	if time.Since(started) > time.Second {
		t.Fatal("FIFO reader waited for writer")
	}
}
