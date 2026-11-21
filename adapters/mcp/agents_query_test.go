package product_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAgentInstructionsInitializeSDK(t *testing.T) {
	f := newMethodQueryFixture(t)
	client := f.connect(t, "")
	assertAgentInitialization(t, client)
}

func assertAgentInitialization(t *testing.T, client *mcp.ClientSession) {
	t.Helper()
	instructions := client.InitializeResult().Instructions
	for _, hint := range []string{"maestro_context", "agents.read", `"agent_id":"maestro"`, "next_offset", "owner.context", "system", "current user task", "display", "authority"} {
		if !strings.Contains(instructions, hint) {
			t.Errorf("initialize instructions missing %q: %q", hint, instructions)
		}
	}
	tools, err := client.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools.Tools) != 3 {
		t.Fatalf("changed tool count: %d", len(tools.Tools))
	}
	var contextResult struct {
		Agents struct {
			Read  string `json:"read_operation"`
			ID    string `json:"agent_id"`
			Limit int    `json:"max_page_bytes"`
			Mode  string `json:"mode"`
		} `json:"agents"`
	}
	methodQueryDecode(t, client, "maestro_context", map[string]any{}, &contextResult)
	if contextResult.Agents.Read != "agents.read" || contextResult.Agents.ID != "maestro" || contextResult.Agents.Limit != 4096 || contextResult.Agents.Mode != "roles_not_authority" {
		t.Fatalf("missing canonical role discovery: %+v", contextResult)
	}
}

func TestAgentInstructionsQuerySDK(t *testing.T) {
	f := newMethodQueryFixture(t)
	client := f.connect(t, "")
	before := methodTreeSnapshot(t, f.root)
	assertAgentQueries(t, client)
	if !reflect.DeepEqual(before, methodTreeSnapshot(t, f.root)) {
		t.Fatal("canonical role queries changed local state")
	}
}

