package setup_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
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
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This is a real offline product-process tracer. The sole engineering trust
// fixture is native installer attestation, bound to the exact built seed bytes.
// It never invokes a native Code/Desktop client or calls a model/provider.
func TestModernFactoryFirstInstallAndRestart(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("actual supported native runner required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	root := t.TempDir()
	if retained := os.Getenv("MAESTRO_MODERN_TEST_ROOT"); retained != "" {
		if err := os.Mkdir(retained, 0700); err != nil {
			t.Fatal(err)
		}
		root = retained
	}
	defer func() {
		if os.Getenv("MAESTRO_MODERN_TEST_ROOT") == "" {
			_ = filepath.WalkDir(root, func(path string, d os.DirEntry, e error) error {
				if e == nil && d.IsDir() {
					_ = os.Chmod(path, 0700)
				}
				return e
			})
		}
	}()
	source, err := filepath.Abs("../../..")
	must(t, err)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	registry, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "engineering-fixture", "key_id": "ephemeral", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": "2020-01-01T00:00:00Z", "valid_until": "2100-01-01T00:00:00Z"}}})
	registryPath := filepath.Join(root, "authority.json")
	must(t, os.WriteFile(registryPath, registry, 0600))
	pack := os.Getenv("MAESTRO_MODERN_DOCLING_PACK")
	packBefore := ""
	if pack != "" {
		fixtureRoot := filepath.Clean(filepath.Join(filepath.Dir(pack), "../../.."))
		fixtureInputs := []string{pack, pack + ".sig", filepath.Join(filepath.Dir(pack), "release-manifest.json"), filepath.Join(filepath.Dir(pack), "release-manifest.json.sig"), filepath.Join(fixtureRoot, "authority.json"), filepath.Join(filepath.Dir(pack), "installed", "e7cdd0f0f1506878ce3a9390f54798f6a51a288651e8e73a4b836dd67f43333a", "pack.json")}
		fixtureHashes := map[string]string{}
		for _, path := range fixtureInputs {
			fixtureHashes[path] = fileIdentity(t, path)
		}
		historicalRegistry, err := releaseverify.LoadAuthorityRegistry(filepath.Join(fixtureRoot, "authority.json"), time.Now)
		must(t, err)
		historical, key, err := releaseverify.VerifyManifest(read(t, filepath.Join(filepath.Dir(pack), "release-manifest.json")), read(t, filepath.Join(filepath.Dir(pack), "release-manifest.json.sig")), historicalRegistry)
		must(t, err)
		if historical.SchemaVersion != 1 || !ed25519.Verify(key, read(t, pack), read(t, pack+".sig")) {
			t.Fatal("historical signed pack authority changed")
		}
		defer func() {
			for path, identity := range fixtureHashes {
				if fileIdentity(t, path) != identity {
					t.Errorf("historical fixture changed %s", path)
				}
			}
			body, _ := json.MarshalIndent(fixtureHashes, "", "  ")
			must(t, os.WriteFile(filepath.Join(root, "docling-source-inputs.json"), append(body, byte(10)), 0600))
		}()
		packBefore = sum(read(t, pack))
		if packBefore != "e7cdd0f0f1506878ce3a9390f54798f6a51a288651e8e73a4b836dd67f43333a" {
			t.Fatal("supplied E4 pack differs from root attestation")
		}
		defer func() {
			if sum(read(t, pack)) != packBefore {
				t.Error("source pack changed")
			}
		}()
	}
	factory, err := releasepack.BuildModernCandidate(ctx, releasepack.ModernOptions{Root: source, Output: filepath.Join(root, "factory"), Version: "0.1.42", InstallerVersion: "0.3.7", Channel: "canary", AuthorityRegistry: registryPath, Target: releasepack.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}, DoclingPack: pack})
	must(t, err)
	release := filepath.Join(root, "signed-release")
	manifest, err := releasepack.SignCandidate(releasepack.SignCandidateOptions{Candidate: factory.Directory, Output: release, Registry: registryPath, Issuer: "engineering-fixture", KeyID: "ephemeral", PrivateKey: private})
	must(t, err)
	must(t, releasecontract.ValidateSchemaFile(filepath.Join(source, "schemas/release-manifest-v2.schema.json")))
	must(t, releasecontract.ValidateSchemaDocument(filepath.Join(source, "schemas/release-manifest-v2.schema.json"), read(t, filepath.Join(release, "release-manifest.json"))))
	seedBody := read(t, factory.Installer)
	seedSHA := sum(seedBody)
	nativeChecks := 0
	config := setup.Config{Installer: factory.Installer, AuthorityRegistry: factory.AuthorityRegistry, LookupEnv: func(string) (string, bool) { return "", false }, VerifyNative: func(_ context.Context, path string) error {
		nativeChecks++
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if sum(body) != seedSHA {
			return fmt.Errorf("engineering native verifier: seed bytes changed")
		}
		return nil
	}}
	// The successful inner native-admission owner is composed in process with
	// the controlled engineering verifier. Production command defaults remain
	// strict and are exercised by the separate direct-inner negative.
	config.Run = func(ctx context.Context, self string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "install" {
			flags := flag.NewFlagSet("engineering-inner-composition", flag.ContinueOnError)
			body := flags.String("confirmed-plan", "", "")
			digest := flags.String("review-digest", "", "")
			confirmed := flags.Bool("confirm", false, "")
			if err := flags.Parse(args[1:]); err != nil {
				return nil, err
			}
			var plan installer.Plan
			if err := releasecontract.DecodeStrictJSON([]byte(*body), &plan); err != nil {
				return nil, err
			}
			state, err := installer.ActivateSeeded(ctx, installer.SeededActivationOptions{Self: self, Version: "0.3.7", AuthorityRegistrySHA256: sum(registry), Plan: plan, ReviewDigest: *digest, Confirmed: *confirmed, VerifyNative: config.VerifyNative})
			if err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{"state": "activated", "profile": state.Profile, "native_admission": "controlled_in_process_engineering_fixture"})
		}
		return exec.CommandContext(ctx, self, args...).CombinedOutput()
	}
	user := filepath.Join(root, "owner")
	must(t, os.Mkdir(user, 0700))
	workspace := filepath.Join(root, "workspace")
	must(t, os.Mkdir(workspace, 0700))
	must(t, os.WriteFile(filepath.Join(workspace, "existing-owner.txt"), []byte("retain exact owner workspace bytes"), 0600))
	request := setup.Request{ReleaseDirectory: release, ManagedRoot: filepath.Join(root, "managed"), UserHome: user, Workspace: workspace, CodePacket: filepath.Join(root, "code"), DesktopPacket: filepath.Join(root, "desktop.mcpb")}
	before := snapshot(t, root)
	review, err := setup.ReviewInstallation(request, config)
	must(t, err)
	if !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("review changed filesystem")
	}
	if review.Installation.Profile != "mcp-local-v1" || review.Installation.BootstrapperVersion != "0.3.7" || review.Installation.ServerVersion != "0.1.42" || review.Installation.DataRoot != filepath.Join(user, ".maestro-install") || review.Home.Home != filepath.Join(user, ".maestro") {
		t.Fatalf("incorrect exact review: %+v", review)
	}
	if _, err = setup.Apply(ctx, request, config, review.Digest, false); err == nil {
		t.Fatal("missing confirmation accepted")
	}
	if _, err = setup.Apply(ctx, request, config, strings.Repeat("0", 64), true); err == nil {
		t.Fatal("stale digest accepted")
	}
	if !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("refused confirmation wrote state")
	}
	drifted := config
	drifted.LookupEnv = func(key string) (string, bool) {
		if key == "MAESTRO_HOME" {
			return filepath.Join(root, "different-reviewed-home"), true
		}
		return "", false
	}
	if _, err = setup.Apply(ctx, request, drifted, review.Digest, true); err == nil {
		t.Fatal("home selection drift accepted")
	}
	if !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("selection-drift refusal wrote state")
	}

	production := exec.CommandContext(ctx, factory.Installer, "review")
	requestBody, _ := json.Marshal(request)
	production.Stdin = bytes.NewReader(requestBody)
	if output, err := production.CombinedOutput(); err == nil {
		t.Fatalf("production command accepted an unsigned engineering seed: %s", output)
	}
	if !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("production native refusal wrote state")
	}
	// Existing populated, unadmitted owner data refuses without effects.
	foreignUser := filepath.Join(root, "ambiguous-owner")
	must(t, os.MkdirAll(filepath.Join(foreignUser, ".maestro"), 0700))
	foreignFile := filepath.Join(foreignUser, ".maestro", "foreign.txt")
	must(t, os.WriteFile(foreignFile, []byte("preserve ambiguous owner data"), 0600))
	foreign := request
	foreign.UserHome = foreignUser
	foreign.ManagedRoot = filepath.Join(root, "ambiguous-managed")
	foreign.CodePacket = filepath.Join(root, "ambiguous-code")
	foreign.DesktopPacket = filepath.Join(root, "ambiguous.mcpb")
	ambiguousBefore := snapshot(t, root)
	if _, err = setup.ReviewInstallation(foreign, config); err == nil {
		t.Fatal("populated unadmitted home accepted")
	}
	if !reflect.DeepEqual(ambiguousBefore, snapshot(t, root)) {
		t.Fatal("ambiguous-home refusal wrote state")
	}
	// Authenticated server and seed mutation must refuse before managed effects.
	serverName := ""
	for _, artifact := range manifest.Artifacts {
		if artifact.Kind == "mcp_server" {
			serverName = artifact.Name
		}
	}
	serverSource := filepath.Join(release, serverName)
	serverBody := read(t, serverSource)
	must(t, os.WriteFile(serverSource, []byte("tampered"), 0755))
	if _, err = setup.ReviewInstallation(request, config); err == nil {
		t.Fatal("tampered signed server accepted")
	}
	must(t, os.WriteFile(serverSource, serverBody, 0755))
	must(t, os.WriteFile(factory.Installer, []byte("tampered seed"), 0755))
	if _, err = setup.ReviewInstallation(request, config); err == nil {
		t.Fatal("tampered native seed accepted")
	}
	must(t, os.WriteFile(factory.Installer, seedBody, 0755))
	result, err := setup.Apply(ctx, request, config, review.Digest, true)
	must(t, err)
	if result.State != "ready" || result.WorkspaceID == "" || result.CodeRegistration != "pending" || result.DesktopLoading != "pending" || nativeChecks < 4 {
		t.Fatalf("incorrect setup result: %+v", result)
	}
	if string(read(t, filepath.Join(workspace, "existing-owner.txt"))) != "retain exact owner workspace bytes" {
		t.Fatal("owner workspace data changed")
	}
	state, err := installtx.ReadStateForManagedRoot(review.Installation.DataRoot, request.ManagedRoot)
	must(t, err)
	if state.SchemaVersion != 3 || state.Profile != "mcp-local-v1" || state.Server.Version != "0.1.42" {
		t.Fatalf("modern durable state: %+v", state)
	}
	assertChangedInstalledServerRefused(t, ctx, root, request, config, review, result, state)
	stateBytes := read(t, filepath.Join(review.Installation.DataRoot, "config", "install-state.json"))
	if bytes.Contains(stateBytes, []byte(`"cli_version"`)) {
		t.Fatal("modern state relabelled the legacy CLI")
	}
	identity, err := releasecontract.ReadServerIdentity(result.Installation.CLIPath)
	must(t, err)
	if identity.Version != "0.1.42" || identity.AuthorityRegistrySHA256 != sum(registry) || identity.EmbeddedInputsSHA256 != manifest.Server.EmbeddedInputsSHA256 {
		t.Fatal("installed server/seed identity mismatch")
	}
	plans, err := filepath.Glob(filepath.Join(review.Installation.DataRoot, "updates", "tx-*", "activation-plan.json"))
	must(t, err)
	if len(plans) != 1 {
		t.Fatalf("unexpected canonical transactions: %v", plans)
	}
	for path, wantVersion := range map[string]int{plans[0]: 3, filepath.Join(filepath.Dir(plans[0]), "activation-receipt.json"): 2} {
		var wire map[string]any
		raw := read(t, path)
		must(t, json.Unmarshal(raw, &wire))
		if wire["schema_version"] != float64(wantVersion) || wire["profile"] != "mcp-local-v1" || wire["server"].(map[string]any)["version"] != "0.1.42" {
			t.Fatalf("wrong protected modern wire: %s", raw)
		}
		for _, key := range []string{"cli_version", "cli_sha256", "cli_size", "cli_artifact_name", "staged_cli"} {
			if _, present := wire[key]; present {
				t.Fatalf("modern wire retained historical executable field %s", key)
			}
		}
	}
	assertSourceClosure(t, source, identity, request.ManagedRoot, manifest.Bundle.Version)
	trapUser := filepath.Join(root, "checker-trap-owner")
	must(t, os.Mkdir(trapUser, 0700))
	authority, err := releaseverify.LoadAuthorityRegistry(registryPath, time.Now)
	must(t, err)
	trustedRelease, err := releaseverify.VerifyDirectory(release, authority)
	must(t, err)
	trap := installtx.PrepareOptions{UserHome: trapUser, Transition: "install", ManagedRoot: filepath.Join(root, "checker-trap-managed"), DataRoot: filepath.Join(trapUser, ".maestro-install"), TargetOS: runtime.GOOS, TargetArch: runtime.GOARCH}
	trapPlan, err := installtx.Prepare(trustedRelease, trap)
	must(t, err)
	called := false
	err = installtx.Activate(trapPlan, trustedRelease, installtx.ActivateOptions{PrepareOptions: trap, CheckCLI: func(string, string) error { called = true; return nil }})
	if err == nil || called || !strings.Contains(err.Error(), "legacy checker") {
		t.Fatalf("modern activation used injected healthy callback: %v called=%v", err, called)
	}
	if _, err = os.Lstat(trap.ManagedRoot); !os.IsNotExist(err) {
		t.Fatal("callback refusal created managed payload")
	}

	assertPacket(t, request.CodePacket, identity)
	desktopRoot := filepath.Join(root, "desktop-extracted")
	extract(t, request.DesktopPacket, desktopRoot)
	assertPacket(t, desktopRoot, identity)
	for _, binary := range []string{result.Installation.CLIPath, filepath.Join(request.CodePacket, "scripts", filepath.Base(result.Installation.CLIPath)), filepath.Join(desktopRoot, "server", filepath.Base(result.Installation.CLIPath))} {
		carrier, err := releasecontract.ReadServerIdentity(binary)
		must(t, err)
		if !reflect.DeepEqual(carrier, identity) {
			t.Fatal("carrier identity differs")
		}
		if sum(read(t, binary)) != sum(serverBody) {
			t.Fatal("carrier executable bytes differ")
		}
		cs := connect(t, ctx, binary, request)
		if cs.InitializeResult().ServerInfo.Version != "0.1.42" {
			t.Fatal("SDK used an independent default version")
		}
		status := call(t, ctx, cs, "maestro_query", map[string]any{"operation": "content.status"}, false)
		if status["state"] != result.Content.State {
			t.Fatalf("fixed installed/carrier registry layout failed: %v", status)
		}
		if binary == result.Installation.CLIPath {
			listed, err := cs.ListResources(ctx, nil)
			must(t, err)
			if len(listed.Resources) != len(identity.Resources) {
				t.Fatal("real MCP discovery omitted public resources")
			}
			for _, resource := range identity.Resources {
				out, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: resource.URI})
				must(t, err)
				if len(out.Contents) != 1 || sum([]byte(out.Contents[0].Text)) != resource.SHA256 || int64(len(out.Contents[0].Text)) != resource.Bytes {
					t.Fatalf("real MCP resource bytes differ %s", resource.URI)
				}
			}
		}
		must(t, cs.Close())
		registryLocation := filepath.Join(filepath.Dir(filepath.Dir(binary)), "managed", "trust", "release-authority-registry.json")
		if filepath.Base(filepath.Dir(binary)) == "bin" {
			registryLocation = filepath.Join(request.ManagedRoot, "trust", "release-authority-registry.json")
		}
		originalRegistry := read(t, registryLocation)
		for _, kind := range []string{"wrong_pin", "absent"} {
			if kind == "wrong_pin" {
				must(t, os.WriteFile(registryLocation, append(append([]byte(nil), originalRegistry...), byte(10)), 0600))
			} else {
				must(t, os.Rename(registryLocation, registryLocation+".held"))
			}
			refused := connect(t, ctx, binary, request)
			status := call(t, ctx, refused, "maestro_query", map[string]any{"operation": "content.status"}, false)
			must(t, refused.Close())
			if status["state"] != "unavailable" || status["reason"] != "authority_unavailable" {
				t.Fatalf("fixed seed layout accepted %s: %v", kind, status)
			}
			if kind == "absent" {
				must(t, os.Rename(registryLocation+".held", registryLocation))
			} else {
				must(t, os.WriteFile(registryLocation, originalRegistry, 0600))
			}
		}
	}
	// A real process writes/pause-checkpoints; a fresh carrier process resumes it.
	first := connect(t, ctx, result.Installation.CLIPath, request)
	created := call(t, ctx, first, "maestro_apply", map[string]any{"operation": "execution.create", "objective": "synthetic modern install continuation", "next_step": "write exact artifact", "artifacts": []string{"result.md"}}, false)
	id := created["item_id"].(string)
	started := call(t, ctx, first, "maestro_apply", map[string]any{"operation": "execution.start", "item_id": id, "expected_revision": 1}, false)
	attempt := started["attempt_id"].(string)
	checkpoint := call(t, ctx, first, "maestro_apply", map[string]any{"operation": "execution.checkpoint", "item_id": id, "expected_revision": started["state_revision"], "attempt_id": attempt, "summary": "independent literal checkpoint", "next_step": "verify modern restart artifact"}, false)
	paused := call(t, ctx, first, "maestro_apply", map[string]any{"operation": "execution.pause", "item_id": id, "expected_revision": checkpoint["state_revision"], "attempt_id": attempt}, false)
	must(t, first.Close())
	restarted := connect(t, ctx, filepath.Join(desktopRoot, "server", filepath.Base(result.Installation.CLIPath)), request)
	defer restarted.Close()
	page := call(t, ctx, restarted, "maestro_query", map[string]any{"operation": "execution.list", "limit": 32}, false)
	if len(page["items"].([]any)) != 1 || page["items"].([]any)[0].(map[string]any)["state"] != "paused" {
		t.Fatalf("restart lost paused work: %v", page)
	}
	resumed := call(t, ctx, restarted, "maestro_apply", map[string]any{"operation": "execution.resume", "item_id": id, "expected_revision": paused["state_revision"]}, false)
	if resumed["attempt_id"] == attempt {
		t.Fatal("restart reused fenced attempt")
	}
	next := call(t, ctx, restarted, "maestro_query", map[string]any{"operation": "execution.next", "item_id": id}, false)
	if next["next_step"] != "verify modern restart artifact" {
		t.Fatalf("checkpoint readback mismatch: %v", next)
	}
	var conversionProof any = map[string]string{"state": "unavailable", "reason": "no_pack"}
	if pack == "" {
		if result.Content.State != "unavailable" {
			t.Fatal("absent converter claimed available")
		}
	} else {
		if result.Content.State != "ready" {
			t.Fatalf("supplied converter: %+v", result.Content)
		}
		fixture := read(t, filepath.Join(source, "internal/ingest/docling/testdata/sample.docx"))
		must(t, os.WriteFile(filepath.Join(workspace, "sample.docx"), fixture, 0600))
		bound, err := product.Open(product.Options{ProductRoot: request.ManagedRoot, UserHome: user, WorkspacePath: workspace, LookupEnv: config.LookupEnv, CLIVersion: identity.Version, AuthorityRegistryPath: filepath.Join(request.ManagedRoot, "trust/release-authority-registry.json"), AuthorityRegistrySHA256: sum(registry)})
		must(t, err)
		converted, err := bound.DoclingRuntime().Convert(ctx, ingest.Request{SourcePath: "sample.docx", WorkspacePath: workspace, Policy: ingest.DefaultPolicy()}, result.WorkspaceID)
		must(t, err)
		if converted.Result.Status != ingest.StatusUsable || !strings.Contains(string(converted.Markdown), `MAESTRO\_DOCLING\_OFFLINE\_20261101`) || !bytes.Contains(converted.Document, []byte("MAESTRO_DOCLING_OFFLINE_20261101")) || !converted.Provenance.NetworkDenied {
			t.Fatalf("real signed converter literal oracle absent: %+v", converted.Result)
		}
		var document struct {
			Tables []struct {
				Data struct {
					Rows  int `json:"num_rows"`
					Cols  int `json:"num_cols"`
					Cells []struct {
						Text string `json:"text"`
					} `json:"table_cells"`
				} `json:"data"`
			} `json:"tables"`
		}
		must(t, json.Unmarshal(converted.Document, &document))
		if len(document.Tables) != 1 || document.Tables[0].Data.Rows != 2 || document.Tables[0].Data.Cols != 2 {
			t.Fatal("qualified deterministic table shape changed")
		}
		cells := map[string]bool{}
		for _, cell := range document.Tables[0].Data.Cells {
			cells[cell.Text] = true
		}
		if len(cells) != 4 || !cells["Item"] || !cells["Count"] || !cells["alpha"] || !cells["7"] || converted.Result.SourceSHA256 != "6bd282a12170035e8af5352f2de5a21f4fe4bc5c7919283482a26703c206bf81" {
			t.Fatal("qualified deterministic source/table literal changed")
		}
		conversionProof = converted.Provenance

	}
	if pack == "" {
		assertSetupPendingRecovery(t, ctx, root, request, config, seedSHA)
		assertLegacyRequestPreservesModernInstall(t, ctx, root, request, config, manifest, private)
	}
	if baseline := os.Getenv("MAESTRO_MODERN_BASELINE"); baseline != "" {
		assertBaselineRefuses(t, ctx, root, baseline, review.Installation.DataRoot, request.ManagedRoot)
	} else {
		t.Log("baseline reader/writer witness not requested in this run")
	}
	report := map[string]any{"profile": manifest.Delivery.Profile, "server_protocol": identity.Protocol, "converter_proof": conversionProof, "server_version": identity.Version, "installer_version": review.Installation.BootstrapperVersion, "server_sha256": sum(serverBody), "installer_sha256": seedSHA, "registry_sha256": sum(registry), "inventory_sha256": identity.EmbeddedInputsSHA256, "resources": len(identity.Resources), "content": result.Content, "baseline_requested": os.Getenv("MAESTRO_MODERN_BASELINE") != "", "state": "actual_factory_activation_admission_packets_stdio_restart_passed", "native_attestation": "engineering_fixture_only", "inner_activation": "controlled_in_process_native_verifier_composition", "code_registration": "pending", "desktop_loading": "pending"}
	body, _ := json.MarshalIndent(report, "", "  ")
	must(t, os.WriteFile(filepath.Join(root, "evidence.json"), append(body, '\n'), 0600))
	t.Log(string(body))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func read(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	must(t, err)
	return body
}
func sum(body []byte) string { return fmt.Sprintf("%x", sha256.Sum256(body)) }
func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	must(t, filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		key, _ := filepath.Rel(root, path)
		result[key] = info.Mode().String()
		if !d.IsDir() {
			result[key] += sum(read(t, path))
		}
		return nil
	}))
	return result
}
func connect(t *testing.T, ctx context.Context, binary string, r setup.Request) *mcp.ClientSession {
	t.Helper()
	cmd := exec.CommandContext(ctx, binary, "serve", "--product-root", r.ManagedRoot, "--user-home", r.UserHome, "--workspace", r.Workspace)
	cmd.Env = []string{}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "MAESTRO_HOME=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "modern-install-offline-tracer", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
	must(t, err)
	return cs
}
func call(t *testing.T, ctx context.Context, cs *mcp.ClientSession, tool string, args map[string]any, wantError bool) map[string]any {
	t.Helper()
	out, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	must(t, err)
	if out.IsError != wantError {
		t.Fatalf("%s %#v: %+v", tool, args, out)
	}
	var value map[string]any
	must(t, json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &value))
	return value
}
func extract(t *testing.T, path, destination string) {
	t.Helper()
	archive, err := zip.OpenReader(path)
	must(t, err)
	defer archive.Close()
	must(t, os.Mkdir(destination, 0700))
	for _, entry := range archive.File {
		target := filepath.Join(destination, filepath.FromSlash(entry.Name))
		if strings.Contains(entry.Name, "..") || filepath.IsAbs(entry.Name) {
			t.Fatal("unsafe packet")
		}
		must(t, os.MkdirAll(filepath.Dir(target), 0700))
		file, err := entry.Open()
		must(t, err)
		body, err := io.ReadAll(file)
		must(t, err)
		must(t, file.Close())
		must(t, os.WriteFile(target, body, entry.Mode().Perm()))
	}
}

