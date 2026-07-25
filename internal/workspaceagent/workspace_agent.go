// Package workspaceagent owns the compact, workspace-scoped control plane for
// one workspace agent. Evidence and substantive reasoning remain in the
// workspace dossier rather than in the state read at every turn.
package workspaceagent

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	ErrResearchApprovalRequired = errors.New("external research requires explicit owner approval")
	workspaceIDPattern          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
)

type Pointer struct {
	Path      string `json:"path"`
	Available bool   `json:"available"`
}

type Status struct {
	Initialized  bool              `json:"initialized"`
	WorkspaceID  string            `json:"workspace_id"`
	AgentID      string            `json:"agent_id"`
	State        Pointer           `json:"state"`
	Dossier      Pointer           `json:"dossier"`
	Capabilities map[string]string `json:"capabilities"`
}

type InterviewStep struct {
	Field    string `json:"field"`
	Question string `json:"question"`
}

type Interview struct {
	Kind         string          `json:"kind"`
	Instructions string          `json:"instructions"`
	Steps        []InterviewStep `json:"steps"`
}

type Approval struct {
	ApprovedAt      time.Time `json:"approved_at"`
	ApprovedBy      string    `json:"approved_by"`
	DisclosureLevel string    `json:"disclosure_level"`
}

type ResearchPlan struct {
	WorkspaceID string   `json:"workspace_id"`
	Purpose     string   `json:"purpose"`
	QueryThemes []string `json:"query_themes"`
	Sources     []string `json:"sources"`
	Approval    Approval `json:"approval"`
}

type registry struct {
	SchemaVersion int    `json:"schema_version"`
	WorkspaceID   string `json:"workspace_id"`
	AgentID       string `json:"agent_id"`
}

const stateTemplate = `# Workspace agent state

Keep this file compact: active mandate, next decision, material constraints,
approved research scope, freshness signals and pointers only. Do not store raw
documents, transcripts, research summaries, embeddings or broad history here.
`

const dossierTemplate = `# Workspace dossier

Store reviewed interview notes, sourced claims, hypotheses, counter-evidence,
decisions and refresh history here. Every material claim needs provenance,
date, classification and confidence. External research requires a recorded
owner-approved plan before any query is executed.
`

func Initialize(dataRoot, workspaceID string) (Status, error) {
	if !workspaceIDPattern.MatchString(workspaceID) {
		return Status{}, fmt.Errorf("invalid workspace ID %q", workspaceID)
	}
	root := filepath.Join(dataRoot, "workspaces", workspaceID)
	if err := os.MkdirAll(filepath.Join(root, "agent"), 0o700); err != nil {
		return Status{}, err
	}
	if err := os.MkdirAll(filepath.Join(root, "dossier"), 0o700); err != nil {
		return Status{}, err
	}
	agentID := "workspace-agent-" + workspaceID
	if err := createJSON(filepath.Join(root, "agent", "agent.json"), registry{SchemaVersion: 1, WorkspaceID: workspaceID, AgentID: agentID}); err != nil {
		return Status{}, err
	}
	if err := createText(filepath.Join(root, "agent", "state.md"), stateTemplate); err != nil {
		return Status{}, err
	}
	if err := createText(filepath.Join(root, "dossier", "README.md"), dossierTemplate); err != nil {
		return Status{}, err
	}
	return Inspect(dataRoot, workspaceID)
}

func Inspect(dataRoot, workspaceID string) (Status, error) {
	if !workspaceIDPattern.MatchString(workspaceID) {
		return Status{}, fmt.Errorf("invalid workspace ID %q", workspaceID)
	}
	root := filepath.Join(dataRoot, "workspaces", workspaceID)
	registryPath := filepath.Join(root, "agent", "agent.json")
	value, err := loadRegistry(registryPath)
	if errors.Is(err, os.ErrNotExist) {
		return Status{WorkspaceID: workspaceID, Capabilities: capabilities()}, nil
	}
	if err != nil {
		return Status{}, err
	}
	if value.WorkspaceID != workspaceID || value.AgentID != "workspace-agent-"+workspaceID {
		return Status{}, errors.New("workspace agent registry does not match workspace")
	}
	return Status{
		Initialized:  true,
		WorkspaceID:  workspaceID,
		AgentID:      value.AgentID,
		State:        pointer(root, "agent/state.md"),
		Dossier:      pointer(root, "dossier/README.md"),
		Capabilities: capabilities(),
	}, nil
}

func ColdStartInterview() Interview {
	return Interview{
		Kind:         "workspace_agent_setup",
		Instructions: "Conduct a concise, user-reviewed setup interview. Do not persist answers automatically. Before external research, show the minimized query plan and obtain explicit approval.",
		Steps: []InterviewStep{
			{Field: "mandate", Question: "Qual problema, decisão ou entrega este workspace deve apoiar?"},
			{Field: "scope", Question: "Qual é o cliente/projeto, horizonte, classificação e quais materiais locais estão autorizados?"},
			{Field: "stakeholders", Question: "Quem usa o trabalho e quais restrições, riscos ou dependências importam?"},
			{Field: "hypotheses", Question: "Quais hipóteses de upside, downside e perguntas em aberto devemos testar?"},
			{Field: "research", Question: "Quais perguntas públicas justificam pesquisa e quais termos/fontes podem ser usados sem revelar estratégia confidencial?"},
			{Field: "refresh", Question: "Quais sinais ou datas devem disparar a atualização do contexto?"},
		},
	}
}

func (plan ResearchPlan) Validate() error {
	if !workspaceIDPattern.MatchString(plan.WorkspaceID) || strings.TrimSpace(plan.Purpose) == "" || len(plan.QueryThemes) == 0 || len(plan.Sources) == 0 {
		return errors.New("research plan requires workspace, purpose, query themes and sources")
	}
	if plan.Approval.ApprovedAt.IsZero() || strings.TrimSpace(plan.Approval.ApprovedBy) == "" || plan.Approval.DisclosureLevel != "public_only" {
		return ErrResearchApprovalRequired
	}
	return nil
}

func capabilities() map[string]string {
	return map[string]string{
		"guided_interview":            "supported",
		"research_plan_validation":    "supported",
		"external_research_execution": "unavailable",
		"public_economic_rollup":      "unavailable",
	}
}

func pointer(root, relative string) Pointer {
	_, err := os.Stat(filepath.Join(root, filepath.FromSlash(relative)))
	return Pointer{Path: relative, Available: err == nil}
}

func createText(path, content string) error {
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
	_, writeErr := file.WriteString(content)
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func createJSON(path string, value registry) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return err
	}
	err = json.NewEncoder(file).Encode(value)
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func loadRegistry(path string) (registry, error) {
	file, err := os.Open(path)
	if err != nil {
		return registry{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var value registry
	if err := decoder.Decode(&value); err != nil {
		return registry{}, err
	}
	if value.SchemaVersion != 1 || value.WorkspaceID == "" || value.AgentID == "" {
		return registry{}, errors.New("workspace agent registry is invalid")
	}
	return value, nil
}
