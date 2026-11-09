package setup_test

import (
	"bufio"
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

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/releasepack"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/updateservice"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestModernClientRegistrationLifecycle(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("actual admitted-home engineering runner required")
	}
	if os.Getenv("MAESTRO_CLIENT_REGISTRATION_OLD_SERVER") == "" && os.Getenv("MAESTRO_CLIENT_REGISTRATION_BASELINE") == "" {
		t.Skip("explicit immutable baseline or sealed old actual server required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Minute)
	defer cancel()
	root := t.TempDir()
	if retained := os.Getenv("MAESTRO_CLIENT_REGISTRATION_TEST_ROOT"); retained != "" {
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
	type built struct {
		factory   releasepack.ModernCandidate
		verified  releaseverify.VerifiedRelease
		directory string
	}
	build := func(version, sourceRoot string) built {
		factory, e := releasepack.BuildModernCandidate(ctx, releasepack.ModernOptions{Root: sourceRoot, Output: filepath.Join(root, "factory-"+version), Version: version, InstallerVersion: "0.3.7", Channel: "canary", AuthorityRegistry: registryPath, Target: releasepack.Target{OS: runtime.GOOS, Arch: runtime.GOARCH}})
		must(t, e)
		directory := filepath.Join(root, "signed-"+version)
		_, e = releasepack.SignCandidate(releasepack.SignCandidateOptions{Candidate: factory.Directory, Output: directory, Registry: registryPath, Issuer: "engineering-fixture", KeyID: "ephemeral", PrivateKey: private})
		must(t, e)
		verified, e := releaseverify.VerifyDirectory(directory, registry)
		must(t, e)
		return built{factory, verified, directory}
	}
	n := build("0.1.42", source)
	futureSource := clientFutureSource(t, ctx, source, root)
	next := build("0.1.43", futureSource)
	oldPath := os.Getenv("MAESTRO_CLIENT_REGISTRATION_OLD_SERVER")
	oldCopy := filepath.Join(root, "old-actual-server")
	if oldPath != "" {
		info, e := os.Stat(oldPath)
		must(t, e)
		must(t, os.WriteFile(oldCopy, read(t, oldPath), info.Mode().Perm()))
	} else {
		// The archived executable requires the real reviewed release tuple at link time.
		// Its historical code and missing client capability remain unchanged.
		ldflags := fmt.Sprintf("-X main.Version=%s -X 'main.CompatibleBundle=%s' -X main.AuthorityRegistrySHA256=%s", n.verified.Manifest.Server.Version, n.verified.Manifest.Server.CompatibleBundle, sum(registryBody))
		command := exec.CommandContext(ctx, "go", "build", "-mod=readonly", "-buildvcs=false", "-ldflags", ldflags, "-o", oldCopy, "./cmd/maestro-mcp")
		command.Dir = filepath.Join(os.Getenv("MAESTRO_CLIENT_REGISTRATION_BASELINE"), "adapters/mcp")
		if output, e := command.CombinedOutput(); e != nil {
			t.Fatalf("read-only archived baseline build: %s %v", output, e)
		}
	}
	oldSHA := sum(read(t, oldCopy))
	oldIdentity, e := releasecontract.ReadServerIdentity(oldCopy)
	must(t, e)
	if oldIdentity.SchemaVersion != 2 || oldIdentity.ExecutionContract != releasecontract.SyncedSourceContract {
		t.Fatal("old actual source-aware server identity did not execute")
	}
	oldCapabilityOutput, oldCapabilityError := exec.CommandContext(ctx, oldCopy, "client-capabilities").CombinedOutput()
	var oldCapabilityExit *exec.ExitError
	if !errors.As(oldCapabilityError, &oldCapabilityExit) || oldCapabilityExit.ExitCode() != 1 || !bytes.Contains(oldCapabilityOutput, []byte("operation unavailable")) {
		t.Fatalf("actual old server capability refusal not observed: %s %v", oldCapabilityOutput, oldCapabilityError)
	}
	oldCapabilityProof, _ := json.MarshalIndent(map[string]any{"old_server_sha256": oldSHA, "actual_identity": oldIdentity, "capability_exit_code": oldCapabilityExit.ExitCode(), "capability_stderr": string(oldCapabilityOutput)}, "", "  ")
	must(t, os.WriteFile(filepath.Join(root, "old-capability-proof.json"), append(oldCapabilityProof, '\n'), 0600))
	process := filepath.Join(root, "engineering-client-process.test")
	compile := exec.CommandContext(ctx, "go", "test", "-mod=readonly", "-c", "-o", process, "./cmd/maestro-install")
	compile.Dir = filepath.Join(source, "adapters/mcp")
	if output, e := compile.CombinedOutput(); e != nil {
		t.Fatalf("compile explicit engineering native process composition: %s %v", output, e)
	}
	seedSHA := sum(read(t, n.factory.Installer))
	if bytes.Contains(read(t, n.factory.Installer), []byte(currentMethodMarker)) {
		t.Fatal("old admitted installer unexpectedly contains future method marker")
	}
	c := setup.Config{Installer: n.factory.Installer, AuthorityRegistry: n.factory.AuthorityRegistry, LookupEnv: func(string) (string, bool) { return "", false }}
	c.VerifyNative = func(_ context.Context, path string) error {
		if sum(read(t, path)) != seedSHA {
			return errors.New("engineering native seed byte identity changed")
		}
		return nil
	}
	c.Run = func(ctx context.Context, self string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "install" {
			f := flag.NewFlagSet("engineering-native-seed", flag.ContinueOnError)
			body := f.String("confirmed-plan", "", "")
			digest := f.String("review-digest", "", "")
			confirmed := f.Bool("confirm", false, "")
			if e := f.Parse(args[1:]); e != nil {
				return nil, e
			}
			var p installer.Plan
			if e := releasecontract.DecodeStrictJSON([]byte(*body), &p); e != nil {
				return nil, e
			}
			state, e := installer.ActivateSeeded(ctx, installer.SeededActivationOptions{Self: self, Version: "0.3.7", AuthorityRegistrySHA256: sum(registryBody), Plan: p, ReviewDigest: *digest, Confirmed: *confirmed, VerifyNative: c.VerifyNative})
			if e != nil {
				return nil, e
			}
			return json.Marshal(map[string]string{"state": "activated", "profile": state.Profile})
		}
		return exec.CommandContext(ctx, self, args...).CombinedOutput()
	}
	user, ws := filepath.Join(root, "owner"), filepath.Join(root, "workspace")
	must(t, os.Mkdir(user, 0700))
	must(t, os.Mkdir(ws, 0700))
	must(t, os.WriteFile(filepath.Join(ws, "result.md"), []byte("literal initial result\n"), 0600))
	request := setup.Request{ReleaseDirectory: n.directory, ManagedRoot: filepath.Join(root, "managed"), UserHome: user, Workspace: ws, CodePacket: filepath.Join(root, "legacy-code"), DesktopPacket: filepath.Join(root, "legacy-desktop.mcpb")}
	review, err := setup.ReviewInstallation(request, c)
	must(t, err)
	installed, err := setup.Apply(ctx, request, c, review.Digest, true)
	must(t, err)
	seed := installer.SeededOperationOptions{Self: filepath.Join(request.ManagedRoot, "maestro-install"), Version: "0.3.7", AuthorityRegistrySHA256: sum(registryBody), ManagedRoot: request.ManagedRoot, UserHome: user, DataRoot: filepath.Join(user, ".maestro-install"), VerifyNative: c.VerifyNative}
	tuple := releasecontract.SourceTuple{SourceID: "client-team", Channel: "canary", OS: runtime.GOOS, Arch: runtime.GOARCH}
	syncRoot := filepath.Join(root, "synced")
	now := time.Now().UTC().Truncate(time.Second)
	publish := func(sequence uint64, release built, recalls []releasecontract.ReleaseRecall) {
		_, e := releasepack.PublishGeneration(releasepack.PublishGenerationOptions{Root: syncRoot, ReleaseDirectory: release.directory, Registry: registryPath, PrivateKey: private, Generation: releasecontract.Generation{SchemaVersion: 1, Product: "maestro", Protocol: releasecontract.GenerationProtocol, Issuer: release.verified.Manifest.Issuer, Source: tuple, Sequence: sequence, PublishedAt: now.Add(-time.Minute).Format(time.RFC3339), ExpiresAt: now.Add(7 * 24 * time.Hour).Format(time.RFC3339), Release: release.verified.Manifest.Release, ManifestSHA256: release.verified.ManifestSHA256, ClientInputsSHA256: release.verified.Manifest.Delivery.ClientInputsSHA256, Recalls: recalls, MinimumReaderContract: releasecontract.SyncedSourceContract}})
		must(t, e)
	}
	check := func() (updateservice.CheckResult, error) {
		o := installer.SeededSourceOptions{SeededOperationOptions: seed, SourceRoot: syncRoot, ExpectedSource: tuple}
		r, e := installer.ReviewSourceSeeded(ctx, o)
		must(t, e)
		o.ReviewDigest = r.Digest
		return installer.CheckSourceSeeded(ctx, o)
	}
	publish(1, n, []releasecontract.ReleaseRecall{})
	fixture := filepath.Join(root, "native-process.json")
	fixtureBody, _ := json.Marshal(map[string]string{"Self": seed.Self, "Version": seed.Version, "Pin": seed.AuthorityRegistrySHA256, "SeedSHA": seedSHA, "ManagedRoot": seed.ManagedRoot, "UserHome": user, "Workspace": ws})
	must(t, os.WriteFile(fixture, fixtureBody, 0600))
	t.Setenv("MAESTRO_CLIENT_PROCESS_FIXTURE", fixture)
	codeState := filepath.Join(root, "controlled-code-state.json")
	codeRoot := filepath.Join(root, "stable-code")
	desktop := filepath.Join(root, "stable-desktop.mcpb")
	initialCode, _ := json.Marshal(map[string]any{"Root": codeRoot, "Scope": "user", "Enabled": false})
	must(t, os.WriteFile(codeState, initialCode, 0600))
	t.Setenv("MAESTRO_CODE_PROCESS_STATE", codeState)
	// Actual console first run: no JSON hand authoring, no invented carrier ID.
	// Declining source review leaves private custody and client files unchanged.
	beforeDecline := snapshot(t, root)
	decline := exec.CommandContext(ctx, process, "test-client-manage")
	decline.Stdin = strings.NewReader(syncRoot + "\nclient-team\n\n")
	declineOutput, declineErr := decline.CombinedOutput()
	if declineErr == nil || !reflect.DeepEqual(beforeDecline, snapshot(t, root)) {
		t.Fatalf("first-run decline mutated state: %s %v", declineOutput, declineErr)
	}
	first := exec.CommandContext(ctx, process, "test-client-manage")
	first.Stdin = strings.NewReader(syncRoot + "\nclient-team\nYES\n" + codeRoot + "\n" + desktop + "\n" + process + "\nuser\nYES\nYES\n")
	firstOutput, err := first.CombinedOutput()
	must(t, os.WriteFile(filepath.Join(root, "first-run-output.txt"), firstOutput, 0600))
	if err != nil {
		t.Fatalf("actual first-run console/native engineering process: %s %v", firstOutput, err)
	}
	records, err := os.ReadDir(filepath.Join(seed.DataRoot, "clients"))
	must(t, err)
	if len(records) != 1 {
		t.Fatalf("expected one finite record, got %v", records)
	}
	id := strings.TrimSuffix(records[0].Name(), ".json")
	options := installer.ClientOptions{SeededOperationOptions: seed, LookupEnv: c.LookupEnv, Request: installer.ClientRequest{CarrierID: id, Workspace: ws, CodeRoot: codeRoot, DesktopPath: desktop, CodeExecutable: process, CodeScope: "user"}}
	status, err := installer.ReadClientsSeeded(ctx, options)
	must(t, err)
	if status.State != "current_materialization" || status.Record.Code.State != "registered" || status.Record.Desktop.InstallConfirmation != "awaiting_user_install" {
		t.Fatalf("first registered materialization incomplete: %+v", status)
	}
	record := *status.Record
	alternateWorkspace := filepath.Join(root, "alternate-registered-workspace")
	must(t, os.Mkdir(alternateWorkspace, 0700))
	alternateRegistration, e := workspace.Initialize(workspace.Options{WorkspacePath: alternateWorkspace, DataRoot: filepath.Join(installed.Home, "state")})
	must(t, e)
	if alternateRegistration.WorkspaceID == record.Binding.WorkspaceID {
		t.Fatal("alternate workspace did not register a distinct identity")
	}

	// Optional Code setup must finish on this carrier after a later restart.
	pendingRoot, pendingDesktop := filepath.Join(root, "pending-code"), filepath.Join(root, "pending-desktop.mcpb")
	pendingCodeState := filepath.Join(root, "pending-controlled-code-state.json")
	pendingCodeBody, _ := json.Marshal(map[string]any{"Root": pendingRoot, "Scope": "user", "Enabled": false})
	must(t, os.WriteFile(pendingCodeState, pendingCodeBody, 0600))
	pendingFirst := exec.CommandContext(ctx, process, "test-client-manage")
	pendingFirst.Stdin = strings.NewReader(pendingRoot + "\n" + pendingDesktop + "\n\nuser\nYES\n")
	pendingOutput, pendingErr := pendingFirst.CombinedOutput()
	must(t, os.WriteFile(filepath.Join(root, "pending-first-console.txt"), pendingOutput, 0600))
	if pendingErr != nil {
		t.Fatalf("optional Code first setup: %s %v", pendingOutput, pendingErr)
	}
	pendingRecords, e := os.ReadDir(filepath.Join(seed.DataRoot, "clients"))
	must(t, e)
	pendingID := ""
	for _, entry := range pendingRecords {
		if entry.Name() != id+".json" {
			pendingID = strings.TrimSuffix(entry.Name(), ".json")
		}
	}
	if len(pendingRecords) != 2 || pendingID == "" {
		t.Fatal("optional setup did not create exactly one additional carrier")
	}
	pendingOptions := installer.ClientOptions{SeededOperationOptions: seed, LookupEnv: c.LookupEnv, Request: installer.ClientRequest{CarrierID: pendingID, Workspace: ws, CodeRoot: pendingRoot, DesktopPath: pendingDesktop, CodeScope: "user"}}
	pendingStatus, e := installer.ReadClientsSeeded(ctx, pendingOptions)
	must(t, e)
	if pendingStatus.Record.Code.Executable != "" {
		t.Fatal("optional executable was silently selected")
	}
	t.Run("DeferredCodeSelectionSameCarrier", func(t *testing.T) {
		beforeSelection := snapshot(t, root)
		declined := exec.CommandContext(ctx, process, "test-client-manage", pendingID)
		declined.Env = append(os.Environ(), "MAESTRO_CODE_PROCESS_STATE="+pendingCodeState)
		declined.Stdin = strings.NewReader("register-code\n" + process + "\n\n")
		if output, e := declined.CombinedOutput(); e == nil || !reflect.DeepEqual(beforeSelection, snapshot(t, root)) {
			t.Fatalf("declined later selection changed state: %s %v", output, e)
		}
		later := exec.CommandContext(ctx, process, "test-client-manage", pendingID)
		later.Env = append(os.Environ(), "MAESTRO_CODE_PROCESS_STATE="+pendingCodeState)
		later.Stdin = strings.NewReader("register-code\n" + process + "\nYES\nYES\n")
		output, e := later.CombinedOutput()
		if e == nil || !strings.Contains(string(output), "client_tree_replacement_custody") {
			t.Fatalf("deferred Code selection did not retain current tree: %s %v", output, e)
		}
		must(t, os.Rename(pendingRoot, pendingRoot+".previous-"+pendingStatus.Record.Materialization.CodeTreeSHA256))
		later = exec.CommandContext(ctx, process, "test-client-manage", pendingID)
		later.Env = append(os.Environ(), "MAESTRO_CODE_PROCESS_STATE="+pendingCodeState)
		later.Stdin = strings.NewReader("register-code\n" + process + "\nYES\nYES\n")
		output, e = later.CombinedOutput()
		must(t, os.WriteFile(filepath.Join(root, "pending-later-console.txt"), output, 0600))
		if e != nil {
			t.Fatalf("later exact reviewed executable selection: %s %v", output, e)
		}
		selected, e := installer.ReadClientsSeeded(ctx, pendingOptions)
		must(t, e)
		if selected.Record.CarrierID != pendingID || selected.Record.Code.Executable != process || selected.Record.Code.State != "registered" {
			t.Fatalf("same pending carrier not registered: %+v", selected)
		}
		changedSelection := pendingOptions
		changedSelection.Request.CodeExecutable = seed.Self
		if _, e = installer.ReviewClientsSeeded(ctx, changedSelection); e == nil {
			t.Fatal("already selected executable silently replaceable")
		}
		selectionProof, _ := json.MarshalIndent(map[string]any{"carrier_id": pendingID, "initial_executable_unselected": true, "declined_selection_preserved_bytes_modes": true, "later_exact_reviewed_executable": process, "registered_same_carrier": true, "selected_executable_replacement_refused": true}, "", "  ")
		must(t, os.WriteFile(filepath.Join(root, "deferred-code-selection-proof.json"), append(selectionProof, '\n'), 0600))

	})
	desktopSHA := record.Materialization.DesktopSHA256
	recordPath := filepath.Join(seed.DataRoot, "clients", id+".json")
	production := exec.CommandContext(ctx, seed.Self, "stable-launch", "--product-root", seed.ManagedRoot, "--user-home", user, "--workspace", ws, "--carrier-id", id)
	if output, e := production.CombinedOutput(); e == nil {
		t.Fatalf("shipping unsigned native installer bypassed trust: %s", output)
	}
	// Status is a no-write reader, including unsupported/hardlinked private state.
	beforeStatus := snapshot(t, root)
	_, err = installer.ReadClientsSeeded(ctx, options)
	must(t, err)
	if !reflect.DeepEqual(beforeStatus, snapshot(t, root)) {
		t.Fatal("status wrote state")
	}
	linked := filepath.Join(root, "record-hardlink")
	must(t, os.Link(recordPath, linked))
	if _, e := installer.ReadClientsSeeded(ctx, options); e == nil {
		t.Fatal("multiply-linked private carrier record admitted")
	}
	must(t, os.Remove(linked))
	connectStable := func() *mcp.ClientSession {
		cmd := exec.CommandContext(ctx, process, "test-client-launch", id)
		client := mcp.NewClient(&mcp.Implementation{Name: "engineering-stable-client-witness", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
		}})
		cs, e := client.Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, &mcp.ClientSessionOptions{ProtocolVersion: "2025-11-25"})
		must(t, e)
		return cs
	}
	live := connectStable()
	defer live.Close()
	created := call(t, ctx, live, "maestro_apply", map[string]any{"operation": "execution.create", "objective": "stable literal continuation", "next_step": "recover exact next step", "artifacts": []string{"result.md"}}, false)
	itemID := created["item_id"].(string)
	captured := call(t, ctx, live, "maestro_apply", map[string]any{"operation": "memory.capture", "text": "CLIENT_STABLE_LITERAL_MEMORY"}, false)
	captureBody, _ := json.Marshal(captured)
	must(t, os.WriteFile(filepath.Join(root, "capture-result.json"), captureBody, 0600))
	// Authored workspace knowledge is an independent ordinary source. SDK reads
	// the named page across restart; no converter availability is fabricated.
	must(t, os.MkdirAll(filepath.Join(ws, "brain", "knowledge"), 0700))
	must(t, os.WriteFile(filepath.Join(ws, "brain", "knowledge", "client-proof.md"), []byte("# Client proof\nCLIENT_STABLE_LITERAL_KNOWLEDGE\n"), 0600))
	knowledge := call(t, ctx, live, "maestro_query", map[string]any{"operation": "knowledge.collect", "purpose": "verify named authored workspace knowledge", "pages": []string{"knowledge/client-proof.md"}}, false)
	knowledgeBody, _ := json.Marshal(knowledge)
	if !bytes.Contains(knowledgeBody, []byte("CLIENT_STABLE_LITERAL_KNOWLEDGE")) {
		t.Fatalf("initial authored knowledge not collected: %s", knowledgeBody)
	}
	// Passive lifecycle selection is explicit and separate from native loading.
	beforePassive := snapshot(t, root)
	if _, e = installer.PrepareStableLifecycleHook(ctx, options, "SessionStart"); e == nil {
		t.Fatal("plain carrier acquired passive lifecycle authority")
	}
	if !reflect.DeepEqual(beforePassive, snapshot(t, root)) {
		t.Fatal("unselected passive hook changed state")
	}
	passiveID := "edededededededededededededededed"
	passiveOptions := installer.ClientOptions{SeededOperationOptions: seed, LookupEnv: c.LookupEnv, Request: installer.ClientRequest{CarrierID: passiveID, Workspace: ws, CodeRoot: filepath.Join(root, "passive-code"), DesktopPath: filepath.Join(root, "passive-desktop.mcpb"), CodeScope: "user", LifecycleContext: true}}
	passiveReview, e := installer.ReviewClientsSeeded(ctx, passiveOptions)
	must(t, e)
	beforePassive = snapshot(t, root)
	if _, e = setup.ApplyClients(ctx, passiveOptions, passiveReview.Digest, false); e == nil || !reflect.DeepEqual(beforePassive, snapshot(t, root)) {
		t.Fatal("declined passive materialization changed state")
	}
	changedPassive := passiveOptions
	changedPassive.Request.LifecycleContext = false
	if _, e = setup.ApplyClients(ctx, changedPassive, passiveReview.Digest, true); e == nil || !reflect.DeepEqual(beforePassive, snapshot(t, root)) {
		t.Fatal("changed passive opt-in reused reviewed digest")
	}
	passiveRecord, e := setup.ApplyClients(ctx, passiveOptions, passiveReview.Digest, true)
	must(t, e)
	if !passiveRecord.Materialization.LifecycleContext {
		t.Fatal("reviewed passive selection was not recorded")
	}
	if _, e = installer.PrepareStableLifecycleHook(ctx, passiveOptions, "PreToolUse"); e == nil {
		t.Fatal("passive stable launcher accepted a native enforcement event")
	}
	// Consolidation remains an explicit operation; lifecycle never captures or dreams.
	call(t, ctx, live, "maestro_apply", map[string]any{"operation": "memory.consolidate", "cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")}, false)
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "Stop"} {
		launch, e := installer.PrepareStableLifecycleHook(ctx, passiveOptions, event)
		must(t, e)
		if launch.Executable != filepath.Join(seed.ManagedRoot, "bin", "maestro-mcp") || len(launch.Args) < 3 || launch.Args[0] != "lifecycle-hook" || launch.Args[1] != "--event" || launch.Args[2] != event {
			t.Fatalf("stable hook bypassed canonical current launcher: %+v", launch)
		}
		body, _ := json.Marshal(map[string]any{"session_id": "stable-passive-proof", "prompt_id": "stable-passive-turn", "cwd": ws, "hook_event_name": event, "prompt": "PRIVATE_PASSIVE_PROMPT", "last_assistant_message": "PRIVATE_PASSIVE_BODY"})
		beforeHook := snapshot(t, installed.Home)
		beforeWorkspace := snapshot(t, ws)
		hook := exec.CommandContext(ctx, launch.Executable, launch.Args...)
		hook.Stdin = bytes.NewReader(body)
		output, e := hook.CombinedOutput()
		if e != nil {
			t.Fatalf("actual current stable hook %s: %s %v", event, output, e)
		}
		if bytes.Contains(output, []byte("PRIVATE_PASSIVE")) {
			t.Fatal("passive hook leaked host bodies")
		}
		if event == "SessionStart" && !bytes.Contains(output, []byte("CLIENT_STABLE_LITERAL_MEMORY")) {
			t.Fatalf("stable startup omitted committed memory: %s", output)
		}
		if event != "SessionStart" && bytes.Contains(output, []byte("CLIENT_STABLE_LITERAL_MEMORY")) {
			t.Fatal("prompt/Stop repeated generated memory")
		}
		if event != "Stop" && !reflect.DeepEqual(beforeHook, snapshot(t, installed.Home)) {
			t.Fatal("passive context changed owner state")
		}
		if !reflect.DeepEqual(beforeWorkspace, snapshot(t, ws)) {
			t.Fatal("passive hook changed authored workspace")
		}
	}
	passiveReceipt, e := claudeadapter.Receipt(lifecycle.StopFinalize, claudeadapter.NativeInput{SessionID: "stable-passive-proof"})
	must(t, e)
	passiveReceipt.IdempotencyKey = lifecycle.IdempotencyKey(passiveReceipt.IdempotencyKey, "stable-passive-turn")
	receiptBody := read(t, filepath.Join(installed.Home, "state", "runtime", "receipts", passiveRecord.Binding.WorkspaceID, passiveReceipt.IdempotencyKey+".json"))
	var observedStop lifecycle.Receipt
	must(t, json.Unmarshal(receiptBody, &observedStop))
	if observedStop.Event != lifecycle.StopFinalize || observedStop.Provenance != lifecycle.AdapterCommand || observedStop.State != "observed" || bytes.Contains(receiptBody, []byte("PRIVATE_PASSIVE")) || bytes.Contains(receiptBody, []byte("stable-passive-proof")) {
		t.Fatalf("actual stable Stop did not retain body-free scoped observation: %s", receiptBody)
	}

	ownerBefore, workspaceBefore := snapshot(t, installed.Home), snapshot(t, ws)
	beforeRefresh := read(t, recordPath)
	publish(2, n, []releasecontract.ReleaseRecall{})
	_, err = check()
	if !errors.Is(err, updateservice.ErrCurrent) {
		t.Fatal(err)
	}
	status, err = installer.ReadClientsSeeded(ctx, options)
	must(t, err)
	if status.State != "current_materialization" || !bytes.Equal(beforeRefresh, read(t, recordPath)) {
		t.Fatal("metadata-only repeated release invalidated/rewrote materialization")
	}
	// Preserve a deliberate disabled native registration across supported update.
	output, err := exec.CommandContext(ctx, process, "plugin", "disable", record.Code.PluginID, "--scope", "user", "--json").CombinedOutput()
	if err != nil {
		t.Fatalf("controlled disable: %s %v", output, err)
	}
	publish(3, next, []releasecontract.ReleaseRecall{})
	checked, err := check()
	must(t, err)
	pending, err := updateservice.StagePending(seed.DataRoot, checked.Current, checked, checked.Current.Source.ObservedAt)
	must(t, err)
	_, err = installer.UpdateSeeded(ctx, installer.SeededUpdateOptions{SeededOperationOptions: seed, PlanID: pending.Plan.ID})
	must(t, err)
	if _, e := installer.PrepareStableLaunch(ctx, options); e == nil {
		t.Fatal("N current record acquired N+1 without regeneration")
	}
	if _, e = installer.PrepareStableLifecycleHook(ctx, passiveOptions, "SessionStart"); e == nil {
		t.Fatal("stale passive carrier acquired N+1 without current materialization")
	}

	if _, e := installer.PrepareQualifiedLaunch(ctx, installer.QualifiedLaunchOptions{SeededOperationOptions: seed, ExpectedBinding: installtx.ReleaseBinding{ManifestSHA256: record.Materialization.ManifestSHA256, ClientInputsSHA256: record.Materialization.ClientInputsSHA256}, Workspace: ws}); e == nil {
		t.Fatal("release-bound N packet acquired N+1")
	}
	call(t, ctx, live, "maestro_query", map[string]any{"operation": "execution.list", "limit": 32}, true)
	// The real profile has no strong shared-tree replacement custody. Refuse
	// automatic replacement, preserving the old record/tree; then the explicit
	// operator step moves that exact tree to its immutable digest-named backup.
	autoReview, e := installer.ReviewClientsSeeded(ctx, options)
	must(t, e)
	oldCodeBefore := snapshot(t, codeRoot)
	if _, e = setup.ApplyClients(ctx, options, autoReview.Digest, true); e == nil || !strings.Contains(e.Error(), "client_tree_replacement_custody") || !reflect.DeepEqual(oldCodeBefore, snapshot(t, codeRoot)) {
		t.Fatalf("automatic replacement did not refuse/preserve: %v", e)
	}
	must(t, os.Rename(codeRoot, codeRoot+".previous-"+record.Materialization.CodeTreeSHA256))
	preCrashRecord := read(t, recordPath)
	crash := exec.CommandContext(ctx, process, "test-client-interrupt", id)
	crashOutput, crashErr := crash.CombinedOutput()
	var exit *exec.ExitError
	if !errors.As(crashErr, &exit) || exit.ExitCode() != 73 {
		t.Fatalf("actual swap interruption failed: %s %v", crashOutput, crashErr)
	}
	if !bytes.Equal(preCrashRecord, read(t, recordPath)) {
		t.Fatal("interrupted materialization published record")
	}
	crashProof, _ := json.MarshalIndent(map[string]any{"exit_code": exit.ExitCode(), "lock": string(read(t, filepath.Join(seed.ManagedRoot, ".activation.lock"))), "record_unchanged": true, "packet_proof": json.RawMessage(read(t, fixture+".interrupted"))}, "", "  ")
	must(t, os.WriteFile(filepath.Join(root, "client-interruption-proof.json"), append(crashProof, '\n'), 0600))
	options.Request.Reconcile = true
	materialReview, err := installer.ReviewClientsSeeded(ctx, options)
	must(t, err)
	record, err = setup.ApplyClients(ctx, options, materialReview.Digest, true)
	must(t, err)
	if record.Materialization.DesktopSHA256 != desktopSHA || record.Materialization.ServerSHA256 != sum(read(t, filepath.Join(request.ManagedRoot, "bin", "maestro-mcp"))) || record.Materialization.Release != "0.1.43" {
		t.Fatal("stable Desktop reuse/current binding failed")
	}
	for _, name := range []string{"WrongCachedCodeBinding", "ChangedCodeBackup", "MissingCodeManagementLauncher", "AuthenticCachedCodeRecovery"} {
		t.Run(name, func(t *testing.T) {
			var nonce [16]byte
			_, e := rand.Read(nonce[:])
			must(t, e)
			carrierID := fmt.Sprintf("%x", nonce[:])
			selected := installer.ClientOptions{SeededOperationOptions: seed, LookupEnv: c.LookupEnv, Request: installer.ClientRequest{CarrierID: carrierID, Workspace: ws, CodeRoot: filepath.Join(root, name+"-code"), DesktopPath: filepath.Join(root, name+"-desktop.mcpb"), CodeScope: "user"}}
			apply := func() (installer.ClientRecord, error) {
				review, err := installer.ReviewClientsSeeded(ctx, selected)
				must(t, err)
				return setup.ApplyClients(ctx, selected, review.Digest, true)
			}
			original, e := apply()
			must(t, e)
			currentForCandidate, e := installtx.ReadSourceState(seed.DataRoot, seed.ManagedRoot)
			must(t, e)
			candidate := selected.Request.CodeRoot + ".candidate-" + carrierID + "-" + currentForCandidate.Binding.ManifestSHA256
			backup := selected.Request.CodeRoot + ".previous-" + original.Materialization.CodeTreeSHA256
			recordFile := filepath.Join(seed.DataRoot, "clients", carrierID+".json")
			if name == "ChangedCodeBackup" {
				// Explicit operator retention; production never moves a shared tree.
				must(t, os.Rename(selected.Request.CodeRoot, backup))
				original, e = apply()
				must(t, e)
				if original.Materialization.CodeBackupTreeSHA256 == "" {
					t.Fatal("actual repeated materialization did not record its backup")
				}
				must(t, os.WriteFile(filepath.Join(backup, "Manage Maestro.command"), []byte("#!/bin/sh\n# unknown edited backup must survive\nexit 0\n"), 0700))
			} else {
				must(t, filepath.WalkDir(selected.Request.CodeRoot, func(path string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					relative, err := filepath.Rel(selected.Request.CodeRoot, path)
					if err != nil {
						return err
					}
					destination := filepath.Join(candidate, relative)
					info, err := d.Info()
					if err != nil {
						return err
					}
					if d.IsDir() {
						return os.Mkdir(destination, info.Mode().Perm())
					}
					return os.WriteFile(destination, read(t, path), info.Mode().Perm())
				}))
				if name == "WrongCachedCodeBinding" {
					plugin := filepath.Join(candidate, "plugins", "maestro-product")
					must(t, os.RemoveAll(plugin))
					server := filepath.Join(seed.ManagedRoot, "bin", "maestro-mcp")
					current, err := installtx.ReadSourceState(seed.DataRoot, seed.ManagedRoot)
					must(t, err)
					args := []string{"plugin", "--destination", plugin, "--server", server, "--version", current.Release, "--product-root", seed.ManagedRoot, "--user-home", user, "--workspace", alternateWorkspace, "--installed-manifest", current.Binding.ManifestSHA256, "--installed-client-inputs", current.Binding.ClientInputsSHA256, "--carrier-profile", installer.StableClientProfile, "--carrier-id", carrierID, "--carrier-version", installer.StableCarrierVersion}
					command := exec.CommandContext(ctx, server, args...)
					command.Dir = alternateWorkspace
					if output, err := command.CombinedOutput(); err != nil {
						t.Fatalf("actual Current wrong-workspace Code generator: %s %v", output, err)
					}
					cmd := exec.CommandContext(ctx, server, "packet-verify", "--destination", plugin, "--packet-format", "plugin", "--carrier-profile", installer.StableClientProfile, "--carrier-id", carrierID, "--carrier-version", installer.StableCarrierVersion)
					if output, e := cmd.CombinedOutput(); e != nil {
						t.Fatalf("actual current wrong-workspace packet integrity: %s %v", output, e)
					}
				} else if name == "MissingCodeManagementLauncher" {
					// Exact reachable state after marketplace publication but before
					// its required management launcher is written.
					must(t, os.Remove(filepath.Join(candidate, "Manage Maestro.command")))
				}
			}
			capture := func(path string) map[string]string {
				if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
					return nil
				} else {
					must(t, err)
				}
				return snapshot(t, path)
			}
			beforeRecord, beforeCode, beforeDesktop := capture(recordFile), capture(selected.Request.CodeRoot), capture(selected.Request.DesktopPath)
			beforeBackup, beforeCandidate := capture(backup), capture(candidate)
			actual, applyErr := apply()
			if name == "AuthenticCachedCodeRecovery" {
				must(t, applyErr)
				if actual.Materialization.CodeTreeSHA256 != original.Materialization.CodeTreeSHA256 || !reflect.DeepEqual(beforeCandidate, capture(selected.Request.CodeRoot)) {
					t.Fatal("authentic complete Code candidate was not recovered exactly")
				}
				_, e = installer.PrepareStableLaunch(ctx, selected)
				must(t, e)
			} else {
				preserved := reflect.DeepEqual(beforeRecord, capture(recordFile)) && reflect.DeepEqual(beforeCode, capture(selected.Request.CodeRoot)) && reflect.DeepEqual(beforeDesktop, capture(selected.Request.DesktopPath)) && reflect.DeepEqual(beforeBackup, capture(backup))
				if beforeCandidate != nil {
					preserved = preserved && reflect.DeepEqual(beforeCandidate, capture(candidate))
				}
				errorText := ""
				if applyErr != nil {
					errorText = applyErr.Error()
				}
				proof, _ := json.MarshalIndent(map[string]any{"case": name, "carrier_id": carrierID, "actual_current_server_sha256": original.Materialization.ServerSHA256, "recorded_workspace_id": original.Binding.WorkspaceID, "alternate_workspace_id": alternateRegistration.WorkspaceID, "apply_error": errorText, "recorded_tree_after_apply": actual.Materialization.CodeTreeSHA256, "published_and_unknown_bytes_modes_preserved": preserved}, "", "  ")
				must(t, os.WriteFile(filepath.Join(root, name+"-proof.json"), append(proof, '\n'), 0600))
				if applyErr == nil || !preserved {
					t.Fatalf("unsafe Code recovery: error=%v preserved=%v", applyErr, preserved)
				}
			}
		})
	}

	// Both recovery branches must reject a valid Current packet whose native
	// argv binds this same carrier to a different registered workspace.
	for _, name := range []string{"WrongDesktopBindingExistingRecord", "WrongDesktopBindingFirstPublication", "WrongCachedDesktopCandidateBinding", "AuthenticDesktopFirstPublicationRecovery"} {
		existingRecord := name == "WrongDesktopBindingExistingRecord"
		t.Run(name, func(t *testing.T) {
			var nonce [16]byte
			_, e := rand.Read(nonce[:])
			must(t, e)
			wrongID := fmt.Sprintf("%x", nonce[:])
			selected := installer.ClientOptions{SeededOperationOptions: seed, LookupEnv: c.LookupEnv, Request: installer.ClientRequest{CarrierID: wrongID, Workspace: ws, CodeRoot: filepath.Join(root, name+"-code"), DesktopPath: filepath.Join(root, name+"-desktop.mcpb"), CodeScope: "user"}}
			recordFile := filepath.Join(seed.DataRoot, "clients", wrongID+".json")
			backup := selected.Request.DesktopPath + ".previous-" + wrongID + ".mcpb"
			if existingRecord {
				review, e := installer.ReviewClientsSeeded(ctx, selected)
				must(t, e)
				_, e = setup.ApplyClients(ctx, selected, review.Digest, true)
				must(t, e)
				must(t, os.WriteFile(backup, read(t, selected.Request.DesktopPath), 0600))
			}
			current, e := installtx.ReadSourceState(seed.DataRoot, seed.ManagedRoot)
			must(t, e)
			server := filepath.Join(seed.ManagedRoot, "bin", "maestro-mcp")
			carrier := packaging.StableCarrier{Profile: installer.StableClientProfile, Protocol: installer.StableClientProtocol, ID: wrongID, Version: installer.StableCarrierVersion}
			generate := func(destination, workspacePath string) string {
				args := []string{"mcpb", "--destination", destination, "--server", server, "--version", current.Release, "--product-root", seed.ManagedRoot, "--user-home", user, "--workspace", workspacePath, "--installed-manifest", current.Binding.ManifestSHA256, "--installed-client-inputs", current.Binding.ClientInputsSHA256, "--carrier-profile", carrier.Profile, "--carrier-id", carrier.ID, "--carrier-version", carrier.Version}
				cmd := exec.CommandContext(ctx, server, args...)
				cmd.Dir = workspacePath
				if output, e := cmd.CombinedOutput(); e != nil {
					t.Fatalf("actual same-carrier current packet generator: %s %v", output, e)
				}
				inspect := exec.CommandContext(ctx, server, "packet-verify", "--destination", destination, "--packet-format", "mcpb", "--carrier-profile", carrier.Profile, "--carrier-id", carrier.ID, "--carrier-version", carrier.Version)
				body, e := inspect.CombinedOutput()
				if e != nil {
					t.Fatalf("actual current archive integrity: %s %v", body, e)
				}
				var observed packaging.PacketStatus
				must(t, releasecontract.DecodeStrictJSON(body, &observed))
				return observed.NativeContractSHA256
			}
			expectedPath, wrongPath := filepath.Join(root, name+"-expected.mcpb"), filepath.Join(root, name+"-other-workspace.mcpb")
			expectedNative := generate(expectedPath, ws)
			if name == "AuthenticDesktopFirstPublicationRecovery" {
				expectedBody := read(t, expectedPath)
				must(t, os.WriteFile(selected.Request.DesktopPath, expectedBody, 0600))
				// Desktop candidate appearance witnesses that canonical Code
				// inspection has completed before the Desktop generator runs.
				// Preserve a subsequent writer's bytes, never adopt a fresh
				// hash of uninspected content into the current private record.
				codeCandidate := selected.Request.CodeRoot + ".candidate-" + wrongID + "-" + current.Binding.ManifestSHA256
				desktopCandidate := selected.Request.DesktopPath + ".candidate-" + wrongID + "-" + current.Binding.ManifestSHA256 + ".mcpb"
				type observedChange struct {
					body []byte
					err  error
				}
				changed := make(chan observedChange, 1)
				stopWriter := make(chan struct{})
				go func() {
					ticker := time.NewTicker(time.Millisecond)
					defer ticker.Stop()
					deadline := time.NewTimer(time.Minute)
					defer deadline.Stop()
					for {
						if _, err := os.Lstat(desktopCandidate); err == nil {
							path := filepath.Join(codeCandidate, "Manage Maestro.command")
							before, err := os.ReadFile(path)
							if err != nil {
								changed <- observedChange{err: err}
								return
							}
							body := append(append([]byte{}, before...), []byte("# OBSERVED_POST_INSPECTION_WRITER\n")...)
							err = os.WriteFile(path, body, 0700)
							changed <- observedChange{body: body, err: err}
							return
						} else if !errors.Is(err, os.ErrNotExist) {
							changed <- observedChange{err: err}
							return
						}
						select {
						case <-ticker.C:
						case <-stopWriter:
							changed <- observedChange{err: errors.New("post-inspection candidate transition was not observed")}
							return
						case <-deadline.C:
							changed <- observedChange{err: errors.New("post-inspection candidate writer timed out")}
							return
						}
					}
				}()
				selected.Request.Reconcile = true
				review, e := installer.ReviewClientsSeeded(ctx, selected)
				must(t, e)
				_, changeErr := setup.ApplyClients(ctx, selected, review.Digest, true)
				close(stopWriter)
				mutation := <-changed
				must(t, mutation.err)
				if changeErr == nil {
					t.Fatal("post-inspection Code bytes were adopted as a fresh current materialization")
				}
				if _, err := os.Lstat(recordFile); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("observed post-inspection change published a client record: %v", err)
				}
				preserved := false
				for _, path := range []string{selected.Request.CodeRoot, codeCandidate} {
					if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
						continue
					} else {
						must(t, err)
					}
					if bytes.Equal(read(t, filepath.Join(path, "Manage Maestro.command")), mutation.body) {
						preserved = true
					}
					// Explicit operator retention preserves changed bytes while
					// allowing a fresh, separately inspected publication.
					must(t, os.Rename(path, path+".observed-change"))
				}
				if !preserved {
					t.Fatal("observed post-inspection writer bytes were lost")
				}
				review, e = installer.ReviewClientsSeeded(ctx, selected)
				must(t, e)
				recovered, e := setup.ApplyClients(ctx, selected, review.Digest, true)
				must(t, e)
				if recovered.Materialization.DesktopNativeContractSHA256 != expectedNative || recovered.Materialization.DesktopSHA256 != sum(expectedBody) || !bytes.Equal(expectedBody, read(t, selected.Request.DesktopPath)) {
					t.Fatal("authentic interrupted first publication was not adopted exactly")
				}
				if _, e = os.Lstat(backup); !errors.Is(e, os.ErrNotExist) {
					t.Fatal("first publication invented a Desktop backup")
				}
				proof, _ := json.MarshalIndent(map[string]any{"first_publication_recovery": true, "carrier_id": wrongID, "actual_current_server_sha256": current.Server.SHA256, "native_contract_sha256": expectedNative, "actual_destination_package_sha256": sum(expectedBody), "destination_bytes_unchanged": true, "backup_absent": true}, "", "  ")
				must(t, os.WriteFile(filepath.Join(root, name+"-proof.json"), append(proof, '\n'), 0600))
				return
			}
			wrongNative := generate(wrongPath, alternateWorkspace)
			if expectedNative == wrongNative {
				t.Fatal("different registered workspace did not change native contract")
			}
			cachedCandidate := name == "WrongCachedDesktopCandidateBinding"
			candidatePath := selected.Request.DesktopPath + ".candidate-" + wrongID + "-" + current.Binding.ManifestSHA256 + ".mcpb"
			wrongDestination := selected.Request.DesktopPath
			if cachedCandidate {
				wrongDestination = candidatePath
			}
			must(t, os.WriteFile(wrongDestination, read(t, wrongPath), 0600))
			capture := func(path string) map[string]string {
				if _, e := os.Lstat(path); errors.Is(e, os.ErrNotExist) {
					return nil
				} else {
					must(t, e)
				}
				return snapshot(t, path)
			}
			beforeRecord, beforeDestination, beforeBackup, beforeCode := capture(recordFile), capture(selected.Request.DesktopPath), capture(backup), capture(selected.Request.CodeRoot)
			beforeCandidate := capture(candidatePath)
			selected.Request.Reconcile = true
			review, e := installer.ReviewClientsSeeded(ctx, selected)
			must(t, e)
			actual, applyErr := setup.ApplyClients(ctx, selected, review.Digest, true)
			preserved := reflect.DeepEqual(beforeRecord, capture(recordFile)) && reflect.DeepEqual(beforeDestination, capture(selected.Request.DesktopPath)) && reflect.DeepEqual(beforeBackup, capture(backup)) && reflect.DeepEqual(beforeCode, capture(selected.Request.CodeRoot)) && (!cachedCandidate || reflect.DeepEqual(beforeCandidate, capture(candidatePath)))
			errorText := ""
			if applyErr != nil {
				errorText = applyErr.Error()
			}
			proof, _ := json.MarshalIndent(map[string]any{"branch": name, "same_carrier": wrongID, "actual_current_server_sha256": current.Server.SHA256, "reviewed_workspace_id": record.Binding.WorkspaceID, "other_registered_workspace_id": alternateRegistration.WorkspaceID, "expected_native_contract_sha256": expectedNative, "wrong_destination_native_contract_sha256": wrongNative, "inspector_accepts_actual_current_server_resources": true, "known_old_backup_present": existingRecord, "wrong_cached_candidate": cachedCandidate, "apply_error": errorText, "returned_record_native_contract_sha256": actual.Materialization.DesktopNativeContractSHA256, "destination_backup_record_code_bytes_modes_preserved": preserved}, "", "  ")
			must(t, os.WriteFile(filepath.Join(root, name+"-proof.json"), append(proof, '\n'), 0600))
			if applyErr == nil || !preserved {
				t.Fatalf("wrong native contract adopted or state changed: error=%v preserved=%v expected=%s actual_destination=%s recorded=%s", applyErr, preserved, expectedNative, wrongNative, actual.Materialization.DesktopNativeContractSHA256)
			}
		})
	}
	t.Run("ManualDesktopInterruptedReplacement", func(t *testing.T) {
		pendingStatus, e := installer.ReadClientsSeeded(ctx, pendingOptions)
		must(t, e)
		pendingOptions.Request.CodeExecutable = pendingStatus.Record.Code.Executable
		pendingRecordPath := filepath.Join(seed.DataRoot, "clients", pendingID+".json")
		before := read(t, pendingRecordPath)
		oldDesktop := pendingStatus.Record.Materialization.DesktopSHA256
		// Manual custody step, performed by the operator outside the product.
		must(t, os.Rename(pendingOptions.Request.CodeRoot, pendingOptions.Request.CodeRoot+".previous-"+pendingStatus.Record.Materialization.CodeTreeSHA256))
		must(t, os.Rename(pendingDesktop, pendingDesktop+".previous-"+oldDesktop+".mcpb"))
		manualCrash := exec.CommandContext(ctx, process, "test-client-interrupt", pendingID, "manual-desktop-update")
		output, crashErr := manualCrash.CombinedOutput()
		var crashed *exec.ExitError
		if !errors.As(crashErr, &crashed) || crashed.ExitCode() != 73 {
			t.Fatalf("manual Desktop process interruption: %s %v", output, crashErr)
		}
		if !bytes.Equal(before, read(t, pendingRecordPath)) || sum(read(t, pendingDesktop)) == oldDesktop {
			t.Fatal("manual crash did not swap current bytes before preserving old record")
		}
		backup := pendingDesktop + ".previous-" + oldDesktop + ".mcpb"
		if sum(read(t, backup)) != oldDesktop {
			t.Fatal("manual swap lost exact old parent")
		}
		// Cleanup only releases the known dead process lock after an assertion;
		// no packet repair or record publication is supplied by the harness.
		defer func() {
			_ = installtx.GuardQualifiedReconciliationCurrent(seed.ManagedRoot, seed.DataRoot, installtx.ReleaseBinding{}, time.Now(), func() error { return nil })
		}()
		pendingOptions.Request.Reconcile = true
		review, e := installer.ReviewClientsSeeded(ctx, pendingOptions)
		must(t, e)
		recovered, e := setup.ApplyClients(ctx, pendingOptions, review.Digest, true)
		must(t, e)
		if recovered.Materialization.DesktopSHA256 != sum(read(t, pendingDesktop)) || recovered.Materialization.BootstrapServerSHA256 != record.Materialization.ServerSHA256 {
			t.Fatal("manual current Desktop swap was not reconciled")
		}
		if sum(read(t, backup)) != oldDesktop {
			t.Fatal("reconciliation repeated Desktop swap or lost parent")
		}
		pendingOptions.Request.Reconcile = false
		pendingOptions.Request.ManualDesktopUpdate = true
		review, e = installer.ReviewClientsSeeded(ctx, pendingOptions)
		must(t, e)
		_, e = setup.ApplyClients(ctx, pendingOptions, review.Digest, true)
		must(t, e)
		knownBackup := read(t, backup)
		publishedRecord, publishedDesktop := read(t, pendingRecordPath), read(t, pendingDesktop)
		must(t, os.WriteFile(backup, []byte("unknown desktop backup must survive"), 0600))
		review, e = installer.ReviewClientsSeeded(ctx, pendingOptions)
		must(t, e)
		if _, e = setup.ApplyClients(ctx, pendingOptions, review.Digest, true); e == nil || string(read(t, backup)) != "unknown desktop backup must survive" || !bytes.Equal(publishedRecord, read(t, pendingRecordPath)) || !bytes.Equal(publishedDesktop, read(t, pendingDesktop)) {
			t.Fatal("unknown Desktop backup admitted or changed")
		}
		must(t, os.WriteFile(backup, knownBackup, 0600))
		alias := backup + ".alias"
		must(t, os.Link(backup, alias))
		review, e = installer.ReviewClientsSeeded(ctx, pendingOptions)
		must(t, e)
		if _, e = setup.ApplyClients(ctx, pendingOptions, review.Digest, true); e == nil || !bytes.Equal(knownBackup, read(t, backup)) || !bytes.Equal(publishedRecord, read(t, pendingRecordPath)) {
			t.Fatal("aliased Desktop backup admitted or changed")
		}
		must(t, os.Remove(alias))
		proof, _ := json.MarshalIndent(map[string]any{"exit_code": crashed.ExitCode(), "old_record_unchanged_at_crash": true, "old_package_sha256": oldDesktop, "current_package_sha256": recovered.Materialization.DesktopSHA256, "reconciled_without_repeating_swap": true, "subsequent_manual_replacement": true, "unknown_and_aliased_backup_preserved": true}, "", "  ")
		must(t, os.WriteFile(filepath.Join(root, "manual-desktop-recovery-proof.json"), append(proof, '\n'), 0600))
	})
	futureDoctor := read(t, filepath.Join(futureSource, "bundles/base/skills/maestro-doctor/SKILL.md"))
	if !bytes.Contains(read(t, filepath.Join(codeRoot, "plugins/maestro-product/skills/maestro-doctor/SKILL.md")), []byte(currentMethodMarker)) {
		t.Fatal("N1 Code method came from old installer instead of authenticated current generator")
	}
	if !bytes.Equal(futureDoctor, read(t, filepath.Join(request.ManagedRoot, "bundles/0.1.43/skills/maestro-doctor/SKILL.md"))) {
		t.Fatal("signed N1 managed method differs from independent fixture source")
	}
	if sum(read(t, seed.Self)) != seedSHA || bytes.Contains(read(t, seed.Self), []byte(currentMethodMarker)) {
		t.Fatal("update changed independently admitted N installer")
	}
	nextIdentity, e := releasecontract.ReadServerIdentity(filepath.Join(request.ManagedRoot, "bin/maestro-mcp"))
	must(t, e)
	assertSourceClosure(t, futureSource, nextIdentity, request.ManagedRoot, "0.1.43")
	codeReview, err := setup.ReviewCodeRegistration(ctx, record, nil)
	must(t, err)
	record, err = setup.RegisterCode(ctx, options, nil, codeReview.Digest, true)
	must(t, err)
	if record.Code.Enabled || record.Code.FolderVersion != "0.1.43" {
		t.Fatal("disabled update/current folder readback failed")
	}
	restarted := connectStable()
	defer restarted.Close()
	methodRead, e := restarted.ReadResource(ctx, &mcp.ReadResourceParams{URI: "maestro://methods/maestro-doctor/SKILL.md"})
	must(t, e)
	if len(methodRead.Contents) != 1 || methodRead.Contents[0].Text != string(futureDoctor) {
		t.Fatal("actual N1 stdio resource did not read exact future method")
	}
	nextStep := call(t, ctx, restarted, "maestro_query", map[string]any{"operation": "execution.next", "item_id": itemID}, false)
	if nextStep["next_step"] != "recover exact next step" {
		t.Fatal(nextStep)
	}
	memories := call(t, ctx, restarted, "maestro_query", map[string]any{"operation": "memory.list", "limit": 32}, false)
	memoryBody, _ := json.Marshal(memories)
	if !bytes.Contains(memoryBody, []byte("source_digest")) {
		t.Fatalf("memory did not survive restart: %s", memoryBody)
	}
	captureID, _ := captured["id"].(string)
	if nested, ok := captured["capture"].(map[string]any); ok {
		captureID, _ = nested["id"].(string)
	}
	if captureID == "" {
		t.Fatalf("capture did not report its canonical ID: %v", captured)
	}
	memoryRead := call(t, ctx, restarted, "maestro_query", map[string]any{"operation": "memory.read", "capture_id": captureID}, false)
	memoryReadBody, _ := json.Marshal(memoryRead)
	if !bytes.Contains(memoryReadBody, []byte("CLIENT_STABLE_LITERAL_MEMORY")) {
		t.Fatal("literal memory body lost across stable restart")
	}
	knowledge = call(t, ctx, restarted, "maestro_query", map[string]any{"operation": "knowledge.collect", "purpose": "verify named authored workspace knowledge after update", "pages": []string{"knowledge/client-proof.md"}}, false)
	knowledgeBody, _ = json.Marshal(knowledge)
	if !bytes.Contains(knowledgeBody, []byte("CLIENT_STABLE_LITERAL_KNOWLEDGE")) {
		t.Fatalf("knowledge lost across stable restart: %s", knowledgeBody)
	}
	if !reflect.DeepEqual(ownerBefore, snapshot(t, installed.Home)) || !reflect.DeepEqual(workspaceBefore, snapshot(t, ws)) {
		t.Fatal("materialization/update changed owner/workspace bytes")
	}
	wrong := options
	wrong.Request.Workspace = filepath.Join(root, "another-workspace")
	if _, e := installer.PrepareStableLaunch(ctx, wrong); e == nil {
		t.Fatal("stable ID redirected to another workspace")
	}
	rollback, err := installer.ReviewRollbackSeeded(ctx, seed)
	must(t, err)
	_, err = installer.RollbackSeeded(ctx, installer.SeededRollbackOptions{SeededOperationOptions: seed, ReviewDigest: rollback.Digest})
	must(t, err)
	if _, e := installer.PrepareStableLaunch(ctx, options); e == nil {
		t.Fatal("rollback restored materialization/source currency")
	}
	// Rollback changes the public Code payload too; the explicit operator
	// preserves N1 before exclusive publication of authenticated restored N.
	must(t, os.Rename(codeRoot, codeRoot+".previous-"+record.Materialization.CodeTreeSHA256))
	materialReview, err = installer.ReviewClientsSeeded(ctx, options)
	must(t, err)
	record, err = setup.ApplyClients(ctx, options, materialReview.Digest, true)
	must(t, err)
	if record.Materialization.Release != "0.1.42" || record.Materialization.DesktopSHA256 != desktopSHA {
		t.Fatal("rollback regeneration did not preserve bootstrap")
	}
	if bytes.Contains(read(t, filepath.Join(codeRoot, "plugins/maestro-product/skills/maestro-doctor/SKILL.md")), []byte(currentMethodMarker)) {
		t.Fatal("rollback regeneration retained future method instead of restored current source")
	}
	finalState, err := installtx.ReadSourceState(seed.DataRoot, seed.ManagedRoot)
	must(t, err)
	if finalState.Source.Sequence != 3 {
		t.Fatal("rollback reset source history")
	}
	// Reconciliation cannot reclaim a live owner's canonical lock; ordinary
	// launch refuses even a dead lock until this separate reviewed operation.
	hold := exec.CommandContext(ctx, process, "test-client-hold")
	holdIn, e := hold.StdinPipe()
	must(t, e)
	holdOut, e := hold.StdoutPipe()
	must(t, e)
	must(t, hold.Start())
	line, e := bufio.NewReader(holdOut).ReadString('\n')
	must(t, e)
	if line != "ready\n" {
		t.Fatal("live process witness did not start")
	}
	lockPath := filepath.Join(seed.ManagedRoot, ".activation.lock")
	liveLock := []byte(fmt.Sprintf("%d\n", hold.Process.Pid))
	must(t, os.WriteFile(lockPath, liveLock, 0600))
	liveRecord := read(t, recordPath)
	materialReview, e = installer.ReviewClientsSeeded(ctx, options)
	must(t, e)
	if _, e = setup.ApplyClients(ctx, options, materialReview.Digest, true); e == nil || !bytes.Equal(liveLock, read(t, lockPath)) || !bytes.Equal(liveRecord, read(t, recordPath)) {
		t.Fatal("live installation owner was reclaimed/mutated")
	}
	must(t, holdIn.Close())
	must(t, hold.Wait())
	if _, e = installer.PrepareStableLaunch(ctx, options); e == nil {
		t.Fatal("ordinary launch reclaimed a dead owner lock")
	}
	materialReview, e = installer.ReviewClientsSeeded(ctx, options)
	must(t, e)
	record, e = setup.ApplyClients(ctx, options, materialReview.Digest, true)
	must(t, e)
	if _, e = os.Lstat(lockPath); !errors.Is(e, os.ErrNotExist) {
		t.Fatal("explicit dead-owner reconciliation did not release lock")
	}
	if !reflect.DeepEqual(ownerBefore, snapshot(t, installed.Home)) || !reflect.DeepEqual(workspaceBefore, snapshot(t, ws)) {
		t.Fatal("client reconciliation changed owner/workspace")
	}
	publish(4, n, []releasecontract.ReleaseRecall{{Release: n.verified.Manifest.Release, ManifestSHA256: n.verified.ManifestSHA256}})
	_, recallErr := check()
	if recallErr == nil {
		t.Fatal("recalled current release accepted")
	}
	if _, e = installer.PrepareStableLaunch(ctx, options); e == nil {
		t.Fatal("current client record bypassed authenticated recall")
	}
	proof, _ := json.MarshalIndent(map[string]any{"native_admission": "engineering process composition over exact real seed; shipping negative passed", "actual_clients": "unattested", "old_server_sha256": oldSHA, "n_artifacts": n.verified.Manifest.Artifacts, "next_artifacts": next.verified.Manifest.Artifacts, "actual_installer_sha256": seedSHA, "unused_future_factory_seed_sha256": sum(read(t, next.factory.Installer)), "original_doctor_sha256": sum(read(t, filepath.Join(source, "bundles/base/skills/maestro-doctor/SKILL.md"))), "future_doctor_sha256": sum(futureDoctor), "old_installer_lacks_future_marker": true, "next_release": "0.1.43", "stable_desktop_sha256": desktopSHA, "carrier_id": id, "source_sequence_after_rollback": finalState.Source.Sequence, "first_run": "actual console source review and generated ID", "code": "supported controlled process readback; disabled preserved", "sdk": "actual stable launcher process to real shipping server stdio continuation/memory/knowledge and exact changed future-method resource", "owner_workspace_preserved": true}, "", "  ")
	must(t, os.WriteFile(filepath.Join(root, "client-lifecycle-proof.json"), append(proof, '\n'), 0600))
}