func assertSourceClosure(t *testing.T, source string, identity releasecontract.ServerIdentity, managed, version string) {
	t.Helper()
	resources := map[string]releasecontract.EmbeddedResource{}
	for _, r := range identity.Resources {
		resources[r.URI] = r
	}
	ids := strings.Fields(string(read(t, filepath.Join(source, "adapters/mcp/packaging/testdata/method-ids.txt"))))
	if len(ids) != 92 {
		t.Fatal("independent 92-method oracle changed")
	}
	expected := map[string]bool{}
	for _, id := range ids {
		directory := filepath.Join(source, "bundles/base/skills", id)
		prefix := "skills/" + id
		if _, err := os.Stat(directory); os.IsNotExist(err) {
			directory = filepath.Join(source, "bundles/tech-core/skills", id)
			prefix = "bundles/tech-core/skills/" + id
		}
		must(t, filepath.WalkDir(directory, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, _ := filepath.Rel(directory, path)
			uri := "maestro://methods/" + id + "/" + filepath.ToSlash(rel)
			body := read(t, path)
			r, ok := resources[uri]
			if !ok || r.SHA256 != sum(body) || r.Bytes != int64(len(body)) {
				t.Fatalf("missing/changed public method asset %s", uri)
			}
			expected[uri] = true
			// Managed bundle closure is independently read back from source bytes.
			if !bytes.Equal(body, read(t, filepath.Join(managed, "bundles", version, filepath.FromSlash(prefix), rel))) {
				t.Fatalf("managed method asset differs: %s", path)
			}
			return nil
		}))
	}
	agents := []string{"catalog.json", "activation-policy.json", "pa-expert-registry.json", "case-agent/AGENT.md", "client-account-agent/AGENT.md", "darwin/AGENT.md", "gamma-guardian/AGENT.md", "maestro/AGENT.md", "pa-expert/AGENT.md", "yoda/AGENT.md", "templates/case_agent/AGENT.md", "templates/client_account_agent/AGENT.md", "templates/pa_expert/AGENT.md", "templates/quality_guardian/AGENT.md"}
	for _, name := range agents {
		uri := "maestro://agents/" + name
		body := read(t, filepath.Join(source, "bundles/base/agents", filepath.FromSlash(name)))
		if resources[uri].SHA256 != sum(body) || resources[uri].Bytes != int64(len(body)) {
			t.Fatalf("missing canonical agent asset %s", name)
		}
		expected[uri] = true
		if !bytes.Equal(body, read(t, filepath.Join(managed, "bundles", version, "agents", filepath.FromSlash(name)))) {
			t.Fatal("managed canonical agent differs")
		}
	}
	for uri, path := range map[string]string{"maestro://methods/LICENSE.md": "adapters/mcp/packaging/LICENSE.md", "maestro://methods/pocock-upstream.json": "bundles/tech-core/skills/pocock-upstream.json", "maestro://methods/agent-skill-policy.json": "bundles/base/skills/agent-skill-policy.json"} {
		if resources[uri].SHA256 != sum(read(t, filepath.Join(source, path))) {
			t.Fatalf("public license/policy/provenance mismatch %s", uri)
		}
		expected[uri] = true
	}
	for _, uri := range []string{"maestro://methods/METHODS.md", "maestro://methods/catalog.json"} {
		if _, ok := resources[uri]; !ok {
			t.Fatalf("missing public metadata %s", uri)
		}
		expected[uri] = true
	}
	if len(expected) != len(identity.Resources) {
		t.Fatalf("public inventory has extra/missing resources: expected %d got %d", len(expected), len(identity.Resources))
	}
}
func assertPacket(t *testing.T, root string, identity releasecontract.ServerIdentity) {
	t.Helper()
	var packet struct {
		Mode  string `json:"mode"`
		Files []struct {
			Path, SHA256 string
			Mode         uint32
		}
		Resources []releasecontract.EmbeddedResource `json:"embedded_resources"`
	}
	must(t, json.Unmarshal(read(t, filepath.Join(root, "maestro-packet.json")), &packet))
	if packet.Mode != "mcp_only" {
		t.Fatal("first install silently qualified a native host")
	}
	for _, file := range packet.Files {
		if sum(read(t, filepath.Join(root, filepath.FromSlash(file.Path)))) != file.SHA256 {
			t.Fatalf("packet bytes differ %s", file.Path)
		}
	}
	sort.Slice(packet.Resources, func(i, j int) bool { return packet.Resources[i].URI < packet.Resources[j].URI })
	if !reflect.DeepEqual(packet.Resources, identity.Resources) {
		t.Fatal("packet inventory differs from actual compiled server")
	}
}

