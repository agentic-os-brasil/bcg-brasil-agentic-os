package packaging_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
	"os"
	"path/filepath"
	"testing"
)

func assertAgentPacket(t *testing.T, root string, plugin bool) {
	t.Helper()
	canonical, err := baseagents.Files()
	if err != nil {
		t.Fatal(err)
	}
	paths := []string{"activation-policy.json", "case-agent/AGENT.md", "catalog.json", "client-account-agent/AGENT.md", "darwin/AGENT.md", "gamma-guardian/AGENT.md", "maestro/AGENT.md", "pa-expert-registry.json", "pa-expert/AGENT.md", "templates/case_agent/AGENT.md", "templates/client_account_agent/AGENT.md", "templates/pa_expert/AGENT.md", "templates/quality_guardian/AGENT.md", "yoda/AGENT.md"}
	var inventory struct {
		Files []struct {
			Path, SHA256 string
			Bytes        int64
			Mode         uint32
		}
	}
	body, err := os.ReadFile(filepath.Join(root, packaging.InventoryName))
	if err != nil || json.Unmarshal(body, &inventory) != nil {
		t.Fatal("packet inventory unavailable", err)
	}
	expected := map[string][]byte{}
	for _, path := range paths {
		expected["managed/agents/"+path] = canonical[path]
	}
	native, err := claudeagents.Project(claudeagents.PluginNamespace)
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range native {
		if plugin {
			expected[agent.Path] = agent.Body
		} else if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(agent.Path))); !os.IsNotExist(err) {
			t.Fatal("MCPB carries native agent", agent.Path, err)
		}
	}
	for path, want := range expected {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
		if err != nil || string(got) != string(want) {
			t.Fatalf("canonical/projected packet agent %s unavailable or changed: %v", path, err)
		}
		found := false
		for _, entry := range inventory.Files {
			if entry.Path == path {
				found = true
				if entry.SHA256 != fmt.Sprintf("%x", sha256.Sum256(want)) || entry.Bytes != int64(len(want)) || entry.Mode != 0644 {
					t.Fatal("agent inventory provenance mismatch", path)
				}
			}
		}
		if !found {
			t.Fatal("agent missing from exact inventory", path)
		}
	}
	for _, path := range []string{"agents/maestro.md", "hooks/hooks.json"} {
		if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(path))); !os.IsNotExist(err) {
			t.Fatal("unexpected native hub/hooks", path, err)
		}
	}
}
