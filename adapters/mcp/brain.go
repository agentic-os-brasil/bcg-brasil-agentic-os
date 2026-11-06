package product

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacepolicy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func brainQueries() []any {
	fields := memoryTextFields()
	fields["offset"] = map[string]any{"type": "integer", "minimum": 0, "maximum": 256 << 10}
	fields["page"], fields["purpose"] = strSchema(256), strSchema(768)
	return []any{variant("brain.status", map[string]any{}), variant("brain.templates", map[string]any{}), variant("brain.read", fields, "page", "purpose")}
}
func brainMutations() []any {
	edits := func(extra map[string]any) map[string]any {
		fields := map[string]any{"page": strSchema(256), "expected_page_revision": strSchema(64)}
		for key, value := range extra {
			fields[key] = value
		}
		return fields
	}
	return []any{
		variant("brain.prepare", map[string]any{}),
		variant("brain.create", map[string]any{"page": strSchema(256), "text": strSchema(256 << 10)}, "page", "text"),
		variant("brain.append", edits(map[string]any{"section": strSchema(256), "text": strSchema(8 << 10)}), "page", "expected_page_revision", "section", "text"),
		variant("brain.update", edits(map[string]any{"text": strSchema(256 << 10)}), "page", "expected_page_revision", "text"),
		variant("brain.set_field", edits(map[string]any{"field": strSchema(256), "value": strSchema(4096)}), "page", "expected_page_revision", "field", "value"),
		variant("brain.link", edits(map[string]any{"section": strSchema(256), "target": strSchema(256), "label": strSchema(512)}), "page", "expected_page_revision", "section", "target", "label"),
	}
}

type BrainMutation struct {
	Operation        string
	Page             string
	Text             string
	Section          string
	ExpectedRevision string
	Field            string
	Value            string
	Target           string
	Label            string
}
type brainPreparedPage struct {
	Page    string
	Text    string
	Current *atlasops.PageProjection
	Replay  bool
}
type BrainReview struct {
	WorkspaceID string
	Scope       string
	Mutation    BrainMutation
	Pages       []brainPreparedPage
	Target      *atlasops.PageProjection
	Provenance  atlasops.Provenance
	Digest      string
}