func assertBaselineRefuses(t *testing.T, ctx context.Context, root, baseline, data, managed string) {
	t.Helper()
	probe, err := os.MkdirTemp(os.Getenv("GOTMPDIR"), "modern-baseline-probe-")
	must(t, err)
	inputs := map[string]string{}
	for _, directory := range []string{"internal/installtx", "internal/releasecontract", "internal/releaseverify", "internal/rolemigration", "internal/processwait"} {
		entries, err := os.ReadDir(filepath.Join(baseline, directory))
		must(t, err)
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".go") {
				path := filepath.Join(baseline, directory, entry.Name())
				inputs[path] = fileIdentity(t, path)
			}
		}
	}
	for _, name := range []string{"go.mod", "go.sum"} {
		inputs[filepath.Join(baseline, name)] = fileIdentity(t, filepath.Join(baseline, name))
	}
	mod := strings.Replace(string(read(t, filepath.Join(baseline, "go.mod"))), "module github.com/agentic-os-brasil/bcg-brasil-agentic-os", "module github.com/agentic-os-brasil/bcg-brasil-agentic-os/baselineprobe", 1) + "\nrequire github.com/agentic-os-brasil/bcg-brasil-agentic-os v0.0.0\nreplace github.com/agentic-os-brasil/bcg-brasil-agentic-os => " + baseline + "\n"
	must(t, os.WriteFile(filepath.Join(probe, "go.mod"), []byte(mod), 0600))
	must(t, os.WriteFile(filepath.Join(probe, "go.sum"), read(t, filepath.Join(baseline, "go.sum")), 0600))
	helper := `package main
import("encoding/json";"fmt";"os";"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx")
func main(){data,managed:=os.Args[1],os.Args[2];_,readErr:=installtx.ReadStateForManagedRoot(data,managed);body,err:=os.ReadFile(data+"/config/install-state.json");if err!=nil{panic(err)};var state installtx.State;if err=json.Unmarshal(body,&state);err!=nil{panic(err)};writeErr:=installtx.WriteState(data,state);if readErr==nil||writeErr==nil{panic("unchanged baseline accepted modern state")};fmt.Printf("baseline reader refused: %v\nbaseline writer refused: %v\n",readErr,writeErr)}
`
	must(t, os.WriteFile(filepath.Join(probe, "main.go"), []byte(helper), 0600))
	binary := filepath.Join(probe, "baseline-reader-writer")
	command := exec.CommandContext(ctx, "go", "build", "-C", probe, "-mod=readonly", "-buildvcs=false", "-o", binary, ".")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("build unchanged baseline witness: %v: %s", err, output)
	}
	statePath := filepath.Join(data, "config/install-state.json")
	before := sum(read(t, statePath))
	output, err = exec.CommandContext(ctx, binary, data, managed).CombinedOutput()
	if err != nil {
		t.Fatalf("baseline strict witness failed: %v: %s", err, output)
	}
	if sum(read(t, statePath)) != before {
		t.Fatal("baseline writer mutated protected modern state")
	}
	t.Log(string(output))
	for path, hash := range inputs {
		if fileIdentity(t, path) != hash {
			t.Fatalf("baseline input changed %s", path)
		}
	}
	evidence := map[string]any{"baseline_inputs_before_and_after": inputs, "helper_sha256": sum([]byte(helper)), "binary_sha256": sum(read(t, binary)), "state_sha256_before_and_after": before, "output": string(output)}
	body, _ := json.MarshalIndent(evidence, "", "  ")
	must(t, os.WriteFile(filepath.Join(root, "baseline-witness.json"), append(body, '\n'), 0600))
}

