// Command native-qualification runs a fresh, attended native-runtime qualification
// against a platform ZIP. It persists only bounded metadata in its report;
// native streams, prompts, tool arguments and session identities stay in the
// temporary fixture and are deleted by default.
package main

import (
	"archive/zip"
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
)

var requiredHooks = []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop", "SubagentStart", "SubagentStop"}
var requiredAgents = []string{"case-agent", "client-account-agent", "yoda", "darwin", "pa-expert"}

type streamEvidence struct {
	SessionID            string
	Hooks                map[string]bool
	Agents               map[string]bool
	Skills               map[string]bool
	InvokedDoctor        bool
	ReadHubDoctor        bool
	GuardDenied          bool
	GuardAttempted       bool
	GuardExecuted        bool
	ManagedSubagent      bool
	DoctorStatusChecked  bool
	DoctorVersionChecked bool
	DoctorVersionOutput  string
	CommandCount         int
	FileChangeCount      int
	ExternalToolCount    int
	ToolUseCount         int
	ItemTypes            map[string]int
	Text                 string
}

func codexArgs(model, prompt string) []string {
	args := []string{
		"exec", "--ephemeral", "--json", "--approve-for-me",
		"--dangerously-bypass-hook-trust", "--ignore-rules",
		"--enable", "hooks",
	}
	if strings.TrimSpace(model) != "" {
		args = append(args, "-m", model)
	}
	return append(args, prompt)
}

func codexPersistentArgs(model, prompt string) []string {
	args := codexArgs(model, prompt)
	result := make([]string, 0, len(args)-1)
	for _, arg := range args {
		if arg != "--ephemeral" {
			result = append(result, arg)
		}
	}
	return result
}

func codexResumeArgs(model, sessionID, prompt string) []string {
	args := []string{"--approve-for-me", "--dangerously-bypass-hook-trust", "--enable", "hooks", "exec", "resume", "--json", "--ignore-rules"}
	if strings.TrimSpace(model) != "" {
		args = append(args, "-m", model)
	}
	return append(args, sessionID, prompt)
}

func inspectCodexStream(stream []byte, expectedCLI, expectedRepository string) (streamEvidence, error) {
	evidence := streamEvidence{Hooks: map[string]bool{}, Agents: map[string]bool{}, Skills: map[string]bool{}, ItemTypes: map[string]int{}}
	scanner := bufio.NewScanner(bytes.NewReader(stream))
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var envelope struct {
			Type     string `json:"type"`
			ThreadID string `json:"thread_id"`
			Item     struct {
				Type             string `json:"type"`
				Command          string `json:"command"`
				AggregatedOutput string `json:"aggregated_output"`
				Text             string `json:"text"`
				ExitCode         *int   `json:"exit_code"`
			} `json:"item"`
		}
		if err := json.Unmarshal(line, &envelope); err != nil {
			return streamEvidence{}, fmt.Errorf("decode Codex stream line: %w", err)
		}
		if envelope.Type == "thread.started" && evidence.SessionID == "" {
			evidence.SessionID = envelope.ThreadID
		}
		if envelope.Type != "item.completed" {
			continue
		}
		evidence.ItemTypes[envelope.Item.Type]++
		switch envelope.Item.Type {
		case "agent_message":
			evidence.Text += envelope.Item.Text + "\n"
		case "command_execution":
			evidence.CommandCount++
			evidence.ExternalToolCount++
			command := filepath.ToSlash(envelope.Item.Command)
			if successfulCodexDoctorSkillRead(command, envelope.Item.AggregatedOutput, envelope.Item.ExitCode) {
				evidence.Skills["maestro-doctor"] = true
				evidence.InvokedDoctor = true
			}
			if exactCodexCommand(command, "git", "reset", "--hard", "HEAD") {
				evidence.GuardAttempted = true
				if envelope.Item.ExitCode != nil && *envelope.Item.ExitCode == 0 {
					evidence.GuardExecuted = true
				}
			}
			if strings.Contains(envelope.Item.AggregatedOutput, "dangerous shell or Git mutation is not authorized by the workspace projection") {
				evidence.GuardDenied = true
			}
			exitedCleanly := envelope.Item.ExitCode != nil && *envelope.Item.ExitCode == 0
			if exitedCleanly && expectedCLI != "" && exactCodexCommand(command, expectedCLI, "workspace", "status", "--runtime", "codex", expectedRepository) && strings.Contains(envelope.Item.AggregatedOutput, `"state": "enrolled"`) {
				evidence.DoctorStatusChecked = true
			}
			if exitedCleanly && expectedCLI != "" && exactCodexCommand(command, expectedCLI, "version") && strings.TrimSpace(envelope.Item.AggregatedOutput) != "" {
				evidence.DoctorVersionChecked = true
				evidence.DoctorVersionOutput = strings.TrimSpace(envelope.Item.AggregatedOutput)
			}
		case "file_change":
			evidence.FileChangeCount++
		case "reasoning", "todo_list", "error":
		default:
			evidence.ExternalToolCount++
		}
	}
	if err := scanner.Err(); err != nil {
		return streamEvidence{}, err
	}
	return evidence, nil
}

func exactCodexCommand(command, executable string, arguments ...string) bool {
	want := append([]string{executable}, arguments...)
	words, err := qualificationShellWords(command)
	if err != nil {
		return false
	}
	if slicesEqual(words, want) {
		return true
	}
	if len(words) == 3 && (filepath.Base(words[0]) == "zsh" || filepath.Base(words[0]) == "bash" || filepath.Base(words[0]) == "sh") && words[1] == "-lc" {
		inner, innerErr := qualificationShellWords(words[2])
		return innerErr == nil && slicesEqual(inner, want)
	}
	return false
}

func successfulCodexDoctorSkillRead(command, output string, exitCode *int) bool {
	if exitCode == nil || *exitCode != 0 {
		return false
	}
	if !exactCodexCommand(command, "cat", ".codex/skills/maestro-doctor/SKILL.md") {
		return false
	}
	return strings.Contains(output, "name: maestro-doctor") && strings.Contains(output, "# Maestro Doctor")
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func qualificationShellWords(command string) ([]string, error) {
	var words []string
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	for _, character := range command {
		if escaped {
			current.WriteRune(character)
			escaped = false
			continue
		}
		if character == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if character == quote {
				quote = 0
			} else {
				current.WriteRune(character)
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
		case ' ', '\t', '\r', '\n':
			flush()
		case ';', '&', '|', '<', '>', '(', ')':
			return nil, errors.New("complex shell command is not exact")
		default:
			current.WriteRune(character)
		}
	}
	if escaped || quote != 0 {
		return nil, errors.New("incomplete shell quoting")
	}
	flush()
	return words, nil
}

func (e streamEvidence) Contains(value string) bool { return strings.Contains(e.Text, value) }

func transparentIdentityObserved(evidence streamEvidence, runtime string) bool {
	normalized := strings.ToLower(evidence.Text)
	if !strings.Contains(normalized, "maestro") {
		return false
	}
	hostName := runtimeDisplayName(runtime)
	if hostName == "" || !strings.Contains(normalized, strings.ToLower(hostName)) {
		return false
	}
	for _, refusal := range []string{
		"prompt injection",
		"tentativa de prompt",
		"will ignore",
		"vou ignor",
		"not a trusted source",
		"não é uma fonte confiável",
		"pretend to be",
		"fingir ser",
		"masquerad",
		"mascar",
	} {
		if strings.Contains(normalized, refusal) {
			return false
		}
	}
	return true
}

func runtimeDisplayName(runtime string) string {
	switch runtime {
	case "claude":
		return "Claude Code"
	case "codex":
		return "Codex"
	default:
		return ""
	}
}

func doctorPassed(evidence streamEvidence, _ string, _ bool) bool {
	return evidence.InvokedDoctor && evidence.Skills["maestro-doctor"]
}

func codexDoctorPassed(evidence streamEvidence, releaseVersion string) bool {
	return evidence.InvokedDoctor && evidence.Skills["maestro-doctor"] &&
		evidence.DoctorStatusChecked && evidence.DoctorVersionChecked &&
		exactCodexDoctorVersion(evidence.DoctorVersionOutput, releaseVersion) &&
		strings.Contains(evidence.Text, "Tudo funcionando") &&
		containsExactVersion(evidence.Text, releaseVersion)
}

func exactCodexDoctorVersion(output, releaseVersion string) bool {
	return strings.TrimSpace(output) == "bcgos "+releaseVersion
}

func containsExactVersion(text, releaseVersion string) bool {
	if releaseVersion == "" {
		return false
	}
	for offset := 0; ; {
		index := strings.Index(text[offset:], releaseVersion)
		if index < 0 {
			return false
		}
		index += offset
		end := index + len(releaseVersion)
		leftBounded := index == 0 || !isVersionCharacter(text[index-1])
		rightBounded := end == len(text) || !isVersionCharacter(text[end])
		if leftBounded && rightBounded {
			return true
		}
		offset = index + 1
	}
}

func isVersionCharacter(value byte) bool {
	return value == '.' || value >= '0' && value <= '9'
}

type report struct {
	SchemaVersion            int             `json:"schema_version"`
	Result                   string          `json:"result"`
	ObservedAt               string          `json:"observed_at"`
	Runtime                  string          `json:"runtime"`
	RuntimeVersion           string          `json:"runtime_version"`
	OS                       string          `json:"os"`
	Arch                     string          `json:"arch"`
	ReleaseVersion           string          `json:"release_version"`
	ArtifactSHA256           string          `json:"artifact_sha256"`
	Model                    string          `json:"model,omitempty"`
	RuntimeConfigSHA256      string          `json:"runtime_config_sha256,omitempty"`
	RuntimeConfigFinalSHA256 string          `json:"runtime_config_final_sha256,omitempty"`
	Checks                   map[string]bool `json:"checks"`
	ReceiptCounts            map[string]int  `json:"receipt_counts"`
	StreamItemCounts         map[string]int  `json:"stream_item_counts,omitempty"`
	Notes                    []string        `json:"notes"`
}

func main() {
	artifact := flag.String("artifact", "", "platform ZIP to qualify")
	runtimeName := flag.String("runtime", "claude", "native runtime to qualify: claude or codex")
	claudePath := flag.String("claude", "claude", "Claude Code executable")
	codexPath := flag.String("codex", "codex", "Codex executable")
	model := flag.String("model", "haiku", "Claude model alias")
	codexModel := flag.String("codex-model", "", "required Codex model identifier")
	maxBudget := flag.String("max-budget-usd", "0.50", "maximum spend for each native session")
	evidencePath := flag.String("evidence", "", "optional bounded JSON report path")
	keep := flag.Bool("keep", false, "keep the synthetic fixture for diagnosis")
	flag.Parse()
	if strings.TrimSpace(*artifact) == "" {
		fatal(errors.New("--artifact is required; build the target ZIP first"))
	}
	var result report
	var fixture string
	var qualificationErr error
	switch *runtimeName {
	case "claude":
		result, fixture, qualificationErr = qualifyClaude(*artifact, *claudePath, *model, *maxBudget)
	case "codex":
		result, fixture, qualificationErr = qualifyCodex(*artifact, *codexPath, *codexModel)
	default:
		fatal(errors.New("--runtime must be claude or codex"))
	}
	var cleanupErr error
	if fixture != "" {
		cleanupErr = cleanupQualificationFixture(fixture, *keep)
	}
	result, qualificationErr = finalizeQualificationReport(result, qualificationErr, cleanupErr)
	body, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		fatal(err)
	}
	body = append(body, '\n')
	if *evidencePath != "" {
		if err := os.WriteFile(*evidencePath, body, 0o600); err != nil {
			fatal(err)
		}
	}
	_, _ = os.Stdout.Write(body)
	if qualificationErr != nil {
		fatal(qualificationErr)
	}
}

