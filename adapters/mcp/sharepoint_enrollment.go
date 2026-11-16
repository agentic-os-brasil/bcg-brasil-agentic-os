package product

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/priorwork"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func sharepointEnrollmentQueries() []any {
	return []any{variant("sharepoint.enrollment.status", map[string]any{})}
}
func sharepointEnrollmentMutations() []any {
	return []any{
		variant("sharepoint.enrollment.select", map[string]any{"folder_urls": map[string]any{"type": "array", "minItems": 1, "maxItems": 8, "uniqueItems": true, "items": strSchema(2048)}, "purpose": map[string]any{"type": "string", "const": "prior_work_retrieval"}}, "folder_urls", "purpose"),
		variant("sharepoint.enrollment.revoke", map[string]any{}),
	}
}
func (b *Bound) sharepointSources() priorwork.SourceSelectionStore {
	return priorwork.SourceSelectionStore{Root: filepath.Join(b.home, "atlases", "organization", "sharepoint-work"), CustodyRoot: b.home}
}
func sharepointEnrollmentState(status priorwork.SourceSelectionStatus) map[string]any {
	return map[string]any{"local_control": status.State, "selection": status, "signed_enrollment": "not_attested", "provider": map[string]any{"required": "approved Claude SharePoint MCP connection", "configured": false, "state": "unavailable", "reason": "provider_not_admitted", "codex_collection": "unavailable/corporate_policy"}, "external_access_verified": false, "source_content_verified": false, "ingestion_state": "not_executed", "next_action": "An attended Claude operator must admit the approved SharePoint connection and current project scope before collection. Organizational prior-work import separately requires an independently signed enrollment and collector receipt; local selection is not that authority."}
}
func (b *Bound) handleSharepointEnrollment(ctx context.Context, req *mcp.CallToolRequest, in input) *mcp.CallToolResult {
	store := b.sharepointSources()
	var status priorwork.SourceSelectionStatus
	var err error
	attempted := false
	if in.Operation == "sharepoint.enrollment.status" {
		status, err = store.Status(b.workspaceID)
	} else {
		state := priorwork.SourceSelected
		if in.Operation == "sharepoint.enrollment.revoke" {
			state = priorwork.SourceRevoked
		}
		var review *priorwork.SourceSelectionReview
		review, err = store.PrepareSelection(b.workspaceID, state, in.FolderURLs)
		if err == nil {
			defer review.Close()
			presentation := map[string]any{"operation": in.Operation, "workspace_id": b.workspaceID, "workspace_path": b.options.WorkspacePath, "destination": store.Root, "current": review.Current, "proposed": review.Proposed, "provenance": "attended_owner_native_review", "effect": "Record this exact local project source choice only. Revocation stops future local selection use and preserves ingested bytes, immutable history, signed organizational policy and unrelated workspaces. No connector access, collection, signed enrollment, background job or remote write is authorized. Server timestamp is assigned only after approval."}
			err = b.confirm(ctx, req, "Authorize this exact local SharePoint source control?", review, presentation)
			if err == nil {
				attempted = true
				err = b.guardPublication(func() error {
					var e error
					status, e = store.CommitSelection(review, review.Digest, func() error {
						if e := ctx.Err(); e != nil {
							return e
						}
						return b.validate()
					})
					return e
				})
			}
		}
	}
	if err == nil {
		err = b.validate()
	}
	if err == nil {
		return result(sharepointEnrollmentState(status), nil)
	}
	var pending *confirmationPending
	if errors.As(err, &pending) {
		return pending.result
	}
	code := "sharepoint_selection_rejected"
	switch {
	case errors.Is(err, errConfirmationUnavailable):
		code = "confirmation_unavailable"
	case errors.Is(err, errConfirmationDeclined):
		code = "confirmation_declined"
	case errors.Is(err, priorwork.ErrSourceSelectionChanged), errors.Is(err, errOwnerReviewChanged):
		code = "revision_conflict"
	}
	out := result(map[string]any{"code": code, "retryable": false, "local_effects_may_exist": attempted, "next_action": "Preserve current source state and re-read sharepoint.enrollment.status. Native review and approved provider admission cannot be supplied by model arguments."}, nil)
	out.IsError = true
	return out
}
