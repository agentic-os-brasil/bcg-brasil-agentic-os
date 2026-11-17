package product_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func accountNarrativeCall(t *testing.T, cs *mcp.ClientSession, op string, fields map[string]any, fail bool) map[string]any {
	t.Helper()
	tool := "maestro_query"
	if op == "save" {
		tool = "maestro_apply"
	}
	args := map[string]any{"operation": "account.narrative." + op}
	for k, v := range fields {
		args[k] = v
	}
	return contentCall(t, cs, tool, args, fail)
}
func prepareNarrativeCase(t *testing.T, f ownerFixture, cs *mcp.ClientSession) string {
	t.Helper()
	ready := contentCall(t, cs, "maestro_apply", map[string]any{"operation": "case.setup.prepare", "mode": "client", "project_name": "Synthetic narrative case", "account_id": "synthetic", "account_owner": "owner", "account_mandate": "Keep explicitly reviewed synthetic relationship context.", "expected_setup_revision": "absent"}, false)
	setup := ready["setup"].(map[string]any)
	id := ready["workspace_id"].(string)
	page := setup["brief_page"].(string)
	text := strings.Replace(strings.Replace(authoredBrief(id, page, "Synthetic case brief"), "scope: workspace/"+id, "scope: account/synthetic/case/"+id, 1), "sensitivity: internal", "sensitivity: client-confidential", 1)
	created := brainCall(t, cs, "create", map[string]any{"page": page, "text": text}, false)
	contentCall(t, cs, "maestro_apply", map[string]any{"operation": "case.setup.finish", "expected_setup_revision": ready["setup_revision"], "expected_page_revision": created["revision"]}, false)
	return id
}
func accountNarrativeText(id, note string, ids ...string) string {
	var cases strings.Builder
	for _, agentID := range ids {
		fmt.Fprintf(&cases, "- `%s` — account `synthetic`; case `%s`\n", agentID, strings.TrimPrefix(agentID, "case-agent-"))
	}
	return "---\nid: accounts/synthetic/synthetic\ntitle: Synthetic account\nsummary: Explicitly reviewed synthetic relationship context.\ntype: account\nscope: account/synthetic\nstatus: active\nsensitivity: client-confidential\nupdated: 2026-11-16\n---\n# Synthetic account\n\n## Relationship\n\n" + note + "\n\n## Cases\n\n" + cases.String()
}
func TestAccountNarrativePublicJourney(t *testing.T) {
	for _, stdio := range []bool{false, true} {
		t.Run(fmt.Sprint(stdio), func(t *testing.T) {
			f := newOwnerFixture(t, stdio)
			allow := true
			var review string
			cs := f.connect(t, func(_ context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				review = r.Params.Message
				if !allow {
					return &mcp.ElicitResult{Action: "decline"}, nil
				}
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			})
			accountNarrativeCall(t, cs, "status", nil, true)
			id := prepareNarrativeCase(t, f, cs)
			status := accountNarrativeCall(t, cs, "status", nil, false)
			plan := contentCall(t, cs, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "account-case-setup"}, false)
			narrowReady, broadPending, hostPending := false, false, false
			for _, raw := range plan["dependencies"].([]any) {
				d := raw.(map[string]any)
				switch d["id"] {
				case "domain.account_narrative":
					narrowReady = d["state"] == "available"
				case "domain.account_case_scaffold":
					broadPending = d["state"] == "unavailable"
				case "host.trusted_review":
					hostPending = d["state"] == "unavailable"
				}
			}
			if !narrowReady || !broadPending || !hostPending || plan["execution_state"] != "not_executed" {
				t.Fatal("method plan invented broad account/host authority", plan)
			}
			if status["state"] != "absent" || status["revision"] != "absent" || status["page"] != "accounts/synthetic/synthetic.md" {
				t.Fatal("fresh narrative status", status)
			}
			target := filepath.Join(f.root, "accounts", "synthetic", "synthetic.md")
			if _, e := os.Lstat(target); !os.IsNotExist(e) {
				t.Fatal("status created narrative", e)
			}
			body := accountNarrativeText(id, "SYNTHETIC_ACCOUNT_ONLY", "case-agent-"+id)
			args := map[string]any{"text": body, "expected_page_revision": "absent", "case_agent_ids": []string{"case-agent-" + id}}
			allow = false
			if got := accountNarrativeCall(t, cs, "save", args, true); got["code"] != "confirmation_declined" {
				t.Fatal(got)
			}
			if _, e := os.Lstat(target); !os.IsNotExist(e) {
				t.Fatal("decline created page", e)
			}
			allow = true
			saved := accountNarrativeCall(t, cs, "save", args, false)
			var shown struct {
				ProposedText string                    `json:"proposed_text"`
				Current      *struct{ Content string } `json:"current"`
			}
			if e := json.Unmarshal([]byte(review[strings.Index(review, "{"):]), &shown); e != nil || shown.ProposedText != body || shown.Current != nil {
				t.Fatal("incomplete create review", e)
			}
			if saved["state"] != "written" {
				t.Fatal(saved)
			}
			got, e := os.ReadFile(target)
			if e != nil || string(got) != body {
				t.Fatal("wrong shared account destination", e)
			}
			read := accountNarrativeCall(t, cs, "read", map[string]any{"purpose": "Read the exact saved account narrative"}, false)
			if read["text"] != body || read["revision"] != saved["revision"] {
				t.Fatal("readback", read)
			}
			if retry := accountNarrativeCall(t, cs, "save", args, false); retry["operation_id"] != saved["operation_id"] || retry["state"] != "unchanged" {
				t.Fatal("create replay", retry)
			}
			stateRoot := filepath.Join(filepath.Dir(f.root), "state")
			foreign, e := agentscaffold.Scaffold(stateRoot, agentscaffold.Request{AgentID: "case-agent-second", Role: "case_agent", ScopeKind: "case", ScopeID: "second", ParentAgent: "maestro", ParentRole: "hub", AccountAgentID: "client-account-agent-synthetic"})
			if e != nil {
				t.Fatal(e)
			}
			revised := accountNarrativeText(id, "REVIEWED_SECOND_CASE", "case-agent-"+id, foreign.Instance.AgentID)
			update := map[string]any{"text": revised, "expected_page_revision": saved["revision"], "case_agent_ids": []string{"case-agent-" + id, foreign.Instance.AgentID}}
			changed := accountNarrativeCall(t, cs, "save", update, false)
			if e := json.Unmarshal([]byte(review[strings.Index(review, "{"):]), &shown); e != nil || shown.Current == nil || shown.Current.Content != body || shown.ProposedText != revised {
				t.Fatal("complete old/new review missing", e)
			}
			if retry := accountNarrativeCall(t, cs, "save", update, false); retry["operation_id"] != changed["operation_id"] {
				t.Fatal("update replay duplicated", retry)
			}
			update["text"] = strings.Replace(revised, "REVIEWED_SECOND_CASE", "STALE", 1)
			accountNarrativeCall(t, cs, "save", update, true)
			// A second separately registered workspace reuses the exact account owner.
			second := f
			second.options.WorkspacePath = filepath.Join(filepath.Dir(f.options.WorkspacePath), "second-workspace")
			if _, err := workspace.Initialize(workspace.Options{WorkspacePath: second.options.WorkspacePath, DataRoot: stateRoot}); err != nil {
				t.Fatal(err)
			}
			other := second.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			})
			prepareNarrativeCase(t, second, other)
			shared := accountNarrativeCall(t, other, "read", map[string]any{"purpose": "Read the one account narrative from a second admitted case"}, false)
			if shared["revision"] != changed["revision"] || shared["text"] != revised {
				t.Fatal("account narrative was copied or lost across workspaces", shared)
			}
			other.Close()
			// A cold process reads the same shared source.
			cs.Close()
			cold := f.connect(t, nil)
			read = accountNarrativeCall(t, cold, "read", map[string]any{"purpose": "Cold read of the same shared account"}, false)
			if read["text"] != revised || read["revision"] != changed["revision"] {
				t.Fatal("cold narrative lost", read)
			}
			if got := accountNarrativeCall(t, cold, "save", map[string]any{"text": body, "expected_page_revision": changed["revision"], "case_agent_ids": []string{"case-agent-" + id}}, true); got["code"] != "confirmation_unavailable" {
				t.Fatal("missing review", got)
			}
			accountNarrativeCall(t, cold, "read", map[string]any{"purpose": "No model-selected root", "page": "../foreign.md"}, true)
			if got, e := os.ReadFile(target); e != nil || string(got) != revised {
				t.Fatal("refusal changed narrative", e)
			}
		})
	}
}

