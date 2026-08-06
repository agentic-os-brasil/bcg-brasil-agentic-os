package main

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

const workspaceFlowSchemaVersion = 1

const (
	workspaceFlowSourceMutationNone               = "none"
	workspaceFlowSourceMutationNoneUntilConfirmed = "none_until_confirmed"
	workspaceFlowSourceMutationPointerOnly        = "pointer_recorded_pending_analysis"
)

type workspaceFlowMode string

const (
	workspaceFlowModeUpdate             workspaceFlowMode = "update"
	workspaceFlowModeWorkspaceMigration workspaceFlowMode = "workspace_migration"
	workspaceFlowModeExternalImport     workspaceFlowMode = "external_import"
)

type workspaceFlowSourceKind string

const (
	workspaceFlowSourceInstalled        workspaceFlowSourceKind = "installed_maestro"
	workspaceFlowSourceMaestroWorkspace workspaceFlowSourceKind = "maestro_workspace"
	workspaceFlowSourceExternalFolder   workspaceFlowSourceKind = "external_folder"
)

type workspaceFlowSource struct {
	Kind       workspaceFlowSourceKind `json:"kind"`
	Label      string                  `json:"label"`
	Path       string                  `json:"-"`
	PathChosen bool                    `json:"path_chosen"`
}

type workspaceFlowSelection struct {
	SchemaVersion int                 `json:"schema_version"`
	FlowID        string              `json:"flow_id"`
	Mode          workspaceFlowMode   `json:"mode"`
	Source        workspaceFlowSource `json:"source"`
}

type workspaceFlowSelectionResponse struct {
	workspaceFlowSelection
	Backend string `json:"backend"`
}

type workspaceFlowItem struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type workspaceFlowCapability struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Message string `json:"message"`
}

type workspaceFlowAnalysis struct {
	SchemaVersion           int                       `json:"schema_version"`
	FlowID                  string                    `json:"flow_id"`
	Mode                    workspaceFlowMode         `json:"mode"`
	State                   string                    `json:"state"`
	Classification          string                    `json:"classification"`
	Summary                 string                    `json:"summary"`
	Source                  workspaceFlowSource       `json:"source"`
	SourceMutation          string                    `json:"source_mutation"`
	InstalledVersion        string                    `json:"installed_version,omitempty"`
	TargetVersion           string                    `json:"target_version,omitempty"`
	MigrationSummary        string                    `json:"migration_summary,omitempty"`
	WorkspacePath           string                    `json:"workspace_path,omitempty"`
	WorkspacePreserved      bool                      `json:"workspace_preserved"`
	MigrationRequired       bool                      `json:"migration_required"`
	Mapped                  []workspaceFlowItem       `json:"mapped"`
	Excluded                []workspaceFlowItem       `json:"excluded"`
	Ambiguous               []workspaceFlowItem       `json:"ambiguous"`
	CapabilitiesUnavailable []workspaceFlowCapability `json:"capabilities_unavailable"`
	PlanDigest              string                    `json:"plan_digest"`
	ConfirmationRequired    bool                      `json:"confirmation_required"`
}

type workspaceFlowStage struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type workspaceFlowReceipt struct {
	SchemaVersion  int                  `json:"schema_version"`
	FlowID         string               `json:"flow_id"`
	ReceiptID      string               `json:"receipt_id"`
	PlanDigest     string               `json:"plan_digest"`
	Operation      string               `json:"operation"`
	Status         string               `json:"status"`
	Valid          bool                 `json:"valid"`
	Ready          bool                 `json:"ready"`
	WorkspacePath  string               `json:"workspace_path,omitempty"`
	SourceMutation string               `json:"source_mutation"`
	Stages         []workspaceFlowStage `json:"stages"`
}

type workspaceFlowBackend interface {
	Analyze(context.Context, workspaceFlowSelection) (workspaceFlowAnalysis, error)
	Confirm(context.Context, workspaceFlowSelection, string) (workspaceFlowReceipt, error)
}

type workspaceFlowCapabilityError struct {
	Capability string
}

