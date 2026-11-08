package product

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
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
		variant("brain.move", map[string]any{"page": strSchema(256), "target": strSchema(256), "expected_page_revision": strSchema(64), "expected_lifecycle_revision": map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991}}, "page", "target", "expected_page_revision", "expected_lifecycle_revision"),
		variant("brain.retire", map[string]any{"page": strSchema(256), "expected_page_revision": strSchema(64), "expected_lifecycle_revision": map[string]any{"type": "integer", "minimum": 0, "maximum": 9007199254740991}}, "page", "expected_page_revision", "expected_lifecycle_revision"),
		variant("brain.prepare", map[string]any{}),
		variant("brain.create", map[string]any{"page": strSchema(256), "text": strSchema(256 << 10)}, "page", "text"),
		variant("brain.append", edits(map[string]any{"section": strSchema(256), "text": strSchema(8 << 10)}), "page", "expected_page_revision", "section", "text"),
		variant("brain.update", edits(map[string]any{"text": strSchema(256 << 10)}), "page", "expected_page_revision", "text"),
		variant("brain.set_field", edits(map[string]any{"field": strSchema(256), "value": strSchema(4096)}), "page", "expected_page_revision", "field", "value"),
		variant("brain.link", edits(map[string]any{"section": strSchema(256), "target": strSchema(256), "label": strSchema(512)}), "page", "expected_page_revision", "section", "target", "label"),
	}
}

type BrainMutation struct {
	ExpectedLifecycleRevision *uint64 `json:",omitempty"`
	Operation                 string
	Page                      string
	Text                      string
	Section                   string
	ExpectedRevision          string
	Field                     string
	Value                     string
	Target                    string
	Label                     string
}
type brainPreparedPage struct {
	Page    string
	Text    string
	Current *atlasops.PageProjection
	Replay  bool
}
type BrainReview struct {
	LifecycleRevision uint64
	Lifecycle         *atlasops.LifecycleReview `json:",omitempty"`
	WorkspaceID       string
	Scope             string
	Mutation          BrainMutation
	Pages             []brainPreparedPage
	Target            *atlasops.PageProjection
	Provenance        atlasops.Provenance
	Digest            string
}

// An incomplete preparation reports only confirmed receipts and unconfirmed
// fixed-plan pages. Pending does not claim absence: an interrupted canonical
// publication may already have effects that an explicit retry must reconcile.
type brainPreparationError struct {
	Completed []atlasops.Result
	Pending   []string
	cause     error
}

func (failure *brainPreparationError) Error() string { return "brain_prepare_incomplete" }
func (failure *brainPreparationError) Unwrap() error { return failure.cause }
func incompleteBrainPreparation(review BrainReview, completed []atlasops.Result, next int, cause error) *brainPreparationError {
	failure := &brainPreparationError{Completed: append([]atlasops.Result{}, completed...), Pending: []string{}, cause: cause}
	for _, page := range review.Pages[next:] {
		failure.Pending = append(failure.Pending, page.Page)
	}
	return failure
}

