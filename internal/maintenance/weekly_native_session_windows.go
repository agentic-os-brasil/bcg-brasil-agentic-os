//go:build windows

package maintenance

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"golang.org/x/sys/windows"
)

func validWeeklyWindowsProcess(actual WeeklyNativeProcess) bool {
	return actual.Process == nil && actual.ProcessHandle != 0 && actual.ThreadHandle != 0 && actual.JobHandle != 0 && actual.ProcessID > 1 && actual.ThreadID > 0
}
func observeWeeklyNativeStart(ctx context.Context, actual WeeklyNativeProcess, after time.Time) (processidentity.WeeklyNativeChild, error) {
	if !validWeeklyWindowsProcess(actual) {
		return processidentity.WeeklyNativeChild{}, ErrQualificationRequired
	}
	child, err := processidentity.ObserveWeeklyWindowsChild(ctx, windows.Handle(actual.ProcessHandle), windows.Handle(actual.ThreadHandle), windows.Handle(actual.JobHandle), actual.ProcessID, actual.ThreadID)
	if err != nil {
		return child, err
	}
	if child.StartedAt().Before(after) {
		return processidentity.WeeklyNativeChild{}, ErrQualificationRequired
	}
	return child, nil
}
func verifyWeeklyNativeTerminal(ctx context.Context, actual WeeklyNativeProcess, prior processidentity.WeeklyNativeChild) error {
	if !validWeeklyWindowsProcess(actual) {
		return ErrQualificationRequired
	}
	return processidentity.VerifyWeeklyWindowsTerminal(ctx, windows.Handle(actual.ProcessHandle), windows.Handle(actual.ThreadHandle), windows.Handle(actual.JobHandle), actual.ProcessID, actual.ThreadID, prior)
}
func verifyWeeklyNativeExit(actual WeeklyNativeProcess, status *os.ProcessState) error {
	if !validWeeklyWindowsProcess(actual) || status != nil {
		return ErrQualificationRequired
	}
	var code uint32
	if err := windows.GetExitCodeProcess(windows.Handle(actual.ProcessHandle), &code); err != nil {
		return err
	}
	if code != 0 {
		return errors.New("weekly native retained Windows process did not exit successfully")
	}
	return nil
}
