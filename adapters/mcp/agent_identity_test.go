package product_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceagent"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Identity personalization is part of the existing owner onboarding journey.
// Both transports exercise the actual closed tools and complete host review.
func assertAgentIdentityJourney(t *testing.T) {
	t.Helper()
	for _, stdio := range []bool{false, true} {
		f := newOwnerFixture(t, stdio)
		assertIdentityApprovalEffect(t, f)
		identityRoot := filepath.Join(filepath.Dir(f.root), "state")
		profilePath := filepath.Join(identityRoot, "agents", "personalization.json")
		allow := true
		reviews := 0
		var duringReview func() error
		var lastReview string
		native := func(_ context.Context, req *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
			reviews++
			lastReview = req.Params.Message
			if duringReview != nil {
				if err := duringReview(); err != nil {
					return nil, err
				}
			}
			if !allow {
				return &mcp.ElicitResult{Action: "decline"}, nil
			}
			return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
		}
		cs := f.connect(t, native)
		call := func(operation string, fields map[string]any, fail bool) map[string]any {
			tool := "maestro_apply"
			if operation == "interview" || operation == "review" {
				tool = "maestro_query"
			}
			args := map[string]any{"operation": "agent.identity." + operation}
			for key, value := range fields {
				args[key] = value
			}
			return contentCall(t, cs, tool, args, fail)
		}
		interview := func() map[string]any { return call("interview", nil, false) }
		answer := func(role, name, emoji string) map[string]any {
			current := interview()
			return map[string]any{"owner_id": "maestro-test", "role": role, "display_name": name, "emoji": emoji, "question_id": "identity-" + role, "expected_profile_revision": current["profile_revision"]}
		}
		got := interview()
		discovery, _ := json.Marshal(got)
		if strings.Contains(string(discovery), "bcgos ") || strings.Contains(string(discovery), "--consent") {
			t.Fatal("MCP identity discovery returned superseded CLI instructions")
		}
		plan := contentCall(t, cs, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "agent-identity-setup"}, false)
		routeRegistered, hostPending, instancePending := false, false, false
		for _, raw := range plan["dependencies"].([]any) {
			dependency := raw.(map[string]any)
			switch dependency["id"] {
			case "domain.agent_identity_registration":
				routeRegistered = dependency["state"] == "available"
			case "host.trusted_review":
				hostPending = dependency["state"] == "unavailable"
			case "domain.account_case_scaffold":
				instancePending = dependency["state"] == "unavailable" && !dependency["required"].(bool)
			}
		}
		if !routeRegistered || !hostPending || !instancePending || plan["execution_state"] != "not_executed" {
			t.Fatal("identity method selection invented host or instance authority", plan)
		}
		next, ok := got["next_question"].(map[string]any)
		if !ok || next["role"] != "maestro" || got["profile_revision"] != "absent" {
			t.Fatal("fresh identity interview was not the canonical first question", got)
		}
		if _, err := os.Lstat(filepath.Join(identityRoot, "agents")); !os.IsNotExist(err) {
			t.Fatal("identity discovery initialized state", err)
		}
		call("answer", answer("darwin", "Off-sequence Health", "🧬"), true)
		malformed := answer("maestro", "Wrong managed id", "🎼")
		malformed["agent_id"] = "other-managed-id"
		call("answer", malformed, true)
		if reviews != 0 {
			t.Fatal("off-sequence or invalid identity reached native review")
		}
		draft := call("propose", answer("maestro", "Synthetic Hub <>&", "🎼"), false)
		if draft["state"] != "drafted" {
			t.Fatal("explicit identity proposal was not retained as an inactive draft", draft)
		}
		var view struct {
			Current  *agentidentity.Profile       `json:"current_profile"`
			Proposed agentidentity.ProfileChoices `json:"proposed_profile"`
		}
		start := strings.Index(lastReview, "{")
		if start < 0 {
			t.Fatal("identity native review has no full content")
		}
		if err := json.Unmarshal([]byte(lastReview[start:]), &view); err != nil || view.Current != nil || len(view.Proposed.Selections) != 1 || view.Proposed.Selections[0].DisplayName != "Synthetic Hub <>&" {
			t.Fatal("identity proposal was not completely reviewed", err)
		}
		if err := cs.Close(); err != nil {
			t.Fatal(err)
		}
		cs = f.connect(t, native)
		got = interview()
		if got["state"] != "review_required" || got["open_draft_id"] != draft["proposal_id"] || got["profile_revision"] != "absent" {
			t.Fatal("identity pending review did not survive restart", got)
		}
		metadata := call("review", map[string]any{"proposal_id": draft["proposal_id"]}, false)
		if metadata["profile"] != nil || metadata["proposed_profile"] != nil || metadata["state"] != "drafted" {
			t.Fatal("ordinary draft query exposed review bodies or lost its lifecycle", metadata)
		}
		allow = false
		if refused := call("apply", map[string]any{"proposal_id": draft["proposal_id"]}, true); refused["code"] != "confirmation_declined" {
			t.Fatal("pending draft decline did not remain explicit", refused)
		}
		allow = true
		if pending := interview(); pending["state"] != "review_required" || pending["open_draft_id"] != draft["proposal_id"] || pending["profile_revision"] != "absent" {
			t.Fatal("declined draft changed or disappeared", pending)
		}
		applied := call("apply", map[string]any{"proposal_id": draft["proposal_id"]}, false)
		if applied["state"] != "applied" {
			t.Fatal("native confirmed draft did not apply", applied)
		}
		replay := call("apply", map[string]any{"proposal_id": draft["proposal_id"]}, false)
		if !reflect.DeepEqual(applied, replay) {
			t.Fatal("identical current applied receipt was not replayable", applied, replay)
		}
		if err := cs.Close(); err != nil {
			t.Fatal(err)
		}
		cs = f.connect(t, native)
		contextReply := contentCall(t, cs, "maestro_context", map[string]any{}, false)
		presentation := contextReply["agent_identity"].(map[string]any)
		if presentation["state"] != "profile_confirmed" || presentation["presentation_only"] != true {
			t.Fatal("restarted ordinary context did not consume reviewed identities", contextReply)
		}
		consumed := false
		for _, raw := range presentation["managed"].([]any) {
			entry := raw.(map[string]any)
			if entry["agent_id"] == "maestro" && entry["role"] == "maestro" && entry["display_name"] == "Synthetic Hub <>&" && entry["emoji"] == "🎼" {
				consumed = true
			}
		}
		if !consumed {
			t.Fatal("ordinary context lost the reviewed display preference or canonical id", presentation)
		}
		got = interview()
		beforeRevision := got["profile_revision"]
		if got["next_question"].(map[string]any)["role"] != "yoda" {
			t.Fatal("identity did not advance one canonical question", got)
		}
		allow = false
		refused := call("answer", answer("yoda", "DECLINED_REVIEWER", "🦉"), true)
		allow = true
		if refused["code"] != "confirmation_declined" || interview()["profile_revision"] != beforeRevision {
			t.Fatal("declined identity answer changed the confirmed profile", refused)
		}
		if err := cs.Close(); err != nil {
			t.Fatal(err)
		}
		cs = f.connect(t, nil)
		refused = call("answer", answer("yoda", "NO_FORM_REVIEWER", "🦉"), true)
		if refused["code"] != "confirmation_unavailable" || interview()["profile_revision"] != beforeRevision {
			t.Fatal("missing native form authorized an identity answer", refused)
		}
		if err := cs.Close(); err != nil {
			t.Fatal(err)
		}
		cs = f.connect(t, native)
		stale := answer("yoda", "STALE_REVIEWER", "🦉")
		duringReview = func() error {
			profile, err := agentidentity.Load(identityRoot)
			if err != nil {
				return err
			}
			profile.Selections[0].DisplayName = "Other client hub"
			profile.UpdatedAt = time.Now().UTC()
			if err := agentidentity.Save(identityRoot, profile); err != nil {
				return fmt.Errorf("identity review held a canonical lock or failed the independent change: %w", err)
			}
			return nil
		}
		refused = call("answer", stale, true)
		duringReview = nil
		if refused["code"] != "revision_conflict" {
			t.Fatal("changed identity review was not fenced", refused)
		}
		got = interview()
		if got["next_question"].(map[string]any)["role"] != "yoda" || identitySelection(got, "maestro")["display_name"] != "Other client hub" {
			t.Fatal("stale review overwrote independent owner work", got)
		}
		beforeCount := reviews
		if got := call("answer", answer("yoda", "Synthetic Reviewer", "🦉"), false); got["state"] != "applied" || reviews != beforeCount+1 {
			t.Fatal("ordinary identity answer was not one exact native form", got)
		}
		if err := cs.Close(); err != nil {
			t.Fatal(err)
		}
		cs = f.connect(t, native)
		got = interview()
		if got["next_question"].(map[string]any)["role"] != "darwin" || identitySelection(got, "yoda")["display_name"] != "Synthetic Reviewer" {
			t.Fatal("restarted identity did not preserve earlier confirmed answers", got)
		}
		beforeCount = reviews
		call("answer", answer("darwin", "Synthetic Health", "🧬"), false)
		got = interview()
		if got["state"] != "complete" || reviews != beforeCount+1 || identitySelection(got, "darwin")["display_name"] != "Synthetic Health" {
			t.Fatal("three-question identity onboarding did not complete", got)
		}
		beforeRevision = got["profile_revision"]
		invalid := answer("maestro", "Invalid emoji", "text")
		call("answer", invalid, true)
		for _, field := range []string{"approved", "consent", "no_client_data", "confirmed", "review_digest", "ownership_scope"} {
			invalid = answer("maestro", "Forged approval", "🎼")
			invalid[field] = true
			call("answer", invalid, true)
		}
		call("answer", stale, true)
		missing := answer("case_agent", "Missing Case", "📁")
		missing["agent_id"] = "missing-case-agent"
		if got := call("answer", missing, true); got["code"] != "identity_instance_unavailable" {
			t.Fatal("missing instance became a global preference", got)
		}
		if interview()["profile_revision"] != beforeRevision {
			t.Fatal("invalid identity request changed canonical preferences")
		}
		ws, err := workspace.Inspect(f.options.WorkspacePath, identityRoot)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := workspaceagent.Initialize(identityRoot, ws.WorkspaceID); err != nil {
			t.Fatal(err)
		}
		request := agentscaffold.WorkspaceRequest(ws.WorkspaceID)
		request.OwnerID = "maestro-test"
		registration, err := agentscaffold.Scaffold(identityRoot, request)
		if err != nil {
			t.Fatal(err)
		}
		registered := answer("case_agent", "Synthetic Case", "📁")
		registered["agent_id"] = request.AgentID
		call("answer", registered, false)
		afterRegistration, err := agentscaffold.Inspect(identityRoot, request.AgentID)
		if err != nil || !reflect.DeepEqual(registration.Instance, afterRegistration.Instance) {
			t.Fatal("presentation preference changed signed instance authority", err)
		}
		if got := interview(); identitySelection(got, "case_agent")["display_name"] != "Synthetic Case" {
			t.Fatal("registered instance preference was not persisted", got)
		}
		beforeRevision = interview()["profile_revision"]
		other, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(filepath.Dir(f.options.WorkspacePath), "other-workspace"), DataRoot: identityRoot})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := workspaceagent.Initialize(identityRoot, other.WorkspaceID); err != nil {
			t.Fatal(err)
		}
		foreign := agentscaffold.WorkspaceRequest(other.WorkspaceID)
		foreign.OwnerID = "maestro-test"
		if _, err := agentscaffold.Scaffold(identityRoot, foreign); err != nil {
			t.Fatal(err)
		}
		missing = answer("case_agent", "Foreign Case", "📁")
		missing["agent_id"] = foreign.AgentID
		if got := call("answer", missing, true); got["code"] != "identity_instance_unavailable" {
			t.Fatal("foreign workspace instance was admitted", got)
		}
		if interview()["profile_revision"] != beforeRevision {
			t.Fatal("foreign instance changed current preferences")
		}
		// Preserve an existing maximum-size valid personalization profile and
		// review every current/proposed choice, including JSON-expanding names.
		large, err := agentidentity.Load(identityRoot)
		if err != nil {
			t.Fatal(err)
		}
		for len(large.Selections) < 128 {
			large.Selections = append(large.Selections, agentidentity.Selection{Role: "case_agent", AgentID: fmt.Sprintf("legacy-display-%03d", len(large.Selections)), DisplayName: strings.Repeat("&", 80), Emoji: "📁", OwnerID: "maestro-test", OwnershipScope: "case"})
		}
		large.UpdatedAt = time.Now().UTC()
		if err := agentidentity.Save(identityRoot, large); err != nil {
			t.Fatal(err)
		}
		call("answer", answer("maestro", "Reviewed large profile", "🎼"), false)
		start = strings.Index(lastReview, "{")
		if start < 0 {
			t.Fatal("large identity review missing")
		}
		if err := json.Unmarshal([]byte(lastReview[start:]), &view); err != nil || view.Current == nil || len(view.Current.Selections) != 128 || len(view.Proposed.Selections) != 128 {
			t.Fatal("large identity native review truncated canonical choices", err)
		}
		t.Logf("full identity native review: selections=128 message_bytes=%d", len(lastReview))
		for _, selection := range large.Selections {
			if selection.Role == "maestro" {
				continue
			}
			seenCurrent, seenProposed := false, false
			for _, shown := range view.Current.Selections {
				if shown == selection {
					seenCurrent = true
				}
			}
			for _, shown := range view.Proposed.Selections {
				if shown == selection {
					seenProposed = true
				}
			}
			if !seenCurrent || !seenProposed {
				t.Fatal("large profile review changed or omitted an untouched choice")
			}
		}
		outside := filepath.Join(filepath.Dir(identityRoot), "identity-hardlink.json")
		original, err := os.ReadFile(profilePath)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Link(profilePath, outside); err != nil {
			t.Fatal("create ordinary-account identity hardlink fixture", err)
		}
		if got := call("interview", nil, true); got["code"] != "identity_state_invalid" {
			t.Fatal("aliased personalization was not blocked", got)
		}
		if body, err := os.ReadFile(outside); err != nil || string(body) != string(original) {
			t.Fatal("alias refusal modified outside bytes", err)
		}
		if err := os.Remove(outside); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(profilePath, []byte("{invalid"), 0600); err != nil {
			t.Fatal(err)
		}
		if got := call("interview", nil, true); got["code"] != "identity_state_invalid" {
			t.Fatal("corrupt profile became a fresh interview", got)
		}
		if guided := agentidentity.GuidedIdentityInterview(identityRoot); guided.State != "blocked" {
			t.Fatal("canonical identity owner treated corrupt state as absence", guided)
		}
		contextReply = contentCall(t, cs, "maestro_context", map[string]any{}, false)
		presentation = contextReply["agent_identity"].(map[string]any)
		if presentation["state"] != "profile_invalid" || contextReply["state"] != "ready" {
			t.Fatal("invalid optional identity was hidden as fresh or blocked unrelated workspace context", contextReply)
		}
	}
}

