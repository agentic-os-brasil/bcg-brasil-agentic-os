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

	devharness "github.com/DScardini91/bcg-brasil-agentic-os/internal/dev/harness"
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
	ToolInput struct {
		Command  string `json:"command"`
		FilePath string `json:"file_path"`
		Path     string `json:"path"`
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
	markerDir := filepath.Join(root, ".git", "bcg-harness")
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
	validated, err := os.ReadFile(filepath.Join(root, ".git", "bcg-harness", "validated-tree"))
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
		fmt.Fprintln(&buffer, "Claude Code e o runtime principal de desenvolvimento.")
		fmt.Fprintln(&buffer, "Use as skills nativas conforme .claude/skill-routing.json; nao ignore o harness.")
		return 0, writeHook(out, "SessionStart", buffer.String())
	case "pre-tool":
		if reason, recovery, blocked := BlockedCommand(input.ToolInput.Command); blocked {
			message := fmt.Sprintf("BLOQUEADO: %s. Nada foi apagado. Proximo comando seguro: %s", reason, recovery)
			return 0, writeDeniedHook(out, message)
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
