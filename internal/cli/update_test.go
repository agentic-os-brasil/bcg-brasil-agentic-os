package cli

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateplan"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
)

type fakeReleaseUpdateService struct {
	pending    updateservice.Pending
	checkErr   error
	confirmErr error
	confirmed  string
}

func (service *fakeReleaseUpdateService) Check(context.Context) (updateservice.Pending, error) {
	return service.pending, service.checkErr
}

func (service *fakeReleaseUpdateService) Confirm(planID string) (updateservice.Pending, error) {
	service.confirmed = planID
	return service.pending, service.confirmErr
}

type staticSecureStore struct {
	body []byte
	err  error
}

func (store staticSecureStore) Available() error {
	return store.err
}

func (store staticSecureStore) Get(string) ([]byte, error) {
	if store.err != nil {
		return nil, store.err
	}
	return append([]byte(nil), store.body...), nil
}

func (staticSecureStore) Put(string, []byte) error {
	return nil
}

func (staticSecureStore) Delete(string) error {
	return nil
}

type inertProvider struct{}

func (inertProvider) ListReleases(context.Context) ([]releaseprovider.Release, error) {
	return nil, errors.New("not called")
}

func (inertProvider) FetchAsset(context.Context, releaseprovider.Asset, string) error {
	return errors.New("not called")
}

func TestRunUpdateWritesExactPlanAndStartsOnlyItsConfirmation(t *testing.T) {
	plan := testUpdatePlan("0123456789abcdef0123456789abcdef")
	service := &fakeReleaseUpdateService{
		pending: updateservice.Pending{SchemaVersion: 1, Plan: plan},
	}
	var output bytes.Buffer
	var errorOutput bytes.Buffer
	if code := runUpdate([]string{"--check"}, &output, &errorOutput, service); code != ExitOK {
		t.Fatalf("check exit = %d; out=%s err=%s", code, output.String(), errorOutput.String())
	}
	var checked releaseUpdateResult
	if err := json.Unmarshal(output.Bytes(), &checked); err != nil {
		t.Fatal(err)
	}
	if checked.State != "available" ||
		!checked.ConfirmationRequired ||
		checked.PlanID != plan.ID ||
		checked.Plan == nil ||
		checked.Plan.ID != plan.ID ||
		!strings.Contains(checked.NextAction, plan.ID) {
		t.Fatalf("unexpected check result: %#v", checked)
	}

	output.Reset()
	errorOutput.Reset()
	if code := runUpdate([]string{"--confirm", plan.ID}, &output, &errorOutput, service); code != ExitOK {
		t.Fatalf("confirm exit = %d; out=%s err=%s", code, output.String(), errorOutput.String())
	}
	var confirmed releaseUpdateResult
	if err := json.Unmarshal(output.Bytes(), &confirmed); err != nil {
		t.Fatal(err)
	}
	if confirmed.State != "activation_started" ||
		confirmed.ConfirmationRequired ||
		confirmed.PlanID != plan.ID ||
		service.confirmed != plan.ID {
		t.Fatalf("unexpected confirmation result: %#v service=%#v", confirmed, service)
	}
}

func TestRunUpdateReturnsSchemaVersionedOperationalStates(t *testing.T) {
	tests := map[string]struct {
		err   error
		state string
		code  int
	}{
		"unavailable": {
			err:   errReleaseUnavailable,
			state: "unavailable", code: ExitUnavailable,
		},
		"authentication": {
			err:   errAuthenticationRequired,
			state: "authentication_required", code: ExitUnavailable,
		},
		"current": {
			err:   updateservice.ErrCurrent,
			state: "current", code: ExitOK,
		},
		"no pending": {
			err:   errNoPendingUpdate,
			state: "no_pending_update", code: ExitUnavailable,
		},
		"missing install state": {
			err:   os.ErrNotExist,
			state: "error", code: ExitFailure,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			service := &fakeReleaseUpdateService{checkErr: test.err}
			var output bytes.Buffer
			var errorOutput bytes.Buffer
			if code := runUpdate([]string{"--check"}, &output, &errorOutput, service); code != test.code {
				t.Fatalf("exit = %d, want %d; out=%s err=%s", code, test.code, output.String(), errorOutput.String())
			}
			var result releaseUpdateResult
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatalf("output is not one JSON result: %v", err)
			}
			if result.SchemaVersion != 1 || result.State != test.state {
				t.Fatalf("unexpected result: %#v", result)
			}
		})
	}
}

