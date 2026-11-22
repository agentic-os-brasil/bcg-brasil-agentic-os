package product

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestInstalledOwnerReviewAcceptsCanonicalDesktopPreparationOnly(t *testing.T) {
	root := t.TempDir()
	selection := installer.ClientRequest{VersionedSlots: true, NativeEnforcement: true, GenerationID: strings.Repeat("b", 32), CarrierID: strings.Repeat("c", 32), Workspace: filepath.Join(root, "workspace"), CodeRoot: filepath.Join(root, "code"), DesktopPath: filepath.Join(root, "desktop.mcpb"), CodeExecutable: filepath.Join(root, "claude"), CodeScope: "user", OwnerReviewChannel: "local-browser"}
	projection, err := installer.DesktopPreparationOptions(installer.ClientOptions{Request: selection})
	if err != nil {
		t.Fatal(err)
	}
	material, err := installer.ClientGenerationPaths(projection.Request, strings.Repeat("d", 64))
	if err != nil {
		t.Fatal(err)
	}
	material.OwnerReviewChannel = selection.OwnerReviewChannel
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
	material.DesktopMethods = &installer.ClientMethodFrontend{BootstrapProtocol: "maestro-live-method-bootstrap-v1", Archive: filepath.Join(installer.ClientMethodsRoot(material), "maestro-desktop-methods-0.1.0.zip"), SHA256: strings.Repeat("5", 64), Version: material.Release, Files: 2}
	r := installer.ClientRecord{SchemaVersion: 3, Profile: installer.StableClientProfile, Protocol: installer.StableClientProtocol, CarrierVersion: installer.StableCarrierVersion, CarrierID: selection.CarrierID, Binding: installer.ClientBinding{Home: filepath.Join(root, "brain"), Workspace: selection.Workspace, WorkspaceID: strings.Repeat("6", 32)}, Materialization: material, Code: installer.CodeObservation{Executable: selection.CodeExecutable, Marketplace: "maestro-local-" + selection.CarrierID, PluginID: "maestro-product@maestro-local-" + selection.CarrierID, Scope: selection.CodeScope, State: installer.ClientCodeQualificationPending, EvidenceClass: "not_observed"}, Preparation: &installer.ClientPreparation{Phase: installer.ClientPreparationDesktopReady, Selection: selection}}
	if _, err := installer.DesktopPreparationRecord(r); err != nil {
		t.Fatal("fixture is not a valid canonical partial shape", err)
	}
	check := func(record installer.ClientRecord) error {
		b := &Bound{options: Options{WorkspacePath: selection.Workspace, OwnerReviewChannel: selection.OwnerReviewChannel}}
		return b.validateInstalledOwnerReviewSnapshot(context.Background(), r.Binding.Home, r.Binding.WorkspaceID, installer.ClientStatus{State: installer.ClientPartialPreparation, Record: &record})
	}
	if err := check(r); err != nil {
		t.Fatal("canonical complete Desktop with pending Code refused", err)
	}
	for _, kind := range []string{"unknown phase", "different prepared role", "Code packet", "Code proof", "Code registered", "selected Desktop", "missing methods", "home", "workspace", "channel"} {
		t.Run(kind, func(t *testing.T) {
			body, err := json.Marshal(r)
			if err != nil {
				t.Fatal(err)
			}
			var bad installer.ClientRecord
			if err := json.Unmarshal(body, &bad); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "unknown phase":
				bad.Preparation.Phase = "ready"
			case "different prepared role":
				bad.Materialization.Target = ""
			case "Code packet":
				bad.Materialization.CodeRoot = selection.CodeRoot
			case "Code proof":
				bad.Materialization.NativeEnforcement = true
			case "Code registered":
				bad.Code.State = "registered"
			case "selected Desktop":
				bad.Preparation.Selection.Target = "desktop"
			case "missing methods":
				bad.Materialization.DesktopMethods = nil
			case "home":
				bad.Binding.Home = filepath.Join(root, "other-brain")
			case "workspace":
				bad.Binding.Workspace = filepath.Join(root, "other-workspace")
			case "channel":
				bad.Materialization.OwnerReviewChannel = "other"
			}
			if err := check(bad); err == nil {
				t.Fatal("changed or fabricated client authority accepted")
			}
		})
	}
	b := &Bound{options: Options{WorkspacePath: selection.Workspace, OwnerReviewChannel: selection.OwnerReviewChannel}}
	status := installer.ClientStatus{State: installer.ClientPartialPreparation, Record: &r}
	if err := b.validateInstalledOwnerReviewSnapshot(context.Background(), r.Binding.Home, r.Binding.WorkspaceID, status); err != nil {
		t.Fatal(err)
	}
	r.Desktop.Handoff = "open_requested"
	if err := b.validateInstalledOwnerReviewSnapshot(context.Background(), r.Binding.Home, r.Binding.WorkspaceID, status); err == nil {
		t.Fatal("canonical snapshot changed after receiver pin")
	}
}

