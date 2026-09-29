// Package zipruntime implements the native ZIP helper without Python dependencies.
package zipruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/codexadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipmigration"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Run dispatches bounded native hook input. JSON denial uses exit zero, never
// continue:false, which is not a supported PreToolUse enforcement response.
func Run(args []string, in io.Reader, out io.Writer) error {
	if len(args) == 3 && args[0] == "bind-codex" && args[1] == "--root" {
		if err := BindCodex(args[2]); err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(map[string]string{"state": "configured", "native_qualification": "unattested"})
	}
	if len(args) == 5 && args[0] == "context-cap" && args[1] == "--root" && args[3] == "--max" {
		return capContext(args[2], args[4], in, out)
	}
	if len(args) == 1 && args[0] == "caseos-check" {
		body, err := io.ReadAll(io.LimitReader(in, 4097))
		if err != nil || len(body) > 4096 {
			return errors.New("invalid bounded caseOS endpoint input")
		}
		var request struct{ Endpoint string }
		if json.Unmarshal(body, &request) != nil {
			return errors.New("invalid caseOS endpoint input")
		}
		if err = ProbeCaseOSEndpoint(context.Background(), request.Endpoint, nil); err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(map[string]string{"state": "adapter_observed", "check": "tls_mcp_initialize", "native_qualification": "unattested"})
	}
	if len(args) != 4 || args[0] != "codex-hook" || args[2] != "--root" {
		return errors.New("usage: codex-hook EVENT --root ROOT")
	}
	event, root := args[1], args[3]
	body, readErr := io.ReadAll(io.LimitReader(in, codexadapter.MaximumNativeInputBytes+1))
	input, err := codexadapter.ParseReader(bytes.NewReader(body))
	if readErr != nil {
		err = readErr
	}
	var native struct{ CWD string }
	if err == nil {
		err = json.Unmarshal(body, &native)
	}
	cwd := native.CWD
	if cwd == "" {
		cwd = root
	}
	if event == "PreToolUse" {
		decision := codexadapter.FailClosedDenial()
		if err == nil {
			decision = guard(root, cwd, input)
		}
		return json.NewEncoder(out).Encode(decision)
	}
	if err != nil {
		return errors.New("invalid bounded hook input")
	}
	switch event {
	case "SessionStart", "UserPromptSubmit":
		if event == "SessionStart" {
			migration := zipmigration.Run(root, zipmigration.Options{})
			if migration.State != "committed" && migration.State != "not_needed" {
				return json.NewEncoder(out).Encode(map[string]any{"continue": false, "stopReason": "Maestro migration is " + migration.State + ". Retained data remains available in the prior installation. Resolve migration before new writes."})
			}
		}
		return json.NewEncoder(out).Encode(map[string]any{"hookSpecificOutput": map[string]string{"hookEventName": event, "additionalContext": "You are Maestro for this professional workspace. Read AGENTS.md and load the maestro-operator skill. Use brain/accounts/.active to resolve case scope. Continue from explicit checkpoints; do not infer missing state. caseOS requires the caseos-connect skill and a current consented, allowlisted connection. Hooks are configured; this adapter invocation does not prove native qualification."}})
	case "PostToolUse", "Stop":
		semantic := lifecycle.PostActionObserve
		if event == "Stop" {
			semantic = lifecycle.StopFinalize
		}
		receipt, err := codexadapter.Receipt(semantic, input)
		if err != nil {
			return errors.New("invalid lifecycle metadata")
		}
		dataRoot := filepath.Join(root, "brain/.maestro")
		physical, err := resolveProspective(dataRoot)
		if err != nil {
			return errors.New("unsafe receipt root")
		}
		physicalRoot, err := filepath.EvalSymlinks(root)
		if err != nil || physical != filepath.Join(physicalRoot, "brain/.maestro") {
			return errors.New("aliased receipt root")
		}
		if _, err = lifecycle.Record(dataRoot, lifecycle.IdempotencyKey(root), receipt); err != nil {
			return errors.New("lifecycle receipt unavailable")
		}
		return json.NewEncoder(out).Encode(map[string]any{})
	default:
		return errors.New("unsupported Codex event")
	}
}

