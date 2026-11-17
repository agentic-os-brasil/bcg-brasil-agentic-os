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
	err := installtx.GuardQualifiedCurrent(o.ManagedRoot, o.DataRoot, installtx.ReleaseBinding{}, time.Now(), func() error {
		status, err := ReadClientsSeeded(ctx, o)
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
		launch, err = PrepareQualifiedLaunch(ctx, QualifiedLaunchOptions{SeededOperationOptions: o.SeededOperationOptions, ExpectedBinding: installtx.ReleaseBinding{ManifestSHA256: r.Materialization.ManifestSHA256, ClientInputsSHA256: r.Materialization.ClientInputsSHA256}, Workspace: r.Binding.Workspace})
		if err != nil {
			return err
		}
		launch, err = nativeHookLaunch(launch, event, selection)
		return err
	})
	if err == nil {
		err = recheckExperimentalOperation(o.SeededOperationOptions)
	}
	return launch, err
}

func nativeHookLaunch(launch QualifiedLaunch, event, selection string) (QualifiedLaunch, error) {
	if !experimentalNativeHookEvent(event) || !clientSHA.MatchString(selection) || len(launch.Args) == 0 || launch.Args[0] != "serve" {
		return QualifiedLaunch{}, errors.New("canonical qualified native launch required")
	}
	launch.Args = append([]string{"hook", "--event", event, "--binding-sha256", selection}, launch.Args[1:]...)
	return launch, nil
}
