// Package ownerctx owns human-readable, user-local owner context pointers.
package ownerctx

import (
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Pointer struct {
	Path      string `json:"path"`
	Available bool   `json:"available"`
	State     string `json:"state"`
}

// Facet is an inspectable pointer and its declared handling policy. It never
// includes the facet body, including when the facet is sensitive.
type Facet struct {
	Pointer
	Sensitivity string   `json:"sensitivity"`
	Readers     []string `json:"readers"`
	Refinement  string   `json:"refinement"`
}

type Capability struct {
	State   string `json:"state"`
	Message string `json:"message"`
}

type Status struct {
	Initialized    bool                  `json:"initialized"`
	Facets         map[string]Facet      `json:"facets"`
	OperatingState Pointer               `json:"operating_state"`
	Tasks          Pointer               `json:"tasks"`
	Onboarding     OnboardingStatus      `json:"onboarding"`
	SelfIndex      Pointer               `json:"self_index"`
	Expansion      ExpansionStatus       `json:"expansion"`
	OpenTasks      TaskStatus            `json:"open_tasks"`
	Capabilities   map[string]Capability `json:"capabilities"`
}

// ExpansionStatus is a bounded projection of the six non-sensitive SELF
// facets. It contains no answer, draft body or interview token.
type ExpansionStatus struct {
	State       string `json:"state"`
	Total       int    `json:"total"`
	Current     int    `json:"current"`
	Unknown     int    `json:"unknown"`
	Stale       int    `json:"stale"`
	NextFacet   string `json:"next_facet,omitempty"`
	ReviewCount int    `json:"review_count"`
}

// OnboardingStatus is a deterministic projection of the consented owner
// facets. It never includes a facet body; runtimes use its next question to
// resume the interview instead of assuming that setup was completed.
type OnboardingStatus struct {
	State            string        `json:"state"`
	Track            string        `json:"track"`
	EstimatedMinutes int           `json:"estimated_minutes,omitempty"`
	Remaining        []string      `json:"remaining,omitempty"`
	NextQuestion     InterviewStep `json:"next_question,omitempty"`
	ReviewDigest     string        `json:"review_digest,omitempty"`
}

// TaskStatus exposes only a bounded list of explicitly marked open items in
// the owner-local work state. It is not inferred from prompts or files.
type TaskStatus struct {
	State string `json:"state"`
	Count int    `json:"count"`
}

type InterviewStep struct {
	Facet       string `json:"facet"`
	Question    string `json:"question"`
	AudioPrompt string `json:"audio_prompt,omitempty"`
}

// Interview is a runtime-neutral prompt contract. Runtimes may present it
// conversationally, but must not infer or persist answers without user review.
type Interview struct {
	Kind             string          `json:"kind"`
	Track            string          `json:"track"`
	EstimatedMinutes int             `json:"estimated_minutes"`
	Instructions     string          `json:"instructions"`
	Steps            []InterviewStep `json:"steps"`
}

type facetRecord struct {
	Path        string   `json:"path"`
	Sensitivity string   `json:"sensitivity"`
	Readers     []string `json:"readers"`
	Refinement  string   `json:"refinement"`
}

type registry struct {
	SchemaVersion             int                       `json:"schema_version"`
	Facets                    map[string]facetRecord    `json:"facets"`
	Producers                 map[string]producerRecord `json:"producers"`
	OperatingState            string                    `json:"operating_state"`
	OnboardingConfirmedAt     string                    `json:"onboarding_confirmed_at,omitempty"`
	OnboardingConfirmedSHA256 string                    `json:"onboarding_confirmed_sha256,omitempty"`
	OnboardingTrack           string                    `json:"onboarding_track,omitempty"`
}

type producerRecord struct {
	CapabilitySHA256 string `json:"capability_sha256"`
	AuthorizedAt     string `json:"authorized_at"`
}

type facetTemplate struct {
	Record facetRecord
	Body   string
}

var facets = map[string]facetTemplate{
	"professional-role":     {facetRecord{"owner/self/professional-role.md", "professional", []string{"session", "walter"}, "proposal_only"}, "# Professional role\n\n## Current\n\nDescreva responsabilidades, contexto profissional e resultados pelos quais voce responde.\n"},
	"communication-style":   {facetRecord{"owner/self/communication-style.md", "professional", []string{"session", "walter"}, "automatic_with_audit"}, "# Communication style\n\n## Current\n\nDescreva como prefere colaborar com o Agentic OS: tom, nivel de detalhe, idioma e formato.\n"},
	"voice":                 {facetRecord{"owner/self/voice.md", "professional", []string{"session", "walter"}, "automatic_with_audit"}, "# Voice\n\n## Current\n\nDescreva como voce quer falar com o mundo em entregas externas: emails, documentos, propostas e apresentacoes.\n"},
	"preferences":           {facetRecord{"owner/self/preferences.md", "professional", []string{"session", "walter"}, "automatic_with_audit"}, "# Preferences\n\n## Current\n\nRegistre preferencias de ferramentas, formatos, rotinas e formas de trabalho.\n"},
	"decision-rules":        {facetRecord{"owner/self/decision-rules.md", "professional", []string{"session", "walter"}, "proposal_only"}, "# Decision rules\n\n## Current\n\nRegistre principios e trade-offs que devem orientar recomendacoes importantes.\n"},
	"working-boundaries":    {facetRecord{"owner/self/working-boundaries.md", "professional", []string{"session", "walter"}, "confirmation_required"}, "# Working boundaries\n\n## Current\n\nRegistre limites de escopo, confidencialidade e quando escalar decisoes.\n"},
	"psychological-profile": {facetRecord{"owner/self/psychological-profile.md", "sensitive", []string{"walter"}, "confirmation_required"}, "# Psychological profile\n\nOpcional. Inclua apenas uma sintese profissional revisada por voce, com fontes e finalidades autorizadas. Nunca use como diagnostico ou rotulo deterministico.\n"},
}

const statePath = "owner/operating/work-state.md"
const selfIndexPath = "owner/self/README.md"
const stateTemplate = "# Work state\n\nRegistre somente estado operacional recente: prioridades, bloqueios, proximas acoes e itens aguardando resposta.\n\n## Tarefas abertas\n- [ ]\n"
const selfIndexTemplate = `# Owner SELF

Este diretorio e a fonte canonica local do SELF profissional do owner. Cada
faceta abaixo so muda por resposta explicita, draft revisavel e confirmacao do
owner. Conversas, observacoes, client data e inferencias nunca sobrescrevem
estas paginas.

## Facetas

- [Professional role](professional-role.md) — papel, responsabilidades e
  resultados que definem sucesso.
- [Communication style](communication-style.md) — idioma, tom, detalhe,
  formato e como o Maestro deve desafiar.
- [Voice](voice.md) — como entregas em nome do owner devem e nao devem soar.
- [Preferences](preferences.md) — ferramentas, formatos, rituais e formas de
  colaboracao que ajudam ou atrapalham.
- [Decision rules](decision-rules.md) — principios para trade-offs e sinais
  que justificam mudar de direcao.
- [Working boundaries](working-boundaries.md) — limites de confidencialidade,
  escopo, autonomia e escalada.

## Contrato de verdade

Cada pagina deve registrar apenas afirmacoes que o owner reconhece como suas,
com linguagem suficientemente concreta para orientar trabalho e suficientemente
estavel para sobreviver a uma sessao. Instrucao explicita atual prevalece;
correcoes devem atravessar novo draft e confirmacao. Ausencia e staleness sao
lacunas visiveis, nunca permissao para completar o perfil por inferencia.
Cada faceta mantem somente uma secao ` + "`## Current`" + ` concisa. Historico nao e
acumulado como prosa: revisoes anteriores ficam em
` + "`owner/refinement/versions/<facet>/`" + ` e sao referenciadas pelos receipts de
auditoria. Facetas repetitivas, transcript-like ou acima dos limites fechados
falham antes de virar draft.

` + "`psychological-profile.md`" + ` e opcional, sensivel e exclusivo de Walter; ele
nao integra entrevistas de expansao, o indice de sessao ou recomendacoes
deterministicas. A confirmacao ` + "`no_client_data`" + ` e uma declaracao do owner,
nao um classificador automatico de conteudo.
`

const (
	OnboardingTrackQuick    = "quick"
	OnboardingTrackComplete = "complete"
)

var onboardingFacets = []string{"professional-role", "communication-style", "voice", "preferences", "decision-rules", "working-boundaries"}

var quickOnboardingFacets = []string{"professional-role", "communication-style", "working-boundaries"}

type onboardingTrackDefinition struct {
	ID               string
	EstimatedMinutes int
	Facets           []string
}

var onboardingTracks = map[string]onboardingTrackDefinition{
	OnboardingTrackQuick:    {ID: OnboardingTrackQuick, EstimatedMinutes: 7, Facets: quickOnboardingFacets},
	OnboardingTrackComplete: {ID: OnboardingTrackComplete, EstimatedMinutes: 25, Facets: onboardingFacets},
}

func Initialize(root string) (Status, error) {
	directory := filepath.Join(root, "owner")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return Status{}, err
	}
	for _, template := range facets {
		if err := create(filepath.Join(root, filepath.FromSlash(template.Record.Path)), template.Body); err != nil {
			return Status{}, err
		}
	}
	if err := create(filepath.Join(root, filepath.FromSlash(statePath)), stateTemplate); err != nil {
		return Status{}, err
	}
	if err := create(filepath.Join(root, filepath.FromSlash(selfIndexPath)), selfIndexTemplate); err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(filepath.Join(directory, "sources", "assessments"), 0o700); err != nil {
		return Status{}, err
	}
	if _, _, err := ensurePromptHistoryStore(root); err != nil {
		return Status{}, err
	}
	path := filepath.Join(directory, "registry.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		definitions := make(map[string]facetRecord, len(facets))
		for id, template := range facets {
			definitions[id] = template.Record
		}
		body, err := json.MarshalIndent(registry{SchemaVersion: 3, Facets: definitions, Producers: map[string]producerRecord{}, OperatingState: statePath}, "", "  ")
		if err != nil {
			return Status{}, err
		}
		if err := os.WriteFile(path, append(body, '\n'), 0o600); err != nil {
			return Status{}, err
		}
	} else if err != nil {
		return Status{}, err
	}
	return Inspect(root)
}

