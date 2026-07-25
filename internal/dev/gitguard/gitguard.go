package gitguard

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/clauderouting"
	devharness "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/dev/harness"
)

var destructivePatterns = []struct {
	pattern  *regexp.Regexp
	reason   string
	recovery string
}{
	{regexp.MustCompile(`(?i)git\s+push\b[^\n]*(--force|-f\b)`), "force push pode apagar o historico remoto", "git status"},
	{regexp.MustCompile(`(?i)git\s+push\b[^\n]*(origin\s+)?(main|master)(\s|$)`), "push direto na branch principal ignora a revisao humana", "go run ./dev/harness doctor"},
	{regexp.MustCompile(`(?i)git\s+reset\s+--hard\b`), "reset --hard pode apagar trabalho local", "go run ./dev/harness recover"},
	{regexp.MustCompile(`(?i)git\s+clean\b[^\n]*-[a-z]*f`), "git clean pode apagar arquivos ainda nao salvos", "go run ./dev/harness recover"},
	{regexp.MustCompile(`(?i)git\s+branch\s+-D\b`), "excluir uma branch a forca pode apagar trabalho", "go run ./dev/harness recover"},
	{regexp.MustCompile(`(?i)git\s+push\b[^\n]*--delete\b`), "excluir branch remota exige revisao humana", "git status"},
	{regexp.MustCompile(`(?i)gh\s+pr\s+merge\b`), "o agente prepara o PR, mas uma pessoa decide o merge", "gh pr view --web"},
}

