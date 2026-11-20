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
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
)

// Exercise actual untagged shipping defaults after controlled fixture setup;
// the experimental lifecycle installation has a separate admission contract.
func TestCurrentClientShippingNativeRefusal(t *testing.T) {
	root := os.Getenv("MAESTRO_CLIENT_REGISTRATION_TEST_ROOT")
	if root == "" {
		t.Skip("explicit actual current-client fixture required")
	}
	var err error
	root, err = os.MkdirTemp(root, "shipping-native-")
	must(t, err)
	provisionShippingNativeFixture(t, root)
	var fixture struct{ Self, ManagedRoot, UserHome, Workspace string }
	must(t, json.Unmarshal(read(t, filepath.Join(root, "native-process.json")), &fixture))
	records, err := filepath.Glob(filepath.Join(fixture.UserHome, ".maestro-install/clients/*.json"))
	must(t, err)
	var record installer.ClientRecord
	selected := 0
	// Match reviewed anchors; materialization paths name versioned slots.
	selection := installer.ClientPacketSelection{CodeRoot: filepath.Join(root, "stable-code"), DesktopPath: filepath.Join(root, "stable-desktop.mcpb")}
	for _, path := range records {
		var candidate installer.ClientRecord
		must(t, json.Unmarshal(read(t, path), &candidate))
		if candidate.Materialization.Selection != nil && *candidate.Materialization.Selection == selection &&
			candidate.Binding.ManagedRoot == fixture.ManagedRoot && candidate.Binding.UserHome == fixture.UserHome && candidate.Binding.Workspace == fixture.Workspace {
			record = candidate
			selected++
		}
	}
	if selected != 1 {
		t.Fatal("one exact primary actual finite current record required")
	}
	client := installer.ClientRequestFromRecord(record)
	client.Reconcile = true
	request := map[string]any{"managed_root": fixture.ManagedRoot, "user_home": fixture.UserHome, "client": client}
	body, _ := json.Marshal(request)
	fixed := []string{"--product-root", fixture.ManagedRoot, "--user-home", fixture.UserHome, "--workspace", fixture.Workspace, "--carrier-id", record.CarrierID}
	cases := [][]string{append([]string{"stable-launch"}, fixed...), {"clients-review"}, {"clients-apply", "--confirm", "--review-digest", strings.Repeat("f", 64)}, {"clients-reconcile", "--confirm", "--review-digest", strings.Repeat("f", 64)}, append([]string{"clients-status"}, fixed...), append([]string{"code-review"}, fixed...)}
	before := snapshot(t, root)
	var results []any
	for _, args := range cases {
		command := exec.Command(fixture.Self, args...)
		command.Stdin = bytes.NewReader(body)
		output, e := command.CombinedOutput()
		var exit *exec.ExitError
		if !errors.As(e, &exit) || exit.ExitCode() != 1 || !bytes.Contains(output, []byte("native installer self-trust")) {
			t.Fatalf("shipping %v did not preserve actual native-default refusal: %s %v", args, output, e)
		}
		results = append(results, map[string]any{"argv": append([]string{fixture.Self}, args...), "exit_code": exit.ExitCode(), "output": string(output)})
	}
	if !reflect.DeepEqual(before, snapshot(t, root)) {
		t.Fatal("shipping native refusals changed retained installation/owner/workspace")
	}
	proof, _ := json.MarshalIndent(map[string]any{"actual_shipping_installer_sha256": sum(read(t, fixture.Self)), "cases": results, "complete_fixture_bytes_modes_unchanged": true, "native_override": false}, "", "  ")
	must(t, os.WriteFile(root+".shipping-native-negative.json", append(proof, '\n'), 0600))
	t.Logf("actual untagged shipping refusal proof: %s", root+".shipping-native-negative.json")
}

