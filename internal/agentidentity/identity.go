// Package agentidentity owns the user-visible identity layer for managed
// agents. Names and avatars are customization, not authority.
package agentidentity

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const SchemaVersion = 1

type RoleDescriptor struct {
	Role                 string                `json:"role"`
	Purpose              string                `json:"purpose"`
	DefaultName          string                `json:"default_name"`
	DefaultEmoji         string                `json:"default_emoji"`
	Suggestions          []string              `json:"suggestions"`
	EmojiSuggestions     []string              `json:"emoji_suggestions"`
	NarrativeSuggestions []NarrativeSuggestion `json:"narrative_suggestions,omitempty"`
	OwnershipScope       string                `json:"ownership_scope"`
	CustomizationNote    string                `json:"customization_note"`
}

// NarrativeSuggestion is a transparent, reversible naming reference. BestFor
// describes an explicitly stated working preference, never an inferred
// psychological profile or a change in agent authority.
type NarrativeSuggestion struct {
	Name      string   `json:"name"`
	Reference string   `json:"reference"`
	Story     string   `json:"story"`
	BestFor   []string `json:"best_for"`
	AvoidWhen string   `json:"avoid_when,omitempty"`
}

type InterviewStep struct {
	Field       string `json:"field"`
	Question    string `json:"question"`
	Explanation string `json:"explanation"`
}

// ProfileInputContract makes the interview self-describing for a runtime
// agent. The human-facing field names in Steps (for example agent_names) are
// interview concepts; this contract is the exact strict JSON envelope used
// by `agent personalize draft`.
type ProfileInputContract struct {
	Command              string            `json:"command"`
	SchemaVersion        int               `json:"schema_version"`
	RequiredFields       []string          `json:"required_fields"`
	OptionalFields       []string          `json:"optional_fields"`
	SelectionFields      []string          `json:"selection_fields"`
	AgentIDRequiredRoles []string          `json:"agent_id_required_roles"`
	OwnershipScopes      map[string]string `json:"ownership_scopes"`
	Guidance             string            `json:"guidance"`
}

type Interview struct {
	Kind                 string               `json:"kind"`
	SchemaVersion        int                  `json:"schema_version"`
	Instructions         string               `json:"instructions"`
	OwnershipExplanation string               `json:"ownership_explanation"`
	AvatarExplanation    string               `json:"avatar_explanation"`
	Steps                []InterviewStep      `json:"steps"`
	Agents               []RoleDescriptor     `json:"agents"`
	CapabilityTracks     []CapabilityTrack    `json:"capability_tracks"`
	ProfileInput         ProfileInputContract `json:"profile_input"`
}

type CapabilityTrack struct {
	ID           string `json:"id"`
	DisplayName  string `json:"display_name"`
	Description  string `json:"description"`
	Availability string `json:"availability"`
}

type Selection struct {
	Role           string `json:"role"`
	AgentID        string `json:"agent_id,omitempty"`
	DisplayName    string `json:"display_name"`
	Emoji          string `json:"emoji"`
	OwnerID        string `json:"owner_id"`
	OwnershipScope string `json:"ownership_scope"`
}

type Profile struct {
	SchemaVersion    int         `json:"schema_version"`
	OwnerID          string      `json:"owner_id"`
	Confirmed        bool        `json:"confirmed"`
	UpdatedAt        time.Time   `json:"updated_at"`
	Selections       []Selection `json:"selections"`
	CapabilityTracks []string    `json:"capability_tracks,omitempty"`
}

type ManagedTarget struct {
	AgentID string
	Role    string
}

var managedTargets = []ManagedTarget{
	{AgentID: "maestro", Role: "maestro"},
	{AgentID: "walter", Role: "walter"},
	{AgentID: "darwin", Role: "darwin"},
	{AgentID: "gamma-guardian", Role: "quality_guardian"},
}