var sensitiveExtensions = map[string]bool{
	".csv": true, ".docx": true, ".parquet": true, ".pdf": true, ".pptx": true, ".xls": true, ".xlsx": true,
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`\bghp_[A-Za-z0-9]{20,}\b`),
	regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{20,}\b`),
}

type hookInput struct {
	SessionID   string `json:"session_id"`
	ToolName    string `json:"tool_name"`
	CommandName string `json:"command_name"`
	ToolInput   struct {
		Command   string `json:"command"`
		FilePath  string `json:"file_path"`
		Name      string `json:"name"`
		Path      string `json:"path"`
		Skill     string `json:"skill"`
		SkillName string `json:"skill_name"`
	} `json:"tool_input"`
}

type hookOutput struct {
	HookSpecificOutput struct {
		HookEventName            string `json:"hookEventName"`
		AdditionalContext        string `json:"additionalContext,omitempty"`
		PermissionDecision       string `json:"permissionDecision,omitempty"`
		PermissionDecisionReason string `json:"permissionDecisionReason,omitempty"`
	} `json:"hookSpecificOutput"`
}

// BlockedCommand classifies irreversible or governed Git commands.
func BlockedCommand(command string) (reason, recovery string, blocked bool) {
	for _, candidate := range destructivePatterns {
		if candidate.pattern.MatchString(command) {
			return candidate.reason, candidate.recovery, true
		}
	}
	return "", "", false
}

// Setup installs the repository-owned Git hooks for this clone.
func Setup(root string, out io.Writer) error {
	if err := git(root, "config", "core.hooksPath", ".githooks"); err != nil {
		return err
	}
	fmt.Fprintln(out, "[ok] hooks locais instalados")
	return Doctor(root, out)
}

// Doctor reports contribution readiness without changing files or Git history.
func Doctor(root string, out io.Writer) error {
	fmt.Fprintln(out, "BCG Agentic OS - diagnostico de contribuicao")
	checks := []struct {
		label string
		args  []string
	}{
		{"branch", []string{"branch", "--show-current"}},
		{"remote", []string{"remote", "get-url", "origin"}},
		{"nome Git", []string{"config", "user.name"}},
		{"email Git", []string{"config", "user.email"}},
		{"hooks", []string{"config", "core.hooksPath"}},
	}
	problems := 0
	for _, check := range checks {
		value, err := gitOutput(root, check.args...)
		if err != nil || strings.TrimSpace(value) == "" {
			fmt.Fprintf(out, "[aviso] %s nao configurado\n", check.label)
			problems++
			continue
		}
		fmt.Fprintf(out, "[ok] %s: %s\n", check.label, strings.TrimSpace(value))
	}
	status, err := gitOutput(root, "status", "--short")
	if err != nil {
		return err
	}
	dirty := status != ""
	if !dirty {
		fmt.Fprintln(out, "[ok] nenhum arquivo local pendente")
	} else {
		fmt.Fprintln(out, "[aviso] ha arquivos locais pendentes; nada foi alterado")
	}
	if dirty {
		fmt.Fprintln(out, "proximo passo seguro: use $recover-work no agente")
	} else if problems > 0 {
		fmt.Fprintln(out, "proximo passo seguro: use $start-contributing no agente")
	} else {
		fmt.Fprintln(out, "proximo passo seguro: use $start-work no agente")
	}
	return nil
}

// Recover inspects state and gives one non-destructive next action.
func Recover(root string, out io.Writer) error {
	branch, _ := gitOutput(root, "branch", "--show-current")
	status, err := gitOutput(root, "status", "--short")
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "branch atual: %s\n", branch)
	if status == "" {
		fmt.Fprintln(out, "nenhum trabalho local pendente; nada precisa ser recuperado")
		fmt.Fprintln(out, "proximo passo seguro: go run ./dev/harness doctor")
		return nil
	}
	fmt.Fprintln(out, "seus arquivos continuam no disco; nenhum comando destrutivo foi executado")
	fmt.Fprintln(out, status)
	fmt.Fprintln(out, "proximo passo seguro: peca ao agente para usar $recover-work")
	return nil
}

// PreCommit validates the exact staged snapshot before allowing a commit.
func PreCommit(root string, out io.Writer) error {
	branch, err := gitOutput(root, "branch", "--show-current")
	if err != nil {
		return err
	}
	if branch == "main" || branch == "master" {
		return block("commits na branch principal nao sao permitidos", "go run ./dev/harness recover")
	}
	unstaged, err := gitOutput(root, "diff", "--name-only")
	if err != nil {
		return err
	}
	if unstaged != "" {
		return block("ha mudancas fora do commit; o gate precisa validar exatamente o que sera salvo", "go run ./dev/harness recover")
	}
	untracked, err := gitOutput(root, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return err
	}
	if untracked != "" {
		return block("ha arquivos novos fora do commit; o gate precisa validar exatamente o que sera salvo", "go run ./dev/harness recover")
	}
	if err := scanStaged(root); err != nil {
		return err
	}
	if err := devharness.Validate(root, true, out); err != nil {
		return block("a validacao completa falhou; nenhum commit foi criado", "go run ./dev/harness validate --full")
	}
	tree, err := gitOutput(root, "write-tree")
	if err != nil {
		return err
	}
	markerDir, err := gitPath(root, "bcg-harness")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(markerDir, 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(markerDir, "validated-tree"), []byte(tree+"\n"), 0o600); err != nil {
		return err
	}
	fmt.Fprintln(out, "[ok] snapshot do commit validado")
	return nil
}

// PrePush blocks direct main pushes and requires the committed tree to match validation.
func PrePush(root string, scanner *bufio.Scanner, out io.Writer) error {
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 3 && (fields[2] == "refs/heads/main" || fields[2] == "refs/heads/master") {
			return block("push direto na branch principal nao e permitido", "go run ./dev/harness doctor")
		}
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	validatedPath, err := gitPath(root, filepath.Join("bcg-harness", "validated-tree"))
	if err != nil {
		return err
	}
	validated, err := os.ReadFile(validatedPath)
	if err != nil {
		return block("este trabalho ainda nao tem uma validacao completa registrada", "go run ./dev/harness validate --full")
	}
	current, err := gitOutput(root, "rev-parse", "HEAD^{tree}")
	if err != nil {
		return err
	}
	if strings.TrimSpace(string(validated)) != current {
		return block("o codigo mudou depois da ultima validacao completa", "go run ./dev/harness validate --full")
	}
	fmt.Fprintln(out, "[ok] push corresponde ao snapshot validado")
	return nil
}

func scanStaged(root string) error {
	names, err := gitOutputRaw(root, "diff", "--cached", "--name-only", "-z", "--diff-filter=ACMR")
	if err != nil {
		return err
	}
	for _, name := range strings.Split(names, "\x00") {
		if name == "" {
			continue
		}
		lower := strings.ToLower(name)
		ext := strings.ToLower(filepath.Ext(name))
		if sensitiveExtensions[ext] && !strings.Contains(filepath.ToSlash(lower), "testdata/") {
			return block("arquivo de dados ou documento pode conter informacao de cliente: "+name, "git status")
		}
		if strings.Contains(lower, ".env") || ext == ".pem" || ext == ".key" || ext == ".p12" || ext == ".pfx" {
			return block("arquivo de credencial nao pode entrar no repositorio: "+name, "git status")
		}
		content, err := gitOutputRaw(root, "show", ":"+name)
		if err != nil {
			return block("nao foi possivel verificar o conteudo staged de "+name, "git status")
		}
		for _, pattern := range secretPatterns {
			if pattern.MatchString(content) {
				return block("possivel segredo encontrado em "+name, "git status")
			}
		}
	}
	return nil
}

// ClaudeHook implements the thin Claude adapter around canonical Go policy.
func ClaudeHook(root, event string, in io.Reader, out io.Writer) (int, error) {
	var input hookInput
	if err := json.NewDecoder(in).Decode(&input); err != nil && !errors.Is(err, io.EOF) {
		return 0, fmt.Errorf("decode Claude hook input: %w", err)
	}
	switch event {
	case "session-start":
		var buffer bytes.Buffer
		_ = Doctor(root, &buffer)
		manifest, err := clauderouting.Load(root)
		if err != nil {
			return 0, err
		}
		fmt.Fprintln(&buffer, "Claude Code e o runtime principal de desenvolvimento.")
		fmt.Fprintf(&buffer, "Golden path obrigatorio: %s. Fallback: $%s.\n", dollarSkills(manifest.GoldenPath), manifest.Fallback)
		fmt.Fprintln(&buffer, "O harness registra invocacoes nativas e bloqueia mutacoes sem a skill exigida.")
		return 0, writeHook(out, "SessionStart", buffer.String())
	case "skill-used":
		name := firstNonEmpty(input.ToolInput.SkillName, input.ToolInput.Skill, input.ToolInput.Name, input.ToolInput.Command)
		if err := activateClaudeSkill(root, input.SessionID, name); err != nil {
			return 0, writeHook(out, "PostToolUse", "Skill carregada, mas o harness nao conseguiu registrar a ativacao: "+err.Error())
		}
		return 0, nil
	case "prompt-expansion":
		if err := activateClaudeSkill(root, input.SessionID, input.CommandName); err != nil {
			return 0, err
		}
		return 0, writeHook(out, "UserPromptExpansion", "Skill $"+input.CommandName+" registrada como ativa pelo harness.")
	case "pre-tool":
		if reason, recovery, blocked := BlockedCommand(input.ToolInput.Command); blocked {
			message := fmt.Sprintf("BLOQUEADO: %s. Nada foi apagado. Proximo comando seguro: %s", reason, recovery)
			return 0, writeDeniedHook(out, message)
		}
		if required := requiredSkill(input); required != "" && !claudeSkillActive(root, input.SessionID, required) {
			message := fmt.Sprintf("BLOQUEADO: use $%s antes desta acao. Nada foi alterado; o harness exige evidencia de skill ativa.", required)
			return 0, writeDeniedHook(out, message)
		}
		if input.ToolName == "Bash" && !anyClaudeSkillActive(root, input.SessionID) {
			return 0, writeDeniedHook(out, "BLOQUEADO: nenhuma skill de desenvolvimento foi ativada nesta sessao. Comece com $start-work; nada foi alterado.")
		}
		return 0, nil
	case "post-tool":
		path := input.ToolInput.FilePath
		if path == "" {
			path = input.ToolInput.Path
		}
		if strings.HasSuffix(filepath.ToSlash(path), "docs/decisions/decision-log.md") {
			return 0, writeHook(out, "PostToolUse", "Decision log alterado. Rode: go run ./dev/harness decision check")
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("unknown Claude hook event %q", event)
	}
}

func requiredSkill(input hookInput) string {
	path := filepath.ToSlash(firstNonEmpty(input.ToolInput.FilePath, input.ToolInput.Path))
	if input.ToolName == "Edit" || input.ToolName == "Write" {
		if strings.HasSuffix(path, "docs/decisions/decision-log.md") {
			return "record-decision"
		}
		if memoryPath(path) {
			return "evolve-memory"
		}
		return "develop-change"
	}
	if input.ToolName != "Bash" {
		return ""
	}
	command := input.ToolInput.Command
	normalized := filepath.ToSlash(command)
	if strings.Contains(normalized, "docs/decisions/decision-log.md") {
		return "record-decision"
	}
	if regexp.MustCompile(`(?i)\bgo\s+run\s+\./dev/harness\s+decision\b`).MatchString(command) {
		return "record-decision"
	}
	if regexp.MustCompile(`(?i)\bgo\s+run\s+\./dev/harness\s+recover\b`).MatchString(command) {
		return "recover-work"
	}
	if strings.Contains(normalized, "internal/memory") || strings.Contains(normalized, "bundles/base/memory") || strings.Contains(normalized, "schemas/memory-policy.schema.json") || strings.Contains(normalized, "schemas/memory-artifact.schema.json") || strings.Contains(normalized, "schemas/memory-commit.schema.json") || strings.Contains(normalized, "specs/006-memory-persistence.md") {
		return "evolve-memory"
	}
	if regexp.MustCompile(`(?i)\b(git\s+(commit|push)|gh\s+pr\s+create)\b`).MatchString(command) {
		return "prepare-pr"
	}
	if regexp.MustCompile(`(?i)\bgit\s+add\b`).MatchString(command) {
		return "prepare-pr"
	}
	if regexp.MustCompile(`(?i)\bgit\s+(switch|checkout)\b`).MatchString(command) {
		return "start-work"
	}
	if regexp.MustCompile(`(?i)^\s*git\s+pull\s+--ff-only(\s|$)`).MatchString(command) {
		return "start-work"
	}
	if strings.Contains(normalized, "dev/bootstrap/") || regexp.MustCompile(`(?i)\bgo\s+run\s+\./dev/harness\s+setup\b`).MatchString(command) || regexp.MustCompile(`(?i)\bgit\s+config\b`).MatchString(command) {
		return "start-contributing"
	}
	if readOnlyBash(command) {
		return ""
	}
	return "develop-change"
}

func memoryPath(path string) bool {
	return strings.Contains(path, "/internal/memory/") || strings.HasPrefix(path, "internal/memory/") ||
		strings.Contains(path, "/bundles/base/memory/") || strings.HasPrefix(path, "bundles/base/memory/") ||
		strings.HasSuffix(path, "schemas/memory-policy.schema.json") ||
		strings.HasSuffix(path, "schemas/memory-artifact.schema.json") ||
		strings.HasSuffix(path, "schemas/memory-commit.schema.json") ||
		strings.HasSuffix(path, "specs/006-memory-persistence.md")
}

func readOnlyBash(command string) bool {
	trimmed := strings.TrimSpace(command)
	if trimmed == "" {
		return true
	}
	if strings.ContainsAny(trimmed, ";>|`") || strings.Contains(trimmed, "&&") || strings.Contains(trimmed, "||") || strings.Contains(trimmed, "$(") {
		return false
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`^pwd\s*$`),
		regexp.MustCompile(`^ls(\s+-[A-Za-z]+)*(\s+\.?)?\s*$`),
		regexp.MustCompile(`^git\s+status(\s+--(short|porcelain|branch))*\s*$`),
		regexp.MustCompile(`^git\s+diff(\s+--(stat|check|cached|staged|name-only))*\s*$`),
		regexp.MustCompile(`^git\s+branch\s+--show-current(\s|$)`),
		regexp.MustCompile(`^git\s+remote\s+get-url\s+origin\s*$`),
		regexp.MustCompile(`^git\s+rev-parse\s+--show-toplevel\s*$`),
		regexp.MustCompile(`^go\s+run\s+\./dev/harness\s+(validate|doctor|decision\s+check)(\s|$)`),
	}
	for _, pattern := range patterns {
		if pattern.MatchString(trimmed) {
			return true
		}
	}
	return false
}

