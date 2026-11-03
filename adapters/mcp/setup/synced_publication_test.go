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

// This narrowly labelled engineering process uses the real Identity/Server and
// process guard. Only its trusted constructor LookupEnv synchronizes execution;
// the shipping CLI has no hook, flag, approval or identity override for it.
func TestSyncedPublicationInterleavingAndReadOnlyReviews(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("actual admitted-home engineering runner required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()
	root := t.TempDir()
	if retained := os.Getenv("MAESTRO_SYNCED_R1_TEST_ROOT"); retained != "" {
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
	pin := sum(registryBody)
	registry, err := releaseverify.LoadAuthorityRegistry(registryPath, time.Now)
	must(t, err)
	build := func(version string) releasepack.ModernCandidate {
		candidate, e := releasepack.BuildModernCandidate(ctx, releasepack.ModernOptions{Root: source, Output: filepath.Join(root, "factory-"+version), Version: version, InstallerVersion: "0.3.7", Channel: "canary", AuthorityRegistry: registryPath, Target: releasepack.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}})
		must(t, e)
		return candidate
	}
	n := build("0.1.42")
	n1 := build("0.1.43")
	process := filepath.Join(root, "controlled-publication-n")
	pkg := "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/cmd/maestro-mcp"
	ldflags := "-X " + pkg + ".Version=0.1.42 -X '" + pkg + ".CompatibleBundle=" + n.Manifest.Server.CompatibleBundle + "' -X " + pkg + ".AuthorityRegistrySHA256=" + pin
	command := exec.CommandContext(ctx, "go", "test", "-mod=readonly", "-c", "-ldflags", ldflags, "-o", process, "./cmd/maestro-mcp")
	command.Dir = filepath.Join(source, "adapters/mcp")
	if output, e := command.CombinedOutput(); e != nil {
		t.Fatalf("build real controlled test process: %s: %v", output, e)
	}
	identity, err := releasecontract.ReadServerIdentity(process)
	must(t, err)
	if identity.Version != "0.1.42" || identity.ExecutionContract != releasecontract.SyncedSourceContract || identity.EmbeddedInputsSHA256 != n.Manifest.Server.EmbeddedInputsSHA256 || identity.AuthorityRegistrySHA256 != pin {
		t.Fatal("controlled process identity differs from real declared compiled inputs")
	}
	controlledManifest := n.Manifest
	for i, a := range controlledManifest.Artifacts {
		if a.Kind == "mcp_server" {
			body := read(t, process)
			must(t, os.WriteFile(filepath.Join(n.Directory, a.Name), body, 0755))
			controlledManifest.Artifacts[i].SHA256 = sum(body)
			controlledManifest.Artifacts[i].Size = int64(len(body))
		}
	}
	controlledManifest.Delivery.ClientInputsSHA256, err = controlledManifest.ClientInputsDigest()
	must(t, err)
	body, _ := json.MarshalIndent(controlledManifest, "", "  ")
	must(t, os.WriteFile(filepath.Join(n.Directory, "release-manifest.json"), append(body, '\n'), 0600))
	sign := func(candidate releasepack.ModernCandidate, name string) (string, releasecontract.Manifest, releaseverify.VerifiedRelease) {
		directory := filepath.Join(root, name)
		manifest, e := releasepack.SignCandidate(releasepack.SignCandidateOptions{Candidate: candidate.Directory, Output: directory, Registry: registryPath, Issuer: "engineering-fixture", KeyID: "ephemeral", PrivateKey: private})
		must(t, e)
		verified, e := releaseverify.VerifyDirectory(directory, registry)
		must(t, e)
		return directory, manifest, verified
	}
	nDir, nManifest, nVerified := sign(n, "signed-controlled-n")
	n1Dir, n1Manifest, n1Verified := sign(n1, "signed-production-n1")
	seedSHA := sum(read(t, n.Installer))
	if sum(read(t, n1.Installer)) != seedSHA {
		t.Fatal("independent installer changed between actual builds")
	}
	config := setup.Config{Installer: n.Installer, AuthorityRegistry: n.AuthorityRegistry, LookupEnv: func(string) (string, bool) { return "", false }}
	config.VerifyNative = func(_ context.Context, path string) error {
		body, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		if sum(body) != seedSHA {
			return errors.New("controlled engineering native seed bytes changed")
		}
		return nil
	}
	config.Run = func(ctx context.Context, self string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "install" {
			flags := flag.NewFlagSet("engineering-inner", flag.ContinueOnError)
			wire := flags.String("confirmed-plan", "", "")
			digest := flags.String("review-digest", "", "")
			confirmed := flags.Bool("confirm", false, "")
			if e := flags.Parse(args[1:]); e != nil {
				return nil, e
			}
			var plan installer.Plan
			if e := releasecontract.DecodeStrictJSON([]byte(*wire), &plan); e != nil {
				return nil, e
			}
			state, e := installer.ActivateSeeded(ctx, installer.SeededActivationOptions{Self: self, Version: "0.3.7", AuthorityRegistrySHA256: pin, Plan: plan, ReviewDigest: *digest, Confirmed: *confirmed, VerifyNative: config.VerifyNative})
			if e != nil {
				return nil, e
			}
			return json.Marshal(map[string]string{"state": "activated", "profile": state.Profile})
		}
		return exec.CommandContext(ctx, self, args...).CombinedOutput()
	}
	for _, withPins := range []bool{false, true} {
		name := "direct_serve"
		if withPins {
			name = "bound_carrier"
		}
		t.Run(name, func(t *testing.T) {
			caseRoot := filepath.Join(root, name)
			must(t, os.Mkdir(caseRoot, 0700))
			user, work := filepath.Join(caseRoot, "owner"), filepath.Join(caseRoot, "workspace")
			must(t, os.Mkdir(user, 0700))
			must(t, os.Mkdir(work, 0700))
			request := setup.Request{ReleaseDirectory: nDir, ManagedRoot: filepath.Join(caseRoot, "managed"), UserHome: user, Workspace: work, CodePacket: filepath.Join(caseRoot, "code-n"), DesktopPacket: filepath.Join(caseRoot, "desktop-n.mcpb")}
			review, e := setup.ReviewInstallation(request, config)
			must(t, e)
			installed, e := setup.Apply(ctx, request, config, review.Digest, true)
			must(t, e)
			dataRoot := filepath.Join(user, ".maestro-install")
			seed := installer.SeededOperationOptions{Self: filepath.Join(request.ManagedRoot, "maestro-install"), Version: "0.3.7", AuthorityRegistrySHA256: pin, UserHome: user, ManagedRoot: request.ManagedRoot, DataRoot: dataRoot, VerifyNative: config.VerifyNative}
			synced := filepath.Join(caseRoot, "synced")
			tuple := releasecontract.SourceTuple{SourceID: "r1-" + name, Channel: "canary", OS: runtime.GOOS, Arch: runtime.GOARCH}
			now := time.Now().UTC().Truncate(time.Second)
			publish := func(sequence uint64, directory string, m releasecontract.Manifest, v releaseverify.VerifiedRelease) {
				g := releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: m.Issuer, Source: tuple, Sequence: sequence, PublishedAt: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(7 * 24 * time.Hour).Format(time.RFC3339), Release: m.Release, ManifestSHA256: v.ManifestSHA256, ClientInputsSHA256: m.Delivery.ClientInputsSHA256, Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}
				_, e := releasepack.PublishGeneration(releasepack.PublishGenerationOptions{Root: synced, ReleaseDirectory: directory, Registry: registryPath, Generation: g, PrivateKey: private})
				must(t, e)
			}
			sourceOptions := installer.SeededSourceOptions{SeededOperationOptions: seed, SourceRoot: synced, ExpectedSource: tuple}
			check := func() (updateservice.CheckResult, error) {
				review, e := installer.ReviewSourceSeeded(ctx, sourceOptions)
				must(t, e)
				sourceOptions.ReviewDigest = review.Digest
				return installer.CheckSourceSeeded(ctx, sourceOptions)
			}
			publish(1, nDir, nManifest, nVerified)
			checked, e := check()
			if !errors.Is(e, updateservice.ErrCurrent) {
				t.Fatal(e)
			}
			initialBinding := checked.Current.Binding
			publish(2, n1Dir, n1Manifest, n1Verified)
			checked, e = check()
			must(t, e)
			pending, e := updateservice.StagePending(dataRoot, checked.Current, checked, time.Now())
			must(t, e)
			if !withPins {
				t.Run("read_only_before_activation", func(t *testing.T) { r1ReadOnlyReviews(t, ctx, caseRoot, seed, sourceOptions, pending, registry, false) })
			}
			barrier := filepath.Join(caseRoot, "barrier")
			must(t, os.Mkdir(barrier, 0700))
			args := []string{"serve", "--product-root", request.ManagedRoot, "--user-home", user, "--workspace", work}
			if withPins {
				args = append(args, "--installed-manifest", initialBinding.ManifestSHA256, "--installed-client-inputs", initialBinding.ClientInputsSHA256)
			}
			copied := filepath.Join(request.CodePacket, "scripts", "maestro-mcp")
			child := exec.CommandContext(ctx, copied, args...)
			child.Env = append(os.Environ(), "MAESTRO_SYNCED_R1_PUBLICATION_BARRIER="+barrier)
			literal := "R1 real copied N capture " + name
			client := mcp.NewClient(&mcp.Implementation{Name: "r1-publication-client", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				if !strings.Contains(req.Params.Message, literal) {
					return nil, errors.New("capture review omitted exact synthetic text")
				}
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}})
			cs, e := client.Connect(ctx, &mcp.CommandTransport{Command: child, TerminateDuration: time.Second}, &mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
			must(t, e)
			defer cs.Close()
			call(t, ctx, cs, "maestro_apply", map[string]any{"operation": "memory.capture", "text": literal}, false)
			engine, _, e := maintenance.NewProductMemoryEngine(installed.Home)
			must(t, e)
			day := time.Now().UTC().Format("2006-01-02")
			if _, _, e := engine.ActiveArtifact(installed.WorkspaceID, "L1/"+day); !errors.Is(e, os.ErrNotExist) {
				t.Fatalf("fixture already has an active L1: %v", e)
			}
			must(t, os.WriteFile(filepath.Join(barrier, "armed"), []byte("synchronize next real guarded publication\n"), 0600))
			type outcome struct {
				value *mcp.CallToolResult
				err   error
			}
			done := make(chan outcome, 1)
			go func() {
				result, e := cs.CallTool(ctx, &mcp.CallToolParams{Name: "maestro_apply", Arguments: map[string]any{"operation": "memory.consolidate", "cycle": "daily", "date": day}})
				done <- outcome{result, e}
			}()
			waitR1File(t, ctx, filepath.Join(barrier, "ready.json"))
			var ready struct {
				RunningSHA     string `json:"running_sha256"`
				CurrentSHA     string `json:"current_server_sha256"`
				CurrentRelease string `json:"current_release"`
				ManifestPin    string `json:"manifest_pin"`
			}
			must(t, json.Unmarshal(read(t, filepath.Join(barrier, "ready.json")), &ready))
			if ready.RunningSHA != checked.Current.Server.SHA256 || ready.CurrentSHA != ready.RunningSHA || ready.CurrentRelease != "0.1.42" || (ready.ManifestPin != "") != withPins {
				t.Fatal("barrier did not observe actual N after outer admission with exact carrier mode")
			}
			activated, e := installer.UpdateSeeded(ctx, installer.SeededUpdateOptions{SeededOperationOptions: seed, PlanID: pending.Plan.ID})
			must(t, e)
			if activated.Release != "0.1.43" || activated.Server.SHA256 == ready.RunningSHA {
				t.Fatal("canonical independent installer did not activate genuine N1 while N paused")
			}
			if !withPins {
				t.Run("read_only_after_real_receipt", func(t *testing.T) { r1ReadOnlyReviews(t, ctx, caseRoot, seed, sourceOptions, pending, registry, true) })
			}
			must(t, os.WriteFile(filepath.Join(barrier, "continue"), []byte("genuine N1 activation completed\n"), 0600))
			var result outcome
			select {
			case result = <-done:
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
			waitR1File(t, ctx, filepath.Join(barrier, "continued"))
			artifact, _, activeErr := engine.ActiveArtifact(installed.WorkspaceID, "L1/"+day)
			active := activeErr == nil
			succeeded := result.err == nil && result.value != nil && !result.value.IsError
			proof := map[string]any{"process_synchronization": "test-only trusted LookupEnv constructor; shipping binary has no synchronization flag", "native_admission": "controlled in-process engineering native seed verifier", "carrier_pins": withPins, "copied_process_sha256": ready.RunningSHA, "activated_server_sha256": activated.Server.SHA256, "activated_release": activated.Release, "terminal_success": succeeded, "active_domain_output": active, "active_domain_content": artifact.Content, "domain_error": fmt.Sprint(activeErr), "tool_error": fmt.Sprint(result.err)}
			proofBody, _ := json.MarshalIndent(proof, "", "  ")
			must(t, os.WriteFile(filepath.Join(caseRoot, "publication-proof.json"), append(proofBody, '\n'), 0600))
			if succeeded || active {
				t.Errorf("copied source-aware N published across canonical N1 activation: pins=%t terminal_success=%t active_output=%t", withPins, succeeded, active)
			}
		})
	}
}

func waitR1File(t *testing.T, ctx context.Context, path string) {
	t.Helper()
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-deadline.C:
			t.Fatalf("controlled test synchronization did not reach %s", path)
		case <-ticker.C:
			if _, err := os.Stat(path); err == nil {
				return
			}
		}
	}
}

