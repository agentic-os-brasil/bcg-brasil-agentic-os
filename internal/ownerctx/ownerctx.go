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

// ExpansionStatus is a bounded projection of the eight non-sensitive SELF
// facets. It contains no answer, draft body or interview token. The two
// onboarding identity/context facets are intentionally not part of this
// longitudinal professional-self expansion loop.
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
	Guidance         string        `json:"guidance,omitempty"`
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
	"owner-identity":        {facetRecord{"owner/self/owner-identity.md", "identity", []string{"session", "walter"}, "confirmation_required"}, "# Owner identity\n\n## Current\n\nRegistre como o owner prefere ser chamado. Nao inclua identificadores desnecessarios.\n"},
	"personal-context":      {facetRecord{"owner/self/personal-context.md", "sensitive", []string{"session", "walter"}, "confirmation_required"}, "# Authorized personal context\n\n## Current\n\nOpcional. Registre somente o contexto pessoal que o owner autoriza, com finalidade, leitores e limite de retencao claros. O owner pode declarar que nao quer compartilhar contexto pessoal agora.\n"},
	"professional-role":     {facetRecord{"owner/self/professional-role.md", "professional", []string{"session", "walter"}, "proposal_only"}, "# Professional role\n\n## Current\n\nDescreva responsabilidades, contexto profissional e resultados pelos quais voce responde.\n"},
	"communication-style":   {facetRecord{"owner/self/communication-style.md", "professional", []string{"session", "walter"}, "automatic_with_audit"}, "# Communication style\n\n## Current\n\nDescreva como prefere colaborar com o Agentic OS: tom, nivel de detalhe, idioma e formato.\n"},
	"voice":                 {facetRecord{"owner/self/voice.md", "professional", []string{"session", "walter"}, "automatic_with_audit"}, "# Voice\n\n## Current\n\nDescreva como voce quer falar com o mundo em entregas externas: emails, documentos, propostas e apresentacoes.\n"},
	"preferences":           {facetRecord{"owner/self/preferences.md", "professional", []string{"session", "walter"}, "automatic_with_audit"}, "# Preferences\n\n## Current\n\nRegistre preferencias de ferramentas, formatos, rotinas e formas de trabalho.\n"},
	"motivations":           {facetRecord{"owner/self/motivations.md", "professional", []string{"session", "walter"}, "proposal_only"}, "# Professional motivations\n\n## Current\n\nDescreva o impacto, os resultados e o tipo de valor que tornam seu trabalho significativo.\n"},
	"quality-bar":           {facetRecord{"owner/self/quality-bar.md", "professional", []string{"session", "walter"}, "proposal_only"}, "# Quality bar\n\n## Current\n\nDescreva o que precisa ser verdadeiro antes de considerar um trabalho pronto: criterios, validacoes, evidencias e nivel de acabamento.\n"},
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

- [Owner identity](owner-identity.md) — como o owner prefere ser chamado; sem
  identificadores desnecessarios.
- [Authorized personal context](personal-context.md) — contexto pessoal
  opcional, com finalidade, leitores e limites de retencao explicitamente
  autorizados.
- [Professional role](professional-role.md) — papel, responsabilidades e
  resultados que definem sucesso.
- [Communication style](communication-style.md) — idioma, tom, detalhe,
  formato e como o Maestro deve desafiar.
- [Preferences](preferences.md) — ferramentas, formatos, rituais e formas de
  colaboracao que ajudam ou atrapalham.
- [Voice](voice.md) — como entregas em nome do owner devem e nao devem soar.
- [Professional motivations](motivations.md) — impacto e resultados que tornam
  o trabalho importante.
- [Quality bar](quality-bar.md) — criterios de qualidade, QA, evidencias e
  acabamento exigidos antes de chamar algo de pronto.
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

