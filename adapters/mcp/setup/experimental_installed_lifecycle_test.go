//go:build maestro_experimental_installed_seed

package setup_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/pilotdocument"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseprovider"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This is a source-only engineering rehearsal using actual tagged seed/server
// children and ephemeral authority. Distinct inert runtime artifacts test
// finite release binding and current-consumer placement, NOT document conversion, Code or
// Desktop qualification. Those require their genuine independent evidence.
func TestExperimentalInstalledSeedFiniteSourceLifecycle(t *testing.T) {
	experimentalInstalledSeedFiniteSourceLifecycle(t, false, false)
}

// Reuse the exact finite fixture while avoiding unrelated runtime/update tails.
func TestDesktopOnlyExperimentalCurrentConsumer(t *testing.T) {
	experimentalInstalledSeedFiniteSourceLifecycle(t, true, false)
}

func experimentalInstalledSeedFiniteSourceLifecycle(t *testing.T, desktopConsumerOnly, pendingCodeConsumerOnly bool) {
	experimentalRuntimeFixtureHost(t)
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	root := t.TempDir()
	t.Cleanup(func() {
		filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
			if err == nil && entry.IsDir() {
				os.Chmod(path, 0700)
			}
			return err
		})
	})
	user, managed := filepath.Join(root, "user"), filepath.Join(root, "managed")
	brain, ws := filepath.Join(root, "Developer", "existing-brain"), filepath.Join(root, "workspace")
	t.Setenv("MAESTRO_HOME", brain)
	must(t, os.MkdirAll(user, 0700))
	priorProduct := filepath.Join(root, "prior-product")
	must(t, os.Mkdir(priorProduct, 0700))
	home, err := datahome.InitializeFresh(datahome.Options{ProductRoot: priorProduct, UserHome: user})
	must(t, err)
	brainFile := filepath.Join(home.Home, "literal-project.md")
	must(t, os.WriteFile(brainFile, []byte("existing synthetic brain: retain these exact bytes"), 0600))
	initialized, err := workspace.Initialize(workspace.Options{WorkspacePath: ws, DataRoot: filepath.Join(home.Home, "state")})
	must(t, err)

	source, err := filepath.Abs("../../..")
	must(t, err)
	now := time.Now().UTC().Truncate(time.Second)
	issuer, keyID := "maestro-engineering-pilot-"+now.Format("20060102"), "pilot-"+now.Format("20060102")
	public, private, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	registryBody, err := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": issuer, "key_id": keyID, "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": now.Add(-time.Hour).Format(time.RFC3339), "valid_until": now.Add(13 * 24 * time.Hour).Format(time.RFC3339)}}})
	must(t, err)
	registryPath := filepath.Join(root, "registry.json")
	must(t, os.WriteFile(registryPath, registryBody, 0600))
	registry, err := releaseverify.LoadAuthorityRegistry(registryPath, time.Now)
	must(t, err)
	type built struct {
		factory releasepack.ModernCandidate
		release releaseverify.VerifiedRelease
		dir     string
	}
	build := func(version string) built {
		pack := experimentalRuntimeInertPack(t, root, source, version)
		f, e := releasepack.BuildModernCandidate(ctx, releasepack.ModernOptions{Root: source, Output: filepath.Join(root, "factory-"+version), Version: version, InstallerVersion: "0.1.51", Channel: "canary", Target: releasepack.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}, AuthorityRegistry: registryPath, DoclingPack: pack, ExperimentalInstalledSeed: true})
		must(t, e)
		dir := filepath.Join(root, "signed-"+version)
		_, e = releasepack.SignCandidate(releasepack.SignCandidateOptions{Candidate: f.Directory, Output: dir, Registry: registryPath, Issuer: issuer, KeyID: keyID, PrivateKey: private})
		must(t, e)
		v, e := releaseverify.VerifyDirectory(dir, registry)
		must(t, e)
		return built{f, v, dir}
	}
	n, next := build("0.1.51"), build("0.1.52")
	if sum(read(t, n.factory.Installer)) != sum(read(t, next.factory.Installer)) {
		t.Fatal("independently versioned engineering installer changed")
	}
	pinRelease := func(b built) installer.ExperimentalSeedRelease {
		p := installer.ExperimentalSeedRelease{Release: b.release.Manifest.Release, ManifestSHA256: b.release.ManifestSHA256, ClientInputsSHA256: b.release.Manifest.Delivery.ClientInputsSHA256}
		for _, a := range b.release.Manifest.Artifacts {
			switch a.Kind {
			case "mcp_server":
				p.ServerSHA256 = a.SHA256
			case "bundle":
				p.BundleSHA256 = a.SHA256
			case "runtime_pack":
				p.RuntimeSHA256 = a.SHA256
			}
		}
		return p
	}
	tuple := releasecontract.SourceTuple{SourceID: "team-pilot", Channel: "canary", OS: runtime.GOOS, Arch: runtime.GOARCH}
	record := installer.ExperimentalSeedRecord{SchemaVersion: 1, Product: "maestro", Purpose: installer.ExperimentalSeedPurpose, Protocol: installer.ExperimentalSeedProtocol, Profile: installer.ExperimentalSeedProfile, RegistrySHA256: sum(registryBody), InstallerVersion: "0.1.51", InstallerSHA256: sum(read(t, n.factory.Installer)), TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH, Source: tuple, Releases: []installer.ExperimentalSeedRelease{pinRelease(n), pinRelease(next)}, Issuer: issuer, KeyID: keyID, NotBefore: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(7 * 24 * time.Hour).Format(time.RFC3339)}
	if record.Releases[0].RuntimeSHA256 == record.Releases[1].RuntimeSHA256 {
		t.Fatal("different-pack fixture did not bind distinct runtime artifacts")
	}
	body, err := json.Marshal(record)
	must(t, err)
	admissionNow := now
	role, err := installer.VerifyExperimentalSeedAdmission(body, ed25519.Sign(private, body), registryBody, sum(registryBody), func() time.Time { return admissionNow })
	must(t, err)
	trust := filepath.Join(filepath.Dir(n.factory.Installer), "trust")
	must(t, os.WriteFile(filepath.Join(trust, installer.ExperimentalSeedAdmissionName), body, 0600))
	must(t, os.WriteFile(filepath.Join(trust, installer.ExperimentalSeedAdmissionName+".sig"), ed25519.Sign(private, body), 0600))
	o := installer.Options{ExperimentalSeed: role, UserHome: user, ManagedRoot: managed, DataRoot: filepath.Join(user, ".maestro-install"), ReleaseDir: n.dir, Bootstrapper: n.factory.Installer, AuthorityRegistry: registryPath, TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH}
	plan, _, err := installer.Prepare(o)
	must(t, err)
	if plan.NativeTrustMode != string(installer.NativeTrustExperimentalInstalled) || plan.ExperimentalSeed == nil || plan.ExperimentalSeed.Evidence != "engineering_only" || plan.ExperimentalBrainHome != home.Home {
		t.Fatal("first install lost engineering class or existing brain")
	}
	o.ExpectedPlanDigest = plan.PlanDigest
	installed, err := installer.Install(ctx, o)
	must(t, err)
	seed := installer.SeededOperationOptions{ExperimentalSeed: role, Self: filepath.Join(managed, sourceTestExecutable("maestro-install")), Version: record.InstallerVersion, AuthorityRegistrySHA256: record.RegistrySHA256, UserHome: user, ManagedRoot: managed, DataRoot: o.DataRoot}
	must(t, installer.VerifyExperimentalInstalledBinding(seed))
	if installed.CLIPath == "" || !strings.Contains(installed.Output, "activated") {
		t.Fatal("actual engineering activation child did not acknowledge")
	}
	synced := filepath.Join(root, "synced")
	publish := func(sequence uint64, release built) {
		g := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: release.release.Manifest.Issuer, Source: tuple, Sequence: sequence, PublishedAt: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(6 * 24 * time.Hour).Format(time.RFC3339), Release: release.release.Manifest.Release, ManifestSHA256: release.release.ManifestSHA256, ClientInputsSHA256: release.release.Manifest.Delivery.ClientInputsSHA256, Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}
		_, e := releasepack.PublishGeneration(releasepack.PublishGenerationOptions{Root: synced, ReleaseDirectory: release.dir, Registry: registryPath, Generation: g, PrivateKey: private})
		must(t, e)
	}
	sourceOptions := installer.SeededSourceOptions{SeededOperationOptions: seed, SourceRoot: synced, ExpectedSource: tuple}
	check := func() (updateservice.CheckResult, error) {
		r, e := installer.ReviewSourceSeeded(ctx, sourceOptions)
		must(t, e)
		sourceOptions.ReviewDigest = r.Digest
		return installer.CheckSourceSeeded(ctx, sourceOptions)
	}
	publish(1, n)
	checked, err := check()
	if !errors.Is(err, updateservice.ErrCurrent) {
		t.Fatalf("initial finite enrollment: %v", err)
	}
	assertContext := func() {
		current, e := installtx.ReadSourceState(o.DataRoot, managed)
		must(t, e)
		client := connectQualifiedSource(t, ctx, seed, current.Binding, ws, "")
		defer client.Close()
		value := call(t, ctx, client, "maestro_context", map[string]any{}, false)
		if value["workspace_id"] != initialized.WorkspaceID {
			t.Fatalf("actual tagged child changed workspace: %#v", value)
		}
	}
	assertContext()
	if pendingCodeConsumerOnly {
		assertPendingCodeDesktopConsumer(t, ctx, seed, ws, home.Home, brainFile, initialized.WorkspaceID, root)
		return
	}
	// Explicit resources-only fixture coverage is separate from full native
	// enforcement. A full request without the genuine qualifier must fail.
	codeHost, err := os.Executable()
	must(t, err)
	packetRoot := filepath.Join(root, "private-packets")
	must(t, os.Mkdir(packetRoot, 0700))
	desktop := installer.ClientOptions{SeededOperationOptions: seed, Request: installer.ClientRequest{Target: "desktop", VersionedSlots: true, CarrierID: strings.Repeat("d", 32), Workspace: ws, DesktopPath: filepath.Join(packetRoot, "desktop-only.mcpb"), OwnerReviewChannel: "local-browser"}}
	assertDesktop := func() installer.ClientRecord {
		t.Helper()
		review, e := installer.ReviewClientsSeeded(ctx, desktop)
		must(t, e)
		if strings.Contains(strings.Join(review.Effects, " "), "publish_fixed_current_record_Code_command_source") {
			t.Fatal("Desktop review advertises Code publication")
		}
		r, e := setup.ApplyClients(ctx, desktop, review.Digest, true)
		must(t, e)
		status, e := installer.ReadClientsSeeded(ctx, desktop)
		must(t, e)
		if status.Record == nil || r.SchemaVersion != 2 || status.CodeRegistration != "not_selected" || r.Materialization.CodeRoot != "" || r.Materialization.DesktopMethods == nil {
			t.Fatal("Desktop-only current publication differs")
		}
		launch, e := installer.PrepareStableLaunch(ctx, desktop)
		must(t, e)
		command := exec.CommandContext(ctx, launch.Executable, launch.Args...)
		var stderr bytes.Buffer
		command.Stderr = &stderr
		client, e := mcp.NewClient(&mcp.Implementation{Name: "desktop-only-current-consumer", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
		if e != nil {
			t.Fatalf("Desktop actual current SDK startup: %v: %s", e, stderr.String())
		}
		value := call(t, ctx, client, "maestro_context", map[string]any{}, false)
		if value["workspace_id"] != initialized.WorkspaceID {
			t.Fatal("Desktop actual SDK workspace differs")
		}
		call(t, ctx, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "maestro-onboarding"}, false)
		must(t, client.Close())
		handoff, e := installer.PrepareClientManagementHandoff(ctx, desktop)
		must(t, e)
		if filepath.Dir(handoff.Launcher) != installer.ClientMethodsRoot(r.Materialization) {
			t.Fatal("Desktop manager requires Code slot")
		}
		if _, e := installer.CurrentClientPluginPath(ctx, desktop); e == nil {
			t.Fatal("Desktop fabricated current Code plugin")
		}
		if _, e := installer.PrepareStableLifecycleHook(ctx, desktop, "SessionStart"); e == nil {
			t.Fatal("Desktop fabricated Code hook")
		}
		return r
	}
	// Exercise the original installed diagnostic consumer without Code selection.
	checkDesktop := func(r installer.ClientRecord) {
		t.Helper()
		args := []string{"installed-pilot-check", "--product-root", managed, "--user-home", user, "--workspace", ws, "--carrier-id", r.CarrierID, "--installer-version", record.InstallerVersion, "--experimental-seed-sha256", role.Identity().AdmissionSHA256, "--experimental-installed-root", managed, "--check-only"}
		output, _ := exec.CommandContext(ctx, installed.CLIPath, args...).Output()
		var report struct {
			Target string `json:"client_target"`
			Checks []struct{ Name, Status, Reason string }
		}
		must(t, json.Unmarshal(output, &report))
		if report.Target != "desktop" {
			t.Fatalf("actual diagnostic lost admitted Desktop target: %s", output)
		}
		foundSDK := false
		for _, check := range report.Checks {
			if check.Name == "mcp_connection" && check.Status == "passed" {
				foundSDK = true
			}
			if check.Status == "failed" && (strings.Contains(check.Name, "code") || strings.Contains(check.Reason, "Code")) {
				t.Fatalf("Desktop diagnostic requires Code: %+v", check)
			}
		}
		if !foundSDK {
			t.Fatalf("Desktop diagnostic did not observe actual SDK consumer: %s", output)
		}
	}
	initialDesktop := assertDesktop()
	checkDesktop(initialDesktop)
	if repeated := assertDesktop(); repeated.Materialization.DesktopSHA256 != initialDesktop.Materialization.DesktopSHA256 {
		t.Fatal("Desktop replay changed connector")
	}
	if desktopConsumerOnly {
		return
	}
	full := installer.ClientOptions{SeededOperationOptions: seed, Request: installer.ClientRequest{VersionedSlots: true, NativeEnforcement: true, GenerationID: strings.Repeat("2", 32), CarrierID: strings.Repeat("3", 32), Workspace: ws, CodeRoot: filepath.Join(packetRoot, "native-code"), DesktopPath: filepath.Join(packetRoot, "native.mcpb"), CodeExecutable: codeHost, CodeScope: "user"}}
	fullReview, err := installer.ReviewClientsSeeded(ctx, full)
	must(t, err)
	if _, err = setup.ApplyClients(ctx, full, fullReview.Digest, true); err == nil || !strings.Contains(err.Error(), "genuine qualifier") {
		t.Fatalf("full native profile accepted missing genuine proof: %v", err)
	}
	clients := installer.ClientOptions{SeededOperationOptions: seed, Request: installer.ClientRequest{VersionedSlots: true, LifecycleContext: true, CarrierID: strings.Repeat("1", 32), Workspace: ws, CodeRoot: filepath.Join(packetRoot, "resource-code"), DesktopPath: filepath.Join(packetRoot, "resources.mcpb"), CodeScope: "user"}}
	clients.Request.OwnerReviewChannel = "local-browser"
	placeRuntime := func(expected built) {
		review, e := setup.ReviewRuntimeUpdate(ctx, clients)
		if e != nil {
			state, stateErr := installtx.ReadSourceState(o.DataRoot, managed)
			must(t, stateErr)
			launch, launchErr := installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: seed, Workspace: ws, ExpectedBinding: state.Binding})
			must(t, launchErr)
			launch.Args[0] = "runtime-review"
			output, childErr := exec.CommandContext(ctx, launch.Executable, launch.Args...).CombinedOutput()
			t.Fatalf("actual runtime review: %v; current child: %s: %v", e, output, childErr)
		}
		if review.Requirement.ManifestSHA256 != expected.release.ManifestSHA256 || review.Requirement.PackSHA256 != pinRelease(expected).RuntimeSHA256 {
			t.Fatal("actual current-consumer runtime review lost finite release identity")
		}
		availability, e := setup.ApplyRuntimeUpdate(ctx, clients, review.Digest, true)
		must(t, e)
		if availability.State != "ready" || availability.PackSHA256 != pinRelease(expected).RuntimeSHA256 {
			t.Fatalf("actual current-consumer inert placement unavailable: %+v", availability)
		}
	}
	materialize := func() installer.ClientRecord {
		review, e := installer.ReviewClientsSeeded(ctx, clients)
		must(t, e)
		if review.Request.OwnerReviewChannel != "local-browser" || !strings.Contains(strings.Join(review.Effects, " "), "both_Code_and_Desktop_without_approving_any_individual_write") {
			t.Fatal("original review lost the common-client exact-review disclosure")
		}
		r, e := setup.ApplyClients(ctx, clients, review.Digest, true)
		must(t, e)
		status, e := installer.ReadClientsSeeded(ctx, clients)
		must(t, e)
		if r.SchemaVersion != 1 || status.State != "current_materialization" || r.Materialization.NativeEnforcement || r.Materialization.OwnerReviewChannel != "local-browser" || installer.ClientRequestFromRecord(r).OwnerReviewChannel != "local-browser" {
			t.Fatal("explicit resources fixture changed record schema or native class")
		}
		launch, e := installer.PrepareStableLaunch(ctx, clients)
		must(t, e)
		if !strings.HasSuffix(strings.Join(launch.Args, " "), "--owner-review-channel local-browser") {
			t.Fatal("original stable launch lost the current-record review channel")
		}
		changed := clients
		changed.Request.OwnerReviewChannel = ""
		if _, e := installer.ReviewClientsSeeded(ctx, changed); e == nil {
			t.Fatal("an existing carrier changed its reviewed channel")
		}
		must(t, setup.VerifyRecordedDesktopContract(r.Materialization.DesktopPath, clients, runtime.GOOS, r.Materialization.DesktopNativeContractSHA256))
		plugin, e := installer.CurrentClientPluginPath(ctx, clients)
		must(t, e)
		if _, e = os.Stat(filepath.Join(plugin, "skills", "maestro-onboarding", "SKILL.md")); e != nil {
			t.Fatalf("actual current generator omitted onboarding skill: %v", e)
		}
		for _, event := range []string{"SessionStart", "UserPromptSubmit", "Stop"} {
			launch, e := installer.PrepareStableLifecycleHook(ctx, clients, event)
			must(t, e)
			command := exec.CommandContext(ctx, launch.Executable, launch.Args...)
			command.Stdin = strings.NewReader(`{"session_id":"synthetic-eisa","prompt_id":"synthetic-turn","cwd":` + string(mustJSON(t, ws)) + `,"hook_event_name":"` + event + `","stop_hook_active":false}`)
			if output, e := command.CombinedOutput(); e != nil {
				t.Fatalf("actual tagged passive hook %s: %s: %v", event, output, e)
			}
		}
		return r
	}
	initialClients := materialize()
	currentN, err := installtx.ReadSourceState(o.DataRoot, managed)
	must(t, err)
	selectedBinding := product.Options{ExperimentalSeed: role, ExperimentalSeedRoot: managed, ProductRoot: managed, UserHome: user, WorkspacePath: ws, AuthorityRegistryPath: filepath.Join(managed, "trust", "release-authority-registry.json"), AuthorityRegistrySHA256: record.RegistrySHA256, CLIVersion: currentN.Server.Version, InstalledManifestSHA256: currentN.Binding.ManifestSHA256, InstalledClientInputsSHA256: currentN.Binding.ClientInputsSHA256}
	if !t.Run("installed_owner_review_receiver", func(t *testing.T) {
		selected := selectedBinding
		selected.OwnerReviewChannel = "local-browser"
		selected.ManagementCarrierID, selected.ManagementInstallerVersion = clients.Request.CarrierID, seed.Version
		bound, e := product.Open(selected)
		must(t, e)
		for _, change := range []string{"missing-channel", "missing-carrier", "foreign-carrier", "foreign-installer-version"} {
			altered := selected
			switch change {
			case "missing-channel":
				altered.OwnerReviewChannel = ""
			case "missing-carrier":
				altered.ManagementCarrierID, altered.ManagementInstallerVersion = "", ""
			case "foreign-carrier":
				altered.ManagementCarrierID = strings.Repeat("f", 32)
			case "foreign-installer-version":
				altered.ManagementInstallerVersion = "0.0.0"
			}
			if _, e := product.Open(altered); e == nil {
				t.Fatalf("receiving constructor admitted %s", change)
			}
		}
		a, z := mcp.NewInMemoryTransports()
		server, e := bound.Server().Connect(ctx, a, nil)
		must(t, e)
		defer server.Close()
		client, e := mcp.NewClient(&mcp.Implementation{Name: "installed-review-source-fixture", Version: "1"}, nil).Connect(ctx, z, nil)
		must(t, e)
		defer client.Close()
		call(t, ctx, client, "maestro_context", map[string]any{}, false)
		path := filepath.Join(seed.DataRoot, "clients", clients.Request.CarrierID+".json")
		original := read(t, path)
		defer func() { must(t, os.WriteFile(path, original, 0600)) }()
		for _, change := range []string{"channel", "whole-record"} {
			altered := initialClients
			if change == "channel" {
				altered.Materialization.OwnerReviewChannel = ""
			} else {
				altered.Code.State = "synthetic-changed-observation"
			}
			must(t, os.WriteFile(path, mustJSON(t, altered), 0600))
			call(t, ctx, client, "maestro_context", map[string]any{}, true)
			must(t, os.WriteFile(path, original, 0600))
		}
		call(t, ctx, client, "maestro_context", map[string]any{}, false)
	}) {
		return
	}
	if !t.Run("private_packet_native_startup", func(t *testing.T) {
		candidate := filepath.Join(root, "private-startup-candidate")
		must(t, packaging.BuildPlugin(packaging.Options{Destination: candidate, Server: installed.CLIPath, Version: currentN.Server.Version, Binding: selectedBinding}))
		nonce := strings.Repeat("a", 32)
		args := installer.ExperimentalChildArgs([]string{"native-startup-check", "--nonce", nonce, "--packet", "--product-root", candidate, "--user-home", user, "--workspace", ws}, role, managed)
		command := exec.CommandContext(ctx, filepath.Join(candidate, "scripts", sourceTestExecutable("maestro-mcp")), args...)
		command.Stdin = strings.NewReader(`{"hook_event_name":"SessionStart","cwd":` + string(mustJSON(t, ws)) + `,"session_id":"synthetic-private-startup","stop_hook_active":false}`)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("actual private packet startup rejected distinct installed seed root: %s: %v", output, err)
		}
		var startup struct {
			SystemMessage string `json:"systemMessage"`
		}
		must(t, json.Unmarshal(output, &startup))
		if startup.SystemMessage != "maestro_native_startup_unchanged:"+nonce {
			t.Fatalf("private packet startup changed exact read-only nonce observation: %s", output)
		}
	}) {
		return
	}
	// Direct installer activation deliberately omitted first-setup placement.
	// Recover the exact accepted source snapshot through its separate existing
	// review/confirmation owner before asking the current consumer to place it.
	snapshotReview, err := setup.ReviewRuntimeRecoverySource(ctx, clients)
	must(t, err)
	if snapshotReview.Current.ManifestSHA256 != n.release.ManifestSHA256 || snapshotReview.Current.PackSHA256 != record.Releases[0].RuntimeSHA256 {
		t.Fatal("reviewed initial runtime recovery changed finite Current identity")
	}
	_, err = setup.ApplyRuntimeRecoverySource(ctx, clients, snapshotReview.Digest, true)
	must(t, err)
	placeRuntime(n)
	// A long-lived runtime must recheck the fixed role/origin on each use,
	// including when Current itself and the converter bytes remain unchanged.
	bound, err := product.Open(selectedBinding)
	must(t, err)
	converter := bound.DoclingRuntime()
	if availability := converter.Status(ctx); availability.State != "ready" {
		t.Fatalf("valid inert current runtime unavailable: %+v", availability)
	}
	roleSignature := filepath.Join(managed, "trust", installer.ExperimentalSeedAdmissionName+".sig")
	beforeSignature := read(t, roleSignature)
	must(t, os.WriteFile(roleSignature, []byte("changed fixed role signature"), 0600))
	invalidRoleAvailability := converter.Status(ctx)
	must(t, os.WriteFile(roleSignature, beforeSignature, 0600))
	if invalidRoleAvailability.State != "unavailable" {
		t.Fatalf("existing runtime ignored changed installed role: %+v", invalidRoleAvailability)
	}
	if restoredAvailability := converter.Status(ctx); restoredAvailability.State != "ready" || restoredAvailability.PackSHA256 != record.Releases[0].RuntimeSHA256 {
		t.Fatalf("restored exact role did not preserve current runtime readback: %+v", restoredAvailability)
	}
	// Explicit fixed public-document action, independent of runtime readiness.
	// Preparation is not conversion or native/client qualification.
	documentArgs := []string{"installed-pilot-prepare-document", "--product-root", managed, "--user-home", user, "--workspace", ws, "--carrier-id", clients.Request.CarrierID, "--installer-version", record.InstallerVersion, "--experimental-seed-sha256", role.Identity().AdmissionSHA256, "--experimental-installed-root", managed}
	documentPath := filepath.Join(ws, pilotdocument.Name)
	if _, err := os.Lstat(documentPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read-only preparation/check paths created the public workspace document")
	}
	if output, err := exec.CommandContext(ctx, installed.CLIPath, append(append([]string{}, documentArgs...), "--check-only")...).CombinedOutput(); err == nil {
		t.Fatalf("read-only flag triggered document preparation: %s", output)
	}
	if _, err := os.Lstat(documentPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("read-only refusal created public document")
	}
	for _, expectedState := range []string{"fixture_prepared", "fixture_already_present"} {
		output, err := exec.CommandContext(ctx, installed.CLIPath, documentArgs...).CombinedOutput()
		if err != nil {
			t.Fatalf("actual fixed document action: %s %v", output, err)
		}
		var prepared struct {
			Schema                    int `json:"schema_version"`
			State, Source, Conversion string
			SHA                       string `json:"source_sha256"`
			Native                    string `json:"native_qualification"`
		}
		must(t, json.Unmarshal(output, &prepared))
		if prepared.Schema != 1 || prepared.State != expectedState || prepared.Source != pilotdocument.Name || prepared.SHA != pilotdocument.SHA256 || prepared.Conversion != "not_tested" || prepared.Native != "not_tested" {
			t.Fatalf("public fixture action was confused with conversion/native qualification: %s", output)
		}
	}
	canonicalDocument, err := pilotdocument.Bytes()
	must(t, err)
	if !bytes.Equal(read(t, documentPath), canonicalDocument) {
		t.Fatal("actual helper did not prepare canonical fixed public document")
	}
	selection := sum(mustJSON(t, []string{initialClients.Binding.UserHome, initialClients.Binding.Workspace, initialClients.Binding.Home, initialClients.Binding.WorkspaceID}))
	if _, err := installer.PrepareStableNativeHook(ctx, clients, "PreToolUse", selection); err == nil || !strings.Contains(err.Error(), "genuinely published full native carrier required") {
		t.Fatalf("passive-only carrier became native authority: %v", err)
	}
	nativeArgs := []string{"stable-native-hook", "--event", "PreToolUse", "--binding-sha256", selection, "--product-root", managed, "--user-home", user, "--workspace", ws, "--carrier-id", clients.Request.CarrierID}
	nativeCLI := exec.CommandContext(ctx, seed.Self, nativeArgs...)
	var nativeStdout, nativeStderr bytes.Buffer
	nativeCLI.Stdout, nativeCLI.Stderr = &nativeStdout, &nativeStderr
	nativeErr := nativeCLI.Run()
	var nativeExit *exec.ExitError
	if !errors.As(nativeErr, &nativeExit) || nativeExit.ExitCode() != product.HookExitCode("PreToolUse") || nativeStdout.Len() != 0 || strings.TrimSpace(nativeStderr.String()) != "maestro: hook unavailable; inspect admitted state before retrying" {
		t.Fatalf("native CLI refusal lost event-aware masking or bypassed native record requirement: stdout=%s stderr=%s err=%v", nativeStdout.String(), nativeStderr.String(), nativeErr)
	}
	// Bypass the wrapper and call the actual signed/current server command.
	// A correctly installed signed source and matching release pins still grant
	// no native admission to this explicitly passive-only fixture.
	serverNativeArgs := installer.ExperimentalChildArgs([]string{"installed-native-hook", "--event", "PreToolUse", "--binding-sha256", selection, "--carrier-id", clients.Request.CarrierID, "--product-root", managed, "--user-home", user, "--workspace", ws, "--installed-manifest", currentN.Binding.ManifestSHA256, "--installed-client-inputs", currentN.Binding.ClientInputsSHA256}, role, managed)
	for _, change := range []string{"passive", "owner", "workspace", "carrier"} {
		t.Run("installed_native_child_refusal_"+change, func(t *testing.T) {
			args := append([]string(nil), serverNativeArgs...)
			for i := 1; i+1 < len(args); i += 2 {
				if change == "owner" && args[i] == "--user-home" || change == "workspace" && args[i] == "--workspace" {
					args[i+1] = filepath.Join(root, "foreign")
				}
				if change == "carrier" && args[i] == "--carrier-id" {
					args[i+1] = strings.Repeat("f", 32)
				}
			}
			child := exec.CommandContext(ctx, installed.CLIPath, args...)
			child.Stdin = bytes.NewReader(mustJSON(t, map[string]any{"session_id": "synthetic-denied-native", "prompt_id": "synthetic-turn", "cwd": ws, "hook_event_name": "PreToolUse", "tool_name": "Bash", "tool_use_id": "synthetic-probe", "tool_input": map[string]any{"command": "printf synthetic"}, "prompt": "PRIVATE_PAYLOAD_MUST_NOT_BE_LOGGED"}))
			var output, diagnostic bytes.Buffer
			child.Stdout, child.Stderr = &output, &diagnostic
			err := child.Run()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || exit.ExitCode() != product.HookExitCode("PreToolUse") || output.Len() != 0 || strings.TrimSpace(diagnostic.String()) != "maestro: hook unavailable; inspect admitted state before retrying" {
				t.Fatalf("actual installed native child bypassed admission or leaked diagnostics: exit=%v stdout=%s stderr=%s", err, output.String(), diagnostic.String())
			}
		})
	}
	for _, change := range []string{"absent_carrier", "changed_workspace", "expired_role", "modified_packet"} {
		t.Run("native_refusal_"+change, func(t *testing.T) {
			altered := clients
			switch change {
			case "absent_carrier":
				altered.Request.CarrierID = strings.Repeat("f", 32)
			case "changed_workspace":
				altered.Request.Workspace = filepath.Join(root, "foreign-workspace")
			case "expired_role":
				admissionNow = now.Add(8 * 24 * time.Hour)
				defer func() { admissionNow = now }()
			case "modified_packet":
				file := filepath.Join(initialClients.Materialization.CodeRoot, "plugins", "maestro-product", "README-PACKET.md")
				before := read(t, file)
				must(t, os.WriteFile(file, append(append([]byte{}, before...), []byte("changed")...), 0644))
				defer func() { must(t, os.WriteFile(file, before, 0644)) }()
			}
			if _, err := installer.PrepareStableNativeHook(ctx, altered, "PreToolUse", selection); err == nil {
				t.Fatal("invalid native route was admitted")
			}
		})
	}
	publish(2, next)
	checked, err = check()
	must(t, err)
	pending, err := updateservice.StagePending(o.DataRoot, checked.Current, checked, time.Now())
	must(t, err)
	lifecycleRequest, err := json.Marshal(map[string]any{"managed_root": managed, "user_home": user})
	must(t, err)
	lifecycleCLI := func(command string, confirm bool) ([]byte, error) {
		args := []string{command, "--plan-id", pending.Plan.ID}
		if confirm {
			args = append(args, "--confirm")
		}
		cmd := exec.CommandContext(ctx, seed.Self, args...)
		cmd.Stdin = bytes.NewReader(lifecycleRequest)
		return cmd.CombinedOutput()
	}
	if output, err := lifecycleCLI("update-review", false); err != nil {
		t.Fatalf("actual pre-commit update review: %s %v", output, err)
	}
	updated, err := installer.UpdateSeeded(ctx, installer.SeededUpdateOptions{SeededOperationOptions: seed, PlanID: pending.Plan.ID})
	must(t, err)
	if updated.Release != next.release.Manifest.Release || updated.Source.Sequence != 2 {
		t.Fatal("finite N+1 update did not retain source floor")
	}
	if _, err := updateservice.LoadPending(o.DataRoot); err != nil {
		t.Fatalf("committed crash-window fixture lost retained pending evidence: %v", err)
	}
	if output, err := lifecycleCLI("update-review", false); err == nil || !bytes.Contains(output, []byte("standard update must move to a newer release")) {
		t.Fatalf("committed recovery was incorrectly treated as a new update: %s %v", output, err)
	}
	admissionNow = now.Add(8 * 24 * time.Hour)
	_, expiredApplied, expiredErr := installer.ReconcileSeeded(ctx, installer.SeededUpdateOptions{SeededOperationOptions: seed, PlanID: pending.Plan.ID})
	admissionNow = now
	if expiredErr == nil || expiredApplied {
		t.Fatal("expired opaque EISA admission reached committed recovery effects")
	}
	expiredState, err := installtx.ReadSourceState(o.DataRoot, managed)
	must(t, err)
	if installtx.StateDigest(expiredState) != installtx.StateDigest(updated) {
		t.Fatal("expired committed recovery changed existing N1/source custody")
	}
	if output, err := lifecycleCLI("reconcile", true); err != nil {
		t.Fatalf("actual same-plan reconciliation after N1 committed and pending retained: %s %v", output, err)
	} else {
		var recovered struct {
			Release string `json:"release"`
			Applied bool   `json:"applied"`
		}
		must(t, json.Unmarshal(output, &recovered))
		if !recovered.Applied || recovered.Release != next.release.Manifest.Release {
			t.Fatalf("committed recovery did not acknowledge existing N1: %s", output)
		}
	}
	recoveredState, err := installtx.ReadSourceState(o.DataRoot, managed)
	must(t, err)
	if recoveredState.Source.Sequence != 2 || recoveredState.Binding.ManifestSHA256 != next.release.ManifestSHA256 || !bytes.Equal(read(t, brainFile), []byte("existing synthetic brain: retain these exact bytes")) {
		t.Fatal("same-plan recovery changed protected source floor/current release/brain")
	}
	assertContext()
	placeRuntime(next)
	if _, err := installer.PrepareStableNativeHook(ctx, clients, "PreToolUse", selection); err == nil {
		t.Fatal("stale pre-update client became native authority")
	}
	updatedDesktop := assertDesktop()
	if updatedDesktop.Materialization.DesktopSHA256 != initialDesktop.Materialization.DesktopSHA256 || updatedDesktop.Materialization.DesktopPath == initialDesktop.Materialization.DesktopPath || updatedDesktop.Materialization.DesktopMethods.Version != next.release.Manifest.Release {
		t.Fatal("Desktop update lost stable connector reuse or live current methods")
	}
	updatedClients := materialize()
	if updatedClients.Materialization.DesktopSHA256 != initialClients.Materialization.DesktopSHA256 || updatedClients.Materialization.BootstrapServerSHA256 != initialClients.Materialization.BootstrapServerSHA256 || updatedClients.Materialization.ServerSHA256 == initialClients.Materialization.ServerSHA256 {
		t.Fatal("compatible retained MCPB did not keep inert provenance separate from current server")
	}
	checker := exec.CommandContext(ctx, installed.CLIPath, "installed-pilot-check", "--product-root", managed, "--user-home", user, "--workspace", ws, "--carrier-id", clients.Request.CarrierID, "--installer-version", record.InstallerVersion, "--experimental-seed-sha256", role.Identity().AdmissionSHA256, "--experimental-installed-root", managed, "--check-only")
	checkOutput, err := checker.CombinedOutput()
	if err != nil {
		t.Fatalf("real installed checker rejected compatible retained MCPB: %s: %v", checkOutput, err)
	}
	var report struct {
		Evidence           string                                    `json:"evidence_class"`
		Checks             []struct{ Name, Status, Evidence string } `json:"checks"`
		DesktopExtension   string                                    `json:"desktop_extension"`
		DesktopPrompt      string                                    `json:"desktop_prompt"`
		DesktopReadyPrompt string                                    `json:"desktop_ready_prompt"`
		Conversion         struct {
			State    string
			Manifest string `json:"expected_manifest_sha256"`
			Pack     string `json:"expected_pack_sha256"`
		} `json:"conversion"`
	}
	must(t, json.Unmarshal(checkOutput, &report))
	if report.Evidence != "engineering_only" {
		t.Fatalf("checker lost explicit engineering class: %s", checkOutput)
	}
	if report.DesktopExtension != "maestro-connector-"+clients.Request.CarrierID || !strings.Contains(report.DesktopPrompt, report.DesktopExtension) || !strings.Contains(report.DesktopReadyPrompt, report.DesktopExtension) {
		t.Fatal("actual retained-archive checker selected a stale/foreign Desktop connector")
	}
	if report.Conversion.State != "ready_for_participant_test" || report.Conversion.Manifest != next.release.ManifestSHA256 || report.Conversion.Pack != record.Releases[1].RuntimeSHA256 {
		t.Fatalf("actual checker lost Current-bound conversion expectation: %+v", report.Conversion)
	}
	desktopChecked, runtimePending := false, false
	for _, check := range report.Checks {
		desktopChecked = desktopChecked || check.Name == "desktop_archive" && check.Status == "passed"
		runtimePending = runtimePending || check.Name == "document_runtime" && check.Status == "passed"
	}
	if !desktopChecked || !runtimePending {
		t.Fatalf("checker conflated retained Desktop contract or inert runtime fixture with real qualification: %s", checkOutput)
	}
	rollback, err := installer.ReviewRollbackSeeded(ctx, seed)
	must(t, err)
	restored, err := installer.RollbackSeeded(ctx, installer.SeededRollbackOptions{SeededOperationOptions: seed, ReviewDigest: rollback.Digest})
	must(t, err)
	if restored.Release != n.release.Manifest.Release || restored.Source.Sequence != 2 {
		t.Fatal("rollback changed the protected source floor")
	}
	assertContext()
	placeRuntime(n)
	restoredClients := materialize()
	priorRecord := read(t, filepath.Join(seed.DataRoot, "clients", clients.Request.CarrierID+".json"))
	callbacks := 0
	observed, registrationErr := installer.RegisterCodeSeeded(ctx, clients, installer.ClientRecordDigest(restoredClients), func(r installer.ClientRecord) (installer.CodeObservation, error) {
		callbacks++ // an acknowledged synthetic callback; no native Code command.
		admissionNow = now.Add(8 * 24 * time.Hour)
		observation := r.Code
		observation.State, observation.EvidenceClass = "registered", "engineering_process_witness"
		return observation, nil
	})
	if callbacks != 1 || registrationErr == nil || !strings.Contains(registrationErr.Error(), "registration callback completed") || observed.Code.State != "registered" || !bytes.Equal(priorRecord, read(t, filepath.Join(seed.DataRoot, "clients", clients.Request.CarrierID+".json"))) {
		t.Fatalf("expiry after acknowledged synthetic callback lost actual effect or changed prior record: callbacks=%d observation=%+v err=%v", callbacks, observed.Code, registrationErr)
	}
	admissionNow = now
	if !bytes.Equal(read(t, brainFile), []byte("existing synthetic brain: retain these exact bytes")) {
		t.Fatal("source lifecycle altered existing brain")
	}

	// Authentic but outside the two-release role: deny before payload copying,
	// even though the same ephemeral issuer legitimately signed this metadata.
	g := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: next.release.Manifest.Issuer, Source: tuple, Sequence: 3, PublishedAt: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(6 * 24 * time.Hour).Format(time.RFC3339), Release: "0.1.53", ManifestSHA256: strings.Repeat("a", 64), ClientInputsSHA256: strings.Repeat("b", 64), Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}
	outsideBody, err := releasecontract.CanonicalJSON(g)
	must(t, err)
	outsideSig := ed25519.Sign(private, outsideBody)
	verifiedGeneration, err := releaseverify.VerifyGeneration(outsideBody, outsideSig, registry)
	must(t, err)
	generationDir := filepath.Join(synced, "generations", g.GenerationDirectory(verifiedGeneration.SHA256))
	must(t, os.MkdirAll(generationDir, 0700))
	for path, data := range map[string][]byte{filepath.Join(synced, releaseprovider.CurrentGenerationName): outsideBody, filepath.Join(synced, releaseprovider.CurrentGenerationSignatureName): outsideSig, filepath.Join(generationDir, releaseprovider.GenerationName): outsideBody, filepath.Join(generationDir, releaseprovider.GenerationSignatureName): outsideSig} {
		must(t, os.WriteFile(path, data, 0600))
	}
	before, err := installtx.ReadSourceState(o.DataRoot, managed)
	must(t, err)
	if _, err = installer.ReviewSourceSeeded(ctx, sourceOptions); err == nil || !strings.Contains(err.Error(), "outside the two experimental bindings") {
		t.Fatalf("outside-role generation was not explicitly refused: %v", err)
	}
	after, err := installtx.ReadSourceState(o.DataRoot, managed)
	must(t, err)
	if installtx.StateDigest(before) != installtx.StateDigest(after) {
		t.Fatal("outside-role refusal changed source custody")
	}

	// Ordinary production entry stays independent: it cannot consume the
	// engineering status/flags or this unsigned admitted installation.
	productionRoot := filepath.Join(root, "ordinary-production")
	must(t, os.MkdirAll(filepath.Join(productionRoot, "trust"), 0700))
	must(t, os.WriteFile(filepath.Join(productionRoot, "trust", "release-authority-registry.json"), registryBody, 0600))
	production := filepath.Join(productionRoot, filepath.Base(n.factory.Installer))
	ldflags := "-X main.Version=" + record.InstallerVersion + " -X main.AuthorityRegistrySHA256=" + record.RegistrySHA256
	buildProduction := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-buildvcs=false", "-ldflags", ldflags, "-o", production, "./cmd/maestro-install")
	buildProduction.Dir = filepath.Join(source, "adapters", "mcp")
	if output, e := buildProduction.CombinedOutput(); e != nil {
		t.Fatalf("build ordinary entry: %s: %v", output, e)
	}
	status, err := exec.CommandContext(ctx, production, "seed-status").CombinedOutput()
	must(t, err)
	if bytes.Contains(status, []byte("engineering")) || bytes.Contains(status, []byte(`"schema_version":3`)) {
		t.Fatalf("ordinary entry exported engineering status: %s", status)
	}
	request, _ := json.Marshal(map[string]any{"managed_root": managed, "user_home": user, "source_root": synced, "source": tuple})
	command := exec.CommandContext(ctx, production, "source-review")
	command.Stdin = bytes.NewReader(request)
	if output, e := command.CombinedOutput(); e == nil {
		t.Fatalf("ordinary entry consumed engineering custody: %s", output)
	}
	request, _ = json.Marshal(map[string]any{"release_directory": n.dir, "managed_root": filepath.Join(root, "unsigned-production-destination"), "user_home": user, "workspace": ws, "code_packet": filepath.Join(root, "production-code"), "desktop_packet": filepath.Join(root, "production.mcpb")})
	command = exec.CommandContext(ctx, production, "review")
	command.Stdin = bytes.NewReader(request)
	if output, e := command.CombinedOutput(); e == nil || !bytes.Contains(output, []byte("native bootstrapper trust check")) {
		t.Fatalf("ordinary unsigned binary did not retain native refusal: %s: %v", output, e)
	}
	if output, e := exec.CommandContext(ctx, production, nativeArgs...).CombinedOutput(); e == nil || !bytes.Contains(output, []byte("flag provided but not defined")) {
		t.Fatalf("ordinary installer accepted engineering native route flags: %s %v", output, e)
	}
	productionServer := filepath.Join(productionRoot, sourceTestExecutable("maestro-mcp"))
	buildOrdinaryServer := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-buildvcs=false", "-o", productionServer, "./cmd/maestro-mcp")
	buildOrdinaryServer.Dir = filepath.Join(source, "adapters", "mcp")
	if output, e := buildOrdinaryServer.CombinedOutput(); e != nil {
		t.Fatalf("build ordinary server: %s: %v", output, e)
	}
	if output, e := exec.CommandContext(ctx, productionServer, "serve", "--experimental-seed-sha256", role.Identity().AdmissionSHA256, "--experimental-installed-root", managed).CombinedOutput(); e == nil || !bytes.Contains(output, []byte("flag provided but not defined")) {
		t.Fatalf("ordinary server registered experimental flags: %s: %v", output, e)
	}
	// First-use choices remain ordinary explicit local review. This second
	// synthetic owner is genuinely fresh; no locator or populated brain is
	// copied. The valid inert converter exercises actual current-consumer
	// placement and readback, without executing Python or claiming conversion.
	freshUser, freshManaged := filepath.Join(root, "fresh-user"), filepath.Join(root, "fresh-managed")
	freshHome, freshWorkspace := filepath.Join(root, "Developer", "fresh-home"), filepath.Join(root, "fresh-workspace")
	freshPackets := filepath.Join(root, "fresh-private-packets")
	must(t, os.Mkdir(freshUser, 0700))
	must(t, os.Mkdir(freshPackets, 0700))
	t.Setenv("MAESTRO_HOME", freshHome)
	freshRequest := setup.Request{ReleaseDirectory: n.dir, ManagedRoot: freshManaged, UserHome: freshUser, Workspace: freshWorkspace, CodePacket: filepath.Join(freshPackets, "code"), DesktopPacket: filepath.Join(freshPackets, "desktop.mcpb")}
	freshConfig := setup.Config{ExperimentalSeed: role, Installer: n.factory.Installer, AuthorityRegistry: registryPath}
	freshReview, err := setup.ReviewInstallation(freshRequest, freshConfig)
	must(t, err)
	if freshReview.Home.Home != freshHome || freshReview.Home.Diagnostic != "fresh-unconfirmed" {
		t.Fatalf("fresh review did not display explicit proposed home: %+v", freshReview.Home)
	}
	for _, path := range []string{freshHome, freshWorkspace, freshManaged} {
		if _, err = os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("read-only fresh review created state at %s: %v", path, err)
		}
	}
	freshResult, freshErr := setup.Apply(ctx, freshRequest, freshConfig, freshReview.Digest, true)
	if freshErr != nil || freshResult.State != "ready" || freshResult.Content.PackSHA256 != record.Releases[0].RuntimeSHA256 {
		t.Fatalf("valid inert current-consumer setup did not complete: %+v: %v", freshResult, freshErr)
	}
	freshResolved, err := datahome.Resolve(datahome.Options{ProductRoot: freshManaged, UserHome: freshUser})
	must(t, err)
	if !freshResolved.Writable || freshResolved.Home != freshHome || freshResolved.Origin != "fresh-confirmed" {
		t.Fatalf("explicit fresh setup did not initialize the reviewed owner: %+v", freshResolved)
	}
	freshInspection, err := workspace.Inspect(freshWorkspace, filepath.Join(freshHome, "state"))
	must(t, err)
	if freshInspection.State != "ready" || freshResult.WorkspaceID != freshInspection.WorkspaceID {
		t.Fatalf("fresh setup changed the reviewed workspace: %+v: %v", freshInspection, freshErr)
	}
	t.Setenv("MAESTRO_HOME", brain)
	if !bytes.Equal(read(t, brainFile), []byte("existing synthetic brain: retain these exact bytes")) {
		t.Fatal("separate reviewed fresh setup touched original synthetic brain")
	}
	t.Log("engineering_only: real compiled tagged install/enroll/N→N1/rollback/stdio readback and distinct inert runtime placement; existing synthetic Developer brain retained; finite outside-role refused; real conversion and native Code/Desktop qualification remain unproven")
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	must(t, err)
	return body
}
