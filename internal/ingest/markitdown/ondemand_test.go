package markitdown

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func currentOnDemandReceipt() OnDemandReceipt {
	return OnDemandReceipt{
		SchemaVersion:     onDemandSchemaVersion,
		MarkItDownVersion: PinnedMarkItDownVersion,
		PythonVersion:     PinnedPythonVersion,
		Platform:          runtime.GOOS,
		CreatedAt:         "2026-08-13T00:00:00Z",
	}
}

func writeOnDemandReceipt(t *testing.T, root string, receipt OnDemandReceipt) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(OnDemandReceiptPath(root)), 0o700); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(OnDemandReceiptPath(root), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveOnDemandPackReportsUnavailableWhenNotCreated(t *testing.T) {
	pack, err := ResolveOnDemandPack(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if pack.State != "unavailable" || !strings.Contains(pack.Reason, "confirm") {
		t.Fatalf("pack = %+v", pack)
	}
}

func TestResolveOnDemandPackReportsUnavailableWhenOutOfDate(t *testing.T) {
	root := t.TempDir()
	receipt := currentOnDemandReceipt()
	receipt.MarkItDownVersion = "0.0.1"
	writeOnDemandReceipt(t, root, receipt)

	pack, err := ResolveOnDemandPack(root)
	if err != nil {
		t.Fatal(err)
	}
	if pack.State != "unavailable" || !strings.Contains(pack.Reason, "out of date") {
		t.Fatalf("pack = %+v", pack)
	}
}

func TestResolveOnDemandPackReportsUnavailableWhenInterpreterMissing(t *testing.T) {
	root := t.TempDir()
	writeOnDemandReceipt(t, root, currentOnDemandReceipt())

	pack, err := ResolveOnDemandPack(root)
	if err != nil {
		t.Fatal(err)
	}
	if pack.State != "unavailable" || !strings.Contains(pack.Reason, "interpreter") {
		t.Fatalf("pack = %+v", pack)
	}
}

func TestResolveOnDemandPackReportsUnavailableWhenScriptTampered(t *testing.T) {
	root := t.TempDir()
	writeOnDemandReceipt(t, root, currentOnDemandReceipt())
	if err := os.MkdirAll(filepath.Dir(onDemandVenvPython(root)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(onDemandVenvPython(root), []byte("python"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(onDemandAdapterScriptPath(root), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}

	pack, err := ResolveOnDemandPack(root)
	if err != nil {
		t.Fatal(err)
	}
	if pack.State != "unavailable" || !strings.Contains(pack.Reason, "modified") {
		t.Fatalf("pack = %+v", pack)
	}
}

func TestResolveOnDemandPackReady(t *testing.T) {
	root := t.TempDir()
	writeOnDemandReceipt(t, root, currentOnDemandReceipt())
	if err := os.MkdirAll(filepath.Dir(onDemandVenvPython(root)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(onDemandVenvPython(root), []byte("python"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(onDemandAdapterScriptPath(root), onDemandAdapterScript, 0o600); err != nil {
		t.Fatal(err)
	}

	pack, err := ResolveOnDemandPack(root)
	if err != nil {
		t.Fatal(err)
	}
	if pack.State != "ready" || len(pack.Command) != 3 || pack.Command[2] != "--request-stdin" {
		t.Fatalf("pack = %+v", pack)
	}
	if pack.Command[0] != onDemandVenvPython(root) || pack.Command[1] != onDemandAdapterScriptPath(root) {
		t.Fatalf("pack command = %+v", pack.Command)
	}
}

func TestCreateOnDemandEnvRejectsElevatedProcess(t *testing.T) {
	originalElevated := ensureNotElevated
	ensureNotElevated = func() error { return errors.New("elevated process") }
	defer func() { ensureNotElevated = originalElevated }()

	_, err := CreateOnDemandEnv(t.TempDir(), func(string, []string) error {
		t.Fatal("must not run any command when the process is elevated")
		return nil
	})
	if err == nil {
		t.Fatal("expected an error for an elevated process")
	}
}

func TestCreateOnDemandEnvRequiresUV(t *testing.T) {
	originalElevated := ensureNotElevated
	ensureNotElevated = func() error { return nil }
	defer func() { ensureNotElevated = originalElevated }()
	originalLookPath := lookPath
	lookPath = func(string) (string, error) { return "", errors.New("not found") }
	defer func() { lookPath = originalLookPath }()

	_, err := CreateOnDemandEnv(t.TempDir(), func(string, []string) error {
		t.Fatal("must not run any command when uv is unavailable")
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "uv") {
		t.Fatalf("err = %v", err)
	}
}

func TestCreateOnDemandEnvSuccessWritesReceiptAndBecomesReady(t *testing.T) {
	originalElevated := ensureNotElevated
	ensureNotElevated = func() error { return nil }
	defer func() { ensureNotElevated = originalElevated }()
	originalLookPath := lookPath
	lookPath = func(string) (string, error) { return "/fake/uv", nil }
	defer func() { lookPath = originalLookPath }()

	root := t.TempDir()
	var calls [][]string
	run := func(name string, args []string) error {
		if name != "/fake/uv" {
			t.Fatalf("unexpected command %q", name)
		}
		calls = append(calls, args)
		if len(args) > 0 && args[0] == "venv" {
			if err := os.MkdirAll(filepath.Dir(onDemandVenvPython(root)), 0o700); err != nil {
				return err
			}
			return os.WriteFile(onDemandVenvPython(root), []byte("python"), 0o700)
		}
		return nil
	}

	receipt, err := CreateOnDemandEnv(root, run)
	if err != nil {
		t.Fatal(err)
	}
	if receipt.MarkItDownVersion != PinnedMarkItDownVersion || receipt.PythonVersion != PinnedPythonVersion {
		t.Fatalf("receipt = %+v", receipt)
	}
	if len(calls) != 2 || calls[0][0] != "venv" || calls[1][0] != "pip" {
		t.Fatalf("calls = %+v", calls)
	}

	pack, err := ResolveOnDemandPack(root)
	if err != nil {
		t.Fatal(err)
	}
	if pack.State != "ready" {
		t.Fatalf("pack after create = %+v", pack)
	}
}

func TestCreateOnDemandEnvRequiresDataRoot(t *testing.T) {
	if _, err := CreateOnDemandEnv("", func(string, []string) error {
		t.Fatal("must not run any command without a data root")
		return nil
	}); err == nil {
		t.Fatal("expected an error for a missing data root")
	}
}

func TestUvInstallCommandWindowsUsesOfficialPowerShellInstaller(t *testing.T) {
	name, args := uvInstallCommand("windows")
	if name != "powershell" {
		t.Fatalf("name = %q", name)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "https://astral.sh/uv/install.ps1") {
		t.Fatalf("args = %+v", args)
	}
}

func TestUvInstallCommandUnixUsesOfficialShInstaller(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		name, args := uvInstallCommand(goos)
		if name != "sh" {
			t.Fatalf("goos %q: name = %q", goos, name)
		}
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "https://astral.sh/uv/install.sh") {
			t.Fatalf("goos %q: args = %+v", goos, args)
		}
	}
}

func TestInstallUVRejectsElevatedProcess(t *testing.T) {
	originalElevated := ensureNotElevated
	ensureNotElevated = func() error { return errors.New("elevated process") }
	defer func() { ensureNotElevated = originalElevated }()

	err := InstallUV(func(string, []string) error {
		t.Fatal("must not run any command when the process is elevated")
		return nil
	})
	if err == nil {
		t.Fatal("expected an error for an elevated process")
	}
}

func TestInstallUVRunsThePlatformOfficialInstaller(t *testing.T) {
	originalElevated := ensureNotElevated
	ensureNotElevated = func() error { return nil }
	defer func() { ensureNotElevated = originalElevated }()

	wantName, wantArgs := uvInstallCommand(runtime.GOOS)
	var gotName string
	var gotArgs []string
	err := InstallUV(func(name string, args []string) error {
		gotName = name
		gotArgs = args
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotName != wantName || strings.Join(gotArgs, " ") != strings.Join(wantArgs, " ") {
		t.Fatalf("got %q %+v, want %q %+v", gotName, gotArgs, wantName, wantArgs)
	}
}

func TestInstallUVPropagatesInstallerFailure(t *testing.T) {
	originalElevated := ensureNotElevated
	ensureNotElevated = func() error { return nil }
	defer func() { ensureNotElevated = originalElevated }()

	err := InstallUV(func(string, []string) error { return errors.New("network unreachable") })
	if err == nil || !strings.Contains(err.Error(), "install uv") {
		t.Fatalf("err = %v", err)
	}
}
