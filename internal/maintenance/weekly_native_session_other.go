//go:build !darwin && !windows

package maintenance

import (
	"context"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"os"
	"time"
)

func observeWeeklyNativeStart(context.Context, WeeklyNativeProcess, time.Time) (processidentity.WeeklyNativeChild, error) {
	return processidentity.WeeklyNativeChild{}, ErrQualificationRequired
}
func verifyWeeklyNativeTerminal(context.Context, WeeklyNativeProcess, processidentity.WeeklyNativeChild) error {
	return ErrQualificationRequired
}
func verifyWeeklyNativeExit(WeeklyNativeProcess, *os.ProcessState) error {
	return ErrQualificationRequired
}