func Inspect(root string) (Status, error) {
	path := filepath.Join(root, "owner", "registry.json")
	file, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return emptyStatus(), nil
	}
	if err != nil {
		return Status{}, err
	}
	var value registry
	if err := json.Unmarshal(file, &value); err != nil || (value.SchemaVersion != 2 && value.SchemaVersion != 3) {
		return Status{}, errors.New("owner context registry is invalid")
	}
	if value.SchemaVersion == 2 && value.OnboardingTrack == "" {
		// Existing profiles predate explicit track selection. Preserve their
		// already-reviewed full onboarding instead of forcing a new interview.
		value.OnboardingTrack = OnboardingTrackComplete
	}
	status := Status{
		Initialized:    true,
		Facets:         make(map[string]Facet, len(value.Facets)),
		OperatingState: pointer(root, value.OperatingState),
		SelfIndex:      pointer(root, selfIndexPath),
		Tasks:          Pointer{State: "unavailable"},
		Capabilities:   capabilities(),
	}
	for id, record := range value.Facets {
		status.Facets[id] = Facet{Pointer: pointer(root, record.Path), Sensitivity: record.Sensitivity, Readers: record.Readers, Refinement: record.Refinement}
	}
	track := value.OnboardingTrack
	if value.SchemaVersion == 2 && track == "" {
		// Schema-v2 had only the complete interview. Preserve that contract
		// while accepting the new explicit-track registry shape.
		track = OnboardingTrackComplete
	}
	status.Onboarding = onboarding(root, status.Facets, track, value.OnboardingConfirmedAt, value.OnboardingConfirmedSHA256)
	if value.SchemaVersion == 2 && value.OnboardingConfirmedAt != "" && status.Onboarding.State == "review_required" && secureDigestEqual(value.OnboardingConfirmedSHA256, legacyOnboardingDigest(root)) {
		status.Onboarding.State = "complete"
		status.Onboarding.ReviewDigest = ""
	}
	status.Expansion, err = expansionStatus(root, value, status.Onboarding)
	if err != nil {
		return Status{}, err
	}
	status.OpenTasks = openTasks(root, value.OperatingState)
	if status.OpenTasks.State != "unavailable" {
		status.Tasks = Pointer{Path: value.OperatingState, Available: true, State: status.OpenTasks.State}
	}
	return status, nil
}