func TestConfiguredReleaseUpdateCheckBindsInstalledRuntimeAndStagesPending(t *testing.T) {
	service, dataRoot, managedRoot, now := configuredUpdateFixture(t)
	verifiedDirectory := filepath.Join(dataRoot, "updates", "github-release-42")
	if err := os.MkdirAll(verifiedDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	plan := testUpdatePlan("0123456789abcdef0123456789abcdef")
	result := updateservice.CheckResult{
		Plan: plan,
		Verified: releaseverify.VerifiedRelease{
			Directory: verifiedDirectory, ManifestSHA256: strings.Repeat("a", 64),
		},
	}
	providerBuilt := false
	service.provider = func(config releaseprovider.Config, token string) updateservice.Provider {
		if config.Owner != "agentic-os-brasil" || token != "access-secret" {
			t.Fatalf("unexpected provider inputs: %#v token=%q", config, token)
		}
		providerBuilt = true
		return inertProvider{}
	}
	registryLoaded := false
	service.loadRegistry = func(path, digest string, clock func() time.Time) (releaseverify.KeyRegistry, error) {
		if path != filepath.Join(managedRoot, "trust", "release-authority-registry.json") ||
			digest != strings.Repeat("b", 64) ||
			!clock().Equal(now) {
			t.Fatalf("unexpected registry inputs: path=%q digest=%q time=%s", path, digest, clock())
		}
		registryLoaded = true
		return releaseverify.StaticRegistry{}, nil
	}
	service.check = func(_ context.Context, options updateservice.CheckOptions) (updateservice.CheckResult, error) {
		if options.Current.ManagedRoot != managedRoot ||
			options.TargetOS != "darwin" ||
			options.TargetArch != "arm64" ||
			options.StagingRoot != filepath.Join(dataRoot, "updates") ||
			options.Provider == nil ||
			options.Registry == nil {
			t.Fatalf("unexpected check options: %#v", options)
		}
		return result, nil
	}
	staged := false
	service.stage = func(root string, current installtx.State, checked updateservice.CheckResult, createdAt time.Time) (updateservice.Pending, error) {
		if root != dataRoot ||
			current.ManagedRoot != managedRoot ||
			checked.Plan.ID != plan.ID ||
			!createdAt.Equal(now) {
			t.Fatalf("unexpected staging inputs: root=%q current=%#v checked=%#v at=%s", root, current, checked, createdAt)
		}
		staged = true
		return updateservice.Pending{SchemaVersion: 1, Plan: plan, CreatedAt: createdAt}, nil
	}

	pending, err := service.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if pending.Plan.ID != plan.ID || !providerBuilt || !registryLoaded || !staged {
		t.Fatalf("check did not complete exact staging: %#v", pending)
	}
}

func TestConfiguredReleaseUpdateCheckCleansVerifiedDownloadWhenStagingFails(t *testing.T) {
	service, dataRoot, _, _ := configuredUpdateFixture(t)
	verifiedDirectory := filepath.Join(dataRoot, "updates", "github-release-42")
	if err := os.MkdirAll(verifiedDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(verifiedDirectory, "artifact"), []byte("verified"), 0o600); err != nil {
		t.Fatal(err)
	}
	service.provider = func(releaseprovider.Config, string) updateservice.Provider {
		return inertProvider{}
	}
	service.loadRegistry = func(string, string, func() time.Time) (releaseverify.KeyRegistry, error) {
		return releaseverify.StaticRegistry{}, nil
	}
	service.check = func(context.Context, updateservice.CheckOptions) (updateservice.CheckResult, error) {
		return updateservice.CheckResult{
			Plan: testUpdatePlan("0123456789abcdef0123456789abcdef"),
			Verified: releaseverify.VerifiedRelease{
				Directory: verifiedDirectory, ManifestSHA256: strings.Repeat("a", 64),
			},
		}, nil
	}
	service.stage = func(string, installtx.State, updateservice.CheckResult, time.Time) (updateservice.Pending, error) {
		return updateservice.Pending{}, errors.New("simulated pending write failure")
	}
	if _, err := service.Check(context.Background()); err == nil {
		t.Fatal("Check() accepted a failed durable pending write")
	}
	if _, err := os.Stat(verifiedDirectory); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("provisional verified download was not cleaned: %v", err)
	}
}

