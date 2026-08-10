package installer

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
)

func TestValidateAuthenticodeStatusAllowsOnlyTheGovernedLocalBetaException(t *testing.T) {
	tests := []struct {
		name   string
		mode   NativeTrustMode
		status string
		ok     bool
	}{
		{name: "strict valid", mode: NativeTrustStrict, status: "Valid", ok: true},
		{name: "strict unsigned", mode: NativeTrustStrict, status: "NotSigned"},
		{name: "canary simple unsigned", mode: NativeTrustCanarySimple, status: "NotSigned", ok: true},
		{name: "canary simple valid", mode: NativeTrustCanarySimple, status: "Valid", ok: true},
		{name: "canary simple hash mismatch is diagnostic", mode: NativeTrustCanarySimple, status: "HashMismatch", ok: true},
		{name: "canary simple untrusted is diagnostic", mode: NativeTrustCanarySimple, status: "NotTrusted", ok: true},
		{name: "canary simple unknown is diagnostic", mode: NativeTrustCanarySimple, status: "UnknownError", ok: true},
		{name: "canary simple unsupported is diagnostic", mode: NativeTrustCanarySimple, status: "NotSupported", ok: true},
		{name: "local beta valid signature is outside the exact beta contract", mode: NativeTrustWindowsLocalBeta, status: "Valid"},
		{name: "local beta unsigned", mode: NativeTrustWindowsLocalBeta, status: "NotSigned", ok: true},
		{name: "local beta hash mismatch", mode: NativeTrustWindowsLocalBeta, status: "HashMismatch"},
		{name: "local beta untrusted", mode: NativeTrustWindowsLocalBeta, status: "NotTrusted"},
		{name: "local beta unknown", mode: NativeTrustWindowsLocalBeta, status: "UnknownError"},
		{name: "local beta unsupported", mode: NativeTrustWindowsLocalBeta, status: "NotSupported"},
		{name: "empty", mode: NativeTrustWindowsLocalBeta, status: ""},
		{name: "multiple lines", mode: NativeTrustWindowsLocalBeta, status: "NotSigned\nValid"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateAuthenticodeStatus([]byte(test.status), test.mode)
			if test.ok && err != nil {
				t.Fatalf("validateAuthenticodeStatus() error = %v", err)
			}
			if !test.ok && err == nil {
				t.Fatal("validateAuthenticodeStatus() accepted an unsafe status")
			}
		})
	}
}

func TestValidateNativeTrustPolicyRequiresExactWindowsLocalBetaPins(t *testing.T) {
	digestA := strings.Repeat("a", 64)
	digestB := strings.Repeat("b", 64)
	base := Options{
		TargetOS: "windows", TargetArch: "amd64", NativeTrustMode: NativeTrustWindowsLocalBeta,
		LocalBetaPins: LocalBetaPins{
			AuthorityRegistrySHA256: digestA, BootstrapperSHA256: digestB,
			Issuer: "maestro-beta-local", KeyID: "beta-20260805",
		},
	}
	verified := releaseverify.VerifiedRelease{Manifest: releasecontract.Manifest{
		Release: "0.1.21", Channel: "canary",
		Issuer: releasecontract.Issuer{ID: "maestro-beta-local", KeyID: "beta-20260805"},
	}}
	tests := []struct {
		name          string
		mutateOptions func(*Options)
		mutateRelease func(*releaseverify.VerifiedRelease)
		registry      string
		bootstrapper  string
		ok            bool
	}{
		{name: "exact", registry: digestA, bootstrapper: digestB, ok: true},
		{name: "registry mismatch", registry: strings.Repeat("c", 64), bootstrapper: digestB},
		{name: "bootstrapper mismatch", registry: digestA, bootstrapper: strings.Repeat("c", 64)},
		{name: "non windows", registry: digestA, bootstrapper: digestB, mutateOptions: func(options *Options) { options.TargetOS = "darwin" }},
		{name: "stable channel", registry: digestA, bootstrapper: digestB, mutateRelease: func(release *releaseverify.VerifiedRelease) { release.Manifest.Channel = "stable" }},
		{name: "beta channel", registry: digestA, bootstrapper: digestB, mutateRelease: func(release *releaseverify.VerifiedRelease) { release.Manifest.Channel = "beta" }},
		{name: "issuer mismatch", registry: digestA, bootstrapper: digestB, mutateRelease: func(release *releaseverify.VerifiedRelease) { release.Manifest.Issuer.ID = "other-beta" }},
		{name: "key mismatch", registry: digestA, bootstrapper: digestB, mutateRelease: func(release *releaseverify.VerifiedRelease) { release.Manifest.Issuer.KeyID = "beta-other" }},
		{name: "production identity", registry: digestA, bootstrapper: digestB, mutateOptions: func(options *Options) {
			options.LocalBetaPins.Issuer = "maestro-production"
			options.LocalBetaPins.KeyID = "release-20260805"
		}, mutateRelease: func(release *releaseverify.VerifiedRelease) {
			release.Manifest.Issuer.ID = "maestro-production"
			release.Manifest.Issuer.KeyID = "release-20260805"
		}},
		{name: "partial pins", registry: digestA, bootstrapper: digestB, mutateOptions: func(options *Options) { options.LocalBetaPins.KeyID = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			options := base
			release := verified
			if test.mutateOptions != nil {
				test.mutateOptions(&options)
			}
			if test.mutateRelease != nil {
				test.mutateRelease(&release)
			}
			err := validateNativeTrustPolicy(options, release, test.registry, test.bootstrapper)
			if test.ok && err != nil {
				t.Fatalf("validateNativeTrustPolicy() error = %v", err)
			}
			if !test.ok && err == nil {
				t.Fatal("validateNativeTrustPolicy() accepted an unsafe policy")
			}
		})
	}

	strictWithPins := base
	strictWithPins.NativeTrustMode = NativeTrustStrict
	if err := validateNativeTrustPolicy(strictWithPins, verified, digestA, digestB); err == nil {
		t.Fatal("strict mode accepted local-beta pins")
	}
}