` + "`personal-context.md`" + ` e opcional e sensivel. O owner pode responder
"nenhum por enquanto"; qualquer conteudo persistido deve declarar finalidade,
leitores e limite de retencao. ` + "`psychological-profile.md`" + ` e opcional,
sensivel e exclusivo de Walter; ele
nao integra entrevistas de expansao, o indice de sessao ou recomendacoes
deterministicas. A confirmacao ` + "`no_client_data`" + ` e uma declaracao do owner,
nao um classificador automatico de conteudo.
`

const (
	OnboardingTrackQuick    = "quick"
	OnboardingTrackComplete = "complete"
)

var onboardingFacets = []string{"professional-role", "communication-style", "preferences", "voice", "motivations", "quality-bar", "decision-rules", "working-boundaries"}

var onboardingIdentityFacets = []string{"owner-identity", "personal-context"}

var quickOnboardingFacets = append(append([]string(nil), onboardingIdentityFacets...), "professional-role", "communication-style", "preferences", "quality-bar")

var completeOnboardingFacets = append(append([]string(nil), onboardingIdentityFacets...), onboardingFacets...)

// These are the pre-identity track contracts. A previously confirmed profile
// must not be reopened merely because the additive identity questions were
// introduced; a fresh track selection opts into the richer contract.
var legacyQuickOnboardingFacets = []string{"professional-role", "communication-style", "preferences", "quality-bar"}
var legacyCompleteOnboardingFacets = append([]string(nil), onboardingFacets...)

// legacyOnboardingFacets is frozen at the schema-v2 contract. It must never
// grow with the current interview, otherwise an already confirmed legacy
// profile would be reinterpreted as an incomplete profile during inspection.
var legacyOnboardingFacets = []string{"professional-role", "communication-style", "voice", "preferences", "decision-rules", "working-boundaries"}

type onboardingTrackDefinition struct {
	ID               string
	EstimatedMinutes int
	Facets           []string
}

var onboardingTracks = map[string]onboardingTrackDefinition{
	OnboardingTrackQuick:    {ID: OnboardingTrackQuick, EstimatedMinutes: 10, Facets: quickOnboardingFacets},
	OnboardingTrackComplete: {ID: OnboardingTrackComplete, EstimatedMinutes: 30, Facets: completeOnboardingFacets},
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
	} else {
		// Additive migrations keep an existing owner registry and its reviewed
		// content intact while making newly introduced professional facets
		// available to the next onboarding/review pass. No existing pointer or
		// answer is replaced.
		value, err := readRegistry(root)
		if err != nil {
			return Status{}, err
		}
		changed := false
		if value.Facets == nil {
			value.Facets = make(map[string]facetRecord, len(facets))
			changed = true
		}
		for id, template := range facets {
			if _, exists := value.Facets[id]; !exists {
				value.Facets[id] = template.Record
				changed = true
			}
		}
		if value.Producers == nil {
			value.Producers = map[string]producerRecord{}
			changed = true
		}
		if changed {
			if err := writeOwnerRegistry(root, value); err != nil {
				return Status{}, err
			}
		}
	}
	return Inspect(root)
}

func Inspect(root string) (Status, error) {
	path := filepath.Join(root, "owner", "registry.json")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		return emptyStatus(), nil
	} else if err != nil {
		return Status{}, err
	}
	value, err := readRegistry(root)
	if err != nil {
		return Status{}, err
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
	legacyConfirmed := value.SchemaVersion == 2 && value.OnboardingConfirmedAt != "" && secureDigestEqual(value.OnboardingConfirmedSHA256, legacyOnboardingDigest(root))
	if legacyConfirmed {
		// Schema-v2 owners explicitly reviewed the original six-facet contract.
		// Preserve that confirmation until the owner deliberately selects a new
		// track; never turn the additive facet migration into an unsolicited
		// re-interview.
		status.Onboarding = OnboardingStatus{State: "complete", Track: OnboardingTrackComplete, EstimatedMinutes: onboardingTracks[OnboardingTrackComplete].EstimatedMinutes}
	} else {
		status.Onboarding = onboarding(root, status.Facets, track, value.OnboardingConfirmedAt, value.OnboardingConfirmedSHA256)
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
// safely, including the owner's preferred name and an explicit personal-
// context boundary. The owner may upgrade to the complete track later; it
// never infers the omitted external voice, motivations, decision rules or
// boundaries.
func QuickStartInterview() Interview {
	return interviewForTrack(OnboardingTrackQuick)
}

// IsOnboardingFacet reports whether an identifier belongs to the union of
// supported onboarding tracks. It is intentionally narrower than the full
// owner facet registry so a conversational answer cannot target a sensitive
// or unrelated facet by accident.
func IsOnboardingFacet(id string) bool {
	return containsFacet(completeOnboardingFacets, id)
}

func interviewForTrack(track string) Interview {
	definition, ok := onboardingTracks[track]
	if !ok {
		return Interview{}
	}
	allSteps := map[string]InterviewStep{
		"owner-identity":      {Facet: "owner-identity", Question: "Como voce prefere ser chamado pelo Maestro? Se quiser, diga tambem como devo pronunciar seu nome; nao preciso de outros identificadores.", AudioPrompt: "Como voce prefere ser chamado pelo Maestro?"},
		"personal-context":    {Facet: "personal-context", Question: "Existe algum contexto pessoal — por exemplo familia, energia, valores ou limites de vida — que voce autoriza o Maestro a respeitar no trabalho? Compartilhe apenas o minimo necessario ou responda ‘nenhum por enquanto’.", AudioPrompt: "Que contexto pessoal, se houver, voce autoriza o Maestro a respeitar no trabalho? Pode dizer nenhum por enquanto."},
		"professional-role":   {Facet: "professional-role", Question: "Para eu entender quem voce e no trabalho: qual e seu papel, que resultados precisa gerar e onde o Maestro deve criar mais alavancagem?"},
		"communication-style": {Facet: "communication-style", Question: "Como voce prefere que eu pense e comunique com voce: conclusao primeiro ou raciocinio detalhado, mais direto ou mais exploratorio, e em qual idioma?"},
		"voice":               {Facet: "voice", Question: "Quando eu preparar algo para outras pessoas, que voz devo preservar: tom, nivel de firmeza, linguagem e o que definitivamente nao pode aparecer?"},
		"preferences":         {Facet: "preferences", Question: "Quais formatos, ferramentas, ritmos e formas de trabalho fazem voce produzir melhor — e o que costuma tornar uma interacao ruim?"},
		"motivations":         {Facet: "motivations", Question: "O que torna um trabalho realmente importante para voce: qual impacto, tipo de valor ou resultado deve orientar minhas prioridades?"},
		"quality-bar":         {Facet: "quality-bar", Question: "Antes de eu dizer que algo esta pronto, o que precisa ser verificado: quais criterios de qualidade, QA, evidencias ou acabamento sao inegociaveis?"},
		"decision-rules":      {Facet: "decision-rules", Question: "Quando houver trade-offs, quais principios devem orientar minhas recomendacoes e quais decisoes continuam sendo sempre suas?"},
		"working-boundaries":  {Facet: "working-boundaries", Question: "Quais limites de escopo, confidencialidade, pessoas, fontes ou comunicacao externa exigem sua autorizacao antes de eu agir?"},
	}
	steps := make([]InterviewStep, 0, len(definition.Facets))
	for _, facet := range definition.Facets {
		steps = append(steps, allSteps[facet])
	}
	return Interview{
		Kind:             "cold_start",
		Track:            definition.ID,
		EstimatedMinutes: definition.EstimatedMinutes,
		Instructions:     "Conduza uma conversa com uma pergunta por vez. Depois de cada resposta, resuma o que entendeu e confirme se esta correto antes de sugerir a faceta correspondente. Comece pela identidade basica do owner e pelo contexto pessoal que ele autorizar; ‘nenhum por enquanto’ e uma resposta valida. Depois cubra o self profissional: papel, comunicacao, voz, preferencias, motivacoes, qualidade/QA, regras de decisao e limites. Nao infira personalidade, psicologia, historia pessoal, fe ou preferencias visuais; qualquer camada alem do contexto explicitamente autorizado exige uma etapa local separada.",
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
	return OnboardingStatus{State: "required", Track: "selection_required", Remaining: append([]string(nil), completeOnboardingFacets...), NextQuestion: InterviewStep{Facet: "onboarding-track", Question: "Você prefere a entrevista curta (cerca de 10 minutos: seu nome, contexto pessoal autorizado, papel, comunicação, preferências e qualidade/QA) ou a completa (cerca de 30 minutos: identidade, contexto autorizado e as oito facetas do seu self profissional)?"}}
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
	// Preserve a profile confirmed under the pre-identity contract. This is an
	// additive migration: a later explicit track selection will require the new
	// identity/context questions, but an update must not silently invalidate a
	// reviewed owner profile.
	if confirmedAt != "" && legacyDigestMatches(root, definition.ID, confirmedDigest) {
		return OnboardingStatus{State: "complete", Track: definition.ID, EstimatedMinutes: definition.EstimatedMinutes}
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
	if err := writeOwnerRegistry(root, value); err != nil {
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
	if err := writeOwnerRegistry(root, value); err != nil {
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

func legacyDigestMatches(root, track, confirmedDigest string) bool {
	var ids []string
	switch track {
	case OnboardingTrackQuick:
		ids = legacyQuickOnboardingFacets
	case OnboardingTrackComplete:
		ids = legacyCompleteOnboardingFacets
	default:
		return false
	}
	return secureDigestEqual(confirmedDigest, onboardingDigestForFacets(root, track, ids))
}

func onboardingDigestForFacets(root, track string, ids []string) string {
	parts := make([]string, 0, len(ids)+1)
	parts = append(parts, "track\x00"+track)
	for _, id := range ids {
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
	parts := make([]string, 0, len(legacyOnboardingFacets))
	for _, id := range legacyOnboardingFacets {
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