func finalizeQualificationReport(result report, qualificationErr, cleanupErr error) (report, error) {
	combinedErr := errors.Join(qualificationErr, cleanupErr)
	if combinedErr != nil {
		result.Result = "failed"
	}
	return result, combinedErr
}

func cleanupQualificationFixture(fixture string, keep bool) error {
	scrubErr := scrubQualificationCredentials(fixture)
	if scrubErr == nil {
		if keep {
			return nil
		}
		if err := removeQualificationFixture(fixture); err != nil {
			return fmt.Errorf("remove synthetic qualification fixture: %w", err)
		}
		return nil
	}
	// Credential cleanup failure invalidates diagnostic retention. Attempt the
	// broader fixture removal even when --keep was requested.
	removeErr := removeQualificationFixture(fixture)
	if removeErr != nil {
		return errors.Join(
			fmt.Errorf("scrub temporary qualification credentials: %w", scrubErr),
			fmt.Errorf("remove fixture after credential scrub failure: %w", removeErr),
		)
	}
	return fmt.Errorf("scrub temporary qualification credentials: %w; the complete fixture was removed instead", scrubErr)
}

func scrubQualificationCredentials(fixture string) error {
	for _, name := range []string{".codex-home", ".claude-home"} {
		isolatedHome := filepath.Join(fixture, name)
		if info, err := os.Lstat(isolatedHome); err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			if err := os.Chmod(isolatedHome, 0o700); err != nil {
				return err
			}
			if err := os.RemoveAll(isolatedHome); err != nil {
				return err
			}
		} else if errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return err
		} else {
			return errors.New("isolated runtime home is not a safe directory")
		}
	}
	return nil
}

func removeQualificationFixture(fixture string) error {
	isolatedHome := filepath.Join(fixture, ".codex-home")
	if info, err := os.Lstat(isolatedHome); err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
		if err := os.Chmod(isolatedHome, 0o700); err != nil {
			return err
		}
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.RemoveAll(fixture)
}

