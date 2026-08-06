package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestWorkspaceFlowFailsClosedWhenSourceCannotBeInspected(t *testing.T) {
	handler := wizardHandler(options{
		sessionToken: "test-token",
		chooseWorkspaceSource: func(workspaceFlowMode) (string, error) {
			return "/Users/pilot/External-notes", nil
		},
	})
	selection := workspaceFlowRequest(t, "/api/workspace-flow/select", `{"mode":"external_import"}`)
	selection.Header.Set("X-Maestro-Session", "test-token")
	selectionRecorder := httptest.NewRecorder()
	handler.ServeHTTP(selectionRecorder, selection)
	if selectionRecorder.Code != http.StatusOK {
		t.Fatalf("select status = %d, body = %s", selectionRecorder.Code, selectionRecorder.Body.String())
	}
	var selected workspaceFlowSelectionResponse
	if err := json.Unmarshal(selectionRecorder.Body.Bytes(), &selected); err != nil {
		t.Fatal(err)
	}
	if selected.FlowID == "" || selected.Source.Kind != workspaceFlowSourceExternalFolder {
		t.Fatalf("selection = %#v", selected)
	}

	analyze := workspaceFlowRequest(t, "/api/workspace-flow/analyze", `{"flow_id":"`+selected.FlowID+`"}`)
	analyze.Header.Set("X-Maestro-Session", "test-token")
	analyzeRecorder := httptest.NewRecorder()
	handler.ServeHTTP(analyzeRecorder, analyze)
	if analyzeRecorder.Code != http.StatusConflict {
		t.Fatalf("analyze status = %d, body = %s", analyzeRecorder.Code, analyzeRecorder.Body.String())
	}
	if !strings.Contains(analyzeRecorder.Body.String(), `"code":"source_inspection_failed"`) || !strings.Contains(analyzeRecorder.Body.String(), `"can_confirm":false`) {
		t.Fatalf("fail-closed body = %s", analyzeRecorder.Body.String())
	}
}

func TestWorkspaceFlowFixtureShowsClassificationsAndRequiresValidReceipt(t *testing.T) {
	handler := wizardHandler(options{
		simulate:     true,
		sessionToken: "test-token",
		chooseWorkspaceSource: func(workspaceFlowMode) (string, error) {
			return "/Users/pilot/External-notes", nil
		},
	})

	selected := postWorkspaceFlow(t, handler, "/api/workspace-flow/select", `{"mode":"external_import"}`)
	if selected.Code != http.StatusOK {
		t.Fatalf("select status = %d, body = %s", selected.Code, selected.Body.String())
	}
	var selection workspaceFlowSelectionResponse
	decodeWorkspaceFlow(t, selected, &selection)

	analyzed := postWorkspaceFlow(t, handler, "/api/workspace-flow/analyze", `{"flow_id":"`+selection.FlowID+`"}`)
	if analyzed.Code != http.StatusOK {
		t.Fatalf("analyze status = %d, body = %s", analyzed.Code, analyzed.Body.String())
	}
	var analysis workspaceFlowAnalysis
	decodeWorkspaceFlow(t, analyzed, &analysis)
	if analysis.Classification != "external_folder" || analysis.PlanDigest == "" || analysis.State != "plan_ready" {
		t.Fatalf("analysis = %#v", analysis)
	}
	if len(analysis.Mapped) == 0 || len(analysis.Excluded) == 0 || len(analysis.Ambiguous) == 0 || len(analysis.CapabilitiesUnavailable) != 0 || !analysis.CanConfirm {
		t.Fatalf("analysis classifications are incomplete = %#v", analysis)
	}
	if analysis.SourceEffect != workspaceFlowSourcePreserved || !analysis.CanConfirm || analysis.ApprovalAction != workspaceFlowApprovalImport {
		t.Fatalf("source/effect contract = %#v", analysis)
	}

	wrong := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selection.FlowID+`","plan_digest":"wrong","action":"IMPORT"}`)
	if wrong.Code != http.StatusConflict {
		t.Fatalf("wrong digest status = %d, body = %s", wrong.Code, wrong.Body.String())
	}

	confirmed := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selection.FlowID+`","plan_digest":"`+analysis.PlanDigest+`","action":"IMPORT"}`)
	if confirmed.Code != http.StatusOK {
		t.Fatalf("confirm status = %d, body = %s", confirmed.Code, confirmed.Body.String())
	}
	var receipt workspaceFlowReceipt
	decodeWorkspaceFlow(t, confirmed, &receipt)
	if !receipt.Valid || receipt.Status != "committed" || receipt.ReceiptID == "" {
		t.Fatalf("receipt = %#v", receipt)
	}
	for _, stage := range []string{"staging", "validation", "rollback"} {
		if !workspaceFlowStageHasStatus(receipt.Stages, stage) {
			t.Fatalf("receipt does not explain %s stage: %#v", stage, receipt.Stages)
		}
	}
}

