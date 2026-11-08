package product

import (
	"context"
	"errors"
	"path/filepath"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errOwnerReviewChanged = ownerctx.ErrRevisionConflict

func trackSchema() map[string]any {
	return map[string]any{"type": "string", "enum": []string{"quick", "complete"}}
}
func ownerQueries() []any {
	return []any{
		variant("owner.status", map[string]any{}),
		variant("owner.interview", map[string]any{"track": trackSchema()}, "track"),
		variant("owner.review", map[string]any{"proposal_id": strSchema(96)}, "proposal_id"),
		variant("owner.onboarding_review", map[string]any{}),
	}
}
func applySchema() map[string]any {
	s := mutationSchema()
	s["oneOf"] = append(s["oneOf"].([]any),
		variant("owner.track", map[string]any{"track": trackSchema()}, "track"),
		variant("owner.propose", map[string]any{"facet": strSchema(64), "evidence": strSchema(4 << 10), "proposed_body": strSchema(12 << 10)}, "facet", "evidence", "proposed_body"),
		variant("owner.apply", map[string]any{"proposal_id": strSchema(96)}, "proposal_id"),
		variant("owner.confirm_onboarding", map[string]any{}),
	)
	return s
}
func (b *Bound) ownerRoot() string { return filepath.Join(b.home, "brain") }
func ownerMetadata(s ownerctx.Status) map[string]any {
	facets := map[string]any{}
	for id, f := range s.Facets {
		facets[id] = map[string]any{"available": f.Available, "state": f.State, "sensitivity": f.Sensitivity, "readers": f.Readers, "policy": f.Refinement}
	}
	return map[string]any{"initialized": s.Initialized, "onboarding": s.Onboarding, "facets": facets}
}

// InitializeOwner is explicit local setup; opening or querying never calls it.
func (b *Bound) InitializeOwner() (map[string]any, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	s, err := ownerctx.Initialize(b.ownerRoot())
	return ownerMetadata(s), err
}

// OwnerReview and OwnerOnboardingReview expose private bodies only to an
// explicit local operator. MCP handlers project metadata or use trusted UI.
func (b *Bound) OwnerReview(id string) (ownerctx.RefinementReview, error) {
	if err := b.validate(); err != nil {
		return ownerctx.RefinementReview{}, err
	}
	return ownerctx.ReviewRefinement(b.ownerRoot(), id)
}
func (b *Bound) OwnerOnboardingReview() (ownerctx.OnboardingReview, error) {
	if err := b.validate(); err != nil {
		return ownerctx.OnboardingReview{}, err
	}
	return ownerctx.ReviewOnboarding(b.ownerRoot())
}
func (b *Bound) ApplyOwnerReview(id, digest string, confirmed bool) (ownerctx.RefinementReceipt, error) {
	if err := b.validate(); err != nil {
		return ownerctx.RefinementReceipt{}, err
	}
	return ownerctx.ApplyReviewedRefinement(b.ownerRoot(), id, digest, confirmed)
}
func (b *Bound) ConfirmOwnerOnboarding(digest string, confirmed bool) (map[string]any, error) {
	if !confirmed {
		return nil, ownerctx.ErrConfirmationRequired
	}
	if err := b.validate(); err != nil {
		return nil, err
	}
	s, err := ownerctx.ConfirmOnboarding(b.ownerRoot(), digest)
	return ownerMetadata(s), err
}
func (b *Bound) SelectOwnerTrack(track string, confirmed bool) (map[string]any, error) {
	if !confirmed {
		return nil, ownerctx.ErrConfirmationRequired
	}
	if err := b.validate(); err != nil {
		return nil, err
	}
	s, err := ownerctx.SelectOnboardingTrack(b.ownerRoot(), track)
	return ownerMetadata(s), err
}
func (b *Bound) handleOwner(ctx context.Context, req *mcp.CallToolRequest, in input) *mcp.CallToolResult {
	var value any
	var err error
	switch in.Operation {
	case "owner.status":
		var s ownerctx.Status
		s, err = ownerctx.Inspect(b.ownerRoot())
		value = ownerMetadata(s)
	case "owner.interview":
		value = ownerctx.ColdStartInterview()
		if in.Track == ownerctx.OnboardingTrackQuick {
			value = ownerctx.QuickStartInterview()
		}
	case "owner.review", "owner.apply":
		var r ownerctx.RefinementReview
		r, err = b.OwnerReview(in.ProposalID)
		if err != nil {
			break
		}
		value = map[string]any{"id": r.ID, "facet": r.Facet, "policy": r.Policy, "sensitivity": r.Sensitivity, "readers": r.Readers, "base_sha256": r.BaseSHA256, "proposal_sha256": r.ProposalSHA256, "review_digest": r.ReviewDigest}
		if in.Operation == "owner.apply" {
			if err = b.confirm(ctx, req, "Você autoriza substituir esta faceta do seu perfil pelo texto proposto?", r, ownerRefinementPresentation(r)); err == nil {
				value, err = b.ApplyOwnerReview(r.ID, r.ReviewDigest, true)
			}
		}
	case "owner.onboarding_review", "owner.confirm_onboarding":
		var r ownerctx.OnboardingReview
		r, err = b.OwnerOnboardingReview()
		if err != nil {
			break
		}
		value = map[string]any{"track": r.Track, "state": r.State, "facet_count": len(r.Facets), "review_digest": r.ReviewDigest}
		if in.Operation == "owner.confirm_onboarding" {
			if err = b.confirm(ctx, req, "Você revisou e confirma este perfil completo para a trilha escolhida?", r, ownerOnboardingPresentation(r)); err == nil {
				value, err = b.ConfirmOwnerOnboarding(r.ReviewDigest, true)
			}
		}
	case "owner.track":
		var review ownerctx.OnboardingTrackReview
		review, err = ownerctx.ReviewOnboardingTrack(b.ownerRoot(), in.Track)
		if err != nil {
			break
		}
		if err = b.confirm(ctx, req, "Você escolhe esta trilha de entrevista? A escolha exige nova revisão final do perfil.", review, map[string]any{"Trilha atual": ownerReviewLabel(review.CurrentOnboarding.Track), "Trilha escolhida": ownerReviewLabel(review.Track)}); err != nil {
			break
		}
		if err = b.validate(); err == nil {
			var status ownerctx.Status
			status, err = ownerctx.ApplyReviewedOnboardingTrack(b.ownerRoot(), review.Track, review.ReviewDigest, true)
			value = ownerMetadata(status)
		}
	case "owner.propose":
		if !ownerctx.IsOnboardingFacet(in.Facet) {
			err = errors.New("unsupported_facet")
			break
		}
		value, err = ownerctx.SubmitRefinement(b.ownerRoot(), ownerctx.RefinementInput{Facet: in.Facet, Evidence: in.Evidence, ProposedBody: in.ProposedBody})
	default:
		err = errors.New("invalid_operation")
	}
	if err != nil {
		var pending *confirmationPending
		if errors.As(err, &pending) {
			return pending.result
		}
		code := "owner_rejected"
		switch {
		case errors.Is(err, ownerctx.ErrRevisionConflict):
			code = "revision_conflict"
		case errors.Is(err, ownerctx.ErrConfirmationRequired):
			code = "confirmation_required"
		case errors.Is(err, errConfirmationUnavailable):
			code = "confirmation_unavailable"
		case errors.Is(err, errConfirmationDeclined):
			code = "confirmation_declined"
		}
		metadata := map[string]string{"code": code}
		if code == "confirmation_unavailable" {
			metadata["next_action"] = "Use the explicit local owner-review/owner-apply or owner-onboarding-review/owner-confirm-onboarding commands; owner-track selects the interview locally."
		}
		out := result(metadata, nil)
		out.IsError = true
		return out
	}
	return result(value, nil)
}

func ownerRefinementPresentation(r ownerctx.RefinementReview) map[string]any {
	return map[string]any{"Faceta": ownerReviewLabel(r.Facet), "Texto atual": r.CurrentBody, "Texto proposto": r.ProposedBody, "Evidência": r.Evidence, "Sensibilidade": ownerReviewLabel(r.Sensitivity), "Leitores autorizados": ownerReviewReaders(r.Readers), "Regra de atualização": ownerReviewLabel(r.Policy)}
}
func ownerOnboardingPresentation(r ownerctx.OnboardingReview) map[string]any {
	facets := make([]any, 0, len(r.Facets))
	for _, f := range r.Facets {
		facets = append(facets, map[string]any{"Faceta": ownerReviewLabel(f.Facet), "Texto do perfil": f.Body, "Sensibilidade": ownerReviewLabel(f.Sensitivity), "Leitores autorizados": ownerReviewReaders(f.Readers), "Regra de atualização": ownerReviewLabel(f.Policy)})
	}
	return map[string]any{"Trilha escolhida": ownerReviewLabel(r.Track), "Facetas": facets}
}
func ownerReviewReaders(readers []string) []string {
	labels := make([]string, len(readers))
	for i, reader := range readers {
		labels[i] = ownerReviewLabel(reader)
	}
	return labels
}
func ownerReviewLabel(value string) string {
	labels := map[string]string{
		"owner-identity": "Como você prefere ser chamado", "personal-context": "Contexto pessoal autorizado",
		"professional-role": "Papel profissional", "communication-style": "Comunicação",
		"preferences": "Preferências de trabalho", "voice": "Voz em entregas",
		"motivations": "Motivações profissionais", "quality-bar": "Critérios de qualidade",
		"decision-rules": "Regras de decisão", "working-boundaries": "Limites de trabalho",
		"psychological-profile": "Perfil psicológico autorizado",
		"identity":              "Identidade", "professional": "Profissional", "sensitive": "Sensível",
		"session": "Sessões do Maestro", "yoda": "Yoda",
		"confirmation_required": "Exige confirmação", "proposal_only": "Somente propostas para revisão",
		"automatic_with_audit": "Automática com auditoria",
		"quick":                "Curta", "complete": "Completa", "selection_required": "Ainda não escolhida",
	}
	if label, ok := labels[value]; ok {
		return label
	}
	return value
}
