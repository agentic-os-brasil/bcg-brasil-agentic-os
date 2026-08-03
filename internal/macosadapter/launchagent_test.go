package macosadapter

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func testSpec(t *testing.T) Spec {
	t.Helper()
	return Spec{Label: "com.bcg.maestro.maintenance", Program: "/usr/local/bin/bcgos", Arguments: []string{"maintenance", "wake", "--trigger", "presence"}, StartInterval: 900, RunAtLoad: true, StandardOutPath: "/Users/test/Library/Logs/stdout.log", StandardErrPath: "/Users/test/Library/Logs/stderr.log"}
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

func TestLaunchAgentInstallIsIdempotentAndVerifyBindsTheExactSpec(t *testing.T) {
	home := t.TempDir()
	spec := testSpec(t)
	spec.Arguments = append(spec.Arguments, "--workspace", "0123456789abcdef0123456789abcdef")
	first, err := Install(home, spec, true)
	if err != nil {
		t.Fatal(err)
	}
	firstBody, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Install(home, spec, true)
	if err != nil {
		t.Fatal(err)
	}
	secondBody, err := os.ReadFile(second.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstBody) != string(secondBody) {
		t.Fatal("idempotent install changed the rendered plist")
	}
	verified, err := Verify(home, spec)
	if err != nil || verified.Path != first.Path {
		t.Fatalf("verify status=%#v err=%v", verified, err)
	}
	tampered := spec
	tampered.Arguments = append([]string(nil), spec.Arguments...)
	tampered.Arguments[len(tampered.Arguments)-1] = "fedcba9876543210fedcba9876543210"
	if _, err := Verify(home, tampered); err == nil || !strings.Contains(err.Error(), "binding") {
		t.Fatalf("tampered binding err=%v", err)
	}
}

func TestLaunchAgentInstallRejectsExistingPlistSymlink(t *testing.T) {
	home := t.TempDir()
	directory, err := UserLaunchAgentsPath(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "target.plist")
	if err := os.WriteFile(target, []byte("do not replace"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, testSpec(t).Label+".plist")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := Install(home, testSpec(t), true); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("plist symlink err=%v", err)
	}
	body, err := os.ReadFile(target)
	if err != nil || string(body) != "do not replace" {
		t.Fatalf("symlink target changed: body=%q err=%v", body, err)
	}
}

func TestResolveExecutableRejectsSymlinkAndNonExecutableFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("LaunchAgent executable validation uses Darwin path semantics")
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := ResolveExecutable(executable)
	if err != nil || !filepath.IsAbs(resolved) {
		t.Fatalf("resolved=%q err=%v", resolved, err)
	}
	symlink := filepath.Join(t.TempDir(), "bcgos-link")
	if err := os.Symlink(executable, symlink); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveExecutable(symlink); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("executable symlink err=%v", err)
	}
	nonExecutable := filepath.Join(t.TempDir(), "bcgos")
	if err := os.WriteFile(nonExecutable, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveExecutable(nonExecutable); err == nil || !strings.Contains(err.Error(), "executable") {
		t.Fatalf("non-executable file err=%v", err)
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
	spec = testSpec(t)
	spec.Program = `\usr\local\bin\bcgos`
	if _, err := Render(spec); err == nil {
		t.Fatal("Windows-rooted Darwin program accepted")
	}
	spec = testSpec(t)
	spec.StandardOutPath = `\Users\test\Library\Logs\stdout.log`
	if _, err := Render(spec); err == nil {
		t.Fatal("Windows-rooted Darwin diagnostic path accepted")
	}
}
