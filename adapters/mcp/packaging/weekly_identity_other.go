//go:build !darwin && !windows

package packaging

import (
	"context"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"os"
	"time"
)

func weeklyIdentityCanonicalPath(*os.File) (string, error) { return "", errWeeklyIdentityUnavailable }
func weeklyIdentityPathsMatch(a, b string) bool            { return a == b }
func verifyWeeklyIdentitySignature(context.Context, *WeeklyRuntimeIdentity) error {
	return errWeeklyIdentityUnavailable
}
func (*WeeklyRuntimeIdentity) ObserveChild(context.Context, *os.Process, int, time.Time) (processidentity.WeeklyNativeChild, error) {
	return processidentity.WeeklyNativeChild{}, errWeeklyIdentityUnavailable
}
func (*WeeklyRuntimeIdentity) VerifyTerminal(context.Context, *os.Process, processidentity.WeeklyNativeChild) error {
	return errWeeklyIdentityUnavailable
}
