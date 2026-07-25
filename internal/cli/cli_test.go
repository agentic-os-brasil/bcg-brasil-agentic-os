package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMemoryCaptureStatusAndContextCommands(t *testing.T) {
	dataDir := t.TempDir()
	var output bytes.Buffer
	code := RunWithInput([]string{
		"memory", "capture",
		"--data-dir", dataDir,
		"--workspace", "case-a",
		"--kind", "decision",
		"--stdin",
		"--sanitized",
	}, strings.NewReader("sanitized evidence"), &output, &output)
	if code != 0 {
		t.Fatalf("capture exit = %d, output = %s", code, output.String())
	}
	capturePath := filepath.Join(dataDir, "memory", "workspaces", "case-a", "l1", "captures")
	entries, err := os.ReadDir(capturePath)
	if err != nil || len(entries) != 1 {
		t.Fatalf("capture files = %v, err = %v", entries, err)
	}

	output.Reset()
	code = Run([]string{"memory", "status", "--data-dir", dataDir, "--workspace", "case-a"}, &output, &output)
	if code != 0 || !strings.Contains(output.String(), `"state": "captured"`) || !strings.Contains(output.String(), `"dreaming": "unavailable"`) {
		t.Fatalf("status exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	code = Run([]string{
		"memory", "context",
		"--data-dir", dataDir,
		"--workspace", "case-a",
		"--budget-l1", "100",
		"--budget-l2", "100",
		"--budget-l3", "100",
		"--budget-lifetime", "100",
	}, &output, &output)
	if code != 0 {
		t.Fatalf("context exit = %d, output = %s", code, output.String())
	}
	var bundle map[string]any
	if err := json.Unmarshal(output.Bytes(), &bundle); err != nil {
		t.Fatalf("context JSON: %v", err)
	}
	if diagnostics, ok := bundle["diagnostics"].([]any); !ok || len(diagnostics) != 4 {
		t.Fatalf("context diagnostics = %#v", bundle)
	}
}

func TestMemoryCaptureFailsClosedWithoutSanitizedAttestation(t *testing.T) {
	var output bytes.Buffer
	code := RunWithInput([]string{"memory", "capture", "--data-dir", t.TempDir(), "--workspace", "case-a", "--kind", "note", "--stdin"}, strings.NewReader("raw"), &output, &output)
	if code == 0 || !strings.Contains(output.String(), "--sanitized") {
		t.Fatalf("capture exit = %d, output = %s", code, output.String())
	}
}

func TestMemoryCaptureRejectsOversizedInput(t *testing.T) {
	dataDir := t.TempDir()
	var output bytes.Buffer
	code := RunWithInput([]string{
		"memory", "capture", "--data-dir", dataDir, "--workspace", "case-a", "--kind", "note", "--stdin", "--sanitized",
	}, strings.NewReader(strings.Repeat("x", (1<<20)+1)), &output, &output)
	if code == 0 || !strings.Contains(output.String(), "1 MiB") {
		t.Fatalf("capture exit = %d, output = %s", code, output.String())
	}
	if _, err := os.Stat(filepath.Join(dataDir, "memory", "workspaces", "case-a")); !os.IsNotExist(err) {
		t.Fatalf("oversized capture created workspace: %v", err)
	}
}

func TestMemoryContextRequiresEveryBudget(t *testing.T) {
	var output bytes.Buffer
	code := Run([]string{"memory", "context", "--data-dir", t.TempDir(), "--workspace", "case-a"}, &output, &output)
	if code == 0 || !strings.Contains(output.String(), "positive budget required") {
		t.Fatalf("context exit = %d, output = %s", code, output.String())
	}
}

