//go:build windows

package processidentity

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	weeklyKernel32        = windows.NewLazySystemDLL("kernel32.dll")
	weeklyThreadProcessID = weeklyKernel32.NewProc("GetProcessIdOfThread")
	weeklyThreadID        = weeklyKernel32.NewProc("GetThreadId")
	weeklyProcessInJob    = weeklyKernel32.NewProc("IsProcessInJob")
)

// These queries borrow the supervisor's actual CreateProcess handles. They never
// discover, reopen, resume, terminate or close a process or thread.
type weeklyWindowsQueries interface {
	processID(windows.Handle) (uint32, error)
	threadIDs(windows.Handle) (uint32, uint32, error)
	creation(windows.Handle) (uint64, error)
	inJob(windows.Handle, windows.Handle) (bool, error)
	state(windows.Handle) (uint32, error)
	image(windows.Handle) (string, error)
}
type weeklyWindowsSystem struct{}

func (weeklyWindowsSystem) processID(h windows.Handle) (uint32, error) {
	return windows.GetProcessId(h)
}
func (weeklyWindowsSystem) threadIDs(h windows.Handle) (uint32, uint32, error) {
	if weeklyThreadProcessID.Find() != nil || weeklyThreadID.Find() != nil {
		return 0, 0, ErrUnavailable
	}
	p, _, _ := weeklyThreadProcessID.Call(uintptr(h))
	t, _, _ := weeklyThreadID.Call(uintptr(h))
	if p == 0 || t == 0 || p > weeklyMaximumWindowsID || t > weeklyMaximumWindowsID {
		return 0, 0, ErrUnavailable
	}
	return uint32(p), uint32(t), nil
}

const weeklyMaximumWindowsID = uintptr(^uint32(0))

