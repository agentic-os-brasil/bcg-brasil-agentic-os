//go:build !darwin && !windows

package packaging

import (
	"context"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
)

func observeWeeklyNativeDeclaration(context.Context, string, Target) (processidentity.WeeklyNativeDeclaration, error) {
	return processidentity.WeeklyNativeDeclaration{}, ErrWeeklyObservationUnavailable
}
