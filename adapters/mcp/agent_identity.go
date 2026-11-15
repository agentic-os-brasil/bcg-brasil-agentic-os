package product

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errIdentityInstance = errors.New("identity instance registration is unavailable")

func agentIdentityQueries() []any {
	return []any{
		variant("agent.identity.interview", map[string]any{}),
		variant("agent.identity.review", map[string]any{"proposal_id": strSchema(96)}, "proposal_id"),
	}
}
func agentIdentityMutations() []any {
	var variants []any
	for _, operation := range []string{"agent.identity.propose", "agent.identity.answer"} {
		fields := map[string]any{
			"owner_id": strSchema(80), "role": map[string]any{"type": "string", "enum": []string{"maestro", "yoda", "darwin", "client_account_agent", "case_agent", "quality_guardian", "pa_expert"}},
			"agent_id": strSchema(80), "display_name": strSchema(80), "emoji": strSchema(16), "question_id": strSchema(64), "expected_profile_revision": strSchema(64),
		}
		variants = append(variants, variant(operation, fields, "owner_id", "role", "display_name", "emoji", "question_id", "expected_profile_revision"))
	}
	return append(variants, variant("agent.identity.apply", map[string]any{"proposal_id": strSchema(96)}, "proposal_id"))
}
func (b *Bound) identityRoot() string { return filepath.Join(b.home, "state") }

func identityDraftMetadata(draft agentidentity.ProfileDraft) map[string]any {
	return map[string]any{"proposal_id": draft.ID, "state": draft.State, "owner_id": draft.Profile.OwnerID, "selection_count": len(draft.Profile.Selections), "presentation_only": true, "runtime_activated": false}
}

// Only changed instance selections need fresh registration admission. Existing
// untouched preferences are not promoted into runtime or scope authority.
func (b *Bound) admitIdentityChoices(current *agentidentity.Profile, selections []agentidentity.Selection) error {
	for _, selection := range selections {
		unchanged := false
		if current != nil {
			for _, before := range current.Selections {
				comparison := before
				comparison.Role = agentidentity.CanonicalRole(comparison.Role)
				if comparison == selection {
					unchanged = true
					break
				}
			}
		}
		if unchanged {
			continue
		}
		switch selection.Role {
		case "client_account_agent", "case_agent", "pa_expert":
			if selection.AgentID == "" {
				return errIdentityInstance
			}
			registered, err := agentscaffold.Inspect(b.identityRoot(), selection.AgentID)
			if err != nil || !registered.Initialized || registered.Instance.Role != selection.Role || registered.Instance.OwnerID != selection.OwnerID || registered.Instance.OwnershipScope != selection.OwnershipScope {
				return errIdentityInstance
			}
			if selection.Role == "case_agent" && (registered.Instance.ScopeID != b.workspaceID || registered.Instance.ScopeKind != "workspace") {
				return errIdentityInstance
			}
		}
	}
	return nil
}

// Canonical IDs and roles remain immutable; only display preferences join
// ordinary context. Invalid optional preferences are explicit and do not
// masquerade as a new identity interview or block unrelated workspace work.
func (b *Bound) managedIdentityContext() map[string]any {
	state, err := agentidentity.ReadProfile(b.identityRoot())
	status := "defaults_active"
	profile := agentidentity.Profile{}
	if err != nil {
		status = "profile_invalid"
	} else if state.Profile != nil {
		status = "profile_confirmed"
		profile = *state.Profile
	}
	managed := []map[string]string{}
	for _, selection := range agentidentity.ResolveManaged(profile) {
		managed = append(managed, map[string]string{"agent_id": selection.AgentID, "role": selection.Role, "display_name": selection.DisplayName, "emoji": selection.Emoji})
	}
	out := map[string]any{"state": status, "managed": managed, "presentation_only": true}
	if err == nil {
		out["profile_revision"] = state.Revision
	} else {
		out["next_action"] = "Preserve the invalid identity profile; identity onboarding is blocked until its canonical state is inspected."
	}
	return out
}

