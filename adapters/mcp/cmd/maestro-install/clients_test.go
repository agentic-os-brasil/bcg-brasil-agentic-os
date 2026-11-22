package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

func TestPendingCodeAuthenticatedUpdateKeepsOriginalNativeOwner(t *testing.T) {
	root := t.TempDir()
	selection := installer.ClientRequest{VersionedSlots: true, NativeEnforcement: true, GenerationID: strings.Repeat("b", 32), CarrierID: strings.Repeat("c", 32), Workspace: filepath.Join(root, "workspace"), CodeRoot: filepath.Join(root, "code"), DesktopPath: filepath.Join(root, "desktop.mcpb"), CodeExecutable: filepath.Join(root, "selected-Code"), CodeScope: "user"}
	projection, err := installer.DesktopPreparationOptions(installer.ClientOptions{Request: selection})
	if err != nil {
		t.Fatal(err)
	}
	material, err := installer.ClientGenerationPaths(projection.Request, strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	material.Release = "0.1.0"
	material.ManifestSHA256 = strings.Repeat("d", 64)
	material.ClientInputsSHA256 = strings.Repeat("e", 64)
	material.ServerSHA256 = strings.Repeat("f", 64)
	material.EmbeddedInputsSHA256 = strings.Repeat("1", 64)
	material.BundleSHA256 = strings.Repeat("2", 64)
	material.DesktopSHA256 = strings.Repeat("3", 64)
	material.DesktopNativeContractSHA256 = strings.Repeat("4", 64)
	material.BootstrapServerSHA256 = material.ServerSHA256
	material.BootstrapEmbeddedInputsSHA256 = material.EmbeddedInputsSHA256
	material.DesktopMethods = &installer.ClientMethodFrontend{BootstrapProtocol: "maestro-live-method-bootstrap-v1", Archive: filepath.Join(installer.ClientMethodsRoot(material), "maestro-desktop-methods-0.1.0.zip"), SHA256: strings.Repeat("5", 64), Version: material.Release, Files: 1}
	r := installer.ClientRecord{SchemaVersion: 3, Profile: installer.StableClientProfile, Protocol: installer.StableClientProtocol, CarrierVersion: installer.StableCarrierVersion, CarrierID: selection.CarrierID, Binding: installer.ClientBinding{Workspace: selection.Workspace}, Materialization: material, Code: installer.CodeObservation{Executable: selection.CodeExecutable, Marketplace: "maestro-local-" + selection.CarrierID, PluginID: "maestro-product@maestro-local-" + selection.CarrierID, Scope: selection.CodeScope, State: installer.ClientCodeQualificationPending, EvidenceClass: "not_observed"}, Preparation: &installer.ClientPreparation{Phase: installer.ClientPreparationDesktopReady, Selection: selection}}
	if _, err := installer.DesktopPreparationRecord(r); err != nil {
		t.Fatal("invalid pending record shape", err)
	}
	for _, operation := range []string{"update", "resume-code"} {
		t.Run(operation, func(t *testing.T) {
			action, desktop, err := resolveClientManagementAction(r, operation)
			if err != nil || action != operation || desktop {
				t.Fatalf("authenticated request changed to deferral or refused: %s %t %v", action, desktop, err)
			}
			calls := 0
			unavailable := errors.New("original native owner deliberately unavailable; no qualification receipt")
			digest := strings.Repeat("6", 64)
			o := installer.ClientOptions{Request: installer.ClientRequestFromRecord(r)}
			result, err := applyManagedClients(context.Background(), o, digest, desktop, func(got installer.ClientOptions, gotDigest string) (installer.ClientRecord, error) {
				calls++
				if got.Request != selection || gotDigest != digest {
					t.Fatal("native owner received changed selection or review")
				}
				return installer.ClientRecord{}, unavailable
			})
			if !errors.Is(err, unavailable) || calls != 1 || result.SchemaVersion != 0 || result.Preparation != nil {
				t.Fatalf("native failure became Desktop fallback or success: %+v %v calls=%d", result, err, calls)
			}
		})
	}
	for _, change := range []string{"profile", "carrier", "workspace", "phase", "Code claim"} {
		t.Run(change, func(t *testing.T) {
			body, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			var bad installer.ClientRecord
			if err := json.Unmarshal(body, &bad); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "profile":
				bad.Preparation.Selection.NativeEnforcement = false
			case "carrier":
				bad.Preparation.Selection.CarrierID = strings.Repeat("a", 32)
			case "workspace":
				bad.Preparation.Selection.Workspace = filepath.Join(root, "other-workspace")
			case "phase":
				bad.Preparation.Phase = "ready"
			case "Code claim":
				bad.Code.State = "registered"
			}
			for _, operation := range []string{"update", "resume-code"} {
				if _, _, err := resolveClientManagementAction(bad, operation); err == nil {
					t.Fatal("changed pending identity/profile entered publisher dispatch", operation)
				}
			}
		})
	}
}

