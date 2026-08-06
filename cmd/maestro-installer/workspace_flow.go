package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceimport"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacemigration"
)

const workspaceFlowSchemaVersion = 1

const (
	workspaceFlowApprovalImport     = workspaceimport.ConfirmImport
	workspaceFlowApprovalRollback   = workspaceimport.ConfirmRollback
	workspaceFlowSourcePreserved    = "preserved"
	workspaceFlowTargetNone         = "none_before_approval"
	workspaceFlowTargetImport       = "bounded_import_after_approval"
	workspaceFlowTargetMigration    = "managed_in_place_when_authorized"
	workspaceFlowRollbackNotCreated = "not_created"
	workspaceFlowRollbackImport     = "available_from_import_receipt"
	workspaceFlowRollbackMigration  = "bounded_snapshot_when_authorized"
	workspaceFlowPointerState       = "pointer_recorded_pending_analysis"
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

type workspaceFlowBlocker struct {
	Code    string `json:"code"`
	Message string `json:"message"`
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
	SourceEffect            string                    `json:"source_effect"`
	TargetEffect            string                    `json:"target_effect"`
	RollbackEffect          string                    `json:"rollback_effect"`
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
	Blockers                []workspaceFlowBlocker    `json:"blockers,omitempty"`
	PlanID                  string                    `json:"plan_id,omitempty"`
	PlanDigest              string                    `json:"plan_digest"`
	ConfirmationRequired    bool                      `json:"confirmation_required"`
	ApprovalAction          string                    `json:"approval_action,omitempty"`
	CanConfirm              bool                      `json:"can_confirm"`
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
	RunID          string               `json:"run_id,omitempty"`
	PlanID         string               `json:"plan_id,omitempty"`
	PlanDigest     string               `json:"plan_digest"`
	Operation      string               `json:"operation"`
	Status         string               `json:"status"`
	Valid          bool                 `json:"valid"`
	Ready          bool                 `json:"ready"`
	WorkspacePath  string               `json:"workspace_path,omitempty"`
	SourceEffect   string               `json:"source_effect"`
	TargetEffect   string               `json:"target_effect"`
	RollbackEffect string               `json:"rollback_effect"`
	ApprovalAction string               `json:"approval_action,omitempty"`
	ApprovedBy     string               `json:"approved_by,omitempty"`
	ApprovalPlanID string               `json:"approval_plan_id,omitempty"`
	Stages         []workspaceFlowStage `json:"stages"`
}

type workspaceFlowBackend interface {
	Analyze(context.Context, workspaceFlowSelection) (workspaceFlowAnalysis, error)
	Confirm(context.Context, workspaceFlowSelection, string, string) (workspaceFlowReceipt, error)
	Rollback(context.Context, workspaceFlowSelection, string, string, string) (workspaceFlowReceipt, error)
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

func (unavailableWorkspaceFlowBackend) Confirm(context.Context, workspaceFlowSelection, string, string) (workspaceFlowReceipt, error) {
	return workspaceFlowReceipt{}, &workspaceFlowCapabilityError{Capability: "workspace_import_migration"}
}

func (unavailableWorkspaceFlowBackend) Rollback(context.Context, workspaceFlowSelection, string, string, string) (workspaceFlowReceipt, error) {
	return workspaceFlowReceipt{}, &workspaceFlowCapabilityError{Capability: "workspace_import_migration"}
}

type realWorkspaceFlowBackend struct {
	options options
	mu      sync.Mutex
	imports map[string]*realWorkspaceImportFlow
}

type realWorkspaceImportFlow struct {
	selection workspaceFlowSelection
	analysis  workspaceFlowAnalysis
	plan      workspaceimport.Plan
	receipt   *workspaceimport.Receipt
	approval  *workspaceimport.Approval
}

func newRealWorkspaceFlowBackend(options options) *realWorkspaceFlowBackend {
	return &realWorkspaceFlowBackend{options: options, imports: make(map[string]*realWorkspaceImportFlow)}
}

func (backend *realWorkspaceFlowBackend) Analyze(_ context.Context, selection workspaceFlowSelection) (workspaceFlowAnalysis, error) {
	switch selection.Mode {
	case workspaceFlowModeExternalImport:
		return backend.analyzeExternalImport(selection)
	case workspaceFlowModeWorkspaceMigration:
		return backend.analyzeWorkspaceMigration(selection)
	case workspaceFlowModeUpdate:
		return backend.blockedAnalysis(selection, "update_authority_unavailable", "A atualização do workspace depende da autoridade pós-bootstrapper, que ainda não está exposta pelo installer público."), nil
	default:
		return workspaceFlowAnalysis{}, fmt.Errorf("modo de workspace desconhecido: %q", selection.Mode)
	}
}

func (backend *realWorkspaceFlowBackend) Confirm(_ context.Context, selection workspaceFlowSelection, planDigest, action string) (workspaceFlowReceipt, error) {
	if selection.Mode != workspaceFlowModeExternalImport {
		return workspaceFlowReceipt{}, &workspaceFlowCapabilityError{Capability: "workspace_migration_execution"}
	}
	if action != workspaceFlowApprovalImport {
		return workspaceFlowReceipt{}, &workspaceFlowBackendError{Code: "invalid_approval", Status: 409, Message: "a confirmação explícita IMPORT é obrigatória"}
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	flow, ok := backend.imports[selection.FlowID]
	if !ok {
		return workspaceFlowReceipt{}, &workspaceFlowBackendError{Code: "flow_expired", Status: 404, Message: "a análise do import expirou; selecione a fonte novamente"}
	}
	if flow.plan.PlanDigest != planDigest || flow.analysis.PlanDigest != planDigest {
		return workspaceFlowReceipt{}, &workspaceFlowBackendError{Code: "plan_mismatch", Status: 409, Message: "o plano mudou; execute a análise novamente"}
	}
	if !flow.analysis.CanConfirm {
		return workspaceFlowReceipt{}, &workspaceFlowBackendError{Code: "blocked", Status: 409, Message: "este plano tem bloqueios e não pode ser confirmado"}
	}
	if flow.receipt != nil {
		if flow.receipt.State == workspaceimport.PlanStateExecuted {
			return importFlowReceipt(flow.selection.FlowID, flow.plan, flow.approval, *flow.receipt), nil
		}
		return workspaceFlowReceipt{}, &workspaceFlowBackendError{Code: "replay_blocked", Status: 409, Message: "a confirmação já foi consumida ou revertida e não pode ser reaproveitada"}
	}
	approval, err := workspaceimport.Approve(flow.plan, backend.approvedBy(), action)
	if err != nil {
		return workspaceFlowReceipt{}, &workspaceFlowBackendError{Code: "approval_invalid", Status: 409, Message: err.Error()}
	}
	receipt, err := workspaceimport.Execute(backend.options.dataRoot, flow.plan, approval)
	if err != nil {
		return workspaceFlowReceipt{}, classifyWorkspaceImportError(err)
	}
	flow.approval = &approval
	flow.receipt = &receipt
	return importFlowReceipt(flow.selection.FlowID, flow.plan, flow.approval, receipt), nil
}

func (backend *realWorkspaceFlowBackend) Rollback(_ context.Context, selection workspaceFlowSelection, planDigest, receiptID, action string) (workspaceFlowReceipt, error) {
	if selection.Mode != workspaceFlowModeExternalImport {
		return workspaceFlowReceipt{}, &workspaceFlowCapabilityError{Capability: "workspace_migration_execution"}
	}
	if action != workspaceFlowApprovalRollback {
		return workspaceFlowReceipt{}, &workspaceFlowBackendError{Code: "invalid_rollback", Status: 409, Message: "a confirmação explícita ROLLBACK é obrigatória"}
	}
	backend.mu.Lock()
	defer backend.mu.Unlock()
	flow, ok := backend.imports[selection.FlowID]
	if !ok || flow.receipt == nil {
		return workspaceFlowReceipt{}, &workspaceFlowBackendError{Code: "rollback_unavailable", Status: 409, Message: "nenhum import executado está vinculado a esta sessão"}
	}
	if flow.plan.PlanDigest != planDigest || flow.receipt.RunID != receiptID {
		return workspaceFlowReceipt{}, &workspaceFlowBackendError{Code: "rollback_mismatch", Status: 409, Message: "o rollback não está vinculado ao receipt e plano atuais"}
	}
	receipt, err := workspaceimport.Rollback(backend.options.dataRoot, flow.plan, *flow.receipt, action)
	if err != nil {
		return workspaceFlowReceipt{}, classifyWorkspaceImportError(err)
	}
	flow.receipt = &receipt
	return importFlowReceipt(flow.selection.FlowID, flow.plan, flow.approval, receipt), nil
}

func (backend *realWorkspaceFlowBackend) approvedBy() string {
	digest := sha256.Sum256([]byte(backend.options.sessionToken))
	return "installer-session-owner:" + hex.EncodeToString(digest[:])[:16]
}

func (backend *realWorkspaceFlowBackend) analyzeExternalImport(selection workspaceFlowSelection) (workspaceFlowAnalysis, error) {
	base := workspaceFlowAnalysis{
		SchemaVersion: workspaceFlowSchemaVersion, FlowID: selection.FlowID, Mode: selection.Mode,
		Classification: "external_folder", Source: selection.Source, WorkspacePreserved: true,
		SourceEffect: workspaceFlowSourcePreserved, TargetEffect: workspaceFlowTargetImport,
		RollbackEffect: workspaceFlowRollbackImport, ConfirmationRequired: true,
		ApprovalAction: workspaceFlowApprovalImport,
	}
	inspection, err := workspaceimport.Inspect(selection.Source.Path, workspaceimport.DefaultLimits())
	if err != nil {
		return backend.storeBlocked(selection, backend.withBlocker(base, "source_inspection_failed", err.Error())), nil
	}
	base.Classification = workspaceImportClassification(selection.Source, inspection.Classification)
	base.Summary = inspection.ClassificationReason
	base.Mapped, base.Excluded, base.Ambiguous, base.CapabilitiesUnavailable = mapImportInspection(inspection)
	if inspection.State != "ready" {
		base = backend.withBlocker(base, "inspection_blocked", fmt.Sprintf("a inspeção da fonte está em estado %s", inspection.State))
	}
	target, err := defaultWorkspacePath(backend.options)
	if err != nil {
		return backend.storeBlocked(selection, backend.withBlocker(base, "target_unavailable", err.Error())), nil
	}
	base.WorkspacePath = target
	if info, statErr := os.Stat(target); statErr != nil || !info.IsDir() {
		return backend.storeBlocked(selection, backend.withBlocker(base, "target_unavailable", "o workspace alvo ainda não existe; crie um workspace antes do import")), nil
	}
	plan, err := workspaceimport.BuildPlan(selection.Source.Path, target, workspaceimport.DefaultLimits())
	if err != nil {
		return backend.storeBlocked(selection, backend.withBlocker(base, "plan_invalid", err.Error())), nil
	}
	base.PlanID, base.PlanDigest = plan.PlanID, plan.PlanDigest
	base.Mapped, base.Excluded, base.Ambiguous, base.CapabilitiesUnavailable = mapImportPlan(plan)
	for _, conflict := range plan.Conflicts {
		base.Blockers = append(base.Blockers, workspaceFlowBlocker{Code: "conflict", Message: fmt.Sprintf("%s: %s", conflict.Path, conflict.Reason)})
	}
	for _, entry := range plan.Entries {
		if entry.Action == workspaceimport.ActionQuarantine {
			base.Blockers = append(base.Blockers, workspaceFlowBlocker{Code: "capability_unavailable", Message: fmt.Sprintf("%s: %s", entry.SourcePath, entry.Reason)})
		}
	}
	if err := workspaceimport.ValidatePlan(plan); err != nil {
		base.Blockers = append(base.Blockers, workspaceFlowBlocker{Code: "plan_invalid", Message: err.Error()})
	}
	base.Summary = fmt.Sprintf("Fonte externa inspecionada e plano bounded criado para %s.", target)
	if len(base.Blockers) == 0 {
		base.State, base.CanConfirm = "plan_ready", true
	} else {
		base.State = "blocked"
	}
	backend.mu.Lock()
	backend.imports[selection.FlowID] = &realWorkspaceImportFlow{selection: selection, analysis: base, plan: plan}
	backend.mu.Unlock()
	return base, nil
}

func (backend *realWorkspaceFlowBackend) analyzeWorkspaceMigration(selection workspaceFlowSelection) (workspaceFlowAnalysis, error) {
	base := workspaceFlowAnalysis{
		SchemaVersion: workspaceFlowSchemaVersion, FlowID: selection.FlowID, Mode: selection.Mode,
		Classification: "maestro_workspace", Summary: "Workspace Maestro classificado pelo core de migration.", Source: selection.Source,
		WorkspacePreserved: true, MigrationRequired: true, SourceEffect: workspaceFlowSourcePreserved,
		TargetEffect: workspaceFlowTargetMigration, RollbackEffect: workspaceFlowRollbackMigration,
		ConfirmationRequired: true,
	}
	runtimeID := backend.options.primaryRuntime
	if runtimeID != "claude" && runtimeID != "codex" {
		runtimeID = "claude"
	}
	inspection, err := workspacemigration.Inspect(workspacemigration.PlanOptions{WorkspacePath: selection.Source.Path, DataRoot: backend.options.dataRoot, Runtime: runtimeID})
	if err != nil {
		return backend.withBlocker(base, "migration_inspection_failed", err.Error()), nil
	}
	base.WorkspacePath = inspection.WorkspacePath
	base.Summary = fmt.Sprintf("Workspace Maestro classificado como %s pelo core.", inspection.State)
	if inspection.Reason != "" {
		base.MigrationSummary = inspection.Reason
	}
	status := workspacemigration.CapabilityStatus()
	base.CapabilitiesUnavailable = []workspaceFlowCapability{{ID: status.Capability, State: status.Execution, Message: status.Reason}}
	base = backend.withBlocker(base, "capability_unavailable", status.Reason)
	return base, nil
}

func (backend *realWorkspaceFlowBackend) blockedAnalysis(selection workspaceFlowSelection, code, message string) workspaceFlowAnalysis {
	analysis := workspaceFlowAnalysis{
		SchemaVersion: workspaceFlowSchemaVersion, FlowID: selection.FlowID, Mode: selection.Mode,
		State: "blocked", Classification: "maestro_update", Summary: message, Source: selection.Source,
		WorkspacePreserved: true, SourceEffect: workspaceFlowSourcePreserved, TargetEffect: workspaceFlowTargetNone,
		RollbackEffect: workspaceFlowRollbackNotCreated, ConfirmationRequired: true,
	}
	return backend.withBlocker(analysis, code, message)
}

func (backend *realWorkspaceFlowBackend) withBlocker(analysis workspaceFlowAnalysis, code, message string) workspaceFlowAnalysis {
	analysis.Blockers = append(analysis.Blockers, workspaceFlowBlocker{Code: code, Message: message})
	analysis.State, analysis.CanConfirm = "blocked", false
	return analysis
}

func (backend *realWorkspaceFlowBackend) storeBlocked(selection workspaceFlowSelection, analysis workspaceFlowAnalysis) workspaceFlowAnalysis {
	backend.mu.Lock()
	backend.imports[selection.FlowID] = &realWorkspaceImportFlow{selection: selection, analysis: analysis}
	backend.mu.Unlock()
	return analysis
}

type workspaceFlowBackendError struct {
	Code    string
	Status  int
	Message string
}

func (err *workspaceFlowBackendError) Error() string { return err.Message }

func classifyWorkspaceImportError(err error) error {
	message := err.Error()
	lower := strings.ToLower(message)
	code := "import_execute_failed"
	if strings.Contains(lower, "metadata") || strings.Contains(lower, "changed") {
		code = "source_changed"
	} else if strings.Contains(lower, "plan") || strings.Contains(lower, "approval") || strings.Contains(lower, "conflict") {
		code = "plan_invalid"
	}
	return &workspaceFlowBackendError{Code: code, Status: 409, Message: message}
}

func workspaceImportClassification(source workspaceFlowSource, classification string) string {
	if classification == workspaceimport.ClassificationMaestroNative || source.Kind == workspaceFlowSourceMaestroWorkspace {
		return "maestro_workspace"
	}
	return "external_folder"
}

func mapImportInspection(inspection workspaceimport.Inspection) ([]workspaceFlowItem, []workspaceFlowItem, []workspaceFlowItem, []workspaceFlowCapability) {
	mapped, excluded, ambiguous := []workspaceFlowItem{}, []workspaceFlowItem{}, []workspaceFlowItem{}
	for _, entry := range inspection.Entries {
		if entry.Kind == "file" && !entry.Unsafe {
			mapped = append(mapped, workspaceFlowItem{Path: entry.RelativePath, Reason: "metadado elegível para o plano bounded"})
		} else if entry.Unsafe {
			excluded = append(excluded, workspaceFlowItem{Path: entry.RelativePath, Reason: "entrada insegura excluída pelo core"})
		}
	}
	capabilities := []workspaceFlowCapability{}
	if inspection.State == "bounded" {
		ambiguous = append(ambiguous, workspaceFlowItem{Path: "fonte", Reason: "a inspeção atingiu um limite bounded"})
	}
	return mapped, excluded, ambiguous, capabilities
}

func mapImportPlan(plan workspaceimport.Plan) ([]workspaceFlowItem, []workspaceFlowItem, []workspaceFlowItem, []workspaceFlowCapability) {
	mapped, excluded, ambiguous := []workspaceFlowItem{}, []workspaceFlowItem{}, []workspaceFlowItem{}
	capabilities := []workspaceFlowCapability{}
	for _, entry := range plan.Entries {
		switch entry.Action {
		case workspaceimport.ActionCopy:
			mapped = append(mapped, workspaceFlowItem{Path: entry.SourcePath, Reason: "será copiado bounded para o target após IMPORT"})
		case workspaceimport.ActionQuarantine:
			capabilities = append(capabilities, workspaceFlowCapability{ID: "workspace_import_entry", State: entry.Availability, Message: entry.SourcePath + ": " + entry.Reason})
		}
	}
	for _, item := range plan.Exclusions {
		excluded = append(excluded, workspaceFlowItem{Path: item.Path, Reason: item.Reason})
	}
	for _, item := range plan.Conflicts {
		ambiguous = append(ambiguous, workspaceFlowItem{Path: item.Path, Reason: item.Reason})
	}
	return mapped, excluded, ambiguous, capabilities
}

func importFlowReceipt(flowID string, plan workspaceimport.Plan, approval *workspaceimport.Approval, receipt workspaceimport.Receipt) workspaceFlowReceipt {
	status, ready, rollbackStatus := "failed", false, "unavailable"
	if receipt.State == workspaceimport.PlanStateExecuted {
		status, ready, rollbackStatus = "committed", true, "available"
	} else if receipt.State == workspaceimport.PlanStateRolledBack {
		status, rollbackStatus = "rolled_back", "completed"
	}
	result := workspaceFlowReceipt{
		SchemaVersion: workspaceFlowSchemaVersion, FlowID: flowID, ReceiptID: receipt.RunID,
		RunID: receipt.RunID, PlanID: plan.PlanID, PlanDigest: plan.PlanDigest,
		Operation: "external_import", Status: status, Valid: receipt.State == workspaceimport.PlanStateExecuted || receipt.State == workspaceimport.PlanStateRolledBack,
		Ready: ready, SourceEffect: workspaceFlowSourcePreserved, TargetEffect: map[bool]string{true: "bounded_import_committed", false: "bounded_import_rolled_back"}[receipt.State == workspaceimport.PlanStateExecuted],
		RollbackEffect: map[bool]string{true: workspaceFlowRollbackImport, false: "completed"}[receipt.State == workspaceimport.PlanStateExecuted],
		Stages:         []workspaceFlowStage{{ID: "staging", Status: "completed", Detail: "core workspaceimport staging concluído"}, {ID: "validation", Status: "completed", Detail: "receipt do core workspaceimport validado"}, {ID: "rollback", Status: rollbackStatus, Detail: "rollback vinculado ao receipt do core"}},
	}
	if approval != nil {
		result.ApprovalAction, result.ApprovedBy, result.ApprovalPlanID = approval.Confirmation, approval.ApprovedBy, approval.PlanID
	}
	if receipt.State == workspaceimport.PlanStateRolledBack {
		result.ApprovalAction = workspaceFlowApprovalRollback
	}
	return result
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
	if analysis.PlanID == "" {
		analysis.PlanID = "fixture-" + analysis.PlanDigest
	}
	if analysis.SourceEffect == "" {
		analysis.SourceEffect = workspaceFlowSourcePreserved
	}
	if analysis.TargetEffect == "" {
		analysis.TargetEffect = workspaceFlowTargetImport
	}
	if analysis.RollbackEffect == "" {
		analysis.RollbackEffect = workspaceFlowRollbackImport
	}
	if analysis.ApprovalAction == "" {
		analysis.ApprovalAction = workspaceFlowApprovalImport
	}
	return analysis, nil
}

func (backend fixtureWorkspaceFlowBackend) Confirm(_ context.Context, selection workspaceFlowSelection, planDigest, action string) (workspaceFlowReceipt, error) {
	if action != workspaceFlowApprovalImport {
		return workspaceFlowReceipt{}, &workspaceFlowBackendError{Code: "invalid_approval", Status: 409, Message: "a confirmação explícita IMPORT é obrigatória"}
	}
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
	if receipt.PlanID == "" {
		receipt.PlanID = "fixture-" + planDigest
	}
	if receipt.SourceEffect == "" {
		receipt.SourceEffect = workspaceFlowSourcePreserved
	}
	if receipt.TargetEffect == "" {
		receipt.TargetEffect = "bounded_import_committed"
	}
	if receipt.RollbackEffect == "" {
		receipt.RollbackEffect = workspaceFlowRollbackImport
	}
	receipt.ApprovalAction = action
	receipt.ApprovedBy = "simulation-owner"
	receipt.ApprovalPlanID = planDigest
	return receipt, nil
}

func (backend fixtureWorkspaceFlowBackend) Rollback(_ context.Context, selection workspaceFlowSelection, planDigest, receiptID, action string) (workspaceFlowReceipt, error) {
	if action != workspaceFlowApprovalRollback {
		return workspaceFlowReceipt{}, &workspaceFlowBackendError{Code: "invalid_rollback", Status: 409, Message: "a confirmação explícita ROLLBACK é obrigatória"}
	}
	return workspaceFlowReceipt{
		SchemaVersion: workspaceFlowSchemaVersion, FlowID: selection.FlowID, ReceiptID: receiptID, RunID: receiptID,
		PlanDigest: planDigest, Operation: workspaceFlowOperationForMode(selection.Mode), Status: "rolled_back", Valid: true,
		Ready: false, SourceEffect: workspaceFlowSourcePreserved, TargetEffect: "bounded_import_rolled_back", RollbackEffect: "completed",
		ApprovalAction: action, ApprovalPlanID: planDigest,
		Stages: []workspaceFlowStage{{ID: "staging", Status: "completed", Detail: "fixture staging concluído"}, {ID: "validation", Status: "completed", Detail: "fixture receipt validado"}, {ID: "rollback", Status: "completed", Detail: "fixture rollback concluído"}},
	}, nil
}

func workspaceFlowBackendFor(options options) workspaceFlowBackend {
	if options.workspaceFlow != nil {
		return options.workspaceFlow
	}
	if options.simulate {
		return fixtureWorkspaceFlowBackend{}
	}
	return newRealWorkspaceFlowBackend(options)
}

func workspaceFlowBackendName(options options) string {
	if options.workspaceFlow != nil {
		return "injected"
	}
	if options.simulate {
		return "fixture"
	}
	return "workspace-core"
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
	if analysis.Source.Kind != selection.Source.Kind || analysis.Source.PathChosen != selection.Source.PathChosen {
		return errors.New("análise do workspace não está vinculada à fonte selecionada")
	}
	if analysis.SourceEffect != workspaceFlowSourcePreserved {
		return errors.New("análise do workspace reportou efeito inválido na origem")
	}
	if analysis.State != "blocked" && len(analysis.CapabilitiesUnavailable) > 0 {
		return errors.New("análise do workspace tem capability unavailable e não pode ser confirmável")
	}
	if analysis.State == "blocked" {
		if analysis.CanConfirm || len(analysis.Blockers) == 0 {
			return errors.New("análise bloqueada precisa impedir confirmação e explicar o bloqueio")
		}
		return nil
	}
	if analysis.State != "plan_ready" || strings.TrimSpace(analysis.PlanID) == "" || strings.TrimSpace(analysis.PlanDigest) == "" || !analysis.ConfirmationRequired || !analysis.CanConfirm || analysis.ApprovalAction != workspaceFlowApprovalImport {
		return errors.New("análise do workspace não produziu um plano confirmável")
	}
	if analysis.TargetEffect == "" || analysis.RollbackEffect == "" {
		return errors.New("análise do workspace não explica os efeitos no alvo e no rollback")
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
	if !receipt.Valid || !receipt.Ready || receipt.Status != "committed" || strings.TrimSpace(receipt.ReceiptID) == "" || strings.TrimSpace(receipt.PlanID) == "" {
		return errors.New("receipt do workspace é inválido; a jornada não pode mostrar pronto")
	}
	if receipt.SourceEffect != workspaceFlowSourcePreserved || receipt.TargetEffect == "" || receipt.RollbackEffect == "" || receipt.ApprovalAction != workspaceFlowApprovalImport || receipt.ApprovalPlanID != receipt.PlanID || receipt.ApprovedBy == "" {
		return errors.New("receipt do workspace não confirma que a origem permaneceu intacta")
	}
	if selection.Mode == workspaceFlowModeExternalImport && (receipt.TargetEffect != "bounded_import_committed" || receipt.RollbackEffect != workspaceFlowRollbackImport) {
		return errors.New("receipt do import tem efeitos fora do vocabulário permitido")
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

func validateWorkspaceFlowRollbackReceipt(receipt workspaceFlowReceipt, selection workspaceFlowSelection, planDigest, receiptID string) error {
	if receipt.SchemaVersion != workspaceFlowSchemaVersion || receipt.FlowID != selection.FlowID || receipt.PlanDigest != planDigest || receipt.ReceiptID != receiptID || receipt.Operation != workspaceFlowOperationForMode(selection.Mode) {
		return errors.New("receipt de rollback não está vinculado à seleção, ao plano e ao receipt originais")
	}
	if !receipt.Valid || receipt.Ready || receipt.Status != "rolled_back" || receipt.SourceEffect != workspaceFlowSourcePreserved || receipt.RollbackEffect != "completed" || receipt.ApprovalAction != workspaceFlowApprovalRollback || (selection.Mode == workspaceFlowModeExternalImport && receipt.TargetEffect != "bounded_import_rolled_back") {
		return errors.New("receipt de rollback inválido; a jornada não pode declarar rollback concluído")
	}
	return nil
}
