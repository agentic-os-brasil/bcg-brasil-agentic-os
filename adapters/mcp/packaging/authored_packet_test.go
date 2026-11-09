package packaging_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Assert the actual generated entry point, then exercise its exact resource URI
// through the server/config distributed by this packet. A source-only recipe
// or a pointer to an unreadable URI is not an ordinary-conversation caller.
func assertAuthoredBrainOperatorResource(t *testing.T, options packaging.Options) {
	t.Helper()
	const uri = "maestro://methods/maestro-operator/resources/authored-brain.md"
	operator, err := os.ReadFile(filepath.Join(options.Destination, "skills/operator/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{uri, "brain.status", "mutation_available", "resources/read", "trusted host review"} {
		if !strings.Contains(string(operator), required) {
			t.Fatalf("generated operator lacks authored workspace entry instruction %q", required)
		}
	}
	canonical, err := os.ReadFile("../../../bundles/base/skills/maestro-operator/resources/authored-brain.md")
	if err != nil {
		t.Fatal(err)
	}
	distributed, err := os.ReadFile(filepath.Join(options.Destination, "skills/maestro-operator/resources/authored-brain.md"))
	if err != nil || string(distributed) != string(canonical) {
		t.Fatal("packet changed or truncated canonical authored recipe", err)
	}
	configBody, err := os.ReadFile(filepath.Join(options.Destination, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config struct {
		Servers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if err = json.Unmarshal(configBody, &config); err != nil {
		t.Fatal(err)
	}
	server, ok := config.Servers["maestro"]
	if !ok {
		t.Fatal("packet lacks the admitted Maestro binding")
	}
	replace := strings.NewReplacer(
		"${CLAUDE_PLUGIN_ROOT}", options.Destination,
		"${user_config.user_home}", options.Binding.UserHome,
		"${user_config.workspace}", options.Binding.WorkspacePath,
	)
	command := replace.Replace(server.Command)
	args := append([]string(nil), server.Args...)
	for i := range args {
		args[i] = replace.Replace(args[i])
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "packet-authored-recipe-proof", Version: "1"}, nil)
	session, err := client.Connect(ctx, &mcp.CommandTransport{Command: exec.CommandContext(ctx, command, args...), TerminateDuration: time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	resource, err := session.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
	if err != nil || len(resource.Contents) != 1 {
		t.Fatal("generated operator URI is not readable through the actual packet server", err)
	}
	page := resource.Contents[0]
	if page.URI != uri || page.MIMEType != "text/markdown" || page.Text != string(canonical) {
		t.Fatal("actual resource response differs from the canonical authored recipe")
	}
}
