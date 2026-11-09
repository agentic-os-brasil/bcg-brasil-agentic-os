package packaging_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStableInspectorRejectsRelabelledLegacyLaunch(t *testing.T) {
	o := nativePacketOptions(t)
	registry := []byte(`{"schema_version":1,"product":"maestro","authorities":[{"issuer":"engineering-fixture","key_id":"public-only","algorithm":"ed25519","public_key":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=","status":"active","valid_from":"2020-01-01T00:00:00Z","valid_until":"2100-01-01T00:00:00Z"}]}`)
	o.Binding.AuthorityRegistryPath = filepath.Join(filepath.Dir(o.Destination), "public-registry.json")
	o.Binding.AuthorityRegistrySHA256 = fmt.Sprintf("%x", sha256.Sum256(registry))
	if err := os.WriteFile(o.Binding.AuthorityRegistryPath, registry, 0600); err != nil {
		t.Fatal(err)
	}
	if err := packaging.BuildPlugin(o); err != nil {
		t.Fatal(err)
	}
	i, err := product.Identity("0.2.0", ">=0.1.0 <1.0.0", o.Binding.AuthorityRegistrySHA256)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(o.Destination, packaging.InventoryName)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var inventory map[string]any
	if err = json.Unmarshal(body, &inventory); err != nil {
		t.Fatal(err)
	}
	server, err := os.ReadFile(o.Server)
	if err != nil {
		t.Fatal(err)
	}
	inventory["schema_version"] = 2
	inventory["stable"] = map[string]any{
		"carrier":      map[string]string{"profile": "stable-local-v1", "protocol": "maestro-stable-carrier-v1", "carrier_id": "0123456789abcdef0123456789abcdef", "carrier_version": "1.0.0"},
		"managed_root": o.Binding.ProductRoot, "user_home": o.Binding.UserHome, "workspace": o.Binding.WorkspacePath,
		"bootstrap_server_sha256": fmt.Sprintf("%x", sha256.Sum256(server)), "bootstrap_embedded_inputs_sha256": i.EmbeddedInputsSHA256,
	}
	body, err = json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	c := packaging.StableCarrier{Profile: "stable-local-v1", Protocol: "maestro-stable-carrier-v1", ID: "0123456789abcdef0123456789abcdef", Version: "1.0.0"}
	if err = packaging.InspectStablePlugin(o.Destination, c, i, fmt.Sprintf("%x", sha256.Sum256(server))); err == nil {
		t.Fatal("legacy direct-serve packet relabelled as stable carrier")
	}
	// This explicit wire fixture proves inspector behavior only; it grants no
	// installation/native authority and is not the real signed-generation proof.
	config := map[string]any{"mcpServers": map[string]any{"maestro": map[string]any{"type": "stdio", "command": filepath.Join(o.Binding.ProductRoot, "maestro-install"), "args": []string{"stable-launch", "--product-root", o.Binding.ProductRoot, "--user-home", o.Binding.UserHome, "--workspace", o.Binding.WorkspacePath, "--carrier-id", c.ID}}}}
	configBody, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	configBody = append(configBody, '\n')
	pluginPath := filepath.Join(o.Destination, ".claude-plugin", "plugin.json")
	pluginBody, err := os.ReadFile(pluginPath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest map[string]any
	if err = json.Unmarshal(pluginBody, &manifest); err != nil {
		t.Fatal(err)
	}
	delete(manifest, "userConfig")
	pluginBody, err = json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	pluginBody = append(pluginBody, '\n')
	readme := []byte("# Participant-local Maestro carrier\n\nThis packet belongs to one reviewed local client slot. Its fixed installed launcher revalidates the independently admitted source, current materialization and selected workspace before starting the active server. Copied metadata grants no authority. Use the exact management launcher for review/reconciliation; no native loading or production signing is implied.\n")
	replacements := map[string][]byte{".mcp.json": configBody, ".claude-plugin/plugin.json": pluginBody, "README-PACKET.md": readme}
	for name, value := range replacements {
		if err = os.WriteFile(filepath.Join(o.Destination, filepath.FromSlash(name)), value, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range inventory["files"].([]any) {
		f := entry.(map[string]any)
		if value, ok := replacements[f["path"].(string)]; ok {
			f["sha256"] = fmt.Sprintf("%x", sha256.Sum256(value))
			f["bytes"] = len(value)
		}
	}
	body, err = json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, body, 0644); err != nil {
		t.Fatal(err)
	}
	if err = packaging.InspectStablePlugin(o.Destination, c, i, fmt.Sprintf("%x", sha256.Sum256(server))); err != nil {
		t.Fatalf("complete explicit stable inspector fixture: %v", err)
	}
	if err = packaging.InspectStablePlugin(o.Destination, c, i, strings.Repeat("f", 64)); err == nil {
		t.Fatal("foreign current server accepted")
	}
	leaf := filepath.Join(o.Destination, ".mcp.json")
	foreign := filepath.Join(filepath.Dir(o.Destination), "aliased-config")
	if err = os.Link(leaf, foreign); err != nil {
		t.Fatal(err)
	}
	if err = packaging.InspectStablePlugin(o.Destination, c, i, fmt.Sprintf("%x", sha256.Sum256(server))); err == nil {
		t.Fatal("multiply linked packet accepted")
	}
}

func TestStablePacketRequiresCurrentSignedInstallation(t *testing.T) {
	o := nativePacketOptions(t)
	o.Carrier = &packaging.StableCarrier{Profile: "stable-local-v1", Protocol: "maestro-stable-carrier-v1", ID: "0123456789abcdef0123456789abcdef", Version: "1.0.0"}
	if err := packaging.BuildPlugin(o); err == nil {
		t.Fatal("stable packet admitted without current signed installation/source")
	}
}
