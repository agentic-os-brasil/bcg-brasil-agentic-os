package ownerctx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatelock"
)

const expansionSchemaVersion = 1
const expansionStaleAfter = 180 * 24 * time.Hour
const maximumExpansionFacetBytes = 12 << 10
const maximumExpansionFacetLines = 120

type FacetConfirmation struct {
	SHA256      string    `json:"sha256"`
	ConfirmedAt time.Time `json:"confirmed_at"`
}

type expansionRegistry struct {
	SchemaVersion int                          `json:"schema_version"`
	Confirmations map[string]FacetConfirmation `json:"confirmations"`
}

type ExpansionQuestion struct {
	Kind            string `json:"kind"`
	Facet           string `json:"facet"`
	QuestionID      string `json:"question_id"`
	Version         int    `json:"version"`
	Question        string `json:"question"`
	AudioPrompt     string `json:"audio_prompt"`
	QuestionToken   string `json:"question_token"`
	Instructions    string `json:"instructions"`
	MaximumBytes    int    `json:"maximum_bytes"`
	RequiredSection string `json:"required_section"`
}

type ExpansionDraft struct {
	SchemaVersion   int       `json:"schema_version"`
	ID              string    `json:"id"`
	Kind            string    `json:"kind"`
	Facet           string    `json:"facet"`
	QuestionID      string    `json:"question_id"`
	QuestionVersion int       `json:"question_version"`
	QuestionToken   string    `json:"question_token"`
	BaseSHA256      string    `json:"base_sha256"`
	ProposedBody    string    `json:"proposed_body"`
	Consent         bool      `json:"consent"`
	NoClientData    bool      `json:"no_client_data"`
	ReviewDigest    string    `json:"review_digest"`
	State           string    `json:"state"`
	CreatedAt       time.Time `json:"created_at"`
	AppliedAt       time.Time `json:"applied_at,omitempty"`
	RefinementID    string    `json:"refinement_id,omitempty"`
}

var expansionWriteJSON = writePrivateJSON
var expansionCompactDrafts = compactExpansionDrafts

var expansionQuestions = map[string]InterviewStep{
	"professional-role":   {Facet: "professional-role", Question: "Hoje, qual é o seu papel profissional, pelo que você é responsável e que resultado prova que você está indo bem?", AudioPrompt: "Conte seu papel, suas responsabilidades e como você mede sucesso."},
	"communication-style": {Facet: "communication-style", Question: "Como você quer que o Maestro trabalhe e converse com você — idioma, tom, nível de detalhe, formato e quando desafiar?", AudioPrompt: "Como você prefere conversar, receber respostas e ser desafiado?"},
	"voice":               {Facet: "voice", Question: "Quando algo sai em seu nome, como deve soar — e o que nunca deve parecer?", AudioPrompt: "Como sua voz deve soar, e o que ela nunca deve parecer?"},
	"preferences":         {Facet: "preferences", Question: "Quais ferramentas, formatos, rituais e formas de colaboração aumentam ou reduzem sua qualidade e velocidade?", AudioPrompt: "O que ajuda ou atrapalha sua qualidade e velocidade de trabalho?"},
	"decision-rules":      {Facet: "decision-rules", Question: "Quando há um trade-off real, quais princípios pesam mais e quais sinais fazem você mudar de direção?", AudioPrompt: "Quais princípios guiam seus trade-offs e o que faz você mudar de ideia?"},
	"working-boundaries":  {Facet: "working-boundaries", Question: "Quais limites de confidencialidade, escopo, autonomia e escalada o Maestro nunca deve cruzar?", AudioPrompt: "Quais limites o Maestro nunca deve cruzar?"},
}

