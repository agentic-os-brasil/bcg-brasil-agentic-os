package product

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var errMemoryIneligible = errors.New("memory_ineligible")

func memoryPageFields() map[string]any {
	return map[string]any{"offset": map[string]any{"type": "integer", "minimum": 0, "maximum": 4096}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 32}}
}
func memoryTextFields() map[string]any {
	return map[string]any{"offset": map[string]any{"type": "integer", "minimum": 0, "maximum": 65536}, "max_bytes": map[string]any{"type": "integer", "minimum": 4, "maximum": 4096}}
}
func memoryQueries() []any {
	read := memoryTextFields()
	read["capture_id"] = strSchema(32)
	read["expected_revision"] = map[string]any{"type": "integer", "minimum": 1}
	search := memoryPageFields()
	search["query"] = strSchema(256)
	return []any{variant("memory.list", memoryPageFields()), variant("memory.search", search, "query"), variant("memory.read", read, "capture_id"), variant("memory.context", memoryTextFields())}
}
func memoryMutations() []any {
	return []any{
		variant("memory.capture", map[string]any{"text": strSchema(memory.MaxOwnedCaptureBytes)}, "text"),
		variant("memory.correct", map[string]any{"capture_id": strSchema(32), "expected_revision": map[string]any{"type": "integer", "minimum": 1}, "text": strSchema(memory.MaxOwnedCaptureBytes)}, "capture_id", "expected_revision", "text"),
		variant("memory.delete", map[string]any{"capture_id": strSchema(32), "expected_revision": map[string]any{"type": "integer", "minimum": 1}}, "capture_id", "expected_revision"),
		variant("memory.retention", map[string]any{"cutoff": strSchema(40), "expected_visibility_revision": map[string]any{"type": "integer", "minimum": 0}}, "cutoff", "expected_visibility_revision"),
		variant("memory.consolidate", map[string]any{"cycle": map[string]any{"type": "string", "enum": []string{"daily", "weekly"}}, "date": strSchema(10)}, "cycle", "date"),
	}
}
func productApplySchema() map[string]any {
	s := applySchema()
	s["oneOf"] = append(s["oneOf"].([]any), append(append(memoryMutations(), contentMutations()...), maintenanceMutations()...)...)
	return s
}
func (b *Bound) memoryEngine() (*memory.Engine, basememory.RuntimeConfig, error) {
	if err := b.validate(); err != nil {
		return nil, basememory.RuntimeConfig{}, err
	}
	engine, configuration, err := maintenance.NewProductMemoryEngine(b.home)
	if err == nil {
		engine.PublicationGuard = b.guardPublication
	}
	return engine, configuration, err
}

// InitializeMemory is explicit operator setup; reads never prepare a producer key.
func (b *Bound) InitializeMemory() error {
	e, _, err := b.memoryEngine()
	if err != nil {
		return err
	}
	return e.PrepareOwnedCaptureAuthority(b.workspaceID)
}

// MemoryMutation is an operator request, not a capability or a sealed capture.
// Text reaches the local command through stdin; timestamps and provenance are server-owned.
type MemoryMutation struct {
	Operation                  string `json:"operation"`
	Text                       string `json:"text,omitempty"`
	CaptureID                  string `json:"capture_id,omitempty"`
	ExpectedRevision           uint64 `json:"expected_revision,omitempty"`
	Cutoff                     string `json:"cutoff,omitempty"`
	ExpectedVisibilityRevision uint64 `json:"expected_visibility_revision,omitempty"`
}