func fileIdentity(t *testing.T, path string) string {
	t.Helper()
	info, err := os.Lstat(path)
	must(t, err)
	return info.Mode().String() + ":" + sum(read(t, path))
}

// A fully authenticated legacy request must not choose a probe protocol or
// recovery policy for recognized modern state. Both current-byte cases start
// with genuine canonical activation, admission and complete packets.
func assertLegacyRequestPreservesModernInstall(t *testing.T, ctx context.Context, root string, original setup.Request, config setup.Config, modern releasecontract.Manifest, private ed25519.PrivateKey) {
	t.Helper()
	directory := filepath.Join(root, "legacy-profile-request")
	must(t, os.Mkdir(directory, 0700))
	seedName := "bcgos-bootstrap_0.1.42_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		seedName += ".exe"
	}
	seed := filepath.Join(directory, seedName)
	status, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "bootstrapper_version": "0.1.42", "authority_registry_sha256": sum(read(t, config.AuthorityRegistry))})
	seedSource := filepath.Join(directory, "legacy-seed.go")
	program := fmt.Sprintf(`package main
import ("fmt"; "os")
func main() { if len(os.Args) < 2 { os.Exit(2) }; switch os.Args[1] {
case "seed-status": fmt.Println(%q)
case "version": fmt.Println("bcgos 0.1.42")
case "install": fmt.Fprintln(os.Stderr, "legacy fixture deliberately stops before activation"); os.Exit(3)
default: os.Exit(2)
} }
`, string(status))
	must(t, os.WriteFile(seedSource, []byte(program), 0600))
	if output, err := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-o", seed, seedSource).CombinedOutput(); err != nil {
		t.Fatalf("build actual legacy seed: %v: %s", err, output)
	}
	seedSHA := sum(read(t, seed))
	release := filepath.Join(directory, "signed-release")
	must(t, os.Mkdir(release, 0700))
	legacy := releasecontract.Manifest{SchemaVersion: 1, Product: "maestro", Release: "0.1.42", Channel: "canary", Issuer: modern.Issuer, CLI: releasecontract.CLIComponent{Version: "0.1.42", CompatibleBundle: ">=0.1.42 <0.2.0"}, Bundle: releasecontract.BundleComponent{Version: "0.1.42", CompatibleCLI: ">=0.1.42 <0.2.0"}, Migrations: []releasecontract.Migration{}, ReleaseNotes: modern.ReleaseNotes}
	for _, a := range modern.Artifacts {
		if a.Kind == "bundle" {
			legacy.Artifacts = append(legacy.Artifacts, a)
			body := read(t, filepath.Join(original.ReleaseDirectory, a.Name))
			must(t, os.WriteFile(filepath.Join(release, a.Name), body, 0600))
			must(t, os.WriteFile(filepath.Join(release, a.SignatureRef), ed25519.Sign(private, body), 0600))
		}
	}
	cliName := "bcgos_0.1.42_" + runtime.GOOS + "_" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		cliName += ".exe"
	}
	cli := read(t, seed)
	legacy.Artifacts = append(legacy.Artifacts, releasecontract.Artifact{Kind: "cli", OS: runtime.GOOS, Arch: runtime.GOARCH, Name: cliName, Size: int64(len(cli)), SHA256: sum(cli), SignatureRef: cliName + ".sig"})
	must(t, os.WriteFile(filepath.Join(release, cliName), cli, 0700))
	must(t, os.WriteFile(filepath.Join(release, cliName+".sig"), ed25519.Sign(private, cli), 0600))
	must(t, os.WriteFile(filepath.Join(release, legacy.ReleaseNotes.Name), read(t, filepath.Join(original.ReleaseDirectory, legacy.ReleaseNotes.Name)), 0600))
	body, err := json.Marshal(legacy)
	must(t, err)
	must(t, os.WriteFile(filepath.Join(release, "release-manifest.json"), body, 0600))
	must(t, os.WriteFile(filepath.Join(release, "release-manifest.json.sig"), ed25519.Sign(private, body), 0600))

	for _, current := range []string{"healthy_current", "changed_current"} {
		t.Run("legacy_request_against_modern/"+current, func(t *testing.T) {
			r := original
			r.UserHome = filepath.Join(root, current+"-owner")
			r.ManagedRoot = filepath.Join(root, current+"-managed")
			r.Workspace = filepath.Join(root, current+"-workspace")
			r.CodePacket = filepath.Join(root, current+"-code")
			r.DesktopPacket = filepath.Join(root, current+"-desktop.mcpb")
			must(t, os.Mkdir(r.UserHome, 0700))
			must(t, os.Mkdir(r.Workspace, 0700))
			review, err := setup.ReviewInstallation(r, config)
			must(t, err)
			result, err := setup.Apply(ctx, r, config, review.Digest, true)
			must(t, err)
			if result.State != "ready" || result.WorkspaceID == "" {
				t.Fatal("cross-profile test requires genuine admitted modern installation")
			}
			if current == "changed_current" {
				markerName := "execution-marker"
				if runtime.GOOS == "windows" {
					markerName += ".exe"
				}
				replacement := read(t, filepath.Join(root, markerName))
				original := read(t, result.Installation.CLIPath)
				if len(replacement) >= len(original) {
					t.Fatal("marker executable must fit authenticated installed size")
				}
				replacement = append(replacement, make([]byte, len(original)-len(replacement))...)
				path := result.Installation.CLIPath + ".test-replacement"
				must(t, os.WriteFile(path, replacement, 0700))
				must(t, os.Rename(path, result.Installation.CLIPath))
			}
			options := installer.Options{UserHome: r.UserHome, ReleaseDir: release, Bootstrapper: seed, AuthorityRegistry: config.AuthorityRegistry, ManagedRoot: r.ManagedRoot, DataRoot: review.Installation.DataRoot, VerifyNative: func(_ context.Context, path string) error {
				if sum(read(t, path)) != seedSHA {
					return fmt.Errorf("engineering legacy native fixture changed")
				}
				return nil
			}}
			plan, verified, err := installer.Prepare(options)
			must(t, err)
			if plan.Profile != "" || verified.Manifest.SchemaVersion != 1 {
				t.Fatal("test did not admit a valid signed legacy request")
			}
			before := snapshot(t, root)
			_, err = installer.Install(ctx, options)
			if err == nil || !strings.Contains(err.Error(), "installation profile mismatch") {
				t.Errorf("legacy request did not refuse profile mismatch: %v", err)
			}
			after := snapshot(t, root)
			for path := range after {
				if _, existed := before[path]; !existed && strings.HasSuffix(path, ".executed") {
					t.Errorf("cross-profile probe executed changed server: %s", path)
				}
			}
			if !reflect.DeepEqual(before, after) {
				t.Error("cross-profile request changed owner/install/packet state or quarantined the existing installation")
			}
			t.Logf("admitted legacy request / recognized modern state: %v", err)
		})
	}
}

