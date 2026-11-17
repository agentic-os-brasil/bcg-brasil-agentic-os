//go:build windows

package packaging

import (
	"context"
	"errors"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"golang.org/x/sys/windows"
	"io"
	"os"
	"runtime"
	"strings"
	"time"
	"unicode/utf16"
	"unsafe"
)

// The job-specific JobObject transport still awaits focused platform validation
// and complete native tuple binding. This source gate asserts neither native
// qualification nor a new absolute lifetime-image prerequisite.
func weeklyModelSupervisorReadiness() error { return errWeeklyModelSupervisorUnavailable }

// Job-specific stdin/closed-environment launch. Creation-time ownership and
// descendant census reuse the existing pure platform lifetime helpers only.
func runWeeklyModelProcess(ctx context.Context, spec weeklyModelProcess) ([]byte, error) {
	if len(spec.Input) > 32768 || spec.MaxOutput < 1 || spec.MaxOutput > 32768 || spec.MaxStderr < 1 || spec.MaxStderr > 4096 {
		return nil, errors.New("weekly CLI transport bounds invalid")
	}
	path, args, cwd := spec.Path, spec.Args, spec.Directory
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
	inputRead, inputWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer inputRead.Close()
	defer inputWrite.Close()
	errorRead, errorWrite, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	defer errorRead.Close()
	defer errorWrite.Close()
	handles := []windows.Handle{windows.Handle(write.Fd()), windows.Handle(inputRead.Fd()), windows.Handle(errorWrite.Fd())}
	for _, h := range handles {
		if err := windows.SetHandleInformation(h, windows.HANDLE_FLAG_INHERIT, windows.HANDLE_FLAG_INHERIT); err != nil {
			return nil, err
		}
	}
	if err := attributes.Update(windows.PROC_THREAD_ATTRIBUTE_HANDLE_LIST, unsafe.Pointer(&handles[0]), uintptr(len(handles))*unsafe.Sizeof(handles[0])); err != nil {
		return nil, err
	}
	startup := windows.StartupInfoEx{StartupInfo: windows.StartupInfo{Cb: uint32(unsafe.Sizeof(windows.StartupInfoEx{})), Flags: windows.STARTF_USESTDHANDLES, StdInput: handles[1], StdOutput: handles[0], StdErr: handles[2]}, ProcThreadAttributeList: attributes.List()}
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
	environment := utf16.Encode([]rune(strings.Join(spec.Environment, "\x00") + "\x00\x00"))
	process := windows.ProcessInformation{}
	err = windows.CreateProcess(application, command, nil, nil, true, windows.EXTENDED_STARTUPINFO_PRESENT|windows.CREATE_UNICODE_ENVIRONMENT|windows.CREATE_NO_WINDOW|windows.CREATE_SUSPENDED, &environment[0], directory, &startup.StartupInfo, &process)
	runtime.KeepAlive(attributes)
	runtime.KeepAlive(jobs)
	runtime.KeepAlive(handles)
	runtime.KeepAlive(startup)
	runtime.KeepAlive(environment)
	if err != nil {
		return nil, errors.New("native owned process creation unavailable")
	}
	defer windows.CloseHandle(process.Process)
	defer windows.CloseHandle(process.Thread)
	_ = write.Close()
	_ = inputRead.Close()
	_ = errorWrite.Close()
	var inputDone chan error
	output := weeklyModelOutput{limit: spec.MaxOutput}
	stderr := weeklyModelOutput{limit: spec.MaxStderr}
	errorDrained := make(chan error, 1)
	go func() { _, e := io.Copy(&stderr, errorRead); errorDrained <- e }()
	drained := make(chan error, 1)
	go func() { _, err := io.Copy(&output, read); drained <- err }()
	var outputErr, runErr error
	// The job association is still atomic at creation. Keep the root suspended
	// only until its exact-job handle and output reader have been retained, then
	// permit user code; this also admits fast version/auth commands reliably.
	tracker.capture(uintptr(process.ProcessId))
	runErr = tracker.failure()
	actual := maintenance.WeeklyNativeProcess{ProcessHandle: uintptr(process.Process), ThreadHandle: uintptr(process.Thread), JobHandle: uintptr(job), ProcessID: process.ProcessId, ThreadID: process.ThreadId}
	if runErr == nil && spec.native != nil {
		if spec.native.session == nil || spec.native.identity == nil {
			runErr = maintenance.ErrQualificationRequired
		} else {
			runErr = spec.native.session.ObserveStarted(ctx, actual)
			if runErr == nil {
				child, e := spec.native.identity.ObserveWindowsChild(ctx, process.Process, process.Thread, job, process.ProcessId, process.ThreadId)
				runErr = e
				if runErr == nil {
					runErr = spec.native.session.AdmitInput(ctx, actual, child)
				}
			}
		}
	}
	if runErr == nil {
		if err := ctx.Err(); err != nil {
			runErr = err
		}
	}
	if runErr == nil {
		if _, err := windows.ResumeThread(process.Thread); err != nil {
			runErr = errors.New("native owned process resume unavailable")
		}
	}
	if runErr == nil && spec.native != nil {
		runErr = spec.native.check(ctx)
	}
	if runErr == nil {
		inputDone = make(chan error, 1)
		go func() {
			write := func() error { _, e := inputWrite.Write(spec.Input); return errors.Join(e, inputWrite.Close()) }
			var e error
			if spec.native != nil {
				e = spec.native.session.WriteInput(ctx, actual, write)
			} else {
				e = write()
			}
			inputDone <- e
		}()
	}
	inputJoined := inputDone == nil
	outputDone := false
	waitOutput := drained
	running := runErr == nil
	for running {
		select {
		case <-ctx.Done():
			runErr = ctx.Err()
			running = false
			continue
		case stderrErr := <-errorDrained:
			errorDrained = nil
			if stderrErr != nil {
				runErr = stderrErr
				running = false
				continue
			}
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
		select {
		case e := <-inputDone:
			inputJoined = true
			inputDone = nil
			if e != nil {
				runErr = e
				running = false
				continue
			}
		default:
		}
		if inputJoined && spec.native != nil {
			if e := spec.native.check(ctx); e != nil {
				runErr = e
				break
			}
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
	_ = inputWrite.Close()
	if !inputJoined {
		// Closing the owned pipe unblocks the only writer; join it before return.
		runErr = errors.Join(runErr, <-inputDone)
	}
	if errorDrained != nil {
		select {
		case e := <-errorDrained:
			runErr = errors.Join(runErr, e)
		case <-time.After(time.Second):
			_ = errorRead.Close()
			runErr = errors.Join(runErr, errors.New("weekly stderr drain unavailable"))
		}
	}
	if runErr == nil && terminateErr == nil && cleanupErr == nil && outputErr == nil && spec.native != nil {
		runErr = spec.native.session.ObserveTerminal(ctx, actual)
		if runErr == nil {
			runErr = spec.native.session.ObserveReaped(ctx, actual, nil)
		}
	}
	if !outputDone {
		return nil, errors.Join(runErr, terminateErr, cleanupErr, outputErr)
	}
	if err := errors.Join(runErr, terminateErr, cleanupErr, outputErr); err != nil {
		return nil, errors.New("weekly CLI child failed, exceeded bounds or was cancelled")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}