// Reuse the ordinary factory/source/install owners; the verifier is confined to
// setup and is never supplied to the six compiled shipping commands above.
func provisionShippingNativeFixture(t *testing.T, root string) {
	t.Helper()
	ctx := t.Context()
	source, err := filepath.Abs("../../..")
	must(t, err)
	now := time.Now().UTC().Truncate(time.Second)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	must(t, err)
	registryBody, err := json.Marshal(map[string]any{"schema_version": 1, "product": "maestro", "authorities": []any{map[string]any{"issuer": "engineering-fixture", "key_id": "ephemeral", "algorithm": "ed25519", "public_key": base64.StdEncoding.EncodeToString(public), "status": "active", "valid_from": now.Add(-time.Hour).Format(time.RFC3339), "valid_until": now.Add(13 * 24 * time.Hour).Format(time.RFC3339)}}})
	must(t, err)
	registryPath := filepath.Join(root, "authority.json")
	must(t, os.WriteFile(registryPath, registryBody, 0600))
	factory, err := releasepack.BuildModernCandidate(ctx, releasepack.ModernOptions{Root: source, Output: filepath.Join(root, "factory"), Version: "0.1.42", InstallerVersion: "0.3.7", Channel: "canary", AuthorityRegistry: registryPath, Target: releasepack.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}})
	must(t, err)
	release := filepath.Join(root, "signed-release")
	_, err = releasepack.SignCandidate(releasepack.SignCandidateOptions{Candidate: factory.Directory, Output: release, Registry: registryPath, Issuer: "engineering-fixture", KeyID: "ephemeral", PrivateKey: private})
	must(t, err)
	registry, err := releaseverify.LoadAuthorityRegistry(registryPath, time.Now)
	must(t, err)
	verified, err := releaseverify.VerifyDirectory(release, registry)
	must(t, err)
	seedSHA := sum(read(t, factory.Installer))
	config := setup.Config{Installer: factory.Installer, AuthorityRegistry: factory.AuthorityRegistry, LookupEnv: func(string) (string, bool) { return "", false }}
	config.VerifyNative = func(_ context.Context, path string) error {
		body, err := os.ReadFile(path)
		if err != nil {
			return err
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
			if err := flags.Parse(args[1:]); err != nil {
				return nil, err
			}
			var plan installer.Plan
			if err := releasecontract.DecodeStrictJSON([]byte(*body), &plan); err != nil {
				return nil, err
			}
			state, err := installer.ActivateSeeded(ctx, installer.SeededActivationOptions{Self: self, Version: "0.3.7", AuthorityRegistrySHA256: sum(registryBody), Plan: plan, ReviewDigest: *digest, Confirmed: *confirm, VerifyNative: config.VerifyNative})
			if err != nil {
				return nil, err
			}
			return json.Marshal(map[string]string{"state": "activated", "profile": state.Profile})
		}
		return exec.CommandContext(ctx, self, args...).CombinedOutput()
	}
	user, workspace := filepath.Join(root, "owner"), filepath.Join(root, "workspace")
	must(t, os.Mkdir(user, 0700))
	must(t, os.Mkdir(workspace, 0700))
	must(t, os.WriteFile(filepath.Join(workspace, "owner.txt"), []byte("retain exact owner workspace bytes"), 0600))
	request := setup.Request{ReleaseDirectory: release, ManagedRoot: filepath.Join(root, "managed"), UserHome: user, Workspace: workspace, CodePacket: filepath.Join(root, "initial-code"), DesktopPacket: filepath.Join(root, "initial-desktop.mcpb")}
	review, err := setup.ReviewInstallation(request, config)
	must(t, err)
	_, err = setup.Apply(ctx, request, config, review.Digest, true)
	must(t, err)
	seed := installer.SeededOperationOptions{Self: filepath.Join(request.ManagedRoot, sourceTestExecutable("maestro-install")), Version: "0.3.7", AuthorityRegistrySHA256: sum(registryBody), ManagedRoot: request.ManagedRoot, UserHome: user, DataRoot: filepath.Join(user, ".maestro-install"), VerifyNative: config.VerifyNative}
	tuple := releasecontract.SourceTuple{SourceID: "shipping-client-team", Channel: "canary", OS: runtime.GOOS, Arch: runtime.GOARCH}
	syncRoot := filepath.Join(root, "synced")
	_, err = releasepack.PublishGeneration(releasepack.PublishGenerationOptions{Root: syncRoot, ReleaseDirectory: release, Registry: registryPath, PrivateKey: private, Generation: releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: verified.Manifest.Issuer, Source: tuple, Sequence: 1, PublishedAt: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(7 * 24 * time.Hour).Format(time.RFC3339), Release: verified.Manifest.Release, ManifestSHA256: verified.ManifestSHA256, ClientInputsSHA256: verified.Manifest.Delivery.ClientInputsSHA256, Recalls: []releasecontract.ReleaseRecall{}, MinimumReaderContract: releasecontract.SyncedSourceContract}})
	must(t, err)
	sourceOptions := installer.SeededSourceOptions{SeededOperationOptions: seed, SourceRoot: syncRoot, ExpectedSource: tuple}
	sourceReview, err := installer.ReviewSourceSeeded(ctx, sourceOptions)
	must(t, err)
	sourceOptions.ReviewDigest = sourceReview.Digest
	_, err = installer.CheckSourceSeeded(ctx, sourceOptions)
	if !errors.Is(err, updateservice.ErrCurrent) {
		t.Fatalf("active signed shipping source enrollment: %v", err)
	}
	carrier := make([]byte, 16)
	_, err = rand.Read(carrier)
	must(t, err)
	options := installer.ClientOptions{SeededOperationOptions: seed, Request: installer.ClientRequest{CarrierID: fmt.Sprintf("%x", carrier), Workspace: workspace, CodeRoot: filepath.Join(root, "stable-code"), DesktopPath: filepath.Join(root, "stable-desktop.mcpb"), CodeScope: "user", VersionedSlots: true}, LookupEnv: config.LookupEnv}
	clientReview, err := installer.ReviewClientsSeeded(ctx, options)
	must(t, err)
	record, err := setup.ApplyClients(ctx, options, clientReview.Digest, true)
	must(t, err)
	options.Request = installer.ClientRequestFromRecord(record)
	status, err := installer.ReadClientsSeeded(ctx, options)
	must(t, err)
	if status.State != "current_materialization" || status.Record == nil || !reflect.DeepEqual(*status.Record, record) {
		t.Fatal("ordinary shipping fixture did not retain its exact active versioned client")
	}
	fixtureBody, err := json.Marshal(map[string]string{"Self": seed.Self, "ManagedRoot": seed.ManagedRoot, "UserHome": user, "Workspace": workspace})
	must(t, err)
	must(t, os.WriteFile(filepath.Join(root, "native-process.json"), fixtureBody, 0600))
	setupProof, err := json.MarshalIndent(map[string]any{"fixture_setup": "controlled_in_process_exact_seed_byte_verifier_only", "shipping_entry": "ordinary_untagged", "shipping_native_override": false, "installer_sha256": seedSHA, "active_source_sequence": 1, "manifest_sha256": verified.ManifestSHA256, "client_record": record, "actual_vendor_client_loading": "unattested"}, "", "  ")
	must(t, err)
	must(t, os.WriteFile(filepath.Join(root, "shipping-fixture-setup.json"), append(setupProof, '\n'), 0600))
}
