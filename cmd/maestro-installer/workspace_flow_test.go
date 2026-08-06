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
)

func TestWorkspaceFlowFailsClosedWhenCapabilityIsUnavailable(t *testing.T) {
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
	if analyzeRecorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("analyze status = %d, body = %s", analyzeRecorder.Code, analyzeRecorder.Body.String())
	}
	if !strings.Contains(analyzeRecorder.Body.String(), `"code":"capability_unavailable"`) || !strings.Contains(analyzeRecorder.Body.String(), `"ready":false`) {
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
	if len(analysis.Mapped) == 0 || len(analysis.Excluded) == 0 || len(analysis.Ambiguous) == 0 || len(analysis.CapabilitiesUnavailable) == 0 {
		t.Fatalf("analysis classifications are incomplete = %#v", analysis)
	}
	if analysis.SourceMutation != "none_until_confirmed" {
		t.Fatalf("source mutation = %q", analysis.SourceMutation)
	}

	wrong := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selection.FlowID+`","plan_digest":"wrong"}`)
	if wrong.Code != http.StatusConflict {
		t.Fatalf("wrong digest status = %d, body = %s", wrong.Code, wrong.Body.String())
	}

	confirmed := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selection.FlowID+`","plan_digest":"`+analysis.PlanDigest+`"}`)
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
			SchemaVersion: 1, FlowID: "flow-1", Operation: "maestro_update", SourceMutation: workspaceFlowSourceMutationNone,
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
	confirmed := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selected.FlowID+`","plan_digest":"digest-1"}`)
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
		workspaceFlow: stubWorkspaceFlowBackend{analysis: workspaceFlowAnalysis{PlanDigest: "unsafe", SourceMutation: "ingested"}},
	})
	selected := postWorkspaceFlow(t, handler, "/api/workspace-flow/select", `{"mode":"external_import"}`)
	var selection workspaceFlowSelectionResponse
	decodeWorkspaceFlow(t, selected, &selection)
	analyzed := postWorkspaceFlow(t, handler, "/api/workspace-flow/analyze", `{"flow_id":"`+selection.FlowID+`"}`)
	if analyzed.Code != http.StatusConflict || !strings.Contains(analyzed.Body.String(), "mutação") {
		t.Fatalf("unsafe analysis response = %d %s", analyzed.Code, analyzed.Body.String())
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
	confirmed := postWorkspaceFlow(t, handler, "/api/workspace-flow/confirm", `{"flow_id":"`+selection.FlowID+`","plan_digest":"`+analysis.PlanDigest+`"}`)
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

func (stub stubWorkspaceFlowBackend) Analyze(_ context.Context, selection workspaceFlowSelection) (workspaceFlowAnalysis, error) {
	analysis := stub.analysis
	analysis.SchemaVersion = workspaceFlowSchemaVersion
	analysis.FlowID = selection.FlowID
	analysis.Mode = selection.Mode
	analysis.Source = selection.Source
	if analysis.SourceMutation == "" {
		analysis.SourceMutation = workspaceFlowSourceMutationNoneUntilConfirmed
	}
	analysis.State = "plan_ready"
	analysis.ConfirmationRequired = true
	return analysis, nil
}

func (stub stubWorkspaceFlowBackend) Confirm(_ context.Context, selection workspaceFlowSelection, planDigest string) (workspaceFlowReceipt, error) {
	receipt := stub.receipt
	receipt.SchemaVersion = workspaceFlowSchemaVersion
	receipt.FlowID = selection.FlowID
	receipt.PlanDigest = planDigest
	if receipt.Operation == "" {
		receipt.Operation = workspaceFlowOperationForMode(selection.Mode)
	}
	if receipt.SourceMutation == "" {
		receipt.SourceMutation = workspaceFlowSourceMutationNone
	}
	return receipt, nil
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