func TestMemoryCommandsRejectResidualPositionals(t *testing.T) {
	dataDir := t.TempDir()
	tests := []struct {
		name string
		args []string
	}{
		{name: "capture", args: []string{"memory", "capture", "--data-dir", dataDir, "--workspace", "case-a", "--kind", "note", "--stdin", "--sanitized", "SECRET"}},
		{name: "status", args: []string{"memory", "status", "--data-dir", dataDir, "--workspace", "case-a", "typo"}},
		{name: "context", args: []string{"memory", "context", "--data-dir", dataDir, "--workspace", "case-a", "--budget-l1", "1", "--budget-l2", "1", "--budget-l3", "1", "--budget-lifetime", "1", "typo"}},
		{name: "dream", args: []string{"memory", "dream", "daily", "--data-dir", dataDir, "--workspace", "case-a", "typo"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var output bytes.Buffer
			code := RunWithInput(test.args, strings.NewReader("safe"), &output, &output)
			if code == 0 || !strings.Contains(output.String(), "unexpected positional argument") {
				t.Fatalf("exit = %d, output = %s", code, output.String())
			}
		})
	}
	if _, err := os.Stat(filepath.Join(dataDir, "memory", "workspaces", "case-a")); !os.IsNotExist(err) {
		t.Fatalf("rejected capture created workspace: %v", err)
	}
}

func TestMemoryCLIReportsAllInvalidCommitsAsCorrupt(t *testing.T) {
	dataDir := t.TempDir()
	commits := filepath.Join(dataDir, "memory", "workspaces", "case-a", "commits")
	if err := os.MkdirAll(commits, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(commits, "20260720T120000.000000000Z-corrupt.json"), []byte(`{"broken":true}`), 0o600); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if code := Run([]string{"memory", "status", "--data-dir", dataDir, "--workspace", "case-a"}, &output, &output); code != 0 || !strings.Contains(output.String(), `"state": "corrupt"`) {
		t.Fatalf("status exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	code := Run([]string{"memory", "context", "--data-dir", dataDir, "--workspace", "case-a", "--budget-l1", "1", "--budget-l2", "1", "--budget-l3", "1", "--budget-lifetime", "1"}, &output, &output)
	if code == 0 || !strings.Contains(output.String(), `"state":"error"`) || !strings.Contains(output.String(), "none is valid") {
		t.Fatalf("context exit = %d, output = %s", code, output.String())
	}
}

func TestMemoryDreamReportsUnavailableWithoutAdapter(t *testing.T) {
	var output bytes.Buffer
	code := Run([]string{"memory", "dream", "daily", "--data-dir", t.TempDir(), "--workspace", "case-a"}, &output, &output)
	if code != ExitUnavailable || !strings.Contains(output.String(), `"capability": "memory_dreaming"`) || !strings.Contains(output.String(), `"state": "unavailable"`) {
		t.Fatalf("dream exit = %d, output = %s", code, output.String())
	}
}

func TestVersionCommand(t *testing.T) {
	var output bytes.Buffer
	if code := Run([]string{"version"}, &output, &output); code != 0 || !strings.Contains(output.String(), "bcgos") {
		t.Fatalf("version exit = %d, output = %s", code, output.String())
	}
}

func TestPrivateReleaseCommandsFailClosedWithoutApprovedSecureStore(t *testing.T) {
	tests := map[string][]string{
		"auth status":    {"auth", "status"},
		"auth login":     {"auth", "login"},
		"auth logout":    {"auth", "logout"},
		"update check":   {"update", "--check"},
		"update confirm": {"update", "--confirm", "0123456789abcdef0123456789abcdef"},
	}
	for name, args := range tests {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			var errorOutput bytes.Buffer
			if code := Run(args, &output, &errorOutput); code != ExitUnavailable {
				t.Fatalf("Run(%v) exit = %d, want %d; out=%s err=%s", args, code, ExitUnavailable, output.String(), errorOutput.String())
			}
			var result struct {
				SchemaVersion int    `json:"schema_version"`
				State         string `json:"state"`
				Reason        string `json:"reason"`
			}
			if err := json.Unmarshal(output.Bytes(), &result); err != nil {
				t.Fatalf("output is not one JSON document: %v; output=%q", err, output.String())
			}
			if result.SchemaVersion != 1 || result.State != "unavailable" || result.Reason == "" {
				t.Fatalf("unexpected fail-closed result: %#v", result)
			}
			if strings.Contains(strings.ToLower(output.String()), "token") {
				t.Fatal("unavailable response mentioned credential material")
			}
		})
	}
}