func (err *workspaceFlowCapabilityError) Error() string {
	return fmt.Sprintf("a capacidade %q ainda não está disponível; nenhuma pasta foi lida ou alterada", err.Capability)
}

type unavailableWorkspaceFlowBackend struct{}

func (unavailableWorkspaceFlowBackend) Analyze(context.Context, workspaceFlowSelection) (workspaceFlowAnalysis, error) {
	return workspaceFlowAnalysis{}, &workspaceFlowCapabilityError{Capability: "workspace_import_migration"}
}

func (unavailableWorkspaceFlowBackend) Confirm(context.Context, workspaceFlowSelection, string) (workspaceFlowReceipt, error) {
	return workspaceFlowReceipt{}, &workspaceFlowCapabilityError{Capability: "workspace_import_migration"}
}

// The fixture backend is deliberately reachable only through --simulate. It
// makes the complete contract reviewable without implying that import or
// migration exists in internal/workspaceimport or internal/workspacemigration.
type fixtureWorkspaceFlowBackend struct{}

//go:embed testdata/workspace-flow-fixtures.json
var workspaceFlowFixtureData embed.FS

type workspaceFlowFixtures struct {
	Analyses map[workspaceFlowMode]workspaceFlowAnalysis `json:"analyses"`
	Receipts map[workspaceFlowMode]workspaceFlowReceipt  `json:"receipts"`
}

func (fixtureWorkspaceFlowBackend) load() (workspaceFlowFixtures, error) {
	body, err := workspaceFlowFixtureData.ReadFile("testdata/workspace-flow-fixtures.json")
	if err != nil {
		return workspaceFlowFixtures{}, err
	}
	var fixtures workspaceFlowFixtures
	if err := json.Unmarshal(body, &fixtures); err != nil {
		return workspaceFlowFixtures{}, err
	}
	return fixtures, nil
}

func (backend fixtureWorkspaceFlowBackend) Analyze(_ context.Context, selection workspaceFlowSelection) (workspaceFlowAnalysis, error) {
	fixtures, err := backend.load()
	if err != nil {
		return workspaceFlowAnalysis{}, fmt.Errorf("carregar fixture de análise do installer: %w", err)
	}
	analysis, ok := fixtures.Analyses[selection.Mode]
	if !ok {
		return workspaceFlowAnalysis{}, fmt.Errorf("fixture de análise ausente para %q", selection.Mode)
	}
	analysis.SchemaVersion = workspaceFlowSchemaVersion
	analysis.FlowID = selection.FlowID
	analysis.Mode = selection.Mode
	analysis.Source = selection.Source
	return analysis, nil
}

func (backend fixtureWorkspaceFlowBackend) Confirm(_ context.Context, selection workspaceFlowSelection, planDigest string) (workspaceFlowReceipt, error) {
	fixtures, err := backend.load()
	if err != nil {
		return workspaceFlowReceipt{}, fmt.Errorf("carregar fixture de receipt do installer: %w", err)
	}
	receipt, ok := fixtures.Receipts[selection.Mode]
	if !ok {
		return workspaceFlowReceipt{}, fmt.Errorf("fixture de receipt ausente para %q", selection.Mode)
	}
	receipt.SchemaVersion = workspaceFlowSchemaVersion
	receipt.FlowID = selection.FlowID
	receipt.PlanDigest = planDigest
	receipt.SourceMutation = workspaceFlowSourceMutationNone
	return receipt, nil
}

func workspaceFlowBackendFor(options options) workspaceFlowBackend {
	if options.workspaceFlow != nil {
		return options.workspaceFlow
	}
	if options.simulate {
		return fixtureWorkspaceFlowBackend{}
	}
	return unavailableWorkspaceFlowBackend{}
}

func workspaceFlowBackendName(options options) string {
	if options.workspaceFlow != nil {
		return "injected"
	}
	if options.simulate {
		return "fixture"
	}
	return "unavailable"
}

func validateWorkspaceFlowMode(mode workspaceFlowMode) error {
	switch mode {
	case workspaceFlowModeUpdate, workspaceFlowModeWorkspaceMigration, workspaceFlowModeExternalImport:
		return nil
	default:
		return fmt.Errorf("modo de workspace desconhecido: %q", mode)
	}
}

