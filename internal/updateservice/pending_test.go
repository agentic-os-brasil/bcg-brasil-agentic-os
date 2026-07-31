package updateservice

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateplan"
)

func TestPendingUpdateRequiresExactReverifiedPlanBeforeActivation(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "data")
	managedRoot := filepath.Join(t.TempDir(), "managed")
	current := installtx.State{
		SchemaVersion: 2, ManagedRoot: managedRoot,
		Release: "0.1.0", Channel: "canary", CLIVersion: "0.1.0", BundleVersion: "0.1.0",
		TargetOS: "darwin", TargetArch: "arm64", ActivatedAt: time.Unix(1000, 0).UTC(),
	}
	if err := installtx.WriteState(dataRoot, current); err != nil {
		t.Fatal(err)
	}
	verified, registry := pendingSignedRelease(t, dataRoot, "0.2.0")
	plan, err := updateplan.Build(
		current, verified.Manifest, "darwin", "arm64",
		updateplan.SourceBinding{
			Provider: "github", ProviderReleaseID: 42, ManifestSHA256: verified.ManifestSHA256,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	createdAt := time.Unix(2000, 0).UTC()
	pending, err := StagePending(dataRoot, current, CheckResult{Plan: plan, Verified: verified}, createdAt)
	if err != nil {
		t.Fatalf("StagePending() error = %v", err)
	}
	if pending.Plan.ID != plan.ID || pending.CreatedAt != createdAt {
		t.Fatalf("unexpected pending update: %#v", pending)
	}
	confirmed, reverified, err := ConfirmPending(dataRoot, managedRoot, plan.ID, registry)
	if err != nil {
		t.Fatalf("ConfirmPending() error = %v", err)
	}
	if confirmed.ActivationPlanPath == "" || reverified.ManifestSHA256 != plan.ManifestSHA256 {
		t.Fatalf("unexpected confirmation: %#v %#v", confirmed, reverified)
	}
	if _, _, err := ConfirmPending(dataRoot, managedRoot, strings.Repeat("0", 32), registry); err == nil {
		t.Fatal("ConfirmPending() accepted a different plan ID")
	}
	pendingFile := pendingPath(dataRoot)
	pendingBody, err := os.ReadFile(pendingFile)
	if err != nil {
		t.Fatal(err)
	}
	duplicated := bytes.Replace(
		pendingBody,
		[]byte(`"schema_version": 1,`),
		[]byte(`"schema_version": 1, "schema_version": 1,`),
		1,
	)
	if err := os.WriteFile(pendingFile, duplicated, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPending(dataRoot); err == nil {
		t.Fatal("LoadPending() accepted duplicate JSON keys")
	}
	if err := os.WriteFile(pendingFile, pendingBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RemovePending(dataRoot, plan.ID); err != nil {
		t.Fatalf("RemovePending() error = %v", err)
	}
	if _, err := LoadPending(dataRoot); !os.IsNotExist(err) {
		t.Fatalf("pending update remained after removal: %v", err)
	}
}

func TestPendingUpdateFailsClosedAfterVerifiedReleaseMutation(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "data")
	managedRoot := filepath.Join(t.TempDir(), "managed")
	current := installtx.State{
		SchemaVersion: 2, ManagedRoot: managedRoot,
		Release: "0.1.0", Channel: "canary", CLIVersion: "0.1.0", BundleVersion: "0.1.0",
		TargetOS: "darwin", TargetArch: "arm64", ActivatedAt: time.Unix(1000, 0).UTC(),
	}
	if err := installtx.WriteState(dataRoot, current); err != nil {
		t.Fatal(err)
	}
	verified, registry := pendingSignedRelease(t, dataRoot, "0.2.0")
	plan, err := updateplan.Build(
		current, verified.Manifest, "darwin", "arm64",
		updateplan.SourceBinding{Provider: "github", ProviderReleaseID: 42, ManifestSHA256: verified.ManifestSHA256},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := StagePending(dataRoot, current, CheckResult{Plan: plan, Verified: verified}, time.Unix(2000, 0).UTC()); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(verified.Directory, releaseverify.ManifestName)
	body, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	body[len(body)-2] ^= 1
	if err := os.WriteFile(manifestPath, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ConfirmPending(dataRoot, managedRoot, plan.ID, registry); err == nil {
		t.Fatal("ConfirmPending() accepted mutated signed release bytes")
	}
	if _, err := ValidatePendingLaunch(dataRoot, managedRoot, plan.ID, registry); err == nil {
		t.Fatal("ValidatePendingLaunch() accepted mutated signed release bytes")
	}
}

func TestPendingConfirmationBindsPreparedBytesAndActivationSemantics(t *testing.T) {
	tests := map[string]func(*testing.T, string, Pending){
		"staged CLI bytes": func(t *testing.T, _ string, pending Pending) {
			activation, err := installtx.ReadPlan(pending.ActivationPlanPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(activation.StagedCLI, []byte("tampered CLI"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
		"staged bundle bytes": func(t *testing.T, _ string, pending Pending) {
			activation, err := installtx.ReadPlan(pending.ActivationPlanPath)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(activation.StagedBundleArchive, []byte("tampered bundle"), 0o600); err != nil {
				t.Fatal(err)
			}
		},
		"activation channel": func(t *testing.T, _ string, pending Pending) {
			activation, err := installtx.ReadPlan(pending.ActivationPlanPath)
			if err != nil {
				t.Fatal(err)
			}
			activation.Channel = "beta"
			if err := installtx.WritePlan(pending.ActivationPlanPath, activation); err != nil {
				t.Fatal(err)
			}
		},
		"activation target": func(t *testing.T, _ string, pending Pending) {
			activation, err := installtx.ReadPlan(pending.ActivationPlanPath)
			if err != nil {
				t.Fatal(err)
			}
			activation.TargetArch = "amd64"
			if err := installtx.WritePlan(pending.ActivationPlanPath, activation); err != nil {
				t.Fatal(err)
			}
		},
		"activation data root": func(t *testing.T, dataRoot string, pending Pending) {
			activation, err := installtx.ReadPlan(pending.ActivationPlanPath)
			if err != nil {
				t.Fatal(err)
			}
			activation.DataRoot = filepath.Join(filepath.Dir(dataRoot), "different-data")
			if err := installtx.WritePlan(pending.ActivationPlanPath, activation); err != nil {
				t.Fatal(err)
			}
		},
		"pending target": func(t *testing.T, dataRoot string, pending Pending) {
			pending.Plan.TargetArch = "amd64"
			if err := writePendingAtomic(pendingPath(dataRoot), pending); err != nil {
				t.Fatal(err)
			}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			dataRoot, managedRoot, plan, pending, registry := stagedPendingFixture(t)
			mutate(t, dataRoot, pending)
			if _, _, err := ConfirmPending(dataRoot, managedRoot, plan.ID, registry); err == nil {
				t.Fatal("ConfirmPending() accepted mutated prepared update")
			}
		})
	}
}

func TestActivationRejectsInstalledStateChangedAfterConfirmation(t *testing.T) {
	dataRoot, managedRoot, plan, pending, registry := stagedPendingFixture(t)
	confirmed, verified, err := ConfirmPending(dataRoot, managedRoot, plan.ID, registry)
	if err != nil {
		t.Fatal(err)
	}
	if confirmed.Plan.ID != pending.Plan.ID {
		t.Fatalf("confirmed plan = %s, want %s", confirmed.Plan.ID, pending.Plan.ID)
	}
	activeCLI := filepath.Join(managedRoot, "bin", "bcgos")
	if err := os.MkdirAll(filepath.Dir(activeCLI), 0o755); err != nil {
		t.Fatal(err)
	}
	originalCLI := []byte("still-active 0.1.5")
	if err := os.WriteFile(activeCLI, originalCLI, 0o755); err != nil {
		t.Fatal(err)
	}
	changed := installtx.State{
		SchemaVersion: 2, ManagedRoot: managedRoot,
		Release: "0.1.5", Channel: "canary", CLIVersion: "0.1.5", BundleVersion: "0.1.5",
		TargetOS: "darwin", TargetArch: "arm64", ActivatedAt: time.Unix(2500, 0).UTC(),
	}
	if err := installtx.WriteState(dataRoot, changed); err != nil {
		t.Fatal(err)
	}
	err = installtx.Activate(confirmed.ActivationPlanPath, verified, installtx.ActivateOptions{
		PrepareOptions: installtx.PrepareOptions{
			Transition:         "update",
			ConfirmationPlanID: confirmed.Plan.ID,
			FromRelease:        confirmed.Plan.FromRelease,
			FromChannel:        confirmed.Plan.FromChannel,
			FromCLIVersion:     confirmed.Plan.FromCLIVersion,
			FromBundleVersion:  confirmed.Plan.FromBundleVersion,
			TargetOS:           confirmed.Plan.TargetOS,
			TargetArch:         confirmed.Plan.TargetArch,
			ManagedRoot:        managedRoot,
			DataRoot:           dataRoot,
		},
		CheckCLI: func(_, _ string) error { return nil },
	})
	if err == nil {
		t.Fatal("Activate() accepted an installed state changed after confirmation")
	}
	body, readErr := os.ReadFile(activeCLI)
	if readErr != nil || !bytes.Equal(body, originalCLI) {
		t.Fatalf("stale activation changed active CLI: body=%q err=%v", body, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(managedRoot, "bundles", plan.BundleVersion)); !os.IsNotExist(statErr) {
		t.Fatalf("stale activation wrote target bundle: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(managedRoot, "recovery")); !os.IsNotExist(statErr) {
		t.Fatalf("stale activation wrote recovery payload: %v", statErr)
	}
	if _, launchErr := ValidatePendingLaunch(dataRoot, managedRoot, plan.ID, registry); launchErr == nil {
		t.Fatal("ValidatePendingLaunch() accepted divergent installed state without recovery evidence")
	}
}

func TestPendingReconcilesCrashAfterStateCommitBeforeRemoval(t *testing.T) {
	dataRoot, managedRoot, plan, _, registry := stagedPendingFixture(t)
	pending, verified, err := ConfirmPending(dataRoot, managedRoot, plan.ID, registry)
	if err != nil {
		t.Fatal(err)
	}
	activeCLI := filepath.Join(managedRoot, "bin", "bcgos")
	if err := os.MkdirAll(filepath.Dir(activeCLI), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(activeCLI, []byte("bcgos 0.1.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	options := installtx.PrepareOptions{
		Transition:         "update",
		ConfirmationPlanID: pending.Plan.ID,
		FromRelease:        pending.Plan.FromRelease,
		FromChannel:        pending.Plan.FromChannel,
		FromCLIVersion:     pending.Plan.FromCLIVersion,
		FromBundleVersion:  pending.Plan.FromBundleVersion,
		TargetOS:           pending.Plan.TargetOS,
		TargetArch:         pending.Plan.TargetArch,
		ManagedRoot:        managedRoot,
		DataRoot:           dataRoot,
	}
	if err := installtx.Activate(
		pending.ActivationPlanPath,
		verified,
		installtx.ActivateOptions{
			PrepareOptions: options,
			CheckCLI:       func(_, _ string) error { return nil },
		},
	); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	// Simulate process termination after the durable state commit and before
	// RemovePending. A fresh confirmation is expected to be stale.
	if _, _, err := ConfirmPending(dataRoot, managedRoot, plan.ID, registry); err == nil {
		t.Fatal("ConfirmPending() unexpectedly rebuilt an already committed update")
	}
	launchable, err := ValidatePendingLaunch(dataRoot, managedRoot, plan.ID, registry)
	if err != nil {
		t.Fatalf("ValidatePendingLaunch() rejected committed recovery: %v", err)
	}
	if launchable.Plan.ID != plan.ID {
		t.Fatalf("launchable recovery plan = %s, want %s", launchable.Plan.ID, plan.ID)
	}
	reconciledPending, reconciled, err := ReconcilePending(
		dataRoot,
		managedRoot,
		plan.ID,
		registry,
	)
	if err != nil {
		t.Fatalf("ReconcilePending() error = %v", err)
	}
	if !reconciled || reconciledPending.Plan.ID != plan.ID {
		t.Fatalf("committed update was not reconciled: %#v %v", reconciledPending, reconciled)
	}
	if err := RemovePending(dataRoot, plan.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadPending(dataRoot); !os.IsNotExist(err) {
		t.Fatalf("reconciled pending update was not consumed: %v", err)
	}
}

func stagedPendingFixture(
	t *testing.T,
) (string, string, updateplan.Plan, Pending, releaseverify.StaticRegistry) {
	t.Helper()
	dataRoot := filepath.Join(t.TempDir(), "data")
	managedRoot := filepath.Join(t.TempDir(), "managed")
	current := installtx.State{
		SchemaVersion: 2, ManagedRoot: managedRoot,
		Release: "0.1.0", Channel: "canary", CLIVersion: "0.1.0", BundleVersion: "0.1.0",
		TargetOS: "darwin", TargetArch: "arm64", ActivatedAt: time.Unix(1000, 0).UTC(),
	}
	if err := installtx.WriteState(dataRoot, current); err != nil {
		t.Fatal(err)
	}
	verified, registry := pendingSignedRelease(t, dataRoot, "0.2.0")
	plan, err := updateplan.Build(
		current, verified.Manifest, current.TargetOS, current.TargetArch,
		updateplan.SourceBinding{
			Provider: "github", ProviderReleaseID: 42, ManifestSHA256: verified.ManifestSHA256,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := StagePending(
		dataRoot,
		current,
		CheckResult{Plan: plan, Verified: verified},
		time.Unix(2000, 0).UTC(),
	)
	if err != nil {
		t.Fatal(err)
	}
	return dataRoot, managedRoot, plan, pending, registry
}

func pendingSignedRelease(
	t *testing.T,
	dataRoot, version string,
) (releaseverify.VerifiedRelease, releaseverify.StaticRegistry) {
	t.Helper()
	directory := filepath.Join(dataRoot, "updates", "download-42")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatal(err)
	}
	cli := []byte("bcgos " + version + "\n")
	bundle := pendingBundle(t)
	notes := []byte("# Maestro " + version + "\n")
	artifacts := []struct {
		kind, osName, arch, name string
		body                     []byte
	}{
		{"cli", "darwin", "arm64", "bcgos_" + version + "_darwin_arm64", cli},
		{"bundle", "any", "any", "maestro-base_" + version + ".tar.gz", bundle},
	}
	manifest := releasecontract.Manifest{
		SchemaVersion: 1, Product: "maestro", Release: version, Channel: "canary",
		Issuer: releasecontract.Issuer{ID: "maestro-release", KeyID: "pilot-2026"},
		CLI: releasecontract.CLIComponent{
			Version: version, CompatibleBundle: ">=" + version + " <0.2.1",
		},
		Bundle: releasecontract.BundleComponent{
			Version: version, CompatibleCLI: ">=" + version + " <0.2.1",
		},
		Migrations: []releasecontract.Migration{},
	}
	for _, artifact := range artifacts {
		if err := os.WriteFile(filepath.Join(directory, artifact.name), artifact.body, 0o600); err != nil {
			t.Fatal(err)
		}
		signatureName := artifact.name + ".sig"
		if err := os.WriteFile(filepath.Join(directory, signatureName), ed25519.Sign(privateKey, artifact.body), 0o600); err != nil {
			t.Fatal(err)
		}
		manifest.Artifacts = append(manifest.Artifacts, releasecontract.Artifact{
			Kind: artifact.kind, OS: artifact.osName, Arch: artifact.arch, Name: artifact.name,
			Size: int64(len(artifact.body)), SHA256: pendingDigest(artifact.body), SignatureRef: signatureName,
		})
	}
	if version == "0.2.0" {
		manifest.Migrations = []releasecontract.Migration{{
			ID: "practice-agent-to-pa-expert", Component: "bundle", From: ">=0.1.0 <0.2.0", To: "0.2.0", Required: true,
			FromRole: "practice_agent", ToRole: "pa_expert", AliasExpiresAfter: "0.2.0",
			BundleSHA256: manifest.Artifacts[1].SHA256, CatalogSHA256: strings.Repeat("d", 64), PolicySHA256: strings.Repeat("e", 64),
		}}
	}
	if err := os.WriteFile(filepath.Join(directory, "release-notes-"+version+".md"), notes, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest.ReleaseNotes = releasecontract.ReleaseNotes{
		Name: "release-notes-" + version + ".md", SHA256: pendingDigest(notes),
	}
	manifestBody, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifestBody = append(manifestBody, '\n')
	if err := os.WriteFile(filepath.Join(directory, releaseverify.ManifestName), manifestBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(directory, releaseverify.ManifestSignatureName),
		ed25519.Sign(privateKey, manifestBody), 0o600,
	); err != nil {
		t.Fatal(err)
	}
	registry := releaseverify.StaticRegistry{"maestro/maestro-release/pilot-2026": publicKey}
	verified, err := releaseverify.VerifyDirectory(directory, registry)
	if err != nil {
		t.Fatal(err)
	}
	return verified, registry
}

func pendingBundle(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	body := []byte("managed\n")
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: "skills/example/SKILL.md", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg,
	}); err != nil {
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
	return output.Bytes()
}

func pendingDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
