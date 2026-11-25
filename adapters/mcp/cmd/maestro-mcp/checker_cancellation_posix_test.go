//go:build !windows

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