func TestDesktopPreparationIntentRefusesLegacyWithoutCompleteMethods(t *testing.T) {
	root := t.TempDir()
	// A supported schema1 legacy carrier has neither versioned methods nor
	// native qualification. Its old commands retain their existing behavior;
	// it cannot supply this new full-method Desktop operation's success claim.
	legacy := installer.ClientRecord{SchemaVersion: 1, Profile: installer.StableClientProfile, Protocol: installer.StableClientProtocol, CarrierVersion: installer.StableCarrierVersion, CarrierID: strings.Repeat("a", 32), Binding: installer.ClientBinding{Workspace: filepath.Join(root, "workspace")}, Materialization: installer.ClientMaterialization{CodeRoot: filepath.Join(root, "legacy-code"), DesktopPath: filepath.Join(root, "legacy.mcpb")}, Code: installer.CodeObservation{Scope: "user", State: "pending"}}
	if legacy.Materialization.NativeEnforcement || legacy.Materialization.Selection != nil || legacy.Materialization.DesktopMethods != nil {
		t.Fatal("legacy fixture manufactured current qualification or methods")
	}
	if err := requireDesktopPreparationRecord(legacy); err == nil {
		t.Fatal("valid legacy carrier without complete methods accepted full Desktop scope")
	}
	for _, operation := range []string{"prepare-desktop", "update-desktop"} {
		var output strings.Builder
		if err := writeDesktopPreparationStatus(context.Background(), installer.ClientOptions{}, legacy, operation, &output); err == nil || !strings.Contains(err.Error(), "original full both-client profile") || output.Len() != 0 {
			t.Fatalf("unsupported scope reached installation lookup or success output: %s %v", output.String(), err)
		}
	}
	// A versioned resources-only carrier can have the complete Desktop archive
	// while still lacking the selected full Code contract this intent preserves.
	resources := legacy
	resources.Materialization.Release = "0.1.0"
	resources.Materialization.Selection = &installer.ClientPacketSelection{CodeRoot: legacy.Materialization.CodeRoot, DesktopPath: legacy.Materialization.DesktopPath}
	resources.Materialization.DesktopMethods = &installer.ClientMethodFrontend{BootstrapProtocol: "maestro-live-method-bootstrap-v1", Archive: filepath.Join(installer.ClientMethodsRoot(resources.Materialization), "maestro-desktop-methods-0.1.0.zip"), SHA256: strings.Repeat("b", 64), Version: "0.1.0", Files: 1}
	resources.Code.Executable = filepath.Join(root, "selected-Code")
	if err := requireDesktopPreparationRecord(resources); err == nil {
		t.Fatal("resources-only selection supplied the desired full profile")
	}
}

func TestFirstClientOwnerReviewChannelIsAnExplicitCommonChoice(t *testing.T) {
	for _, choice := range []string{"", "native-first", "local-browser", "model-approved"} {
		got, err := promptClientOwnerReviewChannel(func(message string) (string, error) {
			if !strings.Contains(message, "BOTH Code and Desktop") || !strings.Contains(message, "approves no write") {
				t.Fatal("first setup hid the common client scope or individual approval requirement")
			}
			return choice, nil
		}, false)
		if choice == "model-approved" {
			if err == nil {
				t.Fatal("unknown channel accepted")
			}
			continue
		}
		want := ""
		if choice == "local-browser" {
			want = choice
		}
		if err != nil || got != want {
			t.Fatalf("choice %q: channel=%q error=%v", choice, got, err)
		}
	}
}

func TestClientCapabilitiesReadOnlyWithoutInstallation(t *testing.T) {
	oldArgs, oldVersion, oldPin, oldOutput := os.Args, Version, AuthorityRegistrySHA256, os.Stdout
	defer func() { os.Args, Version, AuthorityRegistrySHA256, os.Stdout = oldArgs, oldVersion, oldPin, oldOutput }()
	Version, AuthorityRegistrySHA256 = "0.3.7", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	os.Args = []string{"maestro-install", "client-capabilities"}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	os.Stdout = w
	err = run()
	w.Close()
	os.Stdout = oldOutput
	body, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if err != nil {
		t.Fatalf("read-only capability discovery requires no installation: %v", err)
	}
	var value struct {
		Schema         int    `json:"schema_version"`
		Profile        string `json:"profile"`
		Protocol       string `json:"protocol"`
		Version        string `json:"version"`
		CarrierVersion string `json:"carrier_version"`
	}
	if err = json.Unmarshal(body, &value); err != nil {
		t.Fatal(err)
	}
	if value.Schema != 1 || value.Profile != "stable-local-v1" || value.Protocol != "maestro-stable-carrier-v1" || value.Version != "0.3.7" || value.CarrierVersion != "1.0.0" {
		t.Fatalf("unexpected actual capability: %s", body)
	}
	os.Args = []string{"maestro-install", "lifecycle-capabilities"}
	r2, w2, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer r2.Close()
	os.Stdout = w2
	e = run()
	w2.Close()
	os.Stdout = oldOutput
	body, e2 := io.ReadAll(r2)
	var lifecycle struct {
		Profile         string   `json:"profile"`
		Events          []string `json:"events"`
		Enforcement     string   `json:"enforcement"`
		NativeExecution string   `json:"native_execution"`
	}
	if e != nil || e2 != nil || json.Unmarshal(body, &lifecycle) != nil || lifecycle.Profile != "mcp_lifecycle" || len(lifecycle.Events) != 3 || lifecycle.Enforcement != "unavailable" || lifecycle.NativeExecution != "not_observed" {
		t.Fatalf("finite lifecycle capability: %s %v %v", body, e, e2)
	}

}

