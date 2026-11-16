package product

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceagent"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const maximumAccountNarrativeBytes = 64 << 10

func accountNarrativeQueries() []any {
	return []any{variant("account.narrative.status", map[string]any{}), variant("account.narrative.read", map[string]any{"purpose": strSchema(768)}, "purpose")}
}
func accountNarrativeMutations() []any {
	return []any{variant("account.narrative.save", map[string]any{"text": strSchema(maximumAccountNarrativeBytes), "expected_page_revision": strSchema(64), "case_agent_ids": map[string]any{"type": "array", "minItems": 1, "maxItems": 32, "uniqueItems": true, "items": strSchema(96)}}, "text", "expected_page_revision", "case_agent_ids")}
}

type accountNarrativeOwner struct {
	b            *Bound
	setup        *workspaceagent.SetupOwner
	data         *scheduler.HeldDirectory
	binding      workspaceagent.SetupStatus
	account      agentscaffold.Instance
	engine       *atlasops.Engine
	page         string
	caseBindings []agentscaffold.Instance
}

func (o *accountNarrativeOwner) close() {
	if o.data != nil {
		o.data.Close()
	}
	if o.setup != nil {
		o.setup.Close()
	}
}
func (b *Bound) accountNarrativeOwner() (*accountNarrativeOwner, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	o := &accountNarrativeOwner{b: b}
	var err error
	o.setup, err = workspaceagent.OpenSetup(filepath.Join(b.home, "state"), b.options.WorkspacePath, b.workspaceID)
	if err != nil {
		return nil, err
	}
	fail := func(e error) (*accountNarrativeOwner, error) { o.close(); return nil, e }
	o.binding, err = o.setup.Status()
	if err != nil {
		return fail(err)
	}
	s := o.binding.Setup
	if s == nil || o.binding.State != "active" || s.State != "active" || s.Input.Mode != "client" || o.binding.Markers.Pending != "" {
		return fail(errors.New("account_narrative_scope_unavailable"))
	}
	o.data, err = scheduler.HoldDirectory(filepath.Join(b.home, "state"), nil)
	if err != nil {
		return fail(err)
	}
	status, err := agentscaffold.InspectAt(o.data, s.AccountAgentID)
	if err != nil {
		return fail(err)
	}
	o.account = status.Instance
	if o.account.Role != "client_account_agent" || o.account.ScopeKind != "account" || o.account.ScopeID != s.Input.AccountID {
		return fail(errors.New("account_narrative_scope_unavailable"))
	}
	o.page = "accounts/" + s.Input.AccountID + "/" + s.Input.AccountID + ".md"
	o.engine, err = atlasops.OpenAccountExisting(b.home, s.Input.AccountID, o.validate)
	if err != nil {
		return fail(err)
	}
	return o, nil
}
func (o *accountNarrativeOwner) validate() error {
	if err := o.b.validate(); err != nil {
		return err
	}
	current, err := o.setup.Status()
	if err != nil {
		return err
	}
	if current.State != "active" || current.SetupRevision != o.binding.SetupRevision {
		return errOwnerReviewChanged
	}
	for _, expected := range append([]agentscaffold.Instance{o.account}, o.caseBindings...) {
		actual, err := agentscaffold.InspectAt(o.data, expected.AgentID)
		if err != nil {
			return err
		}
		a, _ := json.Marshal(actual.Instance)
		e, _ := json.Marshal(expected)
		if string(a) != string(e) {
			return errOwnerReviewChanged
		}
	}
	return nil
}
func (o *accountNarrativeOwner) current(purpose string) (*atlasops.PageProjection, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	current, err := o.engine.ReviewPage(atlasops.CollectRequest{Purpose: purpose, Reader: atlasops.ReaderOwnerSession, Pages: []string{o.page}})
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(current.Content) > maximumAccountNarrativeBytes {
		return nil, errors.New("account_narrative_review_too_large")
	}
	if err = o.validate(); err != nil {
		return nil, err
	}
	return &current, nil
}
func (o *accountNarrativeOwner) cases(ids []string) ([]agentscaffold.Instance, error) {
	if len(ids) < 1 || len(ids) > 32 {
		return nil, errors.New("account_case_references_invalid")
	}
	sorted := append([]string{}, ids...)
	sort.Strings(sorted)
	cases := make([]agentscaffold.Instance, 0, len(sorted))
	currentIncluded := false
	for i, id := range sorted {
		if i > 0 && sorted[i-1] == id {
			return nil, errors.New("account_case_references_invalid")
		}
		entry, err := agentscaffold.InspectAt(o.data, id)
		if err != nil {
			return nil, err
		}
		s := entry.Instance
		if s.Role != "case_agent" || s.ScopeKind != "case" || s.AccountAgentID != o.account.AgentID {
			return nil, errors.New("account_case_references_invalid")
		}
		currentIncluded = currentIncluded || s.AgentID == o.binding.Setup.CaseAgentID
		cases = append(cases, s)
	}
	if !currentIncluded {
		return nil, errors.New("account_current_case_required")
	}
	return cases, nil
}
func accountCaseSection(cases []agentscaffold.Instance) string {
	var body strings.Builder
	body.WriteString("## Cases\n\n")
	for _, c := range cases {
		fmt.Fprintf(&body, "- `%s` — account `%s`; case `%s`\n", c.AgentID, strings.TrimPrefix(strings.TrimPrefix(c.AccountAgentID, "client-account-agent-"), "account-agent-"), c.ScopeID)
	}
	return body.String()
}
func accountCasesSection(text string) (string, error) {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	start, end := -1, len(lines)
	for i, line := range lines {
		if line == "## Cases" {
			if start >= 0 {
				return "", errors.New("account_cases_section_invalid")
			}
			start = i
			continue
		}
		if start >= 0 && strings.HasPrefix(line, "## ") && end == len(lines) {
			end = i
		}
	}
	if start < 0 {
		return "", errors.New("account_cases_section_required")
	}
	return strings.TrimRight(strings.Join(lines[start:end], "\n"), "\n") + "\n", nil
}

