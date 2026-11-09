package product

import (
	"errors"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func wikiQueries() []any {
	fields := func(source bool) map[string]any {
		ref := map[string]any{"type": "string", "maxLength": 192, "pattern": `^(index\.md|log\.md|backlinks\.json|layers/(l1|l2|l3|lifetime)/index\.md|concepts/[a-f0-9]{64}\.md)$`}
		maximum := 16 << 20
		if source {
			ref["pattern"] = `^(maestro://memory/)?[a-f0-9]{64}$`
			maximum = 1 << 20
		}
		return map[string]any{
			"generation": map[string]any{"type": "string", "pattern": `^[a-f0-9]{64}$`},
			"reference":  ref,
			"offset":     map[string]any{"type": "integer", "minimum": 0, "maximum": maximum},
			"max_bytes":  map[string]any{"type": "integer", "minimum": 4, "maximum": 4096},
		}
	}
	return []any{variant("wiki.status", map[string]any{}), variant("wiki.read", fields(false), "generation", "reference"), variant("wiki.source", fields(true), "generation", "reference")}
}

func wikiResult(value any, err error) *mcp.CallToolResult {
	if err == nil {
		return result(value, nil)
	}
	code := "wiki_unavailable"
	switch {
	case errors.Is(err, processidentity.ErrChanged):
		code = "restart_required"
	case errors.Is(err, memory.ErrPrivateWikiStale):
		code = "wiki_stale"
	case errors.Is(err, memory.ErrPrivateWikiConflict):
		code = "revision_conflict"
	case errors.Is(err, memory.ErrPrivateWikiCohort):
		code = "wiki_cohort_unavailable"
	case errors.Is(err, memory.ErrPrivateWikiRetirement):
		code = "wiki_retirement_incomplete"
	}
	out := result(map[string]any{"code": code, "retryable": false}, nil)
	out.IsError = true
	return out
}

func (b *Bound) handleWiki(in input) *mcp.CallToolResult {
	e, _, err := b.memoryEngine()
	if err != nil {
		return wikiResult(nil, err)
	}
	limit := in.MaxBytes
	if limit == 0 {
		limit = 4096
	}
	var value any
	switch in.Operation {
	case "wiki.status":
		var snapshot memory.PrivateWikiMaintenanceSnapshot
		snapshot, err = e.InspectPrivateWikiMaintenance(b.workspaceID)
		if err == nil {
			value = memory.PrivateWikiStatus{State: snapshot.State, Generation: snapshot.Generation, SourceWatermark: snapshot.SourceWatermark, DenialGeneration: snapshot.DenialGeneration, ImplementationCurrent: snapshot.ImplementationCurrent, ConfigurationCurrent: snapshot.ConfigurationCurrent, Concepts: snapshot.Concepts, Files: snapshot.Files}
		}
	case "wiki.read":
		value, err = e.ReadPrivateWiki(b.workspaceID, in.Generation, in.Reference, in.Offset, limit)
	case "wiki.source":
		value, err = e.ReadPrivateWikiSource(b.workspaceID, in.Generation, in.Reference, in.Offset, limit)
	default:
		err = memory.ErrPrivateWikiUnavailable
	}
	if err == nil {
		err = b.validate()
	}
	return wikiResult(value, err)
}