func (b *Bound) authoredPage(engine *atlasops.Engine, page, purpose string) (*atlasops.PageProjection, error) {
	if err := b.workspacePage(page); err != nil {
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
	if !map[string]bool{"brain.create": true, "brain.prepare": true, "brain.append": true, "brain.update": true, "brain.set_field": true, "brain.link": true, "brain.move": true, "brain.retire": true}[in.Operation] {
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
	review.LifecycleRevision, err = engine.LifecycleRevision()
	if err != nil {
		return BrainReview{}, err
	}
	if in.Operation == "brain.move" || in.Operation == "brain.retire" {
		if in.ExpectedLifecycleRevision == nil {
			return BrainReview{}, atlasops.ErrLifecycleChanged
		}
		var move atlasops.LifecycleReview
		var moveErr error
		if in.Operation == "brain.retire" {
			move, moveErr = engine.ReviewRetire(atlasops.RetirePageRequest{Page: in.Page, ExpectedRevision: in.ExpectedRevision, ExpectedLifecycleRevision: *in.ExpectedLifecycleRevision, Provenance: brainPageProvenance(review, in.Page)})
		} else {
			move, moveErr = engine.ReviewMove(atlasops.MovePageRequest{Page: in.Page, Target: in.Target, ExpectedRevision: in.ExpectedRevision, ExpectedLifecycleRevision: *in.ExpectedLifecycleRevision, Provenance: brainPageProvenance(review, in.Page)})
		}
		if moveErr != nil {
			return BrainReview{}, moveErr
		}
		review.Lifecycle = &move
		body, _ := json.Marshal(review)
		review.Digest = contentDigest(body)
		return review, nil
	}
	// A receipt can establish exact-once effect identity, never renewed target
	// authority. Both endpoints are re-admitted even for a no-op link replay.
	if in.Operation == "brain.link" {
		review.Target, err = b.authoredPage(engine, in.Target, "Review the exact authored reference target")
		if err != nil {
			return BrainReview{}, err
		}
	}
	for _, page := range pages {
		if err := b.workspacePage(page.Page); err != nil {
			return BrainReview{}, err
		}
		current, err := b.authoredPage(engine, page.Page, "Review exact authored workspace change")
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return BrainReview{}, err
		}
		replay := false
		switch in.Operation {
		case "brain.prepare", "brain.create":
			location, _, locErr := engine.LocatePage(page.Page)
			if locErr != nil {
				return BrainReview{}, locErr
			}
			if _, err := atlas.ValidateAuthoredIdentity(location.Location, b.workspaceID, page.Text, location.StableID); err != nil {
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
					if _, err = atlas.ValidateAuthoredIdentity(current.Page, b.workspaceID, page.Text, current.StableID); err != nil {
						return BrainReview{}, err
					}
				case "brain.append":
					page.Text, err = atlasops.PreviewAppend(current.Content, in.Section, in.Text)
				case "brain.set_field":
					page.Text, _, err = atlasops.PreviewField(current.Content, in.Field, in.Value)
				case "brain.link":
					page.Text, _, err = atlasops.PreviewLink(current.Content, current.Page, in.Section, review.Target.Page, in.Label)
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
	currentLifecycle, lifeErr := engine.LifecycleRevision()
	if lifeErr != nil {
		return BrainReview{}, lifeErr
	}
	if currentLifecycle != review.LifecycleRevision {
		return BrainReview{}, atlasops.ErrLifecycleChanged
	}
	body, _ := json.Marshal(review)
	review.Digest = contentDigest(body)
	return review, nil
}
func brainPresentation(review BrainReview) any {
	if review.Lifecycle != nil {
		return map[string]any{"operation": review.Mutation.Operation, "workspace_id": review.WorkspaceID, "lifecycle": review.Lifecycle, "reference_scope": "Currently admitted authored corpus only; affected references are presented, not rewritten.", "scope": "Exact reviewed move or fixed private archive only. Stable identity and reserved names are preserved; retirement makes authored content unavailable. An occupied target is refused. Pending changes require an exact attended retry."}
	}
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
	engine, err := b.workspaceEngine(func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		return b.validate()
	})
	if err != nil {
		return nil, err
	}
	engine.ExpectLifecycleRevision(review.LifecycleRevision)
	if b.observeWorkspaceTransition != nil {
		b.observeWorkspaceTransition("reviewed", in.Page)
	}
	if review.Lifecycle != nil {
		if in.Operation == "brain.retire" {
			return engine.RetirePage(atlasops.RetirePageRequest{Page: in.Page, ExpectedRevision: in.ExpectedRevision, ExpectedLifecycleRevision: *in.ExpectedLifecycleRevision, ReferencesDigest: review.Lifecycle.ReferencesDigest, Provenance: brainPageProvenance(review, in.Page)})
		}
		return engine.MovePage(atlasops.MovePageRequest{Page: in.Page, Target: in.Target, ExpectedRevision: in.ExpectedRevision, ExpectedLifecycleRevision: *in.ExpectedLifecycleRevision, ReferencesDigest: review.Lifecycle.ReferencesDigest, Provenance: brainPageProvenance(review, in.Page)})
	}
	out := []atlasops.Result{}
	failed := func(next int, cause error) (any, error) {
		if in.Operation == "brain.prepare" {
			return nil, incompleteBrainPreparation(review, out, next, cause)
		}
		return nil, cause
	}
	for index, page := range review.Pages {
		if err = ctx.Err(); err != nil {
			return failed(index, err)
		}
		if err = b.workspacePage(page.Page); err != nil {
			return failed(index, err)
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
			result, err = engine.Link(atlasops.LinkRequest{Page: page.Page, Section: in.Section, Target: in.Target, Label: in.Label, ExpectedRevision: in.ExpectedRevision, ExpectedTargetRevision: review.Target.Revision, Provenance: provenance})
		default:
			result, err = engine.CreatePage(atlasops.CreatePageRequest{Page: page.Page, Body: page.Text, Provenance: provenance})
		}

		if err != nil {
			return failed(index, err)
		}
		if result.State == atlasops.StateProposed {
			return failed(index, errOwnerReviewChanged)
		}
		out = append(out, result)
		if in.Operation == "brain.prepare" && result.State == atlasops.StateWritten && b.afterBrainPrepareEffect != nil {
			if err := b.afterBrainPrepareEffect(); err != nil {
				return failed(index+1, err)
			}
		}
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
					_, diagnostic := atlas.ValidateAuthoredIdentity(current.Page, b.workspaceID, current.Content, current.StableID)
					diagnostics[page.Page] = []string{}
					if diagnostic != nil {
						diagnostics[page.Page] = []string{diagnostic.Error()}
					}
				}
			}
			revision, revErr := engine.LifecycleRevision()
			if revErr != nil {
				err = revErr
				break
			}
			value = map[string]any{"lifecycle_revision": revision, "workspace_id": b.workspaceID, "prepared": len(missing) == 0, "mutation_available": atlasops.WorkspaceMutationReady(), "missing": missing, "diagnostics": diagnostics}
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
					metadata["workspace_id"], metadata["page"], metadata["revision"], metadata["pointer"] = b.workspaceID, current.Page, current.Revision, current.Pointer
					metadata["stable_id"], metadata["lifecycle_revision"] = current.StableID, current.LifecycleRevision
					_, diagnostic := atlas.ValidateAuthoredIdentity(current.Page, b.workspaceID, current.Content, current.StableID)
					metadata["diagnostics"] = []string{}
					if diagnostic != nil {
						metadata["diagnostics"] = []string{diagnostic.Error()}
					}
					value = metadata
				}
			}
		case "brain.prepare", "brain.create", "brain.append", "brain.update", "brain.set_field", "brain.link", "brain.move", "brain.retire":
			mutation := BrainMutation{ExpectedLifecycleRevision: in.ExpectedLifecycleRevision, Operation: in.Operation, Page: in.Page, Text: in.Text, Section: in.Section, ExpectedRevision: in.ExpectedPageRevision, Field: in.Field, Value: in.Value, Target: in.Target, Label: in.Label}
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
		if err != nil && in.Operation == "brain.prepare" {
			if prepared, ok := value.(map[string]any); ok {
				if completed, ok := prepared["pages"].([]atlasops.Result); ok {
					err = &brainPreparationError{Completed: completed, Pending: []string{}, cause: err}
				}
			}
		}
	}
	if err == nil {
		return result(value, nil)
	}
	var pending *confirmationPending
	if errors.As(err, &pending) {
		return pending.result
	}
	var incomplete *brainPreparationError
	if errors.As(err, &incomplete) {
		preserved := []string{}
		for _, entry := range incomplete.Completed {
			if entry.State == atlasops.StateUnchanged {
				preserved = append(preserved, entry.Page)
			}
		}
		out := result(map[string]any{"code": "brain_prepare_incomplete", "state": "incomplete", "workspace_id": b.workspaceID, "completed": incomplete.Completed, "pending": incomplete.Pending, "preserved_pages": preserved, "retryable": false, "next_action": "Inspect brain.status, then explicitly review a new brain.prepare. Pending pages are unconfirmed and may already exist; do not repair or delete them automatically."}, nil)
		out.IsError = true
		return out
	}
	if errors.Is(err, atlasops.ErrLifecyclePending) {
		_, revision, admissionErr := engine.LocatePage(in.Page)
		if errors.Is(admissionErr, atlasops.ErrLifecyclePending) {
			out := result(map[string]any{"code": "brain_lifecycle_incomplete", "state": "incomplete", "workspace_id": b.workspaceID, "lifecycle_revision": revision, "retryable": false, "next_action": "Keep the original page, target and exact content/lifecycle revisions. Explicitly review that same brain.move or brain.retire to resume its matching operation; preserve existing files and do not repair or overwrite them automatically."}, nil)
			out.IsError = true
			return out
		}
	}
	code := "brain_rejected"
	switch {
	case errors.Is(err, errConfirmationUnavailable):
		code = "confirmation_unavailable"
	case errors.Is(err, errConfirmationDeclined):
		code = "confirmation_declined"
	case errors.Is(err, errOwnerReviewChanged), errors.Is(err, atlasops.ErrReviewedTargetChanged), errors.Is(err, atlasops.ErrLifecycleChanged):
		code = "revision_conflict"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		code = "cancelled"
	}
	failure := map[string]any{"code": code, "retryable": false}
	if code == "revision_conflict" {
		failure["next_action"] = "Preserve newer work. Read the currently admitted page and brain.status, then explicitly review a fresh request with those exact content and lifecycle revisions."
	}
	out := result(failure, nil)
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
		return engine.ReviewLinkReplay(atlasops.LinkRequest{Page: page, Section: in.Section, Target: in.Target, Label: in.Label, ExpectedRevision: in.ExpectedRevision, ExpectedTargetRevision: review.Target.Revision, Provenance: provenance})
	}
	return false, nil
}
