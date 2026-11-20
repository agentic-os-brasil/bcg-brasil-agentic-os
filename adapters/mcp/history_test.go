package product_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/selectedhistory"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func historyCall(t *testing.T, cs *mcp.ClientSession, tool, operation string, fields map[string]any) map[string]any {
	t.Helper()
	args := map[string]any{"operation": operation}
	for key, value := range fields {
		args[key] = value
	}
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	if out == nil || out.IsError || len(out.Content) != 1 {
		t.Fatalf("%s did not complete through public route: %+v", operation, out)
	}
	var value map[string]any
	if err = json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestHistoryPublicRetainedJourney(t *testing.T) {
	f := newOwnerFixture(t, false)
	path := ownerctx.PromptHistoryEntriesPath(f.root)
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	tools, err := cs.ListTools(context.Background(), nil)
	if err != nil || len(tools.Tools) == 0 {
		t.Fatal("tools/list unavailable", err)
	}
	status := historyCall(t, cs, "maestro_query", "history.status", nil)
	if status["capture_mode"] != "attended_only" {
		t.Fatal("status did not disclose explicit per-record retention", status)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(after) != string(before) {
		t.Fatal("status changed retained history", err)
	}
	historyCall(t, cs, "maestro_apply", "history.record", map[string]any{"owner_id": "owner", "history_recorded_at": time.Now().UTC().Format(time.RFC3339Nano), "history_prompt": "Synthetic owned prompt for public read-back", "history_language": "en", "history_session_id": "session-1", "history_occurrence_id": "occurrence-1"})
	entries, err := ownerctx.ExportPromptHistory(f.root)
	if err != nil || len(entries) != 1 || entries[0].Prompt != "Synthetic owned prompt for public read-back" {
		t.Fatalf("canonical retained history was not written: %+v %v", entries, err)
	}
	if entries[0].ScopeKind != ownerctx.PromptScopeWorkspace {
		t.Fatal("record escaped admitted workspace", entries[0].ScopeKind)
	}
	if filepath.Dir(path) != filepath.Join(f.root, "owner", "prompt-history") {
		t.Fatal("canonical owner changed")
	}
}

func historyFailure(t *testing.T, cs *mcp.ClientSession, tool, operation string, fields map[string]any) map[string]any {
	t.Helper()
	args := map[string]any{"operation": operation}
	for key, value := range fields {
		args[key] = value
	}
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil || out == nil || !out.IsError {
		t.Fatal("expected public history refusal", operation, out, err)
	}
	var value map[string]any
	if err = json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &value); err != nil {
		t.Fatal(err)
	}
	return value
}
func historyRecordFields(body, occurrence string) map[string]any {
	return map[string]any{"owner_id": "owner", "history_prompt": body, "history_language": "en", "history_session_id": "session-1", "history_occurrence_id": occurrence, "history_recorded_at": time.Now().UTC().Format(time.RFC3339Nano)}
}
func TestHistoryPublicSelectedSourceConsentAndChange(t *testing.T) {
	for _, mode := range []string{"accepted", "declined", "unavailable", "changed", "foreign-workspace"} {
		t.Run(mode, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			source := filepath.Join(filepath.Dir(f.options.UserHome), "selected-logs")
			if err := os.Mkdir(source, 0700); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(source, "session.jsonl")
			body, _ := json.Marshal(map[string]any{"cwd": f.options.WorkspacePath, "type": "user", "message": "private selected source marker"})
			body = append(body, '\n')
			if err := os.WriteFile(path, body, 0600); err != nil {
				t.Fatal(err)
			}
			var handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)
			if mode != "unavailable" {
				handler = func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					var shown struct {
						Plan selectedhistory.Plan `json:"plan"`
					}
					start := strings.Index(req.Params.Message, "{")
					if start < 0 || json.Unmarshal([]byte(req.Params.Message[start:]), &shown) != nil {
						t.Fatal("native history review JSON unavailable")
					}
					sources := shown.Plan.Manifest.Sources
					if len(sources) != 1 || sources[0].Root != source || sources[0].Workspace != f.options.WorkspacePath ||
						len(sources[0].Sessions) != 1 || sources[0].Sessions[0].Path != "session.jsonl" {
						t.Error("exact selected paths missing from native review")
					}
					if mode == "changed" {
						if err := os.WriteFile(path, []byte("{}\n"), 0600); err != nil {
							t.Fatal(err)
						}
					}
					action := "accept"
					if mode == "declined" {
						action = "decline"
					}
					return &mcp.ElicitResult{Action: action, Content: map[string]any{"approved": true}}, nil
				}
			}
			cs := f.connect(t, handler)
			manifest := map[string]any{"workspaces": []string{f.options.WorkspacePath}, "sources": []any{map[string]any{"workspace": f.options.WorkspacePath, "root": source, "kind": "claude_code", "sessions": []any{map[string]string{"path": "session.jsonl", "id": "session-1"}}}}}
			if mode == "foreign-workspace" {
				manifest["workspaces"] = []string{f.options.UserHome}
				historyFailure(t, cs, "maestro_query", "history.selection", map[string]any{"history_manifest": manifest})
				return
			}
			selection := historyCall(t, cs, "maestro_query", "history.selection", map[string]any{"history_manifest": manifest})
			metadata, _ := json.Marshal(selection)
			encodedSource, _ := json.Marshal(source)
			sourceFragment := string(encodedSource[1 : len(encodedSource)-1])
			if strings.Contains(string(metadata), "private selected source marker") || strings.Contains(string(metadata), sourceFragment) {
				t.Fatal("selection query disclosed bodies or paths")
			}
			fields := map[string]any{"history_manifest": manifest, "history_selection_digest": selection["selection_digest"]}
			if mode == "accepted" {
				out := historyCall(t, cs, "maestro_apply", "history.read", fields)
				sessions := out["sessions"].([]any)
				if len(sessions) != 1 || sessions[0].(map[string]any)["body"] != string(body) {
					t.Fatal("selected read not exact", out)
				}
			} else {
				out := historyFailure(t, cs, "maestro_apply", "history.read", fields)
				raw, _ := json.Marshal(out)
				if strings.Contains(string(raw), sourceFragment) || strings.Contains(string(raw), "private selected source marker") {
					t.Fatal("refusal leaked private source")
				}
			}
			entries, err := ownerctx.ExportPromptHistory(f.root)
			if err != nil || len(entries) != 0 {
				t.Fatal("selected read retained transcript", entries, err)
			}
		})
	}
}
func TestHistoryPublicRetainedControlsReplayAndSize(t *testing.T) {
	f := newOwnerFixture(t, false)
	reviews := 0
	handler := func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		reviews++
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	}
	cs := f.connect(t, handler)
	// 32KiB of NULs serializes above the old128KiB gate and reaches real native review.
	body := strings.Repeat("\x00", (32<<10)-1) + "x"
	fields := historyRecordFields(body, "max-record")
	historyCall(t, cs, "maestro_apply", "history.record", fields)
	restarted := f.connect(t, handler)
	historyCall(t, restarted, "maestro_apply", "history.record", fields)
	entries, err := ownerctx.ExportPromptHistory(f.root)
	if err != nil || len(entries) != 1 || entries[0].Prompt != body {
		t.Fatal("max/restarted record not exact", err)
	}
	fields["history_prompt"] = "conflicting occurrence"
	historyFailure(t, cs, "maestro_apply", "history.record", fields)
	fields["history_prompt"] = strings.Repeat("x", (32<<10)+1)
	fields["history_occurrence_id"] = "oversize"
	historyFailure(t, cs, "maestro_apply", "history.record", fields)
	historyCall(t, cs, "maestro_apply", "history.record", historyRecordFields("export exactly this owned prompt", "export-record"))
	entries, err = ownerctx.ExportPromptHistory(f.root)
	if err != nil {
		t.Fatal(err)
	}
	id := ""
	for _, entry := range entries {
		if entry.Prompt == "export exactly this owned prompt" {
			id = entry.ID
		}
	}
	exported := historyCall(t, cs, "maestro_apply", "history.export", map[string]any{"owner_id": "owner", "history_ids": []string{id}})
	if exported["entries"].([]any)[0].(map[string]any)["prompt"] != "export exactly this owned prompt" {
		t.Fatal("export not exact")
	}
	historyCall(t, cs, "maestro_apply", "history.delete", map[string]any{"owner_id": "owner", "history_ids": []string{id}})
	historyCall(t, cs, "maestro_apply", "history.configure", map[string]any{"owner_id": "owner", "history_config": map[string]any{"schema_version": 1, "max_entries": 1, "max_bytes": 8 << 20, "max_age_seconds": 86400}})
	historyCall(t, cs, "maestro_apply", "history.reset", map[string]any{"owner_id": "owner"})
	entries, err = ownerctx.ExportPromptHistory(f.root)
	if err != nil || len(entries) != 0 || reviews < 7 {
		t.Fatal("controls not reviewed/applied", reviews, err)
	}
}
func TestHistoryPublicDeclineMissingReviewAndCASPreserve(t *testing.T) {
	for _, mode := range []string{"decline", "unavailable", "changed-store", "changed-scope"} {
		t.Run(mode, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			before, err := os.ReadFile(ownerctx.PromptHistoryEntriesPath(f.root))
			if err != nil {
				t.Fatal(err)
			}
			var handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)
			if mode != "unavailable" {
				handler = func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
					if mode == "changed-store" {
						_, err := ownerctx.RecordUserPrompt(f.root, ownerctx.PromptHistoryInput{OwnerID: "owner", OccurrenceID: "concurrent", Prompt: "concurrent owned entry", Language: "en", Source: "owner", SessionID: "another-session", ScopeKind: ownerctx.PromptScopeWorkspace, ScopeID: "another-scope", ContentKind: "user_prompt"})
						if err != nil {
							t.Fatal(err)
						}
					}
					if mode == "changed-scope" {
						putContentFile(t, filepath.Join(f.options.WorkspacePath, "brain", "accounts", ".active"), []byte("new-account/new-case\n"))
					}
					action := "accept"
					if mode == "decline" {
						action = "decline"
					}
					return &mcp.ElicitResult{Action: action, Content: map[string]any{"approved": true}}, nil
				}
			}
			cs := f.connect(t, handler)
			historyFailure(t, cs, "maestro_apply", "history.record", historyRecordFields("must remain unrecorded", "negative"))
			after, err := os.ReadFile(ownerctx.PromptHistoryEntriesPath(f.root))
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(after), "must remain unrecorded") {
				t.Fatal("rejected operation wrote prompt")
			}
			if mode != "changed-store" && string(after) != string(before) {
				t.Fatal("decline/unavailable/changed scope altered entries")
			}
		})
	}
}