func TestWorkspaceFlowRealExternalImportExecuteRollbackAndReplayGuard(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "external")
	target := filepath.Join(root, "maestro")
	dataRoot := filepath.Join(root, "data")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "note.md"), []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := wizardHandler(options{
		sessionToken: "test-token", dataRoot: dataRoot,
		workspacePath:         func() (string, error) { return target, nil },
		chooseWorkspaceSource: func(workspaceFlowMode) (string, error) { return source, nil },
	})
	selected := postWorkspaceFlow(t, handler, "/api/workspace-flow/select", `{"mode":"external_import"}`)
	var selection workspaceFlowSelectionResponse
	decodeWorkspaceFlow(t, selected, &selection)
	analyzed := postWorkspaceFlow(t, handler, "/api/workspace-flow/analyze", `{"flow_id":"`+selection.FlowID+`"}`)
	if analyzed.Code != http.StatusOK {
		t.Fatalf("real analyze status = %d, body = %s", analyzed.Code, analyzed.Body.String())
	}
	var analysis workspaceFlowAnalysis
	decodeWorkspaceFlow(t, analyzed, &analysis)
	if analysis.State != "plan_ready" || analysis.PlanID == "" || analysis.PlanDigest == "" || !analysis.CanConfirm || analysis.ApprovalAction != workspaceFlowApprovalImport {
		t.Fatalf("real analysis = %#v", analysis)
	}
	if analysis.SourceEffect != workspaceFlowSourcePreserved || analysis.TargetEffect != workspaceFlowTargetImport || analysis.RollbackEffect != workspaceFlowRollbackImport {
		t.Fatalf("real effect contract = %#v", analysis)
	}
	confirmed := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selection.FlowID+`","plan_digest":"`+analysis.PlanDigest+`","action":"IMPORT"}`)
	if confirmed.Code != http.StatusOK {
		t.Fatalf("real confirm status = %d, body = %s", confirmed.Code, confirmed.Body.String())
	}
	var receipt workspaceFlowReceipt
	decodeWorkspaceFlow(t, confirmed, &receipt)
	if err := validateWorkspaceFlowReceipt(receipt, selection.workspaceFlowSelection, analysis.PlanDigest); err != nil {
		t.Fatal(err)
	}
	if receipt.ApprovalPlanID != analysis.PlanID || receipt.ApprovedBy == "" || receipt.ApprovalAction != workspaceFlowApprovalImport {
		t.Fatalf("approval binding = %#v", receipt)
	}
	if body, err := os.ReadFile(filepath.Join(target, "note.md")); err != nil || string(body) != "original\n" {
		t.Fatalf("imported target = %q (%v)", body, err)
	}
	rollback := postWorkspaceFlow(t, handler, "/api/workspace-flow/rollback", `{"flow_id":"`+selection.FlowID+`","plan_digest":"`+analysis.PlanDigest+`","receipt_id":"`+receipt.ReceiptID+`","action":"ROLLBACK"}`)
	if rollback.Code != http.StatusOK {
		t.Fatalf("rollback status = %d, body = %s", rollback.Code, rollback.Body.String())
	}
	var rolledBack workspaceFlowReceipt
	decodeWorkspaceFlow(t, rollback, &rolledBack)
	if err := validateWorkspaceFlowRollbackReceipt(rolledBack, selection.workspaceFlowSelection, analysis.PlanDigest, receipt.ReceiptID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "note.md")); !os.IsNotExist(err) {
		t.Fatalf("rollback left target file behind: %v", err)
	}
	replay := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selection.FlowID+`","plan_digest":"`+analysis.PlanDigest+`","action":"IMPORT"}`)
	if replay.Code != http.StatusConflict || !strings.Contains(replay.Body.String(), "reaproveitada") {
		t.Fatalf("replay response = %d %s", replay.Code, replay.Body.String())
	}
}

