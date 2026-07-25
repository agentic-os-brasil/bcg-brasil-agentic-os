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
		first.Instance.InputContract != "bounded_workspace_packet" ||
		first.Instance.ToolAccess != "scoped" || !first.Instance.MayDelegate ||
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

func TestScaffoldCreatesWorkspaceAccountAndPracticeSpecialistChains(t *testing.T) {
	root := t.TempDir()
	initializeWorkspaceScope(t, root, "ws-alpha")
	if _, err := Scaffold(root, WorkspaceRequest("ws-alpha")); err != nil {
		t.Fatal(err)
	}
	request := Request{
		AgentID: "capability-research", Role: "capability_specialist",
		ScopeKind: "workspace", ScopeID: "ws-alpha",
		ParentAgent: "workspace-agent-ws-alpha", ParentRole: "workspace_agent",
	}
	status, err := Scaffold(root, request)
	if err != nil {
		t.Fatalf("Scaffold(%s): %v", request.AgentID, err)
	}
	if status.Instance.MayDelegate || status.Instance.ToolAccess != "scoped" ||
		status.Instance.ParentAgentID != request.ParentAgent ||
		status.Instance.ScopeID != request.ScopeID {
		t.Fatalf("unexpected specialist scaffold: %#v", status)
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
	accountCapability := Request{
		AgentID: "capability-account-research", Role: "capability_specialist",
		ScopeKind: "account", ScopeID: "client-alpha",
		ParentAgent: account.AgentID, ParentRole: "account_agent",
	}
	if _, err := Scaffold(root, accountCapability); err != nil {
		t.Fatal(err)
	}

	canonPath, canonSHA256 := preparePracticeCanon(t, root, "insurance")
	practice := Request{
		AgentID: "practice-agent-insurance", Role: "practice_agent",
		ScopeKind: "practice", ScopeID: "insurance",
		ParentAgent: "maestro", ParentRole: "hub",
		Owner: "practice-owner", Mandate: "Maintain the bounded insurance canon.",
		CanonPath: canonPath, CanonSHA256: canonSHA256,
	}
	if _, err := Scaffold(root, practice); err != nil {
		t.Fatal(err)
	}
	subject := Request{
		AgentID: "subject-insurance", Role: "subject_specialist",
		ScopeKind: "practice", ScopeID: "insurance",
		ParentAgent: practice.AgentID, ParentRole: "practice_agent",
	}
	if _, err := Scaffold(root, subject); err != nil {
		t.Fatal(err)
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
			AgentID: "capability-research", Role: "capability_specialist",
			ScopeKind: "workspace", ScopeID: "ws-alpha",
			ParentAgent: "practice-insurance", ParentRole: "practice_agent",
		},
		{
			AgentID: "../capability-research", Role: "capability_specialist",
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

func TestSubjectScaffoldRejectsChangedPracticeCanon(t *testing.T) {
	root := t.TempDir()
	canonPath, canonSHA256 := preparePracticeCanon(t, root, "insurance")
	practice := Request{
		AgentID: "practice-agent-insurance", Role: "practice_agent",
		ScopeKind: "practice", ScopeID: "insurance",
		ParentAgent: "maestro", ParentRole: "hub",
		Owner: "practice-owner", Mandate: "Maintain the bounded insurance canon.",
		CanonPath: canonPath, CanonSHA256: canonSHA256,
	}
	if _, err := Scaffold(root, practice); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(canonPath)), []byte("changed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	subject := Request{
		AgentID: "subject-insurance", Role: "subject_specialist",
		ScopeKind: "practice", ScopeID: "insurance",
		ParentAgent: practice.AgentID, ParentRole: "practice_agent",
	}
	if _, err := Scaffold(root, subject); err == nil {
		t.Fatal("subject specialist accepted a parent with changed canon bytes")
	}
}

func TestPracticeScaffoldRejectsCanonSymlinkOutsidePractice(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows symlink creation requires host-specific privileges")
	}
	root := t.TempDir()
	outside := filepath.Join(root, "outside-canon.md")
	body := []byte("# Outside canon\n")
	if err := os.WriteFile(outside, body, 0o600); err != nil {
		t.Fatal(err)
	}
	practiceDirectory := filepath.Join(root, "practices", "insurance")
	if err := os.MkdirAll(practiceDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	canonPath := filepath.Join(practiceDirectory, "canon.md")
	if err := os.Symlink(outside, canonPath); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	request := Request{
		AgentID: "practice-agent-insurance", Role: "practice_agent",
		ScopeKind: "practice", ScopeID: "insurance",
		ParentAgent: "maestro", ParentRole: "hub",
		Owner: "practice-owner", Mandate: "Maintain the bounded insurance canon.",
		CanonPath:   "practices/insurance/canon.md",
		CanonSHA256: hex.EncodeToString(digest[:]),
	}
	if _, err := Scaffold(root, request); err == nil {
		t.Fatal("practice canon escaped its scope through a symlink")
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
		AgentID: "capability-research", Role: "capability_specialist",
		ScopeKind: "workspace", ScopeID: "ws-alpha",
		ParentAgent: "workspace-agent-ws-alpha", ParentRole: "workspace_agent",
	}
	if _, err := Scaffold(root, request); err != nil {
		t.Fatal(err)
	}
	request.ScopeID = "ws-beta"
	request.ParentAgent = "workspace-agent-ws-beta"
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
			AgentID: "capability-account-research", Role: "capability_specialist",
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

func preparePracticeCanon(t *testing.T, root, practiceID string) (string, string) {
	t.Helper()
	relative := filepath.Join("practices", practiceID, "canon.md")
	path := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("# Governed practice canon\n")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(body)
	return filepath.ToSlash(relative), hex.EncodeToString(digest[:])
}