func r1ReadOnlyReviews(t *testing.T, ctx context.Context, root string, seed installer.SeededOperationOptions, source installer.SeededSourceOptions, pending updateservice.Pending, registry releaseverify.KeyRegistry, receipt bool) {
	t.Helper()
	statePath := filepath.Join(seed.DataRoot, "config", "install-state.json")
	original := read(t, statePath)
	current, e := installtx.ReadSourceState(seed.DataRoot, seed.ManagedRoot)
	must(t, e)
	legacy, _ := json.MarshalIndent(map[string]any{"schema_version": 1, "release": current.Release, "channel": current.Channel, "cli_version": current.Server.Version, "bundle_version": current.BundleVersion, "target_os": current.TargetOS, "target_arch": current.TargetArch, "activated_at": current.ActivatedAt}, "", "  ")
	type check struct {
		name string
		call func() error
	}
	checks := []check{}
	if !receipt {
		checks = append(checks, check{"source_review", func() error { _, e := installer.ReviewSourceSeeded(ctx, source); return e }}, check{"rollback_review", func() error { _, e := installer.ReviewRollbackSeeded(ctx, seed); return e }}, check{"pending_review", func() error {
			_, e := installer.ReviewPendingSeeded(ctx, installer.SeededUpdateOptions{SeededOperationOptions: seed, PlanID: pending.Plan.ID})
			return e
		}}, check{"canonical_pending_review", func() error {
			_, _, e := updateservice.ConfirmPending(seed.DataRoot, seed.ManagedRoot, pending.Plan.ID, registry)
			return e
		}}, check{"compiled_source_review_derived_binding", func() error {
			body, _ := json.Marshal(map[string]string{"managed_root": seed.ManagedRoot, "user_home": seed.UserHome})
			command := exec.CommandContext(ctx, seed.Self, "source-review")
			command.Stdin = bytes.NewReader(body)
			_, e := command.CombinedOutput()
			return e
		}})
	} else {
		plan, e := installtx.ReadPlan(pending.ActivationPlanPath)
		must(t, e)
		if _, e := os.Stat(filepath.Join(filepath.Dir(pending.ActivationPlanPath), installtx.ReceiptName)); e != nil {
			t.Fatal("genuine canonical activation receipt absent")
		}
		options := installtx.PrepareOptions{UserHome: seed.UserHome, Transition: "update", ConfirmationPlanID: pending.Plan.ID, FromRelease: plan.FromRelease, FromChannel: plan.FromChannel, FromCLIVersion: plan.FromCLIVersion, FromBundleVersion: plan.FromBundleVersion, TargetOS: plan.TargetOS, TargetArch: plan.TargetArch, ManagedRoot: seed.ManagedRoot, DataRoot: seed.DataRoot, Source: plan.Source, SourceStateSHA256: plan.SourceStateSHA256}
		rollback, e := installer.ReviewRollbackSeeded(ctx, seed)
		must(t, e)
		checks = append(checks, check{"reconciliation_review", func() error { _, e := installtx.ValidateReconciliation(pending.ActivationPlanPath, options); return e }}, check{"reconciliation_error_readback", func() error {
			_, e := installtx.ReconcileActivated(pending.ActivationPlanPath, installtx.ActivateOptions{PrepareOptions: options})
			return e
		}}, check{"seeded_reconciliation_error_readback", func() error {
			_, _, e := installer.ReconcileSeeded(ctx, installer.SeededUpdateOptions{SeededOperationOptions: seed, PlanID: pending.Plan.ID})
			return e
		}}, check{"qualified_activation_rejection", func() error {
			return installtx.Activate(pending.ActivationPlanPath, releaseForR1(t, pending.VerifiedDirectory, registry), installtx.ActivateOptions{PrepareOptions: options})
		}}, check{"qualified_rollback_rejection", func() error { return installtx.RollbackQualified(seed.ManagedRoot, seed.DataRoot, rollback.Digest) }})
	}
	for _, route := range checks {
		t.Run(route.name, func(t *testing.T) {
			must(t, os.WriteFile(statePath, append(append([]byte(nil), legacy...), '\n'), 0600))
			defer func() { must(t, os.WriteFile(statePath, original, 0600)) }()
			before := snapshot(t, root)
			err := route.call()
			if err == nil {
				t.Error("unsupported valid schema1 received source-aware admission")
			}
			if !reflect.DeepEqual(before, snapshot(t, root)) {
				t.Errorf("new source-aware %s migrated valid schema1 before rejection: %v", route.name, err)
			}
		})
	}
}

func releaseForR1(t *testing.T, path string, registry releaseverify.KeyRegistry) releaseverify.VerifiedRelease {
	t.Helper()
	v, e := releaseverify.VerifyDirectory(path, registry)
	must(t, e)
	return v
}