func activateClaudeSkill(root, sessionID, rawName string) error {
	manifest, err := clauderouting.Load(root)
	if err != nil {
		return err
	}
	fields := strings.Fields(strings.TrimSpace(rawName))
	if len(fields) == 0 {
		return errors.New("Claude nao informou qual skill foi invocada")
	}
	name := strings.TrimPrefix(fields[0], "/")
	if !manifest.HasSkill(name) {
		return fmt.Errorf("skill Claude nao roteada: %s", name)
	}
	path, err := claudeSessionPath(root, sessionID)
	if err != nil {
		return err
	}
	active, _ := os.ReadFile(path)
	if strings.Contains("\n"+string(active), "\n"+name+"\n") {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	return os.WriteFile(path, append(active, []byte(name+"\n")...), 0o600)
}

func claudeSkillActive(root, sessionID, name string) bool {
	path, err := claudeSessionPath(root, sessionID)
	if err != nil {
		return false
	}
	active, err := os.ReadFile(path)
	return err == nil && strings.Contains("\n"+string(active), "\n"+name+"\n")
}

func anyClaudeSkillActive(root, sessionID string) bool {
	path, err := claudeSessionPath(root, sessionID)
	if err != nil {
		return false
	}
	active, err := os.ReadFile(path)
	return err == nil && strings.TrimSpace(string(active)) != ""
}

func claudeSessionPath(root, sessionID string) (string, error) {
	if !regexp.MustCompile(`^[A-Za-z0-9._-]+$`).MatchString(sessionID) {
		return "", fmt.Errorf("Claude session_id ausente ou invalido; reinicie a sessao antes de modificar arquivos")
	}
	return gitPath(root, filepath.Join("bcg-harness", "claude-sessions", sessionID+".skills"))
}

func gitPath(root, name string) (string, error) {
	resolved, err := gitOutput(root, "rev-parse", "--git-path", filepath.ToSlash(name))
	if err != nil {
		return "", err
	}
	resolved = strings.TrimSpace(resolved)
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(root, filepath.FromSlash(resolved))
	}
	return filepath.Clean(resolved), nil
}

