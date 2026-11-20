package packaging

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestCodePilotBindsExplicitChannelAndCanonicalPacket(t *testing.T) {
	in := nativeFixture(t)
	beforeBinding, err := bindingFingerprint(in.Packet.Binding)
	if err != nil {
		t.Fatal(err)
	}
	beforeSelection, err := nativeSelectionFingerprint(in.Packet.Binding)
	if err != nil {
		t.Fatal(err)
	}
	beforeProfile, err := profileFingerprint(in.Packet)
	if err != nil {
		t.Fatal(err)
	}
	in.Packet.CodePilot = true
	in.Packet.Binding.OwnerReviewChannel = "local-browser"
	if err := BuildPlugin(in.Packet); err != nil {
		t.Fatal(err)
	}
	status, err := ValidatePacket(in.Packet.Destination)
	if err != nil || status.CodePilot == nil || status.CodePilot.SelectionSHA256 != beforeSelection || status.NativeMode != "mcp_only" {
		t.Fatal(status, err)
	}
	o := in.Packet.Binding
	if err := status.CodePilot.ValidateSelection(o, true); err != nil {
		t.Fatal(err)
	}
	if v, e := bindingFingerprint(o); e != nil || v != beforeBinding {
		t.Fatal("native binding fingerprint changed", e)
	}
	if v, e := nativeSelectionFingerprint(o); e != nil || v != beforeSelection {
		t.Fatal("native selection changed", e)
	}
	if v, e := profileFingerprint(in.Packet); e != nil || v != beforeProfile {
		t.Fatal("native hook fingerprint changed", e)
	}
	o.OwnerReviewChannel = ""
	if err := status.CodePilot.ValidateSelection(o, true); err == nil {
		t.Fatal("serve omitted selected channel")
	}
	if err := status.CodePilot.ValidateSelection(o, false); err != nil {
		t.Fatal("hooks must retain original no-review arguments", err)
	}
	o.WorkspacePath += "-foreign"
	if err := status.CodePilot.ValidateSelection(o, false); err == nil {
		t.Fatal("hook workspace substitution admitted")
	}
	var config struct {
		Servers map[string]struct {
			Args []string `json:"args"`
		} `json:"mcpServers"`
	}
	raw, err := os.ReadFile(filepath.Join(in.Packet.Destination, ".mcp.json"))
	if err != nil || json.Unmarshal(raw, &config) != nil {
		t.Fatal(err)
	}
	args := config.Servers["maestro"].Args
	if len(args) != 10 || args[8] != "--owner-review-channel" || args[9] != "local-browser" || args[5] != o.UserHome || args[7] != in.Packet.Binding.WorkspacePath {
		t.Fatal("canonical Code selection missing", args)
	}
	readme, err := os.ReadFile(filepath.Join(in.Packet.Destination, "README-PACKET.md"))
	if err != nil || !strings.Contains(string(readme), "even when Code advertises native forms") {
		t.Fatal("channel not disclosed", err)
	}
	// Rehashing a substituted launch option cannot change the canonical profile.
	raw = []byte(strings.Replace(string(raw), "local-browser", "other-channel", 1))
	if err := os.WriteFile(filepath.Join(in.Packet.Destination, ".mcp.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
	var inventory packetInventory
	p := filepath.Join(in.Packet.Destination, InventoryName)
	body, err := os.ReadFile(p)
	if err != nil || json.Unmarshal(body, &inventory) != nil {
		t.Fatal(err)
	}
	for i := range inventory.Files {
		if inventory.Files[i].Path == ".mcp.json" {
			inventory.Files[i].SHA256 = digest(raw)
			inventory.Files[i].Bytes = int64(len(raw))
		}
	}
	body, err = json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, append(body, '\n'), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidatePacket(in.Packet.Destination); err == nil {
		t.Fatal("rehashed channel substitution admitted")
	}
}

func TestCodePilotRefusesOtherPacketAuthorities(t *testing.T) {
	for _, kind := range []string{"default", "desktop", "stable", "installed-manifest", "installed-client", "enrolled", "missing-channel", "unknown-channel", "unprepared-versioned", "unprepared-registry"} {
		t.Run(kind, func(t *testing.T) {
			in := nativeFixture(t)
			in.Packet.CodePilot = true
			in.Packet.Binding.OwnerReviewChannel = "local-browser"
			switch kind {
			case "default":
				in.Packet.CodePilot = false
			case "desktop":
				in.Packet.DesktopPilot = true
			case "stable":
				in.Packet.Carrier = &StableCarrier{}
			case "installed-manifest":
				in.Packet.Binding.InstalledManifestSHA256 = strings.Repeat("a", 64)
			case "installed-client":
				in.Packet.Binding.InstalledClientInputsSHA256 = strings.Repeat("a", 64)
			case "enrolled":
				if err := os.Mkdir(filepath.Join(in.Packet.Binding.UserHome, ".maestro-install"), 0700); err != nil {
					t.Fatal(err)
				}
			case "missing-channel":
				in.Packet.Binding.OwnerReviewChannel = ""
			case "unknown-channel":
				in.Packet.Binding.OwnerReviewChannel = "model-approved"
			case "unprepared-versioned":
				in.Packet.Binding.CLIVersion = in.Packet.Version
			case "unprepared-registry":
				in.Packet.Binding.AuthorityRegistryPath = "registry.json"
				in.Packet.Binding.AuthorityRegistrySHA256 = strings.Repeat("a", 64)
			}
			if err := BuildPlugin(in.Packet); err == nil {
				t.Fatal("nonexperimental/unprepared authority admitted")
			}
			assertNoNativeEffects(t, in)
		})
	}
	in := nativeFixture(t)
	in.Packet.CodePilot = true
	in.Packet.Binding.OwnerReviewChannel = "local-browser"
	in.Packet.Destination += ".mcpb"
	if err := BuildExtension(in.Packet); err == nil {
		t.Fatal("Code option changed Desktop carrier")
	}
}

func TestCodePilotNativeReviewDisclosesChannelWithoutChangingQualification(t *testing.T) {
	in := nativeFixture(t)
	run := inspectionRunner(t, true, nil)
	ordinary, err := reviewNative(context.Background(), in, run)
	if err != nil {
		t.Fatal(err)
	}
	in.Packet.CodePilot = true
	in.Packet.Binding.OwnerReviewChannel = "local-browser"
	reviewed, err := reviewNative(context.Background(), in, run)
	if err != nil || reviewed.Plan.CodePilot == nil || reviewed.Plan.CodePilot.OwnerReviewChannel != "local-browser" {
		t.Fatal(reviewed, err)
	}
	if reviewed.ReviewDigest == ordinary.ReviewDigest || reviewed.Plan.BindingSHA256 != ordinary.Plan.BindingSHA256 || reviewed.Plan.ProfileSHA256 != ordinary.Plan.ProfileSHA256 {
		t.Fatal("channel not separately bound or native fingerprints changed")
	}
	if _, err := qualifyAndBuildNative(context.Background(), in, ordinary.ReviewDigest, true, run, "engineering_fixture"); err == nil {
		t.Fatal("old native review authorized new channel")
	}
	assertNoNativeEffects(t, in)
}

func TestCodePilotCompiledServerRequiresExactStartupChannel(t *testing.T) {
	in := nativeFixture(t)
	bin := filepath.Join(filepath.Dir(in.Packet.Destination), "maestro-mcp")
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/maestro-mcp").CombinedOutput(); err != nil {
		t.Fatalf("build: %s %v", out, err)
	}
	in.Packet.Server = bin
	in.Packet.CodePilot = true
	in.Packet.Binding.OwnerReviewChannel = "local-browser"
	if err := BuildPlugin(in.Packet); err != nil {
		t.Fatal(err)
	}
	args := []string{"serve", "--packet", "--product-root", in.Packet.Destination, "--user-home", in.Packet.Binding.UserHome, "--workspace", in.Packet.Binding.WorkspacePath}
	_, target, err := readNativeBinary(bin)
	if err != nil {
		t.Fatal(err)
	}
	selected := filepath.Join(in.Packet.Destination, "scripts", binaryName(target.Platform()))
	cmd := exec.Command(selected, append(append([]string{}, args...), "--owner-review-channel", "local-browser")...)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "code-pilot-startup-test", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
	if err != nil {
		t.Fatal("exact Code startup rejected", err)
	}
	out, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "maestro_context", Arguments: map[string]any{}})
	if err != nil || out.IsError {
		t.Fatal("bound read-only context unavailable", out, err)
	}
	cs.Close()
	for _, extra := range [][]string{nil, {"--owner-review-channel", "other-channel"}} {
		p := exec.CommandContext(ctx, selected, append(append([]string{}, args...), extra...)...)
		if out, err := p.CombinedOutput(); err == nil {
			t.Fatalf("changed/omitted startup channel admitted: %s", out)
		}
	}
}