func TestHistoryPublicCompiledProcessRestart(t *testing.T) {
	f := newOwnerFixture(t, true)
	if _, err := ownerctx.Initialize(f.root); err != nil {
		t.Fatal(err)
	}
	accept := func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	}
	fields := historyRecordFields("persist only this explicitly reviewed user prompt", "process-once")
	first := f.connect(t, accept)
	historyCall(t, first, "maestro_apply", "history.record", fields)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	second := f.connect(t, accept)
	status := historyCall(t, second, "maestro_query", "history.status", nil)
	if len(status["entries"].([]any)) != 1 {
		t.Fatal("compiled restarted server lost record", status)
	}
	historyCall(t, second, "maestro_apply", "history.record", fields)
	entries, err := ownerctx.ExportPromptHistory(f.root)
	if err != nil || len(entries) != 1 || entries[0].Prompt != fields["history_prompt"] {
		t.Fatal("compiled restart duplicated or changed record", err)
	}
}

func TestHistoryMethodPlanMatchesRegisteredOwners(t *testing.T) {
	f := newOwnerFixture(t, false)
	cs := f.connect(t, nil)
	plan := historyCall(t, cs, "maestro_query", "methods.plan", map[string]any{"method_id": "learn-from-logs"})
	if plan["execution_state"] != "not_executed" || plan["state"] != "requirements_unavailable" {
		t.Fatal("method selection granted host execution", plan)
	}
	domain, host := false, false
	for _, raw := range plan["dependencies"].([]any) {
		dependency := raw.(map[string]any)
		switch dependency["id"] {
		case "domain.opt_in_prompt_history":
			domain = dependency["state"] == "available" && len(dependency["fulfillment_operations"].([]any)) == 9 && len(dependency["missing_operations"].([]any)) == 0
		case "host.trusted_review":
			host = dependency["state"] == "unavailable"
		}
	}
	if !domain || !host {
		t.Fatal("method declaration omitted owners or invented native qualification", plan)
	}
}

