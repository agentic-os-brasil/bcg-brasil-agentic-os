package processwait

import (
	"os"
	"strconv"
	"testing"
	"time"
)

func TestUntilExitRefusesLiveOrInvalidOwner(t *testing.T) {
	pids := []int{0, -1, os.Getpid()}
	if strconv.IntSize == 64 {
		overflow := uint64(1) << 32
		pids = append(pids, int(overflow))
	}
	for _, pid := range pids {
		if err := UntilExit(pid, 10*time.Millisecond); err == nil {
			t.Fatal("live or invalid process classified as exited", pid)
		}
	}
}
