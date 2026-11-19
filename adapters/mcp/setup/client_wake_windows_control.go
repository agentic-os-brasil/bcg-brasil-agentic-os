package setup

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/windowsadapter"
)

func readWindowsWakeControl(binding installer.MaintenanceWakeBinding) (installer.MaintenanceWakeControl, error) {
	control, err := binding.ReadWindowsWakeControl()
	if errors.Is(err, os.ErrNotExist) {
		digest, e := binding.Digest()
		if e != nil {
			return control, e
		}
		return installer.MaintenanceWakeControl{SchemaVersion: 1, BindingSHA256: digest}, nil
	}
	return control, err
}
func windowsWakeStatus(state installer.MaintenanceWakeState, control installer.MaintenanceWakeControl, native *windowsadapter.NativeWakeStatus) ClientWakeStatus {
	out := ClientWakeStatus{Platform: "windows", State: "disabled", Binding: state, WindowsControl: &control, WindowsNative: native, JobAuthority: "separate_original_job_qualification_and_enrollment_required"}
	if control.Revision == 0 {
		out.State = "not_enrolled"
	} else if control.Enabled {
		out.State = "native_task_unavailable"
		if native != nil && native.Present && native.Enabled {
			out.State = "active"
		}
	}
	return out
}
func reviewWindowsWakeState(ctx context.Context, state installer.MaintenanceWakeState, action string) (ClientWakeReview, error) {
	if err := ctx.Err(); err != nil {
		return ClientWakeReview{}, err
	}
	if action != "opt-in" && action != "pause" && action != "revoke" {
		return ClientWakeReview{}, errors.New("closed Windows wake action required")
	}
	control, err := readWindowsWakeControl(state.Binding)
	if err != nil {
		return ClientWakeReview{}, err
	}
	spec, err := state.Binding.WindowsSpec()
	if err != nil || state.BindingSHA256 != control.BindingSHA256 {
		return ClientWakeReview{}, errors.New("exact Windows wake binding required")
	}
	// Required review inputs are original local authority only. Native state is
	// inspected after the durable veto closes, never before it during apply.
	review := ClientWakeReview{Action: action, Status: windowsWakeStatus(state, control, nil), WindowsDefinition: &spec, Effects: []string{"advance_exact_owned_native_definition_revision_and_persist_disabled_before_OS_access", "change_only_exact_owned_logged_in_current_user_TaskScheduler_pulse", "open_only_after_exact_enabled_readback_and_live_current_authority", "keep_all_original_job_grants_qualifications_and_cadences_separate", "native_acknowledgement_is_not_job_execution"}}
	body, err := json.Marshal(review)
	if err != nil {
		return review, err
	}
	review.ReviewDigest = fmt.Sprintf("%x", sha256.Sum256(body))
	return review, nil
}
func applyWindowsClientWake(ctx context.Context, o installer.ClientOptions, action, digest string) (ClientWakeStatus, error) {
	state, err := installer.ReadMaintenanceWake(ctx, o)
	if err != nil {
		return ClientWakeStatus{}, err
	}
	review, err := reviewWindowsWakeState(ctx, state, action)
	if err != nil || review.ReviewDigest != digest {
		return ClientWakeStatus{}, errors.New("native wake review changed; preserve state and review again")
	}
	control := *review.Status.WindowsControl
	var native *windowsadapter.NativeWakeStatus
	started := false
	err = installtx.GuardQualifiedCurrent(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now(), func() error {
		current, e := installer.ReadMaintenanceWake(ctx, o)
		if e != nil {
			return e
		}
		fresh, e := reviewWindowsWakeState(ctx, current, action)
		if e != nil || fresh.ReviewDigest != digest {
			return errors.New("Windows wake current review changed before effect")
		}
		// The only native implementation is the original bounded COM adapter.
		recheck := func() error {
			again, e := installer.ReadMaintenanceWake(ctx, o)
			if e != nil || again.ClientRecordSHA != current.ClientRecordSHA || again.BindingSHA256 != current.BindingSHA256 || again.CurrentManifest != current.CurrentManifest {
				return errors.New("Windows wake current authority changed")
			}
			return ctx.Err()
		}
		started = true
		control, native, e = mutateWindowsWakeDefinition(ctx, current.Binding, control.Revision, action, windowsadapter.NativeWake, recheck)
		return e
	})
	status := windowsWakeStatus(state, control, native)
	if err != nil {
		if started {
			status.State = "partial_os_acknowledgement_unknown"
		} else {
			status.State = "wake_operation_not_applied"
		}
	}
	return status, err
}

// Caller holds the original installation guard for the complete bounded
// transaction. The injected function is a private test seam, never MCP input.
func mutateWindowsWakeDefinition(ctx context.Context, binding installer.MaintenanceWakeBinding, revision uint64, action string, native func(context.Context, windowsadapter.NativeWakeSpec, string) (windowsadapter.NativeWakeStatus, error), recheck func() error) (installer.MaintenanceWakeControl, *windowsadapter.NativeWakeStatus, error) {
	if action != "opt-in" && action != "pause" && action != "revoke" || native == nil || recheck == nil {
		return installer.MaintenanceWakeControl{}, nil, errors.New("closed original Windows wake transaction required")
	}
	disabled, err := binding.WriteWindowsWakeControl(ctx, revision, false)
	if err != nil {
		return disabled, nil, err
	}
	if err := ctx.Err(); err != nil {
		return disabled, nil, err
	}
	spec, err := binding.WindowsSpec()
	if err != nil {
		return disabled, nil, err
	}
	observed, err := native(ctx, spec, action)
	if err != nil {
		return disabled, nil, err
	}
	if observed.Name != spec.Name || observed.BindingSHA256 != spec.BindingSHA256 {
		return disabled, &observed, errors.New("exact native wake acknowledgement required")
	}
	if err := ctx.Err(); err != nil {
		return disabled, &observed, err
	}
	if action != "opt-in" {
		if observed.Enabled {
			return disabled, &observed, errors.New("native pause acknowledgement unavailable")
		}
		return disabled, &observed, nil
	}
	if !observed.Present || !observed.Enabled || observed.State != "active" {
		return disabled, &observed, errors.New("native opt-in acknowledgement unavailable")
	}
	if err := recheck(); err != nil {
		return disabled, &observed, err
	}
	enabled, err := binding.WriteWindowsWakeControl(ctx, disabled.Revision, true)
	if err != nil {
		return disabled, &observed, err
	}
	return enabled, &observed, nil
}
