package releasepack

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBuildScriptPortableProducesDeterministicNoGoNoNativeKits(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"macos", "windows"} {
		t.Run(target, func(t *testing.T) {
			output := filepath.Join(t.TempDir(), scriptPortableName("0.2.0", target))
			options := ScriptPortableOptions{Root: root, Output: output, Version: "0.2.0", TargetOS: target}
			result, err := BuildScriptPortable(options)
			if err != nil {
				t.Fatal(err)
			}
			if result.Status != "script-only-controlled-beta" || result.SHA256 == "" {
				t.Fatalf("unexpected result: %#v", result)
			}
			entries := readScriptPortableZIP(t, output)
			rootName := strings.TrimSuffix(filepath.Base(output), ".zip") + "/"
			for _, required := range []string{
				"COMECE-AQUI.txt", "capabilities.json", "inventory.sha256", "runtime-inventory.sha256", "maestro-os/CLAUDE.md",
				"projection/skills/maestro-onboarding/SKILL.md", "projection/skills/bcgos-operator/SKILL.md", "projection/settings.local.json",
				"projection/agents/client-account-agent.md", "projection/agents/case-agent.md", "projection/agents/walter.md", "projection/agents/darwin.md", "projection/agents/pa-expert.md",
			} {
				if _, ok := entries[rootName+required]; !ok {
					t.Fatalf("missing %s", required)
				}
			}
			for _, redundant := range []string{
				"payload/skills/maestro-onboarding/SKILL.md",
				"payload/skills/dream-memory/SKILL.md",
				"payload/agents/maestro/AGENT.md",
				"payload/bundles/tech-core/skills/pr-review/SKILL.md",
			} {
				if _, ok := entries[rootName+redundant]; ok {
					t.Fatalf("redundant projection source was disclosed: %s", redundant)
				}
			}
			if body := string(entries[rootName+"projection/skills/maestro-onboarding/SKILL.md"]); strings.Contains(body, "<maestro-cli>") {
				t.Fatalf("script-only onboarding retains native CLI instructions")
			}
			if body := string(entries[rootName+"projection/skills/execution-continuity/SKILL.md"]); !strings.Contains(body, "continuity-state.json") || !strings.Contains(body, "continuity-lite-v1") {
				t.Fatalf("script-only continuity overlay is missing its bounded handoff contract")
			}
			for _, retained := range []string{"agent-identity-setup", "account-case-setup", "case-agent-setup", "execution-continuity", "workspace-agent-setup"} {
				if _, ok := entries[rootName+"projection/skills/"+retained+"/SKILL.md"]; !ok {
					t.Fatalf("file-driven capability %s was not retained", retained)
				}
			}
			for _, unavailable := range []string{"dream-memory", "find-prior-work", "ingest-content", "retro"} {
				if _, ok := entries[rootName+"projection/skills/"+unavailable+"/SKILL.md"]; ok {
					t.Fatalf("native-authority capability %s was projected as available", unavailable)
				}
			}
			if target == "macos" {
				for _, required := range []string{"install.sh", "Start Maestro.command", "maestro-hook.sh"} {
					if _, ok := entries[rootName+required]; !ok {
						t.Fatalf("missing %s", required)
					}
				}
				body := string(entries[rootName+"install.sh"])
				start := string(entries[rootName+"Start Maestro.command"])
				startHere := string(entries[rootName+"COMECE-AQUI.txt"])
				for _, required := range []string{"--reveal-workspace", `/usr/bin/open "$WORKSPACE"`, "reveal_workspace"} {
					if !strings.Contains(body+start, required) {
						t.Fatalf("macOS quick-start is missing %q", required)
					}
				}
				if !strings.Contains(startHere, "Start Maestro.command") || !strings.Contains(startHere, "Se o duplo clique") {
					t.Fatalf("macOS start-here does not lead with launcher and preserve the Claude fallback")
				}
				for _, forbidden := range []string{"go build", "xattr", "spctl", "sudo"} {
					if strings.Contains(body, forbidden) {
						t.Fatalf("macOS installer contains forbidden %q", forbidden)
					}
				}
			} else {
				for _, required := range []string{"Install-Maestro.ps1", "Start Maestro.cmd", "Maestro-Hook.ps1"} {
					if _, ok := entries[rootName+required]; !ok {
						t.Fatalf("missing %s", required)
					}
				}
				joined := string(entries[rootName+"Install-Maestro.ps1"]) + string(entries[rootName+"Start Maestro.cmd"])
				startHere := string(entries[rootName+"COMECE-AQUI.txt"])
				for _, required := range []string{"RevealWorkspace", "Show-StableWorkspace", "Invoke-Item -LiteralPath $Workspace"} {
					if !strings.Contains(joined, required) {
						t.Fatalf("Windows quick-start is missing %q", required)
					}
				}
				if !strings.Contains(startHere, "Start Maestro.cmd") || !strings.Contains(startHere, "Se o duplo clique") {
					t.Fatalf("Windows start-here does not lead with launcher and preserve the Claude fallback")
				}
				for _, required := range []string{"UTF8Encoding", "MAESTRO-SCRIPT-ENCODING", "ReadAllBytes"} {
					if !strings.Contains(joined, required) {
						t.Fatalf("Windows installer is missing strict CLAUDE.md encoding guard %q", required)
					}
				}
				for _, forbidden := range []string{"ExecutionPolicy Bypass", "Set-ExecutionPolicy", "Unblock-File", "go.exe"} {
					if strings.Contains(joined, forbidden) {
						t.Fatalf("Windows installer contains forbidden %q", forbidden)
					}
				}
			}
			settings := string(entries[rootName+"projection/settings.local.json"])
			seed := string(entries[rootName+"maestro-os/CLAUDE.md"])
			if strings.Contains(seed, "reveal-workspace") || strings.Contains(seed, "RevealWorkspace") {
				t.Fatalf("Claude-first fallback must not launch Finder or Explorer")
			}
			capabilities := string(entries[rootName+"capabilities.json"])
			if !strings.Contains(capabilities, "agent_route_lite_v1") || !strings.Contains(capabilities, "managed_agent_route_completion_assurance") {
				t.Fatalf("script kit does not declare the degraded route contract")
			}
			handlerName := "maestro-hook.sh"
			if target == "windows" {
				handlerName = "Maestro-Hook.ps1"
			}
			handler := string(entries[rootName+handlerName])
			for _, required := range []string{"agent-route-lite", "transition_count", "stop_hook_active", "client-account-agent", "case-agent", "idle:walter", "darwin", "route state is busy or requires repair before finishing"} {
				if !strings.Contains(handler, required) {
					t.Fatalf("%s lacks route-lite contract %q", handlerName, required)
				}
			}
			if target == "macos" {
				for _, required := range []string{"/usr/bin/head -c 65537", "/usr/bin/base64", "route_payload_base64", "route_input"} {
					if !strings.Contains(handler, required) {
						t.Fatalf("POSIX route hook lacks bounded stdin reader %q", required)
					}
				}
				for _, forbidden := range []string{"route_payload=$(cat)", "maestro-route-input", "maestro-route-xml"} {
					if strings.Contains(handler, forbidden) {
						t.Fatalf("POSIX route hook persists or materializes unbounded hook input through %q", forbidden)
					}
				}
			}
			if target == "windows" {
				for _, required := range []string{"Read-BoundedHookInput", "OpenStandardInput", "65537"} {
					if !strings.Contains(handler, required) {
						t.Fatalf("PowerShell route hook lacks bounded stdin reader %q", required)
					}
				}
				if strings.Contains(handler, "[Console]::In.ReadToEnd()") {
					t.Fatal("PowerShell route hook materializes stdin before enforcing its bound")
				}
			}
			for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop", "SubagentStart", "SubagentStop"} {
				if !strings.Contains(settings, `"`+event+`"`) {
					t.Fatalf("script kit is missing %s hook", event)
				}
			}
			for name, body := range entries {
				lower := strings.ToLower(name)
				if strings.HasSuffix(lower, ".go") || strings.HasSuffix(lower, ".exe") || strings.HasSuffix(lower, ".dll") ||
					bytes.HasPrefix(body, []byte("MZ")) || bytes.HasPrefix(body, []byte{0xcf, 0xfa, 0xed, 0xfe}) ||
					bytes.HasPrefix(body, []byte{0xca, 0xfe, 0xba, 0xbe}) {
					t.Fatalf("script kit contains native/compiler payload %s", name)
				}
			}

			second := filepath.Join(t.TempDir(), filepath.Base(output))
			options.Output = second
			secondResult, err := BuildScriptPortable(options)
			if err != nil {
				t.Fatal(err)
			}
			if secondResult.SHA256 != result.SHA256 {
				t.Fatalf("non-deterministic ZIP: %s != %s", secondResult.SHA256, result.SHA256)
			}
		})
	}
}

func TestWindowsArtifactAcceptanceUsesRealZIPsWithoutPolicyBypass(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	body := string(mustReadScriptFile(t, filepath.Join(root, "dev", "acceptance", "script-only-windows-artifact.ps1")))
	for _, required := range []string{"PreviousZip", "CandidateZip", "PreviousVersion", "CandidateVersion", "Confirm-ArchiveDigest", "Expand-Archive", "Get-ExecutionPolicy -List", "Convert-HookJson", "Confirm-EncodingRejectedWithoutWorkspaceMutation $candidateInstaller", "[IO.File]::WriteAllBytes", "[Convert]::ToBase64String", "utf8_bom_rejected_without_workspace_mutation", "invalid_utf8_rejected_without_workspace_mutation", "Invoke-StartLauncher", "$env:ComSpec", "WriteLine('S')", "nontechnical_launcher_install", "$output = $InputBody | & powershell.exe @allArguments", "SessionStart nao comprovou perfil bounded", "subagent-stop", "hook-events.jsonl", `@('PostToolUse','Stop','SubagentStart','SubagentStop')`, "agent_route_lite_strategic_completion", "agent_route_lite_direct_walter", "agent_route_lite_stop_fail_closed", "Confirm-BoundedHookInputBeforeEOF", "agent_route_lite_input_bounded_before_eof", "route-direct-walter-stop", "route-string-escape", "route-busy-stop", "CLIENT PROMPT MUST NOT PERSIST", "rollback", "owner_sentinel_preserved", "reviewed_session_profile_preserved"} {
		if !strings.Contains(body, required) {
			t.Fatalf("Windows artifact acceptance is missing %q", required)
		}
	}
	if regexp.MustCompile(`(?i)powershell(?:\.exe)?[^\r\n]*\s-ExecutionPolicy\b`).MatchString(body) {
		t.Fatal("Windows artifact acceptance passes an execution-policy override")
	}
	for _, forbidden := range []string{"Set-ExecutionPolicy", "Unblock-File", "RunAs"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("Windows artifact acceptance contains forbidden %q", forbidden)
		}
	}
}

