//go:build darwin && arm64

package setup_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

// Actual compiled consumer and foreground manager, ephemeral signed releases,
// inert distinct archives. No model, real converter, native host or authority
// issuance is exercised or claimed by this controlled process witness.
func TestRuntimeUpdateCompiledChangedPackLifecycle(t *testing.T) {
	journeyStarted := time.Now()
	// Measured Mac runs exceed12m across builds, reconciliation and rollback.
	// This fixture budget changes no per-operation production deadline.
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()
	root := t.TempDir()
	retained := os.Getenv("MAESTRO_RUNTIME_UPDATE_TEST_ROOT")
	if retained != "" {
		if !filepath.IsAbs(retained) || filepath.Clean(retained) != retained {
			t.Fatal("retained fixture requires a canonical absolute new directory")
		}
		parent, pathErr := filepath.EvalSymlinks(filepath.Dir(retained))
		must(t, pathErr)
		if parent != filepath.Dir(retained) {
			t.Fatal("retained fixture parent must be physical, without aliases")
		}
		must(t, os.Mkdir(retained, 0700))
		root = retained // Fresh synthetic bindings retain their original absolute paths.
	}
	must(t, os.Chmod(root, 0700))
	t.Cleanup(func() {
		filepath.WalkDir(root, func(p string, d os.DirEntry, e error) error {
			if e == nil && d.IsDir() {
				os.Chmod(p, 0700)
			}
			return e
		})
		if retained != "" && !t.Failed() {
			must(t, os.RemoveAll(root))
		}
	})
	source, e := filepath.Abs("../../..")
	must(t, e)
	public, private, e := ed25519.GenerateKey(rand.Reader)
	must(t, e)
	registryBody, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "engineering-fixture", "key_id": "ephemeral", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": "2020-01-01T00:00:00Z", "valid_until": "2100-01-01T00:00:00Z"}}})
	registryPath := filepath.Join(root, "authority.json")
	must(t, os.WriteFile(registryPath, registryBody, 0600))
	registry, e := releaseverify.LoadAuthorityRegistry(registryPath, time.Now)
	must(t, e)
	type built struct {
		factory            releasepack.ModernCandidate
		verified           releaseverify.VerifiedRelease
		directory, packSHA string
	}
	build := func(version, marker string) built {
		pack := runtimeInertPack(t, root, source, version, marker)
		factory, e := releasepack.BuildModernCandidate(ctx, releasepack.ModernOptions{Root: source, Output: filepath.Join(root, "factory-"+version), Version: version, InstallerVersion: "0.3.7", Channel: "canary", AuthorityRegistry: registryPath, Target: releasepack.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}, DoclingPack: pack})
		must(t, e)
		directory := filepath.Join(root, "signed-"+version)
		_, e = releasepack.SignCandidate(releasepack.SignCandidateOptions{Candidate: factory.Directory, Output: directory, Registry: registryPath, Issuer: "engineering-fixture", KeyID: "ephemeral", PrivateKey: private})
		must(t, e)
		verified, e := releaseverify.VerifyDirectory(directory, registry)
		must(t, e)
		return built{factory, verified, directory, sum(read(t, pack))}
	}
	n := build("0.1.42", "N synthetic inert")
	next := build("0.1.43", "N1 DIFFERENT synthetic inert")
	t.Logf("journey timing: two candidates ready elapsed=%s", time.Since(journeyStarted))
	seedSHA := sum(read(t, n.factory.Installer))
	verifyNative := func(_ context.Context, path string) error {
		body, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if sum(body) != seedSHA {
			return errors.New("controlled native seed bytes changed")
		}
		return nil
	}
	config := setup.Config{Installer: n.factory.Installer, AuthorityRegistry: n.factory.AuthorityRegistry, LookupEnv: func(string) (string, bool) { return "", false }, VerifyNative: verifyNative}
	interruptFirstPlacement := true
	config.Run = func(ctx context.Context, self string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "runtime-apply" && interruptFirstPlacement {
			interruptFirstPlacement = false
			return nil, errors.New("synthetic first-placement interruption before consumer launch")
		}
		if len(args) > 0 && args[0] == "install" {
			f := flag.NewFlagSet("controlled-inert-install", flag.ContinueOnError)
			body := f.String("confirmed-plan", "", "")
			digest := f.String("review-digest", "", "")
			confirm := f.Bool("confirm", false, "")
			if e := f.Parse(args[1:]); e != nil {
				return nil, e
			}
			var plan installer.Plan
			if e := releasecontract.DecodeStrictJSON([]byte(*body), &plan); e != nil {
				return nil, e
			}
			state, e := installer.ActivateSeeded(ctx, installer.SeededActivationOptions{Self: self, Version: "0.3.7", AuthorityRegistrySHA256: sum(registryBody), Plan: plan, ReviewDigest: *digest, Confirmed: *confirm, VerifyNative: verifyNative})
			if e != nil {
				return nil, e
			}
			return json.Marshal(map[string]string{"state": "activated", "profile": state.Profile})
		}
		return exec.CommandContext(ctx, self, args...).CombinedOutput()
	}

	// A preexisting compatible legacy converter does not satisfy a different
	// authenticated initial Current, even before source enrollment.
	interruptFirstPlacement = false
	legacyDifferent := runtimeCompatibleLegacyRelease(t, root, "different", n.verified, registry, private, nil)
	var nextArchive []byte
	for _, artifact := range next.verified.Manifest.Artifacts {
		if artifact.Kind == "runtime_pack" {
			nextArchive = read(t, filepath.Join(next.directory, artifact.Name))
		}
	}
	legacySame := runtimeCompatibleLegacyRelease(t, root, "same-pack", n.verified, registry, private, nextArchive)
	for _, scenario := range []struct {
		name    string
		legacy  releaseverify.VerifiedRelease
		packSHA string
	}{{"different-pack", legacyDifferent, n.packSHA}, {"same-pack-new-manifest", legacySame, next.packSHA}} {
		priorUser := filepath.Join(root, "prior-owner-"+scenario.name)
		priorWS := filepath.Join(root, "prior-workspace-"+scenario.name)
		priorManaged := filepath.Join(root, "prior-managed-"+scenario.name)
		must(t, os.Mkdir(priorUser, 0700))
		must(t, os.Mkdir(priorWS, 0700))
		priorHome, e := datahome.InitializeFresh(datahome.Options{ProductRoot: source, UserHome: priorUser, LookupEnv: config.LookupEnv})
		must(t, e)
		_, e = workspace.Initialize(workspace.Options{WorkspacePath: priorWS, DataRoot: filepath.Join(priorHome.Home, "state")})
		must(t, e)
		must(t, os.WriteFile(filepath.Join(priorWS, "sample.txt"), []byte("synthetic initial source"), 0600))
		priorOptions := docling.Options{DataRoot: filepath.Join(priorHome.Home, "state"), AuthorityRegistryPath: registryPath, AuthorityRegistrySHA256: sum(registryBody), CLIVersion: next.verified.Manifest.Server.Version}
		must(t, docling.PlaceVerified(ctx, priorOptions, scenario.legacy))
		if got := docling.New(priorOptions).Status(ctx); got.State != "ready" || got.PackSHA256 != scenario.packSHA {
			t.Fatalf("legacy precondition %+v", got)
		}
		priorRoot := filepath.Join(priorHome.Home, "state/ingestion/docling")
		priorBytes := snapshot(t, priorRoot)
		priorRequest := setup.Request{ReleaseDirectory: next.directory, ManagedRoot: priorManaged, UserHome: priorUser, Workspace: priorWS, CodePacket: filepath.Join(root, "prior-code-"+scenario.name), DesktopPacket: filepath.Join(root, "prior-desktop-"+scenario.name+".mcpb")}
		priorConfig := config
		sawInitialReview := false
		priorConfig.Run = func(ctx context.Context, self string, args ...string) ([]byte, error) {
			body, err := config.Run(ctx, self, args...)
			if err == nil && len(args) > 0 && args[0] == "runtime-review" {
				var rr product.RuntimeUpdateReview
				must(t, json.Unmarshal(body, &rr))
				if rr.Requirement.ManifestSHA256 != next.verified.ManifestSHA256 || rr.Requirement.PackSHA256 != next.packSHA || rr.Availability.State != "unavailable" || rr.Availability.Reason != "current_pack_pending" || len(rr.Effects) == 0 {
					t.Fatalf("initial Current reused compatible legacy pack/manifest: %+v", rr)
				}
				cs := connect(t, ctx, self, priorRequest)
				status := call(t, ctx, cs, "maestro_query", map[string]any{"operation": "content.status"}, false)
				if status["state"] != "unavailable" || status["reason"] != "current_pack_pending" {
					t.Fatalf("initial MCP availability ignored Current: %v", status)
				}
				converted := call(t, ctx, cs, "maestro_apply", map[string]any{"operation": "content.convert", "source_path": "sample.txt"}, false)
				result := converted["result"].(map[string]any)
				if result["status"] != "unavailable" {
					t.Fatalf("initial MCP conversion used obsolete pack: %v", converted)
				}
				must(t, cs.Close())
				sawInitialReview = true
			}
			return body, err
		}
		priorReview, e := setup.ReviewInstallation(priorRequest, priorConfig)
		must(t, e)
		priorResult, e := setup.Apply(ctx, priorRequest, priorConfig, priorReview.Digest, true)
		must(t, e)
		if !sawInitialReview || priorResult.State != "ready" || priorResult.Content.PackSHA256 != next.packSHA {
			t.Fatalf("initial changed-pack placement not complete: %+v", priorResult)
		}
		priorState, e := installtx.ReadSourceState(filepath.Join(priorUser, ".maestro-install"), priorManaged)
		must(t, e)
		if priorState.SchemaVersion != 4 || priorState.Source != nil || priorState.Binding.ManifestSHA256 != next.verified.ManifestSHA256 {
			t.Fatal("initial Current was not exact before enrollment")
		}
		beforeInitialGuard := snapshot(t, priorManaged)
		if _, e = installtx.ValidateActivatedCurrent(priorManaged, filepath.Join(priorUser, ".maestro-install"), priorState.Binding); e != nil {
			t.Fatal(e)
		}
		wrongInitial := priorState.Binding
		wrongInitial.ManifestSHA256 = n.verified.ManifestSHA256
		publishedWrongInitial := false
		if e = installtx.GuardActivatedCurrent(priorManaged, filepath.Join(priorUser, ".maestro-install"), wrongInitial, func() error { publishedWrongInitial = true; return nil }); e == nil || publishedWrongInitial {
			t.Fatal("stale initial Current guard published")
		}
		if !reflect.DeepEqual(beforeInitialGuard, snapshot(t, priorManaged)) {
			t.Fatal("initial Current read/refusal changed managed bytes")
		}

		afterPrior := snapshot(t, priorRoot)
		for path, identity := range priorBytes {
			if afterPrior[path] != identity {
				t.Fatalf("prior converter bytes/mode changed: %s", path)
			}
		}
		var selection struct {
			ManifestSHA256 string `json:"manifest_sha256"`
		}
		must(t, json.Unmarshal(read(t, filepath.Join(priorRoot, "current.json")), &selection))
		if selection.ManifestSHA256 != next.verified.ManifestSHA256 {
			t.Fatal("initial selection retained stale manifest")
		}
		t.Logf("%s: compatible legacy N -> initial schema4 Current N1 reviewed/replaced before enrollment; MCP availability/use refused stale metadata; prior bytes preserved", scenario.name)

	}
	interruptFirstPlacement = true
	user := filepath.Join(root, "owner")
	ws := filepath.Join(root, "workspace")
	must(t, os.Mkdir(user, 0700))
	must(t, os.Mkdir(ws, 0700))
	request := setup.Request{ReleaseDirectory: n.directory, ManagedRoot: filepath.Join(root, "managed"), UserHome: user, Workspace: ws, CodePacket: filepath.Join(root, "code-initial"), DesktopPacket: filepath.Join(root, "desktop-initial.mcpb")}
	review, e := setup.ReviewInstallation(request, config)
	must(t, e)
	result, e := setup.Apply(ctx, request, config, review.Digest, true)
	if e == nil || result.State != "activated_setup_pending" || !strings.Contains(e.Error(), "synthetic first-placement interruption") {
		t.Fatalf("first placement interruption was not truthful: %+v %v", result, e)
	}
	brainFile := filepath.Join(result.Home, "brain", "runtime-owner-note.md")
	must(t, os.WriteFile(brainFile, []byte("owner brain must remain byte-identical"), 0600))
	if _, e := os.Lstat(filepath.Join(result.Home, "state/ingestion/docling")); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("interrupted initial converter unexpectedly placed: %v", e)
	}
	seed := installer.SeededOperationOptions{Self: filepath.Join(request.ManagedRoot, "maestro-install"), Version: "0.3.7", AuthorityRegistrySHA256: sum(registryBody), ManagedRoot: request.ManagedRoot, UserHome: user, DataRoot: filepath.Join(user, ".maestro-install"), VerifyNative: verifyNative}
	syncRoot := filepath.Join(root, "synced")
	tuple := releasecontract.SourceTuple{SourceID: "runtime-fixture", Channel: "canary", OS: runtime.GOOS, Arch: runtime.GOARCH}
	now := time.Now().UTC().Truncate(time.Second)
	publish := func(sequence uint64, v built) {
		g := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: v.verified.Manifest.Issuer, Source: tuple, Sequence: sequence, PublishedAt: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(24 * time.Hour).Format(time.RFC3339), Release: v.verified.Manifest.Release, ManifestSHA256: v.verified.ManifestSHA256, ClientInputsSHA256: v.verified.Manifest.Delivery.ClientInputsSHA256, Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}
		_, e := releasepack.PublishGeneration(releasepack.PublishGenerationOptions{Root: syncRoot, ReleaseDirectory: v.directory, Registry: registryPath, Generation: g, PrivateKey: private})
		must(t, e)
	}
	publish(1, n)
	sourceOptions := installer.SeededSourceOptions{SeededOperationOptions: seed, SourceRoot: syncRoot, ExpectedSource: tuple}
	sr, e := installer.ReviewSourceSeeded(ctx, sourceOptions)
	must(t, e)
	sourceOptions.ReviewDigest = sr.Digest
	_, e = installer.CheckSourceSeeded(ctx, sourceOptions)
	if !errors.Is(e, updateservice.ErrCurrent) {
		t.Fatal(e)
	}
	clients := installer.ClientOptions{SeededOperationOptions: seed, Request: installer.ClientRequest{CarrierID: strings.Repeat("a", 32), Workspace: ws, CodeRoot: filepath.Join(root, "code-current"), DesktopPath: filepath.Join(root, "desktop-current.mcpb"), CodeScope: "user", VersionedSlots: true}}
	cr, e := installer.ReviewClientsSeeded(ctx, clients)
	must(t, e)
	_, e = setup.ApplyClients(ctx, clients, cr.Digest, true)
	must(t, e)
	process := filepath.Join(root, "manager.test")
	compile := exec.CommandContext(ctx, "go", "test", "-mod=readonly", "-c", "-o", process, "./cmd/maestro-install")
	compile.Dir = filepath.Join(source, "adapters/mcp")
	if out, e := compile.CombinedOutput(); e != nil {
		t.Fatalf("manager compile %s %v", out, e)
	}
	fixture := filepath.Join(root, "manager.json")
	body, _ := json.Marshal(map[string]any{"Self": seed.Self, "Version": seed.Version, "Pin": seed.AuthorityRegistrySHA256, "SeedSHA": seedSHA, "ManagedRoot": seed.ManagedRoot, "UserHome": user, "Client": clients.Request})
	must(t, os.WriteFile(fixture, body, 0600))
	manager := func(action string, wantSuccess bool) string {
		actionStarted := time.Now()
		t.Logf("journey timing: manager %s start elapsed=%s", action, actionStarted.Sub(journeyStarted))
		command := exec.CommandContext(ctx, process, "-test.run=^TestRuntimeCurrentnessManagerProcess$", "-test.v")
		command.Env = append(os.Environ(), "MAESTRO_RUNTIME_MANAGER_FIXTURE="+fixture, "MAESTRO_RUNTIME_MANAGER_ACTION="+action)
		out, e := command.CombinedOutput()
		actionFinished := time.Now()
		t.Logf("journey timing: manager %s finish elapsed=%s duration=%s error=%v", action, actionFinished.Sub(journeyStarted), actionFinished.Sub(actionStarted), e)
		if wantSuccess && e != nil {
			if retained != "" {
				diagnostic := map[string]any{"evidence": "synthetic_failure_diagnostic_not_qualification", "action": action, "error": e.Error(), "context_error": ctx.Err() != nil, "root": root}
				captured := out
				if len(captured) > 64<<10 {
					captured = captured[:64<<10]
					diagnostic["manager_output_truncated"] = true
				}
				must(t, os.WriteFile(filepath.Join(root, "failed-manager-output.log"), captured, 0600))
				current, readErr := installtx.ReadSourceState(seed.DataRoot, seed.ManagedRoot)
				if readErr != nil {
					diagnostic["current_state_error"] = readErr.Error()
				} else {
					name := "maestro-mcp"
					if current.TargetOS == "windows" {
						name += ".exe"
					}
					server := filepath.Join(seed.ManagedRoot, "bin", name)
					diagnostic["current_manifest"] = current.Binding.ManifestSHA256
					diagnostic["expected_server_sha256"] = current.Server.SHA256
					actual, hashErr := installer.ClientFileDigest(server)
					diagnostic["actual_server_sha256"] = actual
					if hashErr != nil {
						diagnostic["server_hash_error"] = hashErr.Error()
					}
					paths, pathErr := installer.ClientGenerationPaths(clients.Request, current.Binding.ManifestSHA256)
					if pathErr != nil {
						diagnostic["generation_error"] = pathErr.Error()
					} else {
						stage := paths.DesktopPath + ".candidate-" + clients.Request.CarrierID + "-" + current.Binding.ManifestSHA256 + ".mcpb"
						diagnostic["mcpb_argv_reconstructed_from_original_generator_not_process_trace"] = []string{server, "mcpb", "--destination", stage, "--server", server, "--version", current.Release, "--product-root", seed.ManagedRoot, "--user-home", user, "--workspace", ws, "--installed-manifest", current.Binding.ManifestSHA256, "--installed-client-inputs", current.Binding.ClientInputsSHA256, "--carrier-profile", installer.StableClientProfile, "--carrier-id", clients.Request.CarrierID, "--carrier-version", installer.StableCarrierVersion}
						if info, statErr := os.Lstat(stage); statErr == nil {
							diagnostic["mcpb_stage_bytes"], diagnostic["mcpb_stage_mode"] = info.Size(), info.Mode().String()
						} else {
							diagnostic["mcpb_stage_error"] = statErr.Error()
						}
					}
					// The original read-only identity probe retains its own 15s limit.
					identity, identityErr := releasecontract.ReadServerIdentity(server)
					diagnostic["identity_probe"] = map[string]any{"version": identity.Version, "embedded_inputs_sha256": identity.EmbeddedInputsSHA256, "resources": len(identity.Resources)}
					if identityErr != nil {
						diagnostic["identity_probe_error"] = identityErr.Error()
					}
				}
				body, marshalErr := json.MarshalIndent(diagnostic, "", "  ")
				must(t, marshalErr)
				must(t, os.WriteFile(filepath.Join(root, "failed-manager-diagnostic.json"), body, 0600))
				t.Logf("retained exact synthetic failure at %s; child stderr was discarded by original clientCommand and is not fabricated", root)
			}
			t.Fatalf("actual manager %s: %s %v", action, out, e)
		}
		if !wantSuccess && e == nil {
			t.Fatalf("declined manager unexpectedly succeeded: %s", out)
		}
		return string(out)
	}
	// First setup failed before any converter metadata existed. Source enrollment
	// says Current and deliberately has not produced an update snapshot.
	firstState, e := installtx.ValidateQualifiedLaunch(seed.ManagedRoot, seed.DataRoot, installtx.ReleaseBinding{}, time.Now())
	must(t, e)
	if _, e = installtx.ValidateActivatedCurrent(seed.ManagedRoot, seed.DataRoot, firstState.Binding); e == nil {
		t.Fatal("enrolled source entered initial Current reader")
	}
	initialPublished := false
	if e = installtx.GuardActivatedCurrent(seed.ManagedRoot, seed.DataRoot, firstState.Binding, func() error { initialPublished = true; return nil }); e == nil || initialPublished {
		t.Fatal("enrolled source entered initial Current guard")
	}
	firstSnapshot := filepath.Join(seed.DataRoot, "updates", "synced-"+firstState.Source.GenerationSHA256)
	if _, e = os.Lstat(firstSnapshot); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("Current source check unexpectedly staged: %v", e)
	}
	beforeRecoveryReview := snapshot(t, root)
	recoveryReview, e := setup.ReviewRuntimeRecoverySource(ctx, clients)
	must(t, e)
	if !reflect.DeepEqual(beforeRecoveryReview, snapshot(t, root)) {
		t.Fatal("snapshot recovery review wrote files")
	}
	if _, e = setup.ApplyRuntimeRecoverySource(ctx, clients, recoveryReview.Digest, false); e == nil {
		t.Fatal("unconfirmed snapshot staged")
	}
	if _, e = setup.ApplyRuntimeRecoverySource(ctx, clients, strings.Repeat("0", 64), true); e == nil {
		t.Fatal("stale snapshot digest staged")
	}
	if !reflect.DeepEqual(beforeRecoveryReview, snapshot(t, root)) {
		t.Fatal("unconfirmed/stale snapshot altered owner state")
	}
	declinedSnapshot := manager("reconcile-decline-snapshot", false)
	if !strings.Contains(declinedSnapshot, "snapshot staging declined") {
		t.Fatalf("snapshot decline not explicit: %s", declinedSnapshot)
	}
	if !reflect.DeepEqual(beforeRecoveryReview, snapshot(t, root)) {
		t.Fatal("declined snapshot altered owner state")
	}
	recoveredInitial := manager("reconcile", true)
	if !strings.Contains(recoveredInitial, "stage_current_authenticated_release_snapshot") {
		t.Fatalf("initial snapshot effect was not reviewed: %s", recoveredInitial)
	}
	initialRuntime, e := setup.ReviewRuntimeUpdate(ctx, clients)
	must(t, e)
	if initialRuntime.Availability.State != "ready" || initialRuntime.Availability.PackSHA256 != n.packSHA {
		t.Fatalf("initial recovery converter %+v", initialRuntime)
	}
	if _, e = os.Stat(firstSnapshot); e != nil {
		t.Fatal("reviewed Current snapshot missing", e)
	}
	beforeExistingSnapshotReview := snapshot(t, root)
	if _, e = setup.ReviewRuntimeRecoverySource(ctx, clients); e == nil {
		t.Fatal("existing Current snapshot admitted for automatic restaging")
	}
	if !reflect.DeepEqual(beforeExistingSnapshotReview, snapshot(t, root)) {
		t.Fatal("existing snapshot or owner data changed during refused restaging")
	}
	// Recovery begins source-aware, so original fixed metadata remains absent.
	if _, e = os.Lstat(filepath.Join(result.Home, "state/ingestion/docling/release-manifest.json")); !errors.Is(e, os.ErrNotExist) {
		t.Fatalf("source-aware recovery invented legacy metadata: %v", e)
	}
	t.Logf("first-placement interruption -> source Current with no snapshot -> reviewed manager reconcile recovered actual consumer (%d output bytes)", len(recoveredInitial))
	publish(2, next)
	// Declining the converter after actual activation leaves a truthful partial.
	declined := manager("update-decline-runtime", false)
	if !strings.Contains(declined, "source-updated/runtime-pending") {
		t.Fatalf("partial outcome concealed: %s", declined)
	}
	partial, e := installtx.ValidateQualifiedLaunch(seed.ManagedRoot, seed.DataRoot, installtx.ReleaseBinding{}, time.Now())
	must(t, e)
	if partial.Release != "0.1.43" {
		t.Fatal("backend activation was fabricated as rolled back")
	}
	pendingRuntime, e := setup.ReviewRuntimeUpdate(ctx, clients)
	must(t, e)
	if pendingRuntime.Availability.State != "unavailable" || pendingRuntime.Availability.Reason != "current_pack_pending" {
		t.Fatalf("stale runtime falsely ready after decline: %+v", pendingRuntime)
	}
	currentExecutable := filepath.Join(seed.ManagedRoot, "bin", "maestro-mcp")
	rejected := exec.CommandContext(ctx, currentExecutable, "runtime-review", "--product-root", seed.ManagedRoot, "--user-home", user, "--workspace", ws, "--installed-manifest", partial.Binding.ManifestSHA256, "--installed-client-inputs", partial.Binding.ClientInputsSHA256, "--release-directory", n.directory)
	if output, e := rejected.CombinedOutput(); e == nil {
		t.Fatalf("signed other release accepted for current runtime: %s", output)
	}
	if _, e := setup.ApplyRuntimeUpdate(ctx, clients, pendingRuntime.Digest, false); e == nil {
		t.Fatal("unconfirmed runtime applied")
	}
	// Recovery operates after the backend is already Current, using the same owner.
	completed := manager("reconcile", true)
	t.Logf("actual manager reconcile after source-updated/runtime-pending passed (%d output bytes)", len(completed))
	state, e := installtx.ValidateQualifiedLaunch(seed.ManagedRoot, seed.DataRoot, installtx.ReleaseBinding{}, time.Now())
	must(t, e)
	if state.Release != "0.1.43" || state.Source.Sequence != 2 {
		t.Fatalf("update source custody %+v", state)
	}
	rr, e := setup.ReviewRuntimeUpdate(ctx, clients)
	must(t, e)
	if rr.Availability.State != "ready" || rr.Availability.PackSHA256 != next.packSHA || rr.Requirement.ServerVersion != "0.1.43" || rr.Requirement.PackSHA256 == n.packSHA {
		t.Fatalf("current converter %+v", rr)
	}
	// Same-pack update/reconcile must not repeat placement or manufacture native proof.
	same := manager("update", true)
	t.Logf("actual Current update/readback passed (%d output bytes)", len(same))
	// A saved exact runtime review becomes stale when the independently accepted
	// source/current changes; it is not reusable approval for another release.
	savedReview := rr
	rollback := manager("rollback", true)
	t.Logf("actual manager rollback passed (%d output bytes)", len(rollback))
	state, e = installtx.ValidateQualifiedLaunch(seed.ManagedRoot, seed.DataRoot, installtx.ReleaseBinding{}, time.Now())
	must(t, e)
	if state.Release != "0.1.42" || state.Source.Sequence != 2 {
		t.Fatalf("rollback lost advanced custody %+v", state)
	}
	rr, e = setup.ReviewRuntimeUpdate(ctx, clients)
	must(t, e)
	if rr.Availability.State != "ready" || rr.Availability.PackSHA256 != n.packSHA {
		t.Fatalf("retained rollback %+v", rr)
	}
	if _, e = setup.ApplyRuntimeUpdate(ctx, clients, savedReview.Digest, true); e == nil {
		t.Fatal("stale runtime review accepted after rollback")
	}
	if !bytes.Equal(read(t, brainFile), []byte("owner brain must remain byte-identical")) {
		t.Fatal("owner brain changed")
	}
	if _, e = os.Lstat(filepath.Join(result.Home, "state/ingestion/docling/release-manifest.json")); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("original legacy absence overwritten", e)
	}
	for _, sha := range []string{n.packSHA, next.packSHA} {
		if _, e = os.Stat(filepath.Join(result.Home, "state/ingestion/docling/installed", sha, "inert-fixture.txt")); e != nil {
			t.Fatal("retained prior payload missing", e)
		}
	}
	t.Logf("PASS actual manager/consumer N=%s -> N1=%s -> rollback N; source floor=%d; legacy absence/brain retained; installer=%s actual_server=%s", n.packSHA, next.packSHA, state.Source.Sequence, seed.Version, state.Server.Version)
}

