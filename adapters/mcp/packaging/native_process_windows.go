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
	err = windows.CreateProcess(application, command, nil, nil, true, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NO_WINDOW, nil, directory, &startup.StartupInfo, &process)
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
	outputDone := false
	waitOutput := drained
	running := true
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
	terminateErr := windows.TerminateJobObject(job, 1)
	cleanupErr := waitNativeJobEmpty(job, 2*time.Second)
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

func waitNativeJobEmpty(job windows.Handle, limit time.Duration) error {
	// JOBOBJECT_BASIC_ACCOUNTING_INFORMATION has four LARGE_INTEGER times and
	// four DWORD counters. ActiveProcesses is the third DWORD counter.
	var info struct {
		Times                                 [4]int64
		PageFaults, Total, Active, Terminated uint32
	}
	deadline := time.Now().Add(limit)
	for {
		var returned uint32
		err := windows.QueryInformationJobObject(job, windows.JobObjectBasicAccountingInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)), &returned)
		runtime.KeepAlive(&info)
		runtime.KeepAlive(&returned)
		if err != nil || returned != uint32(unsafe.Sizeof(info)) {
			return errors.New("native owned job closure unavailable")
		}
		if info.Active == 0 {
			return nil
		}
		if !time.Now().Before(deadline) {
			return errors.New("native owned job members did not close")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