func TestWindowsScriptPortableHostedAcceptanceRunsUnderStandardUser(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	workflow := string(mustReadScriptFile(t, filepath.Join(root, ".github", "workflows", "windows-script-portable.yml")))
	runner := string(mustReadScriptFile(t, filepath.Join(root, "dev", "acceptance", "run-script-only-windows-ci.ps1")))

	for _, required := range []string{
		"windows-latest",
		"workflow_dispatch:",
		"portable-script-windows",
		"0.1.18",
		"run-script-only-windows-ci.ps1",
		"windows-script-preview18-${status}",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("Windows hosted acceptance workflow is missing %q", required)
		}
	}
	for _, required := range []string{
		"New-LocalUser",
		"Remove-LocalUser",
		"WindowsBuiltInRole]::Administrator",
		"TestWindowsScriptPortableInstallsUpdatesRollsBackAndRunsSevenHooksAsUnelevatedUser",
		"script-only-windows-artifact.ps1",
		"MAESTRO-WINDOWS-CI",
	} {
		if !strings.Contains(runner, required) {
			t.Fatalf("Windows standard-user runner is missing %q", required)
		}
	}
	for _, forbidden := range []string{"ExecutionPolicy Bypass", "Set-ExecutionPolicy", "Unblock-File", "RunAs"} {
		if strings.Contains(workflow, forbidden) || strings.Contains(runner, forbidden) {
			t.Fatalf("Windows hosted acceptance contains forbidden policy/elevation surface %q", forbidden)
		}
	}
}