func TestSkillsIndexCommandExposesManagedPointers(t *testing.T) {
	var output bytes.Buffer
	if code := Run([]string{"skills", "index"}, &output, &output); code != ExitOK || !strings.Contains(output.String(), `"schema_version": 1`) || !strings.Contains(output.String(), `"dream-memory"`) || strings.Contains(output.String(), "Daily dreaming cannot") {
		t.Fatalf("skills index exit = %d, output = %s", code, output.String())
	}
}

func TestProfileCommandsSwitchTheCanonicalUserPreference(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	var output bytes.Buffer
	if code := runProfile([]string{"show"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"profile": "standard"`) {
		t.Fatalf("profile show exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	if code := runProfile([]string{"set", "advanced"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"profile": "advanced"`) || !strings.Contains(output.String(), `"source": "configured"`) {
		t.Fatalf("profile set exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	if code := runProfile([]string{"show"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"profile": "advanced"`) {
		t.Fatalf("profile persisted exit = %d, output = %s", code, output.String())
	}
}

func TestOwnerCommandsExposeFacetsAndColdStartInterview(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	var output bytes.Buffer
	if code := runOwner([]string{"init"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"voice"`) || !strings.Contains(output.String(), `"psychological-profile"`) {
		t.Fatalf("owner init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runOwner([]string{"interview"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"cold_start"`) || strings.Contains(output.String(), `"psychological-profile"`) {
		t.Fatalf("owner interview exit = %d, output = %s", code, output.String())
	}
}

func TestOwnerRefineAppliesEligibleFacetFromStdinAndRequiresConfirmationToRevert(t *testing.T) {
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	var output bytes.Buffer
	if code := runOwner([]string{"init"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("owner init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	code := runOwnerWithInput([]string{"refine", "submit", "--facet", "voice", "--evidence", "owner approved three drafts", "--stdin"}, strings.NewReader("# Voice\n\nClear and decisive.\n"), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"state": "proposed"`) || strings.Contains(output.String(), `"audit_id"`) {
		t.Fatalf("refine submit exit = %d, output = %s", code, output.String())
	}
	var proposal struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(output.Bytes(), &proposal); err != nil || proposal.ID == "" {
		t.Fatalf("proposal receipt = %s, err = %v", output.String(), err)
	}
	output.Reset()
	code = runOwner([]string{"refine", "apply", "--confirm", proposal.ID}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"state": "applied"`) || !strings.Contains(output.String(), `"audit_id"`) {
		t.Fatalf("confirmed apply exit = %d, output = %s", code, output.String())
	}
	var receipt struct {
		AuditID string `json:"audit_id"`
	}
	if err := json.Unmarshal(output.Bytes(), &receipt); err != nil || receipt.AuditID == "" {
		t.Fatalf("refine receipt = %s, err = %v", output.String(), err)
	}
	output.Reset()
	code = runOwner([]string{"refine", "revert", receipt.AuditID}, &output, &output, func() (string, error) { return dataRoot, nil })
	if code == ExitOK || !strings.Contains(output.String(), "--confirm") {
		t.Fatalf("unconfirmed revert exit = %d, output = %s", code, output.String())
	}
}

func TestAtlasCommandsBootstrapOnlyPrivateOwnerAndWorkspaceRoots(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "Developer", "case-a")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("workspace init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runAtlas([]string{"init", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"managed": {`) || !strings.Contains(output.String(), `"state": "unavailable"`) || !strings.Contains(output.String(), `"workspace": {`) {
		t.Fatalf("atlas init exit = %d, output = %s", code, output.String())
	}
	if _, err := os.Stat(filepath.Join(workspacePath, "brain", "tasks")); !os.IsNotExist(err) {
		t.Fatalf("unexpected task taxonomy: %v", err)
	}
}

func TestSessionPacketReportsPointersWithoutOwnerFacetBodies(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "Developer", "case-a")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("workspace init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runOwner([]string{"init"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("owner init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runAtlas([]string{"init", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("atlas init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runSession([]string{"packet", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"schema_version": 1`) || !strings.Contains(output.String(), `"catalog_pointer": "bundles/base/skills/catalog.json"`) || strings.Contains(output.String(), "Descreva como voce quer falar") {
		t.Fatalf("session packet exit = %d, output = %s", code, output.String())
	}
}

func TestSessionBridgeProducesTheSameBoundedAdapterInputForEachRuntime(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "Developer", "case-a")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("workspace init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runSession([]string{"bridge", "--runtime", "claude", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"event": "session_start"`) || !strings.Contains(output.String(), `"runtime": "claude"`) || !strings.Contains(output.String(), `"injection_state": "unavailable"`) {
		t.Fatalf("Claude bridge exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runSession([]string{"bridge", "--runtime", "codex", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"runtime": "codex"`) || strings.Contains(output.String(), "Descreva como voce quer falar") {
		t.Fatalf("Codex bridge exit = %d, output = %s", code, output.String())
	}
}

func TestInitPersistsTheSelectedInteractionProfile(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	var output bytes.Buffer
	if code := runInit([]string{"--profile", "power", filepath.Join(root, "workspace")}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"profile": "power"`) {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runProfile([]string{"show"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"profile": "power"`) || !strings.Contains(output.String(), `"source": "configured"`) {
		t.Fatalf("profile after init exit = %d, output = %s", code, output.String())
	}
}

func TestWorkspaceAgentCommandsCreateAndExposeTheGuidedInterview(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "workspace")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"workspace_agent"`) || !strings.Contains(output.String(), `"workspace-agent-`) {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runWorkspaceAgent([]string{"interview", workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"workspace_agent_setup"`) || !strings.Contains(output.String(), `"research"`) {
		t.Fatalf("workspace-agent interview exit = %d, output = %s", code, output.String())
	}
}

func TestWorkspaceAgentResearchAndEconomicCommandsPersistGovernedArtifacts(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "workspace")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	briefInput := `{"reviewed_by":"owner","classification":"confidential","mandate":"support a decision","objectives":["recommendation"],"stakeholders":["sponsor"],"constraints":["four weeks"],"bullish":[{"statement":"upside","evidence":["public signal"],"assumptions":["adoption grows"],"counter_evidence":["weak conversion"],"invalidation_signals":["demand declines"]}],"bearish":[{"statement":"downside","evidence":["public risk"],"assumptions":["cost stays high"],"counter_evidence":["efficiency improves"],"invalidation_signals":["cost falls"]}],"research_questions":["public market size"]}`
	code := runWorkspaceAgentWithInput([]string{"brief", "submit", "--stdin", workspacePath}, strings.NewReader(briefInput), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"brief_id"`) {
		t.Fatalf("brief submit exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	planInput := `{"purpose":"public market context","valid_until":"2027-07-25T12:00:00Z","max_queries":1,"query_themes":["market size"],"sources":["ibge.gov.br"]}`
	code = runWorkspaceAgentWithInput([]string{"research", "plan", "--stdin", workspacePath}, strings.NewReader(planInput), &output, &output, func() (string, error) { return dataRoot, nil })
	var plan struct {
		PlanID string `json:"plan_id"`
	}
	if code != ExitOK || json.Unmarshal(output.Bytes(), &plan) != nil || plan.PlanID == "" {
		t.Fatalf("research plan exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	code = runWorkspaceAgentWithInput([]string{"research", "approve", "--plan", plan.PlanID, "--approved-by", "owner", "--confirm", workspacePath}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"state": "approved"`) {
		t.Fatalf("research approve exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	queryInput := `{"plan_id":"` + plan.PlanID + `","query":"market size"}`
	code = runWorkspaceAgentWithInput([]string{"research", "query", "--stdin", workspacePath}, strings.NewReader(queryInput), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"slot": 1`) {
		t.Fatalf("research query exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	evidenceInput := `{"plan_id":"` + plan.PlanID + `","query":"market size","source_url":"https://www.ibge.gov.br/example","retrieved_at":"2026-07-25T12:00:00Z","valid_until":"2027-07-25T12:00:00Z","claim":"Public fact","evidence_strength":"primary","classification":"public"}`
	code = runWorkspaceAgentWithInput([]string{"research", "record", "--stdin", workspacePath}, strings.NewReader(evidenceInput), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"state": "recorded"`) {
		t.Fatalf("research record exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	snapshotInput := `{"as_of":"2026-07-25T12:00:00Z","claims":[{"statement":"Public macro claim","classification":"public","source_urls":["https://www.bcb.gov.br/example"]}],"sources":[{"url":"https://www.bcb.gov.br/example","retrieved_at":"2026-07-25T12:00:00Z"}],"attestation":{}}`
	code = runWorkspaceAgentWithInput([]string{"economic", "import", "--stdin", "--attested-public", "--attested-by", "owner", "--confirm-no-workspace-derivation"}, strings.NewReader(snapshotInput), &output, &output, func() (string, error) { return dataRoot, nil })
	var snapshot struct {
		SnapshotID string `json:"snapshot_id"`
	}
	if code != ExitOK || json.Unmarshal(output.Bytes(), &snapshot) != nil || snapshot.SnapshotID == "" {
		t.Fatalf("economic import exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	code = runWorkspaceAgentWithInput([]string{"economic", "attach", "--snapshot", snapshot.SnapshotID, workspacePath}, strings.NewReader(""), &output, &output, func() (string, error) { return dataRoot, nil })
	if code != ExitOK || !strings.Contains(output.String(), `"state": "attached"`) {
		t.Fatalf("economic attach exit = %d, output = %s", code, output.String())
	}
}

func TestInitPersistsStandardWhenNoProfileIsSelected(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	var output bytes.Buffer
	if code := runInit([]string{filepath.Join(root, "workspace")}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}
	output.Reset()
	if code := runProfile([]string{"show"}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"profile": "standard"`) || !strings.Contains(output.String(), `"source": "configured"`) {
		t.Fatalf("profile after default init exit = %d, output = %s", code, output.String())
	}
}

func TestProductStatusAndDoctorDescribeReadyWorkspace(t *testing.T) {
	root := t.TempDir()
	workspacePath := filepath.Join(root, "Developer", "case-a")
	dataRoot := filepath.Join(root, "local", "BCGOS")
	var output bytes.Buffer
	if code := runInit([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK {
		t.Fatalf("init exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	if code := runProductStatus([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }); code != ExitOK || !strings.Contains(output.String(), `"state": "ready"`) || !strings.Contains(output.String(), `"brain_readable": true`) || !strings.Contains(output.String(), `"profile": "standard"`) {
		t.Fatalf("status exit = %d, output = %s", code, output.String())
	}

	output.Reset()
	available := func(name string) bool { return name == "claude" }
	if code := runDoctor([]string{workspacePath}, &output, &output, func() (string, error) { return dataRoot, nil }, available); code != ExitOK || !strings.Contains(output.String(), `"claude_code"`) || !strings.Contains(output.String(), `"runtime_capabilities"`) || !strings.Contains(output.String(), `"context_inject"`) || !strings.Contains(output.String(), `"interaction_profile"`) || !strings.Contains(output.String(), `"codex"`) || !strings.Contains(output.String(), `"unavailable"`) {
		t.Fatalf("doctor exit = %d, output = %s", code, output.String())
	}
}

func TestDoctorExplainsUninitializedWorkspace(t *testing.T) {
	root := t.TempDir()
	var output bytes.Buffer
	code := runDoctor([]string{filepath.Join(root, "not-initialized")}, &output, &output, func() (string, error) { return filepath.Join(root, "local", "BCGOS"), nil }, func(string) bool { return false })
	if code != ExitOK || !strings.Contains(output.String(), `"state": "action_required"`) || !strings.Contains(output.String(), "bcgos init") {
		t.Fatalf("doctor exit = %d, output = %s", code, output.String())
	}
}