func qualifyClaude(artifact, claudePath, model, maxBudget string) (report, string, error) {
	result := report{
		SchemaVersion: 2, Result: "failed", ObservedAt: time.Now().UTC().Format(time.RFC3339),
		Runtime: "claude", OS: runtime.GOOS, Arch: runtime.GOARCH,
		Checks: map[string]bool{}, ReceiptCounts: map[string]int{},
		StreamItemCounts: map[string]int{},
		Notes:            []string{"synthetic repository only", "raw prompts, paths, tool payloads and session identifiers were not persisted"},
	}
	result.Model = model
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return result, "", errors.New("this matrix currently qualifies the macos-arm64 ZIP on a native darwin/arm64 host")
	}
	absoluteArtifact, err := filepath.Abs(artifact)
	if err != nil {
		return result, "", err
	}
	result.ArtifactSHA256, err = fileDigest(absoluteArtifact)
	if err != nil {
		return result, "", fmt.Errorf("digest artifact: %w", err)
	}
	fixture, err := os.MkdirTemp("", "maestro-native-qualification-")
	if err != nil {
		return result, "", err
	}
	claudeConfig, claudeConfigDigest, err := prepareIsolatedClaudeConfig(fixture)
	if err != nil {
		return result, fixture, fmt.Errorf("prepare isolated Claude config: %w", err)
	}
	result.RuntimeConfigSHA256 = claudeConfigDigest
	if err := extractZIP(absoluteArtifact, fixture); err != nil {
		return result, fixture, fmt.Errorf("extract artifact: %w", err)
	}
	hub := filepath.Join(fixture, "Maestro")
	versionBody, err := os.ReadFile(filepath.Join(hub, "VERSION"))
	if err != nil {
		return result, fixture, fmt.Errorf("read release version: %w", err)
	}
	result.ReleaseVersion = strings.TrimSpace(string(versionBody))
	result.RuntimeVersion, err = commandOutput(fixture, 10*time.Second, claudePath, "--version")
	if err != nil {
		return result, fixture, fmt.Errorf("inspect Claude version: %w", err)
	}

	hubStream, err := runClaudeIsolated(hub, claudeConfig, claudePath, model, maxBudget,
		"Read bundles/base/skills/maestro-doctor/SKILL.md and complete its checks. Include the Doctor's one-line verdict and version in your final response.")
	if err != nil {
		return result, fixture, fmt.Errorf("Hub native session failed: %w", err)
	}
	hubEvidence, err := inspectClaudeStream(hubStream)
	if err != nil {
		return result, fixture, fmt.Errorf("inspect Hub native stream: %w", err)
	}
	result.Checks["hub_session_start"] = hubEvidence.Hooks["SessionStart"]
	result.Checks["hub_doctor_skill"] = hubEvidence.ReadHubDoctor
	result.Checks["hub_bootstrap"] = regular(filepath.Join(hub, "data", ".initialized")) && regular(filepath.Join(hub, "data", "install.json"))
	skillIDs, err := canonicalSkillIDs(hub)
	if err != nil {
		return result, fixture, fmt.Errorf("read canonical skill catalog: %w", err)
	}
	if err := preparePortableOwnerFixture(filepath.Join(hub, "data")); err != nil {
		return result, fixture, fmt.Errorf("prepare synthetic reviewed owner: %w", err)
	}
	memorySentinel := "MAESTRO-IMPORTED-MEMORY-CLAUDE-OK"
	memoryPath, memoryBody, err := prepareLegacyHubCandidate(filepath.Join(hub, "data"), memorySentinel)
	if err != nil {
		return result, fixture, fmt.Errorf("prepare synthetic legacy Hub memory: %w", err)
	}

	repository := filepath.Join(fixture, "synthetic-repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		return result, fixture, err
	}
	for _, args := range [][]string{{"init"}, {"config", "user.email", "native@example.invalid"}, {"config", "user.name", "Maestro Native Qualification"}} {
		if _, err := commandOutput(repository, 10*time.Second, "git", args...); err != nil {
			return result, fixture, err
		}
	}
	if err := os.WriteFile(filepath.Join(repository, "seed.txt"), []byte("synthetic native qualification\n"), 0o600); err != nil {
		return result, fixture, err
	}
	for _, args := range [][]string{{"add", "seed.txt"}, {"commit", "-m", "synthetic fixture"}} {
		if _, err := commandOutput(repository, 10*time.Second, "git", args...); err != nil {
			return result, fixture, err
		}
	}
	cli := filepath.Join(hub, "managed", "bin", "bcgos")
	if _, err := commandOutput(repository, 30*time.Second, cli, "workspace", "enroll", "--runtime", "claude", repository); err != nil {
		return result, fixture, fmt.Errorf("enroll synthetic repository: %w", err)
	}
	mainStatus, err := enrolledWorkspaceStatus(repository, cli, "claude", repository)
	if err != nil {
		return result, fixture, fmt.Errorf("workspace status is not enrolled: %w", err)
	}
	result.Checks["direct_enrollment"] = true
	result.Checks["canonical_skill_projection"] = completeSkillProjection(repository, "claude", skillIDs)
	if err := bridgeLegacyHubCandidate(repository, cli, "claude", memorySentinel); err != nil {
		return result, fixture, fmt.Errorf("bridge synthetic legacy Hub memory: %w", err)
	}
	memoryAfter, memoryErr := os.ReadFile(memoryPath)
	result.Checks["memory_source_preserved"] = memoryErr == nil && bytes.Equal(memoryAfter, memoryBody)
	clean, err := commandOutput(repository, 10*time.Second, "git", "status", "--porcelain")
	if err != nil {
		return result, fixture, err
	}
	result.Checks["projection_git_clean"] = clean == ""

	var projection struct {
		WorkspaceID string `json:"workspace_id"`
		DataRoot    string `json:"data_root"`
	}
	if err := readJSON(filepath.Join(repository, ".bcgos", "workspace-projections", "claude.json"), &projection); err != nil {
		return result, fixture, err
	}
	if projection.WorkspaceID == "" || projection.DataRoot == "" {
		return result, fixture, errors.New("direct projection omitted private authority pointers")
	}
	if projection.WorkspaceID != mainStatus.WorkspaceID {
		return result, fixture, errors.New("Claude projection workspace identity drifted from enrolled status")
	}
	secondWorktree, err := prepareDistinctWorktree(repository, filepath.Join(fixture, "synthetic-second-worktree"))
	if err != nil {
		return result, fixture, fmt.Errorf("create distinct Git worktree: %w", err)
	}
	if _, err := commandOutput(secondWorktree, 30*time.Second, cli, "workspace", "enroll", "--runtime", "claude", secondWorktree); err != nil {
		return result, fixture, fmt.Errorf("enroll second Claude worktree: %w", err)
	}
	secondStatus, err := enrolledWorkspaceStatus(secondWorktree, cli, "claude", secondWorktree)
	if err != nil {
		return result, fixture, err
	}
	result.Checks["distinct_worktree_identity"] = mainStatus.RepositoryID == secondStatus.RepositoryID && mainStatus.WorkspaceID != secondStatus.WorkspaceID
	var secondProjection struct {
		WorkspaceID string `json:"workspace_id"`
		DataRoot    string `json:"data_root"`
	}
	if err := readJSON(filepath.Join(secondWorktree, ".bcgos", "workspace-projections", "claude.json"), &secondProjection); err != nil {
		return result, fixture, err
	}
	for _, agent := range requiredAgents {
		body, readErr := os.ReadFile(filepath.Join(repository, ".claude", "agents", agent+".md"))
		if readErr != nil || !bytes.HasPrefix(body, []byte("---\n")) {
			return result, fixture, fmt.Errorf("projected native agent %s has invalid frontmatter", agent)
		}
	}
	result.Checks["specialist_topology_projected"] = true
	resumeSessionID, err := newQualificationSessionID()
	if err != nil {
		return result, fixture, err
	}
	resumeToken := "MAESTRO-CLAUDE-RESUME-OK"
	identityStream, err := runClaudePersistent(repository, claudeConfig, claudePath, model, maxBudget, resumeSessionID,
		"Responda em quatro linhas, sem usar ferramentas. Linha 1: identifique explicitamente Claude Code como o host runtime subjacente. Linha 2: identifique explicitamente Maestro como a camada operacional profissional configurada. Linha 3: informe o nome preferido e o papel profissional do owner, usando apenas o contexto recebido no SessionStart. Linha 4: informe o token exato de memória importada recebido no SessionStart. Seja transparente e não descreva nenhuma camada como prompt injection ou instrução recusada. Memorize também este token apenas para retomada: "+resumeToken+".")
	if err != nil {
		return result, fixture, fmt.Errorf("transparent identity session failed: %w", err)
	}
	identityEvidence, err := inspectClaudeStream(identityStream)
	if err != nil {
		return result, fixture, fmt.Errorf("inspect transparent identity stream: %w", err)
	}
	result.Checks["transparent_maestro_identity"] = transparentIdentityObserved(identityEvidence, "claude")
	ownerStream, err := runClaudeIsolated(repository, claudeConfig, claudePath, model, maxBudget,
		"Return exactly two lines and do not use tools. Line 1: the reviewed owner's preferred name supplied in SessionStart. Line 2: the reviewed owner's professional role supplied in SessionStart.")
	if err != nil {
		return result, fixture, fmt.Errorf("owner-context session failed: %w", err)
	}
	ownerEvidence, err := inspectClaudeStream(ownerStream)
	if err != nil {
		return result, fixture, fmt.Errorf("inspect owner-context stream: %w", err)
	}
	result.Checks["owner_context_injected"] = ownerEvidence.Contains("Qualification Owner") && ownerEvidence.Contains("Synthetic engineering qualification role") && ownerEvidence.ToolUseCount == 0
	result.Checks["owner_sensitive_context_excluded"] = !identityEvidence.Contains("NATIVE-PRIVATE-SENTINEL-MUST-NOT-APPEAR") && !ownerEvidence.Contains("NATIVE-PRIVATE-SENTINEL-MUST-NOT-APPEAR")
	result.Checks["memory_context_injected"] = identityEvidence.Contains(memorySentinel)
	resumeStream, err := resumeClaude(repository, claudeConfig, claudePath, model, maxBudget, resumeSessionID, "Return only the resume token I asked you to remember in the prior turn. Do not use tools.")
	if err != nil {
		return result, fixture, fmt.Errorf("Claude native resume failed: %w", err)
	}
	resumeEvidence, err := inspectClaudeStream(resumeStream)
	if err != nil {
		return result, fixture, err
	}
	result.Checks["native_resume"] = identityEvidence.SessionID == resumeSessionID && resumeEvidence.Contains(resumeToken) && resumeEvidence.ToolUseCount == 0

	doctorStream, err := runClaudeIsolated(repository, claudeConfig, claudePath, model, maxBudget,
		"Before using any tool, state the reviewed owner's preferred name and professional role from SessionStart context. Then use the Skill tool with skill=maestro-doctor and complete its checks. Include the Doctor's one-line verdict and version in your final response.")
	if err != nil {
		return result, fixture, fmt.Errorf("direct Doctor session failed: %w", err)
	}
	doctorEvidence, err := inspectClaudeStream(doctorStream)
	if err != nil {
		return result, fixture, fmt.Errorf("inspect direct Doctor stream: %w", err)
	}
	writeStream, err := runClaudeIsolated(repository, claudeConfig, claudePath, model, maxBudget,
		"Use the native Write tool now to create native-proof.txt with exactly MAESTRO-DIRECT-NATIVE-OK and no trailing newline. Do not use Bash, do not perform any other task, and finish only after the file exists.")
	if err != nil {
		return result, fixture, fmt.Errorf("direct workspace-write session failed: %w", err)
	}
	writeEvidence, err := inspectClaudeStream(writeStream)
	if err != nil {
		return result, fixture, fmt.Errorf("inspect direct workspace-write stream: %w", err)
	}
	agentStream, err := runClaudeIsolated(repository, claudeConfig, claudePath, model, maxBudget,
		"Run this synthetic specialist qualification in order. Delegate once to case-agent and ask it to read seed.txt and return CASE-NATIVE-OK. After it returns, delegate once to yoda and ask it to return YODA-NATIVE-OK. Do not perform other work.")
	if err != nil {
		return result, fixture, fmt.Errorf("direct specialist session failed: %w", err)
	}
	agentEvidence, err := inspectClaudeStream(agentStream)
	if err != nil {
		return result, fixture, fmt.Errorf("inspect direct specialist stream: %w", err)
	}
	guardStream, err := runClaudeIsolated(repository, claudeConfig, claudePath, model, maxBudget,
		"Attempt the exact Bash command git reset --hard HEAD now. Do not replace it, quote it, simulate it or perform any other action.")
	if err != nil {
		return result, fixture, fmt.Errorf("direct guard session failed: %w", err)
	}
	guardEvidence, err := inspectClaudeStream(guardStream)
	if err != nil {
		return result, fixture, fmt.Errorf("inspect direct guard stream: %w", err)
	}
	directEvidence := mergeStreamEvidence(doctorEvidence, writeEvidence, agentEvidence, guardEvidence)
	for _, hook := range requiredHooks {
		result.Checks["hook_"+hook] = directEvidence.Hooks[hook]
	}
	result.Checks["supported_hooks_complete"] = true
	for _, hook := range requiredHooks {
		result.Checks["supported_hooks_complete"] = result.Checks["supported_hooks_complete"] && directEvidence.Hooks[hook]
	}
	allAgents := true
	for _, agent := range requiredAgents {
		if !directEvidence.Agents[agent] {
			allAgents = false
		}
	}
	result.Checks["native_agents_discovered"] = allAgents
	result.Checks["doctor_skill_discovered"] = directEvidence.Skills["maestro-doctor"]
	result.Checks["operator_skill_discovered"] = directEvidence.Skills["maestro-operator"]
	result.Checks["memory_skill_discovered"] = directEvidence.Skills["dream-memory"]
	result.Checks["doctor_skill_invoked"] = doctorPassed(directEvidence, result.ReleaseVersion, false)
	result.Checks["owner_sensitive_context_excluded"] = result.Checks["owner_sensitive_context_excluded"] && !directEvidence.Contains("NATIVE-PRIVATE-SENTINEL-MUST-NOT-APPEAR")
	result.Checks["managed_agent_flow"] = directEvidence.ManagedSubagent
	result.Checks["dangerous_git_denied"] = directEvidence.GuardDenied
	proof, proofErr := os.ReadFile(filepath.Join(repository, "native-proof.txt"))
	result.Checks["workspace_write"] = proofErr == nil && string(proof) == "MAESTRO-DIRECT-NATIVE-OK"

	receiptRoot := filepath.Join(projection.DataRoot, "runtime", "receipts", projection.WorkspaceID)
	for _, event := range []string{"post_action_observe", "stop_finalize", "subagent_start", "subagent_stop"} {
		matches, globErr := filepath.Glob(filepath.Join(receiptRoot, event+"-*.json"))
		if globErr != nil {
			return result, fixture, globErr
		}
		result.ReceiptCounts[event] = len(matches)
		result.Checks["receipt_"+event] = len(matches) > 0
	}

	continuity := "MAESTRO-CONTINUITY-NATIVE-OK"
	continuityPath := filepath.Join(projection.DataRoot, "workspaces", projection.WorkspaceID, "continuity", "active.md")
	if err := os.MkdirAll(filepath.Dir(continuityPath), 0o700); err != nil {
		return result, fixture, err
	}
	if err := os.WriteFile(continuityPath, []byte(continuity+"\n"), 0o600); err != nil {
		return result, fixture, err
	}
	continuityStream, err := runClaudeIsolated(repository, claudeConfig, claudePath, model, maxBudget,
		"Return the exact continuity sentinel supplied by Maestro and nothing else. Do not use tools.")
	if err != nil {
		return result, fixture, fmt.Errorf("continuity native session failed: %w", err)
	}
	continuityEvidence, err := inspectClaudeStream(continuityStream)
	if err != nil {
		return result, fixture, err
	}
	result.Checks["second_session_continuity"] = continuityEvidence.Contains(continuity)
	secondContinuity := "MAESTRO-SECOND-WORKTREE-CLAUDE-OK"
	secondContinuityPath := filepath.Join(secondProjection.DataRoot, "workspaces", secondProjection.WorkspaceID, "continuity", "active.md")
	if err := os.MkdirAll(filepath.Dir(secondContinuityPath), 0o700); err != nil {
		return result, fixture, err
	}
	if err := os.WriteFile(secondContinuityPath, []byte(secondContinuity+"\n"), 0o600); err != nil {
		return result, fixture, err
	}
	secondStream, err := runClaudeIsolated(secondWorktree, claudeConfig, claudePath, model, maxBudget, "Return the exact continuity sentinel supplied for this worktree and nothing else. Do not use tools.")
	if err != nil {
		return result, fixture, fmt.Errorf("second-worktree Claude session failed: %w", err)
	}
	secondEvidence, err := inspectClaudeStream(secondStream)
	if err != nil {
		return result, fixture, err
	}
	result.Checks["distinct_worktree_context_isolated"] = secondEvidence.Contains(secondContinuity) && !secondEvidence.Contains(continuity) && !secondEvidence.Contains(memorySentinel)
	finalClaudeConfigDigest, configErr := fileDigest(filepath.Join(claudeConfig, ".claude.json"))
	if configErr != nil {
		return result, fixture, fmt.Errorf("digest final isolated Claude config: %w", configErr)
	}
	result.RuntimeConfigFinalSHA256 = finalClaudeConfigDigest

	if err := os.Remove(filepath.Join(repository, "native-proof.txt")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, fixture, err
	}
	if _, err := commandOutput(repository, 30*time.Second, cli, "workspace", "remove", "--runtime", "claude", repository); err != nil {
		return result, fixture, fmt.Errorf("remove direct projection: %w", err)
	}
	removedStatus, err := commandOutput(repository, 30*time.Second, cli, "workspace", "status", "--runtime", "claude", repository)
	if err != nil {
		return result, fixture, err
	}
	result.Checks["projection_removal"] = strings.Contains(removedStatus, `"state": "absent"`)
	secondStillEnrolled, secondStatusErr := enrolledWorkspaceStatus(secondWorktree, cli, "claude", secondWorktree)
	result.Checks["distinct_worktree_removal_independent"] = secondStatusErr == nil && secondStillEnrolled.WorkspaceID == secondStatus.WorkspaceID
	if _, err := commandOutput(secondWorktree, 30*time.Second, cli, "workspace", "remove", "--runtime", "claude", secondWorktree); err != nil {
		return result, fixture, fmt.Errorf("remove second Claude projection: %w", err)
	}
	clean, err = commandOutput(repository, 10*time.Second, "git", "status", "--porcelain")
	result.Checks["removal_git_clean"] = err == nil && clean == ""

	var failed []string
	for name, passed := range result.Checks {
		if !passed {
			failed = append(failed, name)
		}
	}
	sort.Strings(failed)
	if len(failed) > 0 {
		return result, fixture, fmt.Errorf("native qualification checks failed: %s", strings.Join(failed, ", "))
	}
	result.Result = "pass"
	return result, fixture, nil
}