func identitySelection(interview map[string]any, role string) map[string]any {
	if profile, ok := interview["profile"].(map[string]any); ok {
		if selections, ok := profile["selections"].([]any); ok {
			for _, raw := range selections {
				selection := raw.(map[string]any)
				if selection["role"] == role {
					return selection
				}
			}
		}
	}
	return map[string]any{}
}

// The modern public protocol must bind approval to the effect, so consent to
// retain an inactive draft cannot be transplanted into profile publication.
func assertIdentityApprovalEffect(t *testing.T, f ownerFixture) {
	t.Helper()
	f.manualInputResponses = true
	cs := f.connectProtocol(t, nil, "2026-07-28")
	defer cs.Close()
	args := map[string]any{"operation": "agent.identity.propose", "owner_id": "maestro-test", "role": "maestro", "display_name": "Draft Only Hub", "emoji": "🎼", "question_id": "identity-maestro", "expected_profile_revision": "absent"}
	meta := mcp.Meta{mcp.MetaKeyClientCapabilities: map[string]any{"elicitation": map[string]any{"form": map[string]any{}}}}
	first, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: args, Meta: meta})
	if err != nil || first.IsError || len(first.InputRequests) != 1 {
		t.Fatal("modern identity draft did not request its exact native review", first, err)
	}
	key := ""
	for id, request := range first.InputRequests {
		key = id
		if form, ok := request.(*mcp.ElicitParams); !ok || !strings.Contains(form.Message, "draft only") {
			t.Fatal("first native review did not delimit the inactive-draft effect")
		}
	}
	args["operation"] = "agent.identity.answer"
	out, err := cs.CallTool(context.Background(), &mcp.CallToolParams{Name: "maestro_apply", Arguments: args, Meta: meta, InputResponses: mcp.InputResponseMap{key: &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !out.IsError {
		t.Fatal("draft-only native approval authorized the broader profile-publication effect")
	}
	var failure map[string]any
	if len(out.Content) != 1 {
		t.Fatal("effect-switch refusal has no typed result")
	}
	if err := json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &failure); err != nil || failure["code"] != "revision_conflict" {
		t.Fatal("effect-switch approval was not rejected as a changed review", failure, err)
	}
	current := contentCall(t, cs, "maestro_query", map[string]any{"operation": "agent.identity.interview"}, false)
	if current["profile_revision"] != "absent" || current["state"] != "action_required" {
		t.Fatal("rejected effect-switch created canonical identity authority", current)
	}
}
