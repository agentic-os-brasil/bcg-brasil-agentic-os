package agentscaffold

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentidentity"
)

func TestWorkspaceScaffoldIsConcreteDataFreeAndIdempotent(t *testing.T) {
	root := t.TempDir()
	initializeWorkspaceScope(t, root, "ws-alpha")
	request := WorkspaceRequest("ws-alpha")
	first, err := Scaffold(root, request)
	if err != nil {
		t.Fatal(err)
	}
	if !first.Initialized || first.Existing ||
		first.Instance.AgentID != "workspace-agent-ws-alpha" ||
		first.Instance.InputContract != "bounded_case_packet" ||
		first.Instance.ToolAccess != "scoped" || first.Instance.MayDelegate ||
		first.Instance.RuntimeState != "unavailable" {
		t.Fatalf("unexpected workspace scaffold: %#v", first)
	}
	definition, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(first.Definition.Path)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(definition), "ws-alpha") ||
		!strings.Contains(string(definition), "context gatekeeper") {
		t.Fatal("managed workspace template is missing its role or contains instance data")
	}
	second, err := Scaffold(root, request)
	if err != nil {
		t.Fatal(err)
	}
	if !second.Existing || second.Instance.CreatedAt != first.Instance.CreatedAt {
		t.Fatalf("idempotent scaffold changed identity: %#v", second)
	}
}

func TestScaffoldUsesConfirmedAgentPersonalizationWithoutChangingAuthority(t *testing.T) {
	root := t.TempDir()
	initializeWorkspaceScope(t, root, "ws-personalized")
	if err := agentidentity.Save(root, agentidentity.Profile{
		SchemaVersion: 1, OwnerID: "daniel", Confirmed: true, UpdatedAt: time.Now().UTC(),
		Selections: []agentidentity.Selection{{Role: "case_agent", AgentID: "workspace-agent-ws-personalized", DisplayName: "Forge", Emoji: "⚙️", OwnerID: "daniel", OwnershipScope: "case"}},
	}); err != nil {
		t.Fatal(err)
	}
	status, err := Scaffold(root, WorkspaceRequest("ws-personalized"))
	if err != nil {
		t.Fatal(err)
	}
	if status.Instance.DisplayName != "Forge" || status.Instance.Emoji != "⚙️" || status.Instance.OwnerID != "daniel" ||
		status.Instance.OwnershipScope != "case" || status.Instance.RuntimeState != "unavailable" {
		t.Fatalf("unexpected personalized scaffold: %#v", status.Instance)
	}
}

func TestScaffoldCreatesAccountAndRejectsAgentToAgentChild(t *testing.T) {
	root := t.TempDir()
	initializeWorkspaceScope(t, root, "ws-alpha")
	if _, err := Scaffold(root, WorkspaceRequest("ws-alpha")); err != nil {
		t.Fatal(err)
	}
	account := Request{
		AgentID: "account-agent-client-alpha", Role: "account_agent",
		ScopeKind: "account", ScopeID: "client-alpha",
		ParentAgent: "maestro", ParentRole: "hub",
		Owner: "account-owner", Mandate: "Maintain curated client-level context.",
	}
	if _, err := Scaffold(root, account); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(root, Request{
		AgentID: "retired-research", Role: "retired_specialist_role",
		ScopeKind: "account", ScopeID: "client-alpha",
		ParentAgent: account.AgentID, ParentRole: "client_account_agent",
	}); err == nil {
		t.Fatal("Client Account Agent unexpectedly delegated a case capability directly")
	}

}

