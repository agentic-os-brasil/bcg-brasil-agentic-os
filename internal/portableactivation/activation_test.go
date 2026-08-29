package portableactivation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestActivateVerifiesInstalledCLIAndIsIdempotent(t *testing.T) {
	managedRoot, dataRoot, cliPath := activationFixture(t)
	first, err := Activate(Options{ManagedRoot: managedRoot, DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Activate(Options{ManagedRoot: managedRoot, DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	if first.State != "activated" || second.State != "already_ready" || first.CLISHA256 != second.CLISHA256 {
		t.Fatalf("unexpected receipts: first=%+v second=%+v", first, second)
	}
	if first.CLISHA256 != digestFile(t, cliPath) {
		t.Fatal("activation receipt does not bind the installed CLI")
	}
}

func TestConcurrentActivationSerializesToOneStableState(t *testing.T) {
	managedRoot, dataRoot, _ := activationFixture(t)
	var wait sync.WaitGroup
	receipts := make(chan Receipt, 2)
	errorsSeen := make(chan error, 2)
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			receipt, err := Activate(Options{ManagedRoot: managedRoot, DataRoot: dataRoot})
			receipts <- receipt
			errorsSeen <- err
		}()
	}
	wait.Wait()
	close(receipts)
	close(errorsSeen)
	for err := range errorsSeen {
		if err != nil {
			t.Fatal(err)
		}
	}
	states := map[string]int{}
	for receipt := range receipts {
		states[receipt.State]++
	}
	if states["activated"] != 1 || states["already_ready"] != 1 {
		t.Fatalf("concurrent activation states = %v", states)
	}
	var active state
	if err := readStrictJSON(filepath.Join(dataRoot, StateRelativePath), &active); err != nil || active.Version != "0.1.12" {
		t.Fatalf("active state = %+v, %v", active, err)
	}
}

func TestActivateRejectsTamperedCLIWithoutPublishingState(t *testing.T) {
	managedRoot, dataRoot, cliPath := activationFixture(t)
	if err := os.WriteFile(cliPath, []byte("tampered"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Activate(Options{ManagedRoot: managedRoot, DataRoot: dataRoot}); err == nil {
		t.Fatal("Activate() accepted tampered CLI")
	}
	if _, err := os.Stat(filepath.Join(dataRoot, StateRelativePath)); !os.IsNotExist(err) {
		t.Fatalf("activation state exists after tamper failure: %v", err)
	}
}

func TestActivateRejectsSymlinkedCLIParentWithoutPublishingState(t *testing.T) {
	managedRoot, dataRoot, cliPath := activationFixture(t)
	binRoot := filepath.Dir(cliPath)
	if err := os.Remove(cliPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(binRoot); err != nil {
		t.Fatal(err)
	}
	outsideBin := filepath.Join(t.TempDir(), "outside-bin")
	if err := os.MkdirAll(outsideBin, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outsideBin, filepath.Base(cliPath)), []byte("verified CLI"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideBin, binRoot); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if _, err := Activate(Options{ManagedRoot: managedRoot, DataRoot: dataRoot}); err == nil || !strings.Contains(err.Error(), "crosses a symlink") {
		t.Fatalf("Activate() error = %v, want CLI parent symlink refusal", err)
	}
	if _, err := os.Stat(filepath.Join(dataRoot, StateRelativePath)); !os.IsNotExist(err) {
		t.Fatalf("activation state was written after symlink refusal: %v", err)
	}
}

func TestVerifyRejectsTamperAfterSuccessfulActivation(t *testing.T) {
	managedRoot, dataRoot, cliPath := activationFixture(t)
	if _, err := Activate(Options{ManagedRoot: managedRoot, DataRoot: dataRoot}); err != nil {
		t.Fatal(err)
	}
	if receipt, err := Verify(Options{ManagedRoot: managedRoot, DataRoot: dataRoot}); err != nil || receipt.State != "ready" {
		t.Fatalf("Verify() = %+v, %v", receipt, err)
	}
	if err := os.WriteFile(cliPath, []byte("tampered after activation"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(Options{ManagedRoot: managedRoot, DataRoot: dataRoot}); err == nil {
		t.Fatal("Verify() accepted CLI tampering after activation")
	}
}

func TestActivateRejectsBootstrapVersionMismatchWithoutPublishingState(t *testing.T) {
	managedRoot, dataRoot, _ := activationFixture(t)
	if _, err := Activate(Options{ManagedRoot: managedRoot, DataRoot: dataRoot, ExpectedVersion: "9.9.9"}); err == nil {
		t.Fatal("Activate() accepted a bootstrapper version mismatch")
	}
	if _, err := os.Stat(filepath.Join(dataRoot, StateRelativePath)); !os.IsNotExist(err) {
		t.Fatalf("activation state exists after version mismatch: %v", err)
	}
}

func TestActivateUpdatesVersionAtomicallyAndPreservesPreviousState(t *testing.T) {
	managedRoot, dataRoot, cliPath := activationFixture(t)
	if _, err := Activate(Options{ManagedRoot: managedRoot, DataRoot: dataRoot}); err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := readStrictJSON(filepath.Join(managedRoot, ManifestFileName), &manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte("verified CLI v2"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest.Version = "0.1.13"
	manifest.CLISHA256 = digestFile(t, cliPath)
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(managedRoot, ManifestFileName), append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	receipt, err := Activate(Options{ManagedRoot: managedRoot, DataRoot: dataRoot})
	if err != nil || receipt.State != "updated" || receipt.Version != "0.1.13" {
		t.Fatalf("updated activation = %+v, %v", receipt, err)
	}
	var previous state
	if err := readStrictJSON(filepath.Join(dataRoot, PreviousStateRelativePath), &previous); err != nil {
		t.Fatal(err)
	}
	if previous.Version != "0.1.12" {
		t.Fatalf("previous activation version = %s", previous.Version)
	}
}

func TestActivateRejectsChangedBytesAtSameVersionAndPreservesActiveState(t *testing.T) {
	managedRoot, dataRoot, cliPath := activationFixture(t)
	if _, err := Activate(Options{ManagedRoot: managedRoot, DataRoot: dataRoot}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dataRoot, StateRelativePath))
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := readStrictJSON(filepath.Join(managedRoot, ManifestFileName), &manifest); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte("different same-version CLI"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest.CLISHA256 = digestFile(t, cliPath)
	body, _ := json.MarshalIndent(manifest, "", "  ")
	if err := os.WriteFile(filepath.Join(managedRoot, ManifestFileName), append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Activate(Options{ManagedRoot: managedRoot, DataRoot: dataRoot}); err == nil {
		t.Fatal("Activate() accepted changed CLI bytes at the same version")
	}
	after, err := os.ReadFile(filepath.Join(dataRoot, StateRelativePath))
	if err != nil || string(after) != string(before) {
		t.Fatal("failed replacement changed the active installation state")
	}
}

func activationFixture(t *testing.T) (string, string, string) {
	t.Helper()
	root := t.TempDir()
	managedRoot := filepath.Join(root, "managed")
	dataRoot := filepath.Join(root, "data")
	cliName := "bcgos"
	if runtime.GOOS == "windows" {
		cliName += ".exe"
	}
	cliPath := filepath.Join(managedRoot, "bin", cliName)
	if err := os.MkdirAll(filepath.Dir(cliPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cliPath, []byte("verified CLI"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest := Manifest{
		SchemaVersion: 1,
		Version:       "0.1.12",
		TargetOS:      runtime.GOOS,
		TargetArch:    runtime.GOARCH,
		CLIPath:       filepath.ToSlash(filepath.Join("bin", cliName)),
		CLISHA256:     digestFile(t, cliPath),
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(managedRoot, ManifestFileName), append(body, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	return managedRoot, dataRoot, cliPath
}

func digestFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
