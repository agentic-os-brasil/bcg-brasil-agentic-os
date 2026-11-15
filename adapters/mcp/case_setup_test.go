package product_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacepolicy"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The existing owner journey owns this public SDK and shipping-stdio workflow.
func assertCaseSetupJourney(t *testing.T) {
	assertCaseSetupRefusals(t)
	t.Helper()
	root := physicalTempDir(t)
	markers := filepath.Join(root, "brain", "accounts")
	if err := os.MkdirAll(markers, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(markers, ".pending"), []byte("synthetic/first\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if !workspacepolicy.CasePathAllowed(root, filepath.Join(markers, "synthetic", "cases", "first", "first.md")) {
		t.Error("first pending case is refused before an active case exists")
	}
	if workspacepolicy.CasePathAllowed(root, filepath.Join(markers, "synthetic", "cases", "foreign", "first.md")) {
		t.Error("pending selection admitted a foreign case")
	}
	if err := os.WriteFile(filepath.Join(markers, ".active"), []byte("corrupt\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if workspacepolicy.CasePathAllowed(root, filepath.Join(markers, "synthetic", "cases", "first", "first.md")) {
		t.Error("invalid active state became a fresh pending selection")
	}
	for _, stdio := range []bool{false, true} {
		f := newOwnerFixture(t, stdio)
		allow := false
		var presentation string
		cs := f.connect(t, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			presentation = req.Params.Message
			if !allow {
				return &mcp.ElicitResult{Action: "decline"}, nil
			}
			return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
		})
		call := func(op string, fields map[string]any, fail bool) map[string]any {
			tool := "maestro_apply"
			if op == "status" {
				tool = "maestro_query"
			}
			args := map[string]any{"operation": "case.setup." + op}
			for k, v := range fields {
				args[k] = v
			}
			return contentCall(t, cs, tool, args, fail)
		}
		status := call("status", nil, false)
		if status["state"] != "absent" || status["setup_revision"] != "absent" {
			t.Fatal("fresh case status is not absent", status)
		}
		stateRoot := filepath.Join(filepath.Dir(f.root), "state")
		for _, path := range []string{filepath.Join(stateRoot, "agents", "instances"), filepath.Join(stateRoot, "config", "agent-scaffold-integrity.key")} {
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatal("query created enrollment state", path, err)
			}
		}
		in := map[string]any{"mode": "client", "project_name": "Synthetic first useful case", "account_id": "synthetic", "account_owner": "owner", "account_mandate": "Maintain the bounded synthetic account metadata.", "expected_setup_revision": "absent"}
		declined := call("prepare", in, true)
		if declined["code"] != "confirmation_declined" {
			t.Fatal("native decline not honored", declined)
		}
		if _, err := os.Lstat(filepath.Join(stateRoot, "config", "agent-scaffold-integrity.key")); !os.IsNotExist(err) {
			t.Fatal("decline created signing material", err)
		}
		allow = true
		status = call("prepare", in, false)
		if status["state"] != "pending" {
			t.Fatal("first case was not prepared", status)
		}
		setup := status["setup"].(map[string]any)
		id := status["workspace_id"].(string)
		page := setup["brief_page"].(string)
		if page != "accounts/synthetic/cases/"+id+"/"+id+".md" {
			t.Fatal("first case path not bound", setup)
		}
		for _, needle := range []string{"Synthetic first useful case", "catalog_sha256", "template_sha256", "client-account-agent-synthetic", "case-agent-" + id, "bounded_client_account_packet"} {
			if !strings.Contains(presentation, needle) {
				t.Fatal("full local enrollment review omitted", needle)
			}
		}
		signed, err := agentscaffold.Inspect(stateRoot, "case-agent-"+id)
		if err != nil || signed.Instance.RuntimeState != "unavailable" || signed.Instance.AccountAgentID != "client-account-agent-synthetic" {
			t.Fatal("signed local registration missing or implied runtime", signed, err)
		}
		content := strings.Replace(strings.Replace(authoredBrief(id, page, "Synthetic reviewed first brief"), "scope: workspace/"+id, "scope: account/synthetic/case/"+id, 1), "sensitivity: internal", "sensitivity: client-confidential", 1)
		created := brainCall(t, cs, "create", map[string]any{"page": page, "text": content}, false)
		read := brainCall(t, cs, "read", map[string]any{"page": page, "purpose": "Read back the first useful case brief"}, false)
		if read["text"] != content || read["revision"] != created["revision"] {
			t.Fatal("first case brain readback changed", read)
		}
		finish := map[string]any{"expected_setup_revision": status["setup_revision"], "expected_page_revision": created["revision"], "objective": "Continue this explicitly accepted synthetic case", "next_step": "Read the same brief in the second client", "summary": "  Synthetic first case is ready for a reviewed handoff.  "}
		before := call("status", nil, false)
		finish["expected_page_revision"] = strings.Repeat("0", 64)
		call("finish", finish, true)
		if call("status", nil, false)["setup_revision"] != before["setup_revision"] {
			t.Fatal("stale finish changed setup")
		}
		finish["expected_page_revision"] = created["revision"]
		status = call("finish", finish, false)
		if status["state"] != "active" {
			t.Fatal("first case was not completed", status)
		}
		setup = status["setup"].(map[string]any)
		item := setup["item_id"].(string)
		next := contentCall(t, cs, "maestro_query", map[string]any{"operation": "execution.next", "item_id": item}, false)
		if next["state"] != "paused" || next["state_revision"] != float64(4) || next["summary"] != strings.TrimSpace(finish["summary"].(string)) {
			t.Fatal("setup did not create a real paused checkpoint", next)
		}
		if _, err := os.Lstat(filepath.Join(f.options.WorkspacePath, "brain", "accounts", ".pending")); !os.IsNotExist(err) {
			t.Fatal("committed case left its pending selector", err)
		}
		finish["expected_setup_revision"] = status["setup_revision"]
		replay := call("finish", finish, false)
		if replay["setup"].(map[string]any)["item_id"] != item {
			t.Fatal("retry duplicated work", replay)
		}
		cs.Close()
		cold := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
		})
		current := contentCall(t, cold, "maestro_query", map[string]any{"operation": "case.setup.status"}, false)
		if current["state"] != "active" || current["setup"].(map[string]any)["item_id"] != item {
			t.Fatal("cold process lost first-case state", current)
		}
		brainCall(t, cold, "read", map[string]any{"page": page, "purpose": "Continue the same case after cold restart"}, false)
		listed := contentCall(t, cold, "maestro_query", map[string]any{"operation": "execution.list"}, false)
		if len(listed["items"].([]any)) != 1 {
			t.Fatal("setup retry created duplicate ledger items", listed)
		}
		resumed := contentCall(t, cold, "maestro_apply", map[string]any{"operation": "execution.resume", "item_id": item, "expected_revision": 4}, false)
		if resumed["state_revision"] != float64(5) {
			t.Fatal("second client did not continue same execution", resumed)
		}
	}
	// Direct internal work needs neither a fictional account nor account selectors.
	f := newOwnerFixture(t, false)
	var internalReview string
	cs := f.connect(t, func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		internalReview = req.Params.Message
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	prepared := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "case.setup.prepare", "mode": "internal", "project_name": "Synthetic direct internal work", "expected_setup_revision": "absent"}, false)
	setup := prepared["setup"].(map[string]any)
	page := setup["brief_page"].(string)
	id := prepared["workspace_id"].(string)
	fullBody := authoredBrief(id, page, "Synthetic internal first result")
	fullBody += strings.Repeat("<>&", ((255<<10)-len(fullBody))/3)
	created := brainCall(t, cs, "create", map[string]any{"page": page, "text": fullBody}, false)
	done := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "case.setup.finish", "expected_setup_revision": prepared["setup_revision"], "expected_page_revision": created["revision"]}, false)
	var rendered map[string]any
	if err := json.Unmarshal([]byte(internalReview[strings.Index(internalReview, "{"):]), &rendered); err != nil || rendered["brief"].(map[string]any)["text"] != fullBody {
		t.Fatal("expanded native review lost full brief", err)
	}
	if done["state"] != "active" || done["setup"].(map[string]any)["item_id"] != nil {
		t.Fatal("internal no-work setup did not finish without invented work", done)
	}
	if _, err := os.Lstat(filepath.Join(f.options.WorkspacePath, "brain", "accounts")); !os.IsNotExist(err) {
		t.Fatal("internal setup fabricated an account namespace", err)
	}
}