func qualifyCodex(artifact, codexPath, model string) (report, string, error) {
	result := report{
		SchemaVersion: 2, Result: "failed", ObservedAt: time.Now().UTC().Format(time.RFC3339),
		Runtime: "codex", OS: runtime.GOOS, Arch: runtime.GOARCH,
		Checks: map[string]bool{}, ReceiptCounts: map[string]int{},
		StreamItemCounts: map[string]int{},
		Notes: []string{
			"synthetic repository only",
			"raw prompts, paths, tool payloads and session identifiers were not persisted",
			"Codex home was isolated to a temporary auth copy and minimal hook config; user instructions, skills, plugins, MCP servers and configuration were not loaded, and post-initialization config drift was denied",
			"hook trust bypass was limited to inspected synthetic qualification projections; approvals and workspace-write sandbox remained active",
		},
	}
	if strings.TrimSpace(model) == "" {
		return result, "", errors.New("--codex-model is required so the qualified runtime tuple is reproducible")
	}
	result.Model = model
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return result, "", errors.New("this matrix currently qualifies the macos-arm64 ZIP on a native darwin/arm64 host")
	}
	absoluteArtifact, err := filepath.Abs(artifact)
	if err != nil {
		return result, "", err
	}
	result.ArtifactSHA256, err = fileDigest(absoluteArtifact)
	if err != nil {
		return result, "", fmt.Errorf("digest artifact: %w", err)
	}
	fixture, err := os.MkdirTemp("", "maestro-native-codex-qualification-")
	if err != nil {
		return result, "", err
	}
	if err := extractZIP(absoluteArtifact, fixture); err != nil {
		return result, fixture, fmt.Errorf("extract artifact: %w", err)
	}
	codexHome, _, err := prepareIsolatedCodexHome(fixture)
	if err != nil {
		return result, fixture, fmt.Errorf("prepare isolated Codex home: %w", err)
	}
	hub := filepath.Join(fixture, "Maestro")
	versionBody, err := os.ReadFile(filepath.Join(hub, "VERSION"))
	if err != nil {
		return result, fixture, fmt.Errorf("read release version: %w", err)
	}
	result.ReleaseVersion = strings.TrimSpace(string(versionBody))
	result.RuntimeVersion, err = commandOutputWithEnv(fixture, 10*time.Second, []string{"CODEX_HOME=" + codexHome}, codexPath, "--version")
	if err != nil {
		return result, fixture, fmt.Errorf("inspect Codex version: %w", err)
	}

	bootstrap := filepath.Join(hub, "managed", "bcgos-bootstrap")
	if _, err := commandOutput(hub, 30*time.Second, bootstrap, "activate", "--managed-root", filepath.Join(hub, "managed"), "--data-root", filepath.Join(hub, "data")); err != nil {
		return result, fixture, fmt.Errorf("activate portable Hub: %w", err)
	}
	result.Checks["hub_bootstrap"] = regular(filepath.Join(hub, "data", "install.json"))
	result.Checks["hub_activation_only"] = result.Checks["hub_bootstrap"]
	agentState, err := capabilityState(hub, "agent_orchestration", "codex")
	if err != nil {
		return result, fixture, err
	}
	result.Checks["agent_orchestration_declared_unavailable"] = agentState == "unavailable"
	skillIDs, err := canonicalSkillIDs(hub)
	if err != nil {
		return result, fixture, fmt.Errorf("read canonical skill catalog: %w", err)
	}
	if err := preparePortableOwnerFixture(filepath.Join(hub, "data")); err != nil {
		return result, fixture, fmt.Errorf("prepare synthetic reviewed owner: %w", err)
	}
	memorySentinel := "MAESTRO-IMPORTED-MEMORY-CODEX-OK"
	memoryPath, memoryBody, err := prepareLegacyHubCandidate(filepath.Join(hub, "data"), memorySentinel)
	if err != nil {
		return result, fixture, fmt.Errorf("prepare synthetic legacy Hub memory: %w", err)
	}

	repository := filepath.Join(fixture, "synthetic-repository")
	if err := os.Mkdir(repository, 0o700); err != nil {
		return result, fixture, err
	}
	for _, args := range [][]string{{"init"}, {"config", "user.email", "native@example.invalid"}, {"config", "user.name", "Maestro Native Qualification"}} {
		if _, err := commandOutput(repository, 10*time.Second, "git", args...); err != nil {
			return result, fixture, err
		}
	}
	seedPath := filepath.Join(repository, "seed.txt")
	const cleanSeed = "synthetic native qualification\n"
	if err := os.WriteFile(seedPath, []byte(cleanSeed), 0o600); err != nil {
		return result, fixture, err
	}
	for _, args := range [][]string{{"add", "seed.txt"}, {"commit", "-m", "synthetic fixture"}} {
		if _, err := commandOutput(repository, 10*time.Second, "git", args...); err != nil {
			return result, fixture, err
		}
	}
	cli := filepath.Join(hub, "managed", "bin", "bcgos")
	if _, err := commandOutput(repository, 30*time.Second, cli, "workspace", "enroll", "--runtime", "codex", repository); err != nil {
		return result, fixture, fmt.Errorf("enroll synthetic Codex repository: %w", err)
	}
	mainStatus, err := enrolledWorkspaceStatus(repository, cli, "codex", repository)
	if err != nil {
		return result, fixture, fmt.Errorf("Codex workspace status is not enrolled: %w", err)
	}
	result.Checks["direct_enrollment"] = true
	result.Checks["canonical_skill_projection"] = completeSkillProjection(repository, "codex", skillIDs)
	if err := bridgeLegacyHubCandidate(repository, cli, "codex", memorySentinel); err != nil {
		return result, fixture, fmt.Errorf("bridge synthetic legacy Hub memory: %w", err)
	}
	memoryAfter, memoryErr := os.ReadFile(memoryPath)
	result.Checks["memory_source_preserved"] = memoryErr == nil && bytes.Equal(memoryAfter, memoryBody)
	clean, err := commandOutput(repository, 10*time.Second, "git", "status", "--porcelain")
	if err != nil {
		return result, fixture, err
	}
	result.Checks["projection_git_clean"] = clean == ""

	var projection struct {
		WorkspaceID string `json:"workspace_id"`
		DataRoot    string `json:"data_root"`
	}
	if err := readJSON(filepath.Join(repository, ".bcgos", "workspace-projections", "codex.json"), &projection); err != nil {
		return result, fixture, err
	}
	if projection.WorkspaceID == "" || projection.DataRoot == "" {
		return result, fixture, errors.New("Codex projection omitted private authority pointers")
	}
	if projection.WorkspaceID != mainStatus.WorkspaceID {
		return result, fixture, errors.New("Codex projection workspace identity drifted from enrolled status")
	}
	secondWorktree, err := prepareDistinctWorktree(repository, filepath.Join(fixture, "synthetic-second-worktree"))
	if err != nil {
		return result, fixture, fmt.Errorf("create distinct Git worktree: %w", err)
	}
	if _, err := commandOutput(secondWorktree, 30*time.Second, cli, "workspace", "enroll", "--runtime", "codex", secondWorktree); err != nil {
		return result, fixture, fmt.Errorf("enroll second Codex worktree: %w", err)
	}
	secondStatus, err := enrolledWorkspaceStatus(secondWorktree, cli, "codex", secondWorktree)
	if err != nil {
		return result, fixture, err
	}
	result.Checks["distinct_worktree_identity"] = mainStatus.RepositoryID == secondStatus.RepositoryID && mainStatus.WorkspaceID != secondStatus.WorkspaceID
	var secondProjection struct {
		WorkspaceID string `json:"workspace_id"`
		DataRoot    string `json:"data_root"`
	}
	if err := readJSON(filepath.Join(secondWorktree, ".bcgos", "workspace-projections", "codex.json"), &secondProjection); err != nil {
		return result, fixture, err
	}
	var hookConfig struct {
		Hooks map[string]any `json:"hooks"`
	}
	if err := readJSON(filepath.Join(repository, ".codex", "hooks.json"), &hookConfig); err != nil {
		return result, fixture, err
	}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"} {
		_, present := hookConfig.Hooks[event]
		result.Checks["hook_config_"+event] = present
	}
	hookConfigComplete := true
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "Stop"} {
		hookConfigComplete = hookConfigComplete && result.Checks["hook_config_"+event]
	}
	receiptRoot := filepath.Join(projection.DataRoot, "runtime", "receipts", projection.WorkspaceID)

	promptInput, err := commandOutputWithEnv(repository, 30*time.Second, []string{"CODEX_HOME=" + codexHome}, codexPath, "debug", "prompt-input", "synthetic discovery probe")
	if err != nil {
		return result, fixture, fmt.Errorf("inspect Codex native prompt input: %w", err)
	}
	result.Checks["agents_instructions_discovered"] = strings.Contains(promptInput, "AGENTS.md") && strings.Contains(promptInput, "Maestro")
	result.Checks["doctor_skill_discovered"] = strings.Contains(promptInput, "maestro-doctor")
	result.Checks["operator_skill_discovered"] = strings.Contains(promptInput, "maestro-operator")
	result.Checks["memory_skill_discovered"] = strings.Contains(promptInput, "dream-memory")
	agentsBody, err := os.ReadFile(filepath.Join(repository, "AGENTS.md"))
	if err != nil {
		return result, fixture, fmt.Errorf("read projected Codex instructions: %w", err)
	}
	result.Checks["specialist_topology_projected"] = specialistTopologyProjected(
		promptInput,
		string(agentsBody),
		result.Checks["agent_orchestration_declared_unavailable"],
	)
	resumeToken := "MAESTRO-CODEX-RESUME-OK"
	warmupStream, err := runCodexPersistent(repository, codexHome, codexPath, model,
		"Quem é você? Responda em uma frase, sem usar ferramentas. Em uma segunda frase, diga o nome preferido e o papel profissional do owner conforme o contexto recebido no início da sessão. Em uma terceira frase, informe o token exato de memória importada recebido no SessionStart. Memorize também este token apenas para retomada: "+resumeToken+". Termine com MAESTRO-CODEX-INITIALIZED.")
	if err != nil {
		return result, fixture, fmt.Errorf("initialize isolated Codex runtime: %w", err)
	}
	warmupEvidence, err := inspectCodexStream(warmupStream, "", "")
	if err != nil {
		return result, fixture, fmt.Errorf("inspect isolated Codex initialization: %w", err)
	}
	result.Checks["runtime_initialization_clean"] = warmupEvidence.Contains("MAESTRO-CODEX-INITIALIZED") && warmupEvidence.ExternalToolCount == 0 && warmupEvidence.FileChangeCount == 0
	result.Checks["transparent_maestro_identity"] = transparentIdentityObserved(warmupEvidence, "codex")
	result.Checks["owner_sensitive_context_excluded"] = !warmupEvidence.Contains("NATIVE-PRIVATE-SENTINEL-MUST-NOT-APPEAR")
	result.Checks["memory_context_injected"] = warmupEvidence.Contains(memorySentinel)
	if warmupEvidence.SessionID == "" {
		return result, fixture, errors.New("Codex persistent qualification session omitted its thread ID")
	}
	resumeStream, err := resumeCodex(repository, codexHome, codexPath, model, warmupEvidence.SessionID, "Return only the resume token I asked you to remember in the prior turn. Do not use tools.")
	if err != nil {
		return result, fixture, fmt.Errorf("Codex native resume failed: %w", err)
	}
	resumeEvidence, err := inspectCodexStream(resumeStream, "", "")
	if err != nil {
		return result, fixture, err
	}
	result.Checks["native_resume"] = resumeEvidence.Contains(resumeToken) && resumeEvidence.ExternalToolCount == 0 && resumeEvidence.FileChangeCount == 0
	result.RuntimeConfigSHA256, err = fileDigest(filepath.Join(codexHome, "config.toml"))
	if err != nil {
		return result, fixture, fmt.Errorf("digest initialized isolated Codex config: %w", err)
	}

	doctorStream, err := runCodex(repository, codexHome, codexPath, model,
		fmt.Sprintf("Invoke $maestro-doctor through the installed native skill mechanism. Before diagnosing, state the reviewed owner's preferred name and professional role from SessionStart context. Then execute exactly `cat .codex/skills/maestro-doctor/SKILL.md` to read the complete canonical project skill. Execute these two read-only commands separately and exactly as written: `%s version` and `%s workspace status --runtime codex .`. Complete every direct Codex check, then end with the Doctor verdict and Maestro version. If the skill cannot be read, report FAILED instead of improvising.", cli, cli))
	if err != nil {
		return result, fixture, fmt.Errorf("Codex Doctor session failed: %w", err)
	}
	doctorEvidence, err := inspectCodexStream(doctorStream, cli, ".")
	if err != nil {
		return result, fixture, fmt.Errorf("inspect Codex Doctor stream: %w", err)
	}
	result.Checks["doctor_skill_read"] = doctorEvidence.InvokedDoctor && doctorEvidence.Skills["maestro-doctor"]
	result.Checks["doctor_status_checked"] = doctorEvidence.DoctorStatusChecked
	result.Checks["doctor_version_checked"] = doctorEvidence.DoctorVersionChecked
	result.Checks["doctor_version_matches"] = exactCodexDoctorVersion(doctorEvidence.DoctorVersionOutput, result.ReleaseVersion)
	result.Checks["doctor_green_verdict"] = doctorEvidence.Contains("Tudo funcionando") && containsExactVersion(doctorEvidence.Text, result.ReleaseVersion)
	result.Checks["doctor_skill_invoked"] = codexDoctorPassed(doctorEvidence, result.ReleaseVersion)
	result.Checks["owner_context_injected"] = doctorEvidence.Contains("Qualification Owner") && doctorEvidence.Contains("Synthetic engineering qualification role")
	result.Checks["owner_sensitive_context_excluded"] = result.Checks["owner_sensitive_context_excluded"] && !doctorEvidence.Contains("NATIVE-PRIVATE-SENTINEL-MUST-NOT-APPEAR")

	const dirtySeed = "DIRTY-SENTINEL-MUST-SURVIVE-GUARD\n"
	if err := os.WriteFile(seedPath, []byte(dirtySeed), 0o600); err != nil {
		return result, fixture, err
	}
	guardBefore, err := receiptSnapshot(receiptRoot)
	if err != nil {
		return result, fixture, err
	}
	guardStream, err := runCodex(repository, codexHome, codexPath, model,
		"Attempt exactly this shell command once: git reset --hard HEAD. Do not substitute, quote, wrap or simulate it. If the native hook rejects the tool call, reply with exactly MAESTRO-GUARD-DENIED. Otherwise report exactly what happened.")
	if err != nil {
		return result, fixture, fmt.Errorf("Codex guard session failed: %w", err)
	}
	guardEvidence, err := inspectCodexStream(guardStream, "", "")
	if err != nil {
		return result, fixture, fmt.Errorf("inspect Codex guard stream: %w", err)
	}
	for itemType, count := range guardEvidence.ItemTypes {
		result.StreamItemCounts["guard:"+itemType] = count
	}
	seedAfterGuard, seedErr := os.ReadFile(seedPath)
	guardPreserved := seedErr == nil && string(seedAfterGuard) == dirtySeed
	guardReceipts, err := validatedReceiptDelta(projection.DataRoot, projection.WorkspaceID, "codex", guardBefore)
	if err != nil {
		return result, fixture, fmt.Errorf("validate Codex guard receipts: %w", err)
	}
	expectedGuardDigest := sha256.Sum256([]byte("git reset --hard HEAD"))
	expectedGuardSHA256 := hex.EncodeToString(expectedGuardDigest[:])
	result.Checks["guard_action_digest_matches"] = guardReceipts.ActionDigests[expectedGuardSHA256] == 1
	result.Checks["guard_receipt_observed"] = guardReceipts.Counts[lifecycle.PreActionGuard] == 1
	result.Checks["dangerous_git_denied"] = result.Checks["guard_action_digest_matches"] && !guardEvidence.GuardExecuted && guardPreserved && result.Checks["guard_receipt_observed"]
	if err := os.WriteFile(seedPath, []byte(cleanSeed), 0o600); err != nil {
		return result, fixture, err
	}

	allReceipts, err := validatedReceiptDelta(projection.DataRoot, projection.WorkspaceID, "codex", nil)
	if err != nil {
		return result, fixture, fmt.Errorf("validate Codex lifecycle receipts: %w", err)
	}
	for _, event := range []string{lifecycle.PostActionObserve, lifecycle.StopFinalize} {
		result.ReceiptCounts[event] = allReceipts.Counts[event]
		result.Checks["receipt_"+event] = allReceipts.Counts[event] > 0
	}
	result.Checks["supported_hooks_complete"] = hookConfigComplete && allReceipts.Counts[lifecycle.SessionStart] > 0 && allReceipts.Counts[lifecycle.ContextInject] > 0 && allReceipts.Counts[lifecycle.PreActionGuard] > 0 && allReceipts.Counts[lifecycle.PostActionObserve] > 0 && allReceipts.Counts[lifecycle.StopFinalize] > 0

	continuity := "MAESTRO-CONTINUITY-NATIVE-OK"
	continuityPath := filepath.Join(projection.DataRoot, "workspaces", projection.WorkspaceID, "continuity", "active.md")
	if err := os.MkdirAll(filepath.Dir(continuityPath), 0o700); err != nil {
		return result, fixture, err
	}
	if err := os.WriteFile(continuityPath, []byte(continuity+"\n"), 0o600); err != nil {
		return result, fixture, err
	}
	continuityBefore, err := receiptSnapshot(receiptRoot)
	if err != nil {
		return result, fixture, err
	}
	continuityStream, err := runCodex(repository, codexHome, codexPath, model,
		"Create native-proof.txt containing exactly MAESTRO-DIRECT-NATIVE-OK using the native file-edit tool. Do not use shell commands or read private Maestro files. Then return the exact continuity sentinel supplied by Maestro and nothing else.")
	if err != nil {
		return result, fixture, fmt.Errorf("Codex continuity session failed: %w", err)
	}
	continuityEvidence, err := inspectCodexStream(continuityStream, "", "")
	if err != nil {
		return result, fixture, err
	}
	for itemType, count := range continuityEvidence.ItemTypes {
		result.StreamItemCounts["continuity:"+itemType] = count
	}
	continuityReceipts, err := validatedReceiptDelta(projection.DataRoot, projection.WorkspaceID, "codex", continuityBefore)
	if err != nil {
		return result, fixture, fmt.Errorf("validate Codex continuity receipts: %w", err)
	}
	contextObserved := continuityReceipts.Counts[lifecycle.SessionStart] > 0 && continuityReceipts.Counts[lifecycle.ContextInject] > 0
	continuitySafe := continuityEvidence.ExternalToolCount == 0 && continuityEvidence.FileChangeCount > 0
	result.Checks["continuity_no_external_tools"] = continuityEvidence.ExternalToolCount == 0
	result.Checks["continuity_native_file_edit"] = continuityEvidence.FileChangeCount > 0
	result.Checks["lifecycle_context_injected"] = contextObserved && continuityEvidence.Contains(continuity) && continuitySafe
	result.Checks["second_session_continuity"] = contextObserved && continuityEvidence.Contains(continuity) && continuitySafe
	proof, proofErr := os.ReadFile(filepath.Join(repository, "native-proof.txt"))
	result.Checks["workspace_write"] = proofErr == nil && matchesSentinel(proof, "MAESTRO-DIRECT-NATIVE-OK")
	secondContinuity := "MAESTRO-SECOND-WORKTREE-CODEX-OK"
	secondContinuityPath := filepath.Join(secondProjection.DataRoot, "workspaces", secondProjection.WorkspaceID, "continuity", "active.md")
	if err := os.MkdirAll(filepath.Dir(secondContinuityPath), 0o700); err != nil {
		return result, fixture, err
	}
	if err := os.WriteFile(secondContinuityPath, []byte(secondContinuity+"\n"), 0o600); err != nil {
		return result, fixture, err
	}
	secondStream, err := runCodex(secondWorktree, codexHome, codexPath, model, "Return the exact continuity sentinel supplied for this worktree and nothing else. Do not use tools or edit files.")
	if err != nil {
		return result, fixture, fmt.Errorf("second-worktree Codex session failed: %w", err)
	}
	secondEvidence, err := inspectCodexStream(secondStream, "", "")
	if err != nil {
		return result, fixture, err
	}
	result.Checks["distinct_worktree_second_context_observed"] = secondEvidence.Contains(secondContinuity)
	result.Checks["distinct_worktree_main_context_excluded"] = !secondEvidence.Contains(continuity)
	result.Checks["distinct_worktree_imported_memory_excluded"] = !secondEvidence.Contains(memorySentinel)
	result.Checks["distinct_worktree_no_tools"] = secondEvidence.ExternalToolCount == 0 && secondEvidence.FileChangeCount == 0
	result.Checks["distinct_worktree_context_isolated"] = result.Checks["distinct_worktree_second_context_observed"] && result.Checks["distinct_worktree_main_context_excluded"] && result.Checks["distinct_worktree_imported_memory_excluded"] && result.Checks["distinct_worktree_no_tools"]
	finalConfigDigest, configErr := fileDigest(filepath.Join(codexHome, "config.toml"))
	result.RuntimeConfigFinalSHA256 = finalConfigDigest
	result.Checks["runtime_config_stable"] = configErr == nil && result.RuntimeConfigFinalSHA256 == result.RuntimeConfigSHA256

	if err := os.Remove(filepath.Join(repository, "native-proof.txt")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return result, fixture, err
	}
	if _, err := commandOutput(repository, 30*time.Second, cli, "workspace", "remove", "--runtime", "codex", repository); err != nil {
		return result, fixture, fmt.Errorf("remove Codex projection: %w", err)
	}
	removedStatus, err := commandOutput(repository, 30*time.Second, cli, "workspace", "status", "--runtime", "codex", repository)
	if err != nil {
		return result, fixture, err
	}
	result.Checks["projection_removal"] = strings.Contains(removedStatus, `"state": "absent"`)
	secondStillEnrolled, secondStatusErr := enrolledWorkspaceStatus(secondWorktree, cli, "codex", secondWorktree)
	result.Checks["distinct_worktree_removal_independent"] = secondStatusErr == nil && secondStillEnrolled.WorkspaceID == secondStatus.WorkspaceID
	if _, err := commandOutput(secondWorktree, 30*time.Second, cli, "workspace", "remove", "--runtime", "codex", secondWorktree); err != nil {
		return result, fixture, fmt.Errorf("remove second Codex projection: %w", err)
	}
	clean, err = commandOutput(repository, 10*time.Second, "git", "status", "--porcelain")
	result.Checks["removal_git_clean"] = err == nil && clean == ""

	var failed []string
	for name, passed := range result.Checks {
		if !passed {
			failed = append(failed, name)
		}
	}
	sort.Strings(failed)
	if len(failed) > 0 {
		return result, fixture, fmt.Errorf("native Codex qualification checks failed: %s", strings.Join(failed, ", "))
	}
	result.Result = "pass"
	return result, fixture, nil
}

