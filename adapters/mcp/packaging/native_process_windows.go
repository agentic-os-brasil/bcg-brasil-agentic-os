//go:build windows

package packaging

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ProcThreadAttributeValue(13, false, true, false), available on Windows 10+.
// Association happens inside CreateProcessW, never after a running child starts.
const nativeJobListAttribute = 0x0002000d

func runOwnedNativeCommand(ctx context.Context, path string, args []string, cwd string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, errors.New("native owned job unavailable")
	}
	defer windows.CloseHandle(job)
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		return nil, errors.New("native owned job limits unavailable")
	}
	runtime.KeepAlive(&limits)
	tracker, err := newNativeJobTracker(job)
	if err != nil {
		return nil, err
	}
	defer tracker.close()
	attributes, err := windows.NewProcThreadAttributeList(2)
	if err != nil {
		return nil, err
	}
	defer attributes.Delete()
	jobs := []windows.Handle{job}
	if err := attributes.Update(nativeJobListAttribute, unsafe.Pointer(&jobs[0]), unsafe.Sizeof(jobs[0])); err != nil {
		return nil, errors.New("native creation-time job association unavailable")
	}
	read, write, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer read.Close()
	defer write.Close()
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	defer null.Close()
	handles := []windows.Handle{windows.Handle(write.Fd()), windows.Handle(null.Fd())}
	for _, h := range handles {
		if err := windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
			return nil, err
		}
	}
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return nil, err
	}
	startup := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})), Flags: windows.STARTF_USESTDHANDLES, StdInput: handles[1], StdOutput: handles[0], StdErr: handles[1]}, ProcThreadAttributeList: attributes.List()}
	application, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	argv := append([]string{path}, args...)
	escaped := make([]string, len(argv))
	for i, arg := range argv {
		escaped[i] = windows.EscapeArg(arg)
	}
	command, err := windows.UTF16PtrFromString(strings.Join(escaped, " "))
	if err != nil {
		return nil, err
	}
	var directory *uint16
	if cwd != "" {
		directory, err = windows.UTF16PtrFromString(cwd)
		if err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	process := windows.ProcessInformation{}
	err = windows.CreateProcess(application, command, nil, nil, true, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NO_WINDOW|windows.CREATE_SUSPENDED, nil, directory, &startup.StartupInfo, &process)
	runtime.KeepAlive(attributes)
	runtime.KeepAlive(jobs)
	runtime.KeepAlive(handles)
	runtime.KeepAlive(startup)
	if err != nil {
		return nil, errors.New("native owned process creation unavailable")
	}
	defer windows.CloseHandle(process.Process)
	defer windows.CloseHandle(process.Thread)
	_ = write.Close()
	var output boundedNativeOutput
	drained := make(chan error, 1)
	go func() { _, err := io.Copy(&output, read); drained <- err }()
	var outputErr, runErr error
	// The job association is still atomic at creation. Keep the root suspended
	// only until its exact-job handle and output reader have been retained, then
	// permit user code; this also admits fast version/auth commands reliably.
	tracker.capture(uintptr(process.ProcessId))
	runErr = tracker.failure()
	if runErr == nil {
		if _, err := windows.ResumeThread(process.Thread); err != nil {
			runErr = errors.New("native owned process resume unavailable")
		}
	}
	outputDone := false
	waitOutput := drained
	running := runErr == nil
	for running {
		select {
		case <-ctx.Done():
			runErr = ctx.Err()
			running = false
			continue
		case outputErr = <-waitOutput:
			outputDone = true
			waitOutput = nil
			if outputErr != nil {
				runErr = outputErr
				running = false
				continue
			}
		default:
		}
		tracker.captureCurrent()
		if err := tracker.failure(); err != nil {
			runErr = err
			break
		}
		state, err := windows.WaitForSingleObject(process.Process, 25)
		if err != nil {
			runErr = err
			break
		}
		if state == windows.WAIT_OBJECT_0 {
			var exit uint32
			if err := windows.GetExitCodeProcess(process.Process, &exit); err != nil {
				runErr = err
			} else if exit != 0 {
				runErr = fmt.Errorf("native owned process exited with code %d", exit)
			}
			running = false
		} else if state != uint32(windows.WAIT_TIMEOUT) {
			runErr = errors.New("native owned process wait unavailable")
			running = false
		}
	}
	// Retain the non-inherited job handle until all of its members are gone.
	// Even an ordinary parent exit cannot leave a child holding stdout open.
	tracker.captureCurrent()
	terminateErr := windows.TerminateJobObject(job, 1)
	// Keep notification capture alive through termination: late members still
	// contribute to the ever-assigned census and need actual signaled handles.
	cleanupErr := tracker.waitClosed(2 * time.Second)
	if !outputDone {
		select {
		case outputErr = <-drained:
			outputDone = true
		case <-time.After(2 * time.Second):
			_ = read.Close()
			select {
			case outputErr = <-drained:
				outputDone = true
			case <-time.After(time.Second):
			}
			if outputErr == nil {
				outputErr = errors.New("native owned output drain unavailable")
			}
		}
	}
	if !outputDone {
		return nil, errors.Join(runErr, terminateErr, cleanupErr, outputErr)
	}
	return output.Bytes(), errors.Join(runErr, terminateErr, cleanupErr, outputErr)
}

type nativeJobAccounting struct {
	Times                                 [4]int64
	PageFaults, Total, Active, Terminated uint32
}

func nativeAccounting(job windows.Handle) (nativeJobAccounting, error) {
	var info nativeJobAccounting
	var returned uint32
	err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), &returned)
	runtime.KeepAlive(&info)
	if err != nil || returned != uint32(unsafe.Sizeof(info)) {
		return info, errors.New("native owned job closure unavailable")
	}
	return info, nil
}