func ColdStartInterview() Interview {
	return interviewForTrack(OnboardingTrackComplete)
}

// QuickStartInterview captures the minimum operational self needed to begin
// safely. The owner may upgrade to the complete track later; it never infers
// the omitted identity and preference facets.
func QuickStartInterview() Interview {
	return interviewForTrack(OnboardingTrackQuick)
}

func interviewForTrack(track string) Interview {
	definition, ok := onboardingTracks[track]
	if !ok {
		return Interview{}
	}
	allSteps := map[string]InterviewStep{
		"professional-role":   {Facet: "professional-role", Question: "Qual e seu papel, principais responsabilidades e quais resultados voce precisa gerar?"},
		"communication-style": {Facet: "communication-style", Question: "Como voce prefere que o Agentic OS explique, estruture e comunique o trabalho com voce?"},
		"voice":               {Facet: "voice", Question: "Como sua voz deve aparecer em entregas para clientes, lideres e colegas?"},
		"preferences":         {Facet: "preferences", Question: "Quais ferramentas, formatos e rotinas tornam seu trabalho melhor?"},
		"decision-rules":      {Facet: "decision-rules", Question: "Quais principios e trade-offs devem orientar recomendacoes importantes?"},
		"working-boundaries":  {Facet: "working-boundaries", Question: "Quais limites de confidencialidade, escopo e escalonamento o sistema deve respeitar?"},
	}
	steps := make([]InterviewStep, 0, len(definition.Facets))
	for _, facet := range definition.Facets {
		steps = append(steps, allSteps[facet])
	}
	return Interview{
		Kind:             "cold_start",
		Track:            definition.ID,
		EstimatedMinutes: definition.EstimatedMinutes,
		Instructions:     "Conduza uma conversa com uma pergunta por vez. Mostre cada resposta ao dono antes de sugerir que ela seja gravada na faceta correspondente. Nao infira perfil psicologico.",
		Steps:            steps,
	}
	/*
		The full interview remains the canonical identity route. The quick track
		is deliberately narrower, not a hidden shortcut through the same data.
	*/
}

