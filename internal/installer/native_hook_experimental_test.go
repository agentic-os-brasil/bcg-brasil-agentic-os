//go:build maestro_experimental_installed_seed

package installer

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

// These real early returns need distinct opt-in causes before a native client
// retry can tell invalid selection from independently rejected seed admission.
func TestExperimentalNativeHookEarlyAdmissionReportsClosedCause(t *testing.T) {
	t.Setenv("MAESTRO_HOOK_DIAGNOSTICS", "1")
	for _, test := range []struct {
		name, cause string
		options     ClientOptions
	}{
		{name: "selection", cause: "selection"},
		{name: "seed", cause: "seed_admission", options: ClientOptions{
			SeededOperationOptions: SeededOperationOptions{ExperimentalSeed: &VerifiedExperimentalSeed{}},
			Request:                ClientRequest{CarrierID: strings.Repeat("a", 32), Workspace: "/synthetic/Developer/workspace"},
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			now := time.Unix(0, 0)
			finish := nativeHookDiagnostic("parent_admission", true, &output, func() time.Time { return now })
			launch, err := PrepareStableNativeHook(context.Background(), test.options, "UserPromptSubmit", strings.Repeat("b", 64))
			if err == nil || launch.Executable != "" || len(launch.Args) != 0 {
				t.Fatal("missing selection or seed admission supplied a launch")
			}
			finish(err)
			if want := "outcome=unavailable cause=" + test.cause + "\n"; !strings.HasSuffix(output.String(), want) {
				t.Fatalf("early refusal lost its closed diagnostic cause: %q", output.String())
			}
		})
	}
}

func TestExperimentalNativeHookGuardCauseRequiresExactContendedLock(t *testing.T) {
	t.Setenv("MAESTRO_HOOK_DIAGNOSTICS", "1")
	root := t.TempDir()
	lock := filepath.Join(root, ".activation.lock")
	if err := os.WriteFile(lock, []byte("synthetic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	file, collision := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if file != nil {
		file.Close()
		t.Fatal("exclusive lock did not collide")
	}
	if !errors.Is(collision, os.ErrExist) {
		t.Fatal("fixture did not produce a real exclusive-open collision", collision)
	}
	for _, test := range []struct {
		name, cause string
		err         error
	}{
		{"exact_collision", "activation_busy", fmt.Errorf("PRIVATE: %w", collision)},
		{"bare_exists", "current_source", os.ErrExist},
		{"another_file", "current_source", &os.PathError{Op: "open", Path: filepath.Join(root, "other"), Err: os.ErrExist}},
		{"another_operation", "current_source", &os.PathError{Op: "stat", Path: lock, Err: os.ErrExist}},
		{"permission", "current_source", &os.PathError{Op: "open", Path: lock, Err: os.ErrPermission}},
		{"same_text", "current_source", errors.New(collision.Error())},
		{"callback_cause", "binding_inspection", withNativeHookAdmissionCause("binding_inspection", collision)},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := nativeHookCurrentGuardFailure(root, test.err)
			var admission *nativeHookAdmissionError
			if !errors.As(err, &admission) || admission.cause != test.cause || !errors.Is(err, test.err) || err.Error() != test.err.Error() {
				t.Fatal("guard failure acquired a wrong cause or changed its original error", err)
			}
		})
	}
	t.Setenv("MAESTRO_HOOK_DIAGNOSTICS", "")
	if nativeHookCurrentGuardFailure(root, collision) != collision {
		t.Fatal("disabled guard diagnosis changed error identity")
	}
}

func TestExperimentalNativeHookOwnerRequiresIndependentAdmission(t *testing.T) {
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStart", "SubagentStop", "Stop", "foreign"} {
		if _, err := PrepareStableNativeHook(context.Background(), ClientOptions{}, event, strings.Repeat("a", 64)); err == nil {
			t.Fatal("missing independent seed admitted")
		}
	}
	if PassiveLifecycleEvent("PreToolUse") || PassiveLifecycleEvent("SubagentStart") {
		t.Fatal("native owner widened passive lifecycle")
	}
}