var safeID = func(value string) bool {
	if value == "" || len(value) > 80 {
		return false
	}
	for _, r := range value {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

func CanonicalRole(role string) string {
	role = strings.TrimSpace(strings.ToLower(role))
	switch role {
	case "account_agent":
		return "client_account_agent"
	case "workspace_agent":
		return "case_agent"
	case "gamma_guardian", "gamma-guardian":
		return "quality_guardian"
	default:
		return role
	}
}

func IsCanonicalRole(role string) bool {
	switch role {
	case "maestro", "client_account_agent", "case_agent", "walter", "darwin", "pa_expert", "errand_helper", "quality_guardian":
		return true
	default:
		return false
	}
}

func InitialInterview() Interview {
	return Interview{
		Kind:                 "agent_identity_setup",
		SchemaVersion:        SchemaVersion,
		Instructions:         "Escolha nomes e emojis para os agents principais e, se quiser, selecione trilhas profissionais para ativar no Canary. As referências narrativas são um repertório de apresentação: sugestões só podem refletir preferências que o owner declarar explicitamente, nunca uma inferência de perfil. A personalização e a seleção de trilhas não mudam permissões, escopos ou autoridade.",
		OwnershipExplanation: "Você será o owner da personalização. A autoridade operacional continua pertencendo à camada do agent: Maestro, conta, case, governança ou PA Expert registry.",
		AvatarExplanation:    "Cada agent sempre aparece com um emoji-avatar. Você pode aceitar a sugestão ou escolher outro emoji válido; o emoji não concede nenhuma capacidade.",
		Steps: []InterviewStep{
			{Field: "owner_id", Question: "Quem será o owner desta configuração de identidade?", Explanation: "Use um identificador estável do proprietário, não um nome sensível ou credencial."},
			{Field: "agent_names", Question: "Que nomes você quer usar para Maestro, Client Account, Case, Walter, Darwin e PA experts?", Explanation: "Você pode escolher individualmente ou aceitar as sugestões abaixo; conta e case serão vinculados ao agent_id concreto."},
			{Field: "agent_emojis", Question: "Que emoji-avatar deve acompanhar cada agent?", Explanation: "O emoji é visual e personalizável; a definição técnica continua versionada no catálogo."},
			{Field: "ownership_scope", Question: "Esta personalização é global, de uma conta, de um case ou do PA Expert registry?", Explanation: "O scope limita onde o nome e o avatar podem ser usados."},
			{Field: "capability_tracks", Question: "A partir da sua função, qual trilha deve orientar o roteamento? O Tech Core já vem incluído com engineering, data, AI e qualidade; se a função não for claramente técnica, pergunto antes de sugerir.", Explanation: "A recomendação é derivada somente da função declarada e nunca altera a instalação. A seleção de uma trilha técnica personaliza roteamento e divulgação, enquanto o Tech Core completo já está disponível desde a primeira projeção."},
			{Field: "confirmation", Question: "Confirma os nomes, emojis, ownership e trilhas antes de salvar?", Explanation: "Nada é persistido ou ativado sem esta confirmação explícita."},
		},
		Agents: []RoleDescriptor{
			{Role: "maestro", Purpose: "Hub user-facing que coordena o trabalho", DefaultName: "Maestro", DefaultEmoji: "🎼", Suggestions: []string{"Maestro", "Conductor", "Orquestrador"}, EmojiSuggestions: []string{"🎼", "🎵", "🎻"}, OwnershipScope: "system", CustomizationNote: "Nome e avatar podem ser personalizados pelo owner; autoridade permanece no hub."},
			{Role: "client_account_agent", Purpose: "Owner partner-like do relacionamento e contexto curado da conta", DefaultName: "Account Partner", DefaultEmoji: "🤝", Suggestions: []string{"Account Partner", "Compass", "Navigator"}, EmojiSuggestions: []string{"🤝", "🧭", "🌐"}, OwnershipScope: "account", CustomizationNote: "Personalização vale apenas para a conta registrada."},
			{Role: "case_agent", Purpose: "Executa análise, código e entregas de um case", DefaultName: "Case Lead", DefaultEmoji: "⚙️", Suggestions: []string{"Case Lead", "Forge", "Mission Control"}, EmojiSuggestions: []string{"⚙️", "🛠️", "🚀"}, OwnershipScope: "case", CustomizationNote: "Personalização vale apenas para o case registrado."},
			{Role: "walter", Purpose: "Senior advisor e alter ego calmo do owner: refina trabalho de alto leverage contra a intenção intrínseca", DefaultName: "Walter", DefaultEmoji: "🦉", Suggestions: []string{"Walter", "Virgil", "Iroh", "Jarvis", "Athena"}, EmojiSuggestions: []string{"🦉", "🔍", "🧭"}, NarrativeSuggestions: walterNarrativeSuggestions(), OwnershipScope: "governance", CustomizationNote: "Walter refina; não é um naysayer. Nome e avatar podem mudar agora ou depois, sem alterar sua autoridade."},
			{Role: "darwin", Purpose: "Meta-harness evolutivo para saúde, housekeeping e caminhos deliberados de survive and thrive", DefaultName: "Darwin", DefaultEmoji: "🧬", Suggestions: []string{"Darwin", "TARS", "Ariadne", "EVE", "Data"}, EmojiSuggestions: []string{"🧬", "🌱", "🪴"}, NarrativeSuggestions: darwinNarrativeSuggestions(), OwnershipScope: "governance", CustomizationNote: "Darwin orienta evolução e manutenção reversível escopada; nome e avatar podem mudar agora ou depois, sem alterar grants, escopo ou autoridade."},
			{Role: "quality_guardian", Purpose: "Avalia longitudinalmente a qualidade de código e arquitetura", DefaultName: "Gamma Guardian", DefaultEmoji: "🧪", Suggestions: []string{"Gamma Guardian", "Verifier", "Quality Lens"}, EmojiSuggestions: []string{"🧪", "🔬", "✅"}, OwnershipScope: "quality_longitudinal", CustomizationNote: "Gamma é um spoke direto do Maestro, somente leitura e sem contexto de case; o nome e o emoji não alteram o rubric, grants ou autoridade."},
			{Role: "pa_expert", Purpose: "Advisory FPA/IPA versionado pelo PA Expert registry", DefaultName: "PA Expert", DefaultEmoji: "🧠", Suggestions: []string{"PA Expert", "Advisor", "Lens"}, EmojiSuggestions: []string{"🧠", "💡", "🔬"}, OwnershipScope: "pa_expert_registry", CustomizationNote: "O owner pode personalizar a apresentação; a versão e o canon continuam centralizados no PA Expert registry."},
		},
		CapabilityTracks: []CapabilityTrack{
			{ID: "technical-explorer", DisplayName: "Explorador técnico", Description: "Métodos técnicos para estruturar e revisar trabalho de software.", Availability: "optional"},
			{ID: "software-engineering", DisplayName: "Engenharia de software", Description: "Entrega orientada por especificação, revisão humana e evidência de testes.", Availability: "optional"},
			{ID: "data-science", DisplayName: "Ciência de dados", Description: "Avaliação de ciência de dados e decisão de promoção baseada em evidência.", Availability: "optional"},
			{ID: "data-engineering", DisplayName: "Engenharia de dados", Description: "Qualidade e reprodutibilidade de pipelines de dados.", Availability: "optional"},
			{ID: "ai-engineering", DisplayName: "Engenharia de AI", Description: "Trabalho aplicado de inteligência artificial, modelos e sistemas baseados em AI.", Availability: "optional"},
		},
		ProfileInput: ProfileInputContract{
			Command:              "bcgos agent personalize draft --stdin --consent --no-client-data",
			SchemaVersion:        SchemaVersion,
			RequiredFields:       []string{"schema_version", "owner_id", "confirmed", "selections"},
			OptionalFields:       []string{"capability_tracks"},
			SelectionFields:      []string{"role", "agent_id", "display_name", "emoji", "owner_id", "ownership_scope"},
			AgentIDRequiredRoles: []string{"client_account_agent", "case_agent"},
			OwnershipScopes: map[string]string{
				"maestro":              "system",
				"client_account_agent": "account",
				"case_agent":           "case",
				"walter":               "governance",
				"darwin":               "governance",
				"quality_guardian":     "quality_longitudinal",
				"pa_expert":            "pa_expert_registry",
			},
			Guidance: "Use selections[]; agent_names, agent_emojis, scope and ownership_scope are interview labels, not top-level profile fields. Keep only the current guided main-agent answer (Maestro, then Walter, then Darwin) in each draft. For these managed agents, agent_id may be omitted or set to the canonical ID shown by bcgos agent identity: maestro, walter or darwin.",
		},
	}
}

func walterNarrativeSuggestions() []NarrativeSuggestion {
	return []NarrativeSuggestion{
		{Name: "Walter", Reference: "identidade original", Story: "O alter ego sóbrio que preserva continuidade e testa se a intenção intrínseca foi atendida.", BestFor: []string{"alter ego", "continuidade", "sobriedade"}},
		{Name: "Virgil", Reference: "A Divina Comédia", Story: "O guia que atravessa complexidade sem tomar a jornada pelo outro.", BestFor: []string{"clareza", "perspectiva", "decisões complexas"}},
		{Name: "Iroh", Reference: "Avatar: A Lenda de Aang", Story: "Mentoria serena, humana e paciente, com espaço para reflexão.", BestFor: []string{"calma", "humanidade", "reflexão"}},
		{Name: "Morpheus", Reference: "Matrix", Story: "O advisor que questiona o aparente e convida a enxergar premissas ocultas.", BestFor: []string{"questionar premissas", "mudança", "franqueza"}},
		{Name: "Atticus", Reference: "O Sol é para Todos", Story: "Uma presença de integridade, equilíbrio e clareza moral.", BestFor: []string{"ética", "equilíbrio", "clareza moral"}},
		{Name: "Obi-Wan", Reference: "Star Wars", Story: "O guia experiente que prepara o caminho sem retirar agência.", BestFor: []string{"mentoria", "legado", "agência"}},
		{Name: "Athena", Reference: "mitologia grega", Story: "Estratégia e prudência para escolhas que precisam de visão ampla.", BestFor: []string{"estratégia", "decisão", "prudência"}},
		{Name: "Samwise", Reference: "O Senhor dos Anéis", Story: "O companheiro leal que sustenta execução e resiliência.", BestFor: []string{"parceria", "resiliência", "execução"}},
		{Name: "Jarvis", Reference: "Homem de Ferro", Story: "O advisor técnico, preciso e elegante que torna trabalho complexo legível.", BestFor: []string{"precisão", "tecnologia", "elegância"}},
	}
}

func darwinNarrativeSuggestions() []NarrativeSuggestion {
	return []NarrativeSuggestion{
		{Name: "Darwin", Reference: "identidade original", Story: "O meta-harness que mantém o sistema saudável e o ajuda a evoluir com continuidade.", BestFor: []string{"evolução", "ciência", "continuidade"}},
		{Name: "TARS", Reference: "Interestelar", Story: "Um sistema resiliente e pragmático, orientado à missão sob pressão.", BestFor: []string{"resiliência", "pragmatismo", "sistemas"}},
		{Name: "Ariadne", Reference: "A Origem", Story: "A arquiteta que torna sistemas complexos navegáveis e mapeia novos caminhos.", BestFor: []string{"arquitetura", "complexidade", "mapas"}},
		{Name: "Daedalus", Reference: "mitologia grega", Story: "O inventor que projeta saídas quando o caminho ainda não existe.", BestFor: []string{"invenção", "engenharia", "novos caminhos"}},
		{Name: "EVE", Reference: "WALL-E", Story: "A exploradora que busca sinais de futuro com leveza e esperança.", BestFor: []string{"otimismo", "sinais de futuro", "leveza"}},
		{Name: "HAL", Reference: "2001: Uma Odisseia no Espaço", Story: "Uma referência clássica de sistema de bordo, preservada como opção consciente.", BestFor: []string{"ficção científica clássica"}, AvoidWhen: "Não sugerir por padrão: a referência pode remeter a vigilância e perda de controle."},
		{Name: "KITT", Reference: "A Supermáquina", Story: "O companheiro tecnológico que combina diagnóstico com uma presença mais leve.", BestFor: []string{"companhia tecnológica", "diagnóstico", "retro"}},
		{Name: "Data", Reference: "Star Trek: A Nova Geração", Story: "Curiosidade disciplinada e aprendizagem contínua em direção à humanidade.", BestFor: []string{"aprendizado", "curiosidade", "humanidade"}},
		{Name: "The Doctor", Reference: "Star Trek: Voyager", Story: "O observador cuidadoso que usa protocolos para diagnosticar e melhorar.", BestFor: []string{"diagnóstico", "cuidado", "protocolos"}},
		{Name: "Hermione", Reference: "Harry Potter", Story: "Método, preparo e memória bem organizada antes de agir.", BestFor: []string{"método", "preparo", "memória"}},
	}
}

// RecommendNarrativeSuggestions returns a small, deterministic subset for
// preferences the owner explicitly selected or stated. It deliberately does
// not inspect owner history or infer preferences from any other context.
func RecommendNarrativeSuggestions(role string, explicitPreferences []string, limit int) []NarrativeSuggestion {
	if limit <= 0 || len(explicitPreferences) == 0 {
		return nil
	}
	if limit > 3 {
		limit = 3
	}
	var candidates []NarrativeSuggestion
	switch CanonicalRole(role) {
	case "walter":
		candidates = walterNarrativeSuggestions()
	case "darwin":
		candidates = darwinNarrativeSuggestions()
	default:
		return nil
	}
	preferences := map[string]bool{}
	for _, preference := range explicitPreferences {
		if normalized := normalizedNarrativeTag(preference); normalized != "" {
			for _, tag := range narrativePreferenceTags(normalized) {
				preferences[tag] = true
			}
		}
	}
	type rankedSuggestion struct {
		suggestion NarrativeSuggestion
		score      int
	}
	ranked := make([]rankedSuggestion, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.AvoidWhen != "" {
			continue
		}
		score := 0
		for _, tag := range candidate.BestFor {
			if preferences[normalizedNarrativeTag(tag)] {
				score++
			}
		}
		if score > 0 {
			ranked = append(ranked, rankedSuggestion{suggestion: candidate, score: score})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	if limit > len(ranked) {
		limit = len(ranked)
	}
	result := make([]NarrativeSuggestion, limit)
	for index := range result {
		result[index] = ranked[index].suggestion
	}
	return result
}

func normalizedNarrativeTag(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

// narrativePreferenceTags translates only the six phrases the onboarding
// explicitly offers into the canonical tags carried by each reference. It is
// intentionally closed: arbitrary conversation text cannot become profiling
// input for an identity recommendation.
func narrativePreferenceTags(preference string) []string {
	switch preference {
	case "guia sereno":
		return []string{"calma", "mentoria"}
	case "estrategista":
		return []string{"estratégia", "prudência"}
	case "parceiro firme":
		return []string{"parceria", "resiliência"}
	case "advisor técnico":
		return []string{"tecnologia", "precisão"}
	case "arquiteto de sistemas":
		return []string{"arquitetura", "sistemas"}
	case "observador de evolução":
		return []string{"evolução", "continuidade"}
	default:
		return []string{preference}
	}
}

func (profile Profile) Validate() error {
	if profile.SchemaVersion != SchemaVersion {
		return fmt.Errorf("agent personalization schema_version must be %d", SchemaVersion)
	}
	if !safeID(profile.OwnerID) {
		return errors.New("agent personalization owner_id is required and must be a bounded identifier")
	}
	if !profile.Confirmed {
		return errors.New("agent personalization confirmed must be true before persistence")
	}
	if profile.UpdatedAt.IsZero() {
		return errors.New("agent personalization updated_at is required")
	}
	if len(profile.Selections) == 0 {
		return errors.New("at least one selection is required for agent personalization")
	}
	if len(profile.Selections) > 128 {
		return errors.New("agent personalization supports at most 128 selections")
	}
	if len(profile.CapabilityTracks) > 16 {
		return errors.New("agent personalization supports at most 16 capability tracks")
	}
	seenTracks := map[string]bool{}
	for _, track := range profile.CapabilityTracks {
		if !safeID(track) || seenTracks[track] {
			return fmt.Errorf("capability track %q is invalid or duplicated", track)
		}
		seenTracks[track] = true
	}
	seen := map[string]bool{}
	for _, selection := range profile.Selections {
		selection.Role = CanonicalRole(selection.Role)
		if err := ValidateSelection(selection); err != nil {
			return fmt.Errorf("agent personalization selection for %q is invalid", selection.Role)
		}
		if selection.OwnerID != profile.OwnerID {
			return fmt.Errorf("agent personalization selection for %q must use the profile owner", selection.Role)
		}
		if (selection.Role == "client_account_agent" || selection.Role == "case_agent") && selection.AgentID == "" {
			return fmt.Errorf("agent personalization selection for %q requires an explicit agent_id", selection.Role)
		}
		key := selection.Role + "\x00" + selection.AgentID
		if seen[key] {
			return errors.New("agent personalization selections must be unique")
		}
		seen[key] = true
	}
	return nil
}

func ValidateSelection(selection Selection) error {
	selection.Role = CanonicalRole(selection.Role)
	if !IsCanonicalRole(selection.Role) || (selection.AgentID != "" && !safeID(selection.AgentID)) ||
		!validDisplayName(selection.DisplayName) || !validEmoji(selection.Emoji) || !safeID(selection.OwnerID) ||
		!validOwnershipScope(selection.Role, selection.OwnershipScope) {
		return errors.New("agent personalization selection is invalid")
	}
	return nil
}

func validDisplayName(value string) bool {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || len([]byte(trimmed)) > 80 {
		return false
	}
	for _, r := range trimmed {
		if unicode.IsControl(r) || r == '\n' || r == '\r' {
			return false
		}
	}
	return true
}

func validOwnershipScope(role, scope string) bool {
	switch role {
	case "maestro":
		return scope == "system"
	case "client_account_agent":
		return scope == "account"
	case "case_agent":
		return scope == "case"
	case "walter", "darwin":
		return scope == "governance"
	case "pa_expert":
		return scope == "pa_expert_registry"
	case "quality_guardian":
		return scope == "quality_longitudinal"
	case "errand_helper":
		return scope == "system"
	default:
		return false
	}
}

func validEmoji(value string) bool {
	if value == "" || len([]byte(value)) > 16 || !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return false
	}
	for _, r := range value {
		if r >= 0x1F000 || (r >= 0x2300 && r <= 0x27FF) {
			return true
		}
	}
	return false
}

func Save(root string, profile Profile) error {
	for index := range profile.Selections {
		profile.Selections[index].Role = CanonicalRole(profile.Selections[index].Role)
	}
	SortSelections(&profile)
	sort.Strings(profile.CapabilityTracks)
	if err := profile.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(root, "agents"), 0o700); err != nil {
		return err
	}
	profile.UpdatedAt = profile.UpdatedAt.UTC()
	body, err := json.MarshalIndent(profile, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	temporary, err := os.CreateTemp(filepath.Join(root, "agents"), ".personalization-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(body); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, filepath.Join(root, "agents", "personalization.json"))
}

func Load(root string) (Profile, error) {
	body, err := os.ReadFile(filepath.Join(root, "agents", "personalization.json"))
	if err != nil {
		return Profile{}, err
	}
	var profile Profile
	if err := DecodeStrict(body, &profile); err != nil {
		return Profile{}, err
	}
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func DecodeStrict(body []byte, target *Profile) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("agent personalization input contains trailing JSON")
		}
		return err
	}
	return nil
}

func Resolve(profile Profile, role, agentID string) (Selection, bool) {
	role = CanonicalRole(role)
	for _, selection := range profile.Selections {
		if CanonicalRole(selection.Role) == role && selection.AgentID == agentID {
			return selection, true
		}
	}
	if role == "client_account_agent" || role == "case_agent" {
		return Selection{}, false
	}
	for _, selection := range profile.Selections {
		if CanonicalRole(selection.Role) == role && selection.AgentID == "" {
			return selection, true
		}
	}
	return Selection{}, false
}

func ResolveManaged(profile Profile) []Selection {
	resolved := make([]Selection, 0, len(managedTargets))
	for _, target := range managedTargets {
		selection, ok := Resolve(profile, target.Role, target.AgentID)
		if !ok {
			selection, _ = Default(target.Role)
		}
		selection.Role = target.Role
		selection.AgentID = target.AgentID
		resolved = append(resolved, selection)
	}
	return resolved
}

func Default(role string) (Selection, bool) {
	role = CanonicalRole(role)
	for _, descriptor := range InitialInterview().Agents {
		if descriptor.Role == role {
			return Selection{Role: role, DisplayName: descriptor.DefaultName, Emoji: descriptor.DefaultEmoji, OwnerID: "system", OwnershipScope: descriptor.OwnershipScope}, true
		}
	}
	role = CanonicalRole(role)
	if !IsCanonicalRole(role) {
		return Selection{}, false
	}
	scope := "system"
	return Selection{Role: role, DisplayName: strings.ReplaceAll(role, "_", " "), Emoji: "🔹", OwnerID: "system", OwnershipScope: scope}, true
}

func SortSelections(profile *Profile) {
	sort.Slice(profile.Selections, func(i, j int) bool {
		left := CanonicalRole(profile.Selections[i].Role) + "\x00" + profile.Selections[i].AgentID
		right := CanonicalRole(profile.Selections[j].Role) + "\x00" + profile.Selections[j].AgentID
		return left < right
	})
}
