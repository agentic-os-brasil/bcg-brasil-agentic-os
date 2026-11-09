package setup_test

import (
	"bytes"
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
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This is the whole public product seam: genuine native seed composition,
// signed local source, canonical transaction and rebuilt SDK/stdio consumers.
// The native verifier is explicitly an engineering byte-bound composition;
// production-default CLI refusal remains a required independent negative.
func TestModernSyncedSourceLifecycle(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("actual admitted-home engineering runner required")
	}
	baseline := os.Getenv("MAESTRO_SYNCED_UPDATE_BASELINE")
	if baseline == "" {
		t.Skip("explicit immutable old-modern baseline required for the complete Task2 gate")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	root := t.TempDir()
	if retained := os.Getenv("MAESTRO_SYNCED_TEST_ROOT"); retained != "" {
		must(t, os.Mkdir(retained, 0700))
		root = retained
	}
	source, err := filepath.Abs("../../..")
	must(t, err)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	registryBody, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "engineering-fixture", "key_id": "ephemeral", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": "2020-01-01T00:00:00Z", "valid_until": "2100-01-01T00:00:00Z"}}})
	registryPath := filepath.Join(root, "authority.json")
	must(t, os.WriteFile(registryPath, registryBody, 0600))
	registry, err := releaseverify.LoadAuthorityRegistry(registryPath, time.Now)
	must(t, err)
	type builtRelease struct {
		factory   releasepack.ModernCandidate
		manifest  releasecontract.Manifest
		verified  releaseverify.VerifiedRelease
		directory string
	}
	build := func(version string) builtRelease {
		factory, e := releasepack.BuildModernCandidate(ctx, releasepack.ModernOptions{Root: source, Output: filepath.Join(root, "factory-"+version), Version: version, InstallerVersion: "0.3.7", Channel: "canary", AuthorityRegistry: registryPath, Target: releasepack.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}})
		must(t, e)
		directory := filepath.Join(root, "signed-"+version)
		manifest, e := releasepack.SignCandidate(releasepack.SignCandidateOptions{Candidate: factory.Directory, Output: directory, Registry: registryPath, Issuer: "engineering-fixture", KeyID: "ephemeral", PrivateKey: private})
		must(t, e)
		verified, e := releaseverify.VerifyDirectory(directory, registry)
		must(t, e)
		return builtRelease{factory, manifest, verified, directory}
	}
	n := build("0.1.42")
	next := build("0.1.43")
	recovery := build("0.1.45")
	oldHelper := os.Getenv("MAESTRO_SYNCED_OLD_HELPER")
	if oldHelper == "" {
		helperRoot := filepath.Join(root, "old-helper-build")
		must(t, os.Mkdir(helperRoot, 0700))
		module := strings.Replace(string(read(t, filepath.Join(baseline, "go.mod"))), "module github.com/agentic-os-brasil/bcg-brasil-agentic-os", "module github.com/agentic-os-brasil/bcg-brasil-agentic-os/oldpublicprobe", 1)
		module += "\nrequire github.com/agentic-os-brasil/bcg-brasil-agentic-os v0.0.0\nreplace github.com/agentic-os-brasil/bcg-brasil-agentic-os => " + fmt.Sprintf("%q", baseline) + "\n"
		must(t, os.WriteFile(filepath.Join(helperRoot, "go.mod"), []byte(module), 0600))
		must(t, os.WriteFile(filepath.Join(helperRoot, "go.sum"), read(t, filepath.Join(baseline, "go.sum")), 0600))
		must(t, os.WriteFile(filepath.Join(helperRoot, "main.go"), read(t, filepath.Join(source, "internal/installer/testdata/synced-old-public/main.go")), 0600))
		oldHelper = filepath.Join(helperRoot, "old-public-helper")
		command := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-buildvcs=false", "-o", oldHelper, ".")
		command.Dir = helperRoot
		if output, e := command.CombinedOutput(); e != nil {
			t.Fatalf("compile immutable old public owner: %s: %v", output, e)
		}
	}
	factoryInput := filepath.Join(root, "old-factory-input.json")
	factoryRequest, _ := json.Marshal(map[string]string{"Baseline": baseline, "Registry": registryPath, "Output": filepath.Join(root, "old-factory"), "Version": "0.1.44", "InstallerVersion": "0.3.7"})
	must(t, os.WriteFile(factoryInput, factoryRequest, 0600))
	oldOutput, err := exec.CommandContext(ctx, oldHelper, "factory", factoryInput).CombinedOutput()
	if err != nil {
		t.Fatalf("build unchanged Task1 server and seed: %s: %v", oldOutput, err)
	}
	var oldFactory releasepack.ModernCandidate
	must(t, json.Unmarshal(oldOutput, &oldFactory))
	oldSigned := filepath.Join(root, "old-signed")
	_, err = releasepack.SignCandidate(releasepack.SignCandidateOptions{Candidate: oldFactory.Directory, Output: oldSigned, Registry: registryPath, Issuer: "engineering-fixture", KeyID: "ephemeral", PrivateKey: private})
	must(t, err)
	spoof := filepath.Join(root, "old-server-claimed-capability")
	must(t, os.Mkdir(spoof, 0700))
	entries, err := os.ReadDir(oldFactory.Directory)
	must(t, err)
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			t.Fatal("old candidate contains unexpected directory")
		}
		info, e := entry.Info()
		must(t, e)
		must(t, os.WriteFile(filepath.Join(spoof, entry.Name()), read(t, filepath.Join(oldFactory.Directory, entry.Name())), info.Mode().Perm()))
	}
	spoofManifest := oldFactory.Manifest
	spoofManifest.Server.ExecutionContract = releasecontract.SyncedSourceContract
	spoofManifest.Delivery.ClientInputsSHA256, err = spoofManifest.ClientInputsDigest()
	must(t, err)
	spoofJSON, err := json.MarshalIndent(spoofManifest, "", "  ")
	must(t, err)
	must(t, os.WriteFile(filepath.Join(spoof, "release-manifest.json"), append(spoofJSON, '\n'), 0600))
	spoofSigned := filepath.Join(root, "old-server-claimed-capability-signed")
	_, err = releasepack.SignCandidate(releasepack.SignCandidateOptions{Candidate: spoof, Output: spoofSigned, Registry: registryPath, Issuer: "engineering-fixture", KeyID: "ephemeral", PrivateKey: private})
	must(t, err)
	spoofVerified, err := releaseverify.VerifyDirectory(spoofSigned, registry)
	must(t, err)
	negativeUser := filepath.Join(root, "old-server-negative-owner")
	must(t, os.Mkdir(negativeUser, 0700))
	_, err = installtx.Prepare(spoofVerified, installtx.PrepareOptions{UserHome: negativeUser, Transition: "install", ManagedRoot: filepath.Join(root, "old-server-negative-managed"), DataRoot: filepath.Join(negativeUser, ".maestro-install"), TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH})
	if err == nil || !strings.Contains(err.Error(), "actual server source-aware identity") {
		t.Fatalf("actual Task1 server gained capability from signed declaration: %v", err)
	}
	if n.manifest.Server.ExecutionContract != "maestro-synced-source-v1" || next.manifest.Server.ExecutionContract != n.manifest.Server.ExecutionContract {
		t.Fatal("factory did not bind real source execution capability")
	}
	seedSHA := sum(read(t, n.factory.Installer))
	if sum(read(t, next.factory.Installer)) != seedSHA {
		t.Fatal("independently versioned installer changed between release builds")
	}
	config := setup.Config{Installer: n.factory.Installer, AuthorityRegistry: n.factory.AuthorityRegistry, LookupEnv: func(string) (string, bool) { return "", false }}
	config.VerifyNative = func(_ context.Context, path string) error {
		body, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if sum(body) != seedSHA {
			return errors.New("engineering seed identity changed")
		}
		return nil
	}
	config.Run = func(ctx context.Context, self string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "install" {
			flags := flag.NewFlagSet("controlled-engineering-seed", flag.ContinueOnError)
			body := flags.String("confirmed-plan", "", "")
			digest := flags.String("review-digest", "", "")
			confirm := flags.Bool("confirm", false, "")
			if e := flags.Parse(args[1:]); e != nil {
				return nil, e
			}
			var plan installer.Plan
			if e := releasecontract.DecodeStrictJSON([]byte(*body), &plan); e != nil {
				return nil, e
			}
			state, e := installer.ActivateSeeded(ctx, installer.SeededActivationOptions{Self: self, Version: "0.3.7", AuthorityRegistrySHA256: sum(registryBody), Plan: plan, ReviewDigest: *digest, Confirmed: *confirm, VerifyNative: config.VerifyNative})
			if e != nil {
				return nil, e
			}
			return json.Marshal(map[string]string{"state": "activated", "profile": state.Profile})
		}
		return exec.CommandContext(ctx, self, args...).CombinedOutput()
	}
	user := filepath.Join(root, "owner")
	ws := filepath.Join(root, "workspace")
	must(t, os.Mkdir(user, 0700))
	must(t, os.Mkdir(ws, 0700))
	must(t, os.WriteFile(filepath.Join(ws, "owner.txt"), []byte("literal owner bytes retained across signed update and rollback"), 0600))
	r := setup.Request{ReleaseDirectory: n.directory, ManagedRoot: filepath.Join(root, "managed"), UserHome: user, Workspace: ws, CodePacket: filepath.Join(root, "code-n"), DesktopPacket: filepath.Join(root, "desktop-n.mcpb")}
	review, err := setup.ReviewInstallation(r, config)
	must(t, err)
	installed, err := setup.Apply(ctx, r, config, review.Digest, true)
	must(t, err)
	dataRoot := filepath.Join(user, ".maestro-install")
	seed := installer.SeededOperationOptions{Self: filepath.Join(r.ManagedRoot, "maestro-install"), Version: "0.3.7", AuthorityRegistrySHA256: sum(registryBody), UserHome: user, ManagedRoot: r.ManagedRoot, DataRoot: dataRoot, VerifyNative: config.VerifyNative}
	current, err := installtx.ReadStateForManagedRoot(dataRoot, r.ManagedRoot)
	must(t, err)
	if current.SchemaVersion != 4 || current.Source != nil || current.Binding.ManifestSHA256 != n.verified.ManifestSHA256 {
		t.Fatal("first install did not record exact signed source-aware release")
	}
	if _, err := installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: seed, ExpectedBinding: current.Binding, Workspace: ws}); err == nil {
		t.Fatal("unenrolled source granted qualified launch")
	}
	// The actual production binary must retain native refusal, independently
	// of the controlled in-process native verifier used for positive admission.
	production := exec.CommandContext(ctx, seed.Self, "source-review")
	requestBody, _ := json.Marshal(map[string]any{"managed_root": r.ManagedRoot, "user_home": user, "source_root": filepath.Join(root, "synced"), "source": map[string]string{"source_id": "team-pilot", "channel": "canary", "os": runtime.GOOS, "arch": runtime.GOARCH}})
	production.Stdin = bytes.NewReader(requestBody)
	if output, e := production.CombinedOutput(); e == nil {
		t.Fatalf("unsigned production source CLI accepted fixture: %s", output)
	}
	syncRoot := filepath.Join(root, "synced")
	now := time.Now().UTC().Truncate(time.Second)
	tuple := releasecontract.SourceTuple{SourceID: "team-pilot", Channel: "canary", OS: runtime.GOOS, Arch: runtime.GOARCH}
	publish := func(sequence uint64, release builtRelease, recalls []releasecontract.ReleaseRecall) releaseverify.VerifiedGeneration {
		g := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: release.manifest.Issuer, Source: tuple, Sequence: sequence, PublishedAt: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(7 * 24 * time.Hour).Format(time.RFC3339), Release: release.manifest.Release, ManifestSHA256: release.verified.ManifestSHA256, ClientInputsSHA256: release.manifest.Delivery.ClientInputsSHA256, Recalls: recalls, MinimumReaderContract: releasecontract.SyncedSourceContract}
		v, e := releasepack.PublishGeneration(releasepack.PublishGenerationOptions{Root: syncRoot, ReleaseDirectory: release.directory, Registry: registryPath, Generation: g, PrivateKey: private})
		must(t, e)
		return v
	}
	sourceOptions := installer.SeededSourceOptions{SeededOperationOptions: seed, SourceRoot: syncRoot, ExpectedSource: tuple}
	check := func() (updateservice.CheckResult, error) {
		review, e := installer.ReviewSourceSeeded(ctx, sourceOptions)
		must(t, e)
		sourceOptions.ReviewDigest = review.Digest
		return installer.CheckSourceSeeded(ctx, sourceOptions)
	}
	publish(1, n, []releasecontract.ReleaseRecall{})
	checked, err := check()
	if !errors.Is(err, updateservice.ErrCurrent) {
		t.Fatalf("current source enrollment: %v", err)
	}
	current = checked.Current
	// Independent seed-based observation remains usable while the MCP cannot
	// launch. This proves metadata recovery without inventing healthy payloads.
	must(t, os.Rename(installed.Installation.CLIPath, installed.Installation.CLIPath+".held"))
	missingCheck, missingErr := check()
	if !errors.Is(missingErr, updateservice.ErrCurrent) || missingCheck.Current.Source == nil {
		t.Fatalf("independent source observation depends on missing MCP executable: %v", missingErr)
	}
	if _, e := installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: seed, ExpectedBinding: current.Binding, Workspace: ws}); e == nil {
		t.Fatal("missing MCP executable became launch eligible")
	}
	must(t, os.Rename(installed.Installation.CLIPath+".held", installed.Installation.CLIPath))
	current = missingCheck.Current
	oldProofInput := filepath.Join(root, "old-public-input.json")
	oldProofBody, _ := json.Marshal(map[string]string{"UserHome": user, "ManagedRoot": r.ManagedRoot, "DataRoot": dataRoot, "ReleaseDir": oldSigned, "Installer": oldFactory.Installer, "Registry": registryPath, "Baseline": baseline, "Output": filepath.Join(root, "old-public-proof.json")})
	must(t, os.WriteFile(oldProofInput, oldProofBody, 0600))
	if output, e := exec.CommandContext(ctx, oldHelper, oldProofInput).CombinedOutput(); e != nil {
		t.Fatalf("unchanged old public owner failed refusal proof: %s: %v", output, e)
	}
	qualified, err := installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: seed, ExpectedBinding: current.Binding, Workspace: ws})
	must(t, err)
	if qualified.Executable != installed.Installation.CLIPath || qualified.State.Binding.ManifestSHA256 != n.verified.ManifestSHA256 {
		t.Fatal("launcher selected a different executable")
	}
	liveN := connectQualifiedSource(t, ctx, seed, current.Binding, ws)
	defer liveN.Close()
	created := call(t, ctx, liveN, "maestro_apply", map[string]any{"operation": "execution.create", "objective": "signed source continuation", "next_step": "preserve this literal step", "artifacts": []string{"result.md"}}, false)
	itemID := created["item_id"].(string)
	ownerBefore := snapshot(t, filepath.Join(user, ".maestro"))
	workspaceBefore := snapshot(t, ws)
	publish(2, n, []releasecontract.ReleaseRecall{})
	checked, err = check()
	if !errors.Is(err, updateservice.ErrCurrent) {
		t.Fatal(err)
	}
	if checked.Current.Binding.ManifestSHA256 != current.Binding.ManifestSHA256 || checked.Current.Binding.ClientInputsSHA256 != current.Binding.ClientInputsSHA256 {
		t.Fatal("metadata refresh changed active release identity")
	}
	call(t, ctx, liveN, "maestro_context", map[string]any{}, false)
	clockState := read(t, filepath.Join(dataRoot, "config/install-state.json"))
	clockOptions := updateservice.SyncedOptions{UserHome: user, ManagedRoot: r.ManagedRoot, DataRoot: dataRoot, SourceRoot: syncRoot, ExpectedSource: tuple, Registry: registry, Clock: func() time.Time { return checked.Current.Source.ObservedAt.Add(-time.Second) }}
	clockReview, e := updateservice.ReviewSynced(clockOptions)
	must(t, e)
	if _, e := updateservice.CheckSynced(clockOptions, clockReview.Digest); e == nil || !bytes.Equal(clockState, read(t, filepath.Join(dataRoot, "config/install-state.json"))) {
		t.Fatal("backwards trusted observation changed source custody")
	}
	// The real running MCP and SDK trusted form exercise Service + Worker;
	// neither a direct CheckSynced unit nor a saved receipt substitutes for it.
	publish(3, next, []releasecontract.ReleaseRecall{})
	forms := 0
	driftFirstForm := true
	client := mcp.NewClient(&mcp.Implementation{Name: "signed-update-attended-proof", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		forms++
		if !strings.Contains(req.Params.Message, "source_custody") || !strings.Contains(req.Params.Message, "team-pilot") || !strings.Contains(req.Params.Message, "0.1.43") {
			return nil, errors.New("trusted form omits exact signed source/effects")
		}
		if driftFirstForm {
			driftFirstForm = false
			_, e := check()
			if e != nil {
				return nil, e
			}
		}
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	}})
	cmd := exec.CommandContext(ctx, qualified.Executable, qualified.Args...)
	attended, err := client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, &mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
	must(t, err)
	beforeDecline := snapshot(t, filepath.Join(user, ".maestro"))
	call(t, ctx, attended, "maestro_apply", map[string]any{"operation": "maintenance.qualify", "job_id": "update-check", "expected_revision": 0, "source_root": syncRoot}, true)
	call(t, ctx, attended, "maestro_apply", map[string]any{"operation": "maintenance.qualify", "job_id": "update-check", "expected_revision": 0}, true)
	if !reflect.DeepEqual(beforeDecline, snapshot(t, filepath.Join(user, ".maestro"))) {
		t.Fatal("stale trusted update form granted owner/job effects")
	}
	qualifiedJob := call(t, ctx, attended, "maestro_apply", map[string]any{"operation": "maintenance.qualify", "job_id": "update-check", "expected_revision": 0}, false)
	if forms != 2 || qualifiedJob["qualification_state"] != "qualified" {
		t.Fatalf("real attended check lacks qualification: forms=%d value=%v", forms, qualifiedJob)
	}
	diagnosticStore := maintenance.DiagnosticStore{Root: filepath.Join(user, ".maestro", "state", "maintenance")}
	beforeReplay, _, e := diagnosticStore.Load(installed.WorkspaceID, "update-check", "")
	must(t, e)
	qualificationID := qualifiedJob["qualification"].(map[string]any)["id"]
	replayedCheck := call(t, ctx, attended, "maestro_apply", map[string]any{"operation": "maintenance.qualify", "job_id": "update-check", "expected_revision": qualifiedJob["revision"]}, false)
	afterReplay, _, e := diagnosticStore.Load(installed.WorkspaceID, "update-check", "")
	must(t, e)
	replayProof := map[string]any{"previous_qualification": qualificationID, "repeated_qualification": replayedCheck["qualification"].(map[string]any)["id"], "previous_diagnosis_source": beforeReplay.SourceSHA256, "repeated_diagnosis_source": afterReplay.SourceSHA256, "result": replayedCheck}
	replayProofBody, _ := json.MarshalIndent(replayProof, "", "  ")
	must(t, os.WriteFile(filepath.Join(root, "unchanged-candidate-replay-proof.json"), append(replayProofBody, '\n'), 0600))
	if replayedCheck["qualification"].(map[string]any)["id"] != qualificationID || beforeReplay.SourceSHA256 != afterReplay.SourceSHA256 || replayedCheck["reason"] != "reviewed_no_change" {
		t.Fatalf("unchanged candidate created timestamp-only qualification: prior=%v repeated=%v prior_diagnosis=%s repeated_diagnosis=%s", qualificationID, replayedCheck["qualification"].(map[string]any)["id"], beforeReplay.SourceSHA256, afterReplay.SourceSHA256)
	}
	qualifiedJob = replayedCheck
	call(t, ctx, attended, "maestro_apply", map[string]any{"operation": "maintenance.enroll", "job_id": "update-check", "expected_revision": qualifiedJob["revision"], "timezone": "UTC"}, true)
	report := call(t, ctx, attended, "maestro_query", map[string]any{"operation": "maintenance.report", "job_id": "update-check"}, false)
	if report["update"].(map[string]any)["outcome"] != "candidate_staged_activation_unconfirmed" {
		t.Fatal(report)
	}
	if report["finding"] != "historical_update_check" || report["update"].(map[string]any)["plan_id"] != beforeReplay.Update.PlanID || report["current_pending"].(map[string]any)["state"] != "verified_unconfirmed" || report["current_pending"].(map[string]any)["plan_id"] == beforeReplay.Update.PlanID || report["current_pending"].(map[string]any)["manifest_sha256"] != next.verified.ManifestSHA256 {
		t.Fatal("unchanged replay relabelled historical diagnostic or omitted verified current plan")
	}
	modelReport, _ := json.Marshal(report)
	if bytes.Contains(modelReport, []byte(user)) || bytes.Contains(modelReport, []byte(syncRoot)) {
		t.Fatal("ordinary pending readback exposed private local paths")
	}
	must(t, attended.Close())
	ownerBefore = snapshot(t, filepath.Join(user, ".maestro"))
	pending, err := updateservice.LoadPending(dataRoot)
	must(t, err)
	if pending.SchemaVersion != 2 || pending.Plan.SchemaVersion != 3 || pending.Plan.ToRelease != "0.1.43" {
		t.Fatal("modern pending variant missing")
	}
	prepared, e := installtx.ReadPlan(pending.ActivationPlanPath)
	must(t, e)
	preparedBody := read(t, prepared.StagedCLI)
	preparedInfo, e := os.Stat(prepared.StagedCLI)
	must(t, e)
	must(t, os.Chmod(prepared.StagedCLI, 0700))
	must(t, os.WriteFile(prepared.StagedCLI, []byte("changed prepared executable after exact review"), 0700))
	if _, _, e := updateservice.ConfirmPending(dataRoot, r.ManagedRoot, pending.Plan.ID, registry); e == nil {
		t.Fatal("changed prepared bytes reused confirmation")
	}
	must(t, os.WriteFile(prepared.StagedCLI, preparedBody, 0700))
	must(t, os.Chmod(prepared.StagedCLI, preparedInfo.Mode().Perm()))
	driftedCheck, e := check()
	must(t, e)
	if _, _, e := updateservice.ConfirmPending(dataRoot, r.ManagedRoot, pending.Plan.ID, registry); e == nil {
		t.Fatal("changed source custody reused pending confirmation")
	}
	pending, e = updateservice.StagePending(dataRoot, driftedCheck.Current, driftedCheck, time.Now())
	must(t, e)
	if _, err := installer.ReviewPendingSeeded(ctx, installer.SeededUpdateOptions{SeededOperationOptions: seed, PlanID: strings.Repeat("a", 32)}); err == nil {
		t.Fatal("arbitrary confirmation plan accepted")
	}
	current, err = installer.UpdateSeeded(ctx, installer.SeededUpdateOptions{SeededOperationOptions: seed, PlanID: pending.Plan.ID})
	must(t, err)
	if current.Release != "0.1.43" || current.Source.Sequence != 3 || current.Binding.ManifestSHA256 != next.verified.ManifestSHA256 || sum(read(t, installed.Installation.CLIPath)) != current.Server.SHA256 {
		t.Fatal("actual activated update identity differs")
	}
	call(t, ctx, liveN, "maestro_context", map[string]any{}, true)
	if _, err = installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: seed, ExpectedBinding: checked.Current.Binding, Workspace: ws}); err == nil {
		t.Fatal("cached N carrier acquired N+1 authority")
	}
	if !reflect.DeepEqual(ownerBefore, snapshot(t, filepath.Join(user, ".maestro"))) || !reflect.DeepEqual(workspaceBefore, snapshot(t, ws)) {
		t.Fatal("update changed owner/workspace bodies")
	}
	identity, err := releasecontract.ReadServerIdentity(installed.Installation.CLIPath)
	must(t, err)
	assertSourceClosure(t, source, identity, r.ManagedRoot, current.BundleVersion)
	newSession := connectQualifiedSource(t, ctx, seed, current.Binding, ws)
	nextState := call(t, ctx, newSession, "maestro_query", map[string]any{"operation": "execution.next", "item_id": itemID}, false)
	if nextState["next_step"] != "preserve this literal step" {
		t.Fatal("MCP update lost execution continuation")
	}
	must(t, newSession.Close())
	rollbackReview, err := installer.ReviewRollbackSeeded(ctx, seed)
	must(t, err)
	current, err = installer.RollbackSeeded(ctx, installer.SeededRollbackOptions{SeededOperationOptions: seed, ReviewDigest: rollbackReview.Digest})
	must(t, err)
	if current.Release != "0.1.42" || current.Source.Sequence != 3 || current.Binding.ManifestSHA256 != n.verified.ManifestSHA256 {
		t.Fatal("rollback reset source history or selected different release")
	}
	if !reflect.DeepEqual(ownerBefore, snapshot(t, filepath.Join(user, ".maestro"))) || !reflect.DeepEqual(workspaceBefore, snapshot(t, ws)) {
		t.Fatal("rollback changed owner/workspace bodies")
	}
	// A second genuine update crashes after payload/receipt and before state.
	// The private package hook is reachable only in the focused test binary.
	publish(4, next, []releasecontract.ReleaseRecall{})
	crashChecked, e := check()
	must(t, e)
	crashPending, e := updateservice.StagePending(dataRoot, crashChecked.Current, crashChecked, time.Now())
	must(t, e)
	crashBinary := os.Getenv("MAESTRO_SYNCED_CRASH_HELPER")
	if crashBinary == "" {
		crashBinary = filepath.Join(root, "installtx-crash.test")
		command := exec.CommandContext(ctx, "go", "test", "-mod=readonly", "-c", "-o", crashBinary, "./internal/installtx")
		command.Dir = source
		if output, e := command.CombinedOutput(); e != nil {
			t.Fatalf("compile focused crash witness: %s: %v", output, e)
		}
	}
	crashInput := filepath.Join(root, "crash-input.json")
	crashBody, _ := json.Marshal(map[string]string{"UserHome": user, "ManagedRoot": r.ManagedRoot, "DataRoot": dataRoot, "PlanID": crashPending.Plan.ID, "Registry": registryPath, "Output": filepath.Join(root, "crash-proof.json")})
	must(t, os.WriteFile(crashInput, crashBody, 0600))
	crashCommand := exec.CommandContext(ctx, crashBinary, "-test.run", "^TestSyncedSourceCrashReconciliation$", "-test.v")
	crashCommand.Env = append(os.Environ(), "MAESTRO_SYNCED_CRASH_FIXTURE="+crashInput)
	if output, e := crashCommand.CombinedOutput(); e != nil {
		t.Fatalf("actual source-aware crash reconciliation: %s: %v", output, e)
	}
	reconciled, applied, err := installer.ReconcileSeeded(ctx, installer.SeededUpdateOptions{SeededOperationOptions: seed, PlanID: crashPending.Plan.ID})
	must(t, err)
	if !applied || reconciled.Release != "0.1.43" || reconciled.Source.Sequence != 4 {
		t.Fatal("installer did not read back source-aware reconciled payload")
	}
	rollbackReview, err = installer.ReviewRollbackSeeded(ctx, seed)
	must(t, err)
	current, err = installer.RollbackSeeded(ctx, installer.SeededRollbackOptions{SeededOperationOptions: seed, ReviewDigest: rollbackReview.Digest})
	must(t, err)
	freshN := connectQualifiedSource(t, ctx, seed, current.Binding, ws)
	defer freshN.Close()
	call(t, ctx, freshN, "maestro_context", map[string]any{}, false)
	// An authentic later observation recalls the previous/candidate release;
	// rollback and candidate readiness cannot erase that accumulated denial.
	recalls := []releasecontract.ReleaseRecall{{Release: next.manifest.Release, ManifestSHA256: next.verified.ManifestSHA256}}
	publish(5, n, recalls)
	_, err = check()
	if !errors.Is(err, updateservice.ErrCurrent) {
		t.Fatal(err)
	}
	if _, err = installer.ReviewRollbackSeeded(ctx, seed); err == nil {
		t.Fatal("recalled previous release remained rollback eligible")
	}
	publish(6, next, recalls)
	_, err = check()
	if err == nil || errors.Is(err, updateservice.ErrCurrent) {
		t.Fatal("recalled candidate accepted")
	}
	publish(10, n, recalls)
	_, err = check()
	if !errors.Is(err, updateservice.ErrCurrent) {
		t.Fatal(err)
	}
	freshMetadata := read(t, filepath.Join(syncRoot, "current.json"))
	freshSignature := read(t, filepath.Join(syncRoot, "current.json.sig"))
	var expired releasecontract.Generation
	must(t, json.Unmarshal(freshMetadata, &expired))
	expired.Sequence = 11
	expired.PublishedAt = now.Add(-2 * time.Hour).Format(time.RFC3339)
	expired.ExpiresAt = now.Add(-time.Hour).Format(time.RFC3339)
	expired.Release = "0.1.99"
	expired.ManifestSHA256 = strings.Repeat("9", 64)
	expired.ClientInputsSHA256 = strings.Repeat("8", 64)
	expired.Recalls = append(expired.Recalls, releasecontract.ReleaseRecall{Release: n.manifest.Release, ManifestSHA256: n.verified.ManifestSHA256})
	body, err := releasecontract.CanonicalJSON(expired)
	must(t, err)
	signature := ed25519.Sign(private, body)
	immutable := filepath.Join(syncRoot, "generations", expired.GenerationDirectory(sum(body)))
	must(t, os.MkdirAll(immutable, 0700))
	must(t, os.WriteFile(filepath.Join(immutable, "generation.json"), body, 0600))
	must(t, os.WriteFile(filepath.Join(immutable, "generation.json.sig"), signature, 0600))
	must(t, os.WriteFile(filepath.Join(syncRoot, "current.json"), body, 0600))
	must(t, os.WriteFile(filepath.Join(syncRoot, "current.json.sig"), signature, 0600))
	blocked, err := check()
	if err == nil || blocked.Current.Source == nil || blocked.Current.Source.Sequence != 11 || len(blocked.Current.Source.Recalls) != 2 {
		t.Fatalf("expired higher denial was lost: state=%+v err=%v", blocked.Current, err)
	}
	if _, err = installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: seed, ExpectedBinding: current.Binding, Workspace: ws}); err == nil {
		t.Fatal("expired/recalled current release launched")
	}
	blockedResult := call(t, ctx, freshN, "maestro_context", map[string]any{}, true)
	if blockedResult["code"] != "source_recovery_required" || !strings.Contains(blockedResult["next_action"].(string), "maestro-install") {
		t.Fatal("source refusal lacks bounded independent recovery guidance")
	}
	stateBytes := read(t, filepath.Join(dataRoot, "config/install-state.json"))
	must(t, os.WriteFile(filepath.Join(syncRoot, "current.json"), freshMetadata, 0600))
	must(t, os.WriteFile(filepath.Join(syncRoot, "current.json.sig"), freshSignature, 0600))
	_, err = check()
	if err == nil || !bytes.Equal(stateBytes, read(t, filepath.Join(dataRoot, "config/install-state.json"))) {
		t.Fatal("older fresh replay erased expired higher recall")
	}
	// A fresh nonrecalled target can recover the healthy source-blocked current
	// executable through the independent seed, retaining every prior denial.
	publish(12, recovery, append([]releasecontract.ReleaseRecall{}, expired.Recalls...))
	recoveryCheck, e := check()
	must(t, e)
	recoveryPending, e := updateservice.StagePending(dataRoot, recoveryCheck.Current, recoveryCheck, time.Now())
	must(t, e)
	recovered, e := installer.UpdateSeeded(ctx, installer.SeededUpdateOptions{SeededOperationOptions: seed, PlanID: recoveryPending.Plan.ID})
	must(t, e)
	if recovered.Release != "0.1.45" || recovered.Source.Sequence != 12 || len(recovered.Source.Recalls) != 2 {
		t.Fatal("source-blocked recovery lost target or accumulated denial")
	}
	if _, e := installer.ReviewRollbackSeeded(ctx, seed); e == nil {
		t.Fatal("recovery re-enabled recalled previous release")
	}
	recoveryLaunch, e := installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: seed, ExpectedBinding: recovered.Binding, Workspace: ws})
	must(t, e)
	recoverySession, e := mcp.NewClient(&mcp.Implementation{Name: "recovered-source-client", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: exec.CommandContext(ctx, recoveryLaunch.Executable, recoveryLaunch.Args...), TerminateDuration: time.Second}, nil)
	must(t, e)
	readback := call(t, ctx, recoverySession, "maestro_query", map[string]any{"operation": "execution.next", "item_id": itemID}, false)
	if readback["next_step"] != "preserve this literal step" {
		t.Fatal("source-blocked upgrade lost continuation")
	}
	must(t, recoverySession.Close())
	if !reflect.DeepEqual(ownerBefore, snapshot(t, filepath.Join(user, ".maestro"))) || !reflect.DeepEqual(workspaceBefore, snapshot(t, ws)) {
		t.Fatal("source-blocked recovery changed owner/workspace bodies")
	}
	// Confirmed authentic denial cannot be discarded when cumulative recall
	// custody reaches its finite capacity. Exercise the actual public owners.
	capacityRecalls := make([]releasecontract.ReleaseRecall, 254)
	for i := range capacityRecalls {
		capacityRecalls[i] = releasecontract.ReleaseRecall{Release: fmt.Sprintf("0.2.%d", i+1), ManifestSHA256: fmt.Sprintf("%064x", i+1)}
	}
	publish(13, recovery, capacityRecalls)
	capacityCheck, e := check()
	if !errors.Is(e, updateservice.ErrCurrent) {
		t.Fatalf("fill bounded source custody: %v", e)
	}
	if len(capacityCheck.Current.Source.Recalls) != 256 {
		t.Fatal("recall capacity fixture did not reach exact bound")
	}
	_, e = installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: seed, ExpectedBinding: recovered.Binding, Workspace: ws})
	must(t, e)
	capacityMetadata := read(t, filepath.Join(syncRoot, "current.json"))
	capacitySignature := read(t, filepath.Join(syncRoot, "current.json.sig"))
	publish(14, recovery, []releasecontract.ReleaseRecall{{Release: recovery.manifest.Release, ManifestSHA256: recovery.verified.ManifestSHA256}})
	_, overflowErr := check()
	overflowState, e := installtx.ReadStateForManagedRoot(dataRoot, r.ManagedRoot)
	must(t, e)
	_, overflowLaunchErr := installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: seed, ExpectedBinding: recovered.Binding, Workspace: ws})
	overflowProof := map[string]any{"accepted_sequence": overflowState.Source.Sequence, "recall_count": len(overflowState.Source.Recalls), "launch_allowed": overflowLaunchErr == nil, "check_error": fmt.Sprint(overflowErr)}
	overflowProofBody, _ := json.MarshalIndent(overflowProof, "", "  ")
	must(t, os.WriteFile(filepath.Join(root, "recall-capacity-proof.json"), append(overflowProofBody, '\n'), 0600))
	if overflowErr == nil || overflowState.Source.Sequence != 14 || overflowLaunchErr == nil {
		t.Fatalf("confirmed authentic overflow discarded denial: sequence=%d recalls=%d launch_allowed=%t check=%v", overflowState.Source.Sequence, len(overflowState.Source.Recalls), overflowLaunchErr == nil, overflowErr)
	}
	var sourceWire struct {
		Source struct {
			Exhausted bool `json:"recall_capacity_exhausted"`
		} `json:"source_control"`
	}
	must(t, json.Unmarshal(read(t, filepath.Join(dataRoot, "config/install-state.json")), &sourceWire))
	if !sourceWire.Source.Exhausted {
		t.Fatal("capacity denial lacks a durable monotonic marker")
	}
	startup := exec.CommandContext(ctx, recoveryLaunch.Executable, recoveryLaunch.Args...)
	if output, e := startup.CombinedOutput(); e == nil {
		t.Fatalf("capacity-exhausted server restart admitted: %s", output)
	}
	if _, e := installer.ReviewRollbackSeeded(ctx, seed); e == nil || !strings.Contains(e.Error(), "capacity") {
		t.Fatalf("rollback did not enforce sticky source capacity denial: %v", e)
	}
	publish(15, recovery, []releasecontract.ReleaseRecall{})
	_, e = check()
	if e == nil || !strings.Contains(e.Error(), "capacity") {
		t.Fatalf("higher metadata cleared capacity denial: %v", e)
	}
	stickyBytes := read(t, filepath.Join(dataRoot, "config/install-state.json"))
	must(t, os.WriteFile(filepath.Join(syncRoot, "current.json"), capacityMetadata, 0600))
	must(t, os.WriteFile(filepath.Join(syncRoot, "current.json.sig"), capacitySignature, 0600))
	_, e = check()
	if e == nil || !bytes.Equal(stickyBytes, read(t, filepath.Join(dataRoot, "config/install-state.json"))) {
		t.Fatal("older fresh replay cleared capacity denial")
	}
	proof := map[string]any{"state": "real_signed_source_update_rollback_expired_recall_recovery_capacity_denial_passed", "native_admission": "controlled_in_process_engineering_seed_verifier_only", "real_stdio": true, "trusted_forms": forms, "resource_count": len(identity.Resources), "source_sequence": 15, "recall_capacity_exhausted": true, "recovered_at_sequence": 12, "retained_expired_denial_sequence": 11, "recovered_release": recovered.Release, "n_manifest": n.verified.ManifestSHA256, "n1_manifest": next.verified.ManifestSHA256, "owner_workspace_bytes": "preserved", "native_clients": "not_exercised", "hosted_ci": "not_exercised", "publication": "not_exercised"}
	proofBody, _ := json.MarshalIndent(proof, "", "  ")
	must(t, os.WriteFile(filepath.Join(root, "synced-lifecycle-evidence.json"), append(proofBody, '\n'), 0600))
	t.Log(string(proofBody))
}

// Consume the canonical qualified-launch executable and argv directly. Stable
// client registrations and complete source-bound carriers are a separate unit;
// the signed lifecycle proof never reconstructs a guessed packet command.
func connectQualifiedSource(t *testing.T, ctx context.Context, seed installer.SeededOperationOptions, binding installtx.ReleaseBinding, workspace string) *mcp.ClientSession {
	t.Helper()
	launch, err := installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: seed, ExpectedBinding: binding, Workspace: workspace})
	must(t, err)
	command := exec.CommandContext(ctx, launch.Executable, launch.Args...)
	client := mcp.NewClient(&mcp.Implementation{Name: "qualified-source-engineering", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: command}, nil)
	must(t, err)
	return session
}
