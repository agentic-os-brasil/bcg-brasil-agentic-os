// Package zipruntime implements the native ZIP helper without Python dependencies.
package zipruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/codexadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacepolicy"
	"io"
	"path/filepath"
	"strings"
)

// Run dispatches bounded native hook input. JSON denial uses exit zero, never
// continue:false, which is not a supported PreToolUse enforcement response.
func Run(args []string, in io.Reader, out io.Writer) error {
	return RunWithOptions(args, in, out, Options{})
}

// RunWithOptions shares canonical admission across every owner-state consumer.
func RunWithOptions(args []string, in io.Reader, out io.Writer, options Options) error {
	if len(args) >= 2 && args[len(args)-2] == "--user-home" {
		options.UserHome = args[len(args)-1]
		args = args[:len(args)-2]
	}
	var paths Paths
	for i := 1; i+1 < len(args); i++ {
		if args[i] == "--root" && args[0] != "bind-codex" {
			paths = ResolvePaths(args[i+1], options)
			if !paths.Writable {
				if paths.Diagnostic == "migration_required" && (args[0] == "daily-stop" || args[0] == "daily-dream-ack") {
					return json.NewEncoder(out).Encode(dailyReceipt{State: "migration_blocked", Failed: 1})
				}
				if paths.Diagnostic == "migration_required" && args[0] == "codex-hook" && args[1] == "Stop" {
					return json.NewEncoder(out).Encode(map[string]string{"systemMessage": "daily_recovery migration_blocked; checkpoints retained."})
				}
				if args[0] == "codex-hook" && args[1] == "PreToolUse" {
					return json.NewEncoder(out).Encode(codexadapter.FailClosedDenial())
				}
				if args[0] == "codex-hook" && (args[1] == "SessionStart" || args[1] == "UserPromptSubmit") {
					return json.NewEncoder(out).Encode(map[string]any{"continue": false, "stopReason": "<!-- maestro:state-unavailable --> Maestro owner state unavailable: " + paths.Diagnostic})
				}
				return errors.New("Maestro owner state unavailable: " + paths.Diagnostic)
			}
			break
		}
	}
	if len(args) == 3 && args[1] == "--root" {
		if args[0] == "daily-stop" {
			return dailyStop(paths.OwnerScopeRoot, out)
		}
		if args[0] == "daily-context" {
			return dailyContext(paths.OwnerScopeRoot, out)
		}
		if args[0] == "daily-dream-ack" {
			return dailyDreamAck(paths.OwnerScopeRoot, in, out)
		}
	}
	if len(args) == 3 && args[0] == "bind-codex" && args[1] == "--root" {
		if err := BindCodex(args[2]); err != nil {
			return err
		}
		return json.NewEncoder(out).Encode(map[string]string{"state": "configured", "native_qualification": "unattested"})
	}
	if len(args) == 5 && args[0] == "context-cap" && args[1] == "--root" && args[3] == "--max" {
		return capContext(paths.OwnerScopeRoot, args[4], in, out)
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
	event, root := args[1], paths.OwnerScopeRoot
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
		cwd = paths.ProductRoot
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
		context := "You are Maestro for this professional workspace. Read AGENTS.md and load the maestro-operator skill. Owner mode: " + paths.Mode + ". Owner brain: " + paths.BrainRoot + ". Resolve accounts/.active beneath that brain. Owner scope: " + paths.OwnerScopeRoot + ". Continue from explicit checkpoints; do not infer missing state. caseOS requires the caseos-connect skill and a current consented, allowlisted connection. Hooks are configured; this adapter invocation does not prove native qualification."
		if event == "SessionStart" {
			diagnostic, _ := dailyRecovery(root)
			if diagnostic != "" {
				context += "\n" + diagnostic
			}
			var packet bytes.Buffer
			if dailyContext(root, &packet) == nil {
				context += "\n" + packet.String()
			} else {
				context += "\nDaily continuity unavailable."
			}
		}
		return json.NewEncoder(out).Encode(map[string]any{"hookSpecificOutput": map[string]string{"hookEventName": event, "additionalContext": context}})
	case "PostToolUse", "Stop":
		semantic := lifecycle.PostActionObserve
		diagnostic := ""
		if event == "Stop" {
			semantic = lifecycle.StopFinalize
			var blocked bool
			diagnostic, blocked = dailyRecovery(root)
			if blocked {
				return json.NewEncoder(out).Encode(map[string]string{"systemMessage": diagnostic})
			}
		}
		receipt, err := codexadapter.Receipt(semantic, input)
		if err != nil {
			return errors.New("invalid lifecycle metadata")
		}
		dataRoot := paths.MetadataRoot
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
		if diagnostic != "" {
			return json.NewEncoder(out).Encode(map[string]string{"systemMessage": diagnostic})
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
		if !workspacepolicy.CasePathAllowed(root, p) {
			return deny
		}
	}
	return codexadapter.GuardOutput{}
}