// Notifications are best-effort discovery hints, not completion evidence. Keep
// exact-job handles for the entire command and require their signaled state plus
// a matching monotonic job census. A missed/ambiguous identity cannot qualify.
type nativeJobTracker struct {
	job, port windows.Handle
	mu        sync.Mutex
	members   map[uint32]windows.Handle
	err       error
	stop      chan struct{}
	done      chan struct{}
}

func newNativeJobTracker(job windows.Handle) (*nativeJobTracker, error) {
	port, err := windows.CreateIoCompletionPort(windows.InvalidHandle, 0, 0, 1)
	if err != nil {
		return nil, errors.New("native owned process notifications unavailable")
	}
	association := struct {
		Key  uintptr
		Port windows.Handle
	}{1, port}
	_, err = windows.SetInformationJobObject(job, windows.JobObjectAssociateCompletionPortInformation, uintptr(unsafe.Pointer(&association)), uint32(unsafe.Sizeof(association)))
	runtime.KeepAlive(&association)
	if err != nil {
		_ = windows.CloseHandle(port)
		return nil, errors.New("native owned process notification association unavailable")
	}
	t := &nativeJobTracker{job: job, port: port, members: map[uint32]windows.Handle{}, stop: make(chan struct{}), done: make(chan struct{})}
	go t.observe()
	return t, nil
}

func (t *nativeJobTracker) fail(err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.err == nil {
		t.err = err
	}
}
func (t *nativeJobTracker) failure() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.err
}
func (t *nativeJobTracker) capture(id uintptr) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if id == 0 || id > uintptr(^uint32(0)) {
		t.err = errors.New("native owned process identity unavailable")
		return
	}
	if _, known := t.members[uint32(id)]; known {
		return // Retained handles prevent an old identity becoming new custody.
	}
	if len(t.members) >= 256 {
		t.err = errors.New("native owned process census exceeds bound")
		return
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(id))
	if err != nil {
		t.err = errors.New("native owned process custody unavailable")
		return
	}
	var belongs uint32
	ok, _, _ := nativeIsProcessInJob.Call(uintptr(h), uintptr(t.job), uintptr(unsafe.Pointer(&belongs)))
	runtime.KeepAlive(&belongs)
	if ok == 0 || belongs == 0 {
		_ = windows.CloseHandle(h)
		t.err = errors.New("native owned process membership unavailable")
		return
	}
	t.members[uint32(id)] = h
}
func (t *nativeJobTracker) captureCurrent() {
	var ids struct {
		Assigned, Count uint32
		Processes       [256]uintptr
	}
	err := windows.QueryInformationJobObject(t.job, windows.JobObjectBasicProcessIdList, uintptr(unsafe.Pointer(&ids)), uint32(unsafe.Sizeof(ids)), nil)
	runtime.KeepAlive(&ids)
	if err != nil || ids.Assigned != ids.Count || ids.Count > uint32(len(ids.Processes)) {
		t.fail(errors.New("native owned process inventory unavailable"))
		return
	}
	for _, id := range ids.Processes[:ids.Count] {
		t.capture(id)
	}
}
func (t *nativeJobTracker) observe() {
	defer close(t.done)
	for messages := 0; messages < 4096; {
		select {
		case <-t.stop:
			return
		default:
		}
		// A job port uses lpOverlapped as a numeric PID, not a Go pointer.
		var message uint32
		var key, id uintptr
		ok, _, callErr := nativeGetQueuedCompletionStatus.Call(uintptr(t.port), uintptr(unsafe.Pointer(&message)), uintptr(unsafe.Pointer(&key)), uintptr(unsafe.Pointer(&id)), 10)
		if ok == 0 {
			if errors.Is(callErr, windows.WAIT_TIMEOUT) {
				continue
			}
			t.fail(errors.New("native owned process notification read unavailable"))
			return
		}
		messages++
		if key != 1 {
			t.fail(errors.New("native owned process notification identity unavailable"))
			return
		}
		if message == 6 { // JOB_OBJECT_MSG_NEW_PROCESS; never trust its PID alone.
			t.capture(id)
		}
	}
	t.fail(errors.New("native owned process notifications exceed bound"))
}
func (t *nativeJobTracker) waitClosed(limit time.Duration) error {
	deadline := time.Now().Add(limit)
	for {
		t.captureCurrent()
		info, err := nativeAccounting(t.job)
		if err != nil {
			return err
		}
		t.mu.Lock()
		signaled := true
		for _, member := range t.members {
			state, err := windows.WaitForSingleObject(member, 0)
			if err != nil || state != windows.WAIT_OBJECT_0 {
				signaled = false
			}
		}
		observedErr := t.err
		complete := observedErr == nil && info.Active == 0 && info.Total == uint32(len(t.members)) && signaled
		t.mu.Unlock()
		if complete {
			return nil
		}
		if observedErr != nil && info.Active == 0 && signaled {
			return observedErr // Cleanup of observed members never erases failed proof.
		}
		if !time.Now().Before(deadline) {
			return errors.Join(observedErr, errors.New("native owned process completion census unavailable"))
		}
		time.Sleep(time.Millisecond)
	}
}
func (t *nativeJobTracker) close() {
	close(t.stop)
	<-t.done // Never close a process/port handle while the observer can use it.
	for _, member := range t.members {
		_ = windows.CloseHandle(member)
	}
	_ = windows.CloseHandle(t.port)
}

var nativeKernel32 = windows.NewLazySystemDLL("kernel32.dll")
var nativeIsProcessInJob = nativeKernel32.NewProc("IsProcessInJob")
var nativeGetQueuedCompletionStatus = nativeKernel32.NewProc("GetQueuedCompletionStatus")
