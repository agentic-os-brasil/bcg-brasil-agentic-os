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
	return []any{variant("brain.prepare", map[string]any{}), variant("brain.create", map[string]any{"page": strSchema(256), "text": strSchema(256 << 10)}, "page", "text"), variant("brain.append", map[string]any{"page": strSchema(256), "section": strSchema(256), "text": strSchema(8 << 10), "expected_page_revision": strSchema(64)}, "page", "section", "text", "expected_page_revision")}
}

type BrainMutation struct {
	Operation        string
	Page             string
	Text             string
	Section          string
	ExpectedRevision string
}
type brainPreparedPage struct {
	Page    string
	Text    string
	Current *atlasops.PageProjection
}
type BrainReview struct {
	WorkspaceID string
	Scope       string
	Mutation    BrainMutation
	Pages       []brainPreparedPage
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
	if in.Operation != "brain.create" && in.Operation != "brain.prepare" && in.Operation != "brain.append" {
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
	for _, page := range pages {
		if err := b.brainPage(page.Page); err != nil {
			return BrainReview{}, err
		}
		current, err := b.authoredPage(engine, page.Page, "Review exact authored page creation")
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return BrainReview{}, err
		}
		if in.Operation == "brain.append" {
			if current == nil || in.ExpectedRevision == "" || current.Revision != in.ExpectedRevision {
				return BrainReview{}, errOwnerReviewChanged
			}
			page.Text, err = atlasops.PreviewAppend(current.Content, in.Section, in.Text)
			if err != nil {
				return BrainReview{}, err
			}
		} else if _, err := atlas.ValidateAuthoredPage(page.Page, b.workspaceID, page.Text); err != nil {
			return BrainReview{}, err
		}
		review.Pages = append(review.Pages, brainPreparedPage{Page: page.Page, Text: page.Text, Current: current})
	}
	intent, _ := json.Marshal([]any{review.Scope, review.WorkspaceID, in, pages})
	id := contentDigest(intent)
	review.Provenance = atlasops.Provenance{Origin: atlasops.OriginAttended, SessionID: "brain-" + id, IdempotencyKey: id, Ritual: "authored-brain"}
	body, _ := json.Marshal(review)
	review.Digest = contentDigest(body)
	return review, nil
}
func brainPresentation(review BrainReview) any {
	return map[string]any{"operation": review.Mutation.Operation, "workspace_id": review.WorkspaceID, "pages": review.Pages, "scope": "Exact authored workspace pages only. Existing pages are preserved. This is not memory capture or background enrollment."}
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
		provenance := review.Provenance
		provenance.IdempotencyKey = contentDigest([]byte(provenance.IdempotencyKey + "\x00" + page.Page))
		var result atlasops.Result
		if in.Operation == "brain.append" {
			result, err = engine.AppendEntry(atlasops.AppendEntryRequest{Page: page.Page, Section: in.Section, Entry: in.Text, ExpectedRevision: in.ExpectedRevision, Provenance: provenance})
		} else {
			result, err = engine.CreatePage(atlasops.CreatePageRequest{Page: page.Page, Body: page.Text, Provenance: provenance})
		}
		if err != nil {
			return nil, err
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
		case "brain.prepare", "brain.create", "brain.append":
			mutation := BrainMutation{Operation: in.Operation, Page: in.Page, Text: in.Text, Section: in.Section, ExpectedRevision: in.ExpectedPageRevision}
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
