//go:build maestro_experimental_installed_seed

package setup_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Reuse the original finite signed-source fixture. Its existing host boundary
// remains explicit; this is source integration, not an installed host claim.
func TestBothClientsDesktopPreparationExperimentalConsumer(t *testing.T) {
	experimentalInstalledSeedFiniteSourceLifecycle(t, true, true)
}

func assertPendingCodeDesktopConsumer(t *testing.T, ctx context.Context, seed installer.SeededOperationOptions, workspace, brain, brainFile, workspaceID, root string) {
	t.Helper()
	packetRoot := filepath.Join(root, "partial-client-packets")
	must(t, os.Mkdir(packetRoot, 0700))
	codeCalls := filepath.Join(root, "unexpected-pending-Code-invocation")
	codeExecutable := filepath.Join(root, "pending-Code-host")
	// The original finite fixture runs only on its supported native Mac host.
	// Any Code/auth/model/registration command would leave this exact witness.
	quotedCalls := "'" + strings.ReplaceAll(codeCalls, "'", "'\"'\"'") + "'"
	must(t, os.WriteFile(codeExecutable, []byte("#!/bin/sh\nprintf 'called\\n' >> "+quotedCalls+"\nexit 73\n"), 0700))
	o := installer.ClientOptions{SeededOperationOptions: seed, Request: installer.ClientRequest{VersionedSlots: true, NativeEnforcement: true, GenerationID: strings.Repeat("4", 32), CarrierID: strings.Repeat("5", 32), Workspace: workspace, CodeRoot: filepath.Join(packetRoot, "Code"), DesktopPath: filepath.Join(packetRoot, "Desktop.mcpb"), CodeExecutable: codeExecutable, CodeScope: "user", OwnerReviewChannel: "local-browser"}}
	selection := o.Request
	brainBefore := read(t, brainFile)
	projection, err := installer.DesktopPreparationOptions(o)
	must(t, err)
	state, err := installtx.ReadSourceState(seed.DataRoot, seed.ManagedRoot)
	must(t, err)
	paths, err := installer.ClientGenerationPaths(projection.Request, state.Binding.ManifestSHA256)
	must(t, err)
	recordPath := filepath.Join(seed.DataRoot, "clients", o.Request.CarrierID+".json")
	review, err := installer.ReviewDesktopPreparationSeeded(ctx, o)
	must(t, err)
	base := setup.CurrentClientMaterializer(projection)
	interrupted := errors.New("owned fixture interruption after actual Desktop publication")
	built := false
	_, err = installer.MaterializeDesktopPreparationSeeded(ctx, o, review.Digest, true, func(ctx context.Context, state installtx.State, binding installer.ClientBinding, previous *installer.ClientRecord) (installer.ClientMaterialization, error) {
		if previous != nil {
			t.Fatal("initial preparation already had a current record")
		}
		material, buildErr := base(ctx, state, binding, nil)
		if buildErr != nil {
			return material, buildErr
		}
		built = true
		return material, interrupted
	})
	if err == nil || !built {
		t.Fatalf("actual Desktop publication interruption was not reached: %v", err)
	}
	if _, err := os.Lstat(recordPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("interrupted materialization published a current record", err)
	}
	retained := read(t, paths.DesktopPath)
	review, err = installer.ReviewDesktopPreparationSeeded(ctx, o)
	must(t, err)
	record, err := setup.ApplyDesktopPreparation(ctx, o, review.Digest, true)
	must(t, err)
	if !record.CodePreparationPending() || installer.ClientSelectedTarget(record) != "" || !reflect.DeepEqual(installer.ClientRequestFromRecord(record), selection) || record.Binding.Home != brain || record.Binding.WorkspaceID != workspaceID {
		t.Fatal("partial record lost full selection, workspace or Developer brain binding")
	}
	if !bytes.Equal(retained, read(t, record.Materialization.DesktopPath)) || record.Materialization.DesktopMethods == nil || record.Materialization.CodeRoot != "" || record.Materialization.CodeProviderRoot != "" || record.Materialization.NativeEnforcement {
		t.Fatal("recovery changed Desktop or fabricated current Code authority")
	}
	archive, err := packaging.ValidateArchive(record.Materialization.DesktopPath)
	must(t, err)
	if archive.Stable == nil || archive.NativeContractSHA256 != record.Materialization.DesktopNativeContractSHA256 {
		t.Fatal("full authenticated Desktop native contract missing")
	}
	status, err := installer.ReadClientsSeeded(ctx, o)
	must(t, err)
	if status.State != installer.ClientPartialPreparation || status.Preparation == nil || status.Preparation.SelectedTarget != "both" || status.CodeRegistration != installer.ClientCodeQualificationPending {
		t.Fatal("partial readiness was not reported truthfully", status)
	}
	if _, err := installer.CurrentClientPluginPath(ctx, o); err == nil {
		t.Fatal("pending Code supplied a current plugin")
	}
	if _, err := installer.PrepareStableLifecycleHook(ctx, o, "SessionStart"); err == nil {
		t.Fatal("pending Code supplied enforcing hooks")
	}
	if _, err := setup.RegisterCode(ctx, o, nil, strings.Repeat("a", 64), true); err == nil {
		t.Fatal("pending Code entered registration")
	}
	for _, lifecycle := range []bool{false, true} {
		downgrade := o
		downgrade.Request.NativeEnforcement = false
		downgrade.Request.LifecycleContext = lifecycle
		if _, err := installer.ReviewClientsSeeded(ctx, downgrade); err == nil {
			t.Fatal("pending full Code selection allowed resources-only or lifecycle downgrade")
		}
		builds := 0
		if _, err := installer.MaterializeClientsSeeded(ctx, downgrade, strings.Repeat("a", 64), true, func(context.Context, installtx.State, installer.ClientBinding, *installer.ClientRecord) (installer.ClientMaterialization, error) {
			builds++
			return installer.ClientMaterialization{}, errors.New("downgrade builder must remain unreachable")
		}); err == nil || builds != 0 {
			t.Fatal("downgrade reached materialization", err, builds)
		}
	}
	if _, err := os.Lstat(codeCalls); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("preparation invoked Code, authentication, model or registration before Desktop SDK", err)
	}
	launch, err := installer.PrepareStableLaunch(ctx, o)
	must(t, err)
	command := exec.CommandContext(ctx, launch.Executable, launch.Args...)
	var stderr bytes.Buffer
	command.Stderr = &stderr
	client, err := mcp.NewClient(&mcp.Implementation{Name: "both-selected-Code-pending-Desktop-source-consumer", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: command, TerminateDuration: time.Second}, nil)
	if err != nil {
		t.Fatalf("actual partial Desktop SDK startup: %v: %s", err, stderr.String())
	}
	value := call(t, ctx, client, "maestro_context", map[string]any{}, false)
	if value["workspace_id"] != workspaceID {
		t.Fatal("partial Desktop SDK selected another workspace")
	}
	call(t, ctx, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "maestro-onboarding"}, false)
	call(t, ctx, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "pocock-writing-for-agents"}, false)
	must(t, client.Close())
	confirmed, err := setup.ConfirmDesktop(ctx, o, record.Materialization.DesktopSHA256, record.Materialization.DesktopNativeContractSHA256, installer.ClientRecordDigest(record), true)
	must(t, err)
	if !confirmed.CodePreparationPending() || confirmed.Desktop.InstallConfirmation != "user_confirmed_install" || confirmed.Desktop.Connection != "pending" || confirmed.Desktop.FunctionalCheck != "pending" {
		t.Fatal("Desktop confirmation promoted Code or invented host connection evidence")
	}
	replay, err := installer.ReviewDesktopPreparationSeeded(ctx, o)
	must(t, err)
	again, err := setup.ApplyDesktopPreparation(ctx, o, replay.Digest, true)
	must(t, err)
	if again.Materialization.DesktopSHA256 != confirmed.Materialization.DesktopSHA256 || again.Materialization.DesktopMethods.SHA256 != confirmed.Materialization.DesktopMethods.SHA256 || again.Desktop != confirmed.Desktop {
		t.Fatal("replay changed the verified Desktop or its matching observations")
	}
	beforeDecline := read(t, recordPath)
	_, err = setup.ApplyNativeClientUpdate(ctx, o, setup.NativeClientSelection{}, strings.Repeat("b", 64), strings.Repeat("c", 64), false)
	if err == nil || !bytes.Equal(beforeDecline, read(t, recordPath)) || !bytes.Equal(retained, read(t, again.Materialization.DesktopPath)) {
		t.Fatal("declined Code resumption changed partial preparation")
	}
	if _, err := os.Lstat(codeCalls); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("preparation invoked Code, authentication, model or registration", err)
	}
	if !bytes.Equal(brainBefore, read(t, brainFile)) {
		t.Fatal("client preparation changed existing brain content")
	}
}