func TestExperimentalNativeHookInspectionCannotSupplyMissingAdmission(t *testing.T) {
	called := false
	launch, err := InspectStableNativeHook(context.Background(), ClientOptions{}, "PreToolUse", strings.Repeat("a", 64), func(QualifiedLaunch) error {
		called = true
		return nil
	})
	if err == nil || called || launch.Executable != "" || len(launch.Args) != 0 {
		t.Fatalf("inspection supplied missing native/current admission: called=%v launch=%+v err=%v", called, launch, err)
	}
	if _, err := InspectStableNativeHook(context.Background(), ClientOptions{}, "PreToolUse", strings.Repeat("a", 64), nil); err == nil {
		t.Fatal("missing read-only inspection admitted")
	}
}

func TestQualifiedLaunchAssemblyRetainsExactPins(t *testing.T) {
	// Pure assembly only. Synthetic state here grants no admission, native
	// qualification or installed execution; both public owners retain those.
	root := t.TempDir()
	options := QualifiedLaunchOptions{SeededOperationOptions: SeededOperationOptions{ManagedRoot: root, UserHome: filepath.Join(root, "owner")}, ExpectedBinding: installtx.ReleaseBinding{ManifestSHA256: strings.Repeat("a", 64), ClientInputsSHA256: strings.Repeat("b", 64)}, Workspace: filepath.Join(root, "workspace")}
	state := installtx.State{TargetOS: runtime.GOOS, Profile: releasecontract.ModernProfile, Binding: options.ExpectedBinding}
	launch, err := qualifiedLaunchFromState(options, state)
	want := []string{"serve", "--product-root", root, "--user-home", options.UserHome, "--installed-manifest", options.ExpectedBinding.ManifestSHA256, "--installed-client-inputs", options.ExpectedBinding.ClientInputsSHA256, "--workspace", options.Workspace}
	if err != nil || !reflect.DeepEqual(launch.Args, want) {
		t.Fatalf("qualified assembly changed: %+v %v", launch, err)
	}
	for _, pins := range []installtx.ReleaseBinding{{}, {ManifestSHA256: options.ExpectedBinding.ManifestSHA256}, {ClientInputsSHA256: options.ExpectedBinding.ClientInputsSHA256}, {ManifestSHA256: strings.Repeat("c", 64), ClientInputsSHA256: options.ExpectedBinding.ClientInputsSHA256}, {ManifestSHA256: options.ExpectedBinding.ManifestSHA256, ClientInputsSHA256: strings.Repeat("c", 64)}} {
		changed := options
		changed.ExpectedBinding = pins
		if _, err := qualifiedLaunchFromState(changed, state); err == nil {
			t.Fatal("pure assembly accepted incomplete or changed current-record pins")
		}
	}
}

func TestExperimentalNativeHookPureArgvPreservesQualifiedReleaseAndRole(t *testing.T) {
	// Pure argv composition only: no record, NativeHost or qualification is
	// constructed, and no executable is launched by this fixture.
	tail := []string{"--product-root", "/synthetic/Developer/managed", "--installed-manifest", strings.Repeat("a", 64), "--installed-client-inputs", strings.Repeat("b", 64), "--experimental-seed-sha256", strings.Repeat("c", 64), "--experimental-installed-root", "/synthetic/Developer/managed"}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStart", "SubagentStop", "Stop"} {
		launch, err := nativeHookLaunch(QualifiedLaunch{Executable: "synthetic-current-MCP", Args: append([]string{"serve"}, tail...)}, event, strings.Repeat("d", 64), strings.Repeat("e", 32))
		if err != nil || launch.Executable != "synthetic-current-MCP" || !reflect.DeepEqual(launch.Args[7:], tail) || launch.Args[0] != "installed-native-hook" || launch.Args[2] != event || launch.Args[6] != strings.Repeat("e", 32) {
			t.Fatalf("qualified argv changed: %+v %v", launch, err)
		}
	}
	for _, test := range []struct {
		event, selection string
		args             []string
	}{{"foreign", strings.Repeat("d", 64), []string{"serve"}}, {"Stop", "invalid", []string{"serve"}}, {"Stop", strings.Repeat("d", 64), []string{"other"}}} {
		if _, err := nativeHookLaunch(QualifiedLaunch{Args: test.args}, test.event, test.selection, strings.Repeat("e", 32)); err == nil {
			t.Fatal("noncanonical native launch admitted")
		}
	}
}
