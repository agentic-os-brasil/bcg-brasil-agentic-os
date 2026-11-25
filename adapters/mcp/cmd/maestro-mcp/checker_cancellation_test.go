package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestInstalledCheckerDirectStalledChildCancellationReapsProcess(t *testing.T) {
	ready := filepath.Join(t.TempDir(), "stalled-child.ready")
	body, _ := json.Marshal(installedPilotChildConfig{Mutation: "hang", ReadyMarker: ready})
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInstalledPilotChildFixture$")
	child.Env = append(os.Environ(), "MAESTRO_INSTALLED_CHECK_TEST_CHILD="+string(body))
	done := make(chan error, 1)
	go func() {
		session, err := installedPilotConnect(ctx, child)
		if session != nil {
			_ = session.Close()
		}
		done <- err
	}()
	finished := false
	t.Cleanup(func() {
		cancel()
		if !finished {
			select {
			case <-done:
			case <-time.After(3 * time.Second):
				t.Error("fixture connection did not stop during cleanup")
			}
		}
	})
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		marker, err := os.ReadFile(ready)
		if err == nil {
			if string(marker) == installedPilotChildReady {
				break
			}
			if len(marker) > 0 {
				t.Fatalf("unexpected readiness marker: %q", marker)
			}
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			finished = true
			t.Fatalf("child connection ended before confirmed startup: %v", err)
		case <-ctx.Done():
			t.Fatal("child never confirmed startup", ctx.Err())
		case <-ticker.C:
		}
	}
	cancel()
	select {
	case err := <-done:
		finished = true
		if err == nil || ctx.Err() != context.Canceled {
			t.Fatalf("started stalled child ignored cancellation: %v context=%v", err, ctx.Err())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("started stalled child did not stop within cancellation bound")
	}
	// The result channel synchronizes command shutdown before observing Wait state.
	if child.ProcessState == nil {
		t.Fatal("SDK returned without reaping its direct stalled child")
	}
}

func TestEarlyDispatchOtherCommandsRetainOriginalContext(t *testing.T) {
	handled, err := dispatchEarlyCommand("installed-native-hook", []string{"unchanged"}, io.Discard, func(ctx context.Context, command string, args []string, _ io.Writer) (bool, error) {
		if ctx.Done() != nil || command != "installed-native-hook" || len(args) != 1 || args[0] != "unchanged" {
			t.Fatal("unrelated early command context or arguments changed")
		}
		return true, nil
	})
	if !handled || err != nil {
		t.Fatal("unrelated early dispatch changed", handled, err)
	}
}