func (b *Bound) brainPage(page string) error {
	if artifactPath(page) != nil || !strings.HasSuffix(page, ".md") {
		return errors.New("authored_page_invalid")
	}
	for _, reserved := range []string{"owner/", "memory/", "atlas/owner/"} {
		if strings.HasPrefix(page, reserved) {
			return errors.New("authored_page_unavailable")
		}
	}
	if !workspacepolicy.CasePathAllowed(b.options.WorkspacePath, filepath.Join(b.options.WorkspacePath, "brain", filepath.FromSlash(page))) {
		return errors.New("authored_case_unavailable")
	}
	return nil
}
func (b *Bound) authoredPage(engine *atlasops.Engine, page, purpose string) (*atlasops.PageProjection, error) {
	if err := b.brainPage(page); err != nil {
		return nil, err
	}
	view, err := engine.ReviewPage(atlasops.CollectRequest{Purpose: purpose, Reader: atlasops.ReaderOwnerSession, Pages: []string{page}})
	if err != nil {
		return nil, err
	}
	return &view, nil
}
func (b *Bound) brainReview(in BrainMutation) (BrainReview, error) {
	var review BrainReview
	if err := b.validate(); err != nil {
		return review, err
	}
	engine, err := b.knowledgeEngine()
	if err != nil {
		return review, err
	}
	if !map[string]bool{"brain.create": true, "brain.prepare": true, "brain.append": true, "brain.update": true, "brain.set_field": true, "brain.link": true}[in.Operation] {
		return review, errors.New("authored_operation_invalid")
	}
	pages := []atlas.AuthoredPage{{Page: in.Page, Text: in.Text}}
	if in.Operation == "brain.prepare" {
		if in.Page != "" || in.Text != "" {
			return review, errors.New("authored_input_invalid")
		}
		pages = atlas.WorkspacePages(b.workspaceID, time.Now())
	}
	review = BrainReview{WorkspaceID: b.workspaceID, Scope: b.home, Mutation: in}
	intent, _ := json.Marshal([]any{review.Scope, review.WorkspaceID, in, pages})
	id := contentDigest(intent)
	review.Provenance = atlasops.Provenance{Origin: atlasops.OriginAttended, SessionID: "brain-" + id, IdempotencyKey: id, Ritual: "authored-brain"}
	for _, page := range pages {
		if err := b.brainPage(page.Page); err != nil {
			return BrainReview{}, err
		}
		current, err := b.authoredPage(engine, page.Page, "Review exact authored workspace change")
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return BrainReview{}, err
		}
		replay := false
		switch in.Operation {
		case "brain.prepare", "brain.create":
			if _, err := atlas.ValidateAuthoredPage(page.Page, b.workspaceID, page.Text); err != nil {
				return BrainReview{}, err
			}
		default:
			if current == nil || !validContentDigest(in.ExpectedRevision) {
				return BrainReview{}, errOwnerReviewChanged
			}
			// Keep the revision fence. Only the owning read-only inspection of
			// this exact request may recognize an already applied receipt.
			if current.Revision != in.ExpectedRevision {
				replay, err = brainExactReplay(engine, review, page.Page)
				if err != nil {
					return BrainReview{}, err
				}
				if !replay {
					return BrainReview{}, errOwnerReviewChanged
				}
			}
			if replay {
				page.Text = current.Content
			} else {
				switch in.Operation {
				case "brain.update":
					if _, err = atlas.ValidateAuthoredPage(page.Page, b.workspaceID, page.Text); err != nil {
						return BrainReview{}, err
					}
				case "brain.append":
					page.Text, err = atlasops.PreviewAppend(current.Content, in.Section, in.Text)
				case "brain.set_field":
					page.Text, _, err = atlasops.PreviewField(current.Content, in.Field, in.Value)
				case "brain.link":
					review.Target, err = b.authoredPage(engine, in.Target, "Review the exact authored reference target")
					if err == nil {
						page.Text, _, err = atlasops.PreviewLink(current.Content, in.Page, in.Section, in.Target, in.Label)
					}
				}
			}
			if err != nil {
				return BrainReview{}, err
			}
			if len(page.Text) > 256<<10 {
				return BrainReview{}, errors.New("authored_body_invalid")
			}
		}

		review.Pages = append(review.Pages, brainPreparedPage{Page: page.Page, Text: page.Text, Current: current, Replay: replay})
	}
	body, _ := json.Marshal(review)
	review.Digest = contentDigest(body)
	return review, nil
}
func brainPresentation(review BrainReview) any {
	return map[string]any{"operation": review.Mutation.Operation, "workspace_id": review.WorkspaceID, "pages": review.Pages, "target": review.Target, "expected_page_revision": review.Mutation.ExpectedRevision, "scope": "Exact authored workspace pages only. Creation preserves existing files; edits use the displayed current page and expected revision. This does not authorize memory capture or background enrollment."}
}
func (b *Bound) applyBrainReview(ctx context.Context, in BrainMutation, digest string) (any, error) {
	review, err := b.brainReview(in)
	if err != nil {
		return nil, err
	}
	if digest == "" || digest != review.Digest {
		return nil, errOwnerReviewChanged
	}
	engine, err := b.knowledgeEngine()
	if err != nil {
		return nil, err
	}
	out := []atlasops.Result{}
	for _, page := range review.Pages {
		if err = ctx.Err(); err != nil {
			return nil, err
		}
		if err = b.brainPage(page.Page); err != nil {
			return nil, err
		}
		provenance := brainPageProvenance(review, page.Page)
		var result atlasops.Result
		switch in.Operation {
		case "brain.append":
			result, err = engine.AppendEntry(atlasops.AppendEntryRequest{Page: page.Page, Section: in.Section, Entry: in.Text, ExpectedRevision: in.ExpectedRevision, Provenance: provenance})
		case "brain.update":
			result, err = engine.UpdatePage(atlasops.UpdatePageRequest{Page: page.Page, Body: in.Text, ExpectedRevision: in.ExpectedRevision, Provenance: provenance})
		case "brain.set_field":
			result, err = engine.SetField(atlasops.SetFieldRequest{Page: page.Page, Field: in.Field, Value: in.Value, ExpectedRevision: in.ExpectedRevision, Provenance: provenance})
		case "brain.link":
			result, err = engine.Link(atlasops.LinkRequest{Page: page.Page, Section: in.Section, Target: in.Target, Label: in.Label, ExpectedRevision: in.ExpectedRevision, Provenance: provenance})
		default:
			result, err = engine.CreatePage(atlasops.CreatePageRequest{Page: page.Page, Body: page.Text, Provenance: provenance})
		}

		if err != nil {
			return nil, err
		}
		if result.State == atlasops.StateProposed {
			return nil, errOwnerReviewChanged
		}
		out = append(out, result)
	}
	if in.Operation != "brain.prepare" {
		return out[0], nil
	}
	return map[string]any{"state": "ready", "workspace_id": b.workspaceID, "pages": out}, nil
}
func (b *Bound) handleBrain(ctx context.Context, req *mcp.CallToolRequest, in input) *mcp.CallToolResult {
	engine, err := b.knowledgeEngine()
	var value any
	if err == nil {
		switch in.Operation {
		case "brain.status":
			missing := []string{}
			diagnostics := map[string][]string{}
			for _, page := range atlas.WorkspacePages(b.workspaceID, time.Now()) {
				current, readErr := b.authoredPage(engine, page.Page, "Inspect fixed authored template status")
				if errors.Is(readErr, os.ErrNotExist) {
					missing = append(missing, page.Page)
				} else if readErr != nil {
					err = readErr
					break
				} else {
					diagnostics[page.Page] = atlas.AuthoredDiagnostics(page.Page, b.workspaceID, current.Content)
				}
			}
			value = map[string]any{"workspace_id": b.workspaceID, "prepared": len(missing) == 0, "missing": missing, "diagnostics": diagnostics}
		case "brain.templates":
			templates := []map[string]any{}
			for _, page := range atlas.WorkspacePages(b.workspaceID, time.Now()) {
				if strings.Contains(page.Page, "/template-") {
					templates = append(templates, map[string]any{"page": page.Page, "text": page.Text})
				}
			}
			value = map[string]any{"workspace_id": b.workspaceID, "templates": templates}
		case "brain.read":
			var current *atlasops.PageProjection
			current, err = b.authoredPage(engine, in.Page, in.Purpose)
			if err == nil {
				maximum := in.MaxBytes
				if maximum == 0 {
					maximum = 4096
				}
				var metadata map[string]any
				metadata, err = pageText(current.Content, in.Offset, maximum)
				if err == nil {
					metadata["workspace_id"], metadata["page"], metadata["revision"], metadata["pointer"] = b.workspaceID, in.Page, current.Revision, current.Pointer
					metadata["diagnostics"] = atlas.AuthoredDiagnostics(in.Page, b.workspaceID, current.Content)
					value = metadata
				}
			}
		case "brain.prepare", "brain.create", "brain.append", "brain.update", "brain.set_field", "brain.link":
			mutation := BrainMutation{Operation: in.Operation, Page: in.Page, Text: in.Text, Section: in.Section, ExpectedRevision: in.ExpectedPageRevision, Field: in.Field, Value: in.Value, Target: in.Target, Label: in.Label}
			var review BrainReview
			review, err = b.brainReview(mutation)
			if err == nil {
				err = b.confirm(ctx, req, "Authorize this exact authored workspace change?", review, brainPresentation(review))
			}
			if err == nil {
				value, err = b.applyBrainReview(ctx, mutation, review.Digest)
			}
		default:
			err = errors.New("authored_operation_invalid")
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
	code := "brain_rejected"
	switch {
	case errors.Is(err, errConfirmationUnavailable):
		code = "confirmation_unavailable"
	case errors.Is(err, errConfirmationDeclined):
		code = "confirmation_declined"
	case errors.Is(err, errOwnerReviewChanged):
		code = "revision_conflict"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		code = "cancelled"
	}
	out := result(map[string]any{"code": code, "retryable": false}, nil)
	out.IsError = true
	return out
}

func brainPageProvenance(review BrainReview, page string) atlasops.Provenance {
	provenance := review.Provenance
	provenance.IdempotencyKey = contentDigest([]byte(provenance.IdempotencyKey + "\x00" + page))
	return provenance
}
func brainExactReplay(engine *atlasops.Engine, review BrainReview, page string) (bool, error) {
	in, provenance := review.Mutation, brainPageProvenance(review, page)
	switch in.Operation {
	case "brain.append":
		return engine.ReviewAppendReplay(atlasops.AppendEntryRequest{Page: page, Section: in.Section, Entry: in.Text, ExpectedRevision: in.ExpectedRevision, Provenance: provenance})
	case "brain.update":
		return engine.ReviewUpdateReplay(atlasops.UpdatePageRequest{Page: page, Body: in.Text, ExpectedRevision: in.ExpectedRevision, Provenance: provenance})
	case "brain.set_field":
		return engine.ReviewFieldReplay(atlasops.SetFieldRequest{Page: page, Field: in.Field, Value: in.Value, ExpectedRevision: in.ExpectedRevision, Provenance: provenance})
	case "brain.link":
		return engine.ReviewLinkReplay(atlasops.LinkRequest{Page: page, Section: in.Section, Target: in.Target, Label: in.Label, ExpectedRevision: in.ExpectedRevision, Provenance: provenance})
	}
	return false, nil
}
