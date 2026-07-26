package installtx

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
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
	verified := verifiedTestRelease(t, releaseDirectory, "0.1.0")
	managedRoot := filepath.Join(t.TempDir(), "managed")
	dataRoot := filepath.Join(t.TempDir(), "owner-data")
	ownerFile := filepath.Join(dataRoot, "workspaces", "case", "notes.md")
	writeTestFile(t, "", ownerFile, "owner")

	planPath, err := Prepare(verified, PrepareOptions{
		Transition: "install",
		TargetOS:   "darwin", TargetArch: "arm64", ManagedRoot: managedRoot, DataRoot: dataRoot,
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
	if err := Activate(planPath, verified, ActivateOptions{
		PrepareOptions: PrepareOptions{
			Transition: "install",
			TargetOS:   "darwin", TargetArch: "arm64", ManagedRoot: managedRoot, DataRoot: dataRoot,
		},
		CheckCLI: checker,
	}); err != nil {
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
	firstPlan := stagedPlan(t, managedRoot, dataRoot, "", "0.1.0", "binary 0.1.0")
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
	if err := activateTestPlan(t, firstPlan, checker); err != nil {
		t.Fatal(err)
	}
	secondPlan := stagedPlan(t, managedRoot, dataRoot, "0.1.0", "0.1.1", "broken 0.1.1")
	if err := activateTestPlan(t, secondPlan, checker); err == nil {
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
	if err := activateTestPlan(t, stagedPlan(t, managedRoot, dataRoot, "", "0.1.0", "binary 0.1.0"), checker); err != nil {
		t.Fatal(err)
	}
	if err := activateTestPlan(t, stagedPlan(t, managedRoot, dataRoot, "0.1.0", "0.1.1", "binary 0.1.1"), checker); err != nil {
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

func TestSchemaV1StateMigratesBeforeUpdateAndRollback(t *testing.T) {
	managedRoot := filepath.Join(t.TempDir(), "managed")
	dataRoot := filepath.Join(t.TempDir(), "data")
	activeCLI := filepath.Join(managedRoot, "bin", executableName("darwin"))
	writeTestFile(t, "", activeCLI, "binary 0.1.0")
	oldState := `{
  "schema_version": 1,
  "release": "0.1.0",
  "channel": "canary",
  "cli_version": "0.1.0",
  "bundle_version": "0.1.0",
  "target_os": "darwin",
  "target_arch": "arm64",
  "activated_at": "2026-07-01T00:00:00Z"
}
`
	writeTestFile(t, "", statePath(dataRoot), oldState)
	if _, err := ReadState(dataRoot); err == nil {
		t.Fatal("ReadState() migrated schema v1 without an authoritative managed root")
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
	if err := activateTestPlan(
		t,
		stagedPlan(t, managedRoot, dataRoot, "0.1.0", "0.2.0", "binary 0.2.0"),
		checker,
	); err != nil {
		t.Fatalf("Activate() from schema v1 error = %v", err)
	}
	state, err := ReadState(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != 2 || state.ManagedRoot != managedRoot || state.Previous == nil {
		t.Fatalf("schema v1 state was not migrated canonically: %#v", state)
	}
	if err := Rollback(managedRoot, dataRoot, checker); err != nil {
		t.Fatalf("Rollback() after schema migration error = %v", err)
	}
	state, err = ReadState(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if state.SchemaVersion != 2 || state.Release != "0.1.0" {
		t.Fatalf("unexpected post-migration rollback state: %#v", state)
	}
}

func TestActivateRejectsMutatedPreparedArtifacts(t *testing.T) {
	tests := map[string]func(*testing.T, ActivationPlan){
		"CLI": func(t *testing.T, plan ActivationPlan) {
			if err := os.WriteFile(plan.StagedCLI, []byte("tampered"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"bundle": func(t *testing.T, plan ActivationPlan) {
			if err := os.WriteFile(plan.StagedBundleArchive, []byte("tampered"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			managedRoot := filepath.Join(t.TempDir(), "managed")
			dataRoot := filepath.Join(t.TempDir(), "data")
			planPath := stagedPlan(t, managedRoot, dataRoot, "", "0.2.0", "binary 0.2.0")
			plan, err := ReadPlan(planPath)
			if err != nil {
				t.Fatal(err)
			}
			verified := verifiedForPlan(plan)
			mutate(t, plan)
			if err := Activate(planPath, verified, ActivateOptions{
				PrepareOptions: PrepareOptions{
					Transition: plan.Transition, ConfirmationPlanID: plan.ConfirmationPlanID,
					FromRelease: plan.FromRelease, FromChannel: plan.FromChannel,
					FromCLIVersion: plan.FromCLIVersion, FromBundleVersion: plan.FromBundleVersion,
					TargetOS: plan.TargetOS, TargetArch: plan.TargetArch,
					ManagedRoot: plan.ManagedRoot, DataRoot: plan.DataRoot,
				},
				CheckCLI: func(_, _ string) error { return nil },
			}); err == nil {
				t.Fatal("Activate() accepted mutated prepared bytes")
			}
			if _, err := os.Stat(filepath.Join(managedRoot, "bin", executableName("darwin"))); !os.IsNotExist(err) {
				t.Fatalf("mutated activation changed the active CLI: %v", err)
			}
		})
	}
}

func TestReconcileCompletesCrashAfterPayloadBeforeStateCommit(t *testing.T) {
	const helperEnv = "MAESTRO_ACTIVATION_CRASH_HELPER"
	const planEnv = "MAESTRO_ACTIVATION_CRASH_PLAN"
	if os.Getenv(helperEnv) == "1" {
		planPath := os.Getenv(planEnv)
		plan, err := ReadPlan(planPath)
		if err != nil {
			os.Exit(92)
		}
		err = Activate(planPath, verifiedForPlan(plan), ActivateOptions{
			PrepareOptions: optionsForPlan(plan),
			CheckCLI:       func(_, _ string) error { return nil },
			afterPayload:   func() { os.Exit(91) },
		})
		if err != nil {
			os.Exit(93)
		}
		os.Exit(94)
	}

	managedRoot := filepath.Join(t.TempDir(), "managed")
	dataRoot := filepath.Join(t.TempDir(), "data")
	if err := activateTestPlan(
		t,
		stagedPlan(t, managedRoot, dataRoot, "", "0.1.0", "binary 0.1.0"),
		func(_, _ string) error { return nil },
	); err != nil {
		t.Fatal(err)
	}
	updatePlanPath := stagedPlan(
		t,
		managedRoot,
		dataRoot,
		"0.1.0",
		"0.2.0",
		"binary 0.2.0",
	)
	command := exec.Command(os.Args[0], "-test.run=^TestReconcileCompletesCrashAfterPayloadBeforeStateCommit$")
	command.Env = append(os.Environ(), helperEnv+"=1", planEnv+"="+updatePlanPath)
	err := command.Run()
	exitError, ok := err.(*exec.ExitError)
	if !ok || exitError.ExitCode() != 91 {
		t.Fatalf("activation helper exit = %v, want simulated crash 91", err)
	}
	state, err := ReadState(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if state.Release != "0.1.0" {
		t.Fatalf("simulated pre-state crash changed durable state: %#v", state)
	}
	plan, err := ReadPlan(updatePlanPath)
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := ReconcileActivated(updatePlanPath, ActivateOptions{
		PrepareOptions: optionsForPlan(plan),
		CheckCLI:       func(_, _ string) error { return nil },
	})
	if err != nil {
		t.Fatalf("ReconcileActivated() error = %v", err)
	}
	if !reconciled {
		t.Fatal("ReconcileActivated() did not complete the interrupted activation")
	}
	state, err = ReadState(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if state.Release != "0.2.0" || state.Previous == nil || state.Previous.Release != "0.1.0" {
		t.Fatalf("unexpected reconciled state: %#v", state)
	}
}

func TestReconcileRestoresCrashBeforeFailedSelfCheck(t *testing.T) {
	const helperEnv = "MAESTRO_PRE_SELFCHECK_CRASH_HELPER"
	const planEnv = "MAESTRO_PRE_SELFCHECK_CRASH_PLAN"
	if os.Getenv(helperEnv) == "1" {
		planPath := os.Getenv(planEnv)
		plan, err := ReadPlan(planPath)
		if err != nil {
			os.Exit(96)
		}
		err = Activate(planPath, verifiedForPlan(plan), ActivateOptions{
			PrepareOptions:  optionsForPlan(plan),
			CheckCLI:        func(_, _ string) error { return os.ErrInvalid },
			beforeSelfCheck: func() { os.Exit(95) },
		})
		if err != nil {
			os.Exit(97)
		}
		os.Exit(98)
	}

	managedRoot := filepath.Join(t.TempDir(), "managed")
	dataRoot := filepath.Join(t.TempDir(), "data")
	if err := activateTestPlan(
		t,
		stagedPlan(t, managedRoot, dataRoot, "", "0.1.0", "binary 0.1.0"),
		func(_, _ string) error { return nil },
	); err != nil {
		t.Fatal(err)
	}
	updatePlanPath := stagedPlan(
		t,
		managedRoot,
		dataRoot,
		"0.1.0",
		"0.2.0",
		"binary 0.2.0",
	)
	command := exec.Command(os.Args[0], "-test.run=^TestReconcileRestoresCrashBeforeFailedSelfCheck$")
	command.Env = append(os.Environ(), helperEnv+"=1", planEnv+"="+updatePlanPath)
	err := command.Run()
	exitError, ok := err.(*exec.ExitError)
	if !ok || exitError.ExitCode() != 95 {
		t.Fatalf("activation helper exit = %v, want simulated crash 95", err)
	}

	plan, err := ReadPlan(updatePlanPath)
	if err != nil {
		t.Fatal(err)
	}
	reconciled, err := ReconcileActivated(updatePlanPath, ActivateOptions{
		PrepareOptions: optionsForPlan(plan),
		CheckCLI:       func(_, _ string) error { return os.ErrInvalid },
	})
	if err != nil {
		t.Fatalf("ReconcileActivated() error = %v", err)
	}
	if reconciled {
		t.Fatal("ReconcileActivated() committed a CLI that failed its self-check")
	}
	state, err := ReadState(dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if state.Release != "0.1.0" {
		t.Fatalf("failed self-check changed durable state: %#v", state)
	}
	activeCLI := filepath.Join(managedRoot, "bin", executableName("darwin"))
	body, err := os.ReadFile(activeCLI)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("0.1.0")) {
		t.Fatalf("failed self-check did not restore the source CLI: %q", body)
	}
	if _, err := os.Stat(filepath.Join(managedRoot, "bundles", "0.2.0")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed self-check left target bundle active: %v", err)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(updatePlanPath), ReceiptName)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed self-check left activation receipt behind: %v", err)
	}
}

func TestActivateRejectsPlanAndArtifactRebindingAfterConfirmation(t *testing.T) {
	managedRoot := filepath.Join(t.TempDir(), "managed")
	dataRoot := filepath.Join(t.TempDir(), "data")
	planPath := stagedPlan(t, managedRoot, dataRoot, "", "0.2.0", "binary 0.2.0")
	plan, err := ReadPlan(planPath)
	if err != nil {
		t.Fatal(err)
	}
	verified := verifiedForPlan(plan)
	if err := os.WriteFile(plan.StagedCLI, []byte("attacker replacement"), 0o755); err != nil {
		t.Fatal(err)
	}
	plan.CLISize, plan.CLISHA256 = testFileIdentity(t, plan.StagedCLI)
	if err := WritePlan(planPath, plan); err != nil {
		t.Fatal(err)
	}
	if err := Activate(planPath, verified, ActivateOptions{
		PrepareOptions: PrepareOptions{
			Transition: plan.Transition, ConfirmationPlanID: plan.ConfirmationPlanID,
			FromRelease: plan.FromRelease, FromChannel: plan.FromChannel,
			FromCLIVersion: plan.FromCLIVersion, FromBundleVersion: plan.FromBundleVersion,
			TargetOS: plan.TargetOS, TargetArch: plan.TargetArch,
			ManagedRoot: plan.ManagedRoot, DataRoot: plan.DataRoot,
		},
		CheckCLI: func(_, _ string) error { return nil },
	}); err == nil {
		t.Fatal("Activate() accepted a locally rebound plan and staged artifact")
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

func stagedPlan(t *testing.T, managedRoot, dataRoot, fromVersion, version, cliBody string) string {
	t.Helper()
	transaction := filepath.Join(dataRoot, "updates", "tx-"+strings.ReplaceAll(version, ".", "-"))
	stagedCLI := filepath.Join(transaction, "bin", executableName("darwin"))
	writeTestFile(t, "", stagedCLI, cliBody)
	stagedBundleArchive := filepath.Join(transaction, "bundle.tar.gz")
	writeTestBundle(t, stagedBundleArchive, map[string]string{"skills/example/SKILL.md": version})
	cliSize, cliSHA256 := testFileIdentity(t, stagedCLI)
	bundleSize, bundleSHA256 := testFileIdentity(t, stagedBundleArchive)
	plan := ActivationPlan{
		SchemaVersion: 2, TransactionID: filepath.Base(transaction), Release: version, Channel: "canary",
		CLIVersion: version, BundleVersion: version, TargetOS: "darwin", TargetArch: "arm64",
		ManagedRoot: managedRoot, DataRoot: dataRoot, ManifestSHA256: strings.Repeat("a", 64),
		CLIArtifactName: "bcgos_" + version + "_darwin_arm64", CLISHA256: cliSHA256, CLISize: cliSize,
		BundleArtifactName: "maestro-base_" + version + ".tar.gz", BundleSHA256: bundleSHA256, BundleSize: bundleSize,
		StagedCLI: stagedCLI, StagedBundleArchive: stagedBundleArchive,
	}
	if fromVersion == "" {
		plan.Transition = "install"
	} else {
		plan.Transition = "update"
		plan.ConfirmationPlanID = strings.Repeat("b", 32)
		plan.FromRelease = fromVersion
		plan.FromChannel = "canary"
		plan.FromCLIVersion = fromVersion
		plan.FromBundleVersion = fromVersion
	}
	path := filepath.Join(transaction, PlanName)
	if err := WritePlan(path, plan); err != nil {
		t.Fatal(err)
	}
	return path
}

func activateTestPlan(
	t *testing.T,
	planPath string,
	checker func(path, version string) error,
) error {
	t.Helper()
	plan, err := ReadPlan(planPath)
	if err != nil {
		return err
	}
	return Activate(planPath, verifiedForPlan(plan), ActivateOptions{
		PrepareOptions: optionsForPlan(plan),
		CheckCLI:       checker,
	})
}

func optionsForPlan(plan ActivationPlan) PrepareOptions {
	return PrepareOptions{
		Transition: plan.Transition, ConfirmationPlanID: plan.ConfirmationPlanID,
		FromRelease: plan.FromRelease, FromChannel: plan.FromChannel,
		FromCLIVersion: plan.FromCLIVersion, FromBundleVersion: plan.FromBundleVersion,
		TargetOS: plan.TargetOS, TargetArch: plan.TargetArch,
		ManagedRoot: plan.ManagedRoot, DataRoot: plan.DataRoot,
	}
}

func verifiedForPlan(plan ActivationPlan) releaseverify.VerifiedRelease {
	manifest := testManifest(plan.Release)
	manifest.Channel = plan.Channel
	manifest.CLI.Version = plan.CLIVersion
	manifest.Bundle.Version = plan.BundleVersion
	manifest.Artifacts[0].Name = plan.CLIArtifactName
	manifest.Artifacts[0].Size = plan.CLISize
	manifest.Artifacts[0].SHA256 = plan.CLISHA256
	manifest.Artifacts[1].Name = plan.BundleArtifactName
	manifest.Artifacts[1].Size = plan.BundleSize
	manifest.Artifacts[1].SHA256 = plan.BundleSHA256
	return releaseverify.VerifiedRelease{
		Manifest: manifest, ManifestSHA256: plan.ManifestSHA256,
	}
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

func verifiedTestRelease(t *testing.T, directory, version string) releaseverify.VerifiedRelease {
	t.Helper()
	manifest := testManifest(version)
	for index := range manifest.Artifacts {
		artifact := &manifest.Artifacts[index]
		size, digest := testFileIdentity(t, filepath.Join(directory, artifact.Name))
		artifact.Size = size
		artifact.SHA256 = digest
	}
	return releaseverify.VerifiedRelease{
		Directory: directory, Manifest: manifest, ManifestSHA256: strings.Repeat("f", 64),
	}
}

func testFileIdentity(t *testing.T, path string) (int64, string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(body)
	return int64(len(body)), hex.EncodeToString(sum[:])
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
