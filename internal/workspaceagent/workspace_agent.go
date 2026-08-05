// Package workspaceagent owns the compact, workspace-scoped control plane for
// one workspace agent. Evidence and substantive reasoning remain in the
// workspace dossier rather than in the state read at every turn.
package workspaceagent

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const maximumWorkspaceAgentRegistryBytes = 64 << 10

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
	Role         string            `json:"role"`
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
	PlanID      string    `json:"plan_id"`
	WorkspaceID string    `json:"workspace_id"`
	State       string    `json:"state"`
	CreatedAt   time.Time `json:"created_at"`
	ValidUntil  time.Time `json:"valid_until"`
	MaxQueries  int       `json:"max_queries"`
	Purpose     string    `json:"purpose"`
	QueryThemes []string  `json:"query_themes"`
	Sources     []string  `json:"sources"`
	Approval    Approval  `json:"approval"`
}

type OperationalState struct {
	SchemaVersion             int       `json:"schema_version"`
	WorkspaceID               string    `json:"workspace_id"`
	Lifecycle                 string    `json:"lifecycle"`
	CurrentBriefID            string    `json:"current_brief_id,omitempty"`
	CurrentObjective          string    `json:"current_objective,omitempty"`
	CurrentResearchPlanID     string    `json:"current_research_plan_id,omitempty"`
	CurrentEconomicSnapshotID string    `json:"current_economic_snapshot_id,omitempty"`
	CurrentPlanID             string    `json:"current_plan_id,omitempty"`
	CurrentArtifactID         string    `json:"current_artifact_id,omitempty"`
	CurrentHandoffID          string    `json:"current_handoff_id,omitempty"`
	CurrentNextStep           string    `json:"current_next_step,omitempty"`
	UpdatedAt                 time.Time `json:"updated_at"`
}

type registry struct {
	SchemaVersion int    `json:"schema_version"`
	WorkspaceID   string `json:"workspace_id"`
	AgentID       string `json:"agent_id"`
	Role          string `json:"role"`
}

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
	if err := createJSON(filepath.Join(root, "agent", "agent.json"), registry{SchemaVersion: 1, WorkspaceID: workspaceID, AgentID: agentID, Role: "case_agent"}); err != nil {
		return Status{}, err
	}
	if err := createImmutableJSON(filepath.Join(root, "agent", "state.json"), OperationalState{
		SchemaVersion: 1,
		WorkspaceID:   workspaceID,
		Lifecycle:     "setup",
		UpdatedAt:     time.Now().UTC(),
	}); err != nil && !errors.Is(err, os.ErrExist) {
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
	if value.WorkspaceID != workspaceID || value.AgentID != "workspace-agent-"+workspaceID || (value.Role != "" && value.Role != "case_agent") {
		return Status{}, errors.New("workspace agent registry does not match workspace")
	}
	state, err := requiredPointer(root, "agent/state.json")
	if err != nil {
		return Status{}, err
	}
	dossier, err := requiredPointer(root, "dossier/README.md")
	if err != nil {
		return Status{}, err
	}
	return Status{
		Initialized:  state.Available && dossier.Available,
		WorkspaceID:  workspaceID,
		AgentID:      value.AgentID,
		Role:         "case_agent",
		State:        state,
		Dossier:      dossier,
		Capabilities: capabilities(),
	}, nil
}

func ColdStartInterview() Interview {
	return Interview{
		Kind:         "case_agent_setup",
		Instructions: "Conduct a concise, user-reviewed setup interview. Do not persist answers automatically. Before external research, show the minimized query plan and obtain explicit approval.",
		Steps: []InterviewStep{
			{Field: "decision_and_horizon", Question: "Qual decisão ou entrega este workspace deve apoiar, e até quando?"},
			{Field: "audience_and_constraints", Question: "Quem usará o resultado e quais classificação, restrições ou dependências importam?"},
			{Field: "first_value", Question: "Que resultado concreto faria este primeiro passo ser útil?"},
			{Field: "authorized_material", Question: "Quais materiais locais podem ser considerados e o que deve ficar fora de escopo?"},
			{Field: "balanced_hypotheses", Question: "Qual é a hipótese de upside e a de downside mais importantes? O que mudaria cada visão?"},
			{Field: "handoff", Question: "O que continua em aberto e qual é o próximo passo prático, com responsável?"},
		},
	}
}

func (plan ResearchPlan) Validate() error {
	if !workspaceIDPattern.MatchString(plan.WorkspaceID) || !safeID(plan.PlanID) || plan.State != "approved" || strings.TrimSpace(plan.Purpose) == "" || len(plan.QueryThemes) == 0 || len(plan.Sources) == 0 || plan.ValidUntil.IsZero() || plan.MaxQueries < len(plan.QueryThemes) || plan.MaxQueries > 20 {
		return errors.New("research plan requires workspace, purpose, query themes and sources")
	}
	if !plan.ValidUntil.After(plan.CreatedAt) || time.Now().UTC().After(plan.ValidUntil) {
		return errors.New("research plan is expired")
	}
	if plan.Approval.ApprovedAt.IsZero() || strings.TrimSpace(plan.Approval.ApprovedBy) == "" || plan.Approval.DisclosureLevel != "public_only" {
		return ErrResearchApprovalRequired
	}
	return nil
}

func capabilities() map[string]string {
	return map[string]string{
		"guided_interview":            "supported",
		"versioned_brief":             "supported",
		"research_plan_validation":    "supported",
		"research_evidence_store":     "supported",
		"external_research_execution": "managed_skill_runtime_dependent",
		"public_economic_rollup":      "supported",
	}
}

func pointer(root, relative string) Pointer {
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(relative)))
	return Pointer{Path: relative, Available: err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 && info.Mode().Perm()&0o077 == 0}
}

func requiredPointer(root, relative string) (Pointer, error) {
	p := pointer(root, relative)
	path := filepath.Join(root, filepath.FromSlash(relative))
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return p, nil
	}
	if err != nil {
		return Pointer{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Pointer{}, errors.New("workspace agent dependency must be a regular non-symlink file")
	}
	if info.Mode().Perm()&0o077 != 0 {
		return Pointer{}, errors.New("workspace agent dependency must be owner-only (0600 or stricter)")
	}
	return p, nil
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
	info, err := os.Lstat(path)
	if err != nil {
		return registry{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return registry{}, errors.New("workspace agent registry must be a regular non-symlink file")
	}
	if info.Size() <= 0 || info.Size() > maximumWorkspaceAgentRegistryBytes || info.Mode().Perm()&0o077 != 0 {
		return registry{}, errors.New("workspace agent registry must be a bounded owner-only file")
	}
	file, err := os.Open(path)
	if err != nil {
		return registry{}, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maximumWorkspaceAgentRegistryBytes+1))
	if err != nil {
		return registry{}, err
	}
	if int64(len(body)) > maximumWorkspaceAgentRegistryBytes {
		return registry{}, errors.New("workspace agent registry exceeds the bounded JSON limit")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var value registry
	if err := decoder.Decode(&value); err != nil {
		return registry{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return registry{}, errors.New("workspace agent registry contains multiple JSON values")
	}
	if value.SchemaVersion != 1 || value.WorkspaceID == "" || value.AgentID == "" {
		return registry{}, errors.New("workspace agent registry is invalid")
	}
	return value, nil
}