// TestPreProductionAuthorityMarkerAcceptsCanaryFamily locks in the broader
// pre-production marker set. Two contracts:
//
//  1. Canary/dev/preview/local/rc pins pass — the local-beta trust mode does
//     not force the literal token "beta" or "test" in the issuer/keyID name.
//     A legitimate factory signing the canary channel would otherwise be
//     blocked by its own naming convention.
//  2. Real production identities still fail closed. The marker set stays
//     narrow enough that anything that looks like a shipping release
//     (`maestro-production`, `release-20260805`, bare hashes) is rejected.
func TestPreProductionAuthorityMarkerAcceptsCanaryFamily(t *testing.T) {
	digestA := strings.Repeat("a", 64)
	digestB := strings.Repeat("b", 64)
	acceptedPairs := []struct {
		issuer string
		keyID  string
	}{
		{"maestro-canary", "canary-20260810"},
		{"maestro-preview", "preview-20260810"},
		{"maestro-dev-local", "dev-20260810"},
		{"local-signing-authority", "local-20260810"},
		{"maestro-rc", "rc-1"},
	}
	for _, pair := range acceptedPairs {
		t.Run("accept/"+pair.issuer+"/"+pair.keyID, func(t *testing.T) {
			options := Options{
				TargetOS: "windows", TargetArch: "amd64", NativeTrustMode: NativeTrustWindowsLocalBeta,
				LocalBetaPins: LocalBetaPins{
					AuthorityRegistrySHA256: digestA, BootstrapperSHA256: digestB,
					Issuer: pair.issuer, KeyID: pair.keyID,
				},
			}
			release := releaseverify.VerifiedRelease{Manifest: releasecontract.Manifest{
				Release: "0.1.21", Channel: "canary",
				Issuer: releasecontract.Issuer{ID: pair.issuer, KeyID: pair.keyID},
			}}
			if err := validateNativeTrustPolicy(options, release, digestA, digestB); err != nil {
				t.Fatalf("pre-production pin was rejected: %v", err)
			}
		})
	}
	rejectedPairs := []struct {
		issuer string
		keyID  string
	}{
		{"maestro-production", "release-20260805"},
		{"maestro-signed", "release-20260805"},
		{"maestro", "20260805"},
	}
	for _, pair := range rejectedPairs {
		t.Run("reject/"+pair.issuer+"/"+pair.keyID, func(t *testing.T) {
			options := Options{
				TargetOS: "windows", TargetArch: "amd64", NativeTrustMode: NativeTrustWindowsLocalBeta,
				LocalBetaPins: LocalBetaPins{
					AuthorityRegistrySHA256: digestA, BootstrapperSHA256: digestB,
					Issuer: pair.issuer, KeyID: pair.keyID,
				},
			}
			release := releaseverify.VerifiedRelease{Manifest: releasecontract.Manifest{
				Release: "0.1.21", Channel: "canary",
				Issuer: releasecontract.Issuer{ID: pair.issuer, KeyID: pair.keyID},
			}}
			err := validateNativeTrustPolicy(options, release, digestA, digestB)
			if err == nil {
				t.Fatal("production-looking pin passed local-beta trust check")
			}
			if !strings.Contains(err.Error(), "pre-production marker") {
				t.Fatalf("error should point the operator at the marker set, got: %v", err)
			}
		})
	}
}

func TestDefaultPathsKeepManagedAndOwnerDataSeparate(t *testing.T) {
	tests := []struct {
		platform, home, local string
		managed, data         string
	}{
		{"windows", `C:\Users\pilot`, `C:\Users\pilot\AppData\Local`, `C:\Users\pilot\AppData\Local\Maestro`, `C:\Users\pilot\AppData\Local\BCGOS`},
		{"darwin", "/Users/pilot", "", "/Users/pilot/Library/Application Support/Maestro", "/Users/pilot/Library/Application Support/BCGOS"},
	}
	for _, test := range tests {
		t.Run(test.platform, func(t *testing.T) {
			paths, err := DefaultPaths(test.platform, test.home, test.local)
			if err != nil {
				t.Fatal(err)
			}
			if paths.ManagedRoot != test.managed || paths.DataRoot != test.data {
				t.Fatalf("paths = %#v, want managed=%q data=%q", paths, test.managed, test.data)
			}
		})
	}
}