// DecodeMemoryMutation shares the closed tool schema with the stdin-only
// operator fallback, including required zero-valued visibility revisions.
func DecodeMemoryMutation(body []byte) (MemoryMutation, error) {
	if len(body) > 128<<10 || !utf8.Valid(body) {
		return MemoryMutation{}, errors.New("memory_invalid_input")
	}
	var raw any
	if err := json.Unmarshal(body, &raw); err != nil {
		return MemoryMutation{}, err
	}
	schemaBody, err := json.Marshal(map[string]any{"type": "object", "oneOf": memoryMutations()})
	if err != nil {
		return MemoryMutation{}, err
	}
	var definition jsonschema.Schema
	if err = json.Unmarshal(schemaBody, &definition); err != nil {
		return MemoryMutation{}, err
	}
	resolved, err := definition.Resolve(nil)
	if err != nil {
		return MemoryMutation{}, err
	}
	if err = resolved.Validate(raw); err != nil {
		return MemoryMutation{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var in MemoryMutation
	if err = decoder.Decode(&in); err != nil {
		return MemoryMutation{}, err
	}
	if decoder.Decode(new(any)) != io.EOF {
		return MemoryMutation{}, errors.New("memory_trailing_input")
	}
	return in, nil
}

type MemoryReview struct {
	Scope                string                `json:"scope"`
	WorkspaceID          string                `json:"workspace_id"`
	Mutation             MemoryMutation        `json:"mutation"`
	VisibilityRevision   uint64                `json:"visibility_revision"`
	Current              *memory.CaptureRecord `json:"current,omitempty"`
	RetentionCount       int                   `json:"retention_count,omitempty"`
	RetentionFingerprint string                `json:"retention_fingerprint,omitempty"`
	RetentionFirst       *time.Time            `json:"retention_first,omitempty"`
	RetentionLast        *time.Time            `json:"retention_last,omitempty"`
	ReviewDigest         string                `json:"review_digest"`
}

func (b *Bound) MemoryReview(in MemoryMutation) (MemoryReview, error) {
	e, _, err := b.memoryEngine()
	if err != nil {
		return MemoryReview{}, err
	}
	rev, err := e.VisibilityRevision(b.workspaceID)
	if err != nil {
		return MemoryReview{}, err
	}
	r := MemoryReview{Scope: b.home, WorkspaceID: b.workspaceID, Mutation: in, VisibilityRevision: rev}
	needsText := in.Operation == "memory.capture" || in.Operation == "memory.correct"
	if needsText && (strings.TrimSpace(in.Text) == "" || len(in.Text) > memory.MaxOwnedCaptureBytes || !utf8.ValidString(in.Text) || strings.ContainsRune(in.Text, 0)) {
		return MemoryReview{}, errors.New("memory_invalid_input")
	}
	if !needsText && in.Text != "" {
		return MemoryReview{}, errors.New("memory_invalid_input")
	}
	switch in.Operation {
	case "memory.capture":
		if in.CaptureID != "" || in.ExpectedRevision != 0 || in.Cutoff != "" || in.ExpectedVisibilityRevision != 0 {
			return MemoryReview{}, errors.New("memory_invalid_input")
		}
	case "memory.correct", "memory.delete":
		if in.CaptureID == "" || in.ExpectedRevision == 0 || in.Cutoff != "" || in.ExpectedVisibilityRevision != 0 {
			return MemoryReview{}, errors.New("memory_invalid_input")
		}
		record, err := e.ReadCapture(b.workspaceID, in.CaptureID)
		if err != nil {
			return MemoryReview{}, err
		}
		if record.Revision != in.ExpectedRevision {
			return MemoryReview{}, memory.ErrCaptureConflict
		}
		r.Current = &record
	case "memory.retention":
		if in.CaptureID != "" || in.ExpectedRevision != 0 {
			return MemoryReview{}, errors.New("memory_invalid_input")
		}
		cutoff, err := time.Parse(time.RFC3339Nano, in.Cutoff)
		if err != nil || cutoff.IsZero() {
			return MemoryReview{}, errors.New("memory_invalid_cutoff")
		}
		if rev != in.ExpectedVisibilityRevision {
			return MemoryReview{}, memory.ErrCaptureConflict
		}
		var affected []memory.CaptureReceipt
		for offset := 0; ; {
			page, err := e.ListCaptures(b.workspaceID, offset, 100)
			if err != nil {
				return MemoryReview{}, err
			}
			if page.VisibilityRevision != rev {
				return MemoryReview{}, memory.ErrCaptureConflict
			}
			for _, receipt := range page.Captures {
				record, err := e.ReadCapture(b.workspaceID, receipt.ID)
				if err != nil {
					return MemoryReview{}, err
				}
				if record.RecordedAt.Before(cutoff) {
					affected = append(affected, record.CaptureReceipt)
					at := record.RecordedAt
					if r.RetentionFirst == nil || at.Before(*r.RetentionFirst) {
						r.RetentionFirst = &at
					}
					if r.RetentionLast == nil || at.After(*r.RetentionLast) {
						r.RetentionLast = &at
					}
				}
			}
			if page.NextOffset < 0 {
				break
			}
			offset = page.NextOffset
		}
		sort.Slice(affected, func(i, j int) bool { return affected[i].ID < affected[j].ID })
		body, err := json.Marshal(affected)
		if err != nil {
			return MemoryReview{}, err
		}
		sum := sha256.Sum256(body)
		r.RetentionCount = len(affected)
		r.RetentionFingerprint = hex.EncodeToString(sum[:])
	default:
		return MemoryReview{}, errors.New("memory_invalid_operation")
	}
	after, err := e.VisibilityRevision(b.workspaceID)
	if err != nil {
		return MemoryReview{}, err
	}
	if after != rev {
		return MemoryReview{}, memory.ErrCaptureConflict
	}
	if err = b.validate(); err != nil {
		return MemoryReview{}, err
	}
	body, err := json.Marshal(r)
	if err != nil {
		return MemoryReview{}, err
	}
	sum := sha256.Sum256(body)
	r.ReviewDigest = hex.EncodeToString(sum[:])
	return r, nil
}
func (b *Bound) ApplyMemoryReview(in MemoryMutation, digest string, confirmed bool) (any, error) {
	if !confirmed {
		return nil, errors.New("confirmation_required")
	}
	r, err := b.MemoryReview(in)
	if err != nil {
		return nil, err
	}
	if digest == "" || digest != r.ReviewDigest {
		return nil, memory.ErrCaptureConflict
	}
	e, _, err := b.memoryEngine()
	if err != nil {
		return nil, err
	}
	switch in.Operation {
	case "memory.capture", "memory.correct":
		var sealed memory.OwnedCaptureInput
		if in.Operation == "memory.capture" {
			sealed, err = e.SealReviewedOwnedCapture(b.workspaceID, r.VisibilityRevision, in.Text, time.Now().UTC())
		} else {
			sealed, err = e.SealOwnedCapture(b.workspaceID, in.CaptureID, in.ExpectedRevision, in.Text, time.Now().UTC())
		}
		if err != nil {
			return nil, err
		}
		if err = b.validate(); err != nil {
			return nil, err
		}
		if in.Operation == "memory.capture" {
			return e.CaptureOwned(b.workspaceID, sealed)
		}
		return e.CorrectCapture(b.workspaceID, in.CaptureID, in.ExpectedRevision, sealed)
	case "memory.delete":
		return e.DeleteCapture(b.workspaceID, in.CaptureID, in.ExpectedRevision)
	case "memory.retention":
		cutoff, err := time.Parse(time.RFC3339Nano, in.Cutoff)
		if err != nil {
			return nil, err
		}
		return e.ApplyRetention(b.workspaceID, cutoff, in.ExpectedVisibilityRevision)
	default:
		return nil, errors.New("memory_invalid_operation")
	}
}
func memoryPresentation(r MemoryReview) map[string]any {
	labels := map[string]string{"memory.capture": "Guardar memória revisada", "memory.correct": "Corrigir memória", "memory.delete": "Remover memória", "memory.retention": "Remover memórias anteriores ao corte"}
	out := map[string]any{"Operação": labels[r.Mutation.Operation], "Espaço de trabalho": r.WorkspaceID, "Uso autorizado": "Memória privada deste espaço de trabalho; sem dados de clientes, credenciais ou segredos. A aprovação confirma esta seleção e este texto."}
	if r.Mutation.Text != "" {
		out["Texto proposto completo"] = r.Mutation.Text
	}
	if r.Current != nil {
		out["Texto atual completo"] = r.Current.Text
		out["Registrada em"] = r.Current.RecordedAt
		out["Revisada para contexto"] = r.Current.Eligible
	}
	if r.Mutation.Operation == "memory.retention" {
		out["Corte exclusivo"] = r.Mutation.Cutoff
		out["Quantidade a remover"] = r.RetentionCount
		out["Primeira memória selecionada"] = r.RetentionFirst
		out["Última memória selecionada"] = r.RetentionLast
		out["Alcance da autorização"] = "Todas as memórias registradas antes do corte neste espaço, no estado revisado. Os textos individuais podem ser inspecionados separadamente. A remoção revoga também os resumos derivados."
	}
	return out
}
func memoryResult(value any, err error) *mcp.CallToolResult {
	if err == nil {
		return result(value, nil)
	}
	var pending *confirmationPending
	if errors.As(err, &pending) {
		return pending.result
	}
	code := "memory_rejected"
	switch {
	case errors.Is(err, memory.ErrCaptureCapacity):
		code = "memory_capacity"
	case errors.Is(err, memory.ErrCaptureConflict), errors.Is(err, errOwnerReviewChanged):
		code = "revision_conflict"
	case errors.Is(err, memory.ErrCaptureUnavailable):
		code = "memory_unavailable"
	case errors.Is(err, memory.ErrConfinementUnavailable):
		code = "confinement_unavailable"
	case errors.Is(err, memory.ErrDreamInProgress):
		code = "memory_busy"
	case errors.Is(err, errMemoryIneligible):
		code = "memory_ineligible"
	case errors.Is(err, errConfirmationUnavailable):
		code = "confirmation_unavailable"
	case errors.Is(err, errConfirmationDeclined):
		code = "confirmation_declined"
	}
	metadata := map[string]any{"code": code, "retryable": false}
	if code == "confirmation_unavailable" {
		metadata["next_action"] = "Use local memory-review with JSON on stdin, then memory-apply with the same input, --review-digest and --confirm."
	}
	if code == "memory_ineligible" {
		metadata["next_action"] = "Use memory.list to identify legacy captures, then trusted memory.correct or memory.delete. Unreviewed text is available only in exact trusted review or the local operator review."
	}
	if code == "memory_capacity" {
		metadata["next_action"] = "The canonical lifetime identity budget is full. Existing captures remain readable, correctable and removable; removal does not reclaim identity capacity."
	}

	out := result(metadata, nil)
	out.IsError = true
	return out
}
func pageText(text string, offset, maximum int) (map[string]any, error) {
	if maximum == 0 {
		maximum = 2048
	}
	if maximum < 4 || maximum > 4096 || offset < 0 || offset > len(text) || (offset < len(text) && !utf8.RuneStart(text[offset])) {
		return nil, errors.New("invalid_page")
	}
	end := min(len(text), offset+maximum)
	for end < len(text) && !utf8.RuneStart(text[end]) {
		end--
	}
	next := -1
	if end < len(text) {
		next = end
	}
	return map[string]any{"text": text[offset:end], "offset": offset, "next_offset": next, "total_bytes": len(text)}, nil
}
func boundedMemoryText(text string, maximum int) string {
	if len(text) <= maximum {
		return text
	}
	end := maximum
	for end > 0 && !utf8.RuneStart(text[end]) {
		end--
	}
	return text[:end]
}
func (b *Bound) memoryContext(e *memory.Engine, cfg basememory.RuntimeConfig, offset, maximum int) (any, error) {
	page, err := e.ListCaptures(b.workspaceID, 0, 100)
	if err != nil {
		return nil, err
	}
	visibility := page.VisibilityRevision
	for start := 0; ; {
		if page.VisibilityRevision != visibility {
			return nil, memory.ErrCaptureConflict
		}
		for _, c := range page.Captures {
			if !c.Eligible {
				return nil, errMemoryIneligible
			}
		}
		if page.NextOffset < 0 {
			break
		}
		start = page.NextOffset
		page, err = e.ListCaptures(b.workspaceID, start, 100)
		if err != nil {
			return nil, err
		}
	}
	bundle, err := e.AssembleContext(b.workspaceID)
	if err != nil {
		return nil, err
	}
	text := ""
	layers := []string{}
	if page.Total > 0 {
		for _, section := range bundle.Sections {
			block := "# " + section.Layer + "\n" + boundedMemoryText(section.Content, cfg.ContextBudgets()[section.Layer]) + "\n"
			if len(text)+len(block) > cfg.SessionContextTotalMaxBytes {
				bundle.Diagnostics = append(bundle.Diagnostics, section.Layer+": envelope budget; skipped")
				continue
			}
			text += block
			layers = append(layers, section.Layer)
		}
	}
	value, err := pageText(text, offset, maximum)
	if err != nil {
		return nil, err
	}
	value["layers"] = layers
	value["diagnostics"] = bundle.Diagnostics
	after, err := e.VisibilityRevision(b.workspaceID)
	if err != nil {
		return nil, err
	}
	if after != visibility {
		return nil, memory.ErrCaptureConflict
	}
	value["visibility_revision"] = visibility
	if err = b.validate(); err != nil {
		return nil, err
	}
	return value, nil
}
func (b *Bound) handleMemory(ctx context.Context, req *mcp.CallToolRequest, in input) *mcp.CallToolResult {
	e, cfg, err := b.memoryEngine()
	if err != nil {
		return memoryResult(nil, err)
	}
	var value any
	switch in.Operation {
	case "memory.list", "memory.search":
		limit := in.Limit
		if limit == 0 {
			limit = 16
		}
		if in.Operation == "memory.list" {
			value, err = e.ListCaptures(b.workspaceID, in.Offset, limit)
		} else {
			value, err = e.SearchCaptures(b.workspaceID, in.Query, in.Offset, limit)
		}
	case "memory.read":
		var record memory.CaptureRecord
		record, err = e.ReadCapture(b.workspaceID, in.CaptureID)
		if err == nil && !record.Eligible {
			err = errMemoryIneligible
		}
		if err == nil && in.ExpectedRevision != 0 && uint64(in.ExpectedRevision) != record.Revision {
			err = memory.ErrCaptureConflict
		}
		if err == nil {
			var page map[string]any
			page, err = pageText(record.Text, in.Offset, in.MaxBytes)
			if err == nil {
				page["capture"] = record.CaptureReceipt
				page["recorded_at"] = record.RecordedAt
				value = page
			}
		}
	case "memory.context":
		value, err = b.memoryContext(e, cfg, in.Offset, in.MaxBytes)
	case "memory.capture", "memory.correct", "memory.delete", "memory.retention":
		mutation := MemoryMutation{Operation: in.Operation, Text: in.Text, CaptureID: in.CaptureID, ExpectedRevision: uint64(in.ExpectedRevision), Cutoff: in.Cutoff, ExpectedVisibilityRevision: in.ExpectedVisibilityRevision}
		var review MemoryReview
		review, err = b.MemoryReview(mutation)
		if err == nil {
			err = b.confirm(ctx, req, "Você autoriza esta alteração de memória privada exatamente como apresentada?", review, memoryPresentation(review))
			if err == nil {
				value, err = b.ApplyMemoryReview(mutation, review.ReviewDigest, true)
			}
		}
	case "memory.consolidate":
		var day time.Time
		day, err = time.Parse("2006-01-02", in.Date)
		status := "not_requested"
		var out memory.DreamResult
		if err == nil {
			if in.Cycle == "daily" {
				out, err = e.DreamDailyAttested(ctx, b.workspaceID, day)
			} else {
				var sourceDigest string
				var visibility uint64
				sourceDigest, visibility, err = b.memorySnapshot(e)
				if err == nil {
					daily := e.Synthesizer.(memory.DeterministicL1Synthesizer)
					e.Synthesizer = memory.DeterministicWeeklySynthesizer{Daily: daily, MaxRunes: e.Budgets, MaxInputBytes: cfg.L1MaxInputBytes}
					e.SynthesizerID = memory.DeterministicWeeklySynthesizerID
					e.EligibilityPolicyID = "deterministic-two-l3-generations+exact-owner-review-v1"
					status = "unchanged"
					e.Eligibility = memory.EligibilityFunc(func(ctx context.Context, candidate memory.Artifact) (bool, string, error) {
						eligible, reason, err := (memory.DeterministicLifetimeEligibility{MinL3Generations: 2}).Evaluate(ctx, candidate)
						if err != nil {
							return false, "", err
						}
						if !eligible {
							status = "prerequisites_unmet"
							return false, reason, nil
						}
						// Generated artifacts get fresh timestamps each MRTR round. Authority
						// binds their exact candidate text and stable original source snapshot.
						review := map[string]any{"scope": b.home, "workspace_id": b.workspaceID, "operation": "memory.promote_lifetime", "period": candidate.Period, "layer": candidate.Layer, "content": candidate.Content, "source_snapshot": sourceDigest, "visibility_revision": visibility, "eligibility_policy": e.EligibilityPolicyID, "eligibility_reason": reason}
						view := map[string]any{"Operação": "Promover memória vitalícia", "Espaço de trabalho": b.workspaceID, "Período": candidate.Period, "Texto candidato completo": candidate.Content, "Regra de continuidade": "Duas ou mais gerações semanais de L3, verificadas pela política canônica.", "Alcance": "Aprovação exclusiva deste candidato e destas fontes. A promoção mantém histórico e revoga os derivados se uma fonte for removida."}
						err = b.confirm(ctx, req, "Você revisou e autoriza promover este candidato exato à memória vitalícia?", review, view)
						if errors.Is(err, errConfirmationUnavailable) {
							status = "confirmation_unavailable"
							return false, "trusted exact-candidate confirmation unavailable; L2/L3 retained without lifetime promotion", nil
						}
						if errors.Is(err, errConfirmationDeclined) {
							status = "confirmation_declined"
							return false, "owner declined exact lifetime candidate; L2/L3 retained", nil
						}
						if err != nil {
							return false, "", err
						}
						after, revision, err := b.memorySnapshot(e)
						if err != nil {
							return false, "", err
						}
						if after != sourceDigest || revision != visibility {
							return false, "", memory.ErrCaptureConflict
						}
						status = "approved"
						return true, reason + "; exact candidate approved by the owner", nil
					})
					out, err = e.DreamWeekly(ctx, b.workspaceID, day)
				}
			}
		}
		if err == nil {
			value = map[string]any{"result": out, "synthesis": "local_deterministic", "synthesizer": e.SynthesizerID, "lifetime_status": status}
		}

	default:
		err = errors.New("invalid_operation")
	}
	if err == nil {
		err = b.validate()
	}
	return memoryResult(value, err)
}

// memorySnapshot binds stable eligible source identities and the already active
// context versions; generated candidate timestamps are deliberately excluded.
func (b *Bound) memorySnapshot(e *memory.Engine) (string, uint64, error) {
	var captures []memory.CaptureReceipt
	var visibility uint64
	for offset := 0; ; {
		page, err := e.ListCaptures(b.workspaceID, offset, 100)
		if err != nil {
			return "", 0, err
		}
		if offset == 0 {
			visibility = page.VisibilityRevision
		}
		if page.VisibilityRevision != visibility {
			return "", 0, memory.ErrCaptureConflict
		}
		for _, receipt := range page.Captures {
			if !receipt.Eligible {
				return "", 0, errMemoryIneligible
			}
			captures = append(captures, receipt)
		}
		if page.NextOffset < 0 {
			break
		}
		offset = page.NextOffset
	}
	if len(captures) == 0 {
		return "", 0, errors.New("memory_no_eligible_sources")
	}
	bundle, err := e.AssembleContext(b.workspaceID)
	if err != nil {
		return "", 0, err
	}
	after, err := e.VisibilityRevision(b.workspaceID)
	if err != nil {
		return "", 0, err
	}
	if after != visibility {
		return "", 0, memory.ErrCaptureConflict
	}
	body, err := json.Marshal(struct {
		Captures []memory.CaptureReceipt
		Sections []memory.ContextSection
	}{captures, bundle.Sections})
	if err != nil {
		return "", 0, err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), visibility, nil
}
