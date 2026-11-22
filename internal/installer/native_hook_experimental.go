//go:build maestro_experimental_installed_seed

package installer

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
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
	if o.ExperimentalSeed == nil || !experimentalNativeHookEvent(event) || !clientSHA.MatchString(selection) || !carrierIDPattern.MatchString(o.Request.CarrierID) || o.Request.Workspace == "" {
		return QualifiedLaunch{}, errors.New("fixed experimental native hook selection required")
	}
	if _, err := admitOperationSeed(ctx, o.SeededOperationOptions); err != nil {
		return QualifiedLaunch{}, err
	}
	var launch QualifiedLaunch
	err := installtx.GuardQualifiedCurrentStateContext(ctx, o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now(), func(state installtx.State, finalCheck func(installtx.ReleaseBinding, time.Time) error) error {
		record, recordBody, err := readClientRecord(o.SeededOperationOptions, o.Request.CarrierID)
		if err != nil {
			return err
		}
		status, err := inspectClientRecordAtCurrent(ctx, o, record, state)
		if err != nil {
			return err
		}
		if status.State != "current_materialization" || status.Record == nil {
			return errors.New("current native client materialization required")
		}
		r := *status.Record
		if !r.Materialization.NativeEnforcement || r.Materialization.LifecycleContext || r.Binding.Workspace != o.Request.Workspace {
			return errors.New("genuinely published full native carrier required; passive profiles are insufficient")
		}
		// Equality with the existing native selection fingerprint, not a proof
		// importer: all four fields come from the authenticated current record.
		body, err := json.Marshal([]string{r.Binding.UserHome, r.Binding.Workspace, r.Binding.Home, r.Binding.WorkspaceID})
		if err != nil || fmt.Sprintf("%x", sha256.Sum256(body)) != selection {
			return errors.New("native selected owner/workspace changed")
		}
		expected := installtx.ReleaseBinding{ManifestSHA256: r.Materialization.ManifestSHA256, ClientInputsSHA256: r.Materialization.ClientInputsSHA256}
		// Packet hashing may cross source/authority expiry. Keep the late authenticated
		// time/eligibility and exact-record fence inside this same installation guard.
		_, currentRecordBody, err := readClientRecord(o.SeededOperationOptions, o.Request.CarrierID)
		if err != nil || clientDigest(currentRecordBody) != clientDigest(recordBody) {
			return errors.New("native client record changed during inspection")
		}
		if err := finalCheck(expected, time.Now()); err != nil {
			return err
		}
		if err := recheckExperimentalOperationContext(ctx, o.SeededOperationOptions); err != nil {
			return err
		}
		launch, err = qualifiedLaunchFromState(QualifiedLaunchOptions{SeededOperationOptions: o.SeededOperationOptions, ExpectedBinding: expected, Workspace: r.Binding.Workspace}, state)
		if err != nil {
			return err
		}
		launch, err = nativeHookLaunch(launch, event, selection, o.Request.CarrierID)
		return err
	})
	if err == nil {
		err = recheckExperimentalOperationContext(ctx, o.SeededOperationOptions)
	}
	return launch, err
}

func nativeHookLaunch(launch QualifiedLaunch, event, selection, carrier string) (QualifiedLaunch, error) {
	if !experimentalNativeHookEvent(event) || !clientSHA.MatchString(selection) || !carrierIDPattern.MatchString(carrier) || len(launch.Args) == 0 || launch.Args[0] != "serve" {
		return QualifiedLaunch{}, errors.New("canonical qualified native launch required")
	}
	launch.Args = append([]string{"installed-native-hook", "--event", event, "--binding-sha256", selection, "--carrier-id", carrier}, launch.Args[1:]...)
	return launch, nil
}