func (weeklyWindowsSystem) creation(h windows.Handle) (uint64, error) {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &created, &exited, &kernel, &user); err != nil {
		return 0, ErrUnavailable
	}
	return uint64(created.HighDateTime)<<32 | uint64(created.LowDateTime), nil
}
func (weeklyWindowsSystem) inJob(process, job windows.Handle) (bool, error) {
	if weeklyProcessInJob.Find() != nil {
		return false, ErrUnavailable
	}
	var in int32
	r, _, _ := weeklyProcessInJob.Call(uintptr(process), uintptr(job), uintptr(unsafe.Pointer(&in)))
	if r == 0 {
		return false, ErrUnavailable
	}
	return in != 0, nil
}
func (weeklyWindowsSystem) state(h windows.Handle) (uint32, error) {
	return windows.WaitForSingleObject(h, 0)
}
func (weeklyWindowsSystem) image(h windows.Handle) (string, error) {
	var buffer [32768]uint16
	n := uint32(len(buffer))
	if err := windows.QueryFullProcessImageName(h, 0, &buffer[0], &n); err != nil {
		return "", ErrUnavailable
	}
	return weeklyWindowsImagePath(buffer[:], n)
}
func weeklyWindowsImagePath(buffer []uint16, n uint32) (string, error) {
	if n == 0 || int(n) >= len(buffer) || buffer[n] != 0 {
		return "", ErrUnavailable
	}
	for i := 0; i < int(n); i++ {
		c := buffer[i]
		if c < 32 || c >= 0xD800 && c <= 0xDBFF && (i+1 >= int(n) || buffer[i+1] < 0xDC00 || buffer[i+1] > 0xDFFF) || c >= 0xDC00 && c <= 0xDFFF && (i == 0 || buffer[i-1] < 0xD800 || buffer[i-1] > 0xDBFF) {
			return "", ErrUnavailable
		}
	}
	path := string(utf16.Decode(buffer[:n]))
	if len(path) < 4 || !((path[0] >= 'A' && path[0] <= 'Z') || (path[0] >= 'a' && path[0] <= 'z')) || path[1:3] != `:\` || strings.ContainsAny(path[3:], "/:*?\"<>|\x00") {
		return "", ErrUnavailable
	}
	for _, part := range strings.Split(path[3:], `\`) {
		if part == "" || part == "." || part == ".." || strings.Contains(part, "~") || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") || weeklyWindowsDeviceComponent(part) {
			return "", ErrUnavailable
		}
	}
	return path, nil
}

func weeklyWindowsDeviceComponent(part string) bool {
	name := strings.ToUpper(strings.SplitN(part, ".", 2)[0])
	switch name {
	case "CON", "PRN", "AUX", "NUL", "CONIN$", "CONOUT$":
		return true
	}
	return len(name) == 4 && (strings.HasPrefix(name, "COM") || strings.HasPrefix(name, "LPT")) && name[3] >= '1' && name[3] <= '9'
}

func weeklyRetainedWindowsHandle(handle windows.Handle) bool {
	// Pseudo handles are not owned CreateProcess/Job objects.
	return handle != 0 && uintptr(handle) < ^uintptr(0)-15
}

// ObserveWeeklyWindowsChild observes only facts on the actual retained live
// CreateProcess result. The fixed service session owns attempt provenance.
func ObserveWeeklyWindowsChild(ctx context.Context, process, thread, job windows.Handle, pid, tid uint32) (WeeklyNativeChild, error) {
	return weeklyObserveWindows(ctx, weeklyWindowsSystem{}, process, thread, job, pid, tid, false)
}
func weeklyObserveWindows(ctx context.Context, q weeklyWindowsQueries, process, thread, job windows.Handle, pid, tid uint32, terminal bool) (WeeklyNativeChild, error) {
	if ctx == nil || ctx.Err() != nil || (runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64") || !weeklyRetainedWindowsHandle(process) || !weeklyRetainedWindowsHandle(thread) || !weeklyRetainedWindowsHandle(job) || pid == 0 || tid == 0 {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	actual, err := q.processID(process)
	if err != nil || actual != pid {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	threadProcess, threadID, err := q.threadIDs(thread)
	if err != nil || threadProcess != pid || threadID != tid {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	created, err := q.creation(process)
	if err != nil || created == 0 {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	member, err := q.inJob(process, job)
	if err != nil || !member {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	state, err := q.state(process)
	wanted := uint32(windows.WAIT_TIMEOUT)
	if terminal {
		wanted = windows.WAIT_OBJECT_0
	}
	if err != nil || state != wanted {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	path, err := q.image(process)
	if err != nil {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	after, err := q.creation(process)
	if err != nil || after != created {
		return WeeklyNativeChild{}, ErrChanged
	}
	state, err = q.state(process)
	if err != nil || state != wanted || ctx.Err() != nil {
		return WeeklyNativeChild{}, ErrUnavailable
	}
	return WeeklyNativeChild{valid: true, platform: "windows", pid: int(pid), creationTime: created, processHandle: uintptr(process), threadHandle: uintptr(thread), jobHandle: uintptr(job), threadID: tid, imagePath: path}, nil
}

// VerifyWeeklyWindowsTerminal rechecks the same retained, now signaled object;
// it cannot create a new observation or adopt a reported PID after exit.
func VerifyWeeklyWindowsTerminal(ctx context.Context, process, thread, job windows.Handle, pid, tid uint32, expected WeeklyNativeChild) error {
	return weeklyVerifyWindowsTerminal(ctx, weeklyWindowsSystem{}, process, thread, job, pid, tid, expected)
}
func weeklyVerifyWindowsTerminal(ctx context.Context, q weeklyWindowsQueries, process, thread, job windows.Handle, pid, tid uint32, expected WeeklyNativeChild) error {
	if !expected.valid || expected.platform != "windows" || expected.processHandle != uintptr(process) || expected.threadHandle != uintptr(thread) || expected.jobHandle != uintptr(job) || expected.pid != int(pid) || expected.threadID != tid || expected.creationTime == 0 {
		return ErrUnavailable
	}
	current, err := weeklyObserveWindows(ctx, q, process, thread, job, pid, tid, true)
	if err != nil {
		return err
	}
	if !expected.Same(current) {
		return errors.Join(ErrChanged, ErrUnavailable)
	}
	return nil
}