func runtimeInertPack(t *testing.T, root, source, version, marker string) string {
	t.Helper()
	descriptor, e := docling.DescribeProfile(runtime.GOOS, runtime.GOARCH, docling.LegacyProfile)
	must(t, e)
	type file struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
		Size   int64  `json:"size"`
		Mode   int64  `json:"mode"`
	}
	entries := map[string][]byte{"bridge.py": read(t, filepath.Join(source, "internal/ingest/docling/runtime/bridge.py")), "pylock.toml": read(t, filepath.Join(source, "internal/ingest/docling/runtime/pylock.toml")), "inputs.json": read(t, filepath.Join(source, "internal/ingest/docling/runtime/inputs.json")), "parser-licenses.json": read(t, filepath.Join(source, "internal/ingest/docling/runtime/parser-licenses.json")), "inert-fixture.txt": []byte(marker)}
	var files []file
	paths := []string{}
	for path := range entries {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	for _, path := range paths {
		body := entries[path]
		files = append(files, file{path, sum(body), int64(len(body)), 0444})
	}
	body, e := json.Marshal(map[string]any{"schema_version": 1, "adapter": "docling", "version": docling.DoclingVersion, "os": runtime.GOOS, "arch": runtime.GOARCH, "compatible_cli": ">=0.1.0 <0.2.0", "python": docling.PythonVersion, "python_archive_sha256": descriptor.Python.SHA256, "lock_sha256": descriptor.LockSHA256, "models": []string{}, "inputs": descriptor.Inputs, "files": files})
	must(t, e)
	// pack.json uses its canonical typed field order, as the runtime requires.
	var canonical struct {
		SchemaVersion       int             `json:"schema_version"`
		Adapter             string          `json:"adapter"`
		Version             string          `json:"version"`
		OS                  string          `json:"os"`
		Arch                string          `json:"arch"`
		CompatibleCLI       string          `json:"compatible_cli"`
		Python              string          `json:"python"`
		PythonArchiveSHA256 string          `json:"python_archive_sha256"`
		LockSHA256          string          `json:"lock_sha256"`
		Models              []string        `json:"models"`
		Inputs              []docling.Input `json:"inputs"`
		Files               []file          `json:"files"`
	}
	must(t, json.Unmarshal(body, &canonical))
	body, e = json.Marshal(canonical)
	must(t, e)
	entries["pack.json"] = body
	paths = append(paths, "pack.json")
	sort.Strings(paths)
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	for _, path := range paths {
		body := entries[path]
		must(t, tw.WriteHeader(&tar.Header{Name: path, Mode: 0444, Size: int64(len(body)), Typeflag: tar.TypeReg}))
		_, e = tw.Write(body)
		must(t, e)
	}
	must(t, tw.Close())
	must(t, gz.Close())
	path := filepath.Join(root, fmt.Sprintf("inert-%s.tar.gz", version))
	must(t, os.WriteFile(path, archive.Bytes(), 0600))
	return path
}