// The replacement is a real executable with the authenticated file's exact
// size but different bytes. Its independent control invocation proves it leaves
// a marker if either installed-server admission path executes it.
func assertChangedInstalledServerRefused(t *testing.T, ctx context.Context, root string, r setup.Request, config setup.Config, review setup.Review, result setup.Result, state installtx.State) {
	t.Helper()
	source := filepath.Join(root, "execution-marker.go")
	must(t, os.WriteFile(source, []byte(`package main
import "os"
func main() { p, e := os.Executable(); if e != nil { panic(e) }; if e = os.WriteFile(p+".executed", []byte("changed installed executable ran"), 0600); e != nil { panic(e) } }
`), 0600))
	control := filepath.Join(root, "execution-marker")
	if runtime.GOOS == "windows" {
		control += ".exe"
	}
	if output, err := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-o", control, source).CombinedOutput(); err != nil {
		t.Fatalf("build actual marker executable: %v: %s", err, output)
	}
	if output, err := exec.CommandContext(ctx, control, "identity").CombinedOutput(); err != nil {
		t.Fatalf("control marker execution: %v: %s", err, output)
	}
	if string(read(t, control+".executed")) != "changed installed executable ran" {
		t.Fatal("control executable did not establish independent execution marker")
	}
	original := read(t, result.Installation.CLIPath)
	originalInfo, err := os.Lstat(result.Installation.CLIPath)
	must(t, err)
	replaceInstalled := func(body []byte) {
		path := result.Installation.CLIPath + ".test-replacement"
		must(t, os.WriteFile(path, body, originalInfo.Mode().Perm()))
		must(t, os.Rename(path, result.Installation.CLIPath))
	}
	replacement := read(t, control)
	if len(replacement) >= len(original) {
		t.Fatal("marker executable must fit authenticated installed size")
	}
	replacement = append(replacement, make([]byte, len(original)-len(replacement))...)
	marker := result.Installation.CLIPath + ".executed"
	for _, seam := range []string{"canonical_server_check", "existing_install_recovery"} {
		t.Run("changed_installed_server/"+seam, func(t *testing.T) {
			replaceInstalled(replacement)
			defer func() {
				replaceInstalled(original)
				_ = os.Remove(marker)
			}()
			before := snapshot(t, root)
			var err error
			if seam == "canonical_server_check" {
				err = installtx.CheckActivatedServer(r.ManagedRoot, state)
			} else {
				options := installer.Options{UserHome: r.UserHome, ReleaseDir: r.ReleaseDirectory, Bootstrapper: config.Installer, AuthorityRegistry: config.AuthorityRegistry, ManagedRoot: r.ManagedRoot, DataRoot: review.Installation.DataRoot, ExpectedPlanDigest: review.Installation.PlanDigest, VerifyNative: config.VerifyNative, Run: config.Run}
				_, err = installer.Install(ctx, options)
			}
			if err == nil {
				t.Error("changed installed executable was accepted")
			} else if !strings.Contains(err.Error(), "authenticated size and digest") {
				t.Errorf("changed executable did not fail canonical byte authentication: %v", err)
			}
			if _, markerErr := os.Lstat(marker); !os.IsNotExist(markerErr) {
				t.Errorf("changed installed executable ran before authentication: %v", markerErr)
			}
			after := snapshot(t, root)
			for path := range after {
				if _, existed := before[path]; !existed && strings.HasSuffix(path, ".executed") {
					t.Errorf("changed executable left execution marker: %s", path)
				}
			}
			if !reflect.DeepEqual(before, after) {
				t.Error("changed executable refusal changed installation/owner/packet state")
			}
			t.Logf("installed-file refusal: %v", err)
		})
	}
}