func TestScaffoldHiresClientAccountCaseAndVersionedPAExpert(t *testing.T) {
	root := t.TempDir()
	account := Request{
		AgentID: "client-account-agent-client-alpha", Role: "client_account_agent",
		ScopeKind: "account", ScopeID: "client-alpha",
		ParentAgent: "maestro", ParentRole: "hub",
		Owner: "account-owner", Mandate: "Act as the Partner-like account intelligence owner.",
	}
	accountStatus, err := Scaffold(root, account)
	if err != nil {
		t.Fatal(err)
	}
	if accountStatus.Instance.InputContract != "bounded_client_account_packet" ||
		accountStatus.Instance.MayDelegate {
		t.Fatalf("unexpected Client Account Agent: %#v", accountStatus.Instance)
	}

	caseRequest := Request{
		AgentID: "case-agent-transformation", Role: "case_agent",
		ScopeKind: "case", ScopeID: "transformation",
		ParentAgent: "maestro", ParentRole: "hub", AccountAgentID: account.AgentID,
	}
	caseStatus, err := Scaffold(root, caseRequest)
	if err != nil {
		t.Fatal(err)
	}
	if caseStatus.Instance.InputContract != "bounded_case_packet" ||
		caseStatus.Instance.MayDelegate {
		t.Fatalf("unexpected Case Agent: %#v", caseStatus.Instance)
	}

	canonPath, canonSHA256 := preparePAExpertCanon(t, root, "pa-expert-fpa-pricing")
	expert := Request{
		AgentID: "pa-expert-fpa-pricing", Role: "pa_expert",
		ScopeKind: "practice", ScopeID: "pricing",
		ParentAgent: "maestro", ParentRole: "hub",
		Owner: "pa-expert-curator", Mandate: "Advise cases with the maintained pricing canon.",
		CanonPath: canonPath, CanonSHA256: canonSHA256,
		ExpertKind: "FPA", ExpertVersion: "1.0.0", ExpertLifecycle: "draft",
	}
	expertStatus, err := Scaffold(root, expert)
	if err != nil {
		t.Fatal(err)
	}
	if expertStatus.Instance.InputContract != "bounded_advisory_packet" ||
		expertStatus.Instance.ToolAccess != "none" ||
		expertStatus.Instance.ExpertKind != "FPA" ||
		expertStatus.Instance.ExpertVersion != "1.0.0" ||
		expertStatus.Instance.ExpertLifecycle != "draft" {
		t.Fatalf("unexpected PA expert: %#v", expertStatus.Instance)
	}
}

func TestPAExpertHireRejectsMissingVersionAndChangedCanon(t *testing.T) {
	root := t.TempDir()
	canonPath, canonSHA256 := preparePAExpertCanon(t, root, "pa-expert-ipa-insurance")
	request := Request{
		AgentID: "pa-expert-ipa-insurance", Role: "pa_expert",
		ScopeKind: "practice", ScopeID: "insurance",
		ParentAgent: "maestro", ParentRole: "hub",
		Owner: "pa-expert-curator", Mandate: "Advise cases with the maintained insurance canon.",
		CanonPath: canonPath, CanonSHA256: canonSHA256, ExpertKind: "IPA", ExpertLifecycle: "draft",
	}
	if _, err := Scaffold(root, request); err == nil {
		t.Fatal("unversioned PA expert was hired")
	}
	request.ExpertVersion = "1.0.0"
	if _, err := Scaffold(root, request); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(canonPath)), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(root, request.AgentID); err == nil {
		t.Fatal("PA expert with changed canon remained valid")
	}
}

func TestPAExpertRejectsLegacyCanonNamespace(t *testing.T) {
	root := t.TempDir()
	_, err := Scaffold(root, Request{
		AgentID: "pa-expert-fpa-pricing", Role: "pa_expert",
		ScopeKind: "practice", ScopeID: "pricing",
		ParentAgent: "maestro", ParentRole: "hub",
		Owner: "pa-expert-curator", Mandate: "Advise with the maintained pricing canon.",
		CanonPath:   "legacy-pa-experts/pa-expert-fpa-pricing/canon.md",
		CanonSHA256: strings.Repeat("a", 64), ExpertKind: "FPA",
		ExpertVersion: "1.0.0", ExpertLifecycle: "draft",
	})
	if err == nil {
		t.Fatal("legacy PA Expert canon namespace was accepted")
	}
}