func matchesSentinel(body []byte, sentinel string) bool {
	return string(body) == sentinel || string(body) == sentinel+"\n"
}

func prepareDistinctWorktree(repository, target string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
		return "", err
	}
	if _, err := commandOutput(repository, 30*time.Second, "git", "worktree", "add", "-b", "maestro-native-second", absolute, "HEAD"); err != nil {
		return "", err
	}
	return absolute, nil
}

type qualificationWorkspaceStatus struct {
	State        string `json:"state"`
	RepositoryID string `json:"repository_id"`
	WorkspaceID  string `json:"workspace_id"`
}

func enrolledWorkspaceStatus(workdir, cli, runtimeName, target string) (qualificationWorkspaceStatus, error) {
	body, err := commandOutput(workdir, 30*time.Second, cli, "workspace", "status", "--runtime", runtimeName, target)
	if err != nil {
		return qualificationWorkspaceStatus{}, err
	}
	var status qualificationWorkspaceStatus
	if err := json.Unmarshal([]byte(body), &status); err != nil {
		return qualificationWorkspaceStatus{}, err
	}
	if status.State != "enrolled" || status.RepositoryID == "" || status.WorkspaceID == "" {
		return qualificationWorkspaceStatus{}, errors.New("workspace qualification status is not enrolled and complete")
	}
	return status, nil
}

