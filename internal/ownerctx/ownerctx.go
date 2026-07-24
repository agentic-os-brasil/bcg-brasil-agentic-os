// Package ownerctx owns human-readable, user-local owner context pointers.
package ownerctx

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
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
	Capabilities   map[string]Capability `json:"capabilities"`
}

type InterviewStep struct {
	Facet    string `json:"facet"`
	Question string `json:"question"`
}

// Interview is a runtime-neutral prompt contract. Runtimes may present it
// conversationally, but must not infer or persist answers without user review.
type Interview struct {
	Kind         string          `json:"kind"`
	Instructions string          `json:"instructions"`
	Steps        []InterviewStep `json:"steps"`
}

type facetRecord struct {
	Path        string   `json:"path"`
	Sensitivity string   `json:"sensitivity"`
	Readers     []string `json:"readers"`
	Refinement  string   `json:"refinement"`
}

type registry struct {
	SchemaVersion  int                       `json:"schema_version"`
	Facets         map[string]facetRecord    `json:"facets"`
	Producers      map[string]producerRecord `json:"producers"`
	OperatingState string                    `json:"operating_state"`
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
	"professional-role":     {facetRecord{"owner/self/professional-role.md", "professional", []string{"session", "walter"}, "proposal_only"}, "# Professional role\n\nDescreva responsabilidades, contexto profissional e resultados pelos quais voce responde.\n"},
	"communication-style":   {facetRecord{"owner/self/communication-style.md", "professional", []string{"session", "walter"}, "automatic_with_audit"}, "# Communication style\n\nDescreva como prefere colaborar com o Agentic OS: tom, nivel de detalhe, idioma e formato.\n"},
	"voice":                 {facetRecord{"owner/self/voice.md", "professional", []string{"session", "walter"}, "automatic_with_audit"}, "# Voice\n\nDescreva como voce quer falar com o mundo em entregas externas: emails, documentos, propostas e apresentacoes.\n"},
	"preferences":           {facetRecord{"owner/self/preferences.md", "professional", []string{"session", "walter"}, "automatic_with_audit"}, "# Preferences\n\nRegistre preferencias de ferramentas, formatos, rotinas e formas de trabalho.\n"},
	"decision-rules":        {facetRecord{"owner/self/decision-rules.md", "professional", []string{"session", "walter"}, "proposal_only"}, "# Decision rules\n\nRegistre principios e trade-offs que devem orientar recomendacoes importantes.\n"},
	"working-boundaries":    {facetRecord{"owner/self/working-boundaries.md", "professional", []string{"session", "walter"}, "confirmation_required"}, "# Working boundaries\n\nRegistre limites de escopo, confidencialidade e quando escalar decisoes.\n"},
	"psychological-profile": {facetRecord{"owner/self/psychological-profile.md", "sensitive", []string{"walter"}, "confirmation_required"}, "# Psychological profile\n\nOpcional. Inclua apenas uma sintese profissional revisada por voce, com fontes e finalidades autorizadas. Nunca use como diagnostico ou rotulo deterministico.\n"},
}

const statePath = "owner/operating/work-state.md"
const stateTemplate = "# Work state\n\nRegistre somente estado operacional recente: prioridades, bloqueios, proximas acoes e itens aguardando resposta.\n"

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
	if err := os.MkdirAll(filepath.Join(directory, "sources", "assessments"), 0o700); err != nil {
		return Status{}, err
	}
	path := filepath.Join(directory, "registry.json")
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		definitions := make(map[string]facetRecord, len(facets))
		for id, template := range facets {
			definitions[id] = template.Record
		}
		body, err := json.MarshalIndent(registry{SchemaVersion: 2, Facets: definitions, Producers: map[string]producerRecord{}, OperatingState: statePath}, "", "  ")
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
	if err := json.Unmarshal(file, &value); err != nil || value.SchemaVersion != 2 {
		return Status{}, errors.New("owner context registry is invalid")
	}
	status := Status{
		Initialized:    true,
		Facets:         make(map[string]Facet, len(value.Facets)),
		OperatingState: pointer(root, value.OperatingState),
		Tasks:          Pointer{State: "unavailable"},
		Capabilities:   capabilities(),
	}
	for id, record := range value.Facets {
		status.Facets[id] = Facet{Pointer: pointer(root, record.Path), Sensitivity: record.Sensitivity, Readers: record.Readers, Refinement: record.Refinement}
	}
	return status, nil
}

func ColdStartInterview() Interview {
	return Interview{
		Kind:         "cold_start",
		Instructions: "Conduza uma conversa curta. Mostre cada resposta ao dono antes de sugerir que ela seja gravada na faceta correspondente. Nao infira perfil psicologico.",
		Steps: []InterviewStep{
			{Facet: "professional-role", Question: "Qual e seu papel, principais responsabilidades e quais resultados voce precisa gerar?"},
			{Facet: "communication-style", Question: "Como voce prefere que o Agentic OS explique, estruture e comunique o trabalho com voce?"},
			{Facet: "voice", Question: "Como sua voz deve aparecer em entregas para clientes, lideres e colegas?"},
			{Facet: "preferences", Question: "Quais ferramentas, formatos e rotinas tornam seu trabalho melhor?"},
			{Facet: "decision-rules", Question: "Quais principios e trade-offs devem orientar recomendacoes importantes?"},
			{Facet: "working-boundaries", Question: "Quais limites de confidencialidade, escopo e escalonamento o sistema deve respeitar?"},
		},
	}
}

func emptyStatus() Status {
	return Status{Tasks: Pointer{State: "unavailable"}, Capabilities: capabilities()}
}

func capabilities() map[string]Capability {
	return map[string]Capability{
		"cold_start_interview":   {State: "supported", Message: "guided questions are available for a runtime adapter"},
		"assessment_ingestion":   {State: "unavailable", Message: "assessment extraction requires an approved local ingestion adapter and explicit consent"},
		"refinement_application": {State: "supported", Message: "local proposals apply declared facet policies with audit and reversal"},
		"observation_capture":    {State: "unavailable", Message: "a lifecycle adapter must provide authorized observations"},
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