func expansionStatus(root string, value registry, onboarding OnboardingStatus) (ExpansionStatus, error) {
	result := ExpansionStatus{State: "action_required", Total: len(onboardingFacets)}
	confirmations, err := loadExpansionRegistry(root)
	if err != nil {
		return ExpansionStatus{}, err
	}
	now := time.Now().UTC()
	for _, facet := range onboardingFacets {
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(facets[facet].Record.Path)))
		if err != nil || !facetAnswered(root, facet) {
			result.Unknown++
			if result.NextFacet == "" {
				result.NextFacet = facet
			}
			continue
		}
		confirmation, ok := confirmations.Confirmations[facet]
		if !ok && onboarding.State == "complete" && containsFacet(onboardingTracks[onboarding.Track].Facets, facet) {
			confirmedAt, err := time.Parse(time.RFC3339Nano, value.OnboardingConfirmedAt)
			if err == nil {
				confirmation = FacetConfirmation{SHA256: digest(string(body)), ConfirmedAt: confirmedAt}
				ok = true
			}
		}
		if !ok || confirmation.SHA256 != digest(string(body)) || now.Sub(confirmation.ConfirmedAt) > expansionStaleAfter {
			result.Stale++
			if result.NextFacet == "" {
				result.NextFacet = facet
			}
			continue
		}
		result.Current++
	}
	if drafts, _ := filepath.Glob(filepath.Join(root, "owner", "interview", "drafts", "*.json")); len(drafts) > 0 {
		for _, draftPath := range drafts {
			d, err := readExpansionDraftPath(draftPath)
			if err != nil {
				return ExpansionStatus{}, err
			}
			if d.State == "drafted" || d.State == "prepared" {
				result.ReviewCount++
			}
		}
	}
	if result.ReviewCount > 0 {
		result.State = "review_required"
		result.NextFacet = ""
	} else if result.Unknown == 0 && result.Stale == 0 {
		result.State = "current"
		result.NextFacet = ""
	}
	return result, nil
}

