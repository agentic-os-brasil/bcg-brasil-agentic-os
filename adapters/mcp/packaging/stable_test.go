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
	if err := packaging.BuildPlugin(o); err != nil {
		t.Fatal(err)
	}
	i, err := product.Identity("0.2.0", ">=0.1.0 <1.0.0", strings.Repeat("b", 64))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(o.Destination, "maestro-packet.json")
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
	inventory["profile"] = "stable-local-v1"
	inventory["protocol"] = "maestro-stable-carrier-v1"
	inventory["carrier_id"] = "0123456789abcdef0123456789abcdef"
	inventory["carrier_version"] = "1.0.0"
	inventory["bootstrap_server_sha256"] = fmt.Sprintf("%x", sha256.Sum256(server))
	inventory["bootstrap_embedded_inputs_sha256"] = i.EmbeddedInputsSHA256
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
	configBody, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(o.Destination, ".mcp.json"), configBody, 0600); err != nil {
		t.Fatal(err)
	}
	for _, entry := range inventory["files"].([]any) {
		f := entry.(map[string]any)
		if f["path"] == ".mcp.json" {
			f["sha256"] = fmt.Sprintf("%x", sha256.Sum256(configBody))
		}
	}
	body, err = json.Marshal(inventory)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(path, body, 0600); err != nil {
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
