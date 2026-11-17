//go:build darwin

package processidentity

import (
	"bufio"
	"encoding/json"
	"fmt"
	"golang.org/x/sys/unix"
	"io"
	"os"
	"os/exec"
	"testing"
	"time"
)

func weeklyNativeDarwinFixture() *unix.KinfoProc {
	var r unix.KinfoProc
	r.Proc.P_pid = 123456
	r.Proc.P_stat = 3
	r.Eproc.Ppid = int32(os.Getpid())
	r.Eproc.Pcred.P_ruid = uint32(os.Getuid())
	r.Eproc.Ucred.Uid = uint32(os.Geteuid())
	at := time.Now().Add(-time.Second)
	r.Proc.P_starttime.Sec = at.Unix()
	r.Proc.P_starttime.Usec = int32(at.Nanosecond() / 1000)
	return &r
}
func TestWeeklyNativeDarwinRecordRejectsAmbiguity(t *testing.T) {
	good := weeklyNativeDarwinFixture()
	lower := time.Now().Add(-time.Minute)
	first, err := weeklyDarwinChildFacts(good, 123456, os.Getpid(), lower, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*unix.KinfoProc){func(r *unix.KinfoProc) { r.Proc.P_pid++ }, func(r *unix.KinfoProc) { r.Eproc.Ppid++ }, func(r *unix.KinfoProc) { r.Eproc.Pcred.P_ruid++ }, func(r *unix.KinfoProc) { r.Eproc.Ucred.Uid++ }, func(r *unix.KinfoProc) { r.Proc.P_starttime.Sec = 0 }, func(r *unix.KinfoProc) { r.Proc.P_starttime.Usec = 1000000 }, func(r *unix.KinfoProc) { r.Proc.P_stat = 4 }, func(r *unix.KinfoProc) { r.Proc.P_stat = 5 }, func(r *unix.KinfoProc) { r.Proc.P_flag |= 0x2000 }} {
		r := *good
		change(&r)
		if _, err := weeklyDarwinChildFacts(&r, 123456, os.Getpid(), lower, false); err == nil {
			t.Fatal("ambiguous/nonlive record admitted")
		}
	}
	if _, err := weeklyDarwinChildFacts(nil, 123456, os.Getpid(), lower, false); err == nil {
		t.Fatal("nil record admitted")
	}
	if _, err := weeklyDarwinChildFacts(good, 123456, os.Getpid(), lower, true); err == nil {
		t.Fatal("live root became terminal")
	}
	terminal := *good
	terminal.Proc.P_stat = 5
	last, err := weeklyDarwinChildFacts(&terminal, 123456, os.Getpid(), first.StartedAt(), true)
	if err != nil || !first.Same(last) {
		t.Fatal("same identity was not preserved for terminal comparison", err)
	}
	body, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var copied WeeklyNativeChild
	if err := json.Unmarshal(body, &copied); err != nil {
		t.Fatal(err)
	}
	if copied.Same(first) || copied.Digest() != "" {
		t.Fatal("serialized metadata reconstructed observed identity")
	}
}
func TestWeeklyNativeChildHelper(t *testing.T) {
	if os.Getenv("MAESTRO_WEEKLY_NATIVE_SYNTHETIC") != "1" {
		return
	}
	fmt.Fprintln(os.Stdout, "ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}
func TestWeeklyNativeDarwinOwnedSyntheticChildFacts(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binary, "-test.run=^TestWeeklyNativeChildHelper$")
	cmd.Env = []string{"MAESTRO_WEEKLY_NATIVE_SYNTHETIC=1"}
	input, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	output, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now().Add(-time.Second)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	reaped := false
	defer func() {
		_ = input.Close()
		_ = output.Close()
		if !reaped {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}()
	line, err := bufio.NewReader(output).ReadString('\n')
	if err != nil || line != "ready\n" {
		t.Fatal("synthetic child not ready", err)
	}
	facts, err := ObserveWeeklyNativeChild(cmd.Process, os.Getpid(), started)
	if err != nil || facts.PID() != cmd.Process.Pid || facts.Digest() == "" {
		t.Fatal("actual synthetic child facts unavailable", err)
	}
	if _, err := ObserveWeeklyNativeChild(cmd.Process, os.Getpid()+1, started); err == nil {
		t.Fatal("wrong parent admitted")
	}
	if _, err := ObserveWeeklyNativeChild(cmd.Process, os.Getpid(), time.Now().Add(time.Minute)); err == nil {
		t.Fatal("wrong exact start interval admitted")
	}
	if err := VerifyWeeklyNativeTerminal(cmd.Process, facts); err == nil {
		t.Fatal("still-live root became terminal success")
	}
	_ = input.Close()
	deadline := time.Now().Add(2 * time.Second)
	for {
		record, err := unix.SysctlKinfoProc("kern.proc.pid", cmd.Process.Pid)
		if err == nil && record != nil && record.Proc.P_stat == 5 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("synthetic root did not reach unreaped terminal state")
		}
		time.Sleep(time.Millisecond)
	}
	if err := VerifyWeeklyNativeTerminal(cmd.Process, facts); err != nil {
		t.Fatal("same unreaped synthetic child was not recognized", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	reaped = true
	if err := VerifyWeeklyNativeTerminal(cmd.Process, facts); err == nil {
		t.Fatal("reaped numeric PID reused as retained identity")
	}
}