func workspaceFlowSourceFor(mode workspaceFlowMode, path string) (workspaceFlowSource, error) {
	if err := validateWorkspaceFlowMode(mode); err != nil {
		return workspaceFlowSource{}, err
	}
	source := workspaceFlowSource{Path: path, PathChosen: path != ""}
	switch mode {
	case workspaceFlowModeUpdate:
		source.Kind = workspaceFlowSourceInstalled
		source.Label = "Instalação atual do Maestro"
	case workspaceFlowModeWorkspaceMigration:
		source.Kind = workspaceFlowSourceMaestroWorkspace
		source.Label = "Workspace Maestro selecionado"
	case workspaceFlowModeExternalImport:
		source.Kind = workspaceFlowSourceExternalFolder
		source.Label = "Pasta externa selecionada"
	}
	if source.Path != "" {
		absolute, err := filepath.Abs(source.Path)
		if err != nil {
			return workspaceFlowSource{}, fmt.Errorf("não foi possível normalizar a fonte selecionada: %w", err)
		}
		source.Path = filepath.Clean(absolute)
	}
	return source, nil
}

func validateWorkspaceFlowAnalysis(analysis workspaceFlowAnalysis, selection workspaceFlowSelection) error {
	if analysis.SchemaVersion != workspaceFlowSchemaVersion || analysis.FlowID != selection.FlowID || analysis.Mode != selection.Mode {
		return errors.New("análise do workspace não está vinculada à seleção atual")
	}
	if analysis.State != "plan_ready" || strings.TrimSpace(analysis.PlanDigest) == "" || !analysis.ConfirmationRequired {
		return errors.New("análise do workspace não produziu um plano confirmável")
	}
	if analysis.Source.Kind != selection.Source.Kind || analysis.Source.PathChosen != selection.Source.PathChosen {
		return errors.New("análise do workspace não está vinculada à fonte selecionada")
	}
	if analysis.SourceMutation != workspaceFlowSourceMutationNoneUntilConfirmed {
		return errors.New("análise do workspace reportou mutação antes da confirmação")
	}
	return nil
}

func workspaceFlowOperationForMode(mode workspaceFlowMode) string {
	switch mode {
	case workspaceFlowModeUpdate:
		return "maestro_update"
	case workspaceFlowModeWorkspaceMigration:
		return "workspace_migration"
	case workspaceFlowModeExternalImport:
		return "external_import"
	default:
		return ""
	}
}

func validateWorkspaceFlowReceipt(receipt workspaceFlowReceipt, selection workspaceFlowSelection, planDigest string) error {
	if receipt.SchemaVersion != workspaceFlowSchemaVersion || receipt.FlowID != selection.FlowID || receipt.PlanDigest != planDigest {
		return errors.New("receipt do workspace não está vinculado à seleção ou ao plano")
	}
	if receipt.Operation != workspaceFlowOperationForMode(selection.Mode) {
		return errors.New("receipt do workspace não corresponde à operação confirmada")
	}
	if !receipt.Valid || !receipt.Ready || receipt.Status != "committed" || strings.TrimSpace(receipt.ReceiptID) == "" {
		return errors.New("receipt do workspace é inválido; a jornada não pode mostrar pronto")
	}
	if receipt.SourceMutation != workspaceFlowSourceMutationNone {
		return errors.New("receipt do workspace não confirma que a origem permaneceu intacta")
	}
	expectedStages := map[string]string{"staging": "completed", "validation": "completed", "rollback": "available"}
	seen := make(map[string]bool, len(expectedStages))
	for _, stage := range receipt.Stages {
		expectedStatus, known := expectedStages[stage.ID]
		if !known || seen[stage.ID] {
			continue
		}
		seen[stage.ID] = true
		if stage.Status != expectedStatus || strings.TrimSpace(stage.Detail) == "" {
			return fmt.Errorf("receipt do workspace tem estado inválido na etapa %q", stage.ID)
		}
	}
	for id := range expectedStages {
		if !seen[id] {
			return fmt.Errorf("receipt do workspace não explica a etapa %q", id)
		}
	}
	return nil
}