func TestAccountNarrativeReviewAndScopeFences(t *testing.T) {
	for _, kind := range []string{"page", "selection", "case-registration", "wrong-account", "metadata", "cases"} {
		t.Run(kind, func(t *testing.T) {
			f := newOwnerFixture(t, false)
			var duringReview func()
			reviews := 0
			cs := f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
				reviews++
				if duringReview != nil {
					duringReview()
				}
				return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
			})
			id := prepareNarrativeCase(t, f, cs)
			body := accountNarrativeText(id, "ORIGINAL_ACCOUNT_BODY", "case-agent-"+id)
			saved := accountNarrativeCall(t, cs, "save", map[string]any{"text": body, "expected_page_revision": "absent", "case_agent_ids": []string{"case-agent-" + id}}, false)
			target := filepath.Join(f.root, "accounts", "synthetic", "synthetic.md")
			next := strings.Replace(body, "ORIGINAL_ACCOUNT_BODY", "PROPOSED_ACCOUNT_BODY", 1)
			ids := []string{"case-agent-" + id}
			want := body
			switch kind {
			case "page":
				want = strings.Replace(body, "ORIGINAL_ACCOUNT_BODY", "CONCURRENT_OWNER_EDIT", 1)
				duringReview = func() {
					if err := os.WriteFile(target, []byte(want), 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "selection":
				duringReview = func() {
					p := filepath.Join(f.options.WorkspacePath, "brain", "accounts", ".active")
					if err := os.WriteFile(p, []byte("synthetic/foreign\n"), 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "case-registration":
				duringReview = func() {
					p := filepath.Join(filepath.Dir(f.root), "state", "agents", "instances", "case-agent-"+id, "instance.json")
					raw, e := os.ReadFile(p)
					if e != nil {
						t.Fatal(e)
					}
					var tampered map[string]any
					if e = json.Unmarshal(raw, &tampered); e != nil {
						t.Fatal(e)
					}
					tampered["runtime_state"] = "active"
					raw, e = json.Marshal(tampered)
					if e != nil {
						t.Fatal(e)
					}
					if e = os.WriteFile(p, raw, 0600); e != nil {
						t.Fatal(e)
					}
				}
			case "wrong-account":
				root := filepath.Join(filepath.Dir(f.root), "state")
				if _, e := agentscaffold.Scaffold(root, agentscaffold.Request{AgentID: "client-account-agent-foreign", Role: "client_account_agent", ScopeKind: "account", ScopeID: "foreign", ParentAgent: "maestro", ParentRole: "hub", Owner: "owner", Mandate: "Foreign account remains isolated."}); e != nil {
					t.Fatal(e)
				}
				if _, e := agentscaffold.Scaffold(root, agentscaffold.Request{AgentID: "case-agent-foreign", Role: "case_agent", ScopeKind: "case", ScopeID: "foreign", ParentAgent: "maestro", ParentRole: "hub", AccountAgentID: "client-account-agent-foreign"}); e != nil {
					t.Fatal(e)
				}
				ids = append(ids, "case-agent-foreign")
			case "metadata":
				next = strings.Replace(next, "sensitivity: client-confidential", "sensitivity: internal", 1)
			case "cases":
				next = strings.Replace(next, "; case `"+id+"`", "; case `fabricated`", 1)
			}
			before := reviews
			accountNarrativeCall(t, cs, "save", map[string]any{"text": next, "expected_page_revision": saved["revision"], "case_agent_ids": ids}, true)
			if kind == "wrong-account" || kind == "metadata" || kind == "cases" {
				if reviews != before {
					t.Fatal("invalid request reached native review")
				}
			}
			got, e := os.ReadFile(target)
			if e != nil || string(got) != want {
				t.Fatal("stale/tampered review overwrote narrative", e)
			}
		})
	}
}

func TestAccountNarrativeEscapedBoundary(t *testing.T) {
	f := newOwnerFixture(t, false)
	var presentation string
	cs := f.connect(t, func(_ context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		presentation = r.Params.Message
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	id := prepareNarrativeCase(t, f, cs)
	base := accountNarrativeText(id, "BOUNDARY", "case-agent-"+id)
	before := strings.Replace(base, "BOUNDARY", strings.Repeat("<", (64<<10)-len(base)+len("BOUNDARY")), 1)
	args := map[string]any{"text": before, "expected_page_revision": "absent", "case_agent_ids": []string{"case-agent-" + id}}
	raw, _ := json.Marshal(args)
	if len(raw) < 128<<10 {
		t.Fatal("fixture does not exercise expanded wire guard")
	}
	saved := accountNarrativeCall(t, cs, "save", args, false)
	after := strings.ReplaceAll(before, "<", ">&")
	args["text"] = after
	args["expected_page_revision"] = saved["revision"]
	accountNarrativeCall(t, cs, "save", args, true)
	args["text"] = strings.ReplaceAll(before, "<", ">")
	changed := accountNarrativeCall(t, cs, "save", args, false)
	var shown struct {
		ProposedText string                    `json:"proposed_text"`
		Current      *struct{ Content string } `json:"current"`
	}
	if e := json.Unmarshal([]byte(presentation[strings.Index(presentation, "{"):]), &shown); e != nil || shown.Current == nil || shown.Current.Content != before || shown.ProposedText != args["text"] {
		t.Fatal("escaped current/proposed review was truncated", e)
	}
	read := accountNarrativeCall(t, cs, "read", map[string]any{"purpose": "Read exact maximum bounded account page"}, false)
	if read["text"] != args["text"] || read["revision"] != changed["revision"] {
		t.Fatal("boundary readback", read)
	}
}

func TestAccountNarrativeLegacyCasesFullReview(t *testing.T) {
	f := newOwnerFixture(t, false)
	var presentation string
	cs := f.connect(t, func(_ context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		presentation = r.Params.Message
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	id := prepareNarrativeCase(t, f, cs)
	body := accountNarrativeText(id, "Owner-approved relationship", "case-agent-"+id)
	accountNarrativeCall(t, cs, "save", map[string]any{"text": body, "expected_page_revision": "absent", "case_agent_ids": []string{"case-agent-" + id}}, false)
	// Legacy prose is preserved and completely reviewed even when its Cases
	// section is not a source-backed canonical registration reference list.
	legacy := body + strings.Repeat("<", (64<<10)-len(body))
	path := filepath.Join(f.root, "accounts", "synthetic", "synthetic.md")
	if err := os.WriteFile(path, []byte(legacy), 0600); err != nil {
		t.Fatal(err)
	}
	status := accountNarrativeCall(t, cs, "status", nil, false)
	next := strings.Replace(body, "Owner-approved relationship", strings.Repeat(">", (64<<10)-len(body)+len("Owner-approved relationship")), 1)
	accountNarrativeCall(t, cs, "save", map[string]any{"text": next, "expected_page_revision": status["revision"], "case_agent_ids": []string{"case-agent-" + id}}, false)
	var view struct {
		ProposedText string                   `json:"proposed_text"`
		Current      struct{ Content string } `json:"current"`
		Unverified   int                      `json:"existing_unverified_case_line_count"`
	}
	if err := json.Unmarshal([]byte(presentation[strings.Index(presentation, "{"):]), &view); err != nil || view.Current.Content != legacy || view.ProposedText != next || view.Unverified != 1 {
		t.Fatal("legacy Cases review was truncated or silently trusted", err)
	}
}

func TestAccountNarrativeMaximumCaseReview(t *testing.T) {
	f := newOwnerFixture(t, false)
	var presentation string
	cs := f.connect(t, func(_ context.Context, r *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		presentation = r.Params.Message
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	id := prepareNarrativeCase(t, f, cs)
	ids := []string{"case-agent-" + id}
	for i := 0; i < 31; i++ {
		scope := fmt.Sprintf("synthetic-%02d-%s", i, strings.Repeat("a", 40))
		agentID := "case-agent-" + scope
		if _, err := agentscaffold.Scaffold(filepath.Join(filepath.Dir(f.root), "state"), agentscaffold.Request{AgentID: agentID, Role: "case_agent", ScopeKind: "case", ScopeID: scope, ParentAgent: "maestro", ParentRole: "hub", AccountAgentID: "client-account-agent-synthetic", DisplayName: strings.Repeat("<", 80)}); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, agentID)
	}
	sort.Strings(ids)
	base := accountNarrativeText(id, "MAXIMUM_REVIEW", ids...)
	before := strings.Replace(base, "MAXIMUM_REVIEW", strings.Repeat("<", (64<<10)-len(base)+len("MAXIMUM_REVIEW")), 1)
	saved := accountNarrativeCall(t, cs, "save", map[string]any{"text": before, "expected_page_revision": "absent", "case_agent_ids": ids}, false)
	after := strings.ReplaceAll(before, "<", ">")
	accountNarrativeCall(t, cs, "save", map[string]any{"text": after, "expected_page_revision": saved["revision"], "case_agent_ids": ids}, false)
	start := strings.Index(presentation, "{")
	var shown struct {
		ProposedText string                   `json:"proposed_text"`
		Current      struct{ Content string } `json:"current"`
		Cases        []agentscaffold.Instance `json:"proposed_case_registrations"`
		Existing     []any                    `json:"existing_verified_case_registrations"`
	}
	if err := json.Unmarshal([]byte(presentation[start:]), &shown); err != nil || shown.Current.Content != before || shown.ProposedText != after || len(shown.Cases) != 32 || len(shown.Existing) != 32 {
		t.Fatal("maximum source-backed review incomplete", err)
	}
	if len(presentation[start:]) > 1<<20 {
		t.Fatal("ordinary review bound exceeded")
	}
	t.Logf("64KiB current +64KiB proposed +32 authenticated cases, maximum escaped display names: full trusted presentation=%d bytes", len(presentation[start:]))
}