func TestWorkspaceFlowRealImportRejectsSourceChangeAndTamperedApproval(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "external")
	target := filepath.Join(root, "maestro")
	dataRoot := filepath.Join(root, "data")
	for _, path := range []string{source, target, dataRoot} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(source, "note.md")
	if err := os.WriteFile(path, []byte("before\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := wizardHandler(options{sessionToken: "test-token", dataRoot: dataRoot, workspacePath: func() (string, error) { return target, nil }, chooseWorkspaceSource: func(workspaceFlowMode) (string, error) { return source, nil }})
	selected := postWorkspaceFlow(t, handler, "/api/workspace-flow/select", `{"mode":"external_import"}`)
	var selection workspaceFlowSelectionResponse
	decodeWorkspaceFlow(t, selected, &selection)
	analyzed := postWorkspaceFlow(t, handler, "/api/workspace-flow/analyze", `{"flow_id":"`+selection.FlowID+`"}`)
	var analysis workspaceFlowAnalysis
	decodeWorkspaceFlow(t, analyzed, &analysis)
	tampered := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selection.FlowID+`","plan_digest":"`+analysis.PlanDigest+`","action":"ROLLBACK"}`)
	if tampered.Code != http.StatusBadRequest || !strings.Contains(tampered.Body.String(), "action=IMPORT") {
		t.Fatalf("tampered approval = %d %s", tampered.Code, tampered.Body.String())
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	changed := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selection.FlowID+`","plan_digest":"`+analysis.PlanDigest+`","action":"IMPORT"}`)
	if changed.Code != http.StatusConflict || !strings.Contains(changed.Body.String(), `"code":"source_changed"`) {
		t.Fatalf("source change response = %d %s", changed.Code, changed.Body.String())
	}
	if _, err := os.Stat(filepath.Join(target, "note.md")); !os.IsNotExist(err) {
		t.Fatalf("source change wrote target: %v", err)
	}
}

func TestWorkspaceFlowRealConflictBlocksConfirmation(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "external")
	target := filepath.Join(root, "maestro")
	dataRoot := filepath.Join(root, "data")
	for _, path := range []string{source, target, dataRoot} {
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(source, "note.md"), []byte("source\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "note.md"), []byte("target\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	handler := wizardHandler(options{sessionToken: "test-token", dataRoot: dataRoot, workspacePath: func() (string, error) { return target, nil }, chooseWorkspaceSource: func(workspaceFlowMode) (string, error) { return source, nil }})
	selected := postWorkspaceFlow(t, handler, "/api/workspace-flow/select", `{"mode":"external_import"}`)
	var selection workspaceFlowSelectionResponse
	decodeWorkspaceFlow(t, selected, &selection)
	analyzed := postWorkspaceFlow(t, handler, "/api/workspace-flow/analyze", `{"flow_id":"`+selection.FlowID+`"}`)
	if analyzed.Code != http.StatusConflict {
		t.Fatalf("conflict analyze status = %d, body = %s", analyzed.Code, analyzed.Body.String())
	}
	var analysis workspaceFlowAnalysis
	decodeWorkspaceFlow(t, analyzed, &analysis)
	if analysis.State != "blocked" || analysis.CanConfirm || len(analysis.Blockers) == 0 || !strings.Contains(analyzed.Body.String(), `"code":"conflict"`) {
		t.Fatalf("conflict analysis = %#v", analysis)
	}
	confirm := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selection.FlowID+`","plan_digest":"blocked","action":"IMPORT"}`)
	if confirm.Code != http.StatusConflict || !strings.Contains(confirm.Body.String(), "não pode ser confirmado") {
		t.Fatalf("conflict confirm response = %d %s", confirm.Code, confirm.Body.String())
	}
}

