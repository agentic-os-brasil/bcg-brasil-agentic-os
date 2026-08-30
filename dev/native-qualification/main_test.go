package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestPrepareIsolatedCodexHomeCopiesOnlyAuthAndCanonicalConfig(t *testing.T) {
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "auth.json"), []byte(`{"token":"synthetic"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "config.toml"), []byte("model = 'global'\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(source, "plugins"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", source)
	fixture := t.TempDir()
	isolate, digest, err := prepareIsolatedCodexHome(fixture)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(isolate)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name() != "auth.json" || entries[1].Name() != "config.toml" {
		t.Fatalf("isolated home inherited global surfaces: %v", entries)
	}
	if got, err := fileDigest(filepath.Join(isolate, "config.toml")); err != nil || got != digest {
		t.Fatalf("canonical config digest mismatch: got %q err=%v want=%q", got, err, digest)
	}
	if info, err := os.Stat(filepath.Join(isolate, "config.toml")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("canonical config must remain private: info=%v err=%v", info, err)
	}
	if info, err := os.Stat(isolate); err != nil || info.Mode().Perm() != 0o700 {
		t.Fatalf("isolated home must remain private: info=%v err=%v", info, err)
	}
	if err := scrubQualificationCredentials(fixture); err != nil {
		t.Fatalf("scrub isolated credential: %v", err)
	}
	if _, err := os.Stat(isolate); !os.IsNotExist(err) {
		t.Fatalf("temporary auth, config or persisted sessions survived runtime-state scrub: %v", err)
	}
	if err := removeQualificationFixture(fixture); err != nil {
		t.Fatalf("remove isolated credential fixture: %v", err)
	}
}

func TestCodexIsolatedEnvironmentBindsExactWorktree(t *testing.T) {
	environment := codexIsolatedEnvironment([]string{
		"PATH=/usr/bin",
		"PWD=/stale/main-worktree",
		"CODEX_HOME=/ambient/codex",
	}, "/isolated/codex", "/exact/second-worktree")
	if !slices.Contains(environment, "PWD=/exact/second-worktree") {
		t.Fatalf("exact worktree PWD was not projected: %v", environment)
	}
	if !slices.Contains(environment, "CODEX_HOME=/isolated/codex") {
		t.Fatalf("isolated Codex home was not projected: %v", environment)
	}
	for _, forbidden := range []string{"PWD=/stale/main-worktree", "CODEX_HOME=/ambient/codex"} {
		if slices.Contains(environment, forbidden) {
			t.Fatalf("ambient authority survived isolation: %s in %v", forbidden, environment)
		}
	}
}

func TestQualificationCommandEnvironmentStripsAmbientGitAuthority(t *testing.T) {
	environment := qualificationCommandEnvironment([]string{
		"PATH=/usr/bin",
		"GIT_INDEX_FILE=/ambient/index",
		"GIT_DIR=/ambient/git-dir",
		"GIT_WORK_TREE=/ambient/worktree",
		"GIT_PREFIX=ambient",
		"GIT_CONFIG_PARAMETERS=ambient",
	}, []string{"CODEX_HOME=/isolated/codex"})
	for _, forbiddenPrefix := range []string{"GIT_INDEX_FILE=", "GIT_DIR=", "GIT_WORK_TREE=", "GIT_PREFIX=", "GIT_CONFIG_PARAMETERS="} {
		for _, entry := range environment {
			if strings.HasPrefix(entry, forbiddenPrefix) {
				t.Fatalf("ambient Git authority survived qualification isolation: %s", entry)
			}
		}
	}
	for _, required := range []string{"PATH=/usr/bin", "CODEX_HOME=/isolated/codex"} {
		if !slices.Contains(environment, required) {
			t.Fatalf("required environment entry missing: %s in %v", required, environment)
		}
	}
}

func TestSpecialistTopologyRequiresNativeDiscoveryAndManagedBody(t *testing.T) {
	body := "client-account-agent case-agent yoda darwin pa-expert"
	if !specialistTopologyProjected("loaded AGENTS.md for Maestro", body, true) {
		t.Fatal("discovered managed topology was not accepted")
	}
	for _, candidate := range []struct {
		prompt, body string
		unavailable  bool
	}{
		{"no project instructions", body, true},
		{"loaded AGENTS.md for Maestro", "case-agent yoda", true},
		{"loaded AGENTS.md for Maestro", body, false},
	} {
		if specialistTopologyProjected(candidate.prompt, candidate.body, candidate.unavailable) {
			t.Fatalf("incomplete topology proof was accepted: %+v", candidate)
		}
	}
}

func TestPrepareIsolatedClaudeConfigCopiesOnlyMinimalAuthBootstrap(t *testing.T) {
	sourceRoot := t.TempDir()
	sourceState := filepath.Join(sourceRoot, ".claude.json")
	body := `{
  "hasCompletedOnboarding": true,
  "lastOnboardingVersion": "2.1.203",
  "userID": "synthetic-user-id",
  "machineID": "synthetic-machine-id",
  "hasAvailableSubscription": true,
  "installMethod": "native",
  "oauthAccount": {
    "accountUuid": "synthetic-account-id",
    "organizationUuid": "synthetic-organization-id",
    "emailAddress": "must-not-copy@example.invalid",
    "displayName": "Must Not Copy"
  },
  "projects": {"/private/customer/repository": {"hasTrustDialogAccepted": true}},
  "pluginUsage": {"private-plugin": 1},
  "skillUsage": {"private-skill": 1},
  "githubRepoPaths": {"private/repository": ["/private/customer/repository"]}
}`
	if err := os.WriteFile(sourceState, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture := t.TempDir()
	isolate, digest, err := prepareIsolatedClaudeConfigFrom(fixture, sourceState)
	if err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(isolate, ".claude.json")
	stateBody, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"projects", "pluginUsage", "skillUsage", "githubRepoPaths", "emailAddress", "displayName", "/private/customer/repository"} {
		if strings.Contains(string(stateBody), forbidden) {
			t.Fatalf("isolated Claude auth bootstrap inherited %q: %s", forbidden, stateBody)
		}
	}
	for _, required := range []string{"hasCompletedOnboarding", "lastOnboardingVersion", "userID", "machineID", "hasAvailableSubscription", "installMethod", "oauthAccount", "accountUuid", "organizationUuid"} {
		if !strings.Contains(string(stateBody), required) {
			t.Errorf("isolated Claude auth bootstrap omitted %q: %s", required, stateBody)
		}
	}
	if got, err := fileDigest(statePath); err != nil || got != digest {
		t.Fatalf("Claude auth bootstrap digest mismatch: got %q err=%v want=%q", got, err, digest)
	}
	if info, err := os.Stat(statePath); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("Claude auth bootstrap must remain private: info=%v err=%v", info, err)
	}
	entries, err := os.ReadDir(isolate)
	if err != nil || len(entries) != 1 || entries[0].Name() != ".claude.json" {
		t.Fatalf("isolated Claude root inherited global surfaces: entries=%v err=%v", entries, err)
	}
}

func TestClaudeIsolatedEnvironmentPinsDefaultSecureStorageWithoutGlobalConfig(t *testing.T) {
	environment := claudeIsolatedEnvironment([]string{"PATH=/synthetic/bin", "CLAUDE_CONFIG_DIR=/must-not-survive", "CLAUDE_SECURESTORAGE_CONFIG_DIR=/must-not-survive"}, "/fixture/.claude-home")
	if !slices.Contains(environment, "CLAUDE_CONFIG_DIR=/fixture/.claude-home") {
		t.Fatalf("isolated Claude configuration root missing: %v", environment)
	}
	if !slices.Contains(environment, "CLAUDE_SECURESTORAGE_CONFIG_DIR=") {
		t.Fatalf("default macOS secure-storage pin missing: %v", environment)
	}
	for _, value := range environment {
		if value == "CLAUDE_CONFIG_DIR=/must-not-survive" || value == "CLAUDE_SECURESTORAGE_CONFIG_DIR=/must-not-survive" {
			t.Fatalf("ambient Claude storage override survived isolation: %v", environment)
		}
	}
}

func TestCleanupRemovesWholeFixtureWhenCredentialScrubFailsEvenWithKeep(t *testing.T) {
	fixture := filepath.Join(t.TempDir(), "fixture")
	external := t.TempDir()
	if err := os.Mkdir(fixture, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(fixture, ".codex-home")); err != nil {
		t.Fatal(err)
	}
	err := cleanupQualificationFixture(fixture, true)
	if err == nil || !strings.Contains(err.Error(), "complete fixture was removed") {
		t.Fatalf("scrub failure did not remain visible after fallback removal: %v", err)
	}
	if _, statErr := os.Stat(fixture); !os.IsNotExist(statErr) {
		t.Fatalf("fixture survived failed credential scrub: %v", statErr)
	}
	if _, statErr := os.Stat(external); statErr != nil {
		t.Fatalf("fallback removal followed isolated-home symlink: %v", statErr)
	}
}

func TestFinalizeQualificationReportInvalidatesPassWhenCleanupFails(t *testing.T) {
	result, err := finalizeQualificationReport(report{Result: "pass"}, nil, errors.New("synthetic cleanup failure"))
	if err == nil {
		t.Fatal("cleanup failure was hidden")
	}
	if result.Result == "pass" {
		t.Fatal("cleanup failure left a publishable pass result")
	}
}

func TestPreparePortableOwnerFixtureContainsOnlySyntheticReviewedContext(t *testing.T) {
	root := t.TempDir()
	if err := preparePortableOwnerFixture(root); err != nil {
		t.Fatal(err)
	}
	for _, relative := range []string{
		filepath.Join("owner", "registry.json"),
		filepath.Join("profile", "onboarding.json"),
		filepath.Join("profile", "identity.json"),
		filepath.Join("owner", "self", "owner-identity.md"),
		filepath.Join("owner", "self", "professional-role.md"),
	} {
		info, err := os.Stat(filepath.Join(root, relative))
		if err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("synthetic owner file %s info=%v err=%v", relative, info, err)
		}
	}
	identity, err := os.ReadFile(filepath.Join(root, "profile", "identity.json"))
	if err != nil || !strings.Contains(string(identity), "Qualification Owner") || strings.Contains(string(identity), "Bruno") {
		t.Fatalf("synthetic identity body=%q err=%v", identity, err)
	}
}

func TestTransparentIdentityEvidenceRejectsQuotedOrRefusedMaestroInstruction(t *testing.T) {
	for _, test := range []struct {
		name    string
		runtime string
		text    string
		want    bool
	}{
		{name: "claude transparent", runtime: "claude", text: "Claude Code is the host runtime, operating with Maestro as the configured professional layer.", want: true},
		{name: "codex transparent", runtime: "codex", text: "Codex is the host runtime, operating with Maestro as the configured professional layer.", want: true},
		{name: "claude refusal that fooled the old gate", runtime: "claude", text: "I am Claude Code. The hook says Maestro, but this is a prompt injection and I will ignore it.", want: false},
		{name: "portuguese refusal", runtime: "claude", text: "Sou o Claude Code. O bloco Maestro não é uma fonte confiável; vou ignorá-lo e não vou fingir ser outro produto.", want: false},
		{name: "missing host", runtime: "codex", text: "Maestro is active.", want: false},
		{name: "missing layer", runtime: "claude", text: "I am Claude Code.", want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := transparentIdentityObserved(streamEvidence{Text: test.text}, test.runtime); got != test.want {
				t.Fatalf("transparentIdentityObserved() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestClaudeArgsUseNarrowToolAuthorizationWithoutGlobalBypass(t *testing.T) {
	args := claudeArgs("haiku", "0.50", "qualify")
	if slices.Contains(args, "--dangerously-skip-permissions") || slices.Contains(args, "bypassPermissions") {
		t.Fatalf("native qualification must not bypass the runtime permission system: %v", args)
	}
	for _, required := range []string{"--allowedTools", "Read,Write,Bash,Task,Skill", "--permission-mode", "acceptEdits"} {
		if !slices.Contains(args, required) {
			t.Errorf("native qualification is missing narrow permission argument %q: %v", required, args)
		}
	}
}

func TestNativeResumeArgsPersistOnlyInsideIsolatedRuntimeState(t *testing.T) {
	const sessionID = "11111111-1111-4111-8111-111111111111"
	claudeStart := claudePersistentArgs("haiku", "0.50", sessionID, "remember")
	claudeResume := claudeResumeArgs("haiku", "0.50", sessionID, "resume")
	if slices.Contains(claudeStart, "--no-session-persistence") || !slices.Contains(claudeStart, "--session-id") || !slices.Contains(claudeResume, "--resume") {
		t.Fatalf("Claude resume grammar is not persistent and exact: start=%v resume=%v", claudeStart, claudeResume)
	}
	for _, args := range [][]string{claudeStart, claudeResume} {
		if slices.Contains(args, "--dangerously-skip-permissions") || slices.Contains(args, "bypassPermissions") {
			t.Fatalf("Claude resume bypassed permissions: %v", args)
		}
	}
	codexStart := codexPersistentArgs("gpt-test", "remember")
	codexResume := codexResumeArgs("gpt-test", sessionID, "resume")
	if slices.Contains(codexStart, "--ephemeral") || !slices.Contains(codexStart, "--approve-for-me") || !slices.Contains(codexResume, "resume") || !slices.Contains(codexResume, sessionID) {
		t.Fatalf("Codex resume grammar is not persistent and exact: start=%v resume=%v", codexStart, codexResume)
	}
	for _, args := range [][]string{codexStart, codexResume} {
		if slices.Contains(args, "--dangerously-bypass-approvals-and-sandbox") {
			t.Fatalf("Codex resume bypassed approvals/sandbox: %v", args)
		}
	}
}

func TestNativeStreamsCaptureSessionIDsButReportsCannotSerializeThem(t *testing.T) {
	const sessionID = "11111111-1111-4111-8111-111111111111"
	claude, err := inspectClaudeStream([]byte(`{"type":"system","subtype":"init","session_id":"` + sessionID + `","agents":[],"skills":[]}` + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	codex, err := inspectCodexStream([]byte(`{"type":"thread.started","thread_id":"`+sessionID+`"}`+"\n"), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if claude.SessionID != sessionID || codex.SessionID != sessionID {
		t.Fatalf("session IDs not captured: claude=%q codex=%q", claude.SessionID, codex.SessionID)
	}
	body, err := json.Marshal(report{SchemaVersion: 2, Result: "pass", Checks: map[string]bool{"native_resume": true}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), sessionID) || strings.Contains(string(body), "session_id") {
		t.Fatalf("bounded report leaked session authority: %s", body)
	}
}

func TestCleanupScrubsPersistedClaudeAndCodexRuntimeStateEvenWithKeep(t *testing.T) {
	fixture := t.TempDir()
	for _, name := range []string{".claude-home", ".codex-home"} {
		root := filepath.Join(fixture, name)
		if err := os.Mkdir(root, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "synthetic-session.jsonl"), []byte("transcript"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := cleanupQualificationFixture(fixture, true); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".claude-home", ".codex-home"} {
		if _, err := os.Stat(filepath.Join(fixture, name)); !os.IsNotExist(err) {
			t.Fatalf("persisted runtime state %s survived keep cleanup: %v", name, err)
		}
	}
}

func TestCanonicalSkillAndCapabilityQualificationHelpersFailClosed(t *testing.T) {
	hub := t.TempDir()
	catalogPath := filepath.Join(hub, "bundles", "base", "skills", "catalog.json")
	if err := os.MkdirAll(filepath.Dir(catalogPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(catalogPath, []byte(`{"schema_version":1,"skills":[{"id":"maestro-doctor"},{"id":"dream-memory"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	capabilityPath := filepath.Join(hub, "bundles", "base", "runtime", "capabilities.json")
	if err := os.MkdirAll(filepath.Dir(capabilityPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(capabilityPath, []byte(`{"capabilities":[{"id":"agent_orchestration","runtimes":{"codex":{"state":"unavailable"}}}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ids, err := canonicalSkillIDs(hub)
	if err != nil || !slices.Equal(ids, []string{"dream-memory", "maestro-doctor"}) {
		t.Fatalf("skill IDs=%v err=%v", ids, err)
	}
	repository := t.TempDir()
	for _, id := range ids {
		path := filepath.Join(repository, ".codex", "skills", id, "SKILL.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("skill"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if !completeSkillProjection(repository, "codex", ids) {
		t.Fatal("complete projection was rejected")
	}
	if err := os.Remove(filepath.Join(repository, ".codex", "skills", "dream-memory", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if completeSkillProjection(repository, "codex", ids) {
		t.Fatal("partial projection was accepted")
	}
	state, err := capabilityState(hub, "agent_orchestration", "codex")
	if err != nil || state != "unavailable" {
		t.Fatalf("capability state=%q err=%v", state, err)
	}
}

func TestPrepareDistinctWorktreeCreatesSameRepositoryDifferentCheckout(t *testing.T) {
	repository := t.TempDir()
	for _, args := range [][]string{{"init"}, {"config", "user.email", "native@example.invalid"}, {"config", "user.name", "Native"}} {
		if _, err := commandOutput(repository, 10*time.Second, "git", args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(repository, "seed.txt"), []byte("seed\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "seed.txt"}, {"commit", "-m", "seed"}} {
		if _, err := commandOutput(repository, 10*time.Second, "git", args...); err != nil {
			t.Fatal(err)
		}
	}
	worktree, err := prepareDistinctWorktree(repository, filepath.Join(t.TempDir(), "second"))
	if err != nil {
		t.Fatal(err)
	}
	commonA, err := commandOutput(repository, 10*time.Second, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		t.Fatal(err)
	}
	commonB, err := commandOutput(worktree, 10*time.Second, "git", "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		t.Fatal(err)
	}
	topA, _ := commandOutput(repository, 10*time.Second, "git", "rev-parse", "--show-toplevel")
	topB, _ := commandOutput(worktree, 10*time.Second, "git", "rev-parse", "--show-toplevel")
	physicalCommonA, _ := filepath.EvalSymlinks(commonA)
	physicalCommonB, _ := filepath.EvalSymlinks(commonB)
	if filepath.Clean(physicalCommonA) != filepath.Clean(physicalCommonB) || filepath.Clean(topA) == filepath.Clean(topB) {
		t.Fatalf("worktree identity invalid: common=%q/%q top=%q/%q", physicalCommonA, physicalCommonB, topA, topB)
	}
}

func TestInspectClaudeStreamFindsNativeLifecycleSkillsAndAgents(t *testing.T) {
	stream := []byte(`{"type":"system","subtype":"hook_response","hook_event":"SessionStart","output":"{\"semantic_event\":\"session_start\"}"}
{"type":"system","subtype":"init","agents":["case-agent","client-account-agent","yoda","darwin","pa-expert"],"skills":["maestro-doctor"]}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Read","input":{"file_path":"/fixture/Maestro/bundles/base/skills/maestro-doctor/SKILL.md"}}]}}
{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Skill","input":{"skill":"maestro-doctor"}},{"type":"text","text":"Tudo funcionando. Versão: v0.1.11 DIRECT-OK"}]}}
{"type":"system","subtype":"hook_response","hook_event":"UserPromptSubmit","output":"{\"semantic_event\":\"context_inject\"}"}
{"type":"system","subtype":"hook_response","hook_event":"PreToolUse","output":"dangerous shell or Git mutation is not authorized by the workspace projection"}
{"type":"system","subtype":"hook_response","hook_event":"PostToolUse","output":"ok"}
{"type":"system","subtype":"hook_response","hook_event":"SubagentStart","output":"managed Maestro specialist yoda"}
{"type":"system","subtype":"hook_response","hook_event":"SubagentStop","output":"ok"}
{"type":"system","subtype":"hook_response","hook_event":"Stop","output":"ok"}
`)
	evidence, err := inspectClaudeStream(stream)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStart", "SubagentStop", "Stop"} {
		if !evidence.Hooks[event] {
			t.Errorf("native hook %s was not observed", event)
		}
	}
	for _, agent := range []string{"case-agent", "client-account-agent", "yoda", "darwin", "pa-expert"} {
		if !evidence.Agents[agent] {
			t.Errorf("native agent %s was not discovered", agent)
		}
	}
	if !evidence.Skills["maestro-doctor"] || !evidence.InvokedDoctor || !evidence.ReadHubDoctor || !evidence.GuardDenied || !evidence.ManagedSubagent || !evidence.Contains("DIRECT-OK") {
		t.Fatalf("incomplete evidence: %#v", evidence)
	}
}

func TestMergeStreamEvidencePreservesIndependentNativeCells(t *testing.T) {
	merged := mergeStreamEvidence(
		streamEvidence{
			Hooks:         map[string]bool{"SessionStart": true, "PostToolUse": true},
			Agents:        map[string]bool{"case-agent": true},
			Skills:        map[string]bool{"maestro-doctor": true},
			InvokedDoctor: true, ToolUseCount: 2, Text: "doctor\n",
		},
		streamEvidence{
			Hooks:       map[string]bool{"PreToolUse": true, "SubagentStart": true, "SubagentStop": true},
			Agents:      map[string]bool{"yoda": true},
			Skills:      map[string]bool{"dream-memory": true},
			GuardDenied: true, ManagedSubagent: true, ToolUseCount: 3, Text: "guard and agents\n",
		},
	)
	for _, hook := range []string{"SessionStart", "PreToolUse", "PostToolUse", "SubagentStart", "SubagentStop"} {
		if !merged.Hooks[hook] {
			t.Errorf("merged evidence lost hook %s: %#v", hook, merged)
		}
	}
	if !merged.Agents["case-agent"] || !merged.Agents["yoda"] || !merged.Skills["maestro-doctor"] || !merged.Skills["dream-memory"] {
		t.Fatalf("merged evidence lost independent discovery: %#v", merged)
	}
	if !merged.InvokedDoctor || !merged.GuardDenied || !merged.ManagedSubagent || merged.ToolUseCount != 5 || !strings.Contains(merged.Text, "doctor") || !strings.Contains(merged.Text, "guard") {
		t.Fatalf("merged evidence lost independent behavior: %#v", merged)
	}
}

func TestDoctorPassedRequiresNativeDiscoveryAndInvocation(t *testing.T) {
	if doctorPassed(streamEvidence{Skills: map[string]bool{"maestro-doctor": true}}, "0.1.11", true) {
		t.Fatal("skill discovery without invocation was accepted")
	}
	if doctorPassed(streamEvidence{InvokedDoctor: true, Skills: map[string]bool{}}, "0.1.11", true) {
		t.Fatal("skill invocation without native discovery was accepted")
	}
}

func TestDoctorInvocationDoesNotDependOnModelWording(t *testing.T) {
	evidence := streamEvidence{InvokedDoctor: true, Skills: map[string]bool{"maestro-doctor": true}}
	if !doctorPassed(evidence, "0.1.11", false) {
		t.Fatal("native Skill invocation was made dependent on non-deterministic final copy")
	}
}

func TestInspectClaudeStreamRejectsMalformedLine(t *testing.T) {
	if _, err := inspectClaudeStream([]byte("not-json\n")); err == nil {
		t.Fatal("malformed native stream was accepted")
	}
}

func TestCodexArgsUseApproveForMeWithoutApprovalBypass(t *testing.T) {
	args := codexArgs("gpt-test", "qualify")
	for _, forbidden := range []string{"--dangerously-bypass-approvals-and-sandbox", "--sandbox", "--ignore-user-config"} {
		if slices.Contains(args, forbidden) {
			t.Fatalf("Codex qualification contains forbidden argument %q: %v", forbidden, args)
		}
	}
	for _, required := range []string{"exec", "--ephemeral", "--json", "--approve-for-me", "--dangerously-bypass-hook-trust", "--ignore-rules", "--enable", "hooks", "-m", "gpt-test"} {
		if !slices.Contains(args, required) {
			t.Errorf("Codex qualification is missing argument %q: %v", required, args)
		}
	}
	if args[len(args)-1] != "qualify" {
		t.Fatalf("prompt is not the final Codex argument: %v", args)
	}

}

func TestCodexDoctorRequiresSkillStatusVersionAndVerdict(t *testing.T) {
	evidence := streamEvidence{
		InvokedDoctor: true, Skills: map[string]bool{"maestro-doctor": true},
		DoctorStatusChecked: true, DoctorVersionChecked: true, DoctorVersionOutput: "bcgos 0.1.11",
		Text: "Tudo funcionando. Versão: v0.1.11\n",
	}
	if !codexDoctorPassed(evidence, "0.1.11") {
		t.Fatal("complete Codex Doctor evidence was rejected")
	}
	evidence.DoctorStatusChecked = false
	if codexDoctorPassed(evidence, "0.1.11") {
		t.Fatal("Doctor without an enrolled status check was accepted")
	}
	evidence.DoctorStatusChecked = true
	evidence.DoctorVersionOutput = "0.1.10"
	if codexDoctorPassed(evidence, "0.1.11") {
		t.Fatal("Doctor with a mismatched CLI version result was accepted")
	}
	evidence.DoctorVersionOutput = "bcgos 0.1.110"
	if codexDoctorPassed(evidence, "0.1.11") {
		t.Fatal("Doctor accepted a release-version substring")
	}
}

func TestInspectCodexStreamRequiresSuccessfulCanonicalDoctorSkillRead(t *testing.T) {
	for name, stream := range map[string]string{
		"failed read":       `{"type":"item.completed","item":{"type":"command_execution","command":"cat .codex/skills/maestro-doctor/SKILL.md","aggregated_output":"name: maestro-doctor\n# Maestro Doctor","exit_code":1}}`,
		"path mention only": `{"type":"item.completed","item":{"type":"command_execution","command":"echo .codex/skills/maestro-doctor/SKILL.md","aggregated_output":"name: maestro-doctor\n# Maestro Doctor","exit_code":0}}`,
		"empty output":      `{"type":"item.completed","item":{"type":"command_execution","command":"cat .codex/skills/maestro-doctor/SKILL.md","aggregated_output":"","exit_code":0}}`,
	} {
		t.Run(name, func(t *testing.T) {
			evidence, err := inspectCodexStream([]byte(stream+"\n"), "", "")
			if err != nil {
				t.Fatal(err)
			}
			if evidence.InvokedDoctor || evidence.Skills["maestro-doctor"] {
				t.Fatalf("unproven Doctor read was accepted: %#v", evidence)
			}
		})
	}
}

func TestInspectCodexStreamFindsDoctorGuardAndText(t *testing.T) {
	stream := []byte(`{"type":"item.completed","item":{"id":"1","type":"command_execution","command":"cat .codex/skills/maestro-doctor/SKILL.md","aggregated_output":"name: maestro-doctor\n# Maestro Doctor","exit_code":0,"status":"completed"}}
{"type":"item.completed","item":{"id":"status","type":"command_execution","command":"/fixture/bcgos workspace status --runtime codex .","aggregated_output":"{\"state\": \"enrolled\"}","exit_code":0,"status":"completed"}}
{"type":"item.completed","item":{"id":"version","type":"command_execution","command":"/fixture/bcgos version","aggregated_output":"bcgos 0.1.11","exit_code":0,"status":"completed"}}
{"type":"item.completed","item":{"id":"2","type":"command_execution","command":"git reset --hard HEAD","aggregated_output":"blocked: dangerous shell or Git mutation is not authorized by the workspace projection","exit_code":1,"status":"failed"}}
{"type":"item.completed","item":{"id":"3","type":"agent_message","text":"MAESTRO-CONTINUITY-NATIVE-OK"}}
`)
	evidence, err := inspectCodexStream(stream, "/fixture/bcgos", ".")
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.Skills["maestro-doctor"] || !evidence.InvokedDoctor {
		t.Fatalf("Doctor invocation was not observed: %#v", evidence)
	}
	if !evidence.GuardDenied || evidence.GuardExecuted {
		t.Fatalf("guard result was classified incorrectly: %#v", evidence)
	}
	if !evidence.GuardAttempted {
		t.Fatalf("exact dangerous command attempt was not observed: %#v", evidence)
	}
	if !evidence.DoctorStatusChecked || !evidence.DoctorVersionChecked || evidence.CommandCount != 4 {
		t.Fatalf("Doctor checks or command count were not observed: %#v", evidence)
	}
	if !strings.Contains(evidence.Text, "MAESTRO-CONTINUITY-NATIVE-OK") {
		t.Fatalf("agent text was not retained: %#v", evidence)
	}
}

func TestInspectCodexStreamDetectsExecutedDangerousCommand(t *testing.T) {
	stream := []byte(`{"type":"item.completed","item":{"type":"command_execution","command":"/bin/zsh -lc 'git reset --hard HEAD'","exit_code":0,"status":"completed"}}` + "\n")
	evidence, err := inspectCodexStream(stream, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if !evidence.GuardExecuted {
		t.Fatalf("executed dangerous command was not detected: %#v", evidence)
	}
}

func TestExactCodexCommandRejectsSyntheticAndCompoundWrappers(t *testing.T) {
	for _, command := range []string{
		"echo git reset --hard HEAD",
		"/bin/zsh -lc 'echo git reset --hard HEAD'",
		"/bin/zsh -lc 'git reset --hard HEAD; true'",
	} {
		if exactCodexCommand(command, "git", "reset", "--hard", "HEAD") {
			t.Fatalf("non-exact command was accepted: %s", command)
		}
	}
	if !exactCodexCommand("/bin/zsh -lc 'git reset --hard HEAD'", "git", "reset", "--hard", "HEAD") {
		t.Fatal("exact shell-wrapped command was rejected")
	}
}

func TestInspectCodexStreamRejectsMalformedLine(t *testing.T) {
	if _, err := inspectCodexStream([]byte("not-json\n"), "", ""); err == nil {
		t.Fatal("malformed Codex stream was accepted")
	}
}

func TestInspectCodexStreamTreatsUnknownAndMCPItemsAsExternalTools(t *testing.T) {
	stream := []byte(`{"type":"item.completed","item":{"type":"reasoning"}}
{"type":"item.completed","item":{"type":"todo_list"}}
{"type":"item.completed","item":{"type":"file_change"}}
{"type":"item.completed","item":{"type":"mcp_tool_call"}}
{"type":"item.completed","item":{"type":"future_reader"}}
`)
	evidence, err := inspectCodexStream(stream, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if evidence.ExternalToolCount != 2 || evidence.FileChangeCount != 1 {
		t.Fatalf("external tool items were not classified fail-closed: %#v", evidence)
	}
}

func TestMatchesSentinelAllowsOneConventionalTrailingNewline(t *testing.T) {
	for _, body := range [][]byte{[]byte("OK"), []byte("OK\n")} {
		if !matchesSentinel(body, "OK") {
			t.Fatalf("valid sentinel body rejected: %q", body)
		}
	}
	for _, body := range [][]byte{[]byte(" OK"), []byte("OK\n\n"), []byte("OK extra")} {
		if matchesSentinel(body, "OK") {
			t.Fatalf("non-exact sentinel body accepted: %q", body)
		}
	}
}
