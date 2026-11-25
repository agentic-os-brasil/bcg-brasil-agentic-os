package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// This fixture exercises the exact early-dispatch owner with an in-flight
// synthetic identity and the real report finalizer. It fabricates neither an
// installed role nor a completed execution-ledger operation.
func TestInstalledCheckEntrySignalFixture(t *testing.T) {
	if os.Getenv("MAESTRO_CHECKER_ENTRY_SIGNAL_FIXTURE") != "1" {
		return
	}
	handled, err := dispatchEarlyCommand("installed-pilot-check", nil, os.Stdout, func(ctx context.Context, _ string, _ []string, output io.Writer) (bool, error) {
		body, _ := json.Marshal(installedPilotChildConfig{Mutation: "hang"})
		owned := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInstalledPilotChildFixture$")
		owned.Env = append(os.Environ(), "MAESTRO_INSTALLED_CHECK_TEST_CHILD="+string(body))
		if err := owned.Start(); err != nil {
			return true, err
		}
		fmt.Fprintf(output, "ready %d\n", owned.Process.Pid)
		<-ctx.Done()
		_ = owned.Wait()
		if owned.ProcessState == nil {
			return true, fmt.Errorf("owned child not reaped")
		}
		r := installedPilotReport{pilotReport: pilotReport{SchemaVersion: 2, Status: "partial"}, CheckMode: "functional", FunctionalCheck: "incomplete", FunctionalStage: "execution_checkpoint", FunctionalReason: "cancelled", FunctionalItemID: "synthetic-known-item"}
		r.add("owned_child_shutdown", "passed", "direct_child_reaped", "")
		return true, finishInstalledPilotReport(&r, output)
	})
	if !handled || err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}

func TestInstalledCheckEntryInterruptFinalizesReportAndOwnedChild(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX process signals; native Windows remains separately qualified")
	}
	for _, sig := range []os.Signal{os.Interrupt, syscall.SIGTERM} {
		t.Run(sig.String(), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			child := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestInstalledCheckEntrySignalFixture$")
			child.Env = append(os.Environ(), "MAESTRO_CHECKER_ENTRY_SIGNAL_FIXTURE=1")
			var diagnostic bytes.Buffer
			child.Stderr = &diagnostic
			stdout, err := child.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err = child.Start(); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = child.Process.Kill(); _ = child.Wait() })
			reader := bufio.NewReader(stdout)
			ready, err := reader.ReadString('\n')
			if err != nil || !strings.HasPrefix(ready, "ready ") {
				t.Fatalf("entry did not reach owned work: %q %v %s", ready, err, diagnostic.String())
			}
			var ownedPID int
			if _, err = fmt.Sscanf(ready, "ready %d", &ownedPID); err != nil || ownedPID <= 0 {
				t.Fatal("missing owned child identity", ready, err)
			}
			ownedReaped := false
			t.Cleanup(func() {
				if ownedReaped {
					return
				}
				if owned, err := os.FindProcess(ownedPID); err == nil {
					_ = owned.Kill()
				}
			})
			if err = child.Process.Signal(sig); err != nil {
				t.Fatal(err)
			}
			output, readErr := io.ReadAll(reader)
			waitErr := child.Wait()
			var r installedPilotReport
			if readErr != nil || waitErr != nil || json.Unmarshal(output, &r) != nil {
				t.Fatalf("interrupt bypassed normal report/cleanup: wait=%v read=%v output=%q diagnostic=%s", waitErr, readErr, output, diagnostic.String())
			}
			ownedReaped = true
			if r.FunctionalItemID != "synthetic-known-item" || r.FunctionalCheck != "incomplete" || r.FunctionalReason != "cancelled" || !strings.Contains(r.Markdown, "synthetic-known-item") || len(r.Checks) != 1 || r.Checks[0].Reason != "direct_child_reaped" {
				t.Fatalf("cancelled report lost known work or owned shutdown: %+v", r)
			}
		})
	}
}

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