func emptyStatus() Status {
	return Status{Tasks: Pointer{State: "unavailable"}, SelfIndex: Pointer{State: "missing"}, Onboarding: onboardingSelectionRequired(), Expansion: ExpansionStatus{State: "unavailable"}, OpenTasks: TaskStatus{State: "unavailable"}, Capabilities: capabilities()}
}

func onboardingSelectionRequired() OnboardingStatus {
	return OnboardingStatus{State: "required", Track: "selection_required", Remaining: append([]string(nil), onboardingFacets...), NextQuestion: InterviewStep{Facet: "onboarding-track", Question: "Você prefere a entrevista curta (cerca de 7 minutos, base operacional) ou a completa (cerca de 25 minutos, identidade e preferências mais refinadas)?"}}
}

func onboarding(root string, available map[string]Facet, track, confirmedAt, confirmedDigest string) OnboardingStatus {
	definition, ok := onboardingTracks[track]
	if !ok {
		return onboardingSelectionRequired()
	}
	remaining := make([]string, 0, len(definition.Facets))
	for _, id := range definition.Facets {
		facet, ok := available[id]
		if !ok || !facet.Available || !facetAnswered(root, id) {
			remaining = append(remaining, id)
		}
	}
	if len(remaining) == 0 {
		currentDigest := onboardingDigest(root, definition)
		if confirmedAt != "" && secureDigestEqual(confirmedDigest, currentDigest) {
			return OnboardingStatus{State: "complete", Track: definition.ID, EstimatedMinutes: definition.EstimatedMinutes}
		}
		return OnboardingStatus{State: "review_required", Track: definition.ID, EstimatedMinutes: definition.EstimatedMinutes, ReviewDigest: currentDigest}
	}
	state := "in_progress"
	if len(remaining) == len(definition.Facets) {
		state = "required"
	}
	return OnboardingStatus{State: state, Track: definition.ID, EstimatedMinutes: definition.EstimatedMinutes, Remaining: remaining, NextQuestion: interviewStepForTrack(definition.ID, remaining[0])}
}

func interviewStepForTrack(track, facet string) InterviewStep {
	for _, step := range interviewForTrack(track).Steps {
		if step.Facet == facet {
			return step
		}
	}
	return InterviewStep{}
}

// SelectOnboardingTrack records an explicit owner choice. Changing track
// invalidates any prior confirmation because the reviewed facet set changes.
func SelectOnboardingTrack(root, track string) (Status, error) {
	if _, ok := onboardingTracks[track]; !ok {
		return Status{}, errors.New("onboarding track must be quick or complete")
	}
	value, err := readRegistry(root)
	if err != nil {
		return Status{}, err
	}
	value.SchemaVersion = 3
	value.OnboardingTrack = track
	value.OnboardingConfirmedAt = ""
	value.OnboardingConfirmedSHA256 = ""
	if err := writePrivateJSON(filepath.Join(root, "owner", "registry.json"), value); err != nil {
		return Status{}, err
	}
	return Inspect(root)
}