func TestPAExpertRejectsCanonSymlinkOutsideRegistryRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture requires non-Windows test privileges")
	}
	root := t.TempDir()
	canonPath, canonSHA256 := preparePAExpertCanon(t, root, "pa-expert-fpa-pricing")
	outside := filepath.Join(t.TempDir(), "outside.md")
	if err := os.WriteFile(outside, []byte("# outside canon\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, filepath.FromSlash(canonPath))); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, filepath.FromSlash(canonPath))); err != nil {
		t.Fatal(err)
	}
	_, err := Scaffold(root, Request{
		AgentID: "pa-expert-fpa-pricing", Role: "pa_expert",
		ScopeKind: "practice", ScopeID: "pricing",
		ParentAgent: "maestro", ParentRole: "hub",
		Owner: "pa-expert-curator", Mandate: "Advise with the maintained pricing canon.",
		CanonPath: canonPath, CanonSHA256: canonSHA256,
		ExpertKind: "FPA", ExpertVersion: "1.0.0", ExpertLifecycle: "draft",
	})
	if err == nil {
		t.Fatal("PA Expert canon symlink escaped its registry root")
	}
}

func TestScaffoldRejectsUngovernedRolesEdgesAndScopeReuse(t *testing.T) {
	root := t.TempDir()
	tests := []Request{
		{
			AgentID: "account-agent-client-alpha", Role: "account_agent",
			ScopeKind: "account", ScopeID: "client-alpha",
			ParentAgent: "maestro", ParentRole: "hub",
		},
		{
			AgentID: "general-helper", Role: "errand_helper",
			ScopeKind: "workspace", ScopeID: "ws-alpha",
			ParentAgent: "maestro", ParentRole: "hub",
		},
		{
			AgentID: "subject-insurance", Role: "subject_specialist",
			ScopeKind: "workspace", ScopeID: "ws-alpha",
			ParentAgent: "workspace-agent-ws-alpha", ParentRole: "workspace_agent",
		},
		{
			AgentID: "retired-research", Role: "retired_specialist_role",
			ScopeKind: "workspace", ScopeID: "ws-alpha",
			ParentAgent: "practice-insurance", ParentRole: "practice_agent",
		},
		{
			AgentID: "../retired-research", Role: "retired_specialist_role",
			ScopeKind: "workspace", ScopeID: "ws-alpha",
			ParentAgent: "workspace-agent-ws-alpha", ParentRole: "workspace_agent",
		},
	}
	for _, request := range tests {
		if _, err := Scaffold(root, request); err == nil {
			t.Fatalf("ungoverned scaffold accepted: %#v", request)
		}
	}
}

func TestScaffoldRejectsRetiredPracticeRolesAndIDs(t *testing.T) {
	root := t.TempDir()
	for _, request := range []Request{
		{AgentID: "practice-agent-insurance", Role: "practice_agent", ScopeKind: "practice", ScopeID: "insurance", ParentAgent: "maestro", ParentRole: "hub"},
		{AgentID: "subject-insurance", Role: "subject_specialist", ScopeKind: "practice", ScopeID: "insurance", ParentAgent: "maestro", ParentRole: "hub"},
		{AgentID: "practice-agent-insurance", Role: "pa_expert", ScopeKind: "practice", ScopeID: "insurance", ParentAgent: "maestro", ParentRole: "hub"},
	} {
		if _, err := Scaffold(root, request); err == nil {
			t.Fatalf("retired practice registration was accepted: %#v", request)
		}
	}
}

func TestScaffoldDoesNotOverwriteTamperedDefinitionOrRegistration(t *testing.T) {
	root := t.TempDir()
	initializeWorkspaceScope(t, root, "ws-alpha")
	request := WorkspaceRequest("ws-alpha")
	status, err := Scaffold(root, request)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, filepath.FromSlash(status.Definition.Path))
	if err := os.WriteFile(path, []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(root, request); err == nil {
		t.Fatal("tampered definition was silently overwritten")
	}
	if body, err := os.ReadFile(path); err != nil || string(body) != "tampered\n" {
		t.Fatalf("tampered user-local file was changed: %q, %v", body, err)
	}
}