func TestWorkspaceFlowRealMigrationIsClassifiedButUnavailable(t *testing.T) {
	root := t.TempDir()
	workspacePath := filepath.Join(root, "workspace")
	dataRoot := filepath.Join(root, "data")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: dataRoot}); err != nil {
		t.Fatal(err)
	}
	handler := wizardHandler(options{sessionToken: "test-token", dataRoot: dataRoot, primaryRuntime: "claude", chooseWorkspaceSource: func(workspaceFlowMode) (string, error) { return workspacePath, nil }})
	selected := postWorkspaceFlow(t, handler, "/api/workspace-flow/select", `{"mode":"workspace_migration"}`)
	var selection workspaceFlowSelectionResponse
	decodeWorkspaceFlow(t, selected, &selection)
	analyzed := postWorkspaceFlow(t, handler, "/api/workspace-flow/analyze", `{"flow_id":"`+selection.FlowID+`"}`)
	if analyzed.Code != http.StatusServiceUnavailable {
		t.Fatalf("migration analyze status = %d, body = %s", analyzed.Code, analyzed.Body.String())
	}
	var analysis workspaceFlowAnalysis
	decodeWorkspaceFlow(t, analyzed, &analysis)
	if analysis.Classification != "maestro_workspace" || analysis.State != "blocked" || analysis.CanConfirm || len(analysis.Blockers) == 0 || len(analysis.CapabilitiesUnavailable) != 1 {
		t.Fatalf("migration unavailable analysis = %#v", analysis)
	}
	confirm := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selection.FlowID+`","plan_digest":"blocked","action":"IMPORT"}`)
	if confirm.Code != http.StatusConflict || !strings.Contains(confirm.Body.String(), "não pode ser confirmado") {
		t.Fatalf("migration confirm response = %d %s", confirm.Code, confirm.Body.String())
	}
}

func TestWorkspaceFlowConfirmationDecodersRejectTrailingJSON(t *testing.T) {
	confirmation := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"flow_id":"flow","plan_digest":"digest","action":"IMPORT"}{}`))
	if _, _, _, err := decodeWorkspaceFlowConfirmation(httptest.NewRecorder(), confirmation); err == nil {
		t.Fatal("confirmation decoder accepted trailing JSON")
	}
	rollback := httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(`{"flow_id":"flow","plan_digest":"digest","receipt_id":"receipt","action":"ROLLBACK"}{}`))
	if _, _, _, _, err := decodeWorkspaceFlowRollback(httptest.NewRecorder(), rollback); err == nil {
		t.Fatal("rollback decoder accepted trailing JSON")
	}
}

func TestWorkspaceFlowRejectsForgedExternalEffects(t *testing.T) {
	selection := workspaceFlowSelection{SchemaVersion: workspaceFlowSchemaVersion, FlowID: "flow", Mode: workspaceFlowModeExternalImport, Source: workspaceFlowSource{Kind: workspaceFlowSourceExternalFolder, PathChosen: true}}
	receipt := workspaceFlowReceipt{
		SchemaVersion: workspaceFlowSchemaVersion, FlowID: selection.FlowID, ReceiptID: "receipt", PlanID: "plan", PlanDigest: "digest", Operation: "external_import",
		Status: "committed", Valid: true, Ready: true, SourceEffect: workspaceFlowSourcePreserved, TargetEffect: "forged_target", RollbackEffect: workspaceFlowRollbackImport,
		ApprovalAction: workspaceFlowApprovalImport, ApprovedBy: "owner", ApprovalPlanID: "plan",
		Stages: []workspaceFlowStage{{ID: "staging", Status: "completed", Detail: "staged"}, {ID: "validation", Status: "completed", Detail: "validated"}, {ID: "rollback", Status: "available", Detail: "available"}},
	}
	if err := validateWorkspaceFlowReceipt(receipt, selection, "digest"); err == nil {
		t.Fatal("receipt validator accepted forged external target effect")
	}
	rolledBack := receipt
	rolledBack.Status, rolledBack.Ready, rolledBack.ApprovalAction = "rolled_back", false, workspaceFlowApprovalRollback
	rolledBack.TargetEffect, rolledBack.RollbackEffect = "forged_target", "completed"
	if err := validateWorkspaceFlowRollbackReceipt(rolledBack, selection, "digest", "receipt"); err == nil {
		t.Fatal("rollback validator accepted forged external target effect")
	}
}

func TestWorkspaceFlowUpdatePlanPreservesWorkspaceAndNamesMigration(t *testing.T) {
	handler := wizardHandler(options{simulate: true, sessionToken: "test-token"})
	selected := postWorkspaceFlow(t, handler, "/api/workspace-flow/select", `{"mode":"update"}`)
	var selection workspaceFlowSelectionResponse
	decodeWorkspaceFlow(t, selected, &selection)
	analyzed := postWorkspaceFlow(t, handler, "/api/workspace-flow/analyze", `{"flow_id":"`+selection.FlowID+`"}`)
	var analysis workspaceFlowAnalysis
	decodeWorkspaceFlow(t, analyzed, &analysis)
	if analysis.Classification != "maestro_update" || !analysis.WorkspacePreserved || !analysis.MigrationRequired {
		t.Fatalf("update analysis = %#v", analysis)
	}
	if analysis.InstalledVersion == "" || analysis.TargetVersion == "" || analysis.MigrationSummary == "" {
		t.Fatalf("update version/migration summary = %#v", analysis)
	}
}

func TestWorkspaceFlowRejectsInvalidReceiptBeforeReady(t *testing.T) {
	backend := stubWorkspaceFlowBackend{
		analysis: workspaceFlowAnalysis{SchemaVersion: 1, State: "plan_ready", FlowID: "flow-1", PlanDigest: "digest-1"},
		receipt: workspaceFlowReceipt{
			SchemaVersion: 1, FlowID: "flow-1", PlanID: "plan-1", Operation: "maestro_update", SourceEffect: workspaceFlowSourcePreserved, TargetEffect: "managed_in_place", RollbackEffect: "available",
			ApprovalAction: workspaceFlowApprovalImport, ApprovedBy: "test-owner", ApprovalPlanID: "plan-1",
			Status: "committed", Valid: true, Ready: true,
			Stages: []workspaceFlowStage{
				{ID: "staging", Status: "completed", Detail: "staged"},
				{ID: "validation", Status: "failed", Detail: "validation failed"},
				{ID: "rollback", Status: "available", Detail: "rollback available"},
			},
		},
	}
	handler := wizardHandler(options{sessionToken: "test-token", workspaceFlow: backend})
	selection := postWorkspaceFlow(t, handler, "/api/workspace-flow/select", `{"mode":"update"}`)
	var selected workspaceFlowSelectionResponse
	decodeWorkspaceFlow(t, selection, &selected)
	_ = postWorkspaceFlow(t, handler, "/api/workspace-flow/analyze", `{"flow_id":"`+selected.FlowID+`"}`)
	confirmed := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selected.FlowID+`","plan_digest":"digest-1","action":"IMPORT"}`)
	if confirmed.Code != http.StatusConflict {
		t.Fatalf("invalid receipt status = %d, body = %s", confirmed.Code, confirmed.Body.String())
	}
	if !strings.Contains(confirmed.Body.String(), "receipt") || strings.Contains(confirmed.Body.String(), `"ready":true`) {
		t.Fatalf("invalid receipt response = %s", confirmed.Body.String())
	}
}

