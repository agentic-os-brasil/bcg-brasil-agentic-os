package product

import (
	"context"
	"encoding/json"
	"errors"
	"os"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func ownerAtlasQueries() []any {
	return []any{
		variant("owner.atlas.status", map[string]any{}),
		variant("owner.atlas.collect", map[string]any{"purpose": strSchema(768), "pages": map[string]any{"type": "array", "minItems": 1, "maxItems": 24, "uniqueItems": true, "items": strSchema(256)}}, "purpose", "pages"),
	}
}

func ownerAtlasMutations() []any {
	return []any{
		variant("owner.atlas.prepare", map[string]any{}),
		variant("owner.atlas.create", map[string]any{"page": strSchema(256), "text": strSchema(256 << 10)}, "page", "text"),
		variant("owner.atlas.append", map[string]any{"page": strSchema(256), "section": strSchema(256), "text": strSchema(8 << 10), "expected_page_revision": strSchema(64)}, "page", "section", "text", "expected_page_revision"),
		variant("owner.atlas.update", map[string]any{"page": strSchema(256), "text": strSchema(256 << 10), "expected_page_revision": strSchema(64)}, "page", "text", "expected_page_revision"),
	}
}

type ownerAtlasMutation struct {
	Operation, Page, Text, Section, ExpectedRevision string
}

type ownerAtlasReview struct {
	Scope      string
	Mutation   ownerAtlasMutation
	Pages      []brainPreparedPage
	Provenance atlasops.Provenance
	Digest     string
}

func (b *Bound) ownerAtlasEngine(effect func() error) (*atlasops.Engine, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	return atlasops.OpenOwnerExisting(b.home, nil, effect)
}

func (b *Bound) reviewOwnerAtlas(in ownerAtlasMutation) (ownerAtlasReview, error) {
	if err := b.validate(); err != nil {
		return ownerAtlasReview{}, err
	}
	engine, err := b.ownerAtlasEngine(nil)
	if err != nil && !(in.Operation == "owner.atlas.prepare" && errors.Is(err, os.ErrNotExist)) {
		return ownerAtlasReview{}, err
	}
	pages := []atlas.AuthoredPage{{Page: in.Page, Text: in.Text}}
	if in.Operation == "owner.atlas.prepare" {
		pages = atlas.OwnerPages()
	}
	review := ownerAtlasReview{Scope: b.home, Mutation: in}
	intent, _ := json.Marshal([]any{b.home, in})
	id := contentDigest(intent)
	review.Provenance = atlasops.Provenance{Origin: atlasops.OriginAttended, SessionID: "owner-atlas-" + id, IdempotencyKey: id, Ritual: "owner-atlas"}
	for _, page := range pages {
		selected := brainPreparedPage{Page: page.Page, Text: page.Text}
		if engine != nil {
			current, readErr := engine.ReviewPage(atlasops.CollectRequest{Purpose: "Review the exact attended owner atlas change", Reader: atlasops.ReaderOwnerSession, Pages: []string{page.Page}})
			if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
				return ownerAtlasReview{}, readErr
			}
			if readErr == nil {
				selected.Current = &current
			}
		}
		switch in.Operation {
		case "owner.atlas.prepare", "owner.atlas.create":
		case "owner.atlas.append", "owner.atlas.update":
			if selected.Current == nil || !validContentDigest(in.ExpectedRevision) {
				return ownerAtlasReview{}, errOwnerReviewChanged
			}
			var replay bool
			if in.Operation == "owner.atlas.append" {
				replay, err = engine.ReviewAppendReplay(atlasops.AppendEntryRequest{Page: in.Page, Section: in.Section, Entry: in.Text, ExpectedRevision: in.ExpectedRevision, Provenance: review.Provenance})
			} else {
				replay, err = engine.ReviewUpdateReplay(atlasops.UpdatePageRequest{Page: in.Page, Body: in.Text, ExpectedRevision: in.ExpectedRevision, Provenance: review.Provenance})
			}
			if err != nil {
				return ownerAtlasReview{}, err
			}
			if selected.Current.Revision != in.ExpectedRevision && !replay {
				return ownerAtlasReview{}, errOwnerReviewChanged
			}
			selected.Replay = replay
			if in.Operation == "owner.atlas.append" && !replay {
				selected.Text, err = atlasops.PreviewAppend(selected.Current.Content, in.Section, in.Text)
				if err != nil {
					return ownerAtlasReview{}, err
				}
			}
		default:
			return ownerAtlasReview{}, errors.New("owner_atlas_operation_invalid")
		}
		review.Pages = append(review.Pages, selected)
	}
	body, _ := json.Marshal(review)
	review.Digest = contentDigest(body)
	return review, nil
}