func facetAnswered(root, id string) bool {
	template, ok := facets[id]
	if !ok {
		return false
	}
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(template.Record.Path)))
	return err == nil && strings.TrimSpace(string(body)) != strings.TrimSpace(template.Body)
}

func openTasks(root, relative string) TaskStatus {
	body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return TaskStatus{State: "unavailable"}
	}
	count := 0
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "- [ ]") {
			continue
		}
		if strings.TrimSpace(strings.TrimPrefix(line, "- [ ]")) != "" {
			count++
		}
	}
	if count == 0 {
		return TaskStatus{State: "empty"}
	}
	return TaskStatus{State: "available", Count: count}
}

// ConfirmOnboarding records the owner's explicit review of the currently
// answered non-sensitive facets. Any later change invalidates the digest and
// returns onboarding to review_required.
func ConfirmOnboarding(root, expectedDigest string) (Status, error) {
	if !validOnboardingDigest(expectedDigest) {
		return Status{}, errors.New("onboarding confirmation requires the 64-character SHA-256 review_digest shown by bcgos owner onboarding status; nothing was changed")
	}
	value, err := readRegistry(root)
	if err != nil {
		return Status{}, err
	}
	status := onboarding(root, facetsFromRegistry(root, value), value.OnboardingTrack, "", "")
	if status.State != "review_required" || status.ReviewDigest == "" {
		return Status{}, errors.New("onboarding answers are not ready for owner confirmation")
	}
	if !secureDigestEqual(expectedDigest, status.ReviewDigest) {
		return Status{}, errors.New("onboarding confirmation denied because the reviewed digest no longer matches the current facets; nothing was changed; run bcgos owner onboarding status, review the updated profile, and retry with its review_digest")
	}
	value.OnboardingConfirmedAt = time.Now().UTC().Format(time.RFC3339Nano)
	value.OnboardingConfirmedSHA256 = status.ReviewDigest
	if err := writePrivateJSON(filepath.Join(root, "owner", "registry.json"), value); err != nil {
		return Status{}, err
	}
	return Inspect(root)
}

func validOnboardingDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func secureDigestEqual(left, right string) bool {
	if !validOnboardingDigest(left) || !validOnboardingDigest(right) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(strings.ToLower(left)), []byte(strings.ToLower(right))) == 1
}

func facetsFromRegistry(root string, value registry) map[string]Facet {
	result := make(map[string]Facet, len(value.Facets))
	for id, record := range value.Facets {
		result[id] = Facet{Pointer: pointer(root, record.Path), Sensitivity: record.Sensitivity, Readers: record.Readers, Refinement: record.Refinement}
	}
	return result
}

func onboardingDigest(root string, definition onboardingTrackDefinition) string {
	parts := make([]string, 0, len(definition.Facets)+1)
	parts = append(parts, "track\x00"+definition.ID)
	for _, id := range definition.Facets {
		template := facets[id]
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(template.Record.Path)))
		if err != nil {
			return ""
		}
		parts = append(parts, id+"\x00"+string(body))
	}
	return digest(strings.Join(parts, "\x00"))
}

func legacyOnboardingDigest(root string) string {
	parts := make([]string, 0, len(onboardingFacets))
	for _, id := range onboardingFacets {
		template := facets[id]
		body, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(template.Record.Path)))
		if err != nil {
			return ""
		}
		parts = append(parts, id+"\x00"+string(body))
	}
	return digest(strings.Join(parts, "\x00"))
}

func capabilities() map[string]Capability {
	return map[string]Capability{
		"cold_start_interview":   {State: "supported", Message: "guided questions are available for a runtime adapter"},
		"self_expansion":         {State: "supported", Message: "one-question owner interviews create reviewable, digest-bound local drafts"},
		"assessment_ingestion":   {State: "unavailable", Message: "assessment extraction requires an approved local ingestion adapter and explicit consent"},
		"refinement_application": {State: "supported", Message: "local proposals apply declared facet policies with audit and reversal"},
		"observation_capture":    {State: "supported", Message: "Maestro evaluates every interaction; only material owner-attested signals persist locally"},
		"prompt_history":         {State: "supported", Message: "owner-local user prompts are retained only through explicit bounded controls"},
		"refinement_synthesis":   {State: "unavailable", Message: "a future approved adapter must turn observations into a proposed change"},
	}
}

func pointer(root, relative string) Pointer {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative)))
	return Pointer{Path: relative, Available: err == nil, State: map[bool]string{true: "available", false: "missing"}[err == nil]}
}

func create(path, body string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = file.WriteString(body)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}