func TestWorkspaceFlowRejectsAnalysisThatClaimsMutationBeforeConfirmation(t *testing.T) {
	handler := wizardHandler(options{
		sessionToken: "test-token",
		chooseWorkspaceSource: func(workspaceFlowMode) (string, error) {
			return "/Users/pilot/External-notes", nil
		},
		workspaceFlow: stubWorkspaceFlowBackend{analysis: workspaceFlowAnalysis{PlanDigest: "unsafe", SourceEffect: "changed"}},
	})
	selected := postWorkspaceFlow(t, handler, "/api/workspace-flow/select", `{"mode":"external_import"}`)
	var selection workspaceFlowSelectionResponse
	decodeWorkspaceFlow(t, selected, &selection)
	analyzed := postWorkspaceFlow(t, handler, "/api/workspace-flow/analyze", `{"flow_id":"`+selection.FlowID+`"}`)
	if analyzed.Code != http.StatusConflict || !strings.Contains(analyzed.Body.String(), "efeito") {
		t.Fatalf("unsafe analysis response = %d %s", analyzed.Code, analyzed.Body.String())
	}
}

func TestWorkspaceFlowRejectsConfirmableAnalysisWithUnavailableCapability(t *testing.T) {
	backend := &forgedCapabilityWorkspaceFlowBackend{}
	handler := wizardHandler(options{
		sessionToken: "test-token",
		chooseWorkspaceSource: func(workspaceFlowMode) (string, error) {
			return "/Users/pilot/External-notes", nil
		},
		workspaceFlow: backend,
	})
	selected := postWorkspaceFlow(t, handler, "/api/workspace-flow/select", `{"mode":"external_import"}`)
	var selection workspaceFlowSelectionResponse
	decodeWorkspaceFlow(t, selected, &selection)
	analyzed := postWorkspaceFlow(t, handler, "/api/workspace-flow/analyze", `{"flow_id":"`+selection.FlowID+`"}`)
	if analyzed.Code != http.StatusConflict || !strings.Contains(analyzed.Body.String(), "capability unavailable") {
		t.Fatalf("forged capability analysis = %d %s", analyzed.Code, analyzed.Body.String())
	}
	confirmed := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selection.FlowID+`","plan_digest":"forged-plan","action":"IMPORT"}`)
	if confirmed.Code != http.StatusConflict || backend.confirmCalls != 0 || !strings.Contains(confirmed.Body.String(), "analise a fonte") {
		t.Fatalf("forged capability confirmation = %d calls=%d body=%s", confirmed.Code, backend.confirmCalls, confirmed.Body.String())
	}
}