func TestEngineeringCodeCarrierUsesExistingRuntimeIdentity(t *testing.T) {
	root := t.TempDir()
	b := EngineeringRuntimeBinding{Profile: EngineeringRuntimeProfile, ClientVersion: "0.1.43", RegistrySHA256: strings.Repeat("a", 64), ManifestSHA256: strings.Repeat("b", 64), PackSHA256: strings.Repeat("c", 64), ServerSHA256: strings.Repeat("d", 64), Target: Target{OS: "darwin", Arch: "arm64"}, Issuer: "maestro-engineering-pilot-20261116", KeyID: "pilot-20261116", ExpiresAt: "2026-11-30T00:00:00Z"}
	status := PacketStatus{Version: b.ClientVersion, RegistrySHA256: b.RegistrySHA256, ServerSHA256: b.ServerSHA256, Target: b.Target, NativeMode: "native", CodePilot: &CodePilotBinding{Runtime: &b, UserHome: filepath.Join(root, "owner")}}
	server := filepath.Join(root, "scripts", "maestro-mcp")
	if err := ValidateEngineeringCodePacketSelection(root, status, server, b.ServerSHA256, b.ClientVersion, b.RegistrySHA256); err != nil {
		t.Fatal("versioned exact Code constructor rejected", err)
	}
	for _, kind := range []string{"path", "image", "version", "registry", "enrolled"} {
		t.Run(kind, func(t *testing.T) {
			path, image, version, registry := server, b.ServerSHA256, b.ClientVersion, b.RegistrySHA256
			switch kind {
			case "path":
				path = filepath.Join(root, "server", "maestro-mcp")
			case "image":
				image = strings.Repeat("e", 64)
			case "version":
				version = ""
			case "registry":
				registry = strings.Repeat("e", 64)
			case "enrolled":
				if err := os.MkdirAll(filepath.Join(status.CodePilot.UserHome, ".maestro-install"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if err := ValidateEngineeringCodePacketSelection(root, status, path, image, version, registry); err == nil {
				t.Fatal("different runtime constructor admitted")
			}
		})
	}
}

func TestCodePilotNativeCandidateKeepsOriginalHookVectors(t *testing.T) {
	in := nativeFixture(t)
	in.Packet.CodePilot = true
	in.Packet.Binding.OwnerReviewChannel = "local-browser"
	selection, err := nativeSelectionFingerprint(in.Packet.Binding)
	if err != nil {
		t.Fatal(err)
	}
	_, target, err := readNativeBinary(in.Packet.Server)
	if err != nil {
		t.Fatal(err)
	}
	original, err := nativeProfilePayload(target.Platform(), selection)
	if err != nil {
		t.Fatal(err)
	}
	c := NativeCapability{State: "qualification_candidate", Profile: nativeProfile, EvidenceClass: "unqualified_actual_host_probe", VendorProvenance: "operator_selected_installation"}
	if err := buildNativeCandidate(in.Packet, c, nil); err != nil {
		t.Fatal(err)
	}
	status, err := ValidatePacket(in.Packet.Destination)
	if err != nil || status.CodePilot == nil || status.NativeMode != "qualification_candidate" || status.NativeBindingSHA256 != selection {
		t.Fatal(status, err)
	}
	body, err := os.ReadFile(filepath.Join(in.Packet.Destination, "hooks", "hooks.json"))
	if err != nil || string(body) != string(original["hooks/hooks.json"]) {
		t.Fatal("hook vector changed", err)
	}
	body, err = os.ReadFile(filepath.Join(in.Packet.Destination, ".claude-plugin", "plugin.json"))
	var plugin struct {
		UserConfig map[string]any `json:"userConfig"`
	}
	if err != nil || json.Unmarshal(body, &plugin) != nil || plugin.UserConfig["user_home"] == nil || plugin.UserConfig["workspace"] == nil {
		t.Fatal("native hook user configuration removed", err)
	}
}
