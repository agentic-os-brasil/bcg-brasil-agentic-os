package installtx

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

func TestPrepareAndActivateKeepsManagedAndOwnerDataSeparate(t *testing.T) {
	releaseDirectory := t.TempDir()
	writeTestFile(t, releaseDirectory, "bcgos_0.1.0_darwin_arm64", "binary 0.1.0")
	writeTestBundle(t, filepath.Join(releaseDirectory, "maestro-base_0.1.0.tar.gz"), map[string]string{
		"skills/example/SKILL.md": "managed",
	})
	verified := releaseverify.VerifiedRelease{
		Directory: releaseDirectory,
		Manifest:  testManifest("0.1.0"),
	}
	managedRoot := filepath.Join(t.TempDir(), "managed")
	dataRoot := filepath.Join(t.TempDir(), "owner-data")
	ownerFile := filepath.Join(dataRoot, "workspaces", "case", "notes.md")
	writeTestFile(t, "", ownerFile, "owner")

	planPath, err := Prepare(verified, PrepareOptions{
		TargetOS: "darwin", TargetArch: "arm64", ManagedRoot: managedRoot, DataRoot: dataRoot,
	})
	if err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	checker := func(path, version string) error {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), version) {
			return os.ErrInvalid
		}
		return nil
	}
	if err := Activate(planPath, ActivateOptions{CheckCLI: checker}); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	state, err := ReadState(dataRoot)
	if err != nil {
		t.Fatalf("ReadState() error = %v", err)
	}
	if state.Release != "0.1.0" || state.BundleVersion != "0.1.0" {
		t.Fatalf("unexpected state: %#v", state)
	}
	if body, err := os.ReadFile(ownerFile); err != nil || string(body) != "owner" {
		t.Fatalf("owner data changed: body=%q err=%v", body, err)
	}
}

func TestActivateRestoresLastKnownGoodWhenSelfCheckFails(t *testing.T) {
	managedRoot := filepath.Join(t.TempDir(), "managed")
	dataRoot := filepath.Join(t.TempDir(), "data")
	firstPlan := stagedPlan(t, managedRoot, dataRoot, "0.1.0", "binary 0.1.0")
	checker := func(path, version string) error {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), version) || strings.Contains(string(body), "broken") {
			return os.ErrInvalid
		}
		return nil
	}
	if err := Activate(firstPlan, ActivateOptions{CheckCLI: checker}); err != nil {
		t.Fatal(err)
	}
	secondPlan := stagedPlan(t, managedRoot, dataRoot, "0.1.1", "broken 0.1.1")
	if err := Activate(secondPlan, ActivateOptions{CheckCLI: checker}); err == nil {
		t.Fatal("Activate() accepted a failing self-check")
	}
	state, err := ReadState(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if state.Release != "0.1.0" {
		t.Fatalf("state release = %s, want 0.1.0", state.Release)
	}
	active, err := os.ReadFile(filepath.Join(managedRoot, "bin", executableName("darwin")))
	if err != nil || !strings.Contains(string(active), "0.1.0") {
		t.Fatalf("last-known-good CLI was not restored: body=%q err=%v", active, err)
	}
}

func TestRollbackReactivatesPreviousVerifiedVersion(t *testing.T) {
	managedRoot := filepath.Join(t.TempDir(), "managed")
	dataRoot := filepath.Join(t.TempDir(), "data")
	checker := func(path, version string) error {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if !strings.Contains(string(body), version) {
			return os.ErrInvalid
		}
		return nil
	}
	if err := Activate(stagedPlan(t, managedRoot, dataRoot, "0.1.0", "binary 0.1.0"), ActivateOptions{CheckCLI: checker}); err != nil {
		t.Fatal(err)
	}
	if err := Activate(stagedPlan(t, managedRoot, dataRoot, "0.1.1", "binary 0.1.1"), ActivateOptions{CheckCLI: checker}); err != nil {
		t.Fatal(err)
	}
	if err := Rollback(managedRoot, dataRoot, checker); err != nil {
		t.Fatalf("Rollback() error = %v", err)
	}
	state, err := ReadState(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if state.Release != "0.1.0" || state.Previous == nil || state.Previous.Release != "0.1.1" {
		t.Fatalf("unexpected rolled-back state: %#v", state)
	}
}

func TestExtractBundleRejectsTraversal(t *testing.T) {
	var archive bytes.Buffer
	gzipWriter := gzip.NewWriter(&archive)
	tarWriter := tar.NewWriter(gzipWriter)
	body := []byte("escape")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "../escape", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := extractBundle(bytes.NewReader(archive.Bytes()), t.TempDir()); err == nil {
		t.Fatal("extractBundle() accepted traversal")
	}
}

func stagedPlan(t *testing.T, managedRoot, dataRoot, version, cliBody string) string {
	t.Helper()
	transaction := filepath.Join(dataRoot, "updates", "tx-"+strings.ReplaceAll(version, ".", "-"))
	writeTestFile(t, transaction, "bin/"+executableName("darwin"), cliBody)
	writeTestFile(t, transaction, "bundle/skills/example/SKILL.md", version)
	plan := ActivationPlan{
		SchemaVersion: 1, TransactionID: filepath.Base(transaction), Release: version, Channel: "canary",
		CLIVersion: version, BundleVersion: version, TargetOS: "darwin", TargetArch: "arm64",
		ManagedRoot: managedRoot, DataRoot: dataRoot,
		StagedCLI:    filepath.Join(transaction, "bin", executableName("darwin")),
		StagedBundle: filepath.Join(transaction, "bundle"),
	}
	path := filepath.Join(transaction, PlanName)
	if err := WritePlan(path, plan); err != nil {
		t.Fatal(err)
	}
	return path
}

func testManifest(version string) releasecontract.Manifest {
	return releasecontract.Manifest{
		SchemaVersion: 1, Product: "maestro", Release: version, Channel: "canary",
		Issuer: releasecontract.Issuer{ID: "maestro-release", KeyID: "pilot-2026"},
		CLI:    releasecontract.CLIComponent{Version: version, CompatibleBundle: ">=" + version + " <0.1.1"},
		Bundle: releasecontract.BundleComponent{Version: version, CompatibleCLI: ">=" + version + " <0.1.1"},
		Artifacts: []releasecontract.Artifact{
			{Kind: "cli", OS: "darwin", Arch: "arm64", Name: "bcgos_" + version + "_darwin_arm64"},
			{Kind: "bundle", OS: "any", Arch: "any", Name: "maestro-base_" + version + ".tar.gz"},
		},
		Migrations: []releasecontract.Migration{},
	}
}

func writeTestBundle(t *testing.T, path string, files map[string]string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for name, content := range files {
		body := []byte(content)
		if err := tarWriter.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatal(err)
		}
		if _, err := tarWriter.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeTestFile(t *testing.T, root, relative, body string) {
	t.Helper()
	path := relative
	if root != "" {
		path = filepath.Join(root, filepath.FromSlash(relative))
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}