func TestConfiguredReleaseUpdateCheckReusesOnlyReverifiedPendingPlan(t *testing.T) {
	service, dataRoot, managedRoot, _ := configuredUpdateFixture(t)
	plan := testUpdatePlan("0123456789abcdef0123456789abcdef")
	existing := updateservice.Pending{SchemaVersion: 1, Plan: plan}
	service.auth = releaseprovider.AuthService{Store: staticSecureStore{err: errors.New("must not touch auth")}}
	service.loadRegistry = func(string, string, func() time.Time) (releaseverify.KeyRegistry, error) {
		return releaseverify.StaticRegistry{}, nil
	}
	service.loadPending = func(root string) (updateservice.Pending, error) {
		if root != dataRoot {
			t.Fatalf("pending root = %q, want %q", root, dataRoot)
		}
		return existing, nil
	}
	validated := false
	service.validatePending = func(
		root, installedRoot, planID string,
		registry releaseverify.KeyRegistry,
	) (updateservice.Pending, error) {
		if root != dataRoot ||
			installedRoot != managedRoot ||
			planID != plan.ID ||
			registry == nil {
			t.Fatalf(
				"unexpected pending validation: data=%q managed=%q plan=%q registry=%#v",
				root, installedRoot, planID, registry,
			)
		}
		validated = true
		return existing, nil
	}
	service.provider = func(releaseprovider.Config, string) updateservice.Provider {
		t.Fatal("existing valid pending plan reached the provider")
		return nil
	}
	pending, err := service.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !validated || pending.Plan.ID != plan.ID {
		t.Fatalf("existing exact plan was not reused: %#v", pending)
	}

	service.validatePending = func(
		string, string, string, releaseverify.KeyRegistry,
	) (updateservice.Pending, error) {
		return updateservice.Pending{}, errors.New("stale pending plan")
	}
	if _, err := service.Check(context.Background()); err == nil {
		t.Fatal("Check() replaced a stale or invalid pending plan")
	}
}