func (b *Bound) handleAgentIdentity(ctx context.Context, req *mcp.CallToolRequest, in input) *mcp.CallToolResult {
	var value any
	var err error
	var committed agentidentity.ProfileDraft
	switch in.Operation {
	case "agent.identity.interview":
		var state agentidentity.ProfileState
		state, err = agentidentity.ReadProfile(b.identityRoot())
		if err == nil {
			guided := agentidentity.GuidedIdentityInterview(b.identityRoot())
			if guided.State == "blocked" {
				err = agentidentity.ErrIdentityState
				break
			}
			var fresh agentidentity.ProfileState
			fresh, err = agentidentity.ReadProfile(b.identityRoot())
			if err == nil && state.Revision != fresh.Revision {
				err = agentidentity.ErrIdentityChanged
			}
			value = map[string]any{"state": guided.State, "next_question": guided.NextQuestion, "open_draft_id": guided.OpenDraftID, "guidance": guided.Guidance, "catalog": map[string]any{"kind": guided.Catalog.Kind, "schema_version": guided.Catalog.SchemaVersion, "agents": guided.Catalog.Agents, "ownership_explanation": guided.Catalog.OwnershipExplanation, "avatar_explanation": guided.Catalog.AvatarExplanation}, "profile_revision": state.Revision, "profile": state.Profile, "presentation_only": true, "runtime_activated": false}
		}
	case "agent.identity.review", "agent.identity.apply":
		var review agentidentity.DraftReview
		review, err = agentidentity.PrepareDraftReview(b.identityRoot(), in.ProposalID)
		if err != nil {
			break
		}
		if in.Operation == "agent.identity.review" {
			value = identityDraftMetadata(review.Draft)
			break
		}
		if err = b.admitIdentityChoices(review.Current.Profile, review.Draft.Profile.Selections); err != nil {
			break
		}
		presentation := map[string]any{"operation": in.Operation, "current_profile": review.Current.Profile, "proposed_profile": review.Draft.Profile, "draft_state": review.Draft.State, "scope": "Confirm these exact owner presentation preferences and attest that they contain no client data. Names and emoji do not activate runtimes, grant tools, delegate work or widen scope."}
		err = b.confirm(ctx, req, "Confirm this exact identity draft and owner-only data attestation?", review, presentation)
		if err == nil {
			committed, err = agentidentity.CommitDraftReview(b.identityRoot(), in.ProposalID, review.Digest, func() error {
				if e := ctx.Err(); e != nil {
					return e
				}
				if e := b.validate(); e != nil {
					return e
				}
				return b.admitIdentityChoices(review.Current.Profile, review.Draft.Profile.Selections)
			})
			value = identityDraftMetadata(committed)
		}
	case "agent.identity.propose", "agent.identity.answer":
		answer := agentidentity.Answer{OwnerID: in.IdentityOwnerID, Role: in.IdentityRole, AgentID: in.IdentityAgentID, DisplayName: in.IdentityDisplayName, Emoji: in.IdentityEmoji, QuestionID: in.IdentityQuestionID, ExpectedRevision: in.IdentityExpectedRevision}
		var review agentidentity.AnswerReview
		review, err = agentidentity.PrepareAnswer(b.identityRoot(), answer)
		if err != nil {
			break
		}
		if err = b.admitIdentityChoices(review.Current.Profile, review.Proposed.Selections); err != nil {
			break
		}
		effect := "Create this exact canonical identity draft only; the confirmed profile stays unchanged."
		if in.Operation == "agent.identity.answer" {
			effect = "Create and confirm this exact identity answer through the canonical draft lifecycle."
		}
		presentation := map[string]any{"operation": in.Operation, "question_id": answer.QuestionID, "current_profile": review.Current.Profile, "proposed_profile": review.Proposed, "effect": effect, "scope": "Confirm these exact owner presentation preferences and attest that they contain no client data. Names and emoji do not activate runtimes, grant tools, delegate work or widen scope. Server timestamps and receipts are assigned after confirmation."}
		confirmation := struct {
			Operation string
			Review    agentidentity.AnswerReview
		}{in.Operation, review}
		err = b.confirm(ctx, req, "Authorize this exact identity answer and owner-only data attestation?", confirmation, presentation)
		if err == nil {
			committed, err = agentidentity.CommitAnswer(b.identityRoot(), answer, review.Digest, in.Operation == "agent.identity.answer", func() error {
				if e := ctx.Err(); e != nil {
					return e
				}
				if e := b.validate(); e != nil {
					return e
				}
				return b.admitIdentityChoices(review.Current.Profile, review.Proposed.Selections)
			})
			value = identityDraftMetadata(committed)
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
	code := "identity_rejected"
	switch {
	case errors.Is(err, errConfirmationUnavailable):
		code = "confirmation_unavailable"
	case errors.Is(err, errConfirmationDeclined):
		code = "confirmation_declined"
	case errors.Is(err, agentidentity.ErrIdentityChanged), errors.Is(err, errOwnerReviewChanged):
		code = "revision_conflict"
	case errors.Is(err, agentidentity.ErrIdentityState):
		code = "identity_state_invalid"
	case errors.Is(err, agentidentity.ErrIdentityDraftOpen):
		code = "identity_review_required"
	case errors.Is(err, errIdentityInstance):
		code = "identity_instance_unavailable"
	}
	failure := map[string]any{"code": code, "retryable": false, "next_action": "Read agent.identity.interview and preserve the current profile. Native review and an admitted instance registration cannot be replaced by model arguments."}
	if committed.ID != "" {
		failure["proposal_id"] = committed.ID
		failure["draft_state"] = committed.State
		failure["recovery_required"] = true
	}
	out := result(failure, nil)
	out.IsError = true
	return out
}