func guard(root, cwd string, input codexadapter.NativeInput) codexadapter.GuardOutput {
	deny := codexadapter.FailClosedDenial()
	decision, err := codexadapter.Guard(input)
	if err != nil {
		return deny
	}
	if decision.HookSpecificOutput != nil {
		return decision
	}
	var v map[string]any
	if err = json.Unmarshal(input.ToolInputJSON(), &v); err != nil {
		return deny
	}
	var paths []string
	switch input.ToolName {
	case "Write", "Edit", "MultiEdit":
		p, _ := v["file_path"].(string)
		paths = append(paths, p)
	case "NotebookEdit":
		p, _ := v["notebook_path"].(string)
		paths = append(paths, p)
	case "apply_patch":
		value, _ := v["command"].(string)
		if value == "" {
			value, _ = v["patch"].(string)
		}
		if !strings.HasPrefix(strings.TrimSpace(value), "*** Begin Patch") || !strings.HasSuffix(strings.TrimSpace(value), "*** End Patch") {
			return deny
		}
		for _, line := range strings.Split(value, "\n") {
			for _, prefix := range []string{"*** Add File: ", "*** Update File: ", "*** Delete File: ", "*** Move to: "} {
				if strings.HasPrefix(line, prefix) {
					paths = append(paths, strings.TrimPrefix(line, prefix))
				}
			}
		}
		if len(paths) == 0 {
			return deny
		}
	case "Agent", "spawn_agent":
		return codexadapter.GuardOutput{}
	default:
		return codexadapter.GuardOutput{}
	}
	for _, p := range paths {
		p = strings.ReplaceAll(p, "\\", "/")
		if p != "" && !filepath.IsAbs(p) {
			p = filepath.Join(cwd, filepath.FromSlash(p))
		}
		if !casePathAllowed(root, p) {
			return deny
		}
	}
	return codexadapter.GuardOutput{}
}

// resolveProspective resolves all existing ancestors, including symlink aliases
// for paths whose final file does not exist. Broken aliases deny.
func resolveProspective(p string) (string, error) {
	p, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	var suffix []string
	for {
		_, err = os.Lstat(p)
		if err == nil {
			r, e := filepath.EvalSymlinks(p)
			if e != nil {
				return "", e
			}
			for i := len(suffix) - 1; i >= 0; i-- {
				r = filepath.Join(r, suffix[i])
			}
			return filepath.Clean(r), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", err
		}
		suffix = append(suffix, filepath.Base(p))
		p = parent
	}
}

func casePathAllowed(root, p string) bool {
	if strings.TrimSpace(p) == "" || strings.ContainsAny(p, "\x00\r\n") {
		return false
	}
	p = strings.ReplaceAll(p, "\\", "/")
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, filepath.FromSlash(p))
	}
	target, err := resolveProspective(p)
	if err != nil {
		return false
	}
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	canonicalAccounts := filepath.Join(physicalRoot, "brain/accounts")
	accounts, err := resolveProspective(filepath.Join(root, "brain/accounts"))
	if err != nil {
		return false
	}
	if !strings.EqualFold(accounts, canonicalAccounts) {
		return false
	}
	// A lexical path under accounts may not escape through an alias. External
	// aliases into accounts still undergo physical case classification below.
	lexical, e := filepath.Abs(p)
	if e != nil {
		return false
	}
	lexicalRoot, e := filepath.Abs(root)
	if e != nil {
		return false
	}
	inside := func(base, path string) bool {
		b := strings.ToLower(filepath.Clean(base))
		q := strings.ToLower(filepath.Clean(path))
		return q == b || strings.HasPrefix(q, b+string(filepath.Separator))
	}
	if inside(filepath.Join(lexicalRoot, "brain/accounts"), lexical) && !inside(accounts, target) {
		return false
	}
	rel, err := filepath.Rel(accounts, target)
	if err != nil {
		return false
	}
	parts := strings.Split(strings.ToLower(filepath.ToSlash(rel)), "/")
	if len(parts) < 3 || parts[0] == ".." || parts[1] != "cases" {
		return true
	}
	targetID := parts[0] + "/" + parts[2]
	active, err := readMarker(filepath.Join(accounts, ".active"))
	if err != nil || active == "" {
		return false
	}
	if targetID == active {
		return true
	}
	pending, err := readMarker(filepath.Join(accounts, ".pending"))
	return err == nil && pending == targetID
}

func readMarker(p string) (string, error) {
	physical, err := filepath.EvalSymlinks(p)
	if err != nil || !strings.EqualFold(physical, filepath.Clean(p)) {
		return "", errors.New("aliased marker")
	}
	f, err := os.Open(p)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil || len(b) > 1024 {
		return "", errors.New("invalid marker")
	}
	s := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(string(b), "\uFEFF")))
	parts := strings.Split(s, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || strings.ContainsAny(s, " \t\r\n\\") || strings.HasPrefix(parts[0], ".") || strings.HasPrefix(parts[1], ".") {
		return "", errors.New("invalid marker")
	}
	return s, nil
}
