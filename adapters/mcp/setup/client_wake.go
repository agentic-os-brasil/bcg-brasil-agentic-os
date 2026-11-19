package setup

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/macosadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/windowsadapter"
)

type ClientWakeStatus struct {
	Platform       string                            `json:"platform"`
	State          string                            `json:"state"`
	Binding        installer.MaintenanceWakeState    `json:"binding"`
	Native         *macosadapter.LaunchAgentStatus   `json:"native,omitempty"`
	WindowsNative  *windowsadapter.NativeWakeStatus  `json:"windows_native,omitempty"`
	WindowsControl *installer.MaintenanceWakeControl `json:"windows_control,omitempty"`
	JobAuthority   string                            `json:"job_authority"`
}

type ClientWakeReview struct {
	Action            string                         `json:"action"`
	Status            ClientWakeStatus               `json:"status"`
	OSHome            string                         `json:"os_home"`
	Definition        macosadapter.Spec              `json:"definition"`
	WindowsDefinition *windowsadapter.NativeWakeSpec `json:"windows_definition,omitempty"`
	Effects           []string                       `json:"effects"`
	ReviewDigest      string                         `json:"review_digest"`
}

func InspectClientWake(ctx context.Context, o installer.ClientOptions) (ClientWakeStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	state, err := installer.ReadMaintenanceWake(ctx, o)
	if err != nil {
		return ClientWakeStatus{}, err
	}
	status := ClientWakeStatus{Platform: runtime.GOOS, Binding: state, JobAuthority: "separate_original_job_qualification_and_enrollment_required"}
	if runtime.GOOS == "windows" {
		control, err := readWindowsWakeControl(state.Binding)
		if err != nil {
			return status, err
		}
		spec, err := state.Binding.WindowsSpec()
		if err != nil {
			return status, err
		}
		observed, err := windowsadapter.NativeWake(ctx, spec, "inspect")
		status = windowsWakeStatus(state, control, &observed)
		return status, err
	}
	if runtime.GOOS != "darwin" {
		status.State = "unavailable_native_qualification_pending"
		return status, nil
	}
	lifecycle, err := macosadapter.NewCurrentLifecycle("", macosadapter.ExecCommandRunner{})
	if err != nil {
		return status, err
	}
	native, err := inspectOwnedWake(ctx, lifecycle, lifecycle.CurrentHome, state.Binding)
	status.Native = &native
	status.State = native.State
	return status, err
}

func ReviewClientWake(ctx context.Context, o installer.ClientOptions, action string) (ClientWakeReview, error) {
	if action != "opt-in" && action != "pause" && action != "revoke" {
		return ClientWakeReview{}, errors.New("choose wake opt-in, pause or revoke")
	}
	if runtime.GOOS == "windows" {
		state, err := installer.ReadMaintenanceWake(ctx, o)
		if err != nil {
			return ClientWakeReview{}, err
		}
		return reviewWindowsWakeState(ctx, state, action)
	}
	state, err := installer.ReadMaintenanceWake(ctx, o)
	if err != nil {
		return ClientWakeReview{}, err
	}

	if runtime.GOOS != "darwin" {
		return ClientWakeReview{}, errors.New("native wake enrollment is not qualified on this platform")
	}
	lifecycle, err := macosadapter.NewCurrentLifecycle("", macosadapter.ExecCommandRunner{})
	if err != nil {
		return ClientWakeReview{}, err
	}
	spec, err := state.Binding.MacSpec()
	if err != nil {
		return ClientWakeReview{}, err
	}
	local, err := macosadapter.ReadOwnedWakeFile(lifecycle.CurrentHome, spec)
	if err != nil {
		return ClientWakeReview{}, err
	}
	status := ClientWakeStatus{Platform: runtime.GOOS, State: local.State, Binding: state, Native: &local, JobAuthority: "separate_original_job_qualification_and_enrollment_required"}
	review := ClientWakeReview{Action: action, Status: status, OSHome: lifecycle.CurrentHome, Definition: spec, Effects: []string{"close_exact_owned_definition_before_native_access", "open_only_after_exact_enabled_readback_and_live_current_authority", "change_only_exact_owned_per_user_discovery_pulse", "keep_all_job_grants_schedules_and_domain_state_unchanged", "one_shot_original_due_worker_requires_native_idle_and_current_job_authority", "wake_acknowledgement_is_not_a_domain_commit"}}
	body, err := json.Marshal(review)
	if err != nil {
		return review, err
	}
	review.ReviewDigest = fmt.Sprintf("%x", sha256.Sum256(body))
	return review, nil
}

