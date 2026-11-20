//go:build darwin

package packaging

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"golang.org/x/sys/unix"
)

// Transport behavior cannot qualify the real tuple or establish the handler's
// owner-wide busy/quarantine boundary. Those bindings remain absent.
func weeklyModelSupervisorReadiness() error { return errWeeklyModelSupervisorUnavailable }

type weeklyRootExitWatch struct {
	fd     int
	pid    uint64
	exited bool
}

func newWeeklyRootExitWatch() (*weeklyRootExitWatch, error) {
	fd, err := unix.Kqueue()
	if err != nil {
		return nil, err
	}
	if err := weeklyDarwinCloseOnExec(fd); err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	return &weeklyRootExitWatch{fd: fd}, nil
}
func weeklyDarwinCloseOnExec(fd int) error {
	flags, err := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	if err != nil {
		return err
	}
	if _, err := unix.FcntlInt(uintptr(fd), unix.F_SETFD, flags|unix.FD_CLOEXEC); err != nil {
		return err
	}
	flags, err = unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
	if err != nil {
		return err
	}
	if flags&unix.FD_CLOEXEC == 0 {
		return errors.New("weekly owned descriptor is not close-on-exec")
	}
	return nil
}

// SyscallConn preserves Go's nonblocking pipe/poller ownership; File.Fd would
// switch it to blocking mode and undermine close-to-unblock cancellation.
func weeklyDarwinPipeCloseOnExec(file *os.File) error {
	connection, err := file.SyscallConn()
	if err != nil {
		return err
	}
	var controlErr error
	err = connection.Control(func(fd uintptr) { controlErr = weeklyDarwinCloseOnExec(int(fd)) })
	return errors.Join(err, controlErr)
}

func (w *weeklyRootExitWatch) close() { _ = unix.Close(w.fd) }
func (w *weeklyRootExitWatch) register(pid int) error {
	if pid <= 0 || w.pid != 0 {
		return errors.New("weekly root registration is invalid")
	}
	w.pid = uint64(pid)
	var change unix.Kevent_t
	unix.SetKevent(&change, pid, unix.EVFILT_PROC, unix.EV_ADD|unix.EV_ONESHOT)
	change.Fflags = unix.NOTE_EXIT
	events := make([]unix.Kevent_t, 1)
	zero := unix.Timespec{}
	n, err := unix.Kevent(w.fd, []unix.Kevent_t{change}, events, &zero)
	if err != nil {
		return err
	}
	if n > 0 {
		return w.observe(events[0])
	}
	return nil
}
func (w *weeklyRootExitWatch) observe(event unix.Kevent_t) error {
	if event.Flags&unix.EV_ERROR != 0 || event.Ident != w.pid || event.Filter != unix.EVFILT_PROC || event.Fflags&unix.NOTE_EXIT == 0 {
		return errors.New("weekly root exit notification unavailable")
	}
	w.exited = true
	return nil
}
func (w *weeklyRootExitWatch) poll(interval time.Duration) (bool, error) {
	if w.exited {
		return true, nil
	}
	events := make([]unix.Kevent_t, 1)
	timeout := unix.NsecToTimespec(interval.Nanoseconds())
	n, err := unix.Kevent(w.fd, nil, events, &timeout)
	if errors.Is(err, unix.EINTR) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if n > 0 {
		if err := w.observe(events[0]); err != nil {
			return false, err
		}
	}
	return w.exited, nil
}

// Darwin's POSIX group kill can return EPERM for an existing zombie-only
// group. This is NOT an EPERM whitelist or a descendant census: only the actual
// signal's EPERM triggers one fixed-record PGRP query while Start ownership is
// still exclusive and unreaped. ENOMEM, partial data or any extra member deny.
// The existing classifier owns the complete raw-signal decision. Its private
// metadata-reader seam admits no runtime authority; the sole production caller
// supplies the fixed one-record PGRP read. Query errors are NEVER normalized.
func weeklyDarwinGroupCleanupResult(signalErr error, rootPID int, read func(int) (*unix.KinfoProc, error)) error {
	if rootPID <= 1 || int64(rootPID) > 2147483647 {
		return errors.Join(signalErr, errors.New("weekly retained group identity invalid"))
	}
	switch signalErr {
	case nil, unix.ESRCH:
		// Only the original syscall result may report the already-empty group.
		return nil
	case unix.EPERM:
		if read == nil {
			return signalErr
		}
		record, readErr := read(rootPID)
		if weeklyDarwinZombieRecordMatches(rootPID, record, readErr) {
			return nil
		}
		return errors.Join(signalErr, readErr)
	default:
		return signalErr
	}
}