func dollarSkills(names []string) string {
	withPrefix := make([]string, 0, len(names))
	for _, name := range names {
		withPrefix = append(withPrefix, "$"+name)
	}
	return strings.Join(withPrefix, " -> ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func writeHook(out io.Writer, event, context string) error {
	var output hookOutput
	output.HookSpecificOutput.HookEventName = event
	output.HookSpecificOutput.AdditionalContext = strings.TrimSpace(context)
	return json.NewEncoder(out).Encode(output)
}

func writeDeniedHook(out io.Writer, reason string) error {
	var output hookOutput
	output.HookSpecificOutput.HookEventName = "PreToolUse"
	output.HookSpecificOutput.PermissionDecision = "deny"
	output.HookSpecificOutput.PermissionDecisionReason = strings.TrimSpace(reason)
	return json.NewEncoder(out).Encode(output)
}

func block(reason, recovery string) error {
	return fmt.Errorf("BLOQUEADO: %s. Nada foi apagado. Proximo comando seguro: %s", reason, recovery)
}

func git(root string, args ...string) error {
	_, err := gitOutput(root, args...)
	return err
}

func gitOutput(root string, args ...string) (string, error) {
	output, err := gitOutputRaw(root, args...)
	return strings.TrimSpace(output), err
}

func gitOutputRaw(root string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}