func TestHistoryPublicValidatedCaseScopeExcludesLegacySlugAliases(t *testing.T) {
	f := newOwnerFixture(t, false)
	cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	prepared := historyCall(t, cs, "maestro_apply", "case.setup.prepare", map[string]any{"mode": "client", "project_name": "Synthetic history scope", "account_id": "synthetic", "account_owner": "owner", "account_mandate": "Hold the explicitly selected synthetic case", "expected_setup_revision": "absent"})
	historyFailure(t, cs, "maestro_query", "history.status", nil)
	setup := prepared["setup"].(map[string]any)
	id := prepared["workspace_id"].(string)
	page := setup["brief_page"].(string)
	text := strings.Replace(strings.Replace(authoredBrief(id, page, "Synthetic history scope brief"), "scope: workspace/"+id, "scope: account/synthetic/case/"+id, 1), "sensitivity: internal", "sensitivity: client-confidential", 1)
	created := brainCall(t, cs, "create", map[string]any{"page": page, "text": text}, false)
	active := historyCall(t, cs, "maestro_apply", "case.setup.finish", map[string]any{"expected_setup_revision": prepared["setup_revision"], "expected_page_revision": created["revision"]})
	agentID := active["setup"].(map[string]any)["case_agent_id"].(string)
	historyCall(t, cs, "maestro_apply", "history.record", historyRecordFields("only the validated active case", "case-record"))
	foreign, err := ownerctx.RecordUserPrompt(f.root, ownerctx.PromptHistoryInput{OwnerID: "owner", OccurrenceID: "legacy-slug", Prompt: "foreign legacy slug marker", Language: "en", Source: "owner", SessionID: "legacy-session", ScopeKind: ownerctx.PromptScopeCase, ScopeID: id, ContentKind: "user_prompt"})
	if err != nil {
		t.Fatal(err)
	}
	entries, err := ownerctx.ExportPromptHistory(f.root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.Prompt == "only the validated active case" && (entry.ScopeKind != ownerctx.PromptScopeCase || entry.ScopeID != agentID) {
			t.Fatal("case scope did not use validated registration", entry.ScopeID)
		}
	}
	status := historyCall(t, cs, "maestro_query", "history.status", nil)
	if len(status["entries"].([]any)) != 1 {
		t.Fatal("foreign legacy slug entered current case metadata", status)
	}
	historyFailure(t, cs, "maestro_apply", "history.export", map[string]any{"owner_id": "owner", "history_ids": []string{foreign.ID}})
}
