//go:build maestro_experimental_installed_seed

package installer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
)

func experimentalNativeHookEvent(event string) bool {
	switch event {
	case "SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStart", "SubagentStop", "Stop":
		return true
	}
	return false
}

// PrepareStableNativeHook is separate from the passive three-event owner.
// A native record can be published only by the existing genuine qualifier.
// Reading its bit here supplies no new publication or qualification path.
func PrepareStableNativeHook(ctx context.Context, o ClientOptions, event, selection string) (QualifiedLaunch, error) {
	return prepareStableNativeHook(ctx, o, event, selection, nil)
}

// InspectStableNativeHook adds one synchronous read-only binding inspection to
// this invocation's full native/current admission. Exact-record, current/time
// and role fences run after inspection, inside the existing installation guard.
// The callback must not acquire domain locks or run a hook event. No snapshot or
// callback result supplies reusable admission after this function returns.
func InspectStableNativeHook(ctx context.Context, o ClientOptions, event, selection string, inspect func(QualifiedLaunch) error) (QualifiedLaunch, error) {
	if inspect == nil {
		return QualifiedLaunch{}, withNativeHookAdmissionCause("binding_inspection", errors.New("native binding inspection required"))
	}
	return prepareStableNativeHook(ctx, o, event, selection, inspect)
}

func prepareStableNativeHook(ctx context.Context, o ClientOptions, event, selection string, inspect func(QualifiedLaunch) error) (QualifiedLaunch, error) {
	if o.ExperimentalSeed == nil || !experimentalNativeHookEvent(event) || !clientSHA.MatchString(selection) || !carrierIDPattern.MatchString(o.Request.CarrierID) || o.Request.Workspace == "" {
		return QualifiedLaunch{}, withNativeHookAdmissionCause("selection", errors.New("fixed experimental native hook selection required"))
	}
	if _, err := admitOperationSeed(ctx, o.SeededOperationOptions); err != nil {
		return QualifiedLaunch{}, withNativeHookAdmissionCause("seed_admission", err)
	}
	var launch QualifiedLaunch
	err := installtx.GuardQualifiedCurrentStateContext(ctx, o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now(), func(state installtx.State, finalCheck func(installtx.ReleaseBinding, time.Time) error) error {
		record, recordBody, err := readClientRecord(o.SeededOperationOptions, o.Request.CarrierID)
		if err != nil {
			return withNativeHookAdmissionCause("client_binding", err)
		}
		status, err := inspectClientRecordAtCurrent(ctx, o, record, state)
		if err != nil {
			return withNativeHookAdmissionCause("client_binding", err)
		}
		if status.State != "current_materialization" || status.Record == nil {
			return withNativeHookAdmissionCause("client_binding", errors.New("current native client materialization required"))
		}
		r := *status.Record
		if !r.Materialization.NativeEnforcement || r.Materialization.LifecycleContext || r.Binding.Workspace != o.Request.Workspace {
			return withNativeHookAdmissionCause("native_selection", errors.New("genuinely published full native carrier required; passive profiles are insufficient"))
		}
		// Equality with the existing native selection fingerprint, not a proof
		// importer: all four fields come from the authenticated current record.
		body, err := json.Marshal([]string{r.Binding.UserHome, r.Binding.Workspace, r.Binding.Home, r.Binding.WorkspaceID})
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(body)) != selection {
			return withNativeHookAdmissionCause("native_selection", errors.New("native selected owner/workspace changed"))
		}
		expected := installtx.ReleaseBinding{ManifestSHA256: r.Materialization.ManifestSHA256, ClientInputsSHA256: r.Materialization.ClientInputsSHA256}
		launch, err = qualifiedLaunchFromState(QualifiedLaunchOptions{SeededOperationOptions: o.SeededOperationOptions, ExpectedBinding: expected, Workspace: r.Binding.Workspace}, state)
		if err != nil {
			return withNativeHookAdmissionCause("launch_binding", err)
		}
		launch, err = nativeHookLaunch(launch, event, selection, o.Request.CarrierID)
		if err != nil {
			return withNativeHookAdmissionCause("launch_binding", err)
		}
		if inspect != nil {
			if err := inspect(launch); err != nil {
				return withNativeHookAdmissionCause("binding_inspection", err)
			}
		}
		// Packet hashing may cross source/authority expiry. Keep the late authenticated
		// time/eligibility and exact-record fence inside this same installation guard.
		_, currentRecordBody, err := readClientRecord(o.SeededOperationOptions, o.Request.CarrierID)
		if err != nil || clientDigest(currentRecordBody) != clientDigest(recordBody) {
			return withNativeHookAdmissionCause("record_recheck", errors.New("native client record changed during inspection"))
		}
		if err := finalCheck(expected, time.Now()); err != nil {
			return withNativeHookAdmissionCause("current_recheck", err)
		}
		if err := recheckExperimentalOperationContext(ctx, o.SeededOperationOptions); err != nil {
			return withNativeHookAdmissionCause("role_recheck", err)
		}
		return withNativeHookAdmissionCause("current_recheck", ctx.Err())
	})
	if err != nil {
		return QualifiedLaunch{}, nativeHookCurrentGuardFailure(o.ManagedRoot, err)
	}
	err = withNativeHookAdmissionCause("role_recheck", recheckExperimentalOperationContext(ctx, o.SeededOperationOptions))
	if err != nil {
		return QualifiedLaunch{}, err
	}
	return launch, err
}

// Only the exact failed exclusive-open path proves lock contention. A same-text
// error, another existing file or a callback's own refusal keeps its own cause.
func nativeHookCurrentGuardFailure(managedRoot string, err error) error {
	var admission *nativeHookAdmissionError
	if errors.As(err, &admission) {
		return err
	}
	var pathError *os.PathError
	if errors.As(err, &pathError) && pathError.Op == "open" && pathError.Path == filepath.Join(managedRoot, ".activation.lock") && errors.Is(pathError.Err, os.ErrExist) {
		return withNativeHookAdmissionCause("activation_busy", err)
	}
	return withNativeHookAdmissionCause("current_source", err)
}

func nativeHookLaunch(launch QualifiedLaunch, event, selection, carrier string) (QualifiedLaunch, error) {
	if !experimentalNativeHookEvent(event) || !clientSHA.MatchString(selection) || !carrierIDPattern.MatchString(carrier) || len(launch.Args) == 0 || launch.Args[0] != "serve" {
		return QualifiedLaunch{}, errors.New("canonical qualified native launch required")
	}
	launch.Args = append([]string{"installed-native-hook", "--event", event, "--binding-sha256", selection, "--carrier-id", carrier}, launch.Args[1:]...)
	return launch, nil
}
