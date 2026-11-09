package packaging_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func nativePacketOptions(t *testing.T) packaging.Options {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("owner admission unavailable")
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prod, owner, ws := filepath.Join(root, "product"), filepath.Join(root, "owner"), filepath.Join(root, "workspace")
	for _, p := range []string{prod, owner} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	lookup := func(string) (string, bool) { return "", false }
	h, err := datahome.InitializeFresh(datahome.Options{ProductRoot: prod, UserHome: owner, LookupEnv: lookup})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: ws, DataRoot: filepath.Join(h.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	server, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return packaging.Options{Destination: filepath.Join(root, "plugin"), Server: server, Version: "0.2.0", Binding: product.Options{ProductRoot: prod, UserHome: owner, WorkspacePath: ws, LookupEnv: lookup}}
}

func TestMCPOnlyPacketHasExplicitNativeBoundaryAndInventory(t *testing.T) {
	o := nativePacketOptions(t)
	if err := packaging.BuildPlugin(o); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(o.Destination, "maestro-packet.json"))
	if err != nil {
		t.Fatalf("MCP-only packet must explain native capability and complete inventory: %v", err)
	}
	var packet struct {
		Mode   string                         `json:"mode"`
		Target struct{ OS, Arch string }      `json:"target"`
		Native struct{ State, Reason string } `json:"native"`
		Files  []struct {
			Path, SHA256 string
			Mode         uint32
		} `json:"files"`
		CanonicalAgentIDs []string                         `json:"canonical_agent_ids"`
		References        []struct{ Target, State string } `json:"references"`
	}
	if json.Unmarshal(body, &packet) != nil || packet.Mode != "mcp_only" || packet.Native.State != "unavailable" || packet.Native.Reason == "" || packet.Target.OS != runtime.GOOS || packet.Target.Arch != runtime.GOARCH {
		t.Fatalf("invalid capability/target: %s", body)
	}
	if len(packet.CanonicalAgentIDs) != 7 {
		t.Fatal("seven immutable canonical identities required")
	}
	missingHelper := false
	for _, reference := range packet.References {
		if reference.Target == "brain-index.py" && reference.State == "unavailable_in_packet" {
			missingHelper = true
		}
	}
	if !missingHelper {
		t.Fatal("known absent legacy helper must be individually unavailable, not hidden execution parity")
	}
	for _, p := range []string{"hooks"} {
		if _, err := os.Lstat(filepath.Join(o.Destination, p)); !os.IsNotExist(err) {
			t.Fatalf("unqualified packet activated %s", p)
		}
	}
	inventoryBody, err := os.ReadFile(filepath.Join(o.Destination, packaging.InventoryName))
	if err != nil || json.Unmarshal(inventoryBody, &packet) != nil {
		t.Fatal("closed file inventory unavailable")
	}
	seen := map[string]bool{}
	for _, f := range packet.Files {
		if seen[f.Path] || filepath.IsAbs(f.Path) {
			t.Fatal("duplicate/absolute packet path")
		}
		seen[f.Path] = true
		b, err := os.ReadFile(filepath.Join(o.Destination, filepath.FromSlash(f.Path)))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != f.SHA256 {
			t.Fatalf("hash differs: %s", f.Path)
		}
		info, err := os.Stat(filepath.Join(o.Destination, filepath.FromSlash(f.Path)))
		if err != nil || uint32(info.Mode().Perm()) != f.Mode {
			t.Fatalf("mode differs: %s", f.Path)
		}
	}
	assets, err := baseagents.Files()
	if err != nil {
		t.Fatal(err)
	}
	for p, want := range assets {
		got, err := os.ReadFile(filepath.Join(o.Destination, "managed", "agents", filepath.FromSlash(p)))
		if err != nil || string(got) != string(want) {
			t.Fatalf("canonical agent asset missing/changed: %s", p)
		}
	}
	if !seen["scripts/maestro-mcp"] || !seen["skills/operator/SKILL.md"] || !seen["skills/resume/SKILL.md"] {
		t.Fatal("selected executable/aliases absent from inventory")
	}
	yoda, err := os.ReadFile(filepath.Join(o.Destination, "skills/yoda/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(yoda, []byte("subagent_type: \"maestro-product:yoda\"")) {
		t.Fatal("actual Agent invocation lacks exact product namespace")
	}
}