// Authenticated old release with an explicitly compatible runtime range. The
// new installed release still requires its own exact manifest and archive.
func runtimeCompatibleLegacyRelease(t *testing.T, root, name string, old releaseverify.VerifiedRelease, registry releaseverify.KeyRegistry, key ed25519.PrivateKey, replacement []byte) releaseverify.VerifiedRelease {
	t.Helper()
	destination := filepath.Join(root, "legacy-compatible-release-"+name)
	must(t, os.Mkdir(destination, 0700))
	entries, e := os.ReadDir(old.Directory)
	must(t, e)
	for _, entry := range entries {
		must(t, os.WriteFile(filepath.Join(destination, entry.Name()), read(t, filepath.Join(old.Directory, entry.Name())), 0600))
	}
	manifest := old.Manifest
	manifest.Artifacts = append([]releasecontract.Artifact(nil), old.Manifest.Artifacts...)
	if replacement != nil {
		for i := range manifest.Artifacts {
			a := &manifest.Artifacts[i]
			if a.Kind == "runtime_pack" {
				must(t, os.WriteFile(filepath.Join(destination, a.Name), replacement, 0600))
				must(t, os.WriteFile(filepath.Join(destination, a.SignatureRef), ed25519.Sign(key, replacement), 0600))
				a.Size = int64(len(replacement))
				a.SHA256 = sum(replacement)
			}
		}
	}
	manifest.Bundle.CompatibleServer = ">=0.1.0 <0.2.0"
	manifest.Delivery.ClientInputsSHA256, e = manifest.ClientInputsDigest()
	must(t, e)
	body, e := json.Marshal(manifest)
	must(t, e)
	must(t, os.WriteFile(filepath.Join(destination, releaseverify.ManifestName), body, 0600))
	must(t, os.WriteFile(filepath.Join(destination, releaseverify.ManifestSignatureName), ed25519.Sign(key, body), 0600))
	verified, e := releaseverify.VerifyDirectory(destination, registry)
	must(t, e)
	return verified
}