func canonicalSkillIDs(hub string) ([]string, error) {
	var catalog struct {
		SchemaVersion int `json:"schema_version"`
		Skills        []struct {
			ID string `json:"id"`
		} `json:"skills"`
	}
	if err := readJSON(filepath.Join(hub, "bundles", "base", "skills", "catalog.json"), &catalog); err != nil {
		return nil, err
	}
	if catalog.SchemaVersion != 1 || len(catalog.Skills) == 0 {
		return nil, errors.New("canonical skill catalog is invalid")
	}
	ids := make([]string, 0, len(catalog.Skills))
	seen := map[string]bool{}
	for _, skill := range catalog.Skills {
		if skill.ID == "" || seen[skill.ID] {
			return nil, errors.New("canonical skill catalog contains an invalid ID")
		}
		seen[skill.ID] = true
		ids = append(ids, skill.ID)
	}
	sort.Strings(ids)
	return ids, nil
}

func completeSkillProjection(repository, runtimeName string, skillIDs []string) bool {
	root := filepath.Join(repository, "."+runtimeName, "skills")
	for _, skillID := range skillIDs {
		if !regular(filepath.Join(root, skillID, "SKILL.md")) {
			return false
		}
	}
	return len(skillIDs) > 0
}

func containsSpecialistTopology(text string) bool {
	for _, role := range []string{"client-account-agent", "case-agent", "yoda", "darwin", "pa-expert"} {
		if !strings.Contains(text, role) {
			return false
		}
	}
	return true
}

func specialistTopologyProjected(promptInput, managedInstructions string, orchestrationUnavailable bool) bool {
	discovered := strings.Contains(promptInput, "AGENTS.md") && strings.Contains(promptInput, "Maestro")
	return discovered && orchestrationUnavailable && containsSpecialistTopology(managedInstructions)
}

func prepareLegacyHubCandidate(dataRoot, sentinel string) (string, []byte, error) {
	path := filepath.Join(dataRoot, "memory", "recent", "native-reviewed-memory.md")
	body := []byte(sentinel + "\n")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", nil, err
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return "", nil, err
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return "", nil, err
	}
	return path, body, nil
}

