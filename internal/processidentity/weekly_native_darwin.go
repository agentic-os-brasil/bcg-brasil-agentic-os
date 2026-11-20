//go:build darwin

package processidentity

import (
	"golang.org/x/sys/unix"
	"os"
	"time"
)

// ObserveWeeklyNativeChild reads facts from an actual retained Start child. The
// service's fixed one-use session, not this lookup, establishes attempt custody.
func ObserveWeeklyNativeChild(child *os.Process, expectedParent int, startedAfter time.Time) (WeeklyNativeChild, error) {
	if child == nil || child.Pid <= 0 || child.Pid > 2147483647 || expectedParent != os.Getpid() || startedAfter.IsZero() {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	first, err := unix.SysctlKinfoProc("kern.proc.pid", child.Pid)
	if err != nil {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	a, err := weeklyDarwinChildFacts(first, child.Pid, expectedParent, startedAfter, false)
	if err != nil {
		return WeeklyNativeChild{}, err
	}
	second, err := unix.SysctlKinfoProc("kern.proc.pid", child.Pid)
	if err != nil {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	b, err := weeklyDarwinChildFacts(second, child.Pid, expectedParent, startedAfter, false)
	if err != nil || !a.Same(b) {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	return a, nil
}

// VerifyWeeklyNativeTerminal accepts only the same unreaped exiting object.
// It cannot turn a still-live or newly reported PID into a terminal observation.
func VerifyWeeklyNativeTerminal(child *os.Process, expected WeeklyNativeChild) error {
	if child == nil || !expected.valid || expected.platform != "darwin" || child.Pid != expected.pid || expected.parent != os.Getpid() {
		return ErrUnavailable
	}
	for range 2 {
		record, err := unix.SysctlKinfoProc("kern.proc.pid", child.Pid)
		if err != nil {
			return ErrUnavailable
		}
		actual, err := weeklyDarwinChildFacts(record, expected.pid, expected.parent, expected.StartedAt(), true)
		if err != nil || !expected.Same(actual) {
			return ErrUnavailable
		}
	}
	return nil
}

func weeklyDarwinChildFacts(record *unix.KinfoProc, pid, parent int, startedAfter time.Time, terminal bool) (WeeklyNativeChild, error) {
	if record == nil || pid <= 0 || parent != os.Getpid() || int(record.Proc.P_pid) != pid || int(record.Eproc.Ppid) != parent || record.Eproc.Pcred.P_ruid != uint32(os.Getuid()) || record.Eproc.Ucred.Uid != uint32(os.Geteuid()) {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	stamp := record.Proc.P_starttime
	if stamp.Sec <= 0 || stamp.Usec < 0 || stamp.Usec >= 1000000 {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	// SDK sys/proc.h: SRUN=2, SSLEEP=3, SZOMB=5, P_WEXIT=0x2000.
	state := record.Proc.P_stat
	exiting := record.Proc.P_flag&0x00002000 != 0
	if terminal {
		if state != 5 && !((state == 2 || state == 3) && exiting) {
			return WeeklyNativeChild{}, ErrUnavailable
		}
	} else if (state != 2 && state != 3) || exiting {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	result := WeeklyNativeChild{valid: true, platform: "darwin", pid: pid, parent: parent, realUID: record.Eproc.Pcred.P_ruid, effectiveUID: record.Eproc.Ucred.Uid, startSeconds: stamp.Sec, startMicroseconds: stamp.Usec}
	if result.StartedAt().Before(startedAfter) || result.StartedAt().After(time.Now().UTC().Add(time.Second)) {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	return result, nil
}