func TestDesktopTargetCapabilityAndFrontendRefusals(t *testing.T) {
	oldArgs, oldVersion, oldPin, oldOutput := os.Args, Version, AuthorityRegistrySHA256, os.Stdout
	defer func() { os.Args, Version, AuthorityRegistrySHA256, os.Stdout = oldArgs, oldVersion, oldPin, oldOutput }()
	Version, AuthorityRegistrySHA256 = "0.3.7", strings.Repeat("b", 64)
	os.Args = []string{"maestro-install", "client-target-capabilities"}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	os.Stdout = w
	err = run()
	w.Close()
	os.Stdout = oldOutput
	body, readErr := io.ReadAll(r)
	var got installer.ClientTargetCapability
	if err != nil || readErr != nil || json.Unmarshal(body, &got) != nil || got != installer.DesktopClientCapabilities("installer", Version, AuthorityRegistrySHA256) {
		t.Fatalf("target-aware actual reader must disclose its closed protocol without installation: %s %v %v", body, err, readErr)
	}
	os.Args = append(os.Args, "--unreviewed")
	if err := run(); err == nil {
		t.Fatal("target capabilities accepted extra arguments")
	}
	for _, command := range []string{"manage", "clients-status"} {
		if handled, err := runClients(context.Background(), command, "", lifecycleFlags{ClientTarget: "both"}, "", false); !handled || err == nil || !strings.Contains(err.Error(), "unsupported client target") {
			t.Fatalf("unknown target must refuse before installation/discovery: %v %v", handled, err)
		}
	}
	if handled, err := runClients(context.Background(), "code-apply", "", lifecycleFlags{ClientTarget: "desktop"}, "", false); !handled || err == nil {
		t.Fatal("Code mutation accepted a caller-selected Desktop target")
	}
	if err := manageClients(context.Background(), installer.ClientOptions{Request: installer.ClientRequest{Target: "desktop", CodeConfigurationDirectory: "hidden-profile"}}, strings.NewReader(""), io.Discard); err == nil || !strings.Contains(err.Error(), "cannot include Code choices") {
		t.Fatalf("Desktop choices must refuse before installed state, prompts or effects: %v", err)
	}
	var prompts []string
	_, err = promptClientOwnerReviewChannel(func(prompt string) (string, error) { prompts = append(prompts, prompt); return "", nil }, true)
	if err != nil || len(prompts) != 1 || strings.Contains(prompts[0], "BOTH Code") || !strings.Contains(prompts[0], "approves no write") {
		t.Fatalf("Desktop review must describe its actual selected scope: %v %v", prompts, err)
	}
}

func TestClientPreparationCapabilityWithoutInstallation(t *testing.T) {
	oldArgs, oldVersion, oldPin, oldOutput := os.Args, Version, AuthorityRegistrySHA256, os.Stdout
	defer func() { os.Args, Version, AuthorityRegistrySHA256, os.Stdout = oldArgs, oldVersion, oldPin, oldOutput }()
	Version, AuthorityRegistrySHA256 = "0.3.7", strings.Repeat("b", 64)
	os.Args = []string{"maestro-install", "client-preparation-capabilities"}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	os.Stdout = w
	err = run()
	w.Close()
	os.Stdout = oldOutput
	body, readErr := io.ReadAll(r)
	var got installer.ClientTargetCapability
	if err != nil || readErr != nil || json.Unmarshal(body, &got) != nil || got != installer.ClientPreparationCapabilities("installer", Version, AuthorityRegistrySHA256) {
		t.Fatalf("preparation reader must disclose its finite identity without an installation: %s %v %v", body, err, readErr)
	}
	os.Args = append(os.Args, "--unreviewed")
	if err := run(); err == nil {
		t.Fatal("preparation capability accepted extra arguments")
	}
}
