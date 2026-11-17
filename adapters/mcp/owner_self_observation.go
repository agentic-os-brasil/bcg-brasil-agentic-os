package product

import (
	"context"
	"errors"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type selfObservationArguments struct {
	SelfObservation *ownerctx.SelfObservationDeclaration `json:"self_observation"`
	IntakeID        string                               `json:"intake_id"`
	Next            string                               `json:"next"`
}

func ownerSelfObservationQueries() []any {
	return []any{variant("owner.observation.status", map[string]any{
		"offset": map[string]any{"type": "integer", "minimum": 0, "maximum": 1024},
		"limit":  map[string]any{"type": "integer", "minimum": 1, "maximum": 32},
	})}
}
func ownerSelfObservationMutations() []any {
	declaration := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"signal", "facet", "claim", "confidence", "expires_at"}, "properties": map[string]any{
		"signal":     map[string]any{"type": "string", "enum": []string{"explicit_instruction", "explicit_correction", "explicit_endorsement"}},
		"facet":      map[string]any{"type": "string", "enum": []string{"professional-role", "communication-style", "voice", "preferences", "motivations", "quality-bar", "decision-rules", "working-boundaries"}},
		"claim":      map[string]any{"type": "string", "pattern": `^[a-z0-9][a-z0-9_.:-]{0,127}$`},
		"confidence": map[string]any{"type": "number", "minimum": 0, "maximum": 1},
		"expires_at": map[string]any{"type": "string", "format": "date-time", "maxLength": 64},
	}}
	return []any{
		variant("owner.observation.intake", map[string]any{"self_observation": declaration}, "self_observation"),
		variant("owner.observation.reject", map[string]any{"intake_id": strSchema(96), "next": map[string]any{"type": "string", "enum": []string{"rejected", "redacted"}}}, "intake_id", "next"),
	}
}

func (b *Bound) ownerSelfObservation(ctx context.Context, req *mcp.CallToolRequest, in input) (any, error) {
	switch in.Operation {
	case "owner.observation.status":
		return b.ownerSelfObservationStatus(in)
	case "owner.observation.intake":
		if in.SelfObservation == nil {
			return nil, errors.New("owner declaration required")
		}
		submission, err := ownerctx.NewSelfObservationDeclaration(b.ownerRoot(), *in.SelfObservation)
		if err != nil {
			return nil, err
		}
		review, err := ownerctx.ReviewSelfObservation(b.ownerRoot(), submission)
		if err != nil {
			return nil, err
		}
		if review.CurrentState == "rejected" || review.CurrentState == "redacted" {
			return nil, errors.New("owner declaration is terminal")
		}
		presentation := selfObservationPresentation(review.Submission, review.CurrentState)
		presentation["Atestação do dono para esta entrada"] = "Confirmo que este código representa uma declaração minha, relevante e útil para o trabalho futuro, sem conteúdo de cliente, documento ou conversa. Autorizo seu uso profissional global do dono somente como insumo pendente para proposta."
		presentation["Limite desta confirmação"] = "Esta é uma atestação explícita do dono; não é sanitização automática, comprovação de origem nativa ou de episódio independente."
		if err = b.confirm(ctx, req, "Você confirma que esta declaração é sua, relevante para o trabalho futuro e sem conteúdo de cliente, documento ou conversa, e autoriza seu uso profissional global somente como insumo pendente para proposta? O perfil ativo não será alterado.", review, presentation); err != nil {
			return nil, err
		}
		var receipt ownerctx.SelfObservationIntakeReceipt
		err = b.guardPublication(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := b.validate(); err != nil {
				return err
			}
			var err error
			receipt, err = ownerctx.ApproveSelfObservation(ctx, b.ownerRoot(), review, true)
			return err
		})
		return receipt, err
	case "owner.observation.reject":
		review, err := ownerctx.ReviewSelfObservationControl(b.ownerRoot(), in.IntakeID, in.Next)
		if err != nil {
			return nil, err
		}
		presentation := selfObservationPresentation(review.Submission, review.IntakeState)
		presentation["Novo estado"] = review.Next
		presentation["Efeito"] = "Registrar a rejeição ou remoção desta declaração e impedir captura futura; preservar o perfil ativo."
		presentation["Revisão atual da entrada"] = review.IntakeRevision
		presentation["Estado da observação canônica"] = review.ObservationState
		if err = b.confirm(ctx, req, "Você autoriza rejeitar ou remover esta declaração e impedir sua captura futura? O perfil ativo permanece preservado.", review, presentation); err != nil {
			return nil, err
		}
		var receipt ownerctx.SelfObservationIntakeReceipt
		err = b.guardPublication(func() error {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := b.validate(); err != nil {
				return err
			}
			var err error
			receipt, err = ownerctx.ApplySelfObservationControl(ctx, b.ownerRoot(), review, true)
			return err
		})
		return receipt, err
	}
	return nil, errors.New("invalid observation operation")
}
func selfObservationPresentation(submission ownerctx.SelfObservationSubmission, current any) map[string]any {
	return map[string]any{
		"Origem": "Declaração explícita revisada pelo dono; não comprova observação nativa ou episódio independente.",
		"Escopo": "Profissional global do dono; somente insumo de proposta, nunca perfil ativo.",
		"Sinal":  submission.Signal, "Faceta": ownerReviewLabel(submission.Facet), "Código normalizado": submission.Claim,
		"Confiança declarada": submission.Confidence, "Expira em": submission.ExpiresAt, "Estado atual": current,
		"Efeito": "Guardar somente a entrada pendente. A captura canônica exige qualificação e inscrição separadas de self-observation-capture.",
	}
}
func (b *Bound) ownerSelfObservationStatus(in input) (any, error) {
	now := time.Now().UTC()
	source, err := ownerctx.InspectSelfObservationSource(b.ownerRoot(), now)
	if err != nil {
		return nil, err
	}
	observations, err := ownerctx.ListObservations(b.ownerRoot())
	if err != nil {
		return nil, err
	}
	current, err := ownerctx.InspectSelfObservationSource(b.ownerRoot(), now)
	if err != nil {
		return nil, err
	}
	if current.Fingerprint != source.Fingerprint {
		return nil, ownerctx.ErrRevisionConflict
	}
	limit := in.Limit
	if limit == 0 {
		limit = 16
	}
	if in.Offset < 0 || in.Offset > len(source.Intakes) || limit < 1 || limit > 32 {
		return nil, errors.New("owner observation page invalid")
	}
	pending := map[string]bool{}
	for _, candidate := range source.Pending {
		pending[candidate.ID] = true
	}
	canonical := map[string]ownerctx.ObservationReceipt{}
	for _, observation := range observations {
		canonical[observation.ID] = observation
	}
	end := min(in.Offset+limit, len(source.Intakes))
	items := make([]map[string]any, 0, end-in.Offset)
	for _, entry := range source.Intakes[in.Offset:end] {
		state, revision := "absent", ""
		observation, exists := canonical[entry.ObservationID]
		if exists {
			state, revision = string(observation.State), observation.Revision
		}
		items = append(items, map[string]any{"id": entry.ID, "observation_id": entry.ObservationID, "intake_state": entry.State, "intake_revision": entry.Revision, "observation_state": state, "observation_revision": revision, "capture_pending": pending[entry.ID] && !exists})
	}
	return map[string]any{"items": items, "total": len(source.Intakes), "offset": in.Offset, "next_offset": end, "has_more": end < len(source.Intakes), "source_fingerprint": source.Fingerprint, "source_class": "owner_reviewed_declaration", "scope": "owner_global_professional_proposal_input", "independent_native_episodes": false}, nil
}
