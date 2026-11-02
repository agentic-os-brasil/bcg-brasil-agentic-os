package product_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"testing"

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This list is an independent public contract, not a derivation of the producer.
var canonicalAgentPaths = []string{
	"activation-policy.json", "case-agent/AGENT.md", "catalog.json",
	"client-account-agent/AGENT.md", "darwin/AGENT.md", "gamma-guardian/AGENT.md",
	"maestro/AGENT.md", "pa-expert-registry.json", "pa-expert/AGENT.md",
	"templates/case_agent/AGENT.md", "templates/client_account_agent/AGENT.md",
	"templates/pa_expert/AGENT.md", "templates/quality_guardian/AGENT.md", "yoda/AGENT.md",
}

func assertCanonicalAgentResources(t *testing.T, cs *mcp.ClientSession, resources []*mcp.Resource, secret string) {
	t.Helper()
	ctx := context.Background()
	var got []string
	for _, r := range resources {
		if !strings.HasPrefix(r.URI, "maestro://agents/") {
			continue
		}
		path := strings.TrimPrefix(r.URI, "maestro://agents/")
		got = append(got, path)
		response, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: r.URI})
		if err != nil {
			t.Fatalf("agent resource %s: %v", path, err)
		}
		if len(response.Contents) != 1 {
			t.Fatalf("agent resource count %s", path)
		}
		body := response.Contents[0]
		if body.URI != r.URI || body.MIMEType != r.MIMEType || strings.Contains(body.Text, secret) || strings.Contains(body.Text, "{{") {
			t.Fatalf("invalid canonical resource %s", path)
		}
		if r.Size != int64(len(body.Text)) || r.Meta["sha256"] != fmt.Sprintf("%x", sha256.Sum256([]byte(body.Text))) {
			t.Fatalf("resource integrity metadata %s", path)
		}
		var canonical []byte
		if strings.HasPrefix(path, "templates/") {
			canonical, err = baseagents.Template(strings.Split(path, "/")[1])
		} else if strings.HasSuffix(path, "/AGENT.md") {
			canonical, err = baseagents.Definition(strings.TrimSuffix(path, "/AGENT.md"))
		} else {
			files, e := baseagents.Files()
			err = e
			canonical = files[path]
		}
		if err != nil || body.Text != string(canonical) {
			t.Fatalf("canonical bytes changed for %s: %v", path, err)
		}
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, canonicalAgentPaths) {
		t.Fatalf("agent resource inventory = %v; want %v", got, canonicalAgentPaths)
	}
	for _, uri := range []string{
		"maestro://agents/../private.md", "maestro://agents/%2e%2e/private.md",
		"maestro://agents/maestro/../../private.md", "maestro://agents/maestro/AGENT.md?owner=true",
		"maestro://agents/maestro/AGENT.md#private", "maestro://agents/brain/owner/self.md",
		"maestro://agents/templates/practice_agent/AGENT.md", "maestro://agents/unknown/AGENT.md",
	} {
		if _, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri}); err == nil {
			t.Fatalf("unsafe agent resource admitted: %s", uri)
		}
	}
}
