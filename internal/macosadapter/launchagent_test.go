package macosadapter

import (
	"os"
	"path/filepath"
	"testing"
)

func testSpec(t *testing.T) Spec {
	t.Helper()
	home := t.TempDir()
	return Spec{Label: "com.bcg.maestro.maintenance", Program: "/usr/local/bin/bcgos", Arguments: []string{"maintenance", "wake", "--trigger", "presence"}, StartInterval: 900, RunAtLoad: true, StandardOutPath: filepath.Join(home, "Library", "Logs", "stdout.log"), StandardErrPath: filepath.Join(home, "Library", "Logs", "stderr.log")}
}

func TestLaunchAgentRenderIsConcreteAndParseable(t *testing.T) {
	body, err := Render(testSpec(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := Parse(body); err != nil {
		t.Fatal(err)
	}
	if string(body) == "" || string(body) == "{{BCGOS_BIN}}" {
		t.Fatal("render did not produce a concrete plist")
	}
}

func TestLaunchAgentInstallRequiresOptInAndSupportsLifecycle(t *testing.T) {
	home := t.TempDir()
	spec := testSpec(t)
	if _, err := Install(home, spec, false); err != ErrOptInRequired {
		t.Fatalf("without opt-in err=%v", err)
	}
	status, err := Install(home, spec, true)
	if err != nil || status.State != "adapter_installed_native_qualification_pending" || status.Disabled {
		t.Fatalf("install status=%#v err=%v", status, err)
	}
	status, err = Pause(home, spec.Label)
	if err != nil || !status.Disabled {
		t.Fatalf("pause status=%#v err=%v", status, err)
	}
	status, err = Resume(home, spec.Label)
	if err != nil || status.Disabled {
		t.Fatalf("resume status=%#v err=%v", status, err)
	}
	if err := Uninstall(home, spec.Label); err != nil {
		t.Fatal(err)
	}
	status, err = ReadStatus(home, spec.Label)
	if err != nil || status.State != "not_installed" {
		t.Fatalf("uninstall status=%#v err=%v", status, err)
	}
	if _, err := os.Stat(filepath.Join(home, "Library", "LaunchAgents", spec.Label+".plist")); !os.IsNotExist(err) {
		t.Fatalf("plist remains: %v", err)
	}
}

func TestLaunchAgentRejectsRelativeOrInterpolatedValues(t *testing.T) {
	spec := testSpec(t)
	spec.Program = "bcgos"
	if _, err := Render(spec); err == nil {
		t.Fatal("relative program accepted")
	}
	spec = testSpec(t)
	spec.Arguments[0] = "{{command}}"
	if _, err := Render(spec); err == nil {
		t.Fatal("interpolated argument accepted")
	}
}