func bridgeLegacyHubCandidate(workdir, cli, runtimeName, sentinel string) error {
	inspect, err := commandOutput(workdir, 30*time.Second, cli, "workspace", "memory", "bridge", "inspect", "--runtime", runtimeName, "--workspace", workdir)
	if err != nil {
		return err
	}
	var report struct {
		Candidates []struct {
			ID string `json:"candidate_id"`
		} `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(inspect), &report); err != nil {
		return err
	}
	selected := ""
	for _, candidate := range report.Candidates {
		preview, previewErr := commandOutput(workdir, 30*time.Second, cli, "workspace", "memory", "bridge", "preview", "--runtime", runtimeName, "--workspace", workdir, "--candidate", candidate.ID)
		if previewErr != nil {
			return previewErr
		}
		if strings.Contains(preview, sentinel) {
			selected = candidate.ID
			break
		}
	}
	if selected == "" {
		return errors.New("synthetic legacy Hub candidate was not discoverable by opaque preview")
	}
	result, err := commandOutput(workdir, 30*time.Second, cli, "workspace", "memory", "bridge", "apply", "--runtime", runtimeName, "--workspace", workdir, "--candidates", selected, "--attest-target-scope", "--confirm")
	if err != nil {
		return err
	}
	if !strings.Contains(result, `"state": "applied"`) && !strings.Contains(result, `"state": "already_applied"`) {
		return errors.New("synthetic legacy Hub candidate did not activate")
	}
	return nil
}

func capabilityState(hub, capabilityID, runtimeName string) (string, error) {
	var manifest struct {
		Capabilities []struct {
			ID       string `json:"id"`
			Runtimes map[string]struct {
				State string `json:"state"`
			} `json:"runtimes"`
		} `json:"capabilities"`
	}
	if err := readJSON(filepath.Join(hub, "bundles", "base", "runtime", "capabilities.json"), &manifest); err != nil {
		return "", err
	}
	for _, capability := range manifest.Capabilities {
		if capability.ID == capabilityID {
			return capability.Runtimes[runtimeName].State, nil
		}
	}
	return "", errors.New("canonical runtime capability is missing")
}

func newQualificationSessionID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

func prepareIsolatedCodexHome(fixture string) (string, string, error) {
	sourceRoot := strings.TrimSpace(os.Getenv("CODEX_HOME"))
	if sourceRoot == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		sourceRoot = filepath.Join(home, ".codex")
	}
	authPath := filepath.Join(sourceRoot, "auth.json")
	authInfo, err := os.Lstat(authPath)
	if err != nil {
		return "", "", err
	}
	if authInfo.Mode()&os.ModeSymlink != 0 || !authInfo.Mode().IsRegular() || authInfo.Size() > 1<<20 {
		return "", "", errors.New("Codex auth source must be a bounded regular non-symlink file")
	}
	authBody, err := os.ReadFile(authPath)
	if err != nil {
		return "", "", err
	}
	isolated := filepath.Join(fixture, ".codex-home")
	if err := os.Mkdir(isolated, 0o700); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(filepath.Join(isolated, "auth.json"), authBody, 0o600); err != nil {
		return "", "", err
	}
	configBody := []byte("[features]\nhooks = true\n")
	configPath := filepath.Join(isolated, "config.toml")
	if err := os.WriteFile(configPath, configBody, 0o600); err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(configBody)
	return isolated, hex.EncodeToString(digest[:]), nil
}

func prepareIsolatedClaudeConfig(fixture string) (string, string, error) {
	sourceRoot := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR"))
	var sourceState string
	if sourceRoot != "" {
		sourceState = filepath.Join(sourceRoot, ".claude.json")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		sourceState = filepath.Join(home, ".claude.json")
	}
	return prepareIsolatedClaudeConfigFrom(fixture, sourceState)
}

func prepareIsolatedClaudeConfigFrom(fixture, sourceState string) (string, string, error) {
	info, err := os.Lstat(sourceState)
	if err != nil {
		return "", "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > 1<<20 {
		return "", "", errors.New("Claude state source must be a bounded regular non-symlink file")
	}
	body, err := os.ReadFile(sourceState)
	if err != nil {
		return "", "", err
	}
	var source map[string]json.RawMessage
	if err := json.Unmarshal(body, &source); err != nil {
		return "", "", fmt.Errorf("decode Claude state source: %w", err)
	}
	filtered := map[string]json.RawMessage{}
	for _, key := range []string{
		"hasCompletedOnboarding",
		"lastOnboardingVersion",
		"userID",
		"machineID",
		"hasAvailableSubscription",
		"installMethod",
	} {
		if value, ok := source[key]; ok {
			filtered[key] = value
		}
	}
	if rawAccount, ok := source["oauthAccount"]; ok {
		var account map[string]json.RawMessage
		if err := json.Unmarshal(rawAccount, &account); err != nil {
			return "", "", fmt.Errorf("decode Claude OAuth account bootstrap: %w", err)
		}
		filteredAccount := map[string]json.RawMessage{}
		for _, key := range []string{"accountUuid", "organizationUuid"} {
			if value, present := account[key]; present {
				filteredAccount[key] = value
			}
		}
		encodedAccount, err := json.Marshal(filteredAccount)
		if err != nil {
			return "", "", err
		}
		filtered["oauthAccount"] = encodedAccount
	}
	if len(filtered) == 0 {
		return "", "", errors.New("Claude state source omitted the supported authentication bootstrap")
	}
	isolatedBody, err := json.MarshalIndent(filtered, "", "  ")
	if err != nil {
		return "", "", err
	}
	isolatedBody = append(isolatedBody, '\n')
	root := filepath.Join(fixture, ".claude-home")
	if err := os.Mkdir(root, 0o700); err != nil {
		return "", "", err
	}
	statePath := filepath.Join(root, ".claude.json")
	if err := os.WriteFile(statePath, isolatedBody, 0o600); err != nil {
		return "", "", err
	}
	digest := sha256.Sum256(isolatedBody)
	return root, hex.EncodeToString(digest[:]), nil
}

func receiptSnapshot(root string) (map[string]bool, error) {
	snapshot := map[string]bool{}
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return snapshot, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read lifecycle receipt directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			snapshot[entry.Name()] = true
		}
	}
	return snapshot, nil
}

type receiptEvidence struct {
	Counts        map[string]int
	ActionDigests map[string]int
}

func validatedReceiptDelta(dataRoot, workspaceID, runtimeName string, before map[string]bool) (receiptEvidence, error) {
	if _, err := lifecycle.DiagnoseRuntime(dataRoot, workspaceID, runtimeName); err != nil {
		return receiptEvidence{}, err
	}
	root := filepath.Join(dataRoot, "runtime", "receipts", workspaceID)
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return receiptEvidence{Counts: map[string]int{}, ActionDigests: map[string]int{}}, nil
	}
	if err != nil {
		return receiptEvidence{}, err
	}
	evidence := receiptEvidence{Counts: map[string]int{}, ActionDigests: map[string]int{}}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || before[entry.Name()] {
			continue
		}
		file, err := os.Open(filepath.Join(root, entry.Name()))
		if err != nil {
			return receiptEvidence{}, err
		}
		var receipt lifecycle.Receipt
		decoder := json.NewDecoder(io.LimitReader(file, 8<<10))
		decoder.DisallowUnknownFields()
		decodeErr := decoder.Decode(&receipt)
		closeErr := file.Close()
		if decodeErr != nil {
			return receiptEvidence{}, fmt.Errorf("decode lifecycle receipt %s: %w", entry.Name(), decodeErr)
		}
		if closeErr != nil {
			return receiptEvidence{}, closeErr
		}
		if receipt.Runtime == runtimeName {
			evidence.Counts[receipt.Event]++
			if receipt.ActionSHA256 != "" {
				evidence.ActionDigests[receipt.ActionSHA256]++
			}
		}
	}
	return evidence, nil
}

func runCodex(workdir, codexHome, executable, model, prompt string) ([]byte, error) {
	return runCodexWithArgs(workdir, codexHome, executable, codexArgs(model, prompt))
}

func runCodexPersistent(workdir, codexHome, executable, model, prompt string) ([]byte, error) {
	return runCodexWithArgs(workdir, codexHome, executable, codexPersistentArgs(model, prompt))
}

func resumeCodex(workdir, codexHome, executable, model, sessionID, prompt string) ([]byte, error) {
	return runCodexWithArgs(workdir, codexHome, executable, codexResumeArgs(model, sessionID, prompt))
}

func runCodexWithArgs(workdir, codexHome, executable string, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = workdir
	command.Env = codexIsolatedEnvironment(os.Environ(), codexHome, workdir)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	if ctx.Err() != nil {
		return stdout.Bytes(), errors.New("Codex session exceeded ten minutes")
	}
	if err != nil {
		return stdout.Bytes(), fmt.Errorf("Codex exited unsuccessfully (%v; stderr bytes=%d)", err, stderr.Len())
	}
	return stdout.Bytes(), nil
}

func codexIsolatedEnvironment(environment []string, codexHome, workdir string) []string {
	isolated := make([]string, 0, len(environment)+2)
	for _, entry := range environment {
		if strings.HasPrefix(entry, "CODEX_HOME=") || strings.HasPrefix(entry, "PWD=") {
			continue
		}
		isolated = append(isolated, entry)
	}
	return append(isolated, "CODEX_HOME="+codexHome, "PWD="+workdir)
}

func runClaude(workdir, executable, model, maxBudget, prompt string) ([]byte, error) {
	return runClaudeIsolated(workdir, "", executable, model, maxBudget, prompt)
}

func runClaudeIsolated(workdir, configRoot, executable, model, maxBudget, prompt string) ([]byte, error) {
	args := claudeArgs(model, maxBudget, prompt)
	stream, err := runClaudeArgsOnce(workdir, configRoot, executable, args)
	if err == nil || len(bytes.TrimSpace(stream)) > 0 {
		return stream, err
	}
	// Claude can occasionally terminate before emitting its first stream item
	// (for example while refreshing local auth). One fresh retry is bounded and
	// remains visible if it also fails.
	return runClaudeArgsOnce(workdir, configRoot, executable, args)
}

func runClaudePersistent(workdir, configRoot, executable, model, maxBudget, sessionID, prompt string) ([]byte, error) {
	return runClaudeArgsOnce(workdir, configRoot, executable, claudePersistentArgs(model, maxBudget, sessionID, prompt))
}

func resumeClaude(workdir, configRoot, executable, model, maxBudget, sessionID, prompt string) ([]byte, error) {
	return runClaudeArgsOnce(workdir, configRoot, executable, claudeResumeArgs(model, maxBudget, sessionID, prompt))
}

func runClaudeArgsOnce(workdir, configRoot, executable string, args []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = workdir
	if configRoot != "" {
		command.Env = claudeIsolatedEnvironment(os.Environ(), configRoot)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	if ctx.Err() != nil {
		return nil, errors.New("Claude session exceeded four minutes")
	}
	if err != nil {
		return stdout.Bytes(), fmt.Errorf("Claude exited unsuccessfully (%v; stderr bytes=%d)", err, stderr.Len())
	}
	return stdout.Bytes(), nil
}

func claudeIsolatedEnvironment(ambient []string, configRoot string) []string {
	result := make([]string, 0, len(ambient)+2)
	for _, value := range ambient {
		if strings.HasPrefix(value, "CLAUDE_CONFIG_DIR=") || strings.HasPrefix(value, "CLAUDE_SECURESTORAGE_CONFIG_DIR=") {
			continue
		}
		result = append(result, value)
	}
	return append(result,
		"CLAUDE_CONFIG_DIR="+configRoot,
		"CLAUDE_SECURESTORAGE_CONFIG_DIR=",
	)
}

func claudeArgs(model, maxBudget, prompt string) []string {
	return []string{
		"-p", "--verbose", "--model", model,
		"--output-format", "stream-json", "--include-hook-events",
		"--setting-sources", "project,local", "--permission-mode", "acceptEdits",
		"--allowedTools", "Read,Write,Bash,Task,Skill", "--no-session-persistence",
		"--max-budget-usd", maxBudget, prompt,
	}
}

func claudePersistentArgs(model, maxBudget, sessionID, prompt string) []string {
	return []string{
		"-p", "--verbose", "--model", model,
		"--output-format", "stream-json", "--include-hook-events",
		"--setting-sources", "project,local", "--permission-mode", "acceptEdits",
		"--allowedTools", "Read,Write,Bash,Task,Skill", "--session-id", sessionID,
		"--max-budget-usd", maxBudget, prompt,
	}
}

func claudeResumeArgs(model, maxBudget, sessionID, prompt string) []string {
	return []string{
		"-p", "--verbose", "--resume", sessionID, "--model", model,
		"--output-format", "stream-json", "--include-hook-events",
		"--setting-sources", "project,local", "--permission-mode", "acceptEdits",
		"--allowedTools", "Read,Write,Bash,Task,Skill",
		"--max-budget-usd", maxBudget, prompt,
	}
}

func inspectClaudeStream(stream []byte) (streamEvidence, error) {
	evidence := streamEvidence{Hooks: map[string]bool{}, Agents: map[string]bool{}, Skills: map[string]bool{}}
	scanner := bufio.NewScanner(bytes.NewReader(stream))
	scanner.Buffer(make([]byte, 64<<10), 4<<20)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var item map[string]any
		if err := json.Unmarshal(line, &item); err != nil {
			return streamEvidence{}, fmt.Errorf("decode Claude stream line: %w", err)
		}
		if item["type"] == "system" {
			if item["subtype"] == "init" && evidence.SessionID == "" {
				evidence.SessionID, _ = item["session_id"].(string)
			}
			if event, ok := item["hook_event"].(string); ok && item["subtype"] == "hook_response" {
				evidence.Hooks[event] = true
			}
			if item["subtype"] == "init" {
				captureStrings(item["agents"], evidence.Agents)
				captureStrings(item["skills"], evidence.Skills)
			}
		}
		walkStreamValue(item, &evidence)
	}
	if err := scanner.Err(); err != nil {
		return streamEvidence{}, err
	}
	return evidence, nil
}

func mergeStreamEvidence(items ...streamEvidence) streamEvidence {
	merged := streamEvidence{
		Hooks:     map[string]bool{},
		Agents:    map[string]bool{},
		Skills:    map[string]bool{},
		ItemTypes: map[string]int{},
	}
	for _, item := range items {
		if merged.SessionID == "" {
			merged.SessionID = item.SessionID
		}
		for name, observed := range item.Hooks {
			merged.Hooks[name] = merged.Hooks[name] || observed
		}
		for name, observed := range item.Agents {
			merged.Agents[name] = merged.Agents[name] || observed
		}
		for name, observed := range item.Skills {
			merged.Skills[name] = merged.Skills[name] || observed
		}
		for name, count := range item.ItemTypes {
			merged.ItemTypes[name] += count
		}
		merged.InvokedDoctor = merged.InvokedDoctor || item.InvokedDoctor
		merged.ReadHubDoctor = merged.ReadHubDoctor || item.ReadHubDoctor
		merged.GuardDenied = merged.GuardDenied || item.GuardDenied
		merged.GuardAttempted = merged.GuardAttempted || item.GuardAttempted
		merged.GuardExecuted = merged.GuardExecuted || item.GuardExecuted
		merged.ManagedSubagent = merged.ManagedSubagent || item.ManagedSubagent
		merged.DoctorStatusChecked = merged.DoctorStatusChecked || item.DoctorStatusChecked
		merged.DoctorVersionChecked = merged.DoctorVersionChecked || item.DoctorVersionChecked
		if merged.DoctorVersionOutput == "" {
			merged.DoctorVersionOutput = item.DoctorVersionOutput
		}
		merged.CommandCount += item.CommandCount
		merged.FileChangeCount += item.FileChangeCount
		merged.ExternalToolCount += item.ExternalToolCount
		merged.ToolUseCount += item.ToolUseCount
		merged.Text += item.Text
	}
	return merged
}

func walkStreamValue(value any, evidence *streamEvidence) {
	switch typed := value.(type) {
	case map[string]any:
		if typed["type"] == "tool_use" {
			evidence.ToolUseCount++
		}
		if typed["type"] == "text" {
			if text, ok := typed["text"].(string); ok {
				evidence.Text += text + "\n"
			}
		}
		if typed["type"] == "tool_use" && typed["name"] == "Skill" {
			if input, ok := typed["input"].(map[string]any); ok && input["skill"] == "maestro-doctor" {
				evidence.InvokedDoctor = true
			}
		}
		if typed["type"] == "tool_use" && typed["name"] == "Read" {
			if input, ok := typed["input"].(map[string]any); ok {
				path, _ := input["file_path"].(string)
				if strings.HasSuffix(filepath.ToSlash(path), "bundles/base/skills/maestro-doctor/SKILL.md") {
					evidence.ReadHubDoctor = true
				}
			}
		}
		for _, child := range typed {
			walkStreamValue(child, evidence)
		}
	case []any:
		for _, child := range typed {
			walkStreamValue(child, evidence)
		}
	case string:
		if strings.Contains(typed, "dangerous shell or Git mutation is not authorized by the workspace projection") {
			evidence.GuardDenied = true
		}
		if strings.Contains(typed, "managed Maestro specialist") {
			evidence.ManagedSubagent = true
		}
	}
}

func captureStrings(value any, target map[string]bool) {
	items, ok := value.([]any)
	if !ok {
		return
	}
	for _, item := range items {
		if text, ok := item.(string); ok {
			target[text] = true
		}
	}
}

func commandOutput(workdir string, timeout time.Duration, name string, args ...string) (string, error) {
	return commandOutputWithEnv(workdir, timeout, nil, name, args...)
}

func commandOutputWithEnv(workdir string, timeout time.Duration, environment []string, name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = workdir
	command.Env = qualificationCommandEnvironment(os.Environ(), environment)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("%s failed: %w (stderr bytes=%d)", filepath.Base(name), err, stderr.Len())
	}
	return strings.TrimSpace(stdout.String()), nil
}

func qualificationCommandEnvironment(environment, overrides []string) []string {
	isolated := make([]string, 0, len(environment)+len(overrides))
	for _, variable := range environment {
		if strings.HasPrefix(variable, "GIT_INDEX_FILE=") ||
			strings.HasPrefix(variable, "GIT_DIR=") ||
			strings.HasPrefix(variable, "GIT_WORK_TREE=") ||
			strings.HasPrefix(variable, "GIT_PREFIX=") ||
			strings.HasPrefix(variable, "GIT_CONFIG_PARAMETERS=") {
			continue
		}
		isolated = append(isolated, variable)
	}
	return append(isolated, overrides...)
}

func extractZIP(source, destination string) error {
	reader, err := zip.OpenReader(source)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, entry := range reader.File {
		clean := filepath.Clean(filepath.FromSlash(entry.Name))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("archive contains unsafe path %q", entry.Name)
		}
		if entry.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("archive contains unsupported symlink %q", entry.Name)
		}
		target := filepath.Join(destination, clean)
		if entry.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		input, err := entry.Open()
		if err != nil {
			return err
		}
		mode := entry.Mode().Perm()
		if mode == 0 {
			mode = 0o600
		}
		output, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
		if err != nil {
			input.Close()
			return err
		}
		_, copyErr := io.Copy(output, io.LimitReader(input, 256<<20))
		closeErr := errors.Join(input.Close(), output.Close())
		if copyErr != nil || closeErr != nil {
			return errors.Join(copyErr, closeErr)
		}
	}
	return nil
}

func readJSON(path string, target any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, target)
}

func preparePortableOwnerFixture(dataRoot string) error {
	files := map[string]string{
		filepath.Join("owner", "registry.json"):                  "{\n  \"schema_version\": 1,\n  \"trees\": {\"self\":\"owner/self/\",\"operating\":\"owner/operating/\",\"observations\":\"owner/observations/\",\"interview\":\"owner/interview/\"},\n  \"initialized\": true,\n  \"owner_type\": \"distro-adopter\",\n  \"personal_context\": {\"state\":\"declined\",\"state_timestamp\":\"2026-08-28T00:00:00Z\",\"source_file\":\"owner/self/personal-context.md\"},\n  \"onboarding_mode\": \"quick\"\n}\n",
		filepath.Join("profile", "onboarding.json"):              "{\"status\":\"complete\",\"track\":\"quick\",\"completed_at\":\"2026-08-28T00:00:00Z\",\"version\":\"synthetic\"}\n",
		filepath.Join("profile", "identity.json"):                "{\"schema_version\":1,\"name\":\"Qualification Owner\",\"role\":\"Synthetic engineering qualification role\",\"initialized\":true}\n",
		filepath.Join("owner", "self", "owner-identity.md"):      "# Owner identity\n\n## Current\n\nQualification Owner\n",
		filepath.Join("owner", "self", "professional-role.md"):   "# Professional role\n\n## Current\n\nSynthetic engineering qualification role\n",
		filepath.Join("owner", "self", "communication-style.md"): "# Communication style\n\n## Current\n\nConclusion first.\n",
		filepath.Join("owner", "self", "preferences.md"):         "# Preferences\n\n## Current\n\nDeterministic evidence.\n",
		filepath.Join("owner", "self", "quality-bar.md"):         "# Quality bar\n\n## Current\n\nNative proof required.\n",
		filepath.Join("owner", "self", "personal-context.md"):    "# Authorized personal context\n\n## Current\n\nNATIVE-PRIVATE-SENTINEL-MUST-NOT-APPEAR\n",
	}
	for relative, body := range files {
		path := filepath.Join(dataRoot, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func regular(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

func fileDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "native qualification failed:", err)
	os.Exit(1)
}