func TestScaffoldRejectsCoordinatedManifestAndStateTampering(t *testing.T) {
	root := t.TempDir()
	initializeWorkspaceScope(t, root, "ws-alpha")
	request := WorkspaceRequest("ws-alpha")
	status, err := Scaffold(root, request)
	if err != nil {
		t.Fatal(err)
	}
	instancePath := filepath.Join(root, "agents", "instances", request.AgentID, "instance.json")
	body, err := os.ReadFile(instancePath)
	if err != nil {
		t.Fatal(err)
	}
	var instance Instance
	if err := json.Unmarshal(body, &instance); err != nil {
		t.Fatal(err)
	}
	instance.ScopeID = "ws-beta"
	instance.AgentID = "workspace-agent-ws-beta"
	tampered, err := json.MarshalIndent(instance, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(instancePath, append(tampered, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(root, request.AgentID); err == nil {
		t.Fatal("manifest tampering bypassed the installation integrity signature")
	}

	root = t.TempDir()
	initializeWorkspaceScope(t, root, "ws-alpha")
	status, err = Scaffold(root, request)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(root, filepath.FromSlash(status.State.Path))
	stateBody, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	stateBody = []byte(strings.Replace(string(stateBody), `"runtime_state": "unavailable"`, `"runtime_state": "available"`, 1))
	if err := os.WriteFile(statePath, stateBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Inspect(root, request.AgentID); err == nil {
		t.Fatal("state tampering bypassed the signed state digest")
	}
}

func TestScaffoldRejectsSameIDWithDifferentImmutableScope(t *testing.T) {
	root := t.TempDir()
	initializeWorkspaceScope(t, root, "ws-alpha")
	initializeWorkspaceScope(t, root, "ws-beta")
	if _, err := Scaffold(root, WorkspaceRequest("ws-alpha")); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(root, WorkspaceRequest("ws-beta")); err != nil {
		t.Fatal(err)
	}
	request := Request{
		AgentID: "client-account-agent-client-alpha", Role: "client_account_agent",
		ScopeKind: "account", ScopeID: "client-alpha",
		ParentAgent: "maestro", ParentRole: "hub", Owner: "account-owner", Mandate: "Maintain bounded account context.",
	}
	if _, err := Scaffold(root, request); err != nil {
		t.Fatal(err)
	}
	request.ScopeID = "client-beta"
	if _, err := Scaffold(root, request); err == nil {
		t.Fatal("same specialist ID was rebound to another workspace")
	}
}

func TestScaffoldRejectsUninitializedWorkspaceScope(t *testing.T) {
	if _, err := Scaffold(t.TempDir(), WorkspaceRequest("ws-missing")); err == nil {
		t.Fatal("workspace agent was scaffolded without initialized workspace state")
	}
}

func TestScaffoldRejectsOrphanAccountAndSubjectSpecialists(t *testing.T) {
	root := t.TempDir()
	requests := []Request{
		{
			AgentID: "retired-account-research", Role: "retired_specialist_role",
			ScopeKind: "account", ScopeID: "client-alpha",
			ParentAgent: "account-agent-client-alpha", ParentRole: "account_agent",
		},
		{
			AgentID: "subject-insurance", Role: "subject_specialist",
			ScopeKind: "practice", ScopeID: "insurance",
			ParentAgent: "practice-agent-insurance", ParentRole: "practice_agent",
		},
	}
	for _, request := range requests {
		if _, err := Scaffold(root, request); err == nil {
			t.Fatalf("orphan specialist accepted: %#v", request)
		}
	}
}

func TestScaffoldRefusesSilentIntegrityKeyReplacement(t *testing.T) {
	root := t.TempDir()
	initializeWorkspaceScope(t, root, "ws-alpha")
	initializeWorkspaceScope(t, root, "ws-beta")
	if _, err := Scaffold(root, WorkspaceRequest("ws-alpha")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(root, "config", "agent-scaffold-integrity.key")); err != nil {
		t.Fatal(err)
	}
	if _, err := Scaffold(root, WorkspaceRequest("ws-beta")); err == nil ||
		!strings.Contains(err.Error(), "explicit recovery") {
		t.Fatalf("missing installation key did not fail closed: %v", err)
	}
}

func initializeWorkspaceScope(t *testing.T, root, workspaceID string) {
	t.Helper()
	directory := filepath.Join(root, "workspaces", workspaceID, "agent")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"schema_version":1,"workspace_id":"` + workspaceID + `","agent_id":"workspace-agent-` + workspaceID + `"}` + "\n")
	if err := os.WriteFile(filepath.Join(directory, "agent.json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
}

func preparePAExpertCanon(t *testing.T, root, expertID string) (string, string) {
	t.Helper()
	relative := filepath.Join("pa-experts", expertID, "canon.md")
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("# Governed PA Expert canon\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	return filepath.ToSlash(relative), hex.EncodeToString(digest[:])
}