func TestManagementHandoffRequiresFixedAdmittedContextAndRejectsModelPaths(t *testing.T) {
	b := localReviewFixture(t, "")
	opened := 0
	b.managementOpen = func(context.Context, string) error { opened++; return nil }
	client := localReviewSession(t, b, nil)
	call := func(fields map[string]any) {
		t.Helper()
		out, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: fields})
		if err != nil {
			t.Fatal(err)
		}
		if !out.IsError || opened != 0 {
			t.Fatal("unqualified or model-selected handoff opened", out, opened)
		}
	}
	call(map[string]any{"operation": "installation.update_handoff"})
	for _, field := range []string{"path", "command", "source", "carrier_id", "approved", "review_digest"} {
		call(map[string]any{"operation": "installation.update_handoff", field: "model-selection"})
	}
	b.options.ManagementCarrierID = strings.Repeat("a", 32)
	b.options.ManagementInstallerVersion = "0.3.7"
	// No installer/native admission exists in this ordinary backend fixture.
	// A plausible trusted-context shape still cannot bypass the real owner.
	call(map[string]any{"operation": "installation.update_handoff"})
	b.options.ManagementCarrierID = strings.Repeat("a", 32) + "," + strings.Repeat("b", 32)
	call(map[string]any{"operation": "installation.update_handoff"})
}

func TestManagementHandoffRetainsAcknowledgedRequestWhenRoleExpires(t *testing.T) {
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC)
	registry := []byte(fmt.Sprintf(`{"schema_version":1,"product":"maestro","authorities":[{"issuer":"maestro-engineering-pilot-20261117","key_id":"pilot-20261117","algorithm":"ed25519","public_key":"%s","status":"active","valid_from":"2026-11-17T00:00:00Z","valid_until":"2026-11-30T00:00:00Z"}]}`, base64.StdEncoding.EncodeToString(public)))
	role := installer.ExperimentalSeedRecord{SchemaVersion: 1, Product: "maestro", Purpose: installer.ExperimentalSeedPurpose, Protocol: installer.ExperimentalSeedProtocol, Profile: installer.ExperimentalSeedProfile, RegistrySHA256: fmt.Sprintf("%x", sha256.Sum256(registry)), InstallerVersion: "0.1.51", InstallerSHA256: strings.Repeat("a", 64), TargetOS: "darwin", TargetArch: "arm64", Source: releasecontract.SourceTuple{SourceID: "team-pilot", Channel: "canary", OS: "darwin", Arch: "arm64"}, Issuer: "maestro-engineering-pilot-20261117", KeyID: "pilot-20261117", NotBefore: "2026-11-17T00:00:00Z", ExpiresAt: "2026-11-18T00:00:00Z"}
	for _, version := range []string{"0.1.51", "0.1.52"} {
		role.Releases = append(role.Releases, installer.ExperimentalSeedRelease{Release: version, ManifestSHA256: fmt.Sprintf("%x", sha256.Sum256([]byte(version))), ClientInputsSHA256: strings.Repeat("b", 64), ServerSHA256: strings.Repeat("c", 64), BundleSHA256: strings.Repeat("d", 64), RuntimeSHA256: strings.Repeat("e", 64)})
	}
	body, _ := json.Marshal(role)
	verified, err := installer.VerifyExperimentalSeedAdmission(body, ed25519.Sign(private, body), registry, role.RegistrySHA256, func() time.Time { return now })
	if err != nil {
		t.Fatal(err)
	}
	prepared := installer.ClientManagementHandoff{State: "handoff_prepared", Launcher: "/fixed/admitted/Update Maestro.command", Activation: "not_requested_or_approved"}
	opened := 0
	value, err := requestManagementOpen(context.Background(), prepared, func(context.Context, string) error {
		opened++
		now = time.Date(2026, 11, 18, 0, 0, 1, 0, time.UTC)
		return nil
	})
	if err != nil || opened != 1 {
		t.Fatal("OS acknowledgement missing", err, opened)
	}
	roleErr := verified.Revalidate()
	if roleErr == nil {
		t.Fatal("real role expiry was not observed")
	}
	out := managementHandoffPostValidation(value, roleErr)
	var encoded []byte
	for _, content := range out.Content {
		if text, ok := content.(*mcp.TextContent); ok {
			encoded = []byte(text.Text)
			break
		}
	}
	var metadata map[string]string
	if err := json.Unmarshal(encoded, &metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["state"] != "handoff_requested" || metadata["continuation"] != "pending_new_valid_review" || metadata["role_state"] != "changed_after_request" || out.IsError {
		t.Fatal("acknowledged effect was erased or made retryable", string(encoded), out.IsError)
	}
	if opened != 1 {
		t.Fatal("handoff was repeated", opened)
	}
}

func TestManagementOpenWitnessReportsOnlyRequestedHandoff(t *testing.T) {
	prepared := installer.ClientManagementHandoff{State: "handoff_prepared", Launcher: "/fixed/admitted/Update Maestro.command", Activation: "not_requested_or_approved"}
	opened := 0
	value, err := requestManagementOpen(context.Background(), prepared, func(_ context.Context, path string) error {
		opened++
		if path != prepared.Launcher {
			t.Fatal("fixed manager changed")
		}
		return nil
	})
	if err != nil || opened != 1 {
		t.Fatal(value, err)
	}
	result := value.(map[string]string)
	if result["state"] != "handoff_requested" || result["activation"] != "pending_native_installer_review" || result["client_loading"] != "pending" {
		t.Fatal("OS acknowledgement became activation", result)
	}
	if _, err := requestManagementOpen(context.Background(), prepared, func(context.Context, string) error { return errors.New("normal host opener unavailable") }); err == nil {
		t.Fatal("unavailable host opener became successful")
	}
	prepared.State = "unqualified"
	if _, err := requestManagementOpen(context.Background(), prepared, func(context.Context, string) error { t.Fatal("unqualified handoff opened"); return nil }); err == nil {
		t.Fatal("unqualified handoff accepted")
	}
}
