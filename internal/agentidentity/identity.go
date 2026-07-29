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
	Role              string   `json:"role"`
	Purpose           string   `json:"purpose"`
	DefaultName       string   `json:"default_name"`
	DefaultEmoji      string   `json:"default_emoji"`
	Suggestions       []string `json:"suggestions"`
	EmojiSuggestions  []string `json:"emoji_suggestions"`
	OwnershipScope    string   `json:"ownership_scope"`
	CustomizationNote string   `json:"customization_note"`
}

type InterviewStep struct {
	Field       string `json:"field"`
	Question    string `json:"question"`
	Explanation string `json:"explanation"`
}

type Interview struct {
	Kind                 string           `json:"kind"`
	SchemaVersion        int              `json:"schema_version"`
	Instructions         string           `json:"instructions"`
	OwnershipExplanation string           `json:"ownership_explanation"`
	AvatarExplanation    string           `json:"avatar_explanation"`
	Steps                []InterviewStep  `json:"steps"`
	Agents               []RoleDescriptor `json:"agents"`
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
	SchemaVersion int         `json:"schema_version"`
	OwnerID       string      `json:"owner_id"`
	Confirmed     bool        `json:"confirmed"`
	UpdatedAt     time.Time   `json:"updated_at"`
	Selections    []Selection `json:"selections"`
}

type ManagedTarget struct {
	AgentID string
	Role    string
}

var managedTargets = []ManagedTarget{
	{AgentID: "maestro", Role: "maestro"},
	{AgentID: "walter", Role: "walter"},
	{AgentID: "darwin", Role: "darwin"},
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
	switch role {
	case "account_agent":
		return "client_account_agent"
	case "workspace_agent":
		return "case_agent"
	default:
		return role
	}
}

func IsCanonicalRole(role string) bool {
	switch role {
	case "maestro", "client_account_agent", "case_agent", "walter", "darwin", "pa_expert", "practice_agent", "capability_specialist", "subject_specialist", "errand_helper":
		return true
	default:
		return false
	}
}

func InitialInterview() Interview {
	return Interview{
		Kind:                 "agent_identity_setup",
		SchemaVersion:        SchemaVersion,
		Instructions:         "Escolha nomes e emojis para os agents principais. A personalização muda a forma como eles aparecem, nunca suas permissões, escopos ou autoridade.",
		OwnershipExplanation: "Você será o owner da personalização. A autoridade operacional continua pertencendo à camada do agent: Maestro, conta, case, governança ou PA Expert registry.",
		AvatarExplanation:    "Cada agent sempre aparece com um emoji-avatar. Você pode aceitar a sugestão ou escolher outro emoji válido; o emoji não concede nenhuma capacidade.",
		Steps: []InterviewStep{
			{Field: "owner_id", Question: "Quem será o owner desta configuração de identidade?", Explanation: "Use um identificador estável do proprietário, não um nome sensível ou credencial."},
			{Field: "agent_names", Question: "Que nomes você quer usar para Maestro, Client Account, Case, Walter, Darwin e PA experts?", Explanation: "Você pode escolher individualmente ou aceitar as sugestões abaixo; conta e case serão vinculados ao agent_id concreto."},
			{Field: "agent_emojis", Question: "Que emoji-avatar deve acompanhar cada agent?", Explanation: "O emoji é visual e personalizável; a definição técnica continua versionada no catálogo."},
			{Field: "ownership_scope", Question: "Esta personalização é global, de uma conta, de um case ou do PA Expert registry?", Explanation: "O scope limita onde o nome e o avatar podem ser usados."},
			{Field: "confirmation", Question: "Confirma os nomes, emojis e ownership antes de salvar?", Explanation: "Nada é persistido sem esta confirmação explícita."},
		},
		Agents: []RoleDescriptor{
			{Role: "maestro", Purpose: "Hub user-facing que coordena o trabalho", DefaultName: "Maestro", DefaultEmoji: "🎼", Suggestions: []string{"Maestro", "Conductor", "Orquestrador"}, EmojiSuggestions: []string{"🎼", "🎵", "🎻"}, OwnershipScope: "system", CustomizationNote: "Nome e avatar podem ser personalizados pelo owner; autoridade permanece no hub."},
			{Role: "client_account_agent", Purpose: "Owner partner-like do relacionamento e contexto curado da conta", DefaultName: "Account Partner", DefaultEmoji: "🤝", Suggestions: []string{"Account Partner", "Compass", "Navigator"}, EmojiSuggestions: []string{"🤝", "🧭", "🌐"}, OwnershipScope: "account", CustomizationNote: "Personalização vale apenas para a conta registrada."},
			{Role: "case_agent", Purpose: "Executa análise, código e entregas de um case", DefaultName: "Case Lead", DefaultEmoji: "⚙️", Suggestions: []string{"Case Lead", "Forge", "Mission Control"}, EmojiSuggestions: []string{"⚙️", "🛠️", "🚀"}, OwnershipScope: "case", CustomizationNote: "Personalização vale apenas para o case registrado."},
			{Role: "walter", Purpose: "Pressure-test interno de materiais e recomendações", DefaultName: "Walter", DefaultEmoji: "🛡️", Suggestions: []string{"Walter", "Sentinel", "Red Team"}, EmojiSuggestions: []string{"🛡️", "🔍", "⚖️"}, OwnershipScope: "governance", CustomizationNote: "A personalização não altera o veto ou o gate de revisão."},
			{Role: "darwin", Purpose: "Cirurgião operacional do drift, saúde e governança do sistema", DefaultName: "Darwin", DefaultEmoji: "🧬", Suggestions: []string{"Darwin", "Observer", "Steward"}, EmojiSuggestions: []string{"🧬", "👁️", "🌱"}, OwnershipScope: "governance", CustomizationNote: "Darwin pode executar manutenção reversível e escopada em health/maestro-system; o nome e o emoji não alteram grants, escopo ou autoridade."},
			{Role: "pa_expert", Purpose: "Advisory FPA/IPA versionado pelo PA Expert registry", DefaultName: "PA Expert", DefaultEmoji: "🧠", Suggestions: []string{"PA Expert", "Advisor", "Lens"}, EmojiSuggestions: []string{"🧠", "💡", "🔬"}, OwnershipScope: "pa_expert_registry", CustomizationNote: "O owner pode personalizar a apresentação; a versão e o canon continuam centralizados no PA Expert registry."},
		},
	}
}

func (profile Profile) Validate() error {
	if profile.SchemaVersion != SchemaVersion || !safeID(profile.OwnerID) || !profile.Confirmed || profile.UpdatedAt.IsZero() || len(profile.Selections) == 0 {
		return errors.New("agent personalization profile is incomplete")
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
	case "practice_agent":
		return scope == "practice"
	case "capability_specialist", "subject_specialist":
		return scope == "execution" || scope == "workspace" || scope == "case" || scope == "account" || scope == "practice"
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
	switch role {
	case "practice_agent":
		scope = "practice"
	case "capability_specialist", "subject_specialist":
		scope = "execution"
	}
	return Selection{Role: role, DisplayName: strings.ReplaceAll(role, "_", " "), Emoji: "🔹", OwnerID: "system", OwnershipScope: scope}, true
}

func SortSelections(profile *Profile) {
	sort.Slice(profile.Selections, func(i, j int) bool {
		left := CanonicalRole(profile.Selections[i].Role) + "\x00" + profile.Selections[i].AgentID
		right := CanonicalRole(profile.Selections[j].Role) + "\x00" + profile.Selections[j].AgentID
		return left < right
	})
}
