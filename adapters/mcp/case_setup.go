package product

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceagent"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func caseSetupQueries() []any { return []any{variant("case.setup.status", map[string]any{})} }
func caseSetupMutations() []any {
	return []any{
		variant("case.setup.prepare", map[string]any{"mode": map[string]any{"type": "string", "enum": []string{"client", "internal"}}, "project_name": strSchema(256), "account_id": strSchema(64), "account_owner": strSchema(64), "account_mandate": strSchema(500), "expected_setup_revision": strSchema(64)}, "mode", "project_name", "expected_setup_revision"),
		variant("case.setup.finish", map[string]any{"expected_setup_revision": strSchema(64), "expected_page_revision": strSchema(64), "objective": strSchema(4096), "next_step": strSchema(768), "summary": strSchema(768)}, "expected_setup_revision", "expected_page_revision"),
	}
}
func (b *Bound) handleCaseSetup(ctx context.Context, req *mcp.CallToolRequest, in input) *mcp.CallToolResult {
	owner, err := workspaceagent.OpenSetup(filepath.Join(b.home, "state"), b.options.WorkspacePath, b.workspaceID)
	if err != nil {
		out := result(map[string]any{"code": "case_setup_unavailable", "retryable": false}, nil)
		out.IsError = true
		return out
	}
	defer owner.Close()
	failure := func(err error) *mcp.CallToolResult {
		var pending *confirmationPending
		if errors.As(err, &pending) {
			return pending.result
		}
		code := "case_setup_rejected"
		switch {
		case errors.Is(err, errConfirmationUnavailable):
			code = "confirmation_unavailable"
		case errors.Is(err, errConfirmationDeclined):
			code = "confirmation_declined"
		case err.Error() == "case_work_projection_invalid":
			code = "case_work_projection_invalid"
		case errors.Is(err, workspaceagent.ErrSetupChanged), errors.Is(err, errOwnerReviewChanged):
			code = "case_setup_changed"
		}
		value := owner.PartialStatus()
		value["code"] = code
		if code == "case_work_projection_invalid" {
			value["next_action"] = "Shorten the proposed checkpoint summary and next step, then review finish again. No work intent was saved."
		}
		out := result(value, nil)
		out.IsError = true
		return out
	}
	if in.Operation == "case.setup.status" {
		status, err := owner.Status()
		if err != nil {
			return failure(err)
		}
		return result(status, nil)
	}
	var review workspaceagent.SetupReview
	if in.Operation == "case.setup.prepare" {
		review, err = owner.ReviewPrepare(workspaceagent.SetupInput{Mode: in.Mode, ProjectName: in.ProjectName, AccountID: in.AccountID, AccountOwner: in.AccountOwner, AccountMandate: in.AccountMandate}, in.ExpectedSetupRevision)
	} else {
		status, e := owner.Status()
		err = e
		if err == nil && status.Setup == nil {
			err = errors.New("case_prepare_required")
		}
		if err == nil {
			read := func() (workspaceagent.SetupPage, error) {
				engine, e := b.knowledgeEngine()
				if e != nil {
					return workspaceagent.SetupPage{}, e
				}
				page, e := engine.ReviewPageRetained(atlasops.CollectRequest{Purpose: "Review the exact first-case brief before setup completion", Reader: atlasops.ReaderOwnerSession, Pages: []string{status.Setup.BriefPage}}, owner.BrainDirectory())
				return workspaceagent.SetupPage{Page: page.Page, Revision: page.Revision, Text: page.Content}, e
			}
			page, e := read()
			err = e
			if err == nil && page.Revision != in.ExpectedPageRevision {
				err = workspaceagent.ErrSetupChanged
			}
			var work *workspaceagent.SetupWork
			if in.Objective != "" || in.NextStep != "" || in.Summary != "" {
				work = &workspaceagent.SetupWork{Objective: in.Objective, NextStep: in.NextStep, Summary: in.Summary}
			}
			if err == nil {
				review, err = owner.ReviewFinish(in.ExpectedSetupRevision, page, work)
			}
			if err == nil {
				err = b.confirm(ctx, req, "Authorize this exact first-case completion? Registration is local only; requested work will be checkpointed and paused. No runtime activation or external action.", review, review)
				if err == nil {
					var out workspaceagent.SetupStatus
					out, err = owner.ApplyFinish(review, read)
					if err == nil {
						err = b.validate()
					}
					if err == nil {
						return result(out, nil)
					}
				}
			}
		}
		return failure(err)
	}
	if err == nil {
		err = b.confirm(ctx, req, "Authorize this exact local first-case registration and pending selection? This does not activate agent runtime, grant tools or import account/case content.", review, review)
	}
	if err == nil {
		var out workspaceagent.SetupStatus
		out, err = owner.ApplyPrepare(review)
		if err == nil {
			err = b.validate()
		}
		if err == nil {
			return result(out, nil)
		}
	}
	return failure(err)
}