func TestWorkspaceFlowRejectsReceiptForDifferentOperation(t *testing.T) {
	handler := wizardHandler(options{
		sessionToken: "test-token",
		workspaceFlow: stubWorkspaceFlowBackend{
			analysis: workspaceFlowAnalysis{PlanDigest: "op-plan"},
			receipt: workspaceFlowReceipt{
				Operation: workspaceFlowOperationForMode(workspaceFlowModeExternalImport),
				Valid:     true,
				Ready:     true,
				Status:    "committed",
				Stages: []workspaceFlowStage{
					{ID: "staging", Status: "completed", Detail: "staged"},
					{ID: "validation", Status: "completed", Detail: "validated"},
					{ID: "rollback", Status: "available", Detail: "rollback available"},
				},
			},
		},
	})
	selected := postWorkspaceFlow(t, handler, "/api/workspace-flow/select", `{"mode":"update"}`)
	var selection workspaceFlowSelectionResponse
	decodeWorkspaceFlow(t, selected, &selection)
	analyzed := postWorkspaceFlow(t, handler, "/api/workspace-flow/analyze", `{"flow_id":"`+selection.FlowID+`"}`)
	var analysis workspaceFlowAnalysis
	decodeWorkspaceFlow(t, analyzed, &analysis)
	confirmed := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selection.FlowID+`","plan_digest":"`+analysis.PlanDigest+`","action":"IMPORT"}`)
	if confirmed.Code != http.StatusConflict || !strings.Contains(confirmed.Body.String(), "operação") {
		t.Fatalf("wrong operation response = %d %s", confirmed.Code, confirmed.Body.String())
	}
}

func TestImportIntentIsPointerOnlyAndNeverClaimsIngestion(t *testing.T) {
	workspacePath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspacePath, ".bcgos"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeImportIntent(workspacePath, "/Users/pilot/External-notes"); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(workspacePath, ".bcgos", "import-intake.json"))
	if err != nil {
		t.Fatal(err)
	}
	var intent struct {
		State  string `json:"state"`
		Notice string `json:"notice"`
	}
	if err := json.Unmarshal(body, &intent); err != nil {
		t.Fatal(err)
	}
	if intent.State != "pointer_recorded_pending_analysis" || !strings.Contains(intent.Notice, "not ingestion") {
		t.Fatalf("intent = %#v", intent)
	}
}

type stubWorkspaceFlowBackend struct {
	analysis workspaceFlowAnalysis
	receipt  workspaceFlowReceipt
}

type forgedCapabilityWorkspaceFlowBackend struct {
	confirmCalls int
}

func (backend *forgedCapabilityWorkspaceFlowBackend) Analyze(_ context.Context, selection workspaceFlowSelection) (workspaceFlowAnalysis, error) {
	return workspaceFlowAnalysis{
		SchemaVersion: workspaceFlowSchemaVersion, FlowID: selection.FlowID, Mode: selection.Mode, Source: selection.Source,
		State: "plan_ready", Classification: "external_folder", SourceEffect: workspaceFlowSourcePreserved,
		TargetEffect: workspaceFlowTargetImport, RollbackEffect: workspaceFlowRollbackImport, PlanID: "forged-plan-id", PlanDigest: "forged-plan",
		ConfirmationRequired: true, ApprovalAction: workspaceFlowApprovalImport, CanConfirm: true,
		CapabilitiesUnavailable: []workspaceFlowCapability{{ID: "docling", State: "unavailable", Message: "conversion runtime unavailable"}},
	}, nil
}

