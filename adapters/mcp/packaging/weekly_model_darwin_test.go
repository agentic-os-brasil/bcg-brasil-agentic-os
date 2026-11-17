//go:build darwin

package packaging

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type weeklyPipeFixturePIDs struct {
	Leader int `json:"leader"`
	Child  int `json:"child"`
}

// This is a bounded synthetic same-group topology, not a vendor/native probe.
func TestWeeklyCLILeaderPipeFixture(t *testing.T) {
	switch os.Getenv("MAESTRO_WEEKLY_PIPE_FIXTURE") {
	case "child":
		time.Sleep(2 * time.Second)
		os.Exit(0)
	case "leader":
		child := exec.Command(os.Args[0], "-test.run=^TestWeeklyCLILeaderPipeFixture$")
		child.Env = []string{"MAESTRO_WEEKLY_PIPE_FIXTURE=child"}
		child.Stdout = os.Stdout
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			os.Exit(2)
		}
		body, _ := json.Marshal(weeklyPipeFixturePIDs{Leader: os.Getpid(), Child: child.Process.Pid})
		if err := os.WriteFile(os.Getenv("MAESTRO_WEEKLY_PIPE_STATE"), body, 0600); err != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
}

func TestWeeklyCLILeaderExitRetainsOwnershipUntilGroupCleanup(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(t.TempDir(), "synthetic-owned-pids.json")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	type outcome struct {
		output []byte
		err    error
	}
	done := make(chan outcome, 1)
	started := time.Now()
	go func() {
		output, err := runWeeklyModelProcess(ctx, weeklyModelProcess{Path: executable, Args: []string{"-test.run=^TestWeeklyCLILeaderPipeFixture$"}, Directory: filepath.Dir(statePath), Environment: []string{"MAESTRO_WEEKLY_PIPE_FIXTURE=leader", "MAESTRO_WEEKLY_PIPE_STATE=" + statePath}, Input: []byte("synthetic-only"), MaxOutput: 1024, MaxStderr: 1024})
		done <- outcome{output, err}
	}()
	var pids weeklyPipeFixturePIDs
	for time.Since(started) < time.Second {
		body, readErr := os.ReadFile(statePath)
		if readErr == nil && json.Unmarshal(body, &pids) == nil {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if pids.Leader == 0 || pids.Child == 0 {
		cancel()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
		}
		t.Fatal("synthetic leader did not declare its owned topology")
	}
	// Start the 300ms cancellation interval only once the fixture has declared
	// its topology; Go test-binary startup is not the behavior under test.
	cancellation := time.AfterFunc(300*time.Millisecond, cancel)
	defer cancellation.Stop()
	topologyStarted := time.Now()
	// Root NOTE_EXIT must not require reaping. If the runner is still waiting for
	// its same-group retained pipe, the waitable root must remain owned. Give a
	// completed cleanup time to return, avoiding a reap-to-return sampling race.
	time.Sleep(20 * time.Millisecond)
	var result *outcome
	select {
	case value := <-done:
		result = &value
	default:
	}
	if result == nil && errors.Is(unix.Kill(pids.Leader, 0), unix.ESRCH) {
		time.Sleep(20 * time.Millisecond)
		select {
		case value := <-done:
			result = &value
		default:
			t.Error("leader was reaped while same-group pipe child and cleanup were still pending")
		}
	}
	if result == nil {
		select {
		case value := <-done:
			result = &value
		case <-time.After(3 * time.Second):
			t.Fatal("owned synthetic group/pipe did not finish within cleanup bound")
		}
	}
	if time.Since(topologyStarted) > time.Second {
		t.Error("same-group retained pipe exceeded deadline/cleanup bound")
	}
	if result.err != nil && len(result.output) != 0 {
		t.Error("cancelled/failed synthetic transport returned usable output")
	}
}

func TestWeeklyCLIDarwinWatchAndPipeOwnership(t *testing.T) {
	watch, err := newWeeklyRootExitWatch()
	if err != nil {
		t.Fatal(err)
	}
	flags, err := unix.FcntlInt(uintptr(watch.fd), unix.F_GETFD, 0)
	if err != nil || flags&unix.FD_CLOEXEC == 0 {
		t.Fatal("root watch descriptor not close-on-exec", err)
	}
	if err := watch.register(0); err == nil {
		t.Fatal("invalid root registered")
	}
	watch.close()
	if err := watch.register(os.Getpid()); err == nil {
		t.Fatal("closed-queue registration failure accepted")
	}
	for _, event := range []unix.Kevent_t{{Ident: 1, Filter: unix.EVFILT_PROC, Fflags: unix.NOTE_EXIT}, {Ident: 123, Filter: unix.EVFILT_READ, Fflags: unix.NOTE_EXIT}, {Ident: 123, Filter: unix.EVFILT_PROC, Fflags: unix.NOTE_FORK}, {Ident: 123, Filter: unix.EVFILT_PROC, Fflags: unix.NOTE_EXIT, Flags: unix.EV_ERROR}} {
		w := &weeklyRootExitWatch{pid: 123}
		if w.observe(event) == nil || w.exited {
			t.Fatal("unbound/error/non-exit event accepted")
		}
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	connection, err := read.SyscallConn()
	if err != nil {
		t.Fatal(err)
	}
	var before, after int
	var descriptorErr error
	if err := connection.Control(func(fd uintptr) { before, descriptorErr = unix.FcntlInt(fd, unix.F_GETFL, 0) }); err != nil {
		t.Fatal(err)
	}
	if err := weeklyDarwinPipeCloseOnExec(read); err != nil {
		t.Fatal(err)
	}
	if err := connection.Control(func(fd uintptr) { after, descriptorErr = unix.FcntlInt(fd, unix.F_GETFL, 0) }); err != nil {
		t.Fatal(err)
	}
	if descriptorErr != nil {
		t.Fatal(descriptorErr)
	}
	if before&unix.O_NONBLOCK != after&unix.O_NONBLOCK {
		t.Fatal("CLOEXEC check changed the owned pipe's nonblocking state")
	}
}

func TestWeeklyCLIDarwinStalledInputCancelsAndJoins(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	started := time.Now()
	out, err := runWeeklyModelProcess(ctx, weeklyModelProcess{Path: executable, Args: []string{"-test.run=^TestWeeklyCLIProcessHelper$"}, Directory: t.TempDir(), Environment: []string{"MAESTRO_WEEKLY_SYNTHETIC=stall-input"}, Input: bytes.Repeat([]byte("x"), 32768), MaxOutput: 1024, MaxStderr: 1024})
	if err == nil || len(out) != 0 || time.Since(started) > time.Second {
		t.Fatal("stalled input did not cancel/join within normal bound", err)
	}
}

func TestWeeklyCLIDarwinCancellationDuringDrain(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	ready := filepath.Join(directory, "drain-ready")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	type drainOutcome struct {
		err         error
		outputBytes int
	}
	done := make(chan drainOutcome, 1)
	go func() {
		out, err := runWeeklyModelProcess(ctx, weeklyModelProcess{Path: executable, Args: []string{"-test.run=^TestWeeklyCLIProcessHelper$"}, Directory: directory, Environment: []string{"MAESTRO_WEEKLY_SYNTHETIC=drain", "MAESTRO_WEEKLY_DRAIN_READY=" + ready}, MaxOutput: 1024, MaxStderr: 1024})
		done <- drainOutcome{err: err, outputBytes: len(out)}
	}()
	setup := time.Now().Add(time.Second)
	observed := false
	for time.Now().Before(setup) {
		if _, err := os.Stat(ready); err == nil {
			observed = true
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	cancel()
	select {
	case outcome := <-done:
		if outcome.err == nil || outcome.outputBytes != 0 {
			t.Fatal("cancelled drain succeeded or exposed partial output", outcome.err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled drain retained owned readers")
	}
	if !observed {
		t.Fatal("synthetic drain did not reach its ready state")
	}
}

func weeklyDarwinZombieRecordFixture(rootPID int) *unix.KinfoProc {
	record := &unix.KinfoProc{}
	record.Proc.P_pid = int32(rootPID)
	record.Proc.P_stat = 5
	record.Eproc.Pgid = int32(rootPID)
	record.Eproc.Ppid = int32(os.Getpid())
	record.Eproc.Pcred.P_ruid = uint32(os.Getuid())
	record.Eproc.Ucred.Uid = uint32(os.Geteuid())
	return record
}

func TestWeeklyCLIDarwinZombieOnlyPredicateRejectsAmbiguity(t *testing.T) {
	root := 123
	valid := weeklyDarwinZombieRecordFixture(root)
	if !weeklyDarwinZombieRecordMatches(root, valid, nil) {
		t.Fatal("exact sole zombie refused")
	}
	if weeklyDarwinZombieRecordMatches(root, nil, nil) {
		t.Fatal("nil record accepted")
	}
	for _, err := range []error{unix.ENOMEM, unix.EACCES, unix.EIO, unix.EPERM} {
		if weeklyDarwinZombieRecordMatches(root, valid, err) {
			t.Fatal("record plus query error accepted", err)
		}
	}
	for _, mutate := range []func(*unix.KinfoProc){func(p *unix.KinfoProc) { p.Proc.P_pid++ }, func(p *unix.KinfoProc) { p.Eproc.Pgid++ }, func(p *unix.KinfoProc) { p.Eproc.Ppid++ }, func(p *unix.KinfoProc) { p.Eproc.Pcred.P_ruid++ }, func(p *unix.KinfoProc) { p.Eproc.Ucred.Uid++ }, func(p *unix.KinfoProc) { p.Proc.P_stat = 2 }} {
		copy := *valid
		mutate(&copy)
		if weeklyDarwinZombieRecordMatches(root, &copy, nil) {
			t.Fatal("foreign/live owned-group record accepted")
		}
	}
	if weeklyDarwinZombieRecordMatches(0, valid, nil) {
		t.Fatal("unbound root accepted")
	}
}

func TestWeeklyCLIDarwinLiveGroupRejectsInjectedCleanupEPERM(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(t.TempDir(), "actual-group.json")
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	cmd := exec.Command(executable, "-test.run=^TestWeeklyCLILeaderPipeFixture$")
	cmd.Env = []string{"MAESTRO_WEEKLY_PIPE_FIXTURE=leader", "MAESTRO_WEEKLY_PIPE_STATE=" + state}
	cmd.Stdin = null
	cmd.Stdout = null
	cmd.Stderr = null
	configureNativeProcess(cmd)
	cancelGroup := cmd.Cancel
	cmd.Cancel = nil
	watch, err := newWeeklyRootExitWatch()
	if err != nil {
		t.Fatal(err)
	}
	defer watch.close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cancelGroup(); _ = cmd.Wait() }() // sole Wait, after cleanup
	if err := watch.register(cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Second)
	for !watch.exited && time.Now().Before(until) {
		if _, err := watch.poll(10 * time.Millisecond); err != nil {
			t.Fatal(err)
		}
	}
	if !watch.exited {
		t.Fatal("synthetic leader exit notification missing")
	}
	var pids weeklyPipeFixturePIDs
	body, err := os.ReadFile(state)
	if err != nil || json.Unmarshal(body, &pids) != nil {
		t.Fatal("synthetic owned topology missing", err)
	}
	if err := unix.Kill(pids.Child, 0); err != nil {
		t.Fatal("same-group negative fixture is not live", err)
	}
	record, readErr := unix.SysctlKinfoProc("kern.proc.pgrp", cmd.Process.Pid)
	if !errors.Is(readErr, unix.ENOMEM) {
		t.Fatalf("extra actual group record was not refused: record=%v error=%v", record != nil, readErr)
	}
	if err := weeklyDarwinGroupCleanupResult(unix.EPERM, cmd.Process.Pid, func(pid int) (*unix.KinfoProc, error) { return unix.SysctlKinfoProc("kern.proc.pgrp", pid) }); err == nil {
		t.Fatal("injected cleanup EPERM accepted live extra group member")
	}
	if weeklyDarwinZombieRecordMatches(cmd.Process.Pid, weeklyDarwinZombieRecordFixture(cmd.Process.Pid), readErr) {
		t.Fatal("truncated first zombie record accepted with ENOMEM")
	}
}

func TestWeeklyCLIDarwinActualZombieEPERMCleanup(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	cmd := exec.Command(executable, "-test.run=^TestWeeklyCLIProcessHelper$")
	cmd.Env = []string{"MAESTRO_WEEKLY_SYNTHETIC=echo"}
	cmd.Stdin = null
	cmd.Stdout = null
	cmd.Stderr = null
	configureNativeProcess(cmd)
	cancelGroup := cmd.Cancel
	cmd.Cancel = nil
	watch, err := newWeeklyRootExitWatch()
	if err != nil {
		t.Fatal(err)
	}
	defer watch.close()
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	attempted := false
	defer func() {
		if !attempted {
			_ = cancelGroup()
		}
		if err := cmd.Wait(); err != nil {
			t.Error("synthetic zombie did not reap with real successful status", err)
		}
	}()
	if err := watch.register(cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	until := time.Now().Add(time.Second)
	var record *unix.KinfoProc
	for time.Now().Before(until) {
		if _, err := watch.poll(10 * time.Millisecond); err != nil {
			t.Fatal(err)
		}
		record, err = unix.SysctlKinfoProc("kern.proc.pid", cmd.Process.Pid)
		if err == nil && record.Proc.P_stat == 5 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !watch.exited || !weeklyDarwinZombieRecordMatches(cmd.Process.Pid, record, err) {
		t.Fatal("actual owned root did not reach an observed zombie state", err)
	}
	signalErr := cancelGroup()
	attempted = true
	if !errors.Is(signalErr, unix.EPERM) {
		t.Fatalf("expected actual POSIX zombie-only EPERM, got %v", signalErr)
	}
	// Actual signal EPERM must not be cleared by an unrelated query ESRCH,
	// even when the injected failed reader also supplies matching zombie bytes.
	for _, observed := range []*unix.KinfoProc{nil, record} {
		calls := 0
		rejected := weeklyDarwinGroupCleanupResult(signalErr, cmd.Process.Pid, func(pid int) (*unix.KinfoProc, error) {
			calls++
			if pid != cmd.Process.Pid {
				t.Fatal("query escaped retained group")
			}
			return observed, unix.ESRCH
		})
		if rejected == nil || !errors.Is(rejected, unix.EPERM) || !errors.Is(rejected, unix.ESRCH) || calls != 1 {
			t.Fatal("actual EPERM plus reader ESRCH was cleared at cleanup boundary", rejected)
		}
	}
	if err := weeklyDarwinGroupCleanupResult(signalErr, cmd.Process.Pid, func(pid int) (*unix.KinfoProc, error) { return unix.SysctlKinfoProc("kern.proc.pgrp", pid) }); err != nil {
		t.Fatal("exact sole retained zombie did not classify", err)
	}
	t.Log("actual pre-reap group EPERM accepted only after complete exact sole-zombie PGRP observation; sole Wait follows")
}

func TestWeeklyCLIDarwinDescriptorClosureOnStartFailureAndSuccess(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	// Initialize Go's process-wide pipe poller separately from owned transport
	// descriptors. Read names only: Darwin's /dev/fd entries are not stat-stable.
	warmRead, warmWrite, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	_ = warmRead.Close()
	_ = warmWrite.Close()
	fdNames := func() []string {
		file, err := os.Open("/dev/fd")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		names, err := file.Readdirnames(-1)
		if err != nil {
			t.Fatal(err)
		}
		return names
	}
	before := fdNames()
	for _, path := range []string{filepath.Join(directory, "missing-synthetic-executable"), executable} {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		output, runErr := runWeeklyModelProcess(ctx, weeklyModelProcess{Path: path, Args: []string{"-test.run=^TestWeeklyCLIProcessHelper$"}, Directory: directory, Environment: []string{"MAESTRO_WEEKLY_SYNTHETIC=echo"}, Input: []byte("synthetic-fd-closure"), MaxOutput: 1024, MaxStderr: 1024})
		cancel()
		if path == executable {
			if runErr != nil || string(output) != "synthetic-fd-closure" {
				t.Fatal("synthetic successful closure failed", runErr)
			}
		} else if runErr == nil {
			t.Fatal("missing child unexpectedly started")
		}
	}
	after := fdNames()
	if len(after) > len(before) {
		t.Fatalf("owned transport descriptors retained after return: before=%d after=%d", len(before), len(after))
	}
}

func TestWeeklyCLIDarwinRawSignalESRCHDoesNotQuery(t *testing.T) {
	for _, signalErr := range []error{nil, unix.ESRCH} {
		calls := 0
		err := weeklyDarwinGroupCleanupResult(signalErr, 123, func(int) (*unix.KinfoProc, error) { calls++; return nil, unix.EACCES })
		if err != nil || calls != 0 {
			t.Fatal("raw signal empty-group semantics changed or queried without EPERM", err, calls)
		}
	}
}
