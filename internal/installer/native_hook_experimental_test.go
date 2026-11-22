//go:build maestro_experimental_installed_seed

package installer

import (
	"context"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

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