func TestScriptPortablePOSIXHookProtectsManagedPathsWithoutBlockingOwnerSkill(t *testing.T) {
	workspace := t.TempDir()
	managedState := filepath.Join(workspace, ".maestro-script")
	if err := os.MkdirAll(managedState, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(managedState, "managed-skills"), []byte("maestro-onboarding\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(t.TempDir(), "maestro-hook.sh")
	if err := os.WriteFile(hook, scriptPortablePOSIXHook(), 0o700); err != nil {
		t.Fatal(err)
	}
	task := filepath.Join(workspace, "brain", "tasks", "review.md")
	if err := os.MkdirAll(filepath.Dir(task), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(task, []byte("PRIVATE BODY MUST NOT ENTER THE HOOK\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	continuityPath := filepath.Join(managedState, "continuity-state.json")
	continuity := `{"schema_version":1,"state":"active","revision":1,"task":"brain/tasks/review.md","checkpoint_present":false}` + "\n"
	if err := os.WriteFile(continuityPath, []byte(continuity), 0o600); err != nil {
		t.Fatal(err)
	}
	profileBody := []byte("PRIVATE PROFESSIONAL PROFILE BODY MUST NOT ENTER THE HOOK\n")
	profilePath := filepath.Join(managedState, "local-profile.md")
	if err := os.WriteFile(profilePath, profileBody, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(profileBody)
	sessionProfile := `{"schema_version":1,"interaction_profile":"power","revision":4,"local_profile":".maestro-script/local-profile.md","profile_sha256":"` + hex.EncodeToString(digest[:]) + `","session_use_confirmed":true}` + "\n"
	if err := os.WriteFile(filepath.Join(managedState, "session-profile.json"), []byte(sessionProfile), 0o600); err != nil {
		t.Fatal(err)
	}
	session := exec.Command("/bin/sh", hook, "session-start", workspace)
	if output, err := session.CombinedOutput(); err != nil || !strings.Contains(string(output), "brain/tasks/review.md") || !strings.Contains(string(output), "interaction profile: power") || !strings.Contains(string(output), ".maestro-script/local-profile.md") || !strings.Contains(string(output), "revision: 4") || strings.Contains(string(output), "PRIVATE BODY") || strings.Contains(string(output), hex.EncodeToString(digest[:])) {
		t.Fatalf("bounded continuity context = %v, %s", err, output)
	}
	for _, invalidState := range []string{
		`{"schema_version":"1","state":"active","revision":1,"task":"brain/tasks/review.md","checkpoint_present":false}` + "\n",
		`{"schema_version":1,"state":"active","revision":"1","task":"brain/tasks/review.md","checkpoint_present":false}` + "\n",
		`{"schema_version":1,"state":"active","revision":1,"task":"brain/tasks/review.md","checkpoint_present":"false"}` + "\n",
	} {
		if err := os.WriteFile(continuityPath, []byte(invalidState), 0o600); err != nil {
			t.Fatal(err)
		}
		invalidTypes := exec.Command("/bin/sh", hook, "session-start", workspace)
		if output, err := invalidTypes.CombinedOutput(); err != nil || !strings.Contains(string(output), "CONTINUITY LITE REPAIR REQUIRED") || strings.Contains(string(output), "reviewed task pointer: brain/tasks/review.md") {
			t.Fatalf("continuity accepted a coerced JSON type: %v, %s", err, output)
		}
	}
	if err := os.WriteFile(continuityPath, []byte(continuity), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, invalidState := range []string{
		`{"schema_version":1,"interaction_profile":"power","revision":4,"local_profile":".maestro-script/local-profile.md","profile_sha256":"` + hex.EncodeToString(digest[:]) + `","session_use_confirmed":"true"}` + "\n",
		`{"schema_version":1,"interaction_profile":"power","revision":"4","local_profile":".maestro-script/local-profile.md","profile_sha256":"` + hex.EncodeToString(digest[:]) + `","session_use_confirmed":true}` + "\n",
	} {
		if err := os.WriteFile(filepath.Join(managedState, "session-profile.json"), []byte(invalidState), 0o600); err != nil {
			t.Fatal(err)
		}
		invalidTypes := exec.Command("/bin/sh", hook, "session-start", workspace)
		if output, err := invalidTypes.CombinedOutput(); err != nil || !strings.Contains(string(output), "SESSION PROFILE REPAIR REQUIRED") || strings.Contains(string(output), "interaction profile: power") {
			t.Fatalf("session profile accepted a coerced JSON type: %v, %s", err, output)
		}
	}
	if err := os.WriteFile(filepath.Join(managedState, "session-profile.json"), []byte(sessionProfile), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profilePath, []byte("tampered profile body\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	invalidProfileSession := exec.Command("/bin/sh", hook, "session-start", workspace)
	if output, err := invalidProfileSession.CombinedOutput(); err != nil || !strings.Contains(string(output), "SESSION PROFILE REPAIR REQUIRED") || !strings.Contains(string(output), "standard") || strings.Contains(string(output), "tampered profile body") {
		t.Fatalf("invalid profile fail-closed context = %v, %s", err, output)
	}
	for _, test := range []struct {
		name    string
		payload string
		denied  bool
	}{
		{name: "managed settings", payload: `{"tool_name":"Write","tool_input":{"file_path":".claude/settings.local.json"}}`, denied: true},
		{name: "managed skill", payload: `{"tool_name":"Write","tool_input":{"file_path":".claude/skills/maestro-onboarding/SKILL.md"}}`, denied: true},
		{name: "owner skill", payload: `{"tool_name":"Write","tool_input":{"file_path":".claude/skills/owner-custom/SKILL.md"}}`, denied: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command("/bin/sh", hook, "pre-action-guard", workspace)
			command.Stdin = strings.NewReader(test.payload)
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("hook failed: %v, %s", err, output)
			}
			if got := strings.Contains(string(output), `"permissionDecision":"deny"`); got != test.denied {
				t.Fatalf("denied = %v, want %v: %s", got, test.denied, output)
			}
		})
	}
}

func TestScriptPortablePOSIXAgentRouteLiteEnforcesStrategicCompletionWithoutContent(t *testing.T) {
	workspace := t.TempDir()
	stateRoot := filepath.Join(workspace, ".maestro-script")
	if err := os.MkdirAll(stateRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(t.TempDir(), "maestro-hook.sh")
	if err := os.WriteFile(hook, scriptPortablePOSIXHook(), 0o700); err != nil {
		t.Fatal(err)
	}
	call := func(event, payload string) (string, error) {
		command := exec.Command("/bin/sh", hook, event, workspace)
		command.Stdin = strings.NewReader(payload)
		output, err := command.CombinedOutput()
		return string(output), err
	}
	sessionID := "SESSION SECRET MUST BE DIGESTED"
	input := func(agentID, agentType string) string {
		return `{"session_id":"` + sessionID + `","agent_id":"` + agentID + `","agent_type":"` + agentType + `"}`
	}
	if output, err := call("context-injection", `{"session_id":"`+sessionID+`","prompt":"CLIENT PROMPT MUST NOT PERSIST"}`); err != nil || !strings.Contains(output, "agent-route-lite") {
		t.Fatalf("route begin = %v, %s", err, output)
	}
	if _, err := call("subagent-start", input("account-secret-1", "client-account-agent")); err != nil {
		t.Fatalf("account start: %v", err)
	}
	if output, err := call("stop-finalization", `{"session_id":"`+sessionID+`"}`); err != nil || !strings.Contains(output, `"decision":"block"`) || !strings.Contains(output, "still active") {
		t.Fatalf("active specialist stop gate = %v, %s", err, output)
	}
	if _, err := call("subagent-stop", input("account-secret-1", "client-account-agent")); err != nil {
		t.Fatalf("account framing stop: %v", err)
	}
	if output, err := call("stop-finalization", `{"session_id":"`+sessionID+`"}`); err != nil || !strings.Contains(output, "call Case Agent") {
		t.Fatalf("strategic case gate = %v, %s", err, output)
	}
	if _, err := call("subagent-start", input("case-secret-1", "case-agent")); err != nil {
		t.Fatalf("case start: %v", err)
	}
	if _, err := call("subagent-stop", input("case-secret-1", "case-agent")); err != nil {
		t.Fatalf("case stop: %v", err)
	}
	if output, err := call("stop-finalization", `{"session_id":"`+sessionID+`"}`); err != nil || !strings.Contains(output, "return the Case result") {
		t.Fatalf("account validation gate = %v, %s", err, output)
	}
	if _, err := call("subagent-start", input("account-secret-2", "client-account-agent")); err != nil {
		t.Fatalf("account validation start: %v", err)
	}
	if _, err := call("subagent-stop", input("account-secret-2", "client-account-agent")); err != nil {
		t.Fatalf("account validation stop: %v", err)
	}
	if output, err := call("stop-finalization", `{"session_id":"`+sessionID+`"}`); err != nil || !strings.Contains(output, `"continue":true`) {
		t.Fatalf("completed strategic route = %v, %s", err, output)
	}

	routeFiles, err := filepath.Glob(filepath.Join(stateRoot, "agent-route-lite", "*.json"))
	if err != nil || len(routeFiles) != 1 {
		t.Fatalf("route state files = %v, %v", routeFiles, err)
	}
	body := mustReadScriptFile(t, routeFiles[0])
	if len(body) > 2048 || bytes.Contains(body, []byte(sessionID)) || bytes.Contains(body, []byte("account-secret")) || bytes.Contains(body, []byte("case-secret")) || bytes.Contains(body, []byte("CLIENT PROMPT")) {
		t.Fatalf("route state leaked content or exceeded bound: %s", body)
	}

	if _, err := call("context-injection", `{"session_id":"`+sessionID+`","prompt":"new turn"}`); err != nil {
		t.Fatalf("new turn reset: %v", err)
	}
	if _, err := call("subagent-start", input("darwin-secret", "darwin")); err != nil {
		t.Fatalf("darwin start: %v", err)
	}
	if _, err := call("subagent-stop", input("darwin-secret", "darwin")); err != nil {
		t.Fatalf("darwin stop: %v", err)
	}
	if output, err := call("subagent-start", input("case-after-darwin", "case-agent")); err == nil || !strings.Contains(output, "cannot be mixed") {
		t.Fatalf("Darwin/client mixing accepted: %v, %s", err, output)
	}
	if output, err := call("stop-finalization", `{"session_id":"`+sessionID+`","stop_hook_active":true}`); err != nil || !strings.Contains(output, `"continue":true`) {
		t.Fatalf("reentrant Stop was not allowed: %v, %s", err, output)
	}

	if _, err := call("context-injection", `{"session_id":"`+sessionID+`","prompt":"direct Walter review"}`); err != nil {
		t.Fatalf("route reset for direct Walter: %v", err)
	}
	if _, err := call("subagent-start", input("walter-direct-secret", "walter")); err != nil {
		t.Fatalf("direct Walter start: %v", err)
	}
	if _, err := call("subagent-stop", input("walter-direct-secret", "walter")); err != nil {
		t.Fatalf("direct Walter stop: %v", err)
	}
	if output, err := call("stop-finalization", `{"session_id":"`+sessionID+`"}`); err != nil || !strings.Contains(output, `"continue":true`) {
		t.Fatalf("direct Walter leaf did not complete: %v, %s", err, output)
	}

	if _, err := call("context-injection", `{"session_id":"`+sessionID+`","prompt":"lock and type tests"}`); err != nil {
		t.Fatalf("route reset for fail-closed cases: %v", err)
	}
	if _, err := call("subagent-start", input("account-lock-secret", "client-account-agent")); err != nil {
		t.Fatalf("account start for fail-closed cases: %v", err)
	}
	if output, err := call("stop-finalization", `{"session_id":"`+sessionID+`","stop_hook_active":"true"}`); err != nil || !strings.Contains(output, `"decision":"block"`) {
		t.Fatalf("string stop_hook_active bypassed route: %v, %s", err, output)
	}
	routeFiles, err = filepath.Glob(filepath.Join(stateRoot, "agent-route-lite", "*.json"))
	if err != nil || len(routeFiles) != 1 {
		t.Fatalf("route state for lock regression = %v, %v", routeFiles, err)
	}
	lockPath := routeFiles[0] + ".lock"
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := call("stop-finalization", `{"session_id":"`+sessionID+`"}`); err != nil || !strings.Contains(output, `"decision":"block"`) || !strings.Contains(output, "busy or requires repair") {
		t.Fatalf("busy route Stop failed open: %v, %s", err, output)
	}
	if info, err := os.Stat(lockPath); err != nil || !info.IsDir() {
		t.Fatalf("losing route call removed another process lock: %v", err)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}
	originalState := mustReadScriptFile(t, routeFiles[0])
	for name, invalidState := range map[string][]byte{
		"string schema": bytes.Replace(originalState, []byte(`"schema_version":1`), []byte(`"schema_version":"1"`), 1),
		"string count":  bytes.Replace(originalState, []byte(`"transition_count":1`), []byte(`"transition_count":"1"`), 1),
		"nonhex digest": bytes.Replace(originalState, []byte(`"active_agent_digest":"`), []byte(`"active_agent_digest":"zz`), 1),
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(routeFiles[0], invalidState, 0o600); err != nil {
				t.Fatal(err)
			}
			output, err := call("stop-finalization", `{"session_id":"`+sessionID+`"}`)
			if err != nil || !strings.Contains(output, `"decision":"block"`) || !strings.Contains(output, "requires repair") {
				t.Fatalf("malformed route state failed open: %v, %s", err, output)
			}
		})
	}
}

func TestScriptPortablePOSIXAgentRouteLiteStopsReadingAtItsInputBound(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, ".maestro-script"), 0o700); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(t.TempDir(), "maestro-hook.sh")
	if err := os.WriteFile(hook, scriptPortablePOSIXHook(), 0o700); err != nil {
		t.Fatal(err)
	}

	scratch := t.TempDir()
	prefix := []byte(`{"session_id":"oversized-without-eof"}`)
	input := append(prefix, bytes.Repeat([]byte{' '}, 65537-len(prefix))...)
	command := exec.Command("/bin/sh", hook, "context-injection", workspace)
	command.Env = append(os.Environ(), "TMPDIR="+scratch)
	output, err := runCommandWithOpenStdin(t, command, input, 5*time.Second)
	if err != nil {
		t.Fatalf("oversized hook input failed unexpectedly: %v, %s", err, output)
	}
	if !strings.Contains(output, `"hookEventName":"UserPromptSubmit"`) || !strings.Contains(output, "Native route authority remains unavailable") {
		t.Fatalf("oversized prompt hook did not preserve bounded fallback behavior: %s", output)
	}
	entries, err := os.ReadDir(scratch)
	if err != nil || len(entries) != 0 {
		t.Fatalf("oversized prompt input left temporary content: %v, %v", entries, err)
	}
	routeEntries, err := os.ReadDir(filepath.Join(workspace, ".maestro-script"))
	if err != nil || len(routeEntries) != 0 {
		t.Fatalf("oversized prompt input created route state: %v, %v", routeEntries, err)
	}

	exactWorkspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(exactWorkspace, ".maestro-script"), 0o700); err != nil {
		t.Fatal(err)
	}
	exactPrefix := []byte(`{"session_id":"exact-limit"}`)
	exactInput := append(exactPrefix, bytes.Repeat([]byte{' '}, 65536-len(exactPrefix))...)
	exact := exec.Command("/bin/sh", hook, "context-injection", exactWorkspace)
	exact.Stdin = bytes.NewReader(exactInput)
	exactOutput, exactErr := exact.CombinedOutput()
	if exactErr != nil || !strings.Contains(string(exactOutput), "agent-route-lite enforces") {
		t.Fatalf("exactly 64 KiB hook input was not accepted: %v, %s", exactErr, exactOutput)
	}

	nulWorkspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(nulWorkspace, ".maestro-script"), 0o700); err != nil {
		t.Fatal(err)
	}
	nulPrefix := []byte(`{"session_id":"nul-overflow"}`)
	nulInput := append(nulPrefix, bytes.Repeat([]byte{0}, 65537-len(nulPrefix))...)
	nul := exec.Command("/bin/sh", hook, "context-injection", nulWorkspace)
	nul.Stdin = bytes.NewReader(nulInput)
	nulOutput, nulErr := nul.CombinedOutput()
	if nulErr != nil || !strings.Contains(string(nulOutput), "Native route authority remains unavailable") || strings.Contains(string(nulOutput), "agent-route-lite enforces") {
		t.Fatalf("NUL-padded overflow was not rejected: %v, %s", nulErr, nulOutput)
	}
	nulEntries, err := os.ReadDir(filepath.Join(nulWorkspace, ".maestro-script"))
	if err != nil || len(nulEntries) != 0 {
		t.Fatalf("NUL-padded overflow created route state: %v, %v", nulEntries, err)
	}
}

func TestMacOSScriptPortableInstallsUpdatesAndRollsBackWithoutGo(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("executes packaged POSIX shell lifecycle")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	packageRoot := extractScriptPortable(t, buildScriptPortableFixture(t, root, "0.2.0", "macos"))
	workspace := filepath.Join(t.TempDir(), "owner-workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	runScriptPortable(t, packageRoot, home, "install", "--workspace", workspace)
	runScriptPortable(t, packageRoot, home, "install")
	stateRoot := filepath.Join(home, "Library", "Application Support", "Maestro", "script-runtime", "state")
	if got := strings.TrimSpace(string(mustReadScriptFile(t, filepath.Join(stateRoot, "active-version")))); got != "0.2.0" {
		t.Fatalf("active version = %q", got)
	}
	claude := string(mustReadScriptFile(t, filepath.Join(workspace, "CLAUDE.md")))
	if !strings.Contains(claude, "MAESTRO SCRIPT MANAGED BEGIN") || !strings.Contains(claude, "capabilities.json") {
		t.Fatalf("workspace projection missing: %s", claude)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".claude", "skills", "maestro-onboarding", "SKILL.md")); err != nil {
		t.Fatalf("managed skill missing: %v", err)
	}
	for _, agent := range []string{"client-account-agent.md", "case-agent.md", "walter.md", "darwin.md", "pa-expert.md"} {
		body, err := os.ReadFile(filepath.Join(workspace, ".claude", "agents", agent))
		if err != nil || !strings.Contains(string(body), "BCGOS:MANAGED-CLAUDE-AGENT") {
			t.Fatalf("operational managed agent %s missing: %s, %v", agent, body, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, ".claude", "agents", "maestro.md")); !os.IsNotExist(err) {
		t.Fatalf("Maestro must remain the main-session identity: %v", err)
	}
	settingsPath := filepath.Join(workspace, ".claude", "settings.local.json")
	if body, err := os.ReadFile(settingsPath); err != nil || !strings.Contains(string(body), `"SessionStart"`) || !strings.Contains(string(body), `"SubagentStop"`) {
		t.Fatalf("script hooks were not projected: %s, %v", body, err)
	}
	projectionReceipt := filepath.Join(workspace, ".maestro-script", "projection-receipt.json")
	if body, err := os.ReadFile(projectionReceipt); err != nil || !strings.Contains(string(body), `"state":"configured_on_disk"`) || !strings.Contains(string(body), `"version":"0.2.0"`) {
		t.Fatalf("bounded projection receipt missing: %s, %v", body, err)
	}
	managedSkill := filepath.Join(workspace, ".claude", "skills", "maestro-onboarding", "SKILL.md")
	originalManagedSkill := mustReadScriptFile(t, managedSkill)
	if err := os.Remove(managedSkill); err != nil {
		t.Fatal(err)
	}
	projectionDoctor := exec.Command("/bin/sh", filepath.Join(packageRoot, "install.sh"), "doctor")
	projectionDoctor.Env = append(os.Environ(), "HOME="+home)
	if output, err := projectionDoctor.CombinedOutput(); err == nil || !strings.Contains(string(output), "MAESTRO-SCRIPT-DOCTOR") {
		t.Fatalf("doctor accepted a changed managed skill: %v, %s", err, output)
	}
	runScriptPortable(t, packageRoot, home, "install")
	if got := mustReadScriptFile(t, managedSkill); !bytes.Equal(got, originalManagedSkill) {
		t.Fatalf("rerun did not repair managed skill: %q", got)
	}
	ownerModifiedSkill := []byte("owner-modified managed skill\n")
	if err := os.WriteFile(managedSkill, ownerModifiedSkill, 0o600); err != nil {
		t.Fatal(err)
	}
	conflictingRepair := exec.Command("/bin/sh", filepath.Join(packageRoot, "install.sh"), "install")
	conflictingRepair.Env = append(os.Environ(), "HOME="+home)
	if output, err := conflictingRepair.CombinedOutput(); err == nil || !strings.Contains(string(output), "MAESTRO-SCRIPT-CONFLICT") {
		t.Fatalf("rerun overwrote an owner-modified managed skill: %v, %s", err, output)
	}
	if got := mustReadScriptFile(t, managedSkill); !bytes.Equal(got, ownerModifiedSkill) {
		t.Fatalf("conflict did not preserve owner-modified managed skill: %q", got)
	}
	if err := os.WriteFile(managedSkill, originalManagedSkill, 0o600); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(workspace, ".maestro-script", "hooks", "maestro-hook.sh")
	taskPath := filepath.Join(workspace, "brain", "tasks", "market-review.md")
	if err := os.MkdirAll(filepath.Dir(taskPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(taskPath, []byte("# Confidential task body\n\n## Checkpoint\nContinue with reviewed evidence.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	continuityState := []byte(`{"schema_version":1,"state":"paused","revision":2,"task":"brain/tasks/market-review.md","checkpoint_present":true}` + "\n")
	if err := os.WriteFile(filepath.Join(workspace, ".maestro-script", "continuity-state.json"), continuityState, 0o600); err != nil {
		t.Fatal(err)
	}
	session := exec.Command("/bin/sh", hookPath, "session-start", workspace)
	if output, err := session.CombinedOutput(); err != nil || !strings.Contains(string(output), `"hookEventName":"SessionStart"`) || !strings.Contains(string(output), `brain/tasks/market-review.md`) || !strings.Contains(string(output), `checkpoint present: true`) || strings.Contains(string(output), "Confidential task body") {
		t.Fatalf("session hook = %v, %s", err, output)
	}
	contextHook := exec.Command("/bin/sh", hookPath, "context-injection", workspace)
	if output, err := contextHook.CombinedOutput(); err != nil || !strings.Contains(string(output), `"hookEventName":"UserPromptSubmit"`) {
		t.Fatalf("context hook = %v, %s", err, output)
	}
	guard := exec.Command("/bin/sh", hookPath, "pre-action-guard", workspace)
	guard.Stdin = strings.NewReader(`{"tool_name":"Bash","tool_input":{"command":"rm -rf /"}}`)
	if output, err := guard.CombinedOutput(); err != nil || !strings.Contains(string(output), `"permissionDecision":"deny"`) {
		t.Fatalf("pre-action hook = %v, %s", err, output)
	}
	for _, payload := range []string{
		`{"tool_name":"Write","tool_input":{"file_path":".claude/settings.local.json"}}`,
		`{"tool_name":"Edit","tool_input":{"file_path":".claude/agents/case-agent.md"}}`,
		`{"tool_name":"Write","tool_input":{"file_path":".claude/skills/maestro-onboarding/SKILL.md"}}`,
		`{"tool_name":"Bash","tool_input":{"command":"rm -rf .claude"}}`,
	} {
		guard := exec.Command("/bin/sh", hookPath, "pre-action-guard", workspace)
		guard.Stdin = strings.NewReader(payload)
		if output, err := guard.CombinedOutput(); err != nil || !strings.Contains(string(output), `"permissionDecision":"deny"`) {
			t.Fatalf("relative managed path was not denied: %v, %s", err, output)
		}
	}
	ownerSkillGuard := exec.Command("/bin/sh", hookPath, "pre-action-guard", workspace)
	ownerSkillGuard.Stdin = strings.NewReader(`{"tool_name":"Write","tool_input":{"file_path":".claude/skills/owner-custom/SKILL.md"}}`)
	if output, err := ownerSkillGuard.CombinedOutput(); err != nil || strings.Contains(string(output), `"permissionDecision":"deny"`) {
		t.Fatalf("owner skill was incorrectly denied: %v, %s", err, output)
	}
	observe := exec.Command("/bin/sh", hookPath, "post-action-receipt", workspace)
	observe.Stdin = strings.NewReader(`{"session_id":"session-a","tool_input":{"command":"CLIENT SECRET"}}`)
	if output, err := observe.CombinedOutput(); err != nil {
		t.Fatalf("post-action hook = %v, %s", err, output)
	}
	events := string(mustReadScriptFile(t, filepath.Join(workspace, ".maestro-script", "hook-events.jsonl")))
	if !strings.Contains(events, `"event":"PostToolUse"`) || strings.Contains(events, "CLIENT SECRET") {
		t.Fatalf("metadata-only hook receipt = %s", events)
	}
	for _, hook := range []struct {
		action string
		want   string
	}{
		{action: "stop-finalization", want: `"continue":true`},
		{action: "subagent-start", want: `"hookEventName":"SubagentStart"`},
		{action: "subagent-stop", want: ""},
	} {
		command := exec.Command("/bin/sh", hookPath, hook.action, workspace)
		output, err := command.CombinedOutput()
		if err != nil || (hook.want != "" && !strings.Contains(string(output), hook.want)) {
			t.Fatalf("%s hook = %v, %s", hook.action, err, output)
		}
	}
	events = string(mustReadScriptFile(t, filepath.Join(workspace, ".maestro-script", "hook-events.jsonl")))
	for _, event := range []string{"PostToolUse", "Stop", "SubagentStart", "SubagentStop"} {
		if !strings.Contains(events, `"event":"`+event+`"`) {
			t.Fatalf("missing %s metadata receipt: %s", event, events)
		}
	}
	if err := os.Remove(settingsPath); err != nil {
		t.Fatal(err)
	}
	runScriptPortable(t, packageRoot, home, "install")
	if _, err := os.Stat(settingsPath); err != nil {
		t.Fatalf("reinstall did not repair missing hook settings: %v", err)
	}
	doctorHooks := exec.Command("/bin/sh", filepath.Join(packageRoot, "install.sh"), "doctor")
	doctorHooks.Env = append(os.Environ(), "HOME="+home)
	if output, err := doctorHooks.CombinedOutput(); err != nil || !strings.Contains(string(output), "configured and intact on disk") || !strings.Contains(string(output), "runtime observation pending") {
		t.Fatalf("hook doctor = %v, %s", err, output)
	}
	localProfile := filepath.Join(workspace, ".maestro-script", "local-profile.md")
	if err := os.WriteFile(localProfile, []byte("reviewed local profile\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	claudePath := filepath.Join(workspace, "CLAUDE.md")
	legacyClaude := strings.Replace(string(mustReadScriptFile(t, claudePath)),
		"agent-route-lite provides bounded metadata-only sequence assurance; authenticated native receipts, external-mutation challenges and native signed route authority remain unavailable.",
		"Authenticated native receipts, external-mutation challenges and deterministic specialist-route enforcement remain unavailable.", 1)
	if err := os.WriteFile(claudePath, []byte(legacyClaude), 0o600); err != nil {
		t.Fatal(err)
	}
	var legacyReceipt map[string]any
	if err := json.Unmarshal(mustReadScriptFile(t, projectionReceipt), &legacyReceipt); err != nil {
		t.Fatal(err)
	}
	delete(legacyReceipt, "managed_claude_block_sha256")
	legacyReceiptBody, err := json.Marshal(legacyReceipt)
	if err != nil {
		t.Fatal(err)
	}
	legacyReceiptBody = append(legacyReceiptBody, '\n')
	if err := os.WriteFile(projectionReceipt, legacyReceiptBody, 0o600); err != nil {
		t.Fatal(err)
	}

	nextRoot := extractScriptPortable(t, buildScriptPortableFixture(t, root, "0.3.0", "macos"))
	runScriptPortable(t, nextRoot, home, "install")
	if got := strings.TrimSpace(string(mustReadScriptFile(t, filepath.Join(stateRoot, "previous-version")))); got != "0.2.0" {
		t.Fatalf("previous version = %q", got)
	}
	if got := string(mustReadScriptFile(t, localProfile)); got != "reviewed local profile\n" {
		t.Fatalf("update changed local profile: %q", got)
	}
	olderRoot := extractScriptPortable(t, buildScriptPortableFixture(t, root, "0.1.0", "macos"))
	downgrade := exec.Command("/bin/sh", filepath.Join(olderRoot, "install.sh"), "install")
	downgrade.Env = append(os.Environ(), "HOME="+home)
	if output, err := downgrade.CombinedOutput(); err == nil || !strings.Contains(string(output), "MAESTRO-SCRIPT-VERSION") {
		t.Fatalf("install accepted downgrade: %v, %s", err, output)
	}
	activeSkill := filepath.Join(home, "Library", "Application Support", "Maestro", "script-runtime", "releases", "0.3.0", "projection", "skills", "maestro-onboarding", "SKILL.md")
	originalSkill := mustReadScriptFile(t, activeSkill)
	if err := os.WriteFile(activeSkill, []byte("tampered runtime\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	doctor := exec.Command("/bin/sh", filepath.Join(home, "Library", "Application Support", "Maestro", "script-runtime", "releases", "0.3.0", "install.sh"), "doctor")
	doctor.Env = append(os.Environ(), "HOME="+home)
	if output, err := doctor.CombinedOutput(); err == nil || !strings.Contains(string(output), "MAESTRO-SCRIPT-RUNTIME") {
		t.Fatalf("doctor accepted changed runtime: %v, %s", err, output)
	}
	if err := os.WriteFile(activeSkill, originalSkill, 0o600); err != nil {
		t.Fatal(err)
	}
	runScriptPortable(t, nextRoot, home, "rollback")
	if got := strings.TrimSpace(string(mustReadScriptFile(t, filepath.Join(stateRoot, "active-version")))); got != "0.2.0" {
		t.Fatalf("rolled-back active version = %q", got)
	}
}

func TestMacOSScriptPortableRecoversInterruptedWorkspaceProjection(t *testing.T) {
	t.Parallel()
	if testing.Short() {
		t.Skip("executes packaged POSIX shell recovery lifecycle")
	}
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	currentRoot := extractScriptPortable(t, buildScriptPortableFixture(t, root, "0.2.0", "macos"))
	nextRoot := extractScriptPortable(t, buildScriptPortableFixture(t, root, "0.3.0", "macos"))
	home := filepath.Join(t.TempDir(), "home")
	workspace := filepath.Join(t.TempDir(), "owner-workspace")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	runScriptPortable(t, currentRoot, home, "install", "--workspace", workspace)

	ownerClaudePrefix := []byte("# Owner instructions\n\nKeep this section byte-for-byte.\n")
	ownerClaudeSuffix := []byte("\nOwner suffix with deliberate trailing spaces.  \n")
	managedClaude := mustReadScriptFile(t, filepath.Join(workspace, "CLAUDE.md"))
	managedBlock := managedClaude[strings.Index(string(managedClaude), "<!-- MAESTRO SCRIPT MANAGED BEGIN -->"):]
	originalClaude := append(append(append([]byte{}, ownerClaudePrefix...), managedBlock...), ownerClaudeSuffix...)
	if err := os.WriteFile(filepath.Join(workspace, "CLAUDE.md"), originalClaude, 0o600); err != nil {
		t.Fatal(err)
	}
	localProfile := filepath.Join(workspace, ".maestro-script", "local-profile.md")
	continuity := filepath.Join(workspace, ".maestro-script", "continuity-state.json")
	profileBody := []byte("reviewed owner profile\n")
	continuityBody := []byte(`{"schema_version":1,"state":"paused","revision":3,"task":"brain/tasks/recovery.md","checkpoint_present":true}` + "\n")
	if err := os.WriteFile(localProfile, profileBody, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(continuity, continuityBody, 0o600); err != nil {
		t.Fatal(err)
	}

	stateRoot := filepath.Join(home, "Library", "Application Support", "Maestro", "script-runtime", "state")
	lockPath := filepath.Join(stateRoot, "projection-lock")
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	busy := exec.Command("/bin/sh", filepath.Join(currentRoot, "install.sh"), "install")
	busy.Env = append(os.Environ(), "HOME="+home)
	if output, err := busy.CombinedOutput(); err == nil || !strings.Contains(string(output), "MAESTRO-SCRIPT-BUSY") {
		t.Fatalf("installer stole an incomplete projection lock: %v, %s", err, output)
	}
	if info, err := os.Stat(lockPath); err != nil || !info.IsDir() {
		t.Fatalf("busy projection lock was removed: %v, %v", info, err)
	}
	if err := os.Remove(lockPath); err != nil {
		t.Fatal(err)
	}

	injectPOSIXProjectionFailure(t, nextRoot, "after_claude")
	interrupted := exec.Command("/bin/sh", filepath.Join(nextRoot, "install.sh"), "install")
	interrupted.Env = append(os.Environ(), "HOME="+home, "MAESTRO_SCRIPT_TEST_FAILURE_POINT=after_claude")
	interruptedOutput, interruptedErr := interrupted.CombinedOutput()
	if interruptedErr == nil {
		t.Fatalf("failure injection did not interrupt projection: %s", interruptedOutput)
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "projection-transaction", "prepared")); err != nil {
		t.Fatalf("interrupted projection did not retain recovery journal: %v, output=%s", err, interruptedOutput)
	}
	if got := strings.TrimSpace(string(mustReadScriptFile(t, filepath.Join(stateRoot, "active-version")))); got != "0.2.0" {
		t.Fatalf("interrupted projection advanced global active version: %q", got)
	}
	doctor := exec.Command("/bin/sh", filepath.Join(currentRoot, "install.sh"), "doctor")
	doctor.Env = append(os.Environ(), "HOME="+home)
	if output, err := doctor.CombinedOutput(); err == nil || !strings.Contains(string(output), "repair_required") {
		t.Fatalf("doctor did not expose pending recovery: %v, %s", err, output)
	}
	managedSkill := filepath.Join(workspace, ".claude", "skills", "maestro-onboarding", "SKILL.md")
	interruptedSkill := mustReadScriptFile(t, managedSkill)
	unknownHook := filepath.Join(workspace, ".maestro-script", "hooks", "owner-extra.sh")
	unknownHookBody := []byte("owner hook must survive recovery\n")
	if err := os.WriteFile(unknownHook, unknownHookBody, 0o600); err != nil {
		t.Fatal(err)
	}
	hookConflictedRetry := exec.Command("/bin/sh", filepath.Join(nextRoot, "install.sh"), "install")
	hookConflictedRetry.Env = append(os.Environ(), "HOME="+home)
	if output, err := hookConflictedRetry.CombinedOutput(); err == nil || !strings.Contains(string(output), "MAESTRO-SCRIPT-CONFLICT") {
		t.Fatalf("recovery accepted an unknown live hook: %v, %s", err, output)
	}
	if got := mustReadScriptFile(t, unknownHook); !bytes.Equal(got, unknownHookBody) {
		t.Fatalf("recovery conflict changed unknown hook: %q", got)
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "projection-transaction", "prepared")); err != nil {
		t.Fatalf("hook-conflicted recovery discarded its journal: %v", err)
	}
	if err := os.Remove(unknownHook); err != nil {
		t.Fatal(err)
	}
	if got := mustReadScriptFile(t, managedSkill); !bytes.Equal(got, interruptedSkill) {
		t.Fatalf("hook conflict changed a known managed skill: %q", got)
	}
	recoveredThenInterrupted := exec.Command("/bin/sh", filepath.Join(nextRoot, "install.sh"), "install")
	recoveredThenInterrupted.Env = append(os.Environ(), "HOME="+home, "MAESTRO_SCRIPT_TEST_FAILURE_POINT=after_prepared")
	if output, err := recoveredThenInterrupted.CombinedOutput(); err == nil {
		t.Fatalf("second failure injection did not interrupt after recovery: %s", output)
	}
	if got := mustReadScriptFile(t, filepath.Join(workspace, "CLAUDE.md")); !bytes.Equal(got, originalClaude) {
		t.Fatalf("recovery did not restore CLAUDE.md byte-for-byte:\nwant=%q\ngot =%q", originalClaude, got)
	}

	finalRetry := exec.Command("/bin/sh", filepath.Join(nextRoot, "install.sh"), "install")
	finalRetry.Env = append(os.Environ(), "HOME="+home, "MAESTRO_SCRIPT_TEST_TRACE=1")
	finalRetryOutput, finalRetryErr := finalRetry.CombinedOutput()
	if finalRetryErr != nil {
		t.Fatalf("final retry failed: %v, %s", finalRetryErr, finalRetryOutput)
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "projection-transaction")); !os.IsNotExist(err) {
		t.Fatalf("successful retry left recovery journal: %v", err)
	}
	if got := strings.TrimSpace(string(mustReadScriptFile(t, filepath.Join(stateRoot, "active-version")))); got != "0.3.0" {
		t.Fatalf("retry active version = %q, output=%s", got, finalRetryOutput)
	}
	if got := strings.TrimSpace(string(mustReadScriptFile(t, filepath.Join(stateRoot, "previous-version")))); got != "0.2.0" {
		t.Fatalf("retry previous version = %q", got)
	}
	if got := mustReadScriptFile(t, localProfile); !bytes.Equal(got, profileBody) {
		t.Fatalf("recovery changed owner profile: %q", got)
	}
	if got := mustReadScriptFile(t, continuity); !bytes.Equal(got, continuityBody) {
		t.Fatalf("recovery changed continuity: %q", got)
	}
	if got := mustReadScriptFile(t, filepath.Join(workspace, "CLAUDE.md")); !bytes.Contains(got, ownerClaudePrefix) || !bytes.Contains(got, ownerClaudeSuffix) {
		t.Fatalf("recovery changed external CLAUDE content: %q", got)
	}
	installedNext := filepath.Join(home, "Library", "Application Support", "Maestro", "script-runtime", "releases", "0.3.0", "install.sh")
	doctor = exec.Command("/bin/sh", installedNext, "doctor")
	doctor.Env = append(os.Environ(), "HOME="+home)
	if output, err := doctor.CombinedOutput(); err != nil || !strings.Contains(string(output), "configured and intact on disk") {
		t.Fatalf("doctor after recovered update: %v, %s", err, output)
	}
	interruptedRollback := exec.Command("/bin/sh", installedNext, "rollback")
	interruptedRollback.Env = append(os.Environ(), "HOME="+home, "MAESTRO_SCRIPT_TEST_FAILURE_POINT=after_active_pointer")
	if output, err := interruptedRollback.CombinedOutput(); err == nil {
		t.Fatalf("failure injection did not interrupt rollback pointer commit: %s", output)
	}
	if _, err := os.Stat(filepath.Join(stateRoot, "projection-transaction", "prepared")); err != nil {
		t.Fatalf("interrupted rollback did not retain recovery journal: %v", err)
	}
	doctor = exec.Command("/bin/sh", installedNext, "doctor")
	doctor.Env = append(os.Environ(), "HOME="+home)
	if output, err := doctor.CombinedOutput(); err == nil || !strings.Contains(string(output), "repair_required") {
		t.Fatalf("doctor accepted interrupted pointer commit: %v, %s", err, output)
	}
	runScriptPortable(t, filepath.Dir(installedNext), home, "rollback")
	if got := strings.TrimSpace(string(mustReadScriptFile(t, filepath.Join(stateRoot, "active-version")))); got != "0.2.0" {
		t.Fatalf("recovered rollback active version = %q", got)
	}
	if got := strings.TrimSpace(string(mustReadScriptFile(t, filepath.Join(stateRoot, "previous-version")))); got != "0.3.0" {
		t.Fatalf("recovered rollback previous version = %q", got)
	}
	if got := mustReadScriptFile(t, localProfile); !bytes.Equal(got, profileBody) {
		t.Fatalf("rollback recovery changed owner profile: %q", got)
	}
	if got := mustReadScriptFile(t, continuity); !bytes.Equal(got, continuityBody) {
		t.Fatalf("rollback recovery changed continuity: %q", got)
	}
}

func TestWindowsScriptPortableInstallsUpdatesRollsBackAndRunsSevenHooksAsUnelevatedUser(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("requires native Windows PowerShell")
	}
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Fatal("native Windows PowerShell is unavailable")
	}
	identityCheck := exec.Command(powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-Command", `
$identity = [Security.Principal.WindowsIdentity]::GetCurrent()
$principal = New-Object Security.Principal.WindowsPrincipal($identity)
if ($principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) { exit 1 }
`)
	if output, err := identityCheck.CombinedOutput(); err != nil {
		t.Fatalf("native Windows acceptance must run as an unelevated user: %v\n%s", err, output)
	}

	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	testRoot := filepath.Join(t.TempDir(), "Maestro native Windows acceptance with spaces")
	if err := os.MkdirAll(testRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	packageRoot := moveScriptPortableFixture(t, extractScriptPortable(t, buildScriptPortableFixture(t, root, "0.2.0", "windows")), filepath.Join(testRoot, "package 0.2.0"))
	nextPackageRoot := moveScriptPortableFixture(t, extractScriptPortable(t, buildScriptPortableFixture(t, root, "0.3.0", "windows")), filepath.Join(testRoot, "package 0.3.0"))
	runtimeRoot := filepath.Join(testRoot, "Local App Data", "Maestro script runtime")
	workspaceHome := filepath.Join(testRoot, "Owner Workspace")
	workspace := filepath.Join(workspaceHome, "maestro-os")
	environment := []string{"MAESTRO_SCRIPT_HOME=" + runtimeRoot, "MAESTRO_WORKSPACE_HOME=" + workspaceHome}

	install := filepath.Join(packageRoot, "Install-Maestro.ps1")
	nextInstall := filepath.Join(nextPackageRoot, "Install-Maestro.ps1")
	encodingEnvironment := []string{"MAESTRO_SCRIPT_HOME=" + filepath.Join(testRoot, "Encoding Guard", "Runtime")}
	assertEncodingRejected := func(name string, content []byte) {
		t.Helper()
		encodingWorkspace := filepath.Join(testRoot, "Encoding Guard", name)
		if err := os.MkdirAll(encodingWorkspace, 0o700); err != nil {
			t.Fatal(err)
		}
		claudePath := filepath.Join(encodingWorkspace, "CLAUDE.md")
		if err := os.WriteFile(claudePath, content, 0o600); err != nil {
			t.Fatal(err)
		}
		command := exec.Command(powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", nextInstall, "-Action", "install", "-Workspace", encodingWorkspace)
		command.Env = append(os.Environ(), encodingEnvironment...)
		output, err := command.CombinedOutput()
		if err == nil || !strings.Contains(string(output), "MAESTRO-SCRIPT-ENCODING") {
			t.Fatalf("%s encoding guard = %v, %s", name, err, output)
		}
		if got := mustReadScriptFile(t, claudePath); !bytes.Equal(got, content) {
			t.Fatalf("%s encoding guard changed CLAUDE.md bytes: %x", name, got)
		}
		entries, err := os.ReadDir(encodingWorkspace)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 || entries[0].Name() != "CLAUDE.md" {
			t.Fatalf("%s encoding guard mutated workspace before rejection: %v", name, entries)
		}
	}
	assertEncodingRejected("UTF8 BOM Workspace", []byte{0xef, 0xbb, 0xbf, '#', ' ', 'M', 'a', 'e', 's', 't', 'r', 'o', '\n'})
	assertEncodingRejected("Invalid UTF8 Workspace", []byte{'#', ' ', 'M', 'a', 'e', 's', 't', 'r', 'o', '\n', 0xc3, 0x28})

	commandInterpreter := os.Getenv("ComSpec")
	if commandInterpreter == "" {
		commandInterpreter, err = exec.LookPath("cmd.exe")
		if err != nil {
			t.Fatal("native Windows command interpreter is unavailable")
		}
	}
	launcherRuntime := filepath.Join(testRoot, "Candidate Launcher Runtime")
	launcherWorkspaceHome := filepath.Join(testRoot, "Candidate Launcher Workspace")
	launcher := filepath.Join(nextPackageRoot, "Start Maestro.cmd")
	launcherCommand := exec.Command(commandInterpreter, "/d", "/c", launcher)
	launcherCommand.Env = append(os.Environ(), "MAESTRO_SCRIPT_HOME="+launcherRuntime, "MAESTRO_WORKSPACE_HOME="+launcherWorkspaceHome)
	launcherCommand.Stdin = strings.NewReader("S\n\n")
	if output, err := launcherCommand.CombinedOutput(); err != nil || !strings.Contains(string(output), "Maestro preparado") {
		t.Fatalf("nontechnical Start Maestro.cmd install = %v, %s", err, output)
	}
	if got := strings.TrimSpace(string(mustReadScriptFile(t, filepath.Join(launcherRuntime, "state", "active-version")))); got != "0.3.0" {
		t.Fatalf("candidate launcher installed version = %q", got)
	}
	runWindowsPowerShellScript(t, powershell, install, environment, "", "-Action", "install")
	if output := runWindowsPowerShellScript(t, powershell, install, environment, "", "-Action", "install"); !strings.Contains(output, "0.2.0 ja estava preparado") {
		t.Fatalf("idempotent install output = %s", output)
	}

	settingsPath := filepath.Join(workspace, ".claude", "settings.local.json")
	settings := string(mustReadScriptFile(t, settingsPath))
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop", "SubagentStart", "SubagentStop"} {
		if !strings.Contains(settings, `"`+event+`"`) {
			t.Fatalf("installed settings are missing %s: %s", event, settings)
		}
	}
	hook := filepath.Join(workspace, ".maestro-script", "hooks", "Maestro-Hook.ps1")
	boundedWorkspace := filepath.Join(testRoot, "Bounded Hook Input Workspace")
	if err := os.MkdirAll(filepath.Join(boundedWorkspace, ".maestro-script"), 0o700); err != nil {
		t.Fatal(err)
	}
	boundedPrefix := []byte(`{"session_id":"windows-oversized-without-eof"}`)
	boundedInput := append(boundedPrefix, bytes.Repeat([]byte{' '}, 65537-len(boundedPrefix))...)
	boundedCommand := exec.Command(powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", hook, "-Event", "context-injection", "-Workspace", boundedWorkspace)
	boundedOutput, boundedErr := runCommandWithOpenStdin(t, boundedCommand, boundedInput, 15*time.Second)
	if boundedErr != nil || !strings.Contains(boundedOutput, `"hookEventName":"UserPromptSubmit"`) || !strings.Contains(boundedOutput, "Native route authority remains unavailable") {
		t.Fatalf("Windows oversized hook input was not rejected before EOF: %v, %s", boundedErr, boundedOutput)
	}
	boundedEntries, err := os.ReadDir(filepath.Join(boundedWorkspace, ".maestro-script"))
	if err != nil || len(boundedEntries) != 0 {
		t.Fatalf("Windows oversized hook input created route state: %v, %v", boundedEntries, err)
	}
	exactPrefix := `{"session_id":"windows-exact-limit"}`
	exactInput := exactPrefix + strings.Repeat(" ", 65536-len(exactPrefix))
	exactOutput := runWindowsPowerShellScript(t, powershell, hook, nil, exactInput, "-Event", "context-injection", "-Workspace", boundedWorkspace)
	if !strings.Contains(exactOutput, "agent-route-lite enforces") {
		t.Fatalf("Windows exactly 64 KiB hook input was not accepted: %s", exactOutput)
	}
	profileBody := []byte("WINDOWS PRIVATE PROFILE BODY MUST NOT ENTER HOOK\r\n")
	profileDigest := sha256.Sum256(profileBody)
	if err := os.WriteFile(filepath.Join(workspace, ".maestro-script", "local-profile.md"), profileBody, 0o600); err != nil {
		t.Fatal(err)
	}
	sessionProfile := `{"schema_version":1,"interaction_profile":"power","revision":4,"local_profile":".maestro-script/local-profile.md","profile_sha256":"` + hex.EncodeToString(profileDigest[:]) + `","session_use_confirmed":true}`
	if err := os.WriteFile(filepath.Join(workspace, ".maestro-script", "session-profile.json"), []byte(sessionProfile), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		event   string
		input   string
		want    string
		notWant string
	}{
		{event: "session-start", want: `interaction profile: power`, notWant: "WINDOWS PRIVATE PROFILE BODY"},
		{event: "context-injection", want: `"hookEventName":"UserPromptSubmit"`},
		{event: "pre-action-guard", input: `{"tool_name":"Write","tool_input":{"file_path":".claude/settings.local.json"}}`, want: `"permissionDecision":"deny"`},
		{event: "post-action-receipt"},
		{event: "stop-finalization", want: `"continue":true`},
		{event: "subagent-start", want: `"hookEventName":"SubagentStart"`},
		{event: "subagent-stop"},
	} {
		output := runWindowsPowerShellScript(t, powershell, hook, nil, test.input, "-Event", test.event, "-Workspace", workspace)
		if test.want != "" && !strings.Contains(output, test.want) {
			t.Fatalf("%s hook output = %s", test.event, output)
		}
		if test.notWant != "" && strings.Contains(output, test.notWant) {
			t.Fatalf("%s hook leaked private body: %s", test.event, output)
		}
	}
	routeSession := "WINDOWS SESSION SECRET MUST BE DIGESTED"
	routePayload := func(agentID, agentType string) string {
		return `{"session_id":"` + routeSession + `","agent_id":"` + agentID + `","agent_type":"` + agentType + `"}`
	}
	routeCall := func(event, input, want string) {
		t.Helper()
		output := runWindowsPowerShellScript(t, powershell, hook, nil, input, "-Event", event, "-Workspace", workspace)
		if want != "" && !strings.Contains(output, want) {
			t.Fatalf("%s route output = %s", event, output)
		}
	}
	routeCall("context-injection", `{"session_id":"`+routeSession+`","prompt":"WINDOWS CLIENT PROMPT MUST NOT PERSIST"}`, "agent-route-lite")
	routeCall("subagent-start", routePayload("windows-account-1", "client-account-agent"), `"hookEventName":"SubagentStart"`)
	routeCall("stop-finalization", `{"session_id":"`+routeSession+`"}`, `"decision":"block"`)
	routeCall("subagent-stop", routePayload("windows-account-1", "client-account-agent"), "")
	routeCall("stop-finalization", `{"session_id":"`+routeSession+`"}`, "call Case Agent")
	routeCall("subagent-start", routePayload("windows-case-1", "case-agent"), `"hookEventName":"SubagentStart"`)
	routeCall("subagent-stop", routePayload("windows-case-1", "case-agent"), "")
	routeCall("stop-finalization", `{"session_id":"`+routeSession+`"}`, "return the Case result")
	routeCall("subagent-start", routePayload("windows-account-2", "client-account-agent"), `"hookEventName":"SubagentStart"`)
	routeCall("subagent-stop", routePayload("windows-account-2", "client-account-agent"), "")
	routeCall("stop-finalization", `{"session_id":"`+routeSession+`"}`, `"continue":true`)
	routeCall("context-injection", `{"session_id":"`+routeSession+`","prompt":"direct Walter review"}`, "agent-route-lite")
	routeCall("subagent-start", routePayload("windows-walter-direct", "walter"), `"hookEventName":"SubagentStart"`)
	routeCall("subagent-stop", routePayload("windows-walter-direct", "walter"), "")
	routeCall("stop-finalization", `{"session_id":"`+routeSession+`"}`, `"continue":true`)
	routeFiles, err := filepath.Glob(filepath.Join(workspace, ".maestro-script", "agent-route-lite", "*.json"))
	if err != nil || len(routeFiles) != 1 {
		t.Fatalf("Windows route state files = %v, %v", routeFiles, err)
	}
	routeState := mustReadScriptFile(t, routeFiles[0])
	if len(routeState) > 2048 || bytes.Contains(routeState, []byte(routeSession)) || bytes.Contains(routeState, []byte("windows-account")) || bytes.Contains(routeState, []byte("windows-case")) || bytes.Contains(routeState, []byte("WINDOWS CLIENT PROMPT")) {
		t.Fatalf("Windows route state leaked content or exceeded bound: %s", routeState)
	}
	events := string(mustReadScriptFile(t, filepath.Join(workspace, ".maestro-script", "hook-events.jsonl")))
	for _, event := range []string{"PostToolUse", "Stop", "SubagentStart", "SubagentStop"} {
		if !strings.Contains(events, `"event":"`+event+`"`) {
			t.Fatalf("missing %s metadata receipt: %s", event, events)
		}
	}

	injectWindowsProjectionFailure(t, nextPackageRoot)
	interruptedArgs := []string{"-NoLogo", "-NoProfile", "-NonInteractive", "-File", nextInstall, "-Action", "install"}
	interrupted := exec.Command(powershell, interruptedArgs...)
	interrupted.Env = append(os.Environ(), append(environment, "MAESTRO_SCRIPT_TEST_FAILURE_POINT=after_agents")...)
	if output, err := interrupted.CombinedOutput(); err == nil {
		t.Fatalf("Windows failure injection did not interrupt projection: %s", output)
	}
	stateRoot := filepath.Join(runtimeRoot, "state")
	if _, err := os.Stat(filepath.Join(stateRoot, "projection-transaction", "prepared")); err != nil {
		t.Fatalf("Windows interrupted projection did not retain journal: %v", err)
	}
	currentDoctor := exec.Command(powershell, "-NoLogo", "-NoProfile", "-NonInteractive", "-File", install, "-Action", "doctor")
	currentDoctor.Env = append(os.Environ(), environment...)
	if output, err := currentDoctor.CombinedOutput(); err == nil || !strings.Contains(string(output), "repair_required") {
		t.Fatalf("Windows doctor did not expose pending recovery: %v, %s", err, output)
	}
	runWindowsPowerShellScript(t, powershell, nextInstall, environment, "", "-Action", "install")
	if _, err := os.Stat(filepath.Join(stateRoot, "projection-transaction")); !os.IsNotExist(err) {
		t.Fatalf("Windows successful retry left journal: %v", err)
	}
	if got := strings.TrimSpace(string(mustReadScriptFile(t, filepath.Join(stateRoot, "active-version")))); got != "0.3.0" {
		t.Fatalf("updated active version = %q", got)
	}
	if got := strings.TrimSpace(string(mustReadScriptFile(t, filepath.Join(stateRoot, "previous-version")))); got != "0.2.0" {
		t.Fatalf("updated previous version = %q", got)
	}
	if got := mustReadScriptFile(t, filepath.Join(workspace, ".maestro-script", "local-profile.md")); !bytes.Equal(got, profileBody) {
		t.Fatalf("Windows recovered update changed local profile: %q", got)
	}
	if got := strings.TrimSpace(string(mustReadScriptFile(t, filepath.Join(workspace, ".maestro-script", "session-profile.json")))); got != sessionProfile {
		t.Fatalf("Windows recovered update changed session profile: %q", got)
	}
	installedNext := filepath.Join(runtimeRoot, "releases", "0.3.0", "Install-Maestro.ps1")
	if output := runWindowsPowerShellScript(t, powershell, installedNext, environment, "", "-Action", "doctor"); !strings.Contains(output, "configured and intact on disk") {
		t.Fatalf("doctor output = %s", output)
	}
	runWindowsPowerShellScript(t, powershell, installedNext, environment, "", "-Action", "rollback")
	if got := strings.TrimSpace(string(mustReadScriptFile(t, filepath.Join(stateRoot, "active-version")))); got != "0.2.0" {
		t.Fatalf("rolled-back active version = %q", got)
	}
	if got := mustReadScriptFile(t, filepath.Join(workspace, ".maestro-script", "local-profile.md")); !bytes.Equal(got, profileBody) {
		t.Fatalf("Windows rollback changed local profile: %q", got)
	}
	installedPrevious := filepath.Join(runtimeRoot, "releases", "0.2.0", "Install-Maestro.ps1")
	if output := runWindowsPowerShellScript(t, powershell, installedPrevious, environment, "", "-Action", "doctor"); !strings.Contains(output, "configured and intact on disk") {
		t.Fatalf("doctor after rollback output = %s", output)
	}
}

func TestMacOSScriptPortableCreatesStableWorkspaceOutsideExtractedZIP(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	packageRoot := extractScriptPortable(t, buildScriptPortableFixture(t, root, "0.2.0", "macos"))
	home := filepath.Join(t.TempDir(), "home")
	workspaceHome := filepath.Join(home, "permanent-maestro")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", filepath.Join(packageRoot, "install.sh"), "install")
	command.Env = append(os.Environ(), "HOME="+home, "MAESTRO_WORKSPACE_HOME="+workspaceHome)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("stable install: %v\n%s", err, output)
	}
	workspace := filepath.Join(workspaceHome, "maestro-os")
	if !strings.Contains(string(output), "MAESTRO-SCRIPT-WORKSPACE: "+workspace) {
		t.Fatalf("stable handoff missing: %s", output)
	}
	if _, err := os.Stat(filepath.Join(workspace, ".claude", "settings.local.json")); err != nil {
		t.Fatalf("stable workspace hooks missing: %v", err)
	}
	installed := filepath.Join(home, "Library", "Application Support", "Maestro", "script-runtime", "releases", "0.2.0", "install.sh")
	doctor := exec.Command("/bin/sh", installed, "doctor")
	doctor.Env = append(os.Environ(), "HOME="+home, "MAESTRO_WORKSPACE_HOME="+workspaceHome)
	if doctorOutput, err := doctor.CombinedOutput(); err != nil || !strings.Contains(string(doctorOutput), "configured and intact on disk") {
		t.Fatalf("installed doctor after handoff: %v, %s", err, doctorOutput)
	}
}

func TestMacOSScriptPortableQuickStartRevealsOnlyAfterSuccessfulCommit(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	packageRoot := extractScriptPortable(t, buildScriptPortableFixture(t, root, "0.2.0", "macos"))
	revealLog := filepath.Join(t.TempDir(), "reveal.log")
	injectPOSIXRevealProbe(t, packageRoot, revealLog)
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}

	decline := exec.Command("/bin/sh", filepath.Join(packageRoot, "Start Maestro.command"))
	decline.Env = append(os.Environ(), "HOME="+home)
	decline.Stdin = strings.NewReader("n\n")
	if output, err := decline.CombinedOutput(); err != nil || !strings.Contains(string(output), "nada foi alterado") {
		t.Fatalf("quick-start decline = %v, %s", err, output)
	}
	if _, err := os.Stat(revealLog); !os.IsNotExist(err) {
		t.Fatalf("declined quick-start revealed a workspace: %v", err)
	}

	accept := exec.Command("/bin/sh", filepath.Join(packageRoot, "Start Maestro.command"))
	accept.Env = append(os.Environ(), "HOME="+home)
	accept.Stdin = strings.NewReader("s\n")
	if output, err := accept.CombinedOutput(); err != nil || !strings.Contains(string(output), "Maestro preparado") {
		t.Fatalf("quick-start accept = %v, %s", err, output)
	}
	wantWorkspace := filepath.Join(home, "Maestro", "maestro-os")
	if got := strings.TrimSpace(string(mustReadScriptFile(t, revealLog))); got != wantWorkspace {
		t.Fatalf("revealed workspace = %q, want %q", got, wantWorkspace)
	}

	if err := os.Remove(revealLog); err != nil {
		t.Fatal(err)
	}
	direct := exec.Command("/bin/sh", filepath.Join(packageRoot, "install.sh"), "install")
	direct.Env = append(os.Environ(), "HOME="+home)
	if output, err := direct.CombinedOutput(); err != nil {
		t.Fatalf("direct Claude-style install = %v, %s", err, output)
	}
	if _, err := os.Stat(revealLog); !os.IsNotExist(err) {
		t.Fatalf("direct installer unexpectedly revealed a workspace: %v", err)
	}
}

func TestMacOSScriptPortablePreservesOwnerModifiedHookSettings(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	packageRoot := extractScriptPortable(t, buildScriptPortableFixture(t, root, "0.2.0", "macos"))
	workspace := filepath.Join(t.TempDir(), "owner-workspace")
	if err := os.MkdirAll(filepath.Join(workspace, ".claude"), 0o700); err != nil {
		t.Fatal(err)
	}
	settingsPath := filepath.Join(workspace, ".claude", "settings.local.json")
	ownerSettings := []byte(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"owner-hook"}]}]}}`)
	if err := os.WriteFile(settingsPath, ownerSettings, 0o600); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", filepath.Join(packageRoot, "install.sh"), "install", "--workspace", workspace)
	command.Env = append(os.Environ(), "HOME="+home)
	if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "MAESTRO-SCRIPT-CONFLICT") {
		t.Fatalf("owner settings conflict = %v, %s", err, output)
	}
	if got := mustReadScriptFile(t, settingsPath); !bytes.Equal(got, ownerSettings) {
		t.Fatalf("owner settings were overwritten: %s", got)
	}
}

func TestMacOSScriptPortableRejectsMalformedManagedBlockBeforeProjection(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	packageRoot := extractScriptPortable(t, buildScriptPortableFixture(t, root, "0.2.0", "macos"))
	workspace := filepath.Join(t.TempDir(), "owner-workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatal(err)
	}
	original := []byte("owner content\n<!-- MAESTRO SCRIPT MANAGED BEGIN -->\ndo not erase\n")
	if err := os.WriteFile(filepath.Join(workspace, "CLAUDE.md"), original, 0o600); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", filepath.Join(packageRoot, "install.sh"), "install", "--workspace", workspace)
	command.Env = append(os.Environ(), "HOME="+home)
	if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "MAESTRO-SCRIPT-STATE") {
		t.Fatalf("malformed marker install = %v, %s", err, output)
	}
	if got := mustReadScriptFile(t, filepath.Join(workspace, "CLAUDE.md")); !bytes.Equal(got, original) {
		t.Fatalf("malformed marker changed owner content: %q", got)
	}
}

func TestMacOSScriptPortableRejectsTamperBeforeMutation(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	packageRoot := extractScriptPortable(t, buildScriptPortableFixture(t, root, "0.2.0", "macos"))
	if err := os.WriteFile(filepath.Join(packageRoot, "projection", "skills", "maestro-onboarding", "SKILL.md"), []byte("tampered\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", filepath.Join(packageRoot, "install.sh"), "install", "--workspace", filepath.Join(packageRoot, "maestro-os"))
	command.Env = append(os.Environ(), "HOME="+home)
	if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "MAESTRO-SCRIPT-INTEGRITY") {
		t.Fatalf("tampered install = %v, %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(home, "Library", "Application Support", "Maestro")); !os.IsNotExist(err) {
		t.Fatalf("tamper mutated runtime: %v", err)
	}
}

func TestMacOSScriptPortableRejectsUndeclaredPrefixFileBeforeMutation(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	packageRoot := extractScriptPortable(t, buildScriptPortableFixture(t, root, "0.2.0", "macos"))
	if err := os.WriteFile(filepath.Join(packageRoot, "README"), []byte("undeclared\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	command := exec.Command("/bin/sh", filepath.Join(packageRoot, "install.sh"), "install")
	command.Env = append(os.Environ(), "HOME="+home)
	if output, err := command.CombinedOutput(); err == nil || !strings.Contains(string(output), "arquivo nao declarado: README") {
		t.Fatalf("undeclared prefix file = %v, %s", err, output)
	}
	if _, err := os.Stat(filepath.Join(home, "Library", "Application Support", "Maestro")); !os.IsNotExist(err) {
		t.Fatalf("undeclared file mutated runtime: %v", err)
	}
}

func buildScriptPortableFixture(t *testing.T, root, version, target string) string {
	t.Helper()
	output := filepath.Join(t.TempDir(), scriptPortableName(version, target))
	if _, err := BuildScriptPortable(ScriptPortableOptions{Root: root, Output: output, Version: version, TargetOS: target}); err != nil {
		t.Fatal(err)
	}
	return output
}

func injectPOSIXProjectionFailure(t *testing.T, packageRoot, phase string) {
	t.Helper()
	installer := filepath.Join(packageRoot, "install.sh")
	body := string(mustReadScriptFile(t, installer))
	const marker = "failure_point() {\n  :\n}"
	replacement := "failure_point() {\n  if [ \"${MAESTRO_SCRIPT_TEST_TRACE:-}\" = 1 ]; then printf 'TRACE phase=%s active=%s to=%s\\n' \"$1\" \"$(cat \"$STATE/active-version\" 2>/dev/null || true)\" \"$(cat \"$STATE/projection-transaction/to-version\" 2>/dev/null || true)\" >&2; fi\n  if [ \"${MAESTRO_SCRIPT_TEST_FAILURE_POINT:-}\" = \"$1\" ]; then kill -9 $$; fi\n  :\n}"
	if !strings.Contains(body, marker) {
		t.Fatalf("installer has no inert failure point contract for %s", phase)
	}
	if err := os.WriteFile(installer, []byte(strings.Replace(body, marker, replacement, 1)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeScriptRuntimeInventory(packageRoot, "macos"); err != nil {
		t.Fatal(err)
	}
	if err := writeScriptInventory(packageRoot); err != nil {
		t.Fatal(err)
	}
}

func injectPOSIXRevealProbe(t *testing.T, packageRoot, logPath string) {
	t.Helper()
	installer := filepath.Join(packageRoot, "install.sh")
	body := string(mustReadScriptFile(t, installer))
	const marker = `/usr/bin/open "$WORKSPACE"`
	replacement := `printf '%s\n' "$WORKSPACE" > ` + shellSingleQuote(logPath)
	if !strings.Contains(body, marker) {
		t.Fatalf("installer has no bounded workspace reveal contract")
	}
	if err := os.WriteFile(installer, []byte(strings.Replace(body, marker, replacement, 1)), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writeScriptRuntimeInventory(packageRoot, "macos"); err != nil {
		t.Fatal(err)
	}
	if err := writeScriptInventory(packageRoot); err != nil {
		t.Fatal(err)
	}
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'"'"'`) + "'"
}

func injectWindowsProjectionFailure(t *testing.T, packageRoot string) {
	t.Helper()
	installer := filepath.Join(packageRoot, "Install-Maestro.ps1")
	body := string(mustReadScriptFile(t, installer))
	const marker = "function Invoke-FailurePoint([string]$Phase) {}"
	replacement := "function Invoke-FailurePoint([string]$Phase) { if ($env:MAESTRO_SCRIPT_TEST_FAILURE_POINT -eq $Phase) { Stop-Process -Id $PID -Force } }"
	if !strings.Contains(body, marker) {
		t.Fatal("PowerShell installer has no inert failure point contract")
	}
	if err := os.WriteFile(installer, []byte(strings.Replace(body, marker, replacement, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeScriptRuntimeInventory(packageRoot, "windows"); err != nil {
		t.Fatal(err)
	}
	if err := writeScriptInventory(packageRoot); err != nil {
		t.Fatal(err)
	}
}

func scriptPortableName(version, target string) string {
	profile := "macos-shell-local-beta"
	if target == "windows" {
		profile = "windows-powershell-local-beta"
	}
	return "Maestro-Portable-" + version + "-" + profile + ".zip"
}

func readScriptPortableZIP(t *testing.T, path string) map[string][]byte {
	t.Helper()
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	result := map[string][]byte{}
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		var buffer bytes.Buffer
		if _, err := buffer.ReadFrom(reader); err != nil {
			reader.Close()
			t.Fatal(err)
		}
		reader.Close()
		result[entry.Name] = buffer.Bytes()
	}
	return result
}

func extractScriptPortable(t *testing.T, path string) string {
	t.Helper()
	archive, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	destination := t.TempDir()
	var packageRoot string
	for _, entry := range archive.File {
		target := filepath.Join(destination, filepath.FromSlash(entry.Name))
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				t.Fatal(err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		reader, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		var body bytes.Buffer
		if _, err := body.ReadFrom(reader); err != nil {
			reader.Close()
			t.Fatal(err)
		}
		reader.Close()
		mode := entry.Mode().Perm()
		if err := os.WriteFile(target, body.Bytes(), mode); err != nil {
			t.Fatal(err)
		}
		if packageRoot == "" {
			packageRoot = filepath.Join(destination, strings.Split(entry.Name, "/")[0])
		}
	}
	return packageRoot
}

func moveScriptPortableFixture(t *testing.T, source, destination string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(source, destination); err != nil {
		t.Fatal(err)
	}
	return destination
}

func runScriptPortable(t *testing.T, packageRoot, home string, args ...string) {
	t.Helper()
	command := exec.Command("/bin/sh", append([]string{filepath.Join(packageRoot, "install.sh")}, args...)...)
	command.Env = append(os.Environ(), "HOME="+home)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("install.sh %v: %v\n%s", args, err, output)
	}
}

func runWindowsPowerShellScript(t *testing.T, powershell, script string, environment []string, input string, args ...string) string {
	t.Helper()
	commandArgs := append([]string{"-NoLogo", "-NoProfile", "-NonInteractive", "-File", script}, args...)
	command := exec.Command(powershell, commandArgs...)
	command.Env = append(os.Environ(), environment...)
	command.Stdin = strings.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("PowerShell %s %v: %v\n%s", script, args, err, output)
	}
	return string(output)
}

func runCommandWithOpenStdin(t *testing.T, command *exec.Cmd, input []byte, timeout time.Duration) (string, error) {
	t.Helper()
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}

	writeDone := make(chan error, 1)
	go func() {
		remaining := input
		for len(remaining) > 0 {
			n, writeErr := stdin.Write(remaining)
			if writeErr != nil {
				writeDone <- writeErr
				return
			}
			if n == 0 {
				writeDone <- os.ErrInvalid
				return
			}
			remaining = remaining[n:]
		}
		writeDone <- nil
	}()
	select {
	case err := <-writeDone:
		if err != nil {
			_ = stdin.Close()
			_ = command.Process.Kill()
			_ = command.Wait()
			t.Fatalf("writing bounded hook input: %v", err)
		}
	case <-time.After(timeout):
		_ = stdin.Close()
		_ = command.Process.Kill()
		_ = command.Wait()
		t.Fatal("hook did not consume its bounded input")
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- command.Wait() }()
	select {
	case err := <-waitDone:
		_ = stdin.Close()
		return output.String(), err
	case <-time.After(timeout):
		_ = stdin.Close()
		select {
		case err := <-waitDone:
			return output.String(), err
		case <-time.After(2 * time.Second):
			_ = command.Process.Kill()
			<-waitDone
			t.Fatal("hook read past 65537 bytes or waited for EOF")
		}
	}
	return "", nil
}

func mustReadScriptFile(t *testing.T, path string) []byte {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