func ApplyClientWake(ctx context.Context, o installer.ClientOptions, action, digest string, confirmed bool) (ClientWakeStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if !confirmed || digest == "" {
		return ClientWakeStatus{}, errors.New("exact native wake review and explicit participant opt-in required")
	}
	if runtime.GOOS == "windows" {
		return applyWindowsClientWake(ctx, o, action, digest)
	}
	attempted := false
	var observed macosadapter.LaunchAgentStatus
	review, err := ReviewClientWake(ctx, o, action)
	if err != nil || review.ReviewDigest != digest {
		return ClientWakeStatus{}, errors.New("native wake review changed; preserve state and review again")
	}
	// This bounded native lifecycle transaction never waits for a domain worker
	// or browser. The worker releases admission locks before running RunDue.
	err = installtx.GuardQualifiedCurrent(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now(), func() error {
		current, err := installer.ReadMaintenanceWake(ctx, o)
		if err != nil || current.ClientRecordSHA != review.Status.Binding.ClientRecordSHA || current.BindingSHA256 != review.Status.Binding.BindingSHA256 || current.CurrentManifest != review.Status.Binding.CurrentManifest {
			return errors.New("native wake current selection changed before effect")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		fresh, err := ReviewClientWake(ctx, o, action)
		if err != nil || fresh.ReviewDigest != digest {
			return errors.New("Mac wake current review changed before effect")
		}

		lifecycle, err := macosadapter.NewCurrentLifecycle("", macosadapter.ExecCommandRunner{})
		if err != nil {
			return err
		}
		attempted = true
		observed, err = mutateMacWakeDefinition(ctx, lifecycle, review.OSHome, current.Binding, action, func() error {
			again, err := installer.ReadMaintenanceWake(ctx, o)
			if err != nil || again.ClientRecordSHA != current.ClientRecordSHA || again.BindingSHA256 != current.BindingSHA256 || again.CurrentManifest != current.CurrentManifest {
				return errors.New("Mac wake current authority changed")
			}
			return ctx.Err()
		})
		return err
	})
	if err != nil {
		state := "wake_operation_not_applied"
		if attempted {
			state = "partial_os_acknowledgement_unknown"
		}
		return ClientWakeStatus{Platform: runtime.GOOS, State: state, Binding: review.Status.Binding, JobAuthority: "unchanged_separate_job_authority"}, err
	}
	return ClientWakeStatus{Platform: runtime.GOOS, State: observed.State, Binding: review.Status.Binding, Native: &observed, JobAuthority: review.Status.JobAuthority}, nil
}

func mutateMacWakeDefinition(ctx context.Context, lifecycle macosadapter.Lifecycle, osHome string, binding installer.MaintenanceWakeBinding, action string, recheck func() error) (macosadapter.LaunchAgentStatus, error) {
	spec, err := binding.MacSpec()
	if err != nil {
		return macosadapter.LaunchAgentStatus{}, err
	}
	return lifecycle.ApplyOwnedWake(ctx, osHome, spec, action, recheck)
}

func inspectOwnedWake(ctx context.Context, lifecycle macosadapter.Lifecycle, osHome string, binding installer.MaintenanceWakeBinding) (macosadapter.LaunchAgentStatus, error) {
	spec, err := binding.MacSpec()
	if err != nil {
		return macosadapter.LaunchAgentStatus{}, err
	}
	return lifecycle.InspectOwnedWake(ctx, osHome, spec)
}