func TestConfiguredReleaseUpdateConfirmLaunchesProtectedBootstrapper(t *testing.T) {
	service, dataRoot, managedRoot, now := configuredUpdateFixture(t)
	plan := testUpdatePlan("0123456789abcdef0123456789abcdef")
	service.processID = func() int { return 321 }
	service.loadPending = func(root string) (updateservice.Pending, error) {
		if root != dataRoot {
			t.Fatalf("pending root = %q, want %q", root, dataRoot)
		}
		return updateservice.Pending{SchemaVersion: 1, Plan: plan}, nil
	}
	service.loadRegistry = func(path, digest string, clock func() time.Time) (releaseverify.KeyRegistry, error) {
		if path != filepath.Join(managedRoot, "trust", "release-authority-registry.json") ||
			digest != strings.Repeat("b", 64) ||
			!clock().Equal(now) {
			t.Fatalf("unexpected confirmation registry inputs: path=%q digest=%q", path, digest)
		}
		return releaseverify.StaticRegistry{}, nil
	}
	service.validatePending = func(
		root, installedRoot, planID string,
		registry releaseverify.KeyRegistry,
	) (updateservice.Pending, error) {
		if root != dataRoot ||
			installedRoot != managedRoot ||
			planID != plan.ID ||
			registry == nil {
			t.Fatalf(
				"unexpected confirmation validation: root=%q managed=%q plan=%q",
				root, installedRoot, planID,
			)
		}
		return updateservice.Pending{SchemaVersion: 1, Plan: plan}, nil
	}
	started := false
	service.startActivation = func(bootstrapper, root, planID string, waitPID int, startedAt time.Time) error {
		if bootstrapper != filepath.Join(managedRoot, "bcgos-bootstrap") ||
			root != dataRoot ||
			planID != plan.ID ||
			waitPID != 321 ||
			!startedAt.Equal(now) {
			t.Fatalf(
				"unexpected activation: bootstrapper=%q root=%q plan=%q pid=%d at=%s",
				bootstrapper, root, planID, waitPID, startedAt,
			)
		}
		started = true
		return nil
	}
	pending, err := service.Confirm(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pending.Plan.ID != plan.ID || !started {
		t.Fatalf("exact pending update was not launched: %#v", pending)
	}

	service.loadPending = func(string) (updateservice.Pending, error) {
		other := plan
		other.ID = strings.Repeat("f", 32)
		return updateservice.Pending{SchemaVersion: 1, Plan: other}, nil
	}
	started = false
	if _, err := service.Confirm(plan.ID); !errors.Is(err, errNoPendingUpdate) {
		t.Fatalf("Confirm() mismatched plan error = %v", err)
	}
	if started {
		t.Fatal("mismatched confirmation started the bootstrapper")
	}

	service.loadPending = func(string) (updateservice.Pending, error) {
		return updateservice.Pending{SchemaVersion: 1, Plan: plan}, nil
	}
	for name, validationErr := range map[string]error{
		"stale source state":     errors.New("installed state changed after confirmation"),
		"mutated prepared bytes": errors.New("prepared artifact digest changed"),
	} {
		t.Run(name, func(t *testing.T) {
			service.validatePending = func(
				string, string, string, releaseverify.KeyRegistry,
			) (updateservice.Pending, error) {
				return updateservice.Pending{}, validationErr
			}
			started = false
			if _, err := service.Confirm(plan.ID); err == nil {
				t.Fatal("Confirm() accepted invalid pending activation")
			}
			if started {
				t.Fatal("invalid pending activation started the bootstrapper")
			}
		})
	}
}

func TestConfiguredReleaseUpdateConfirmLaunchesExactPostCommitRecovery(t *testing.T) {
	service, dataRoot, managedRoot, now := configuredUpdateFixture(t)
	current, err := installtx.ReadStateForManagedRoot(dataRoot, managedRoot)
	if err != nil {
		t.Fatal(err)
	}
	verified, registry := signedCLIUpdateFixture(t, dataRoot, "0.2.0")
	plan, err := updateplan.Build(
		current,
		verified.Manifest,
		current.TargetOS,
		current.TargetArch,
		updateplan.SourceBinding{
			Provider: "github", ProviderReleaseID: 42, ManifestSHA256: verified.ManifestSHA256,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := updateservice.StagePending(
		dataRoot,
		current,
		updateservice.CheckResult{Plan: plan, Verified: verified},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	service.loadPending = updateservice.LoadPending
	service.loadRegistry = func(string, string, func() time.Time) (releaseverify.KeyRegistry, error) {
		return registry, nil
	}
	service.validatePending = updateservice.ValidatePendingLaunch
	started := false
	service.startActivation = func(string, string, string, int, time.Time) error {
		started = true
		return nil
	}

	divergent := current
	divergent.Release = "0.1.5"
	divergent.CLIVersion = "0.1.5"
	divergent.BundleVersion = "0.1.5"
	if err := installtx.WriteState(dataRoot, divergent); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Confirm(plan.ID); err == nil {
		t.Fatal("Confirm() accepted divergent State without recovery evidence")
	}
	if started {
		t.Fatal("divergent State started the bootstrapper")
	}
	if err := installtx.WriteState(dataRoot, current); err != nil {
		t.Fatal(err)
	}

	activeCLI := filepath.Join(managedRoot, "bin", "bcgos")
	if err := os.WriteFile(activeCLI, []byte("bcgos 0.1.0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	confirmed, reverified, err := updateservice.ConfirmPending(
		dataRoot,
		managedRoot,
		plan.ID,
		registry,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := installtx.Activate(
		confirmed.ActivationPlanPath,
		reverified,
		installtx.ActivateOptions{
			PrepareOptions: installtx.PrepareOptions{
				Transition:         "update",
				ConfirmationPlanID: plan.ID,
				FromRelease:        plan.FromRelease,
				FromChannel:        plan.FromChannel,
				FromCLIVersion:     plan.FromCLIVersion,
				FromBundleVersion:  plan.FromBundleVersion,
				TargetOS:           plan.TargetOS,
				TargetArch:         plan.TargetArch,
				ManagedRoot:        managedRoot,
				DataRoot:           dataRoot,
			},
			CheckCLI: func(_, _ string) error { return nil },
		},
	); err != nil {
		t.Fatal(err)
	}
	// Pending intentionally remains, simulating a crash after State commit and
	// before the bootstrapper consumes the durable envelope.
	started = false
	recovery, err := service.Confirm(plan.ID)
	if err != nil {
		t.Fatalf("Confirm() rejected exact post-commit recovery: %v", err)
	}
	if !started || recovery.Plan.ID != pending.Plan.ID {
		t.Fatalf("post-commit recovery did not start exact plan: %#v", recovery)
	}

	activation, err := installtx.ReadPlan(pending.ActivationPlanPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(activation.StagedCLI, []byte("mutated"), 0o755); err != nil {
		t.Fatal(err)
	}
	started = false
	if _, err := service.Confirm(plan.ID); err == nil {
		t.Fatal("Confirm() accepted mutated prepared bytes during recovery")
	}
	if started {
		t.Fatal("mutated recovery started the bootstrapper")
	}
}

func TestManagedRootFromCLIExecutablePathIsFixed(t *testing.T) {
	root := filepath.Join(string(filepath.Separator), "opt", "maestro")
	tests := map[string]struct {
		path string
		ok   bool
	}{
		"darwin":        {path: filepath.Join(root, "bin", "bcgos"), ok: true},
		"windows":       {path: filepath.Join(root, "bin", "bcgos.exe"), ok: true},
		"copied":        {path: filepath.Join(root, "bcgos"), ok: false},
		"renamed":       {path: filepath.Join(root, "bin", "attacker"), ok: false},
		"relative root": {path: filepath.Join("managed", "bin", "bcgos"), ok: false},
		"filesystem root": {
			path: filepath.Join(string(filepath.Separator), "bin", "bcgos"), ok: false,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			actual, err := managedRootFromCLIExecutablePath(test.path)
			if test.ok {
				if err != nil || actual != filepath.Clean(root) {
					t.Fatalf("managed root = %q, %v", actual, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("managed root accepted unsafe path %q as %q", test.path, actual)
			}
		})
	}
}

func TestReleaseCapabilityRequiresProviderAuthorityAndNativeStore(t *testing.T) {
	approved := releaseprovider.Config{
		SchemaVersion: 1, State: "approved", Provider: "github",
		AuthBase: "https://github.com", APIBase: "https://api.github.com",
		ClientID: "client-id", Owner: "agentic-os-brasil", Repository: "maestro",
	}
	unavailable := approved
	unavailable.State = "unavailable"
	unavailable.ClientID = ""
	unavailable.Owner = ""
	unavailable.Repository = ""
	unavailable.Reason = "approval pending"
	tests := map[string]struct {
		config releaseprovider.Config
		digest string
		store  releaseprovider.SecureStore
		state  string
	}{
		"provider": {
			config: unavailable, digest: strings.Repeat("b", 64),
			store: staticSecureStore{}, state: "unavailable",
		},
		"authority": {
			config: approved, digest: "",
			store: staticSecureStore{}, state: "unavailable",
		},
		"native store": {
			config: approved, digest: strings.Repeat("b", 64),
			store: staticSecureStore{err: releaseprovider.ErrSecureStoreUnavailable}, state: "unavailable",
		},
		"configured": {
			config: approved, digest: strings.Repeat("b", 64),
			store: staticSecureStore{}, state: "configured",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			inspection := inspectReleaseCapability(test.config, test.digest, test.store)
			if inspection.State != test.state || inspection.Reason == "" {
				t.Fatalf("unexpected capability inspection: %#v", inspection)
			}
		})
	}
}

func configuredUpdateFixture(
	t *testing.T,
) (configuredReleaseUpdateService, string, string, time.Time) {
	t.Helper()
	requestedManagedRoot := filepath.Join(t.TempDir(), "managed")
	dataRoot := filepath.Join(t.TempDir(), "data")
	executable := filepath.Join(requestedManagedRoot, "bin", "bcgos")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("bcgos"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolvedExecutable, err := filepath.EvalSymlinks(executable)
	if err != nil {
		t.Fatal(err)
	}
	managedRoot := filepath.Dir(filepath.Dir(resolvedExecutable))
	now := time.Unix(2000, 0).UTC()
	current := installtx.State{
		SchemaVersion: 2, ManagedRoot: managedRoot,
		Release: "0.1.0", Channel: "canary", CLIVersion: "0.1.0", BundleVersion: "0.1.0",
		TargetOS: "darwin", TargetArch: "arm64", ActivatedAt: time.Unix(1000, 0).UTC(),
	}
	if err := installtx.WriteState(dataRoot, current); err != nil {
		t.Fatal(err)
	}
	token, err := json.Marshal(releaseprovider.Token{
		AccessToken: "access-secret", TokenType: "bearer", ExpiresAt: now.Add(time.Hour),
	})
	if err != nil {
		t.Fatal(err)
	}
	config := releaseprovider.Config{
		SchemaVersion: 1, State: "approved", Provider: "github",
		AuthBase: "https://github.com", APIBase: "https://api.github.com",
		ClientID: "client-id", Owner: "agentic-os-brasil", Repository: "maestro",
	}
	return configuredReleaseUpdateService{
		config: config,
		auth: releaseprovider.AuthService{
			Flow:  releaseprovider.DeviceFlowClient{Now: func() time.Time { return now }},
			Store: staticSecureStore{body: token},
		},
		dataRoot:       func() (string, error) { return dataRoot, nil },
		executable:     func() (string, error) { return executable, nil },
		targetOS:       "darwin",
		targetArch:     "arm64",
		processID:      func() int { return 123 },
		now:            func() time.Time { return now },
		registrySHA256: strings.Repeat("b", 64),
		loadPending: func(string) (updateservice.Pending, error) {
			return updateservice.Pending{}, os.ErrNotExist
		},
	}, dataRoot, managedRoot, now
}

func testUpdatePlan(id string) updateplan.Plan {
	return updateplan.Plan{
		SchemaVersion: 2, ID: id, State: "available",
		FromRelease: "0.1.0", FromChannel: "canary",
		FromCLIVersion: "0.1.0", FromBundleVersion: "0.1.0",
		ToRelease: "0.2.0", Channel: "canary",
		CLIVersion: "0.2.0", BundleVersion: "0.2.0",
		CLIArtifact:    "bcgos_0.2.0_darwin_arm64",
		BundleArtifact: "maestro-base_0.2.0.tar.gz",
		TargetOS:       "darwin", TargetArch: "arm64",
		Provider: "github", ProviderReleaseID: 42,
		ManifestSHA256:       strings.Repeat("a", 64),
		ConfirmationRequired: true,
	}
}

func signedCLIUpdateFixture(
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
	cliBody := []byte("bcgos " + version + "\n")
	bundleBody := cliUpdateBundle(t)
	notesBody := []byte("# Maestro " + version + "\n")
	artifacts := []struct {
		kind, osName, arch, name string
		body                     []byte
	}{
		{"cli", "darwin", "arm64", "bcgos_" + version + "_darwin_arm64", cliBody},
		{"bundle", "any", "any", "maestro-base_" + version + ".tar.gz", bundleBody},
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
		if err := os.WriteFile(
			filepath.Join(directory, signatureName),
			ed25519.Sign(privateKey, artifact.body),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		manifest.Artifacts = append(manifest.Artifacts, releasecontract.Artifact{
			Kind: artifact.kind, OS: artifact.osName, Arch: artifact.arch,
			Name: artifact.name, Size: int64(len(artifact.body)),
			SHA256: cliUpdateDigest(artifact.body), SignatureRef: signatureName,
		})
	}
	notesName := "release-notes-" + version + ".md"
	if err := os.WriteFile(filepath.Join(directory, notesName), notesBody, 0o600); err != nil {
		t.Fatal(err)
	}
	manifest.ReleaseNotes = releasecontract.ReleaseNotes{
		Name: notesName, SHA256: cliUpdateDigest(notesBody),
	}
	manifestBody, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	manifestBody = append(manifestBody, '\n')
	if err := os.WriteFile(
		filepath.Join(directory, releaseverify.ManifestName),
		manifestBody,
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(directory, releaseverify.ManifestSignatureName),
		ed25519.Sign(privateKey, manifestBody),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	registry := releaseverify.StaticRegistry{
		"maestro/maestro-release/pilot-2026": publicKey,
	}
	verified, err := releaseverify.VerifyDirectory(directory, registry)
	if err != nil {
		t.Fatal(err)
	}
	return verified, registry
}

func cliUpdateBundle(t *testing.T) []byte {
	t.Helper()
	var output bytes.Buffer
	gzipWriter := gzip.NewWriter(&output)
	tarWriter := tar.NewWriter(gzipWriter)
	body := []byte("managed\n")
	if err := tarWriter.WriteHeader(&tar.Header{
		Name: "skills/example/SKILL.md", Mode: 0o644,
		Size: int64(len(body)), Typeflag: tar.TypeReg,
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

func cliUpdateDigest(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