func assertAgentQueries(t *testing.T, client *mcp.ClientSession) {
	t.Helper()
	ids := []string{"case-agent", "client-account-agent", "darwin", "gamma-guardian", "maestro", "pa-expert", "repository-spec-reviewer", "repository-standards-reviewer", "yoda"}
	for _, id := range ids {
		expected, err := baseagents.Definition(id)
		if err != nil {
			t.Fatal(err)
		}
		uri := "maestro://agents/" + id + "/AGENT.md"
		resource, err := client.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: uri})
		if err != nil || len(resource.Contents) != 1 {
			t.Fatalf("canonical resource %s: %v", id, err)
		}
		var body strings.Builder
		for offset := 0; ; {
			var page struct {
				ID     string `json:"agent_id"`
				URI    string `json:"resource_uri"`
				Text   string `json:"text"`
				Hash   string `json:"sha256"`
				Bytes  int    `json:"total_bytes"`
				Offset int    `json:"offset"`
				Next   int    `json:"next_offset"`
				More   bool   `json:"has_more"`
				Mode   string `json:"mode"`
			}
			methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "agents.read", "agent_id": id, "offset": offset, "max_bytes": 4096}, &page)
			if page.ID != id || page.URI != uri || page.Hash != fmt.Sprintf("%x", sha256.Sum256(expected)) || page.Bytes != len(expected) || page.Offset != offset || len(page.Text) > 4096 || !utf8.ValidString(page.Text) || page.Next != offset+len(page.Text) || page.More && page.Next <= offset || page.Mode != "roles_not_authority" {
				t.Fatalf("invalid canonical role page %s at %d: %+v", id, offset, page)
			}
			body.WriteString(page.Text)
			if !page.More {
				break
			}
			offset = page.Next
		}
		if body.String() != string(expected) || body.String() != resource.Contents[0].Text {
			t.Fatalf("canonical resource/query bytes differ: %s", id)
		}
	}
	for _, args := range []map[string]any{
		{"operation": "agents.read"},
		{"operation": "agents.read", "agent_id": "unknown"},
		{"operation": "agents.read", "agent_id": "Maestro"},
		{"operation": "agents.read", "agent_id": "case_agent"},
		{"operation": "agents.read", "agent_id": "../maestro"},
		{"operation": "agents.read", "agent_id": "%6daestro"},
		{"operation": "agents.read", "agent_id": "maestro/AGENT.md"},
		{"operation": "agents.read", "agent_id": "maestro://agents/maestro/AGENT.md"},
		{"operation": "agents.read", "agent_id": "templates/case_agent"},
		{"operation": "agents.read", "agent_id": "maestro", "resource_uri": "maestro://methods/METHODS.md"},
		{"operation": "agents.read", "agent_id": "maestro", "offset": -1},
		{"operation": "agents.read", "agent_id": "maestro", "offset": 128 << 10},
		{"operation": "agents.read", "agent_id": "maestro", "max_bytes": 0},
		{"operation": "agents.read", "agent_id": "maestro", "max_bytes": 3},
		{"operation": "agents.read", "agent_id": "maestro", "max_bytes": 4097},
		{"operation": "agents.read", "agent_id": "maestro", "agent_role": "yoda"},
	} {
		if !methodQueryCall(t, client, "maestro_query", args).IsError {
			t.Fatalf("invalid role query admitted: %v", args)
		}
	}
	canonical, err := baseagents.Definition("maestro")
	if err != nil {
		t.Fatal(err)
	}
	var final struct {
		Text string `json:"text"`
		Next int    `json:"next_offset"`
		More bool   `json:"has_more"`
	}
	methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "agents.read", "agent_id": "maestro", "offset": len(canonical)}, &final)
	if final.Text != "" || final.Next != len(canonical) || final.More {
		t.Fatal("invalid terminal role page")
	}
	var first struct {
		Text string `json:"text"`
		Next int    `json:"next_offset"`
		More bool   `json:"has_more"`
	}
	methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "agents.read", "agent_id": "maestro"}, &first)
	if len(first.Text) > 4096 || !utf8.ValidString(first.Text) || first.Next != len(first.Text) || first.More != (len(first.Text) < len(canonical)) || first.Text != string(canonical[:len(first.Text)]) {
		t.Fatal("invalid default canonical role page")
	}
	checkedUTF8 := false
	for offset := range canonical {
		if !utf8.RuneStart(canonical[offset]) {
			checkedUTF8 = true
			args := map[string]any{"operation": "agents.read", "agent_id": "maestro", "offset": offset}
			if !methodQueryCall(t, client, "maestro_query", args).IsError {
				t.Fatal("mid-rune role offset admitted")
			}
			var page struct {
				Text string `json:"text"`
				Next int    `json:"next_offset"`
			}
			methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "agents.read", "agent_id": "maestro", "offset": offset - 1, "max_bytes": 4}, &page)
			if !utf8.ValidString(page.Text) || len(page.Text) > 4 || page.Next <= offset-1 {
				t.Fatal("small UTF-8 role page invalid")
			}
			break
		}
	}
	if !checkedUTF8 {
		t.Fatal("canonical hub role lacks a UTF-8 boundary fixture")
	}
}

func TestAgentInstructionsStdioRestart(t *testing.T) {
	f := newMethodQueryFixture(t)
	binary := filepath.Join(f.root, productTestBinaryName())
	if out, err := exec.Command("go", "build", "-o", binary, "./cmd/maestro-mcp").CombinedOutput(); err != nil {
		t.Fatalf("build: %s: %v", out, err)
	}
	before := methodTreeSnapshot(t, f.root)
	for range 2 {
		client := f.connect(t, binary)
		assertAgentInitialization(t, client)
		assertAgentQueries(t, client)
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if !reflect.DeepEqual(before, methodTreeSnapshot(t, f.root)) {
		t.Fatal("restarted role discovery changed local state")
	}
}

func TestAgentInstructionsQueryRevalidatesBinding(t *testing.T) {
	f := newMethodQueryFixture(t)
	client := f.connect(t, "")
	manifest := filepath.Join(f.options.WorkspacePath, ".bcgos", "workspace.json")
	if err := os.WriteFile(manifest, []byte("{}\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before := methodTreeSnapshot(t, f.root)
	if !methodQueryCall(t, client, "maestro_query", map[string]any{"operation": "agents.read", "agent_id": "maestro"}).IsError {
		t.Fatal("stale binding served canonical role")
	}
	if !reflect.DeepEqual(before, methodTreeSnapshot(t, f.root)) {
		t.Fatal("stale role read repaired local state")
	}
}