func (b *Bound) applyOwnerAtlas(ctx context.Context, in ownerAtlasMutation, digest string) (any, error) {
	review, err := b.reviewOwnerAtlas(in)
	if err != nil {
		return nil, err
	}
	if digest == "" || review.Digest != digest {
		return nil, errOwnerReviewChanged
	}
	if in.Operation == "owner.atlas.prepare" {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if err = b.guardPublication(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := b.validate(); err != nil {
				return err
			}
			_, err := atlas.InitializeOwner(b.home)
			return err
		}); err != nil {
			return nil, err
		}
		return map[string]any{"scope": "owner", "prepared": true}, nil
	}
	engine, err := b.ownerAtlasEngine(func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return b.validate()
	})
	if err != nil {
		return nil, err
	}
	var result atlasops.Result
	switch in.Operation {
	case "owner.atlas.create":
		result, err = engine.CreatePage(atlasops.CreatePageRequest{Page: in.Page, Body: in.Text, Provenance: review.Provenance})
	case "owner.atlas.append":
		result, err = engine.AppendEntry(atlasops.AppendEntryRequest{Page: in.Page, Section: in.Section, Entry: in.Text, ExpectedRevision: in.ExpectedRevision, Provenance: review.Provenance})
	case "owner.atlas.update":
		result, err = engine.UpdatePage(atlasops.UpdatePageRequest{Page: in.Page, Body: in.Text, ExpectedRevision: in.ExpectedRevision, Provenance: review.Provenance})
	}
	if err == nil && result.State == atlasops.StateProposed {
		err = errOwnerReviewChanged
	}
	return result, err
}

func (b *Bound) handleOwnerAtlas(ctx context.Context, req *mcp.CallToolRequest, in input) *mcp.CallToolResult {
	var value any
	engine, err := b.ownerAtlasEngine(nil)
	if in.Operation == "owner.atlas.status" {
		if errors.Is(err, os.ErrNotExist) {
			return result(map[string]any{"scope": "owner", "prepared": false}, nil)
		}
		if err == nil {
			missing := []string{}
			for _, page := range atlas.OwnerPages() {
				_, readErr := engine.ReviewPage(atlasops.CollectRequest{Purpose: "Inspect canonical owner atlas preparation", Reader: atlasops.ReaderOwnerSession, Pages: []string{page.Page}})
				if errors.Is(readErr, os.ErrNotExist) {
					missing = append(missing, page.Page)
				} else if readErr != nil {
					err = readErr
					break
				}
			}
			value = map[string]any{"scope": "owner", "prepared": len(missing) == 0, "missing": missing}
		}
	} else if in.Operation == "owner.atlas.collect" {
		if err == nil {
			value, err = engine.Collect(atlasops.CollectRequest{Purpose: in.Purpose, Reader: atlasops.ReaderOwnerSession, Pages: in.Pages})
		}
	} else {
		var review ownerAtlasReview
		mutation := ownerAtlasMutation{Operation: in.Operation, Page: in.Page, Text: in.Text, Section: in.Section, ExpectedRevision: in.ExpectedPageRevision}
		review, err = b.reviewOwnerAtlas(mutation)
		if err == nil {
			presentation := map[string]any{"operation": mutation.Operation, "scope": "Owner professional atlas only; existing pages are preserved during preparation and creation. No workspace content, Owner Context, memory or schedules are authorized.", "pages": review.Pages}
			err = b.confirm(ctx, req, "Authorize this exact attended owner atlas change?", review, presentation)
			if err == nil {
				value, err = b.applyOwnerAtlas(ctx, mutation, review.Digest)
			}
		}
	}
	if err == nil {
		err = b.validate()
	}
	if err == nil {
		return result(value, nil)
	}
	var pending *confirmationPending
	if errors.As(err, &pending) {
		return pending.result
	}
	code := "owner_atlas_rejected"
	switch {
	case errors.Is(err, errConfirmationUnavailable):
		code = "confirmation_unavailable"
	case errors.Is(err, errConfirmationDeclined):
		code = "confirmation_declined"
	case errors.Is(err, errOwnerReviewChanged):
		code = "revision_conflict"
	}
	out := result(map[string]any{"code": code, "retryable": false}, nil)
	out.IsError = true
	return out
}