func weeklyDarwinZombieRecordMatches(rootPID int, record *unix.KinfoProc, readErr error) bool {
	const zombieState = 5 // SZOMB in the installed public sys/proc.h.
	return rootPID > 1 && int64(rootPID) <= 2147483647 && readErr == nil && record != nil &&
		record.Proc.P_pid == int32(rootPID) && record.Eproc.Pgid == int32(rootPID) &&
		record.Eproc.Ppid == int32(os.Getpid()) &&
		record.Eproc.Pcred.P_ruid == uint32(os.Getuid()) &&
		record.Eproc.Ucred.Uid == uint32(os.Geteuid()) && record.Proc.P_stat == zombieState
}

// One owner retains the successful Start child until cleanup and the sole Wait.
// NOTE_EXIT is notification, not status or a descendant census, and may precede
// the kernel's zombie state. No asynchronous Wait runs.
func runWeeklyModelProcess(ctx context.Context, spec weeklyModelProcess) ([]byte, error) {
	if ctx == nil {
		return nil, errors.New("weekly context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(spec.Input) > 32768 || spec.MaxOutput < 1 || spec.MaxOutput > 32768 || spec.MaxStderr < 1 || spec.MaxStderr > 4096 {
		return nil, errors.New("weekly CLI transport bounds invalid")
	}
	watch, err := newWeeklyRootExitWatch()
	if err != nil {
		return nil, errors.New("weekly root notification unavailable")
	}
	defer watch.close()
	pipes := make([]*os.File, 0, 6)
	defer func() {
		for _, file := range pipes {
			_ = file.Close()
		}
	}()
	pipe := func() (*os.File, *os.File, error) {
		r, w, err := os.Pipe()
		if err != nil {
			return nil, nil, err
		}
		pipes = append(pipes, r, w)
		for _, f := range []*os.File{r, w} {
			if err := weeklyDarwinPipeCloseOnExec(f); err != nil {
				return nil, nil, err
			}
		}
		return r, w, nil
	}
	inputRead, inputWrite, err := pipe()
	if err != nil {
		return nil, err
	}
	outputRead, outputWrite, err := pipe()
	if err != nil {
		return nil, err
	}
	errorRead, errorWrite, err := pipe()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(spec.Path, spec.Args...)
	cmd.Dir = spec.Directory
	cmd.Env = append([]string(nil), spec.Environment...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = inputRead, outputWrite, errorWrite
	configureNativeProcess(cmd)
	cancelGroup := cmd.Cancel
	cmd.Cancel = nil
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, errors.New("weekly CLI child start failed")
	}
	// No return, competing Wait, Release or context watcher below Start: every
	// terminal path reaches owned group cleanup before the sole Wait.
	_ = inputRead.Close()
	_ = outputWrite.Close()
	_ = errorWrite.Close()
	stdout := weeklyModelOutput{limit: spec.MaxOutput}
	stderr := weeklyModelOutput{limit: spec.MaxStderr}
	outputDone, errorDone := make(chan error, 1), make(chan error, 1)
	go func() { _, err := io.Copy(&stdout, outputRead); outputDone <- err }()
	go func() { _, err := io.Copy(&stderr, errorRead); errorDone <- err }()
	var inputDone chan error
	runErr := watch.register(cmd.Process.Pid)
	actual := maintenance.WeeklyNativeProcess{Process: cmd.Process, ProcessID: uint32(cmd.Process.Pid)}
	if runErr == nil && spec.native != nil {
		if spec.native.session == nil || spec.native.identity == nil {
			runErr = maintenance.ErrQualificationRequired
		} else {
			runErr = spec.native.session.ObserveStarted(ctx, actual)
			if runErr == nil {
				child, e := spec.native.identity.ObserveChild(ctx, cmd.Process, os.Getpid(), time.Now().Add(-time.Second))
				runErr = e
				if runErr == nil {
					runErr = spec.native.session.AdmitInput(ctx, actual, child)
				}
			}
		}
	}
	if runErr == nil && spec.native != nil {
		runErr = spec.native.check(ctx)
	}
	if runErr == nil {
		// Registration failure never sends the quoted private input to the child.
		inputDone = make(chan error, 1)
		go func() {
			write := func() error { _, err := inputWrite.Write(spec.Input); return errors.Join(err, inputWrite.Close()) }
			var err error
			if spec.native != nil {
				err = spec.native.session.WriteInput(ctx, actual, write)
			} else {
				err = write()
			}
			inputDone <- err
		}()
	}
	inputJoined, outputJoined, errorJoined := inputDone == nil, false, false
	for runErr == nil && !watch.exited {
		if err := ctx.Err(); err != nil {
			runErr = err
			break
		}
		select {
		case err := <-inputDone:
			inputJoined = true
			inputDone = nil
			runErr = err
		default:
		}
		if inputJoined && spec.native != nil {
			if err := spec.native.check(ctx); err != nil {
				runErr = err
				break
			}
		}
		select {
		case err := <-outputDone:
			outputJoined = true
			outputDone = nil
			runErr = errors.Join(runErr, err)
		default:
		}
		select {
		case err := <-errorDone:
			errorJoined = true
			errorDone = nil
			runErr = errors.Join(runErr, err)
		default:
		}
		if runErr != nil {
			break
		}
		interval := 25 * time.Millisecond
		if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < interval {
			interval = time.Until(deadline)
			if interval <= 0 {
				runErr = ctx.Err()
				if runErr == nil {
					runErr = context.DeadlineExceeded
				}
				break
			}
		}
		_, runErr = watch.poll(interval)
	}
	// The leader is still unreaped on registration/notification/I/O failure too.
	// Retained ownership permits this group signal. Never signal after cmd.Wait.
	cleanupErr := weeklyDarwinGroupCleanupResult(cancelGroup(), cmd.Process.Pid, func(pid int) (*unix.KinfoProc, error) { return unix.SysctlKinfoProc("kern.proc.pgrp", pid) })
	grace := time.Now().Add(2 * time.Second)
	_ = inputWrite.Close()
	join := func(done <-chan error, closeOwned func() error) error {
		remaining := time.Until(grace)
		if remaining < 0 {
			remaining = 0
		}
		timer := time.NewTimer(remaining)
		defer timer.Stop()
		select {
		case err := <-done:
			return err
		case <-timer.C:
			_ = closeOwned()
			// Closing an owned file unblocks the pipe worker; retain synchronous
			// ownership until it joins and cannot mutate a returned buffer.
			err := <-done
			return errors.Join(errors.New("weekly pipe cleanup exceeded grace"), err)
		}
	}
	if !inputJoined {
		cleanupErr = errors.Join(cleanupErr, join(inputDone, inputWrite.Close))
	}
	if !outputJoined {
		cleanupErr = errors.Join(cleanupErr, join(outputDone, outputRead.Close))
	}
	if !errorJoined {
		cleanupErr = errors.Join(cleanupErr, join(errorDone, errorRead.Close))
	}
	// NOTE_EXIT may precede zombie state. Keep synchronous ownership until the
	// actual reap; 2s is a polling/drain target, never a kernel-wait promise.
	if runErr == nil && cleanupErr == nil && ctx.Err() == nil && spec.native != nil {
		cleanupErr = spec.native.session.ObserveTerminal(ctx, actual)
	}
	waitErr := cmd.Wait()
	if runErr == nil && cleanupErr == nil && waitErr == nil && spec.native != nil {
		cleanupErr = spec.native.session.ObserveReaped(ctx, actual, cmd.ProcessState)
	}
	if time.Now().After(grace) {
		cleanupErr = errors.Join(cleanupErr, errors.New("weekly synchronous cleanup exceeded grace"))
	}
	if runErr != nil || cleanupErr != nil || waitErr != nil || ctx.Err() != nil {
		return nil, errors.Join(errors.New("weekly CLI child/cleanup failed or was cancelled"), runErr, cleanupErr, waitErr, ctx.Err())
	}
	return stdout.Bytes(), nil
}