func containsFacet(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func NextExpansionQuestion(root string) (ExpansionQuestion, error) {
	status, err := Inspect(root)
	if err != nil {
		return ExpansionQuestion{}, err
	}
	if !status.Initialized {
		return ExpansionQuestion{}, errors.New("owner context is not initialized")
	}
	if status.Onboarding.State != "complete" {
		return ExpansionQuestion{}, errors.New("complete or confirm owner onboarding before starting ongoing SELF expansion")
	}
	if status.Expansion.State == "review_required" {
		return ExpansionQuestion{}, errors.New("review or confirm the open SELF expansion draft before asking another question")
	}
	if status.Expansion.NextFacet == "" {
		return ExpansionQuestion{}, errors.New("all SELF facets are current")
	}
	step := expansionQuestions[status.Expansion.NextFacet]
	base := facetSHA(root, step.Facet)
	q := ExpansionQuestion{Kind: "owner_self_expansion", Facet: step.Facet, QuestionID: "self-" + step.Facet, Version: 1, Question: step.Question, AudioPrompt: step.AudioPrompt, Instructions: "Peça consentimento, faça somente esta pergunta e mostre o draft completo antes de pedir confirmação. Não infira respostas, não inclua client data e compacte a verdade atual sem acumular transcrição.", MaximumBytes: maximumExpansionFacetBytes, RequiredSection: "## Current"}
	q.QuestionToken = digest(fmt.Sprintf("%d\x00%s\x00%s\x00%d\x00%s", expansionSchemaVersion, q.Kind, q.QuestionID, q.Version, base))
	return q, nil
}

func DraftExpansion(root, questionToken, proposedBody string, consent, noClientData bool) (ExpansionDraft, error) {
	unlock, err := privatelock.Acquire(filepath.Join(root, "owner", "interview", ".transition.lock"))
	if err != nil {
		return ExpansionDraft{}, err
	}
	result, operationErr := draftExpansionLocked(root, questionToken, proposedBody, consent, noClientData)
	unlockErr := unlock()
	if operationErr != nil {
		return result, errors.Join(operationErr, unlockErr)
	}
	return result, unlockErr
}

func draftExpansionLocked(root, questionToken, proposedBody string, consent, noClientData bool) (ExpansionDraft, error) {
	if !consent || !noClientData {
		return ExpansionDraft{}, errors.New("explicit interview consent and owner no-client-data attestation are required")
	}
	if strings.TrimSpace(proposedBody) == "" {
		return ExpansionDraft{}, errors.New("proposed SELF facet body is required")
	}
	if err := validateExpansionBody(proposedBody); err != nil {
		return ExpansionDraft{}, err
	}
	q, err := NextExpansionQuestion(root)
	if err != nil {
		return ExpansionDraft{}, err
	}
	if !secureDigestEqual(questionToken, q.QuestionToken) {
		return ExpansionDraft{}, errors.New("question token is stale or off-sequence; nothing was drafted")
	}
	d := ExpansionDraft{SchemaVersion: expansionSchemaVersion, Kind: q.Kind, Facet: q.Facet, QuestionID: q.QuestionID, QuestionVersion: q.Version, QuestionToken: q.QuestionToken, BaseSHA256: facetSHA(root, q.Facet), ProposedBody: proposedBody, Consent: true, NoClientData: true, State: "drafted", CreatedAt: time.Now().UTC()}
	if digest(proposedBody) == d.BaseSHA256 {
		return ExpansionDraft{}, errors.New("proposed SELF facet duplicates the current canonical revision")
	}
	d.ReviewDigest = expansionDraftDigest(d)
	d.ID = "self-draft-" + d.ReviewDigest[:24]
	if err := expansionWriteJSON(filepath.Join(root, "owner", "interview", "drafts", d.ID+".json"), d); err != nil {
		return ExpansionDraft{}, err
	}
	return d, nil
}

func ReviewExpansion(root, id string) (ExpansionDraft, error) {
	return readExpansionDraftPath(filepath.Join(root, "owner", "interview", "drafts", filepath.Base(id)+".json"))
}

func ConfirmExpansion(root, id, reviewDigest string, confirmed bool) (ExpansionDraft, error) {
	unlock, err := privatelock.Acquire(filepath.Join(root, "owner", "interview", ".transition.lock"))
	if err != nil {
		return ExpansionDraft{}, err
	}
	result, operationErr := confirmExpansionLocked(root, id, reviewDigest, confirmed)
	unlockErr := unlock()
	if operationErr != nil {
		return result, errors.Join(operationErr, unlockErr)
	}
	return result, unlockErr
}

func confirmExpansionLocked(root, id, reviewDigest string, confirmed bool) (ExpansionDraft, error) {
	if !confirmed {
		return ExpansionDraft{}, ErrConfirmationRequired
	}
	d, err := ReviewExpansion(root, id)
	if err != nil {
		return ExpansionDraft{}, err
	}
	if (d.State != "drafted" && d.State != "prepared") || !validExpansionDraft(d) || !secureDigestEqual(reviewDigest, d.ReviewDigest) {
		return ExpansionDraft{}, errors.New("SELF expansion confirmation denied because the reviewed envelope is invalid or closed")
	}
	var receipt RefinementReceipt
	if d.State == "drafted" {
		currentToken := digest(fmt.Sprintf("%d\x00%s\x00%s\x00%d\x00%s", expansionSchemaVersion, d.Kind, d.QuestionID, d.QuestionVersion, facetSHA(root, d.Facet)))
		if !secureDigestEqual(currentToken, d.QuestionToken) || facetSHA(root, d.Facet) != d.BaseSHA256 {
			return ExpansionDraft{}, ErrRevisionConflict
		}
		receipt, err = SubmitRefinement(root, RefinementInput{Facet: d.Facet, Evidence: "explicit owner SELF expansion interview " + d.QuestionID, ProposedBody: d.ProposedBody, ProducerID: "owner-interview"})
		if err != nil {
			return ExpansionDraft{}, err
		}
		d.State, d.RefinementID = "prepared", receipt.ID
		if err := expansionWriteJSON(filepath.Join(root, "owner", "interview", "drafts", d.ID+".json"), d); err != nil {
			return ExpansionDraft{}, err
		}
	} else {
		receipt = RefinementReceipt{ID: d.RefinementID}
	}
	receipt, err = ApplyRefinement(root, receipt.ID, true)
	if err != nil {
		return ExpansionDraft{}, err
	}
	registry, err := loadExpansionRegistry(root)
	if err != nil {
		return ExpansionDraft{}, err
	}
	if registry.Confirmations == nil {
		registry = expansionRegistry{SchemaVersion: expansionSchemaVersion, Confirmations: map[string]FacetConfirmation{}}
	}
	now := time.Now().UTC()
	registry.Confirmations[d.Facet] = FacetConfirmation{SHA256: digest(d.ProposedBody), ConfirmedAt: now}
	if err := expansionWriteJSON(filepath.Join(root, "owner", "interview", "confirmations.json"), registry); err != nil {
		return ExpansionDraft{}, err
	}
	if err := expansionCompactDrafts(root, d); err != nil {
		return ExpansionDraft{}, err
	}
	d.State, d.AppliedAt, d.RefinementID = "applied", now, receipt.ID
	if err := expansionWriteJSON(filepath.Join(root, "owner", "interview", "drafts", d.ID+".json"), d); err != nil {
		return ExpansionDraft{}, err
	}
	return d, nil
}

func expansionDraftDigest(d ExpansionDraft) string {
	envelope := struct {
		SchemaVersion                           int `json:"schema_version"`
		Kind, Facet, QuestionID                 string
		QuestionVersion                         int `json:"question_version"`
		QuestionToken, BaseSHA256, ProposedBody string
		Consent, NoClientData                   bool
	}{d.SchemaVersion, d.Kind, d.Facet, d.QuestionID, d.QuestionVersion, d.QuestionToken, d.BaseSHA256, d.ProposedBody, d.Consent, d.NoClientData}
	body, _ := json.Marshal(envelope)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func validExpansionDraft(d ExpansionDraft) bool {
	validState := (d.State == "drafted" && d.RefinementID == "" && d.AppliedAt.IsZero()) || (d.State == "prepared" && d.RefinementID != "" && d.AppliedAt.IsZero()) || (d.State == "applied" && !d.AppliedAt.IsZero() && d.RefinementID != "")
	validID := d.ReviewDigest != "" && d.ID == "self-draft-"+d.ReviewDigest[:min(24, len(d.ReviewDigest))]
	return d.SchemaVersion == expansionSchemaVersion && d.Kind == "owner_self_expansion" && containsFacet(onboardingFacets, d.Facet) && d.QuestionID == "self-"+d.Facet && d.QuestionVersion == 1 && d.Consent && d.NoClientData && !d.CreatedAt.IsZero() && validState && validID && d.BaseSHA256 != "" && d.ProposedBody != "" && validateExpansionBody(d.ProposedBody) == nil && secureDigestEqual(d.ReviewDigest, expansionDraftDigest(d))
}
func facetSHA(root, facet string) string {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(facets[facet].Record.Path)))
	if err != nil {
		return ""
	}
	return digest(string(body))
}
func loadExpansionRegistry(root string) (expansionRegistry, error) {
	body, err := os.ReadFile(filepath.Join(root, "owner", "interview", "confirmations.json"))
	if errors.Is(err, os.ErrNotExist) {
		return expansionRegistry{SchemaVersion: expansionSchemaVersion, Confirmations: map[string]FacetConfirmation{}}, nil
	}
	if err != nil {
		return expansionRegistry{}, err
	}
	var value expansionRegistry
	if json.Unmarshal(body, &value) != nil || value.SchemaVersion != expansionSchemaVersion {
		return expansionRegistry{}, errors.New("owner SELF expansion confirmations are invalid")
	}
	if value.Confirmations == nil {
		value.Confirmations = map[string]FacetConfirmation{}
	}
	for facet, confirmation := range value.Confirmations {
		if !containsFacet(onboardingFacets, facet) || !validOnboardingDigest(confirmation.SHA256) || confirmation.ConfirmedAt.IsZero() {
			return expansionRegistry{}, errors.New("owner SELF expansion confirmations are invalid")
		}
	}
	return value, nil
}
func readExpansionDraftPath(path string) (ExpansionDraft, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return ExpansionDraft{}, err
	}
	var d ExpansionDraft
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&d); err != nil || !validExpansionDraft(d) {
		return ExpansionDraft{}, errors.New("owner SELF expansion draft is invalid")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return ExpansionDraft{}, errors.New("owner SELF expansion draft contains trailing JSON")
	}
	if filepath.Base(path) != d.ID+".json" {
		return ExpansionDraft{}, errors.New("owner SELF expansion draft path does not match its digest-bound ID")
	}
	return d, nil
}