type accountCaseReference struct {
	AgentID        string `json:"agent_id"`
	AccountAgentID string `json:"account_agent_id"`
	CaseID         string `json:"case_id"`
}

func (o *accountNarrativeOwner) existingCases(section string) ([]accountCaseReference, int) {
	verified := []accountCaseReference{}
	unverified := 0
	seen := map[string]bool{}
	for _, line := range strings.Split(section, "\n") {
		if line == "" || line == "## Cases" {
			continue
		}
		parts := strings.Split(line, "`")
		if len(parts) == 7 && len(verified) < 32 && !seen[parts[1]] {
			entry, err := agentscaffold.InspectAt(o.data, parts[1])
			if err == nil && entry.Instance.Role == "case_agent" && entry.Instance.ScopeKind == "case" && entry.Instance.AccountAgentID == o.account.AgentID && strings.TrimSuffix(strings.TrimPrefix(accountCaseSection([]agentscaffold.Instance{entry.Instance}), "## Cases\n\n"), "\n") == line {
				c := entry.Instance
				verified = append(verified, accountCaseReference{AgentID: c.AgentID, AccountAgentID: c.AccountAgentID, CaseID: c.ScopeID})
				seen[c.AgentID] = true
				continue
			}
		}
		unverified++
	}
	return verified, unverified
}

// A source-backed case reference names only its immutable local registration.
// It is not a resolvable foreign brief link or a grant to cross-case knowledge.
type accountNarrativeReview struct {
	WorkspaceID         string                   `json:"workspace_id"`
	SetupRevision       string                   `json:"setup_revision"`
	Account             agentscaffold.Instance   `json:"account_registration"`
	Page                string                   `json:"page"`
	ExpectedRevision    string                   `json:"expected_page_revision"`
	Current             *atlasops.PageProjection `json:"current"`
	ExistingCases       []accountCaseReference   `json:"existing_verified_case_registrations"`
	UnverifiedCaseLines int                      `json:"existing_unverified_case_line_count"`
	LegacyNotice        string                   `json:"legacy_case_reference_notice"`
	ProposedText        string                   `json:"proposed_text"`
	Cases               []agentscaffold.Instance `json:"proposed_case_registrations"`
	Provenance          atlasops.Provenance      `json:"provenance"`
	Replay              bool                     `json:"replay"`
	Digest              string                   `json:"-"`
}

