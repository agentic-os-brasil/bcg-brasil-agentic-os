//go:build maestro_experimental_installed_seed

package installer

import (
	"context"
	"reflect"
	"strings"
	"testing"
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

func TestExperimentalNativeHookPureArgvPreservesQualifiedReleaseAndRole(t *testing.T) {
	// Pure argv composition only: no record, NativeHost or qualification is
	// constructed, and no executable is launched by this fixture.
	tail := []string{"--product-root", "/synthetic/Developer/managed", "--installed-manifest", strings.Repeat("a", 64), "--installed-client-inputs", strings.Repeat("b", 64), "--experimental-seed-sha256", strings.Repeat("c", 64), "--experimental-installed-root", "/synthetic/Developer/managed"}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStart", "SubagentStop", "Stop"} {
		launch, err := nativeHookLaunch(QualifiedLaunch{Executable: "synthetic-current-MCP", Args: append([]string{"serve"}, tail...)}, event, strings.Repeat("d", 64))
		if err != nil || launch.Executable != "synthetic-current-MCP" || !reflect.DeepEqual(launch.Args[5:], tail) || launch.Args[0] != "hook" || launch.Args[2] != event {
			t.Fatalf("qualified argv changed: %+v %v", launch, err)
		}
	}
	for _, test := range []struct {
		event, selection string
		args             []string
	}{{"foreign", strings.Repeat("d", 64), []string{"serve"}}, {"Stop", "invalid", []string{"serve"}}, {"Stop", strings.Repeat("d", 64), []string{"other"}}} {
		if _, err := nativeHookLaunch(QualifiedLaunch{Args: test.args}, test.event, test.selection); err == nil {
			t.Fatal("noncanonical native launch admitted")
		}
	}
}