func assertCaseSetupRefusals(t *testing.T) {
	t.Helper()
	for _, change := range []string{"missing_form", "personalization", "selector"} {
		f := newOwnerFixture(t, false)
		stateRoot := filepath.Join(filepath.Dir(f.root), "state")
		var id string
		var handler func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error)
		if change != "missing_form" {
			handler = func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				if change == "selector" {
					dir := filepath.Join(f.options.WorkspacePath, "brain", "accounts")
					if err := os.MkdirAll(dir, 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(filepath.Join(dir, ".pending"), []byte("foreign/case\n"), 0600); err != nil {
						t.Fatal(err)
					}
				} else {
					err := agentidentity.Save(stateRoot, agentidentity.Profile{SchemaVersion: 1, OwnerID: "owner", Confirmed: true, UpdatedAt: time.Now().UTC(), Selections: []agentidentity.Selection{{Role: "case_agent", AgentID: "case-agent-" + id, DisplayName: "Changed after native review", Emoji: "🧪", OwnerID: "owner", OwnershipScope: "case"}}})
					if err != nil {
						t.Fatal(err)
					}
				}
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			}
		}
		cs := f.connect(t, handler)
		id = contentCall(t, cs, "maestro_context", map[string]any{}, false)["workspace_id"].(string)
		rejected := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "case.setup.prepare", "mode": "internal", "project_name": "Synthetic review fence", "expected_setup_revision": "absent"}, true)
		expected := "case_setup_changed"
		if change == "missing_form" {
			expected = "confirmation_unavailable"
		}
		if rejected["code"] != expected {
			t.Fatal("native input/review fence returned wrong code", change, rejected)
		}
		for _, p := range []string{filepath.Join(stateRoot, "config", "agent-scaffold-integrity.key"), filepath.Join(stateRoot, "workspaces", id, "agent", "setup.json")} {
			if _, err := os.Lstat(p); !os.IsNotExist(err) {
				t.Fatal("rejected native review persisted a setup or key", change, p, err)
			}
		}
	}
	// A user edit during the finish review stays owned by that user. No ledger or
	// active selection is published from the stale page, even with valid metadata.
	f := newOwnerFixture(t, false)
	var duringReview func()
	cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		if duringReview != nil {
			duringReview()
		}
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	status := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "case.setup.prepare", "mode": "internal", "project_name": "Synthetic live user edit", "expected_setup_revision": "absent"}, false)
	page := status["setup"].(map[string]any)["brief_page"].(string)
	id := status["workspace_id"].(string)
	content := authoredBrief(id, page, "Synthetic reviewable brief")
	created := brainCall(t, cs, "create", map[string]any{"page": page, "text": content}, false)
	contentCall(t, cs, "maestro_apply", map[string]any{"operation": "case.setup.finish", "expected_setup_revision": status["setup_revision"], "expected_page_revision": created["revision"], "objective": "Synthetic bounded projection", "next_step": strings.Repeat("<", 768), "summary": strings.Repeat("<", 768)}, true)
	unchanged := contentCall(t, cs, "maestro_query", map[string]any{"operation": "case.setup.status"}, false)
	if unchanged["setup"].(map[string]any)["work"] != nil {
		t.Fatal("invalid checkpoint projection stranded a persisted work intent", unchanged)
	}
	path := filepath.Join(f.options.WorkspacePath, "brain", filepath.FromSlash(page))
	edited := content + "\nA concurrent user correction must be preserved.\n"
	duringReview = func() {
		if err := os.WriteFile(path, []byte(edited), 0600); err != nil {
			t.Fatal(err)
		}
	}
	out := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "case.setup.finish", "expected_setup_revision": status["setup_revision"], "expected_page_revision": created["revision"], "objective": "Accepted synthetic work", "next_step": "Do the reviewed next step", "summary": "Reviewed synthetic result"}, true)
	if out["code"] != "case_setup_changed" {
		t.Fatal("finish ignored concurrent full-page revision", out)
	}
	listed := contentCall(t, cs, "maestro_query", map[string]any{"operation": "execution.list"}, false)
	if len(listed["items"].([]any)) != 0 {
		t.Fatal("stale finish created work", listed)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != edited {
		t.Fatal("stale finish replaced the user edit", err)
	}
	corrupt := filepath.Join(filepath.Dir(f.root), "state", "workspaces", id, "agent", "state.json")
	if err = os.WriteFile(corrupt, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	blocked := contentCall(t, cs, "maestro_query", map[string]any{"operation": "case.setup.status"}, true)
	if blocked["state"] != "blocked" {
		t.Fatal("corrupt canonical state was mislabeled as fresh", blocked)
	}
}
