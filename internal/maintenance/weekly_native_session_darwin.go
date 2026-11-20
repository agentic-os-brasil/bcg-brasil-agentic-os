//go:build darwin

package maintenance

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
)

func validWeeklyDarwinProcess(actual WeeklyNativeProcess) bool {
	return actual.Process != nil && actual.Process.Pid > 1 && uint32(actual.Process.Pid) == actual.ProcessID && actual.ProcessHandle == 0 && actual.ThreadHandle == 0 && actual.JobHandle == 0 && actual.ThreadID == 0
}
func observeWeeklyNativeStart(ctx context.Context, actual WeeklyNativeProcess, after time.Time) (processidentity.WeeklyNativeChild, error) {
	if err := ctx.Err(); err != nil {
		return processidentity.WeeklyNativeChild{}, err
	}
	if !validWeeklyDarwinProcess(actual) {
		return processidentity.WeeklyNativeChild{}, ErrQualificationRequired
	}
	return processidentity.ObserveWeeklyNativeChild(actual.Process, os.Getpid(), after)
}
func verifyWeeklyNativeTerminal(ctx context.Context, actual WeeklyNativeProcess, prior processidentity.WeeklyNativeChild) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validWeeklyDarwinProcess(actual) {
		return ErrQualificationRequired
	}
	return processidentity.VerifyWeeklyNativeTerminal(actual.Process, prior)
}
func verifyWeeklyNativeExit(actual WeeklyNativeProcess, status *os.ProcessState) error {
	if !validWeeklyDarwinProcess(actual) || status == nil || status.Pid() != int(actual.ProcessID) || !status.Exited() || !status.Success() {
		return errors.New("weekly native actual Wait status is not successful")
	}
	return nil
}
