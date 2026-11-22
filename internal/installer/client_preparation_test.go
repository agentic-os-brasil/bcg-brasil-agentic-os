package installer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Exercise the strict wire reader, selected-profile continuity and publication CAS.
// The initial portable reader regression was also run against accepted production.
func TestClientPreparationRecordSchema(t *testing.T) {
	root := t.TempDir()
	owner := filepath.Join(root, "owner")
	data := filepath.Join(owner, ".maestro-install")
	if err := os.MkdirAll(filepath.Join(data, "clients"), 0700); err != nil {
		t.Fatal(err)
	}
	options := SeededOperationOptions{ManagedRoot: filepath.Join(root, "product"), UserHome: owner, DataRoot: data, AuthorityRegistrySHA256: strings.Repeat("a", 64)}
	selection := ClientRequest{VersionedSlots: true, NativeEnforcement: true, GenerationID: strings.Repeat("b", 32), CarrierID: strings.Repeat("c", 32), Workspace: filepath.Join(root, "workspace"), CodeRoot: filepath.Join(owner, "code"), DesktopPath: filepath.Join(owner, "desktop.mcpb"), CodeExecutable: filepath.Join(root, "claude"), CodeConfigurationDirectory: filepath.Join(owner, "code-profile"), CodeScope: "user"}
	projection := ClientRequest{Target: "desktop", VersionedSlots: true, CarrierID: selection.CarrierID, Workspace: selection.Workspace, DesktopPath: selection.DesktopPath}
	material, err := ClientGenerationPaths(projection, strings.Repeat("d", 64))
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
	material.DesktopMethods = &ClientMethodFrontend{BootstrapProtocol: "maestro-live-method-bootstrap-v1", Archive: filepath.Join(ClientMethodsRoot(material), "maestro-desktop-methods-0.1.0.zip"), SHA256: strings.Repeat("5", 64), Version: material.Release, Files: 2}
	code := CodeObservation{Executable: selection.CodeExecutable, ConfigurationDirectory: selection.CodeConfigurationDirectory, Marketplace: "maestro-local-" + selection.CarrierID, PluginID: "maestro-product@maestro-local-" + selection.CarrierID, Scope: "user", State: "pending_qualification", EvidenceClass: "not_observed"}
	wire := map[string]any{"schema_version": 3, "profile": StableClientProfile, "protocol": StableClientProtocol, "carrier_version": StableCarrierVersion, "carrier_id": selection.CarrierID, "binding": ClientBinding{ManagedRoot: options.ManagedRoot, UserHome: owner, Home: filepath.Join(root, "brain"), Workspace: selection.Workspace, WorkspaceID: strings.Repeat("6", 32), RegistrySHA256: options.AuthorityRegistrySHA256}, "materialization": material, "code": code, "desktop": DesktopObservation{Handoff: "bundle_ready", InstallConfirmation: "awaiting_user_install", Connection: "pending", FunctionalCheck: "pending", EvidenceClass: "not_observed"}, "preparation": map[string]any{"phase": "desktop_prepared_code_pending", "selection": selection}}
	path, err := clientRecordPath(options, selection.CarrierID)
	if err != nil {
		t.Fatal(err)
	}
	write := func(value any) {
		t.Helper()
		body, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write(wire)
	r, _, err := readClientRecord(options, selection.CarrierID)
	if err != nil {
		t.Fatalf("selected both with a complete Desktop and pending Code must be readable: %v", err)
	}
	if r.SchemaVersion != 3 || r.Code.State != "pending_qualification" || r.Materialization.CodeRoot != "" || !r.Materialization.DesktopOnly() {
		t.Fatal("partial record claimed a prepared Code packet or lost its Desktop role")
	}
	for _, lifecycle := range []bool{false, true} {
		t.Run("full selection rejects substituted non-native profile "+map[bool]string{false: "resources-only", true: "lifecycle"}[lifecycle], func(t *testing.T) {
			substitution := selection
			substitution.NativeEnforcement = false
			substitution.LifecycleContext = lifecycle
			if err := validateClientSelectionContinuity(r, substitution, r.Binding); err == nil {
				t.Fatal("ordinary client review admitted a downgrade from the saved full Code selection")
			}
		})
	}
	if err := validateClientSelectionContinuity(r, selection, r.Binding); err != nil {
		t.Fatal("unchanged desired selection for genuine Code promotion refused", err)
	}
	validBody, err := json.Marshal(wire)
	if err != nil {
		t.Fatal(err)
	}
	for _, state := range []string{"registered", "registration_pending"} {
		code.State = state
		wire["code"] = code
		write(wire)
		if _, _, err := readClientRecord(options, selection.CarrierID); err == nil {
			t.Fatalf("pending phase accepted Code state %s", state)
		}
	}
	for _, kind := range []string{"unknown phase", "retarget", "resources-only", "Code packet", "Code proof", "prepared target", "another generation", "legacy schema", "scope", "transient selection"} {
		t.Run(kind, func(t *testing.T) {
			var bad map[string]any
			if err := json.Unmarshal(validBody, &bad); err != nil {
				t.Fatal(err)
			}
			preparation := bad["preparation"].(map[string]any)
			desired := preparation["selection"].(map[string]any)
			prepared := bad["materialization"].(map[string]any)
			switch kind {
			case "unknown phase":
				preparation["phase"] = "ready"
			case "retarget":
				desired["target"] = "desktop"
			case "resources-only":
				desired["native_enforcement"] = false
			case "Code packet":
				prepared["code_root"] = selection.CodeRoot
			case "Code proof":
				prepared["native_enforcement"] = true
			case "prepared target":
				prepared["target"] = ""
			case "another generation":
				prepared["desktop_path"] = selection.DesktopPath
			case "legacy schema":
				bad["schema_version"] = 2
			case "scope":
				desired["code_scope"] = "all"
			case "transient selection":
				desired["reconcile"] = true
			}
			write(bad)
			if _, _, err := readClientRecord(options, selection.CarrierID); err == nil {
				t.Fatal("malformed preparation or invented Code authority was accepted")
			}
		})
	}
	if err := os.WriteFile(path, validBody, 0600); err != nil {
		t.Fatal(err)
	}
	bound := ClientOptions{SeededOperationOptions: options, Request: selection}
	expected := clientDigest(validBody)
	if err := requirePreviousClientRecord(bound, expected); err != nil {
		t.Fatal("unchanged predecessor refused", err)
	}
	if err := requirePreviousClientRecord(bound, ""); err == nil {
		t.Fatal("existing record accepted as absent")
	}
	var changed map[string]any
	if err := json.Unmarshal(validBody, &changed); err != nil {
		t.Fatal(err)
	}
	changed["desktop"].(map[string]any)["handoff"] = "open_requested"
	write(changed)
	if err := requirePreviousClientRecord(bound, expected); err == nil {
		t.Fatal("changed predecessor accepted before publication")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := requirePreviousClientRecord(bound, expected); err == nil {
		t.Fatal("removed predecessor accepted before publication")
	}
	if err := requirePreviousClientRecord(bound, ""); err != nil {
		t.Fatal("unchanged initial absence refused", err)
	}
}
