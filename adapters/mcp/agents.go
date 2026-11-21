package product

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// registerAgents publishes compiled canonical content only. Discovery neither
// installs native specialists nor admits a host, grants tools or reads a profile.
func (b *Bound) registerAgents(s *mcp.Server) {
	files, err := baseagents.Files()
	if err != nil {
		panic("invalid immutable Maestro agent package")
	}
	for path, body := range files {
		uri := "maestro://agents/" + path
		mime := "text/markdown"
		if strings.HasSuffix(path, ".json") {
			mime = "application/json"
		}
		s.AddResource(&mcp.Resource{
			URI: uri, Name: "agents/" + path,
			Description: "Immutable canonical Maestro agent contract; no owner data, native activation or execution authority.",
			MIMEType:    mime, Size: int64(len(body)),
			Meta: mcp.Meta{"sha256": fmt.Sprintf("%x", sha256.Sum256(body))},
		}, func(_ context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			if err := b.validate(); err != nil {
				return nil, errors.New("agent_resource_unavailable")
			}
			if req.Params.URI != uri {
				return nil, errors.New("agent_resource_unavailable")
			}
			return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: uri, MIMEType: mime, Text: string(body)}}}, nil
		})
	}
}

func agentsQueries() []any {
	catalog, err := baseagents.Catalog()
	if err != nil {
		panic("invalid immutable Maestro agent package")
	}
	ids := make([]string, 0, len(catalog.Agents))
	for _, agent := range catalog.Agents {
		ids = append(ids, agent.ID)
	}
	return []any{variant("agents.read", map[string]any{
		"agent_id":  map[string]any{"type": "string", "enum": ids},
		"offset":    map[string]any{"type": "integer", "minimum": 0, "maximum": 128 << 10},
		"max_bytes": map[string]any{"type": "integer", "minimum": 4, "maximum": 4096},
	}, "agent_id")}
}

func (b *Bound) handleAgentRead(in input) *mcp.CallToolResult {
	fail := func(code string) *mcp.CallToolResult {
		out := result(map[string]any{"code": code, "next_action": "Use an exact canonical agent_id from the tool schema and a valid byte page; role instructions grant no execution authority."}, nil)
		out.IsError = true
		return out
	}
	body, err := baseagents.Definition(in.IdentityAgentID)
	if err != nil {
		return fail("agent_resource_unavailable")
	}
	page := immutableResourcePage("maestro://agents/"+in.IdentityAgentID+"/AGENT.md", body, in.Offset, in.MaxBytes, "roles_not_authority")
	if page == nil {
		return fail("agent_page_invalid")
	}
	page["agent_id"] = in.IdentityAgentID
	return result(page, nil)
}