// Simulate an independent packet writer after exact confirmation. Canonical
// activation and admission remain coherent; recovery chooses fresh destinations
// and reuses genuine existing state without deleting the independent packet.
func assertSetupPendingRecovery(t *testing.T, ctx context.Context, root string, original setup.Request, config setup.Config, seedSHA string) {
	t.Helper()
	r := original
	r.UserHome = filepath.Join(root, "pending-owner")
	must(t, os.Mkdir(r.UserHome, 0700))
	r.ManagedRoot = filepath.Join(root, "pending-managed")
	r.Workspace = filepath.Join(root, "pending-workspace")
	must(t, os.Mkdir(r.Workspace, 0700))
	r.CodePacket = filepath.Join(root, "pending-code")
	r.DesktopPacket = filepath.Join(root, "pending-desktop.mcpb")
	marker := []byte("independent existing packet writer")
	injected := false
	fault := config
	fault.VerifyNative = func(_ context.Context, path string) error {
		if sum(read(t, path)) != seedSHA {
			return fmt.Errorf("engineering seed changed")
		}
		if filepath.Dir(path) == r.ManagedRoot && !injected {
			injected = true
			return os.WriteFile(r.DesktopPacket, marker, 0600)
		}
		return nil
	}
	review, err := setup.ReviewInstallation(r, fault)
	must(t, err)
	result, err := setup.Apply(ctx, r, fault, review.Digest, true)
	if err == nil || result.State != "activated_setup_pending" || result.Home == "" || result.WorkspaceID == "" {
		t.Fatalf("packet race acknowledged ready or lost activation: %+v %v", result, err)
	}
	if !bytes.Equal(read(t, r.DesktopPacket), marker) {
		t.Fatal("independent packet was replaced")
	}
	state, err := installtx.ReadStateForManagedRoot(review.Installation.DataRoot, r.ManagedRoot)
	must(t, err)
	must(t, installtx.CheckActivatedServer(r.ManagedRoot, state))
	ownerMarker := filepath.Join(result.Home, "owner-preserved.txt")
	must(t, os.WriteFile(ownerMarker, []byte("exact admitted owner bytes"), 0600))
	recovered := r
	recovered.CodePacket = filepath.Join(root, "recovered-code")
	recovered.DesktopPacket = filepath.Join(root, "recovered-desktop.mcpb")
	freshReview, err := setup.ReviewInstallation(recovered, config)
	must(t, err)
	complete, err := setup.Apply(ctx, recovered, config, freshReview.Digest, true)
	must(t, err)
	if complete.State != "ready" || complete.Installation.Disposition != "already_installed" || complete.Home != result.Home || complete.WorkspaceID != result.WorkspaceID {
		t.Fatalf("honest setup recovery failed: %+v", complete)
	}
	if string(read(t, ownerMarker)) != "exact admitted owner bytes" || !bytes.Equal(read(t, r.DesktopPacket), marker) {
		t.Fatal("recovery changed existing owner/packet bytes")
	}
}

