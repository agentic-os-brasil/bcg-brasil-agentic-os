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
