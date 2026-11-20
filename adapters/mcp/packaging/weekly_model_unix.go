//go:build !windows && !darwin

package packaging

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// The current owned-group runner reaps before retained-pipe cleanup, so its
// declared cancellation topology is not ready. The focused Darwin RED records
// that concrete gap; no universal no-fork or immutable-image claim is required.
func weeklyModelSupervisorReadiness() error { return errWeeklyModelSupervisorUnavailable }

// Reuse the existing owned process-group cancellation primitive only. Native
// proof/admission/custody routines are not involved in model transport.
func runWeeklyModelProcess(ctx context.Context, spec weeklyModelProcess) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(spec.Input) > 32768 || spec.MaxOutput < 1 || spec.MaxOutput > 32768 || spec.MaxStderr < 1 || spec.MaxStderr > 4096 {
		return nil, errors.New("weekly CLI transport bounds invalid")
	}
	cmd := exec.CommandContext(ctx, spec.Path, spec.Args...)
	cmd.Dir = spec.Directory
	cmd.Env = append([]string(nil), spec.Environment...)
	cmd.Stdin = bytes.NewReader(spec.Input)
	cmd.WaitDelay = 2 * time.Second
	configureNativeProcess(cmd)
	stdout := weeklyModelOutput{limit: spec.MaxOutput}
	stderr := weeklyModelOutput{limit: spec.MaxStderr}
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	// Do not signal a numeric process group after cmd.Wait has reaped its
	// leader: that could target a reused identifier. Cancellation while owned
	// uses the existing configureNativeProcess primitive. Detached-descendant
	// closure is not attested by this source transport.

	if err != nil {
		return nil, errors.New("weekly CLI child failed, exceeded bounds or was cancelled")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}