func (o *accountNarrativeOwner) review(in input) (accountNarrativeReview, error) {
	r := accountNarrativeReview{WorkspaceID: o.b.workspaceID, SetupRevision: o.binding.SetupRevision, Account: o.account, Page: o.page, ExpectedRevision: in.ExpectedPageRevision, ProposedText: in.Text}
	if len(in.Text) == 0 || len(in.Text) > maximumAccountNarrativeBytes {
		return r, errors.New("account_narrative_body_invalid")
	}
	meta, err := atlas.ValidateAuthoredPage(o.page, o.b.workspaceID, in.Text)
	if err != nil || meta["type"] != "account" {
		return r, errors.New("account_narrative_metadata_invalid")
	}
	r.Cases, err = o.cases(in.CaseAgentIDs)
	if err != nil {
		return r, err
	}
	section, err := accountCasesSection(in.Text)
	if err != nil {
		return r, err
	}
	if section != accountCaseSection(r.Cases) {
		return r, errors.New("account_case_references_invalid")
	}
	r.Current, err = o.current("Review the complete current shared account narrative")
	if err != nil {
		return r, err
	}
	if r.Current != nil {
		section, _ := accountCasesSection(r.Current.Content)
		r.ExistingCases, r.UnverifiedCaseLines = o.existingCases(section)
		r.LegacyNotice = "Existing verified references are bounded to 32 unique registrations. Remaining legacy or duplicate lines are unverified; their exact bytes remain in the complete current page. Saving replaces the full displayed page and its Cases section."
	}
	// The exact logical request survives cold retries; current page bytes and
	// registration snapshots bind review, not the retained operation identity.
	intent, _ := json.Marshal([]any{o.b.home, o.account.AgentID, in.ExpectedPageRevision, in.Text, in.CaseAgentIDs})
	id := contentDigest(intent)
	r.Provenance = atlasops.Provenance{Origin: atlasops.OriginAttended, SessionID: "account-narrative-" + id, IdempotencyKey: id, Ritual: "account-narrative"}
	if in.ExpectedPageRevision == "absent" {
		if r.Current != nil && r.Current.Content != in.Text {
			return r, errOwnerReviewChanged
		}
	} else {
		if !validContentDigest(in.ExpectedPageRevision) || r.Current == nil {
			return r, errOwnerReviewChanged
		}
		r.Replay, err = o.engine.ReviewUpdateReplay(atlasops.UpdatePageRequest{Page: o.page, Body: in.Text, ExpectedRevision: in.ExpectedPageRevision, Provenance: r.Provenance})
		if err != nil {
			return r, err
		}
		if r.Current.Revision != in.ExpectedPageRevision && !r.Replay {
			return r, errOwnerReviewChanged
		}
	}
	body, _ := json.Marshal(r)
	r.Digest = contentDigest(body)
	return r, o.validate()
}
func (b *Bound) handleAccountNarrative(ctx context.Context, req *mcp.CallToolRequest, in input) *mcp.CallToolResult {
	fail := func(err error) *mcp.CallToolResult {
		var pending *confirmationPending
		if errors.As(err, &pending) {
			return pending.result
		}
		code := "account_narrative_rejected"
		switch {
		case errors.Is(err, errConfirmationUnavailable):
			code = "confirmation_unavailable"
		case errors.Is(err, errConfirmationDeclined):
			code = "confirmation_declined"
		case errors.Is(err, errOwnerReviewChanged):
			code = "revision_conflict"
		}
		out := result(map[string]any{"code": code, "retryable": false, "next_action": "Preserve the account narrative. Inspect the current active client setup and page revision, then review the exact full account save again."}, nil)
		out.IsError = true
		return out
	}
	if in.Operation == "account.narrative.save" && len(req.Params.Arguments) > 512<<10 {
		return fail(errors.New("account_request_too_large"))
	}
	o, err := b.accountNarrativeOwner()
	if err != nil {
		return fail(err)
	}
	defer o.close()
	if in.Operation != "account.narrative.save" {
		purpose := "Inspect the shared account narrative metadata"
		if in.Operation == "account.narrative.read" {
			purpose = in.Purpose
		}
		current, err := o.current(purpose)
		if err != nil {
			return fail(err)
		}
		value := map[string]any{"account_id": o.account.ScopeID, "account_agent_id": o.account.AgentID, "page": o.page, "scope": "account/" + o.account.ScopeID, "state": "absent", "revision": "absent", "current_case_agent_id": o.binding.Setup.CaseAgentID, "current_case_id": o.binding.Setup.CaseID, "current_case_brief": o.binding.Setup.BriefPage, "workspace_id": b.workspaceID, "foreign_case_knowledge": "unavailable"}
		if current != nil {
			value["state"] = "present"
			value["revision"] = current.Revision
			value["pointer"] = current.Pointer
			if in.Operation == "account.narrative.read" {
				value["text"] = current.Content
			}
		}
		return result(value, nil)
	}
	review, err := o.review(in)
	if err != nil {
		return fail(err)
	}
	if err = b.confirm(ctx, req, "Authorize this exact shared client-account narrative? Current and proposed text and case registration references are shown completely. This preserves the separate case briefs and grants no foreign-case read, runtime activation or promotion.", review, review); err != nil {
		return fail(err)
	}
	o.caseBindings = review.Cases
	var applied atlasops.Result
	err = b.guardPublication(func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		fresh, err := o.review(in)
		if err != nil {
			return err
		}
		if fresh.Digest != review.Digest {
			return errOwnerReviewChanged
		}
		if in.ExpectedPageRevision == "absent" {
			applied, err = o.engine.CreatePage(atlasops.CreatePageRequest{Page: o.page, Body: in.Text, Provenance: review.Provenance})
		} else {
			applied, err = o.engine.UpdatePage(atlasops.UpdatePageRequest{Page: o.page, Body: in.Text, ExpectedRevision: in.ExpectedPageRevision, Provenance: review.Provenance})
		}
		if err != nil {
			return err
		}
		if applied.State == atlasops.StateProposed {
			return errOwnerReviewChanged
		}
		return o.validate()
	})
	if err != nil {
		return fail(err)
	}
	read, err := o.current("Verify the exact saved shared account narrative")
	if err != nil || read == nil || read.Revision != applied.Revision || read.Content != in.Text {
		if err == nil {
			err = errOwnerReviewChanged
		}
		return fail(err)
	}
	return result(applied, nil)
}