func validateExpansionBody(body string) error {
	if len([]byte(body)) > maximumExpansionFacetBytes {
		return errors.New("proposed SELF facet exceeds the 12 KiB compaction bound")
	}
	canonical := strings.ReplaceAll(body, "\r\n", "\n")
	lines := strings.Split(canonical, "\n")
	if len(lines) > maximumExpansionFacetLines {
		return errors.New("proposed SELF facet exceeds the 120-line compaction bound")
	}
	currentSections := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Current" {
			currentSections++
		}
		lower := strings.ToLower(trimmed)
		for _, prefix := range []string{"user:", "assistant:", "usuario:", "usuário:", "maestro:", "walter:"} {
			if strings.HasPrefix(lower, prefix) {
				return errors.New("proposed SELF facet looks transcript-like; compact it into current owner truth")
			}
		}
	}
	if currentSections != 1 {
		return errors.New("proposed SELF facet must contain exactly one ## Current section")
	}
	seen := map[string]bool{}
	for _, paragraph := range strings.Split(canonical, "\n\n") {
		normalized := strings.ToLower(strings.Join(strings.Fields(paragraph), " "))
		if normalized == "" || strings.HasPrefix(normalized, "#") {
			continue
		}
		if seen[normalized] {
			return errors.New("proposed SELF facet repeats content; compact duplicate prose before review")
		}
		seen[normalized] = true
	}
	return nil
}

func compactExpansionDrafts(root string, current ExpansionDraft) error {
	paths, err := filepath.Glob(filepath.Join(root, "owner", "interview", "drafts", "*.json"))
	if err != nil {
		return err
	}
	for _, path := range paths {
		if filepath.Base(path) == current.ID+".json" {
			continue
		}
		draft, err := readExpansionDraftPath(path)
		if err != nil {
			return err
		}
		if draft.State == "applied" && draft.Facet == current.Facet {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	return nil
}
