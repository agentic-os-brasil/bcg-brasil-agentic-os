package macosadapter

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// ReadOwnedWakeFile authenticates only the derived wake's local eligibility.
func ReadOwnedWakeFile(home string, spec Spec) (LaunchAgentStatus, error) {
	if err := validateNativeLabel(spec.Label); err != nil || spec.Label == "com.bcg.maestro.maintenance" {
		return LaunchAgentStatus{}, errors.New("exact derived maintenance wake required")
	}
	file, err := ReadStatus(home, spec.Label)
	status := LaunchAgentStatus{State: "not_present", Path: file.Path, Label: spec.Label}
	if err != nil || file.State == "not_installed" {
		return status, err
	}
	status.FilePresent, status.Disabled, status.State = true, file.Disabled, "file_present"
	f, err := scheduler.OpenRegularFile(file.Path)
	if err != nil {
		return status, err
	}
	err = scheduler.ValidatePrivateFile(f, 0600)
	closeErr := f.Close()
	if err != nil || closeErr != nil {
		return status, errors.Join(err, closeErr)
	}
	spec.Disabled = file.Disabled
	_, err = Verify(home, spec)
	return status, err
}

// InspectOwnedWake keeps unknown native acknowledgement distinct from absence.
func (lifecycle Lifecycle) InspectOwnedWake(ctx context.Context, home string, spec Spec) (LaunchAgentStatus, error) {
	status, err := ReadOwnedWakeFile(home, spec)
	if err != nil {
		return status, err
	}
	if !lifecycle.Native || lifecycle.Runner == nil || !samePath(home, lifecycle.CurrentHome) || !uidPattern.MatchString(lifecycle.UID) {
		return status, errors.New("current-user native wake lifecycle unavailable")
	}
	if lifecycle.Timeout <= 0 || lifecycle.Timeout > 15*time.Second {
		lifecycle.Timeout = 15 * time.Second
	}
	if err := ctx.Err(); err != nil {
		return status, err
	}
	printed, err := lifecycle.runResult(ctx, []string{"print", "gui/" + lifecycle.UID + "/" + spec.Label})
	if ctx.Err() != nil {
		return status, ctx.Err()
	}
	if err != nil {
		if printed.ExitCode == 113 {
			if status.FilePresent {
				status.State = "file_present_not_loaded"
			}
			return status, nil
		}
		return status, err
	}
	status.Loaded = true
	program, path, err := parseLaunchctlIdentity(printed.Output, spec.Label)
	if !status.FilePresent || err != nil || program != spec.Program || !samePath(path, status.Path) {
		return status, errors.New("occupied native wake identity is foreign or incomplete; preserved unchanged")
	}
	if err := VerifyLoadedArguments(printed.Output, spec); err != nil {
		return status, err
	}
	disabled, err := lifecycle.runResult(ctx, []string{"print-disabled", "gui/" + lifecycle.UID})
	if err != nil || ctx.Err() != nil {
		return status, errors.Join(err, ctx.Err())
	}
	seen := false
	for _, line := range strings.Split(disabled.Output, "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), "=>", 2)
		if len(parts) != 2 || strings.Trim(strings.TrimSpace(parts[0]), "\"") != spec.Label {
			continue
		}
		value := strings.TrimSpace(parts[1])
		if seen || value != "true" && value != "false" {
			return status, errors.New("native wake enabled acknowledgement is ambiguous")
		}
		seen, status.Enabled = true, value == "false"
	}
	if !seen {
		return status, errors.New("native wake enabled acknowledgement is missing")
	}
	status.NativeQualified, status.State = true, "loaded_disabled"
	if status.Enabled && !status.Disabled {
		status.State = "active_loaded_enabled"
	}
	return status, nil
}

// ApplyOwnedWake runs inside the installer's original publication guard.
func (lifecycle Lifecycle) ApplyOwnedWake(ctx context.Context, home string, spec Spec, action string, recheck func() error) (status LaunchAgentStatus, err error) {
	if action != "opt-in" && action != "pause" && action != "revoke" || recheck == nil {
		return status, errors.New("closed original Mac wake transaction required")
	}
	status, err = ReadOwnedWakeFile(home, spec)
	if err != nil {
		return status, err
	}
	if !lifecycle.Native || lifecycle.Runner == nil || !samePath(home, lifecycle.CurrentHome) || !uidPattern.MatchString(lifecycle.UID) {
		return status, errors.New("current-user native wake lifecycle unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if lifecycle.Timeout <= 0 || lifecycle.Timeout > 15*time.Second {
		lifecycle.Timeout = 15 * time.Second
	}
	closed := spec
	closed.Disabled = true
	// File eligibility closes without an OS call, even after cancellation.
	if status.FilePresent || action == "opt-in" {
		if _, err = Install(home, closed, true); err != nil {
			return status, err
		}
		status.FilePresent, status.Disabled = true, true
	}
	status, err = lifecycle.InspectOwnedWake(ctx, home, closed)
	if err != nil {
		return status, err
	}
	if action != "opt-in" {
		if status.Loaded {
			if err = lifecycle.removeNative(ctx, spec.Label); err != nil {
				return status, err
			}
			status, err = lifecycle.InspectOwnedWake(ctx, home, closed)
			if err != nil || status.Loaded {
				return status, errors.Join(err, errors.New("native wake stop acknowledgement unavailable"))
			}
		}
		if action == "revoke" {
			if err = Uninstall(home, spec.Label); err != nil {
				return status, err
			}
			status.FilePresent, status.State = false, "not_present"
		}
		return status, ctx.Err()
	}
	if err = ctx.Err(); err != nil {
		return status, err
	}
	if err = lifecycle.run(ctx, []string{"enable", "gui/" + lifecycle.UID + "/" + spec.Label}); err != nil {
		return status, err
	}
	if !status.Loaded {
		if err = lifecycle.run(ctx, []string{"bootstrap", "gui/" + lifecycle.UID, status.Path}); err != nil {
			return status, err
		}
	}
	if err = lifecycle.kickstartWithRecovery(ctx, home, spec.Label); err != nil {
		return status, err
	}
	status, err = lifecycle.InspectOwnedWake(ctx, home, closed)
	if err != nil {
		return status, err
	}
	if !status.Loaded || !status.Enabled || !status.NativeQualified || !status.Disabled {
		return status, errors.New("exact enabled native wake acknowledgement unavailable")
	}
	if err = recheck(); err != nil {
		return status, err
	}
	if err = ctx.Err(); err != nil {
		return status, err
	}
	if _, err = Verify(home, closed); err != nil {
		return status, err
	}
	if _, err = Resume(home, spec.Label); err != nil {
		_, closeErr := Install(home, closed, true)
		return status, errors.Join(err, closeErr)
	}
	if err = ctx.Err(); err != nil {
		_, closeErr := Install(home, closed, true)
		return status, errors.Join(err, closeErr)
	}
	status.Disabled, status.State = false, "active_loaded_enabled"
	return status, nil
}