func TestCanonicalInstallRootResolvesSymlinkedParent(t *testing.T) {
	root := t.TempDir()
	physical := filepath.Join(root, "physical")
	if err := os.Mkdir(physical, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(physical, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	resolved, err := canonicalInstallRoot(filepath.Join(alias, "managed"), "managed root")
	if err != nil {
		t.Fatal(err)
	}
	want, err := canonicalInstallRoot(filepath.Join(physical, "managed"), "expected root")
	if err != nil {
		t.Fatal(err)
	}
	if !samePhysicalPath(resolved, want) {
		t.Fatalf("resolved root = %q, want %q", resolved, want)
	}
}

func TestCanonicalInstallRootRejectsSymlinkedLeaf(t *testing.T) {
	root := t.TempDir()
	physical := filepath.Join(root, "physical")
	if err := os.Mkdir(physical, 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(physical, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := canonicalInstallRoot(alias, "owner-data root"); err == nil {
		t.Fatal("canonicalInstallRoot accepted a symlinked leaf")
	}
}

func TestPrepareRejectsBootstrapperNameDriftBeforeNativeCheck(t *testing.T) {
	if err := validateBootstrapperName("wrong-name", "0.1.0", "darwin", "arm64"); err == nil || !strings.Contains(err.Error(), "bootstrapper must be named") {
		t.Fatalf("validateBootstrapperName error = %v, want name rejection", err)
	}
}

func TestReadSeedStatusRejectsTrailingJSON(t *testing.T) {
	_, err := readSeedStatus(context.Background(), "ignored", func(context.Context, string, ...string) ([]byte, error) {
		return []byte(`{"schema_version":1,"product":"maestro","bootstrapper_version":"0.1.0","authority_registry_sha256":"` + strings.Repeat("a", 64) + `"} {}`), nil
	})
	if err == nil || !strings.Contains(err.Error(), "multiple JSON values") {
		t.Fatalf("readSeedStatus error = %v, want trailing JSON rejection", err)
	}
}

func TestFileSHA256(t *testing.T) {
	path := filepath.Join(t.TempDir(), "value")
	writeInstallerFile(t, path, []byte("maestro"))
	digest, err := fileSHA256(path)
	if err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256([]byte("maestro"))
	if digest != hex.EncodeToString(expected[:]) {
		t.Fatalf("digest = %s", digest)
	}
}

func TestInstallDelegatesOnlyAfterFullSeedBinding(t *testing.T) {
	root := t.TempDir()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	registryBody := []byte(fmt.Sprintf(`{"schema_version":1,"product":"maestro","authorities":[{"issuer":"release","key_id":"key-1","algorithm":"ed25519","public_key":"%s","status":"active","valid_from":"2026-01-01T00:00:00Z","valid_until":"2030-01-01T00:00:00Z"}]}`,
		base64.StdEncoding.EncodeToString(publicKey)))
	registryPath := filepath.Join(root, "registry.json")
	writeInstallerFile(t, registryPath, registryBody)
	registryDigest, err := fileSHA256(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	releaseDir := filepath.Join(root, "release")
	writeSignedReleaseFixture(t, releaseDir, privateKey)
	bootstrapper := filepath.Join(root, "bcgos-bootstrap_0.1.0_darwin_arm64")
	writeInstallerFile(t, bootstrapper, []byte("native signed bootstrapper"))
	managedRoot, err := canonicalInstallRoot(filepath.Join(root, "Maestro"), "managed root")
	if err != nil {
		t.Fatal(err)
	}
	dataRoot, err := canonicalInstallRoot(filepath.Join(root, "BCGOS"), "owner-data root")
	if err != nil {
		t.Fatal(err)
	}
	run := func(_ context.Context, path string, args ...string) ([]byte, error) {
		if strings.Contains(filepath.Base(path), "bcgos-bootstrap") && len(args) == 1 && args[0] == "seed-status" {
			return []byte(fmt.Sprintf(`{"schema_version":1,"product":"maestro","bootstrapper_version":"0.1.0","authority_registry_sha256":"%s"}`,
				registryDigest)), nil
		}
		if strings.Contains(filepath.Base(path), "bcgos-bootstrap") && len(args) > 0 && args[0] == "install" {
			if err := os.MkdirAll(filepath.Join(managedRoot, "bin"), 0o700); err != nil {
				return nil, err
			}
			if err := os.WriteFile(filepath.Join(managedRoot, "bin", "bcgos"), []byte("bcgos 0.1.0"), 0o700); err != nil {
				return nil, err
			}
			if err := os.MkdirAll(filepath.Join(managedRoot, "bundles", "0.1.0"), 0o700); err != nil {
				return nil, err
			}
			if err := installtx.WriteState(dataRoot, installedState(managedRoot)); err != nil {
				return nil, err
			}
			return []byte("Maestro installation complete"), nil
		}
		if filepath.Base(path) == "bcgos" && len(args) == 1 && args[0] == "version" {
			return []byte("bcgos 0.1.0\n"), nil
		}
		return nil, fmt.Errorf("unexpected command %s %v", path, args)
	}
	result, err := Install(context.Background(), Options{
		ReleaseDir: releaseDir, Bootstrapper: bootstrapper, AuthorityRegistry: registryPath,
		ManagedRoot: managedRoot, DataRoot: dataRoot, TargetOS: "darwin", TargetArch: "arm64",
		VerifyNative: func(context.Context, string) error { return nil }, Run: run,
	})
	if err != nil {
		t.Fatal(err)
	}
	expectedCLIPath, err := canonicalInstallRoot(filepath.Join(managedRoot, "bin", "bcgos"), "expected CLI root")
	if err != nil {
		t.Fatal(err)
	}
	if result.Release != "0.1.0" || result.CLIPath != expectedCLIPath {
		t.Fatalf("unexpected install result: %#v", result)
	}
	if _, err := os.Stat(filepath.Join(managedRoot, "trust", "release-authority-registry.json")); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareWindowsLocalBetaBindsPinsWithoutExecutingSourceSeedStatus(t *testing.T) {
	fixture := newWindowsLocalBetaFixture(t)
	seedStatusCalled := false
	options := fixture.options(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) == 1 && args[0] == "seed-status" {
			seedStatusCalled = true
		}
		return nil, fmt.Errorf("source bootstrapper must not execute during local-beta prepare")
	})
	plan, _, err := Prepare(options)
	if err != nil {
		t.Fatal(err)
	}
	if seedStatusCalled {
		t.Fatal("Prepare executed seed-status from an unsigned source bootstrapper")
	}
	if plan.NativeTrustMode != string(NativeTrustWindowsLocalBeta) ||
		plan.RegistrySHA256 != fixture.registryDigest ||
		plan.BootstrapperSHA256 != fixture.bootstrapperDigest ||
		plan.ReleaseIssuer != fixture.issuer || plan.ReleaseKeyID != fixture.authorityKey ||
		plan.BootstrapperVersion != "0.1.0" || plan.PlanDigest == "" {
		t.Fatalf("local-beta plan did not bind governed trust inputs: %#v", plan)
	}
}

func TestInstallWindowsLocalBetaRejectsBootstrapperChangedAfterPrepareBeforeExecution(t *testing.T) {
	fixture := newWindowsLocalBetaFixture(t)
	nativeChecks := 0
	installCalled := false
	options := fixture.options(func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "install" {
			installCalled = true
		}
		return nil, fmt.Errorf("bootstrapper must not execute")
	})
	options.VerifyNative = func(_ context.Context, path string) error {
		nativeChecks++
		if nativeChecks == 1 {
			return os.WriteFile(path, []byte("substituted unsigned bootstrapper"), 0o600)
		}
		return nil
	}
	_, err := Install(context.Background(), options)
	if err == nil || !strings.Contains(err.Error(), "installed bootstrapper changed during staging") {
		t.Fatalf("Install() error = %v, want staged bootstrapper digest rejection", err)
	}
	if installCalled {
		t.Fatal("Install delegated to a substituted bootstrapper")
	}
}

func TestInstallWindowsLocalBetaChecksCopiedSeedBeforeDelegation(t *testing.T) {
	fixture := newWindowsLocalBetaFixture(t)
	seedPath := ""
	installCalled := false
	run := func(_ context.Context, path string, args ...string) ([]byte, error) {
		if len(args) == 1 && args[0] == "seed-status" {
			seedPath = path
			return []byte(fmt.Sprintf(`{"schema_version":1,"product":"maestro","bootstrapper_version":"0.1.0","authority_registry_sha256":"%s"}`, fixture.registryDigest)), nil
		}
		if len(args) > 0 && args[0] == "install" {
			installCalled = true
			if err := os.MkdirAll(filepath.Join(fixture.managedRoot, "bin"), 0o700); err != nil {
				return nil, err
			}
			writeInstallerFile(t, filepath.Join(fixture.managedRoot, "bin", "bcgos.exe"), []byte("bcgos"))
			if err := os.MkdirAll(filepath.Join(fixture.managedRoot, "bundles", "0.1.0"), 0o700); err != nil {
				return nil, err
			}
			state := installtx.State{
				SchemaVersion: 2, ManagedRoot: fixture.managedRoot, Release: "0.1.0", Channel: "canary",
				CLIVersion: "0.1.0", BundleVersion: "0.1.0", TargetOS: "windows", TargetArch: "amd64",
				ActivatedAt: time.Date(2026, 8, 2, 20, 0, 0, 0, time.UTC),
			}
			if err := installtx.WriteState(fixture.dataRoot, state); err != nil {
				return nil, err
			}
			return []byte("installed"), nil
		}
		if filepath.Base(path) == "bcgos.exe" && len(args) == 1 && args[0] == "version" {
			return []byte("bcgos 0.1.0\n"), nil
		}
		return nil, fmt.Errorf("unexpected command %s %v", path, args)
	}
	result, err := Install(context.Background(), fixture.options(run))
	if err != nil {
		t.Fatal(err)
	}
	if !installCalled || result.Disposition != "installed" {
		t.Fatalf("local-beta installation was not delegated after checks: %#v", result)
	}
	wantSeedPath := filepath.Join(fixture.managedRoot, "bcgos-bootstrap.exe")
	if seedPath != wantSeedPath {
		t.Fatalf("seed-status path = %q, want copied bootstrapper %q", seedPath, wantSeedPath)
	}
}

func TestInstallWindowsLocalBetaRejectsInvalidCopiedSeedBeforeDelegation(t *testing.T) {
	fixture := newWindowsLocalBetaFixture(t)
	installCalled := false
	run := func(_ context.Context, _ string, args ...string) ([]byte, error) {
		if len(args) == 1 && args[0] == "seed-status" {
			return []byte(fmt.Sprintf(`{"schema_version":2,"product":"maestro","bootstrapper_version":"0.1.0","authority_registry_sha256":"%s"}`, fixture.registryDigest)), nil
		}
		if len(args) > 0 && args[0] == "install" {
			installCalled = true
		}
		return nil, fmt.Errorf("must not delegate")
	}
	_, err := Install(context.Background(), fixture.options(run))
	if err == nil || !strings.Contains(err.Error(), "installed bootstrapper seed binding changed") {
		t.Fatalf("Install() error = %v, want copied seed rejection", err)
	}
	if installCalled {
		t.Fatal("Install delegated after an invalid copied seed status")
	}
}

func TestPrepareWindowsLocalBetaStillRequiresValidEd25519Release(t *testing.T) {
	fixture := newWindowsLocalBetaFixture(t)
	signaturePath := filepath.Join(fixture.releaseDir, "release-manifest.json.sig")
	signature, err := os.ReadFile(signaturePath)
	if err != nil {
		t.Fatal(err)
	}
	signature[0] ^= 0xff
	if err := os.WriteFile(signaturePath, signature, 0o600); err != nil {
		t.Fatal(err)
	}
	nativeCalled := false
	options := fixture.options(func(context.Context, string, ...string) ([]byte, error) {
		return nil, fmt.Errorf("must not run")
	})
	options.VerifyNative = func(context.Context, string) error {
		nativeCalled = true
		return nil
	}
	if _, _, err := Prepare(options); err == nil || !strings.Contains(err.Error(), "release manifest signature verification failed") {
		t.Fatalf("Prepare() error = %v, want Ed25519 rejection", err)
	}
	if nativeCalled {
		t.Fatal("native trust check ran before Ed25519 release rejection")
	}
}

func TestInstallPreservesAHealthyExistingInstallation(t *testing.T) {
	fixture := newInstallerRecoveryFixture(t)
	writeCompleteInstalledFixture(t, fixture)
	installCalled := false
	run := fixture.runner(t, func() error {
		installCalled = true
		return nil
	}, func() ([]byte, error) {
		return []byte("bcgos 0.1.0\n"), nil
	})

	result, err := Install(context.Background(), fixture.options(run))
	if err != nil {
		t.Fatal(err)
	}
	if installCalled {
		t.Fatal("healthy installation was overwritten")
	}
	if result.Disposition != "already_installed" {
		t.Fatalf("disposition = %q", result.Disposition)
	}
	if _, err := os.Stat(filepath.Join(fixture.managedRoot, "bin", "bcgos")); err != nil {
		t.Fatalf("healthy CLI changed: %v", err)
	}
}

func TestInstallDoesNotPreserveExistingInstallationWithBootstrapperDigestDrift(t *testing.T) {
	fixture := newInstallerRecoveryFixture(t)
	writeCompleteInstalledFixture(t, fixture)
	writeInstallerFile(t, filepath.Join(fixture.managedRoot, "bcgos-bootstrap"), []byte("changed bootstrapper"))
	installCalled := false
	run := fixture.runner(t, func() error {
		installCalled = true
		return fmt.Errorf("stop after trust-driven recovery")
	}, func() ([]byte, error) {
		return []byte("bcgos 0.1.0\n"), nil
	})
	_, err := Install(context.Background(), fixture.options(run))
	if err == nil || !installCalled {
		t.Fatalf("Install() error = %v, installCalled=%t; digest-drifted installation was preserved", err, installCalled)
	}
}

func TestInstallQuarantinesAStaleBoundInstallationBeforeReinstall(t *testing.T) {
	fixture := newInstallerRecoveryFixture(t)
	writeInstalledState(t, fixture.dataRoot, fixture.managedRoot)
	writeInstallerFile(t, filepath.Join(fixture.managedRoot, "bin", "bcgos"), []byte("broken"))
	run := fixture.runner(t, func() error {
		if err := os.MkdirAll(filepath.Join(fixture.managedRoot, "bin"), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(fixture.managedRoot, "bin", "bcgos"), []byte("healthy"), 0o700); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(fixture.managedRoot, "bundles", "0.1.0"), 0o700); err != nil {
			return err
		}
		return installtx.WriteState(fixture.dataRoot, installedState(fixture.managedRoot))
	}, func() ([]byte, error) {
		body, err := os.ReadFile(filepath.Join(fixture.managedRoot, "bin", "bcgos"))
		if err != nil || string(body) != "healthy" {
			return nil, fmt.Errorf("CLI is not healthy")
		}
		return []byte("bcgos 0.1.0\n"), nil
	})

	result, err := Install(context.Background(), fixture.options(run))
	if err != nil {
		t.Fatal(err)
	}
	if result.Disposition != "recovered_and_installed" || result.Recovery == nil {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(result.Recovery.ManagedRootBackup); err != nil {
		t.Fatalf("managed recovery missing: %v", err)
	}
	if _, err := os.Stat(result.Recovery.InstallStateBackup); err != nil {
		t.Fatalf("state recovery missing: %v", err)
	}
	if state, err := installtx.ReadStateForManagedRoot(fixture.dataRoot, fixture.managedRoot); err != nil || state.Release != "0.1.0" {
		t.Fatalf("new install state = %#v, err=%v", state, err)
	}
}

func TestInstallQuarantinesCommittedActivationWhenFinalSelfCheckFails(t *testing.T) {
	fixture := newInstallerRecoveryFixture(t)
	run := fixture.runner(t, func() error {
		if err := os.MkdirAll(filepath.Join(fixture.managedRoot, "bin"), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(fixture.managedRoot, "bin", "bcgos"), []byte("activated"), 0o700); err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Join(fixture.managedRoot, "bundles", "0.1.0"), 0o700); err != nil {
			return err
		}
		return installtx.WriteState(fixture.dataRoot, installedState(fixture.managedRoot))
	}, func() ([]byte, error) {
		return nil, fmt.Errorf("post-activation diagnostic failure")
	})

	_, err := Install(context.Background(), fixture.options(run))
	if err == nil || !strings.Contains(err.Error(), "quarantined") {
		t.Fatalf("Install() error = %v", err)
	}
	if _, statErr := os.Stat(fixture.managedRoot); !os.IsNotExist(statErr) {
		t.Fatalf("failed activation remained authoritative: %v", statErr)
	}
	if _, stateErr := installtx.ReadStateForManagedRoot(fixture.dataRoot, fixture.managedRoot); !os.IsNotExist(stateErr) {
		t.Fatalf("orphan install state remained active: %v", stateErr)
	}
	recoveryRoot := fixture.managedRoot + ".interrupted-" + fixture.planDigestPrefix(t)
	if _, statErr := os.Stat(recoveryRoot); statErr != nil {
		t.Fatalf("failed activation was not preserved: %v", statErr)
	}
}

func TestInstallRejectsBootstrapperSuccessWithoutCommittedState(t *testing.T) {
	fixture := newInstallerRecoveryFixture(t)
	run := fixture.runner(t, func() error {
		if err := os.MkdirAll(filepath.Join(fixture.managedRoot, "bin"), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(fixture.managedRoot, "bin", "bcgos"), []byte("healthy"), 0o700); err != nil {
			return err
		}
		return os.MkdirAll(filepath.Join(fixture.managedRoot, "bundles", "0.1.0"), 0o700)
	}, func() ([]byte, error) {
		return []byte("bcgos 0.1.0\n"), nil
	})

	_, err := Install(context.Background(), fixture.options(run))
	if err == nil || !strings.Contains(err.Error(), "committed installation is incomplete") {
		t.Fatalf("Install() error = %v", err)
	}
	if _, statErr := os.Stat(fixture.managedRoot); !os.IsNotExist(statErr) {
		t.Fatalf("uncommitted managed root remained authoritative: %v", statErr)
	}
}

func TestInstallRefusesToReplaceAnAmbiguousManagedRoot(t *testing.T) {
	fixture := newInstallerRecoveryFixture(t)
	writeInstallerFile(t, filepath.Join(fixture.managedRoot, "owner-note.txt"), []byte("preserve me"))
	_, err := Install(context.Background(), fixture.options(fixture.runner(t, func() error {
		return fmt.Errorf("must not run")
	}, func() ([]byte, error) {
		return nil, fmt.Errorf("must not run")
	})))
	if err == nil || !strings.Contains(err.Error(), "unrecognized entry") {
		t.Fatalf("Install() error = %v", err)
	}
	if body, readErr := os.ReadFile(filepath.Join(fixture.managedRoot, "owner-note.txt")); readErr != nil || string(body) != "preserve me" {
		t.Fatalf("ambiguous root changed: body=%q err=%v", body, readErr)
	}
}

type installerRecoveryFixture struct {
	releaseDir, bootstrapper, registryPath string
	managedRoot, dataRoot, registryDigest  string
}

type windowsLocalBetaFixture struct {
	releaseDir, bootstrapper, registryPath                   string
	managedRoot, dataRoot                                    string
	registryDigest, bootstrapperDigest, issuer, authorityKey string
}

func newWindowsLocalBetaFixture(t *testing.T) windowsLocalBetaFixture {
	t.Helper()
	root := t.TempDir()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	issuer, keyID := "maestro-beta-local", "beta-20260805"
	registryBody := []byte(fmt.Sprintf(`{"schema_version":1,"product":"maestro","authorities":[{"issuer":"%s","key_id":"%s","algorithm":"ed25519","public_key":"%s","status":"active","valid_from":"2026-01-01T00:00:00Z","valid_until":"2030-01-01T00:00:00Z"}]}`,
		issuer, keyID, base64.StdEncoding.EncodeToString(privateKey.Public().(ed25519.PublicKey))))
	registryPath := filepath.Join(root, "authority-registry.json")
	writeInstallerFile(t, registryPath, registryBody)
	registryDigest, err := fileSHA256(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	releaseDir := filepath.Join(root, "release")
	writeSignedReleaseFixtureFor(t, releaseDir, privateKey, signedReleaseFixtureOptions{
		channel: "canary", issuer: issuer, keyID: keyID, targetOS: "windows", targetArch: "amd64",
	})
	bootstrapper := filepath.Join(root, "bcgos-bootstrap_0.1.0_windows_amd64.exe")
	writeInstallerFile(t, bootstrapper, []byte("unsigned but package-pinned Windows bootstrapper"))
	bootstrapperDigest, err := fileSHA256(bootstrapper)
	if err != nil {
		t.Fatal(err)
	}
	managedRoot, err := canonicalInstallRoot(filepath.Join(root, "Maestro"), "managed root")
	if err != nil {
		t.Fatal(err)
	}
	dataRoot, err := canonicalInstallRoot(filepath.Join(root, "BCGOS"), "owner-data root")
	if err != nil {
		t.Fatal(err)
	}
	return windowsLocalBetaFixture{
		releaseDir: releaseDir, bootstrapper: bootstrapper, registryPath: registryPath,
		managedRoot: managedRoot, dataRoot: dataRoot, registryDigest: registryDigest,
		bootstrapperDigest: bootstrapperDigest, issuer: issuer, authorityKey: keyID,
	}
}

func (fixture windowsLocalBetaFixture) options(run commandRunner) Options {
	return Options{
		ReleaseDir: fixture.releaseDir, Bootstrapper: fixture.bootstrapper,
		AuthorityRegistry: fixture.registryPath, ManagedRoot: fixture.managedRoot, DataRoot: fixture.dataRoot,
		TargetOS: "windows", TargetArch: "amd64", NativeTrustMode: NativeTrustWindowsLocalBeta,
		LocalBetaPins: LocalBetaPins{
			AuthorityRegistrySHA256: fixture.registryDigest, BootstrapperSHA256: fixture.bootstrapperDigest,
			Issuer: fixture.issuer, KeyID: fixture.authorityKey,
		},
		Clock:        func() time.Time { return time.Date(2026, 8, 2, 20, 0, 0, 0, time.UTC) },
		VerifyNative: func(context.Context, string) error { return nil }, Run: run,
	}
}

func newInstallerRecoveryFixture(t *testing.T) installerRecoveryFixture {
	t.Helper()
	root := t.TempDir()
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	registryBody := []byte(fmt.Sprintf(`{"schema_version":1,"product":"maestro","authorities":[{"issuer":"release","key_id":"key-1","algorithm":"ed25519","public_key":"%s","status":"active","valid_from":"2026-01-01T00:00:00Z","valid_until":"2030-01-01T00:00:00Z"}]}`,
		base64.StdEncoding.EncodeToString(publicKey)))
	registryPath := filepath.Join(root, "registry.json")
	writeInstallerFile(t, registryPath, registryBody)
	registryDigest, err := fileSHA256(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	releaseDir := filepath.Join(root, "release")
	writeSignedReleaseFixture(t, releaseDir, privateKey)
	bootstrapper := filepath.Join(root, "bcgos-bootstrap_0.1.0_darwin_arm64")
	writeInstallerFile(t, bootstrapper, []byte("native signed bootstrapper"))
	managedRoot, err := canonicalInstallRoot(filepath.Join(root, "Maestro"), "managed root")
	if err != nil {
		t.Fatal(err)
	}
	dataRoot, err := canonicalInstallRoot(filepath.Join(root, "BCGOS"), "owner-data root")
	if err != nil {
		t.Fatal(err)
	}
	return installerRecoveryFixture{
		releaseDir: releaseDir, bootstrapper: bootstrapper, registryPath: registryPath,
		managedRoot: managedRoot, dataRoot: dataRoot,
		registryDigest: registryDigest,
	}
}

func (fixture installerRecoveryFixture) options(run commandRunner) Options {
	return Options{
		ReleaseDir: fixture.releaseDir, Bootstrapper: fixture.bootstrapper,
		AuthorityRegistry: fixture.registryPath, ManagedRoot: fixture.managedRoot,
		DataRoot: fixture.dataRoot, TargetOS: "darwin", TargetArch: "arm64",
		Clock:        func() time.Time { return time.Date(2026, 8, 2, 20, 0, 0, 0, time.UTC) },
		VerifyNative: func(context.Context, string) error { return nil }, Run: run,
	}
}

func (fixture installerRecoveryFixture) runner(t *testing.T, install func() error, check func() ([]byte, error)) commandRunner {
	t.Helper()
	return func(_ context.Context, path string, args ...string) ([]byte, error) {
		if strings.Contains(filepath.Base(path), "bcgos-bootstrap") && len(args) == 1 && args[0] == "seed-status" {
			return []byte(fmt.Sprintf(`{"schema_version":1,"product":"maestro","bootstrapper_version":"0.1.0","authority_registry_sha256":"%s"}`,
				fixture.registryDigest)), nil
		}
		if strings.Contains(filepath.Base(path), "bcgos-bootstrap") && len(args) > 0 && args[0] == "install" {
			if err := install(); err != nil {
				return nil, err
			}
			return []byte("Maestro installation complete"), nil
		}
		if filepath.Base(path) == "bcgos" && len(args) == 1 && args[0] == "version" {
			return check()
		}
		return nil, fmt.Errorf("unexpected command %s %v", path, args)
	}
}

func (fixture installerRecoveryFixture) planDigestPrefix(t *testing.T) string {
	t.Helper()
	plan, _, err := Prepare(fixture.options(fixture.runner(t, func() error { return nil }, func() ([]byte, error) {
		return []byte("bcgos 0.1.0\n"), nil
	})))
	if err != nil {
		t.Fatal(err)
	}
	return plan.PlanDigest[:12]
}

func installedState(managedRoot string) installtx.State {
	return installtx.State{
		SchemaVersion: 2, ManagedRoot: managedRoot, Release: "0.1.0", Channel: "canary",
		CLIVersion: "0.1.0", BundleVersion: "0.1.0", TargetOS: "darwin", TargetArch: "arm64",
		ActivatedAt: time.Date(2026, 8, 2, 20, 0, 0, 0, time.UTC),
	}
}

func writeInstalledState(t *testing.T, dataRoot, managedRoot string) {
	t.Helper()
	if err := installtx.WriteState(dataRoot, installedState(managedRoot)); err != nil {
		t.Fatal(err)
	}
}

func writeCompleteInstalledFixture(t *testing.T, fixture installerRecoveryFixture) {
	t.Helper()
	writeInstalledState(t, fixture.dataRoot, fixture.managedRoot)
	writeInstallerFile(t, filepath.Join(fixture.managedRoot, "bin", "bcgos"), []byte("healthy"))
	bootstrapper, err := os.ReadFile(fixture.bootstrapper)
	if err != nil {
		t.Fatal(err)
	}
	writeInstallerFile(t, filepath.Join(fixture.managedRoot, "bcgos-bootstrap"), bootstrapper)
	registry, err := os.ReadFile(fixture.registryPath)
	if err != nil {
		t.Fatal(err)
	}
	writeInstallerFile(t, filepath.Join(fixture.managedRoot, "trust", "release-authority-registry.json"), registry)
	if err := os.MkdirAll(filepath.Join(fixture.managedRoot, "bundles", "0.1.0"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func writeSignedReleaseFixture(t *testing.T, directory string, privateKey ed25519.PrivateKey) {
	writeSignedReleaseFixtureFor(t, directory, privateKey, signedReleaseFixtureOptions{
		channel: "canary", issuer: "release", keyID: "key-1", targetOS: "darwin", targetArch: "arm64",
	})
}

type signedReleaseFixtureOptions struct {
	channel, issuer, keyID, targetOS, targetArch string
}

func writeSignedReleaseFixtureFor(t *testing.T, directory string, privateKey ed25519.PrivateKey, options signedReleaseFixtureOptions) {
	t.Helper()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	artifacts := []struct {
		kind, osName, arch, name string
		body                     []byte
	}{
		{"cli", options.targetOS, options.targetArch, "bcgos_0.1.0_" + options.targetOS + "_" + options.targetArch + map[bool]string{true: ".exe", false: ""}[options.targetOS == "windows"], []byte("bcgos 0.1.0")},
		{"bundle", "any", "any", "maestro-base_0.1.0.tar.gz", []byte("base bundle")},
	}
	notes := []byte("# Maestro 0.1.0\n")
	manifestArtifacts := make([]map[string]any, 0, len(artifacts))
	for _, artifact := range artifacts {
		digest := sha256.Sum256(artifact.body)
		manifestArtifacts = append(manifestArtifacts, map[string]any{
			"kind": artifact.kind, "os": artifact.osName, "arch": artifact.arch,
			"name": artifact.name, "size": len(artifact.body), "sha256": hex.EncodeToString(digest[:]),
			"signature_ref": artifact.name + ".sig",
		})
	}
	noteDigest := sha256.Sum256(notes)
	manifest := map[string]any{
		"schema_version": 1, "product": "maestro", "release": "0.1.0", "channel": options.channel,
		"issuer":    map[string]string{"id": options.issuer, "key_id": options.keyID},
		"cli":       map[string]string{"version": "0.1.0", "compatible_bundle": ">=0.1.0 <0.2.0"},
		"bundle":    map[string]string{"version": "0.1.0", "compatible_cli": ">=0.1.0 <0.2.0"},
		"artifacts": manifestArtifacts, "migrations": []any{},
		"release_notes": map[string]any{"name": "release-notes-0.1.0.md", "sha256": hex.EncodeToString(noteDigest[:])},
	}
	manifestBody, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	writeInstallerFile(t, filepath.Join(directory, "release-manifest.json"), manifestBody)
	writeInstallerFile(t, filepath.Join(directory, "release-manifest.json.sig"), ed25519.Sign(privateKey, manifestBody))
	writeInstallerFile(t, filepath.Join(directory, "release-notes-0.1.0.md"), notes)
	for _, artifact := range artifacts {
		writeInstallerFile(t, filepath.Join(directory, artifact.name), artifact.body)
		writeInstallerFile(t, filepath.Join(directory, artifact.name+".sig"), ed25519.Sign(privateKey, artifact.body))
	}
}

func writeInstallerFile(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
}