func TestDirectInnerInstallRequiresProductionAuthority(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("supported actual native runner required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	root := t.TempDir()
	if retained := os.Getenv("MAESTRO_MODERN_AUTHORITY_ROOT"); retained != "" {
		must(t, os.Mkdir(retained, 0700))
		root = retained
	}
	source, err := filepath.Abs("../../..")
	must(t, err)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	registry, _ := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "engineering-fixture", "key_id": "ephemeral", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": "2020-01-01T00:00:00Z", "valid_until": "2100-01-01T00:00:00Z"}}})
	registryPath := filepath.Join(root, "authority.json")
	must(t, os.WriteFile(registryPath, registry, 0600))
	factory, err := releasepack.BuildModernCandidate(ctx, releasepack.ModernOptions{Root: source, Output: filepath.Join(root, "factory"), Version: "0.1.42", InstallerVersion: "0.3.7", Channel: "canary", AuthorityRegistry: registryPath, Target: releasepack.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}})
	must(t, err)
	release := filepath.Join(root, "signed-release")
	_, err = releasepack.SignCandidate(releasepack.SignCandidateOptions{Candidate: factory.Directory, Output: release, Registry: registryPath, Issuer: "engineering-fixture", KeyID: "ephemeral", PrivateKey: private})
	must(t, err)
	user := filepath.Join(root, "owner")
	must(t, os.Mkdir(user, 0700))
	managed := filepath.Join(root, "managed")
	must(t, os.MkdirAll(filepath.Join(managed, "trust"), 0700))
	self := filepath.Join(managed, "maestro-install")
	if runtime.GOOS == "windows" {
		self += ".exe"
	}
	must(t, os.WriteFile(self, read(t, factory.Installer), 0700))
	must(t, os.WriteFile(filepath.Join(managed, "trust", "release-authority-registry.json"), registry, 0600))
	before := snapshot(t, user)
	command := exec.CommandContext(ctx, self, "install", "--verified-directory", release, "--data-root", filepath.Join(user, ".maestro-install"), "--user-home", user)
	output, err := command.CombinedOutput()
	if err == nil || !reflect.DeepEqual(before, snapshot(t, user)) {
		t.Fatalf("direct production inner command activated without native/review authority: err=%v output=%s", err, output)
	}
	workspace := filepath.Join(root, "workspace")
	must(t, os.Mkdir(workspace, 0700))
	request := setup.Request{ReleaseDirectory: release, ManagedRoot: managed, UserHome: user, Workspace: workspace, CodePacket: filepath.Join(root, "code"), DesktopPacket: filepath.Join(root, "desktop.mcpb")}
	seedSHA := sum(read(t, factory.Installer))
	engineeringNative := func(_ context.Context, path string) error {
		if sum(read(t, path)) != seedSHA {
			return fmt.Errorf("engineering native fixture seed drift")
		}
		return nil
	}
	review, err := setup.ReviewInstallation(request, setup.Config{Installer: factory.Installer, AuthorityRegistry: factory.AuthorityRegistry, VerifyNative: engineeringNative, LookupEnv: func(string) (string, bool) { return "", false }})
	must(t, err)
	body, err := json.Marshal(review.Installation)
	must(t, err)
	// A genuinely authenticated exact reviewed fixture still cannot turn an
	// unsigned renamed process into the independently admitted native seed.
	command = exec.CommandContext(ctx, self, "install", "--confirmed-plan", string(body), "--review-digest", review.Installation.PlanDigest, "--confirm")
	output, err = command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "native installer self-trust") || !reflect.DeepEqual(before, snapshot(t, user)) {
		t.Fatalf("unsigned inner process accepted a valid confirmed fixture: err=%v output=%s", err, output)
	}
	command = exec.CommandContext(ctx, self, "install", "--confirmed-plan", string(body), "--review-digest", review.Installation.PlanDigest)
	output, err = command.CombinedOutput()
	if err == nil || !strings.Contains(string(output), "explicit confirmed canonical install review") || !reflect.DeepEqual(before, snapshot(t, user)) {
		t.Fatalf("inner process accepted absent explicit confirmation: err=%v output=%s", err, output)
	}
	// The same owning API must reject a stale/tampered handoff before effects,
	// even in the controlled engineering native-admission composition.
	changed := review.Installation
	changed.ManagedRoot = filepath.Join(root, "changed-managed")
	_, err = installer.ActivateSeeded(ctx, installer.SeededActivationOptions{Self: self, Version: "0.3.7", AuthorityRegistrySHA256: sum(registry), Plan: changed, ReviewDigest: review.Installation.PlanDigest, Confirmed: true, VerifyNative: engineeringNative})
	if err == nil || !reflect.DeepEqual(before, snapshot(t, user)) {
		t.Fatal("tampered exact inner review reached activation")
	}
}
