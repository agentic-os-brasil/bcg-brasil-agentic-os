package product

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func brainNavigationQueries() []any {
	fields := func() map[string]any {
		return map[string]any{"generation": map[string]any{"type": "string", "pattern": `^[a-f0-9]{64}$`}, "purpose": strSchema(768), "offset": map[string]any{"type": "integer", "minimum": 0, "maximum": 4 << 20}, "max_bytes": map[string]any{"type": "integer", "minimum": 4, "maximum": 4096}}
	}
	navigate := fields()
	navigate["reference"] = strSchema(1024)
	search := fields()
	search["query"] = strSchema(512)
	search["limit"] = map[string]any{"type": "integer", "minimum": 1, "maximum": 24}
	search["max_bytes"] = map[string]any{"type": "integer", "minimum": 512, "maximum": 4096}
	return []any{variant("brain.navigation_status", map[string]any{}), variant("brain.navigate", navigate, "generation", "reference", "purpose"), variant("brain.search", search, "generation", "query", "purpose"), variant("brain.diagnostics", fields(), "generation", "purpose")}
}
func brainNavigationMutations() []any {
	return []any{variant("brain.compile", map[string]any{"pages": map[string]any{"type": "array", "minItems": 1, "maxItems": 24, "uniqueItems": true, "items": strSchema(256)}, "purpose": strSchema(768)}, "pages", "purpose")}
}

func (b *Bound) brainNavigation(ctx context.Context, req *mcp.CallToolRequest, engine *atlasops.Engine, in input) (any, error) {
	if in.Operation == "brain.navigation_status" {
		return engine.NavigationStatus()
	}
	if err := atlasops.ValidateNavigationPurpose(in.Purpose); err != nil {
		return nil, err
	}
	if in.Operation == "brain.compile" {
		review, err := engine.ReviewNavigation(in.Pages, in.Purpose)
		if err != nil {
			return nil, err
		}
		// The exact source snapshots are review data. Their text cannot authorize
		// page edits, source enrollment, memory capture or background compilation.
		presentation := map[string]any{"operation": in.Operation, "workspace_id": b.workspaceID, "generation": review.Generation, "expected_generation": review.ExpectedGeneration, "pages": review.Sources, "effect": "Publish derived authored navigation for only these named pages. Preserve all source Markdown and private memory; no standing authority or background job."}
		if err = b.confirm(ctx, req, "Publish this exact authored navigation cohort?", review, presentation); err != nil {
			return nil, err
		}
		if err = b.validate(); err != nil {
			return nil, err
		}
		body, _ := json.Marshal(review)
		id := contentDigest(body)
		provenance := atlasops.Provenance{Origin: atlasops.OriginAttended, SessionID: "brain-navigation-" + id, IdempotencyKey: id, Ritual: "brain-index"}
		return engine.PublishNavigation(review, provenance)
	}
	view, err := engine.NavigationView(in.Generation)
	if err != nil {
		return nil, err
	}
	maximum := in.MaxBytes
	if maximum == 0 {
		maximum = 4096
	}
	var value any
	switch in.Operation {
	case "brain.navigate", "brain.diagnostics":
		reference := in.Reference
		if in.Operation == "brain.diagnostics" {
			reference = "diagnostics.json"
		}
		resolved, resolveErr := atlas.ResolveNavigationReference(reference)
		if resolveErr != nil {
			return nil, resolveErr
		}
		text, found := view.Files[resolved]
		if !found {
			return nil, errors.New("authored_navigation_reference_unavailable")
		}
		page, err := pageText(text, in.Offset, maximum)
		if err != nil {
			return nil, err
		}
		page["generation"], page["workspace_id"], page["reference"], page["published_at"] = view.Generation, b.workspaceID, reference, view.PublishedAt
		value = page
	case "brain.search":
		if strings.TrimSpace(in.Query) == "" || len(in.Query) > 512 || !utf8.ValidString(in.Query) || strings.ContainsRune(in.Query, 0) {
			return nil, errors.New("authored_navigation_query_invalid")
		}
		limit := in.Limit
		if limit == 0 {
			limit = 8
		}
		if limit < 1 || limit > 24 || maximum < 512 || maximum > 4096 || in.Offset < 0 {
			return nil, errors.New("authored_navigation_bounds_invalid")
		}
		matches := []atlas.NavigationPage{}
		for _, page := range view.Pages {
			if atlas.NavigationMatches(page, in.Query) {
				matches = append(matches, page)
			}
		}
		if in.Offset > len(matches) {
			return nil, errors.New("authored_navigation_offset_invalid")
		}
		hits := []map[string]any{}
		end := in.Offset
		result := func() map[string]any {
			return map[string]any{"workspace_id": b.workspaceID, "generation": view.Generation, "published_at": view.PublishedAt, "matches": hits, "offset": in.Offset, "next_offset": end, "complete": end == len(matches)}
		}
		for end < len(matches) && len(hits) < limit {
			page := matches[end]
			hit := map[string]any{"page": page.Page, "title": page.Title, "revision": page.Revision, "pointer": page.Pointer, "reference": atlas.NavigationReference("pages/" + page.Page)}
			hits = append(hits, hit)
			end++
			encoded, _ := json.Marshal(result())
			if len(encoded) > maximum {
				hits = hits[:len(hits)-1]
				end--
				break
			}
		}
		if len(hits) == 0 && end < len(matches) {
			return nil, errors.New("authored_navigation_budget_too_small")
		}
		value = result()
	default:
		return nil, errors.New("authored_operation_invalid")
	}
	// Recheck source admission/revisions after formatting and before disclosure.
	if _, err = engine.NavigationView(in.Generation); err != nil {
		return nil, err
	}
	return value, nil
}