func (backend *forgedCapabilityWorkspaceFlowBackend) Confirm(_ context.Context, _ workspaceFlowSelection, _, _ string) (workspaceFlowReceipt, error) {
	backend.confirmCalls++
	return workspaceFlowReceipt{}, nil
}

func (backend *forgedCapabilityWorkspaceFlowBackend) Rollback(context.Context, workspaceFlowSelection, string, string, string) (workspaceFlowReceipt, error) {
	return workspaceFlowReceipt{}, nil
}

func (stub stubWorkspaceFlowBackend) Analyze(_ context.Context, selection workspaceFlowSelection) (workspaceFlowAnalysis, error) {
	analysis := stub.analysis
	analysis.SchemaVersion = workspaceFlowSchemaVersion
	analysis.FlowID = selection.FlowID
	analysis.Mode = selection.Mode
	analysis.Source = selection.Source
	if analysis.SourceEffect == "" {
		analysis.SourceEffect = workspaceFlowSourcePreserved
	}
	if analysis.TargetEffect == "" {
		analysis.TargetEffect = workspaceFlowTargetImport
	}
	if analysis.RollbackEffect == "" {
		analysis.RollbackEffect = workspaceFlowRollbackImport
	}
	if analysis.PlanID == "" {
		analysis.PlanID = "stub-" + analysis.PlanDigest
	}
	analysis.ApprovalAction = workspaceFlowApprovalImport
	analysis.CanConfirm = true
	analysis.State = "plan_ready"
	analysis.ConfirmationRequired = true
	return analysis, nil
}

func (stub stubWorkspaceFlowBackend) Confirm(_ context.Context, selection workspaceFlowSelection, planDigest, action string) (workspaceFlowReceipt, error) {
	receipt := stub.receipt
	receipt.SchemaVersion = workspaceFlowSchemaVersion
	receipt.FlowID = selection.FlowID
	receipt.PlanDigest = planDigest
	if receipt.Operation == "" {
		receipt.Operation = workspaceFlowOperationForMode(selection.Mode)
	}
	if receipt.PlanID == "" {
		receipt.PlanID = "stub-" + planDigest
	}
	if receipt.SourceEffect == "" {
		receipt.SourceEffect = workspaceFlowSourcePreserved
	}
	if receipt.TargetEffect == "" {
		receipt.TargetEffect = workspaceFlowTargetImport
	}
	if receipt.RollbackEffect == "" {
		receipt.RollbackEffect = workspaceFlowRollbackImport
	}
	if receipt.ApprovalAction == "" {
		receipt.ApprovalAction = action
	}
	if receipt.ApprovedBy == "" {
		receipt.ApprovedBy = "stub-owner"
	}
	if receipt.ApprovalPlanID == "" {
		receipt.ApprovalPlanID = receipt.PlanID
	}
	return receipt, nil
}

func (stub stubWorkspaceFlowBackend) Rollback(_ context.Context, selection workspaceFlowSelection, planDigest, receiptID, action string) (workspaceFlowReceipt, error) {
	return workspaceFlowReceipt{SchemaVersion: workspaceFlowSchemaVersion, FlowID: selection.FlowID, ReceiptID: receiptID, PlanID: "stub-" + planDigest, PlanDigest: planDigest, Operation: workspaceFlowOperationForMode(selection.Mode), Status: "rolled_back", Valid: true, SourceEffect: workspaceFlowSourcePreserved, TargetEffect: "bounded_import_rolled_back", RollbackEffect: "completed", ApprovalAction: action}, nil
}

func workspaceFlowRequest(t *testing.T, path, body string) *http.Request {
	t.Helper()
	return httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
}

func postWorkspaceFlow(t *testing.T, handler http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := workspaceFlowRequest(t, path, body)
	request.Header.Set("X-Maestro-Session", "test-token")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	return recorder
}

func decodeWorkspaceFlow(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode response %d: %v; body=%s", recorder.Code, err, recorder.Body.String())
	}
}

func workspaceFlowStageHasStatus(stages []workspaceFlowStage, id string) bool {
	for _, stage := range stages {
		if stage.ID == id && stage.Status != "" {
			return true
		}
	}
	return false
}
