// Package cli exposes Maestro's narrow installed control plane. Ordinary
// professional work remains owned by the runtime agent and governed skills.
package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	baseprofile "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/profile"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/actionconfirmation"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/codexadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/nativeagentflow"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/portableactivation"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/profile"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionhook"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceaccess"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceprojection"
)

const (
	ExitOK                        = 0
	ExitFailure                   = 1
	ExitUsage                     = 2
	maximumScopedHookContextBytes = 8 << 10
)

var Version = "0.0.0-dev"

const publicUsage = "usage: bcgos <workspace|hook|version>"

const (
	noncanonicalExternalDenial    = "Maestro denied this external mutation because the request is outside the bounded canonical grammar. Nothing was changed. Use an explicit action and target, then retry."
	unavailableConfirmationDenial = "Maestro denied this external mutation because a user-bound confirmation challenge could not be evaluated. Nothing was changed. Retry from an identified native session."
)

func Run(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, publicUsage)
		return ExitUsage
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprintln(out, publicUsage)
		fmt.Fprintln(out, "workspace lifecycle: enroll, status, repair, remove, access")
		return ExitOK
	case "version":
		fmt.Fprintf(out, "bcgos %s\n", Version)
		return ExitOK
	case "workspace":
		return runWorkspace(args[1:], out, errOut)
	case "hook":
		return runHook(args[1:], in, out, errOut)
	default:
		fmt.Fprintf(errOut, "unknown command %q\n%s\n", args[0], publicUsage)
		return ExitUsage
	}
}

func runWorkspace(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos workspace <enroll|status|repair|remove|access> ...")
		return ExitUsage
	}
	if args[0] == "access" {
		return runWorkspaceAccess(args[1:], out, errOut)
	}
	operation := args[0]
	if operation != "enroll" && operation != "status" && operation != "repair" && operation != "remove" {
		fmt.Fprintln(errOut, "usage: bcgos workspace <enroll|status|repair|remove> --runtime claude|codex <repository-or-worktree>")
		return ExitUsage
	}
	flags := flag.NewFlagSet("workspace "+operation, flag.ContinueOnError)
	flags.SetOutput(errOut)
	runtimeName := flags.String("runtime", "", "claude or codex")
	managedRoot := flags.String("managed-root", "", "activated managed root")
	dataRoot := flags.String("data-root", "", "owner-private data root")
	executable := flags.String("executable", "", "installed CLI path")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 1 || (*runtimeName != "claude" && *runtimeName != "codex") {
		fmt.Fprintf(errOut, "usage: bcgos workspace %s --runtime claude|codex <repository-or-worktree>\n", operation)
		return ExitUsage
	}
	manager, err := resolveManager(*managedRoot, *dataRoot, *executable)
	if err != nil {
		return reportError(errOut, err)
	}
	target := flags.Arg(0)
	var status workspaceprojection.Status
	switch operation {
	case "enroll":
		status, err = manager.Enroll(background(), *runtimeName, target)
	case "status":
		status, err = manager.Status(background(), *runtimeName, target)
	case "repair":
		status, err = manager.Repair(background(), *runtimeName, target)
	case "remove":
		status, err = manager.Remove(background(), *runtimeName, target)
	}
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, status, errOut)
}

func runWorkspaceAccess(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos workspace access <grant|read|status|revoke> --runtime claude|codex --source PATH ...")
		return ExitUsage
	}
	operation := args[0]
	if operation != "grant" && operation != "read" && operation != "status" && operation != "revoke" {
		fmt.Fprintln(errOut, "usage: bcgos workspace access <grant|read|status|revoke> --runtime claude|codex --source PATH ...")
		return ExitUsage
	}
	flags := flag.NewFlagSet("workspace access "+operation, flag.ContinueOnError)
	flags.SetOutput(errOut)
	runtimeName := flags.String("runtime", "", "claude or codex")
	sourcePath := flags.String("source", "", "exact enrolled source repository or worktree")
	targetPath := flags.String("target", "", "exact enrolled target repository or worktree")
	purpose := flags.String("purpose", "", "closed access purpose")
	include := flags.String("include", "", "comma-separated context sources")
	ttl := flags.Duration("ttl", workspaceaccess.DefaultTTL, "grant lifetime")
	confirmed := flags.Bool("confirm", false, "explicit owner confirmation")
	grantID := flags.String("grant-id", "", "opaque grant id")
	managedRoot := flags.String("managed-root", "", "activated managed root")
	dataRoot := flags.String("data-root", "", "owner-private data root")
	executable := flags.String("executable", "", "installed CLI path")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || (*runtimeName != "claude" && *runtimeName != "codex") || strings.TrimSpace(*sourcePath) == "" {
		fmt.Fprintf(errOut, "usage: bcgos workspace access %s --runtime claude|codex --source PATH ...\n", operation)
		return ExitUsage
	}
	manager, err := resolveManager(*managedRoot, *dataRoot, *executable)
	if err != nil {
		return reportWorkspaceAccessError(errOut, operation, errors.New("installed Maestro activation could not be verified"))
	}
	sourceStatus, err := manager.Status(background(), *runtimeName, *sourcePath)
	if err != nil || sourceStatus.State != workspaceprojection.StateEnrolled {
		if err == nil {
			err = fmt.Errorf("source workspace is %s", sourceStatus.State)
		}
		return reportWorkspaceAccessError(errOut, operation, err)
	}
	source := workspaceaccess.Authority{WorkspaceID: sourceStatus.WorkspaceID, RepositoryID: sourceStatus.RepositoryID}
	identity, err := localWorkspaceAccessIdentity()
	if err != nil {
		return reportWorkspaceAccessError(errOut, operation, err)
	}
	store := workspaceaccess.Store{Root: manager.DataRoot}
	switch operation {
	case "grant":
		if strings.TrimSpace(*targetPath) == "" || strings.TrimSpace(*purpose) == "" || strings.TrimSpace(*include) == "" {
			fmt.Fprintln(errOut, "usage: bcgos workspace access grant --runtime claude|codex --source PATH --target PATH --purpose PURPOSE --include context,memory,continuity [--ttl 30m] --confirm")
			return ExitUsage
		}
		targetStatus, statusErr := manager.Status(background(), *runtimeName, *targetPath)
		if statusErr != nil || targetStatus.State != workspaceprojection.StateEnrolled {
			if statusErr == nil {
				statusErr = fmt.Errorf("target workspace is %s", targetStatus.State)
			}
			return reportWorkspaceAccessError(errOut, operation, statusErr)
		}
		status, grantErr := store.Grant(workspaceaccess.GrantRequest{
			Runtime: *runtimeName, Source: source,
			Target:  workspaceaccess.Authority{WorkspaceID: targetStatus.WorkspaceID, RepositoryID: targetStatus.RepositoryID},
			Purpose: *purpose, Sources: splitCommaList(*include), TTL: *ttl, Confirmed: *confirmed,
		}, identity)
		if grantErr != nil {
			return reportWorkspaceAccessError(errOut, operation, grantErr)
		}
		return writeJSON(out, status, errOut)
	case "status":
		if strings.TrimSpace(*grantID) == "" {
			fmt.Fprintln(errOut, "usage: bcgos workspace access status --runtime claude|codex --source PATH --grant-id ID")
			return ExitUsage
		}
		status, statusErr := store.Status(*runtimeName, source, *grantID, identity)
		if statusErr != nil {
			return reportWorkspaceAccessError(errOut, operation, statusErr)
		}
		return writeJSON(out, status, errOut)
	case "revoke":
		if strings.TrimSpace(*grantID) == "" {
			fmt.Fprintln(errOut, "usage: bcgos workspace access revoke --runtime claude|codex --source PATH --grant-id ID")
			return ExitUsage
		}
		status, revokeErr := store.Revoke(*runtimeName, source, *grantID, identity)
		if revokeErr != nil {
			return reportWorkspaceAccessError(errOut, operation, revokeErr)
		}
		return writeJSON(out, status, errOut)
	case "read":
		if strings.TrimSpace(*grantID) == "" {
			fmt.Fprintln(errOut, "usage: bcgos workspace access read --runtime claude|codex --source PATH --grant-id ID")
			return ExitUsage
		}
		result, readErr := store.Read(background(), *runtimeName, source, *grantID, identity, func(ctx context.Context, runtimeName, workspaceID string) (workspaceaccess.Authority, error) {
			enrollment, resolveErr := manager.ResolveEnrolledWorkspace(ctx, runtimeName, workspaceID)
			if resolveErr != nil {
				return workspaceaccess.Authority{}, resolveErr
			}
			return workspaceaccess.Authority{WorkspaceID: enrollment.WorkspaceID, RepositoryID: enrollment.RepositoryID}, nil
		})
		if readErr != nil {
			return reportWorkspaceAccessError(errOut, operation, readErr)
		}
		return writeJSON(out, result, errOut)
	}
	return ExitUsage
}

func splitCommaList(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func localWorkspaceAccessIdentity() (workspaceaccess.Identity, error) {
	current, err := user.Current()
	if err != nil {
		return workspaceaccess.Identity{}, fmt.Errorf("resolve authenticated local OS principal: %w", err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		return workspaceaccess.Identity{}, fmt.Errorf("resolve local device identity: %w", err)
	}
	principal := current.Uid + "\x00" + current.Username
	if strings.Trim(principal, "\x00") == "" || strings.TrimSpace(hostname) == "" {
		return workspaceaccess.Identity{}, errors.New("stable local principal and device identity are required")
	}
	return workspaceaccess.DeriveIdentity(principal, hostname), nil
}

func reportWorkspaceAccessError(errOut io.Writer, operation string, err error) int {
	reason := "the bounded request could not be verified"
	message := strings.ToLower(err.Error())
	switch {
	case errors.Is(err, workspaceaccess.ErrConfirmationRequired):
		reason = "explicit owner confirmation is required"
	case errors.Is(err, workspaceaccess.ErrExpired):
		reason = "the grant expired"
	case errors.Is(err, workspaceaccess.ErrRevoked):
		reason = "the grant was revoked"
	case strings.Contains(message, "target"):
		reason = "the target enrollment is unavailable or changed"
	case strings.Contains(message, "source workspace"):
		reason = "the source enrollment is unavailable or changed"
	case strings.Contains(message, "integrity"):
		reason = "grant integrity verification failed"
	case strings.Contains(message, "identity") || strings.Contains(message, "scope"):
		reason = "the grant identity or scope changed"
	case strings.Contains(message, "symlink"):
		reason = "a private context source is unsafe"
	case strings.Contains(message, "bounded") || strings.Contains(message, "8 kib"):
		reason = "a private context source exceeded its bound"
	case strings.Contains(message, "capacity") || strings.Contains(message, "inspection limit"):
		reason = "the bounded grant store requires maintenance"
	case strings.Contains(message, "ttl") || strings.Contains(message, "purpose") || strings.Contains(message, "registry") || strings.Contains(message, "invalid") || strings.Contains(message, "trailing"):
		reason = "the request is outside the closed workspace-access contract"
	}
	return reportError(errOut, fmt.Errorf("workspace access %s failed: %s; no repository or work content was changed; next safe command: bcgos workspace access status --runtime <claude|codex> --source <repository-or-worktree> --grant-id <id>", operation, reason))
}

func runHook(args []string, in io.Reader, out, errOut io.Writer) int {
	runtimeName, semanticEvent, remaining, err := parseHookRoute(args)
	if err != nil {
		fmt.Fprintln(errOut, "usage: bcgos hook <claude|codex> <event> --managed-root PATH --data-root PATH --workspace-root PATH")
		return ExitUsage
	}
	flags := flag.NewFlagSet("hook", flag.ContinueOnError)
	flags.SetOutput(errOut)
	adapterSource := flags.String("adapter-source", "", "managed adapter marker")
	orchestrationState := flags.String("orchestration-state", "", "workspace-local state marker")
	managedRoot := flags.String("managed-root", "", "activated managed root")
	dataRoot := flags.String("data-root", "", "owner-private data root")
	workspaceRoot := flags.String("workspace-root", "", "exact worktree root")
	executable := flags.String("executable", "", "installed CLI path")
	runtimeFlag := flags.String("runtime", runtimeName, "runtime compatibility marker")
	if err := flags.Parse(remaining); err != nil || flags.NArg() != 0 || *adapterSource != "maestro" || strings.TrimSpace(*orchestrationState) == "" || *runtimeFlag != runtimeName {
		return ExitUsage
	}
	manager, err := resolveManager(*managedRoot, *dataRoot, *executable)
	if err != nil {
		return reportError(errOut, err)
	}
	status, err := manager.Status(background(), runtimeName, *workspaceRoot)
	if err != nil || status.State != workspaceprojection.StateEnrolled {
		if err == nil {
			err = fmt.Errorf("workspace projection is %s", status.State)
		}
		if semanticEvent == "pre_action_guard" {
			return writeHookDenial(out, runtimeName, "workspace enrollment could not be verified", errOut)
		}
		return reportError(errOut, err)
	}
	var body []byte
	body, err = io.ReadAll(io.LimitReader(in, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		if semanticEvent == "pre_action_guard" {
			return writeHookDenial(out, runtimeName, "tool input exceeded the bounded guard contract", errOut)
		}
		return reportError(errOut, errors.New("hook input exceeded the bounded lifecycle contract"))
	}
	switch semanticEvent {
	case "session_start", "context_inject":
		var codexNative *codexadapter.NativeInput
		if semanticEvent == "context_inject" {
			var sessionID, prompt string
			if runtimeName == "claude" {
				native, parseErr := claudeadapter.Parse(body)
				if parseErr != nil {
					return reportError(errOut, parseErr)
				}
				sessionID, prompt = native.SessionID, native.Prompt
				if native.SessionID != "" {
					flow, flowErr := nativeagentflow.New(*dataRoot, status.WorkspaceID)
					if flowErr != nil {
						return reportError(errOut, flowErr)
					}
					if flowErr := flow.BeginTurn(native.SessionID); flowErr != nil {
						return reportError(errOut, fmt.Errorf("start Claude native-agent turn: %w", flowErr))
					}
				}
			} else {
				native, parseErr := codexadapter.ParseReader(strings.NewReader(string(body)))
				if parseErr != nil {
					return reportError(errOut, parseErr)
				}
				codexNative = &native
				sessionID, prompt = native.SessionID, native.Prompt
			}
			if strings.HasPrefix(strings.TrimSpace(prompt), "CONFIRM MAESTRO ") {
				actorID, actorErr := localConfirmedOwnerActor(*dataRoot)
				if actorErr != nil {
					return reportError(errOut, fmt.Errorf("resolve action-confirmation actor: %w", actorErr))
				}
				if _, confirmErr := confirmationStore(*dataRoot, status.WorkspaceID).Confirm(runtimeName, status.WorkspaceID, actorID, sessionID, prompt); confirmErr != nil {
					return reportError(errOut, fmt.Errorf("confirm external action: %w", confirmErr))
				}
			}
		} else if runtimeName == "codex" && strings.TrimSpace(string(body)) != "" {
			native, parseErr := codexadapter.ParseReader(strings.NewReader(string(body)))
			if parseErr == nil {
				codexNative = &native
			}
		}
		contextBody, err := composeScopedContext(*dataRoot, runtimeName, semanticEvent, *workspaceRoot, status)
		if err != nil {
			return reportError(errOut, err)
		}
		if codexNative != nil {
			event := lifecycle.SessionStart
			if semanticEvent == "context_inject" {
				event = lifecycle.ContextInject
			}
			if receipt, receiptErr := codexadapter.Receipt(event, *codexNative); receiptErr == nil {
				_, _ = lifecycle.Record(*dataRoot, status.WorkspaceID, receipt)
			}
		}
		return writeHookContext(out, runtimeName, semanticEvent, contextBody, errOut)
	case "pre_action_guard":
		var sessionID, toolName string
		var toolInput json.RawMessage
		var codexNative *codexadapter.NativeInput
		if runtimeName == "claude" {
			native, parseErr := claudeadapter.Parse(body)
			if parseErr != nil {
				return writeHookDenial(out, runtimeName, "tool input could not be verified by the Claude guard", errOut)
			}
			if reason, managed := claudeagents.GuardTool(native.AgentType, native.ToolName, native.ToolInputJSON(), native.CWD, *workspaceRoot); managed && reason != "" {
				return writeHookDenial(out, runtimeName, reason, errOut)
			}
			global, guardErr := claudeadapter.Guard(native)
			if guardErr != nil {
				return writeHookDenial(out, runtimeName, claudeadapter.ComplexRemovalDenial().HookSpecificOutput.PermissionDecisionReason, errOut)
			}
			if global.HookSpecificOutput != nil {
				return writeHookDenial(out, runtimeName, global.HookSpecificOutput.PermissionDecisionReason, errOut)
			}
			sessionID, toolName, toolInput = native.SessionID, native.ToolName, native.ToolInputJSON()
		} else {
			native, parseErr := codexadapter.ParseReader(strings.NewReader(string(body)))
			if parseErr != nil {
				return writeHookDenial(out, runtimeName, "tool input could not be verified by the Codex guard", errOut)
			}
			global, guardErr := codexadapter.Guard(native)
			if guardErr != nil {
				return writeHookDenial(out, runtimeName, codexadapter.ComplexRemovalDenial().HookSpecificOutput.PermissionDecisionReason, errOut)
			}
			if global.HookSpecificOutput != nil {
				return writeHookDenial(out, runtimeName, global.HookSpecificOutput.PermissionDecisionReason, errOut)
			}
			codexNative = &native
			sessionID, toolName, toolInput = native.SessionID, native.ToolName, native.ToolInputJSON()
		}
		protected, canonicalErr := actionconfirmation.Canonicalize(toolName, toolInput)
		if canonicalErr != nil {
			return writeHookDenial(out, runtimeName, noncanonicalExternalDenial, errOut)
		}
		if protected != nil {
			actorID, actorErr := localConfirmedOwnerActor(*dataRoot)
			if actorErr != nil {
				return writeHookDenial(out, runtimeName, unavailableConfirmationDenial, errOut)
			}
			result, authorizeErr := confirmationStore(*dataRoot, status.WorkspaceID).Authorize(actionconfirmation.Binding{
				Runtime: runtimeName, WorkspaceID: status.WorkspaceID, ActorID: actorID, SessionID: sessionID, Action: *protected,
			})
			if authorizeErr != nil {
				return writeHookDenial(out, runtimeName, unavailableConfirmationDenial, errOut)
			}
			if result.State != actionconfirmation.Authorized {
				return writeHookDenial(out, runtimeName, challengeDenial(result), errOut)
			}
		}
		if reason := unsafeToolPath(body, *workspaceRoot, *executable); reason != "" {
			if codexNative != nil {
				if receipt, receiptErr := codexadapter.Receipt(lifecycle.PreActionGuard, *codexNative); receiptErr == nil {
					_, _ = lifecycle.Record(*dataRoot, status.WorkspaceID, receipt)
				}
			}
			return writeHookDenial(out, runtimeName, reason, errOut)
		}
		fmt.Fprintln(out, "{}")
		return ExitOK
	case "post_action_observe", "stop_finalize":
		return handleFinalization(runtimeName, semanticEvent, body, *dataRoot, status.WorkspaceID, out, errOut)
	case "subagent_start", "subagent_stop":
		if runtimeName != "claude" {
			return reportError(errOut, errors.New("native subagent lifecycle is supported only for Claude"))
		}
		return handleClaudeSubagent(semanticEvent, body, *dataRoot, status.WorkspaceID, out, errOut)
	default:
		return reportError(errOut, fmt.Errorf("unsupported lifecycle event %q", semanticEvent))
	}
}

func handleFinalization(runtimeName, semanticEvent string, body []byte, dataRoot, workspaceID string, out, errOut io.Writer) int {
	event := lifecycle.PostActionObserve
	if semanticEvent == "stop_finalize" {
		event = lifecycle.StopFinalize
	}
	var receipt lifecycle.Receipt
	if runtimeName == "claude" {
		native, err := claudeadapter.Parse(body)
		if err != nil {
			if event == lifecycle.StopFinalize {
				return blockClaudeStop(out, "Maestro could not validate the Claude Stop payload. The session remains active; nothing was lost.", errOut)
			}
			return writeJSON(out, claudeadapter.FinalizationOutput{Continue: true}, errOut)
		}
		receipt, err = claudeadapter.Receipt(event, native)
		if err != nil {
			if event == lifecycle.StopFinalize {
				return blockClaudeStop(out, "Maestro could not validate the Claude Stop receipt. The session remains active; nothing was lost.", errOut)
			}
			return writeJSON(out, claudeadapter.FinalizationOutput{Continue: true}, errOut)
		}
		if event == lifecycle.StopFinalize {
			flow, flowErr := nativeagentflow.New(dataRoot, workspaceID)
			if flowErr != nil {
				return blockClaudeStop(out, "Maestro could not open the strategic completion state. The session remains active; nothing was lost.", errOut)
			}
			ready, reason, flowErr := flow.Finalize(native.SessionID)
			if flowErr != nil {
				return blockClaudeStop(out, "Maestro could not evaluate strategic completion. The session remains active; nothing was lost.", errOut)
			}
			if !ready && !native.StopHookActive {
				return writeJSON(out, claudeadapter.BlockStop(reason), errOut)
			}
		}
		if _, err := lifecycle.Record(dataRoot, workspaceID, receipt); err != nil {
			if event == lifecycle.StopFinalize {
				return blockClaudeStop(out, "Maestro could not persist the Claude Stop receipt. The session remains active; nothing was lost.", errOut)
			}
			return writeJSON(out, claudeadapter.FinalizationOutput{Continue: true}, errOut)
		}
		return writeJSON(out, claudeadapter.FinalizationOutput{Continue: true}, errOut)
	}
	native, err := codexadapter.ParseReader(strings.NewReader(string(body)))
	if err != nil {
		return writeJSON(out, codexadapter.FinalizationOutput{Continue: true}, errOut)
	}
	receipt, err = codexadapter.Receipt(event, native)
	if err != nil {
		return writeJSON(out, codexadapter.FinalizationOutput{Continue: true}, errOut)
	}
	if _, err := lifecycle.Record(dataRoot, workspaceID, receipt); err != nil {
		return writeJSON(out, codexadapter.FinalizationOutput{Continue: true}, errOut)
	}
	return writeJSON(out, codexadapter.FinalizationOutput{Continue: true}, errOut)
}

func blockClaudeStop(out io.Writer, reason string, errOut io.Writer) int {
	return writeJSON(out, claudeadapter.BlockStop(reason), errOut)
}

func handleClaudeSubagent(semanticEvent string, body []byte, dataRoot, workspaceID string, out, errOut io.Writer) int {
	native, err := claudeadapter.Parse(body)
	if err != nil {
		return reportError(errOut, fmt.Errorf("parse Claude subagent event: %w", err))
	}
	event := lifecycle.SubagentStart
	if semanticEvent == "subagent_stop" {
		event = lifecycle.SubagentStop
	}
	receipt, err := claudeadapter.Receipt(event, native)
	if err != nil {
		return reportError(errOut, fmt.Errorf("build Claude subagent receipt: %w", err))
	}
	flow, err := nativeagentflow.New(dataRoot, workspaceID)
	if err != nil {
		return reportError(errOut, err)
	}
	if claudeagents.Managed(native.AgentType) {
		if event == lifecycle.SubagentStart {
			err = flow.Start(native.SessionID, native.AgentID, native.AgentType)
		} else {
			err = flow.Stop(native.SessionID, native.AgentID, native.AgentType)
		}
		if err != nil {
			return reportError(errOut, fmt.Errorf("enforce Claude native-agent sequence: %w", err))
		}
	}
	if _, err := lifecycle.Record(dataRoot, workspaceID, receipt); err != nil {
		return reportError(errOut, fmt.Errorf("record Claude subagent receipt: %w", err))
	}
	if event == lifecycle.SubagentStart && claudeagents.Managed(native.AgentType) {
		return writeJSON(out, claudeadapter.ManagedSubagentStartContext(native.AgentType), errOut)
	}
	return writeJSON(out, claudeadapter.FinalizationOutput{Continue: true}, errOut)
}

func parseHookRoute(args []string) (string, string, []string, error) {
	if len(args) == 0 {
		return "", "", nil, errors.New("hook route is required")
	}
	if args[0] == "session-start" {
		return "codex", "session_start", args[1:], nil
	}
	if len(args) < 2 || (args[0] != "claude" && args[0] != "codex") {
		return "", "", nil, errors.New("hook runtime is invalid")
	}
	events := map[string]string{
		"session-start": "session_start", "context-injection": "context_inject",
		"pre-action-guard": "pre_action_guard", "post-action-receipt": "post_action_observe",
		"stop-finalization": "stop_finalize", "subagent-start": "subagent_start", "subagent-stop": "subagent_stop",
	}
	event, ok := events[args[1]]
	if !ok {
		return "", "", nil, errors.New("hook event is invalid")
	}
	return args[0], event, args[2:], nil
}

func resolveManager(managedRoot, dataRoot, executable string) (workspaceprojection.Manager, error) {
	var err error
	if strings.TrimSpace(executable) == "" {
		executable, err = os.Executable()
		if err != nil {
			return workspaceprojection.Manager{}, err
		}
	}
	executable, err = filepath.Abs(filepath.Clean(executable))
	if err != nil {
		return workspaceprojection.Manager{}, err
	}
	if strings.TrimSpace(managedRoot) == "" {
		binRoot := filepath.Dir(executable)
		if filepath.Base(binRoot) != "bin" {
			return workspaceprojection.Manager{}, errors.New("cannot derive managed root from installed CLI; pass --managed-root explicitly")
		}
		managedRoot = filepath.Dir(binRoot)
	}
	if strings.TrimSpace(dataRoot) == "" {
		dataRoot = filepath.Join(filepath.Dir(managedRoot), "data")
	}
	if _, err := portableactivation.Verify(portableactivation.Options{ManagedRoot: managedRoot, DataRoot: dataRoot, ExpectedVersion: Version}); err != nil {
		return workspaceprojection.Manager{}, fmt.Errorf("installed Maestro activation is not verified: %w", err)
	}
	return workspaceprojection.Manager{ManagedRoot: managedRoot, DataRoot: dataRoot, Executable: executable}, nil
}

func composeScopedContext(dataRoot, runtimeName, semanticEvent, workspaceRoot string, status workspaceprojection.Status) (string, error) {
	ownerStatus, err := ownerctx.Inspect(dataRoot)
	legacyOwnerContext := sessionctx.OwnerContext{}
	portableOwner := false
	if err != nil {
		legacyStatus, context, recognized, legacyErr := inspectPortableOwnerContext(dataRoot, semanticEvent == "session_start")
		if legacyErr != nil {
			return "", fmt.Errorf("inspect portable owner context: %w", legacyErr)
		}
		if !recognized {
			return "", fmt.Errorf("inspect private owner context: %w", err)
		}
		ownerStatus = legacyStatus
		legacyOwnerContext = context
		portableOwner = true
	}
	var ownerSnapshot *ownerctx.UserSelfSnapshot
	if semanticEvent == "session_start" && ownerStatus.Onboarding.State == "complete" && !portableOwner {
		projected, projectionErr := ownerctx.ProjectAnsweredSnapshot(dataRoot, sessionctx.SessionOwnerFacetIDs())
		if projectionErr != nil {
			return "", fmt.Errorf("project reviewed owner context: %w", projectionErr)
		}
		ownerSnapshot = &projected
	}
	profilePolicy, err := baseprofile.Policy()
	if err != nil {
		return "", fmt.Errorf("load interaction profile policy: %w", err)
	}
	profileState, err := (profile.Store{Root: dataRoot, Policy: profilePolicy}).Get()
	if err != nil {
		return "", fmt.Errorf("load interaction profile: %w", err)
	}
	memorySource := sessionctx.MemorySource{}
	if semanticEvent == "session_start" {
		memorySource, err = assembleSessionMemory(dataRoot, status.WorkspaceID)
		if err != nil {
			return "", err
		}
	}
	packet := sessionctx.Build(sessionctx.Sources{
		Profile: profileState,
		Workspace: workspace.Inspection{
			State: "ready", WorkspaceID: status.WorkspaceID,
		},
		Owner:         ownerStatus,
		OwnerSnapshot: ownerSnapshot,
		Memory:        memorySource,
	})
	if legacyOwnerContext.State != "" {
		packet.Owner.Context = legacyOwnerContext
	}
	nativeEvent := "UserPromptSubmit"
	if semanticEvent == "session_start" {
		nativeEvent = "SessionStart"
	}
	canonical := ""
	if runtimeName == "claude" {
		output, buildErr := sessionhook.BuildDirectClaudeEvent(packet, nativeEvent)
		if buildErr != nil {
			return "", fmt.Errorf("build canonical Claude context: %w", buildErr)
		}
		canonical = output.HookSpecificOutput.AdditionalContext
	} else {
		output, buildErr := sessionhook.BuildCodexEvent(packet, nativeEvent)
		if buildErr != nil {
			return "", fmt.Errorf("build canonical Codex context: %w", buildErr)
		}
		canonical = output.HookSpecificOutput.AdditionalContext
	}
	var builder strings.Builder
	builder.WriteString("Maestro direct workspace is active.\n")
	builder.WriteString("repository_id: " + status.RepositoryID + "\n")
	builder.WriteString("workspace_id: " + status.WorkspaceID + "\n")
	if runtimeName == "claude" {
		builder.WriteString("Host runtime: Claude Code.\n")
		builder.WriteString("Native frontend: maestro-hub.\n")
		builder.WriteString("Workspace boundary: exact enrolled worktree.\n")
		builder.WriteString("Governed method projection: integrity-checked .claude/skills pointers.\n")
	} else {
		builder.WriteString("Work only inside the exact opened worktree; do not cross repository or workspace boundaries.\n")
		builder.WriteString("Load governed methods only from integrity-checked .codex/skills pointers.\n")
	}
	builder.WriteString("\n" + canonical + "\n")
	for _, relative := range []string{
		filepath.Join("context", "session-context.md"),
		filepath.Join("memory", "session-context.md"),
		filepath.Join("continuity", "active.md"),
	} {
		scopedRelative := filepath.Join("workspaces", status.WorkspaceID, relative)
		body, err := readOptionalBounded(dataRoot, scopedRelative, 8<<10)
		if err != nil {
			return "", err
		}
		if body != "" {
			block := "\n" + body + "\n"
			if builder.Len()+len(block) <= maximumScopedHookContextBytes {
				builder.WriteString(block)
			}
		}
	}
	if builder.Len() > maximumScopedHookContextBytes {
		return "", errors.New("scoped hook context exceeds 8 KiB")
	}
	return builder.String(), nil
}

func assembleSessionMemory(dataRoot, workspaceID string) (sessionctx.MemorySource, error) {
	policy, err := basememory.Policy()
	if err != nil {
		return sessionctx.MemorySource{}, fmt.Errorf("load memory policy: %w", err)
	}
	runtimeConfig, err := basememory.Runtime()
	if err != nil {
		return sessionctx.MemorySource{}, fmt.Errorf("load memory runtime config: %w", err)
	}
	engine := memory.Engine{Root: dataRoot, Policy: policy, Budgets: runtimeConfig.ContextBudgets()}
	bundle, err := engine.AssembleContext(workspaceID)
	if err != nil {
		return sessionctx.MemorySource{State: "unavailable"}, nil
	}
	if len(bundle.Sections) == 0 {
		return sessionctx.MemorySource{State: "empty", Bundle: bundle}, nil
	}
	return sessionctx.MemorySource{State: "available", Bundle: bundle}, nil
}

func inspectPortableOwnerContext(dataRoot string, includeBodies bool) (ownerctx.Status, sessionctx.OwnerContext, bool, error) {
	registryBody, err := readOptionalBounded(dataRoot, filepath.Join("owner", "registry.json"), 16<<10)
	if err != nil || registryBody == "" {
		return ownerctx.Status{}, sessionctx.OwnerContext{}, false, err
	}
	registry, err := decodeJSONObject(registryBody)
	if err != nil || jsonNumber(registry["schema_version"]) != 1 {
		return ownerctx.Status{}, sessionctx.OwnerContext{}, false, nil
	}
	trees, ok := registry["trees"].(map[string]any)
	if !ok || strings.TrimSpace(jsonString(trees["self"])) != "owner/self/" {
		return ownerctx.Status{}, sessionctx.OwnerContext{}, true, errors.New("portable owner registry has an invalid SELF boundary")
	}
	initialized, _ := registry["initialized"].(bool)
	if !initialized {
		return ownerctx.Status{}, sessionctx.OwnerContext{State: "unavailable"}, true, nil
	}
	onboardingBody, err := readOptionalBounded(dataRoot, filepath.Join("profile", "onboarding.json"), 8<<10)
	if err != nil {
		return ownerctx.Status{}, sessionctx.OwnerContext{}, true, err
	}
	onboarding, err := decodeJSONObject(onboardingBody)
	if err != nil || jsonString(onboarding["status"]) != "complete" {
		return ownerctx.Status{}, sessionctx.OwnerContext{State: "unavailable"}, true, nil
	}
	track := jsonString(onboarding["track"])
	if track != ownerctx.OnboardingTrackQuick && track != ownerctx.OnboardingTrackComplete {
		track = ownerctx.OnboardingTrackQuick
	}
	total := ownerctx.SelfFacetCount()
	ownerStatus := ownerctx.Status{
		Initialized: true,
		Onboarding:  ownerctx.OnboardingStatus{State: "complete", Track: track},
		SelfIndex:   ownerctx.Pointer{Path: "owner/self/README.md", Available: true, State: "available"},
		Expansion:   ownerctx.ExpansionStatus{State: "current", Total: total, Current: total},
		OpenTasks:   ownerctx.TaskStatus{State: "unavailable"},
	}
	if !includeBodies {
		return ownerStatus, sessionctx.OwnerContext{State: "unavailable"}, true, nil
	}

	identityBody, err := readOptionalBounded(dataRoot, filepath.Join("profile", "identity.json"), 8<<10)
	if err != nil {
		return ownerctx.Status{}, sessionctx.OwnerContext{}, true, err
	}
	identity, err := decodeJSONObject(identityBody)
	if err != nil {
		return ownerctx.Status{}, sessionctx.OwnerContext{}, true, errors.New("portable owner identity is invalid")
	}
	name := boundedPortableScalar(jsonString(identity["name"]), 256)
	if name == "" {
		name = boundedPortableScalar(jsonString(identity["display_name"]), 256)
	}
	role := boundedPortableScalar(jsonString(identity["role"]), 1024)
	sections := make(map[string]string)
	if name != "" {
		sections["owner-identity"] = "# Owner identity\n\n## Current\n\n" + name
	}
	for _, id := range sessionctx.SessionOwnerFacetIDs() {
		body, readErr := readOptionalBounded(dataRoot, filepath.Join("owner", "self", id+".md"), 12<<10)
		if readErr != nil {
			return ownerctx.Status{}, sessionctx.OwnerContext{}, true, readErr
		}
		if portableFacetAnswered(body) {
			sections[id] = strings.TrimSpace(body)
		}
	}
	if _, exists := sections["professional-role"]; !exists && role != "" {
		sections["professional-role"] = "# Professional role\n\n## Current\n\n" + role
	}
	context := sessionctx.OwnerContext{State: "available"}
	for _, id := range sessionctx.SessionOwnerFacetIDs() {
		if body := sections[id]; body != "" {
			context.Sections = append(context.Sections, sessionctx.OwnerContextSection{Facet: id, Content: body})
		}
	}
	if len(context.Sections) == 0 {
		context.State = "unavailable"
	}
	return ownerStatus, context, true, nil
}

func decodeJSONObject(body string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	var value map[string]any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("JSON contains multiple values")
	}
	return value, nil
}

func jsonNumber(value any) int64 {
	number, ok := value.(json.Number)
	if !ok {
		return 0
	}
	parsed, _ := number.Int64()
	return parsed
}

func jsonString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func boundedPortableScalar(value string, maximum int) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > maximum || strings.ContainsAny(value, "\r\n\x00") {
		return ""
	}
	return value
}

func portableFacetAnswered(body string) bool {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return false
	}
	normalized := strings.ToLower(trimmed)
	return !strings.Contains(normalized, "não preenchido") &&
		!strings.Contains(normalized, "nao preenchido") &&
		!strings.Contains(normalized, "use /maestro-onboarding") &&
		!strings.Contains(normalized, "registre como o owner") &&
		!strings.Contains(normalized, "descreva responsabilidades") &&
		!strings.Contains(normalized, "descreva como prefere") &&
		!strings.Contains(normalized, "descreva como voce") &&
		!strings.Contains(normalized, "registre preferencias") &&
		!strings.Contains(normalized, "descreva o impacto") &&
		!strings.Contains(normalized, "descreva o que precisa") &&
		!strings.Contains(normalized, "registre principios") &&
		!strings.Contains(normalized, "registre limites")
}

func readOptionalBounded(root, relative string, limit int64) (string, error) {
	if lexicalTraversal(relative) || filepath.IsAbs(relative) {
		return "", errors.New("workspace-scoped context path is unsafe")
	}
	current := root
	for _, component := range strings.Split(filepath.Clean(relative), string(filepath.Separator)) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		if err != nil {
			return "", err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("workspace-scoped context path crosses a symlink")
		}
	}
	path := filepath.Join(root, relative)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > limit {
		return "", errors.New("workspace-scoped context file is unsafe or exceeds its bound")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

func unsafeToolPath(body []byte, workspaceRoot, installedExecutable string) string {
	if len(strings.TrimSpace(string(body))) == 0 {
		return "tool input is required"
	}
	var payload any
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return "tool input is malformed"
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return "tool input contains trailing or malformed content"
	}
	paths := []string{}
	commands := []string{}
	collectGuardFields(payload, "", &paths, &commands)
	for _, candidate := range paths {
		if unresolvedShellExpansion(candidate) {
			return "tool path contains an unresolved shell or environment expansion"
		}
		if lexicalTraversal(candidate) {
			return "tool path traversal is outside the enrolled worktree"
		}
		resolved := candidate
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(workspaceRoot, resolved)
		}
		resolved, _ = filepath.Abs(filepath.Clean(resolved))
		if !inside(workspaceRoot, resolved) || symlinkEscape(workspaceRoot, resolved) {
			return "tool path is outside the enrolled worktree or crosses a symlink"
		}
	}
	for _, command := range commands {
		if matched, reason := governedWorkspaceAccessCommand(command, installedExecutable, workspaceRoot); matched {
			if reason != "" {
				return reason
			}
			continue
		}
		if reason := unsafeShellCommand(command, workspaceRoot); reason != "" {
			return reason
		}
	}
	return ""
}

func governedWorkspaceAccessCommand(command, installedExecutable, workspaceRoot string) (bool, string) {
	commands, err := shellCommandWords(command)
	if err != nil || len(commands) == 0 || len(commands[0]) == 0 {
		return false, ""
	}
	expected, err := filepath.Abs(filepath.Clean(installedExecutable))
	if err != nil {
		return false, ""
	}
	actual := strings.Trim(commands[0][0], "\"'")
	if !filepath.IsAbs(actual) {
		return false, ""
	}
	actual, err = filepath.Abs(filepath.Clean(actual))
	if err != nil || actual != expected {
		return false, ""
	}
	if len(commands) != 1 {
		return true, "governed workspace access must be one exact simple command"
	}
	words := commands[0]
	if len(words) < 4 || words[1] != "workspace" || words[2] != "access" {
		return false, ""
	}
	operation := words[3]
	if operation != "grant" && operation != "read" && operation != "status" && operation != "revoke" {
		return true, "governed workspace access operation is invalid"
	}
	values := map[string]string{}
	confirmed := false
	for index := 4; index < len(words); index++ {
		name := words[index]
		if name == "--confirm" {
			if confirmed {
				return true, "governed workspace access contains a duplicate flag"
			}
			confirmed = true
			continue
		}
		if name != "--runtime" && name != "--source" && name != "--target" && name != "--purpose" && name != "--include" && name != "--ttl" && name != "--grant-id" {
			return true, "governed workspace access contains an unsupported flag"
		}
		if index+1 >= len(words) || strings.HasPrefix(words[index+1], "--") || values[name] != "" {
			return true, "governed workspace access contains a missing or duplicate flag value"
		}
		values[name] = words[index+1]
		index++
	}
	if values["--runtime"] != "claude" && values["--runtime"] != "codex" {
		return true, "governed workspace access runtime is invalid"
	}
	source, err := filepath.Abs(filepath.Clean(values["--source"]))
	expectedSource, sourceErr := filepath.Abs(filepath.Clean(workspaceRoot))
	if err != nil || sourceErr != nil || source != expectedSource {
		return true, "governed workspace access source must be the exact enrolled worktree"
	}
	if operation == "grant" {
		if !confirmed || !filepath.IsAbs(values["--target"]) || values["--purpose"] == "" || values["--include"] == "" {
			return true, "governed workspace access grant is incomplete"
		}
		if values["--purpose"] != workspaceaccess.PurposeReferenceContext && values["--purpose"] != workspaceaccess.PurposeCompareImplementation && values["--purpose"] != workspaceaccess.PurposeReuseLearning && values["--purpose"] != workspaceaccess.PurposeDependencyCoordination {
			return true, "governed workspace access purpose is invalid"
		}
		for _, sourceName := range splitCommaList(values["--include"]) {
			if sourceName != workspaceaccess.SourceContext && sourceName != workspaceaccess.SourceMemory && sourceName != workspaceaccess.SourceContinuity {
				return true, "governed workspace access source selection is invalid"
			}
		}
		if len(splitCommaList(values["--include"])) == 0 {
			return true, "governed workspace access source selection is empty"
		}
		if values["--grant-id"] != "" {
			return true, "governed workspace access grant contains an unrelated grant id"
		}
		if values["--ttl"] != "" {
			ttl, parseErr := time.ParseDuration(values["--ttl"])
			if parseErr != nil || ttl < workspaceaccess.MinimumTTL || ttl > workspaceaccess.MaximumTTL {
				return true, "governed workspace access TTL is invalid"
			}
		}
		return true, ""
	}
	if confirmed || values["--target"] != "" || values["--purpose"] != "" || values["--include"] != "" || values["--ttl"] != "" || len(values["--grant-id"]) != 32 {
		return true, "governed workspace access command is outside its read/status/revoke grammar"
	}
	if _, err := hex.DecodeString(values["--grant-id"]); err != nil || strings.ToLower(values["--grant-id"]) != values["--grant-id"] {
		return true, "governed workspace access grant id is invalid"
	}
	return true, ""
}

func collectGuardFields(value any, key string, paths, commands *[]string) {
	switch typed := value.(type) {
	case map[string]any:
		for childKey, child := range typed {
			collectGuardFields(child, childKey, paths, commands)
		}
	case []any:
		for _, child := range typed {
			collectGuardFields(child, key, paths, commands)
		}
	case string:
		switch strings.ToLower(key) {
		case "path", "file_path", "filepath", "cwd", "directory", "workdir", "working_directory":
			*paths = append(*paths, typed)
		case "command", "cmd", "script":
			*commands = append(*commands, typed)
		}
	}
}

func unsafeShellCommand(command, workspaceRoot string) string {
	normalized := strings.Join(strings.Fields(strings.ToLower(command)), " ")
	if dangerousGitMutation(command) {
		return "dangerous shell or Git mutation is not authorized by the workspace projection"
	}
	if opaqueExecutionWrapper(command) {
		return "shell execution wrapper is outside the bounded workspace guard grammar"
	}
	for _, forbidden := range []string{
		"rm -rf", "rm -fr", "rm -r -f", "remove-item -recurse", "rmdir /s", "sudo ", "doas ",
	} {
		if strings.Contains(normalized, forbidden) {
			return "dangerous shell or Git mutation is not authorized by the workspace projection"
		}
	}
	segments := strings.FieldsFunc(command, func(character rune) bool {
		switch character {
		case ';', '|', '&', '\n', '\r':
			return true
		default:
			return false
		}
	})
	for _, segment := range segments {
		fields := strings.Fields(segment)
		if len(fields) == 0 {
			continue
		}
		executableIndex := 0
		if strings.EqualFold(strings.Trim(fields[0], "\"'"), "env") {
			executableIndex++
			for executableIndex < len(fields) && strings.Contains(fields[executableIndex], "=") {
				executableIndex++
			}
		}
		for index, raw := range fields {
			if index == executableIndex {
				continue
			}
			candidate := strings.Trim(raw, "\"'(){}[],")
			candidate = strings.TrimLeft(candidate, "<>")
			if _, value, found := strings.Cut(candidate, "="); found {
				candidate = value
			}
			if candidate == "" || strings.HasPrefix(candidate, "-") {
				continue
			}
			if unresolvedShellExpansion(candidate) {
				return "shell command path contains an unresolved environment expansion"
			}
			if lexicalTraversal(candidate) {
				return "shell command path traversal is outside the enrolled worktree"
			}
			looksLikePath := filepath.IsAbs(candidate) || strings.ContainsAny(candidate, `/\`) || strings.HasPrefix(candidate, ".")
			if !looksLikePath {
				continue
			}
			resolved := candidate
			if !filepath.IsAbs(resolved) {
				resolved = filepath.Join(workspaceRoot, resolved)
			}
			resolved, _ = filepath.Abs(filepath.Clean(resolved))
			if !inside(workspaceRoot, resolved) || symlinkEscape(workspaceRoot, resolved) {
				return "shell command path is outside the enrolled worktree or crosses a symlink"
			}
		}
	}
	return ""
}

func unresolvedShellExpansion(value string) bool {
	trimmed := strings.TrimSpace(strings.Trim(value, "\"'"))
	if trimmed == "" {
		return false
	}
	return strings.ContainsAny(trimmed, "$`") || strings.HasPrefix(trimmed, "~") ||
		(strings.Contains(trimmed, "%") && strings.Count(trimmed, "%") >= 2)
}

func opaqueExecutionWrapper(command string) bool {
	commands, err := shellCommandWords(command)
	if err != nil {
		return false
	}
	for _, words := range commands {
		index := 0
		for index < len(words) && strings.Contains(words[index], "=") && !strings.HasPrefix(words[index], "-") {
			index++
		}
		for index < len(words) {
			name := strings.ToLower(filepath.Base(strings.Trim(words[index], "\"'")))
			if name == "env" || name == "command" || name == "builtin" || name == "exec" {
				index++
				continue
			}
			break
		}
		if index >= len(words) {
			continue
		}
		executable := strings.ToLower(filepath.Base(strings.Trim(words[index], "\"'")))
		arguments := words[index+1:]
		switch executable {
		case "source", ".":
			return true
		case "python", "python2", "python3", "pypy", "pypy3", "perl", "ruby", "node", "deno", "pwsh", "powershell", "powershell.exe":
			if len(arguments) == 0 || len(arguments) > 1 || (arguments[0] != "--version" && arguments[0] != "-V" && arguments[0] != "--help" && arguments[0] != "-h") {
				return true
			}
		case "sh", "bash", "zsh", "dash":
			if len(arguments) > 0 {
				return true
			}
		case "make", "gmake", "xargs":
			return len(arguments) > 0
		case "npm", "pnpm", "yarn", "bun", "cargo", "go":
			for _, argument := range arguments {
				if argument == "run" || argument == "exec" {
					return true
				}
			}
		case "find":
			for _, argument := range arguments {
				if argument == "-exec" || argument == "-execdir" || argument == "-ok" || argument == "-okdir" {
					return true
				}
			}
		}
	}
	return false
}

func dangerousGitMutation(command string) bool {
	commands, err := shellCommandWords(command)
	if err != nil {
		// An opaque shell expression mentioning Git cannot prove that the
		// destructive subcommand is absent, so the direct-worktree guard fails
		// closed and asks the runtime to emit a simple command instead.
		return strings.Contains(strings.ToLower(command), "git")
	}
	for _, words := range commands {
		if dangerousGitWords(words, 0) {
			return true
		}
	}
	return false
}

func dangerousGitWords(words []string, depth int) bool {
	if len(words) == 0 || depth > 4 {
		return depth > 4
	}
	index := 0
	for index < len(words) && strings.Contains(words[index], "=") && !strings.HasPrefix(words[index], "-") {
		index++
	}
	for index < len(words) {
		name := strings.ToLower(filepath.Base(strings.Trim(words[index], "\"'")))
		switch name {
		case "env":
			index++
			for index < len(words) && (strings.Contains(words[index], "=") || strings.HasPrefix(words[index], "-")) {
				index++
			}
			continue
		case "command", "builtin", "exec":
			index++
			continue
		}
		break
	}
	if index >= len(words) {
		return false
	}
	executable := strings.ToLower(filepath.Base(strings.Trim(words[index], "\"'")))
	arguments := words[index+1:]
	if strings.ContainsAny(executable, "$`(){}") {
		return true
	}
	if executable == "git" || executable == "git.exe" {
		return dangerousGitArguments(arguments, nil, depth)
	}
	if executable == "eval" {
		// eval may synthesize both the executable and its arguments from shell
		// variables or substitutions. The bounded guard cannot prove that an
		// apparently harmless expression will not become destructive Git, so
		// this execution wrapper is denied fail-closed.
		return true
	}
	if executable == "sh" || executable == "bash" || executable == "zsh" || executable == "dash" {
		for argument := 0; argument+1 < len(arguments); argument++ {
			if arguments[argument] == "-c" {
				nested, err := shellCommandWords(arguments[argument+1])
				if err != nil {
					return strings.Contains(strings.ToLower(arguments[argument+1]), "git")
				}
				for _, nestedWords := range nested {
					if dangerousGitWords(nestedWords, depth+1) {
						return true
					}
				}
			}
		}
	}
	// Wrappers such as xargs, find -exec, nice or timeout can move a literal
	// Git executable away from argv[0] and supply additional dynamic arguments.
	// Without a wrapper-specific proof those invocations are opaque, so deny
	// them rather than mistaking Git for inert data.
	for _, argument := range arguments {
		name := strings.ToLower(filepath.Base(strings.Trim(argument, "\"'(){}[],")))
		if name == "git" || name == "git.exe" {
			return true
		}
	}
	return false
}

func dangerousGitArguments(arguments []string, inheritedAliases map[string]string, depth int) bool {
	if depth > 8 {
		return true
	}
	aliases := make(map[string]string, len(inheritedAliases))
	for name, expansion := range inheritedAliases {
		aliases[name] = expansion
	}
	index := 0
	for index < len(arguments) {
		rawArgument := arguments[index]
		argument := strings.ToLower(rawArgument)
		if rawArgument == "--" {
			index++
			break
		}
		if !strings.HasPrefix(rawArgument, "-") {
			break
		}
		if rawArgument == "-c" {
			if index+1 >= len(arguments) {
				return true
			}
			if dangerousGitConfigOverride(arguments[index+1], aliases) {
				return true
			}
			index += 2
			continue
		}
		switch rawArgument {
		case "-C":
			if index+1 >= len(arguments) {
				return true
			}
			index += 2
			continue
		}
		switch argument {
		case "--git-dir", "--work-tree", "--namespace", "--exec-path":
			if index+1 >= len(arguments) {
				return true
			}
			index += 2
			continue
		case "--config-env":
			if index+1 >= len(arguments) || strings.HasPrefix(strings.ToLower(arguments[index+1]), "alias.") {
				return true
			}
			index += 2
			continue
		}
		if strings.HasPrefix(argument, "--git-dir=") || strings.HasPrefix(argument, "--work-tree=") ||
			strings.HasPrefix(argument, "--namespace=") ||
			strings.HasPrefix(argument, "--exec-path=") {
			index++
			continue
		}
		if strings.HasPrefix(argument, "--config-env=") {
			if strings.HasPrefix(strings.TrimPrefix(argument, "--config-env="), "alias.") {
				return true
			}
			index++
			continue
		}
		index++
	}
	if index >= len(arguments) {
		return false
	}
	subcommand := strings.ToLower(arguments[index])
	subarguments := arguments[index+1:]
	if expansion, ok := aliases[subcommand]; ok {
		return dangerousGitAlias(expansion, subarguments, aliases, depth+1)
	}
	switch subcommand {
	case "reset":
		return hasAnyGitOption(subarguments, "--hard", "--merge", "--keep")
	case "clean":
		for _, argument := range subarguments {
			lower := strings.ToLower(argument)
			if lower == "--force" || (strings.HasPrefix(lower, "-") && !strings.HasPrefix(lower, "--") && strings.Contains(strings.TrimPrefix(lower, "-"), "f")) {
				return true
			}
		}
	case "branch":
		return hasAnyGitOption(subarguments, "-d", "-D", "--delete", "-f", "--force", "-M", "-C")
	case "restore", "rm":
		// restore changes tracked worktree/index state and rm removes tracked
		// paths. Their effect cannot be distinguished from discarded work by
		// this bounded hook payload, so require an explicit path outside the
		// native tool call instead of silently allowing them.
		return true
	case "checkout":
		// checkout is intrinsically ambiguous: the same positional token can be
		// a branch or a pathspec that discards unstaged work. Agents can use the
		// unambiguous switch command for safe branch changes.
		return true
	case "switch":
		return hasAnyGitOption(subarguments, "-f", "--force", "--discard-changes", "-C", "--force-create", "--orphan")
	case "stash":
		return firstGitAction(subarguments, "push") == "drop" ||
			firstGitAction(subarguments, "push") == "clear" ||
			firstGitAction(subarguments, "push") == "pop"
	case "reflog":
		action := firstGitAction(subarguments, "show")
		return action == "delete" || action == "expire"
	case "tag":
		return hasAnyGitOption(subarguments, "-d", "--delete", "-f", "--force")
	case "worktree":
		for _, argument := range subarguments {
			if strings.HasPrefix(argument, "-") {
				continue
			}
			action := strings.ToLower(argument)
			return action == "remove" || action == "move" || action == "prune"
		}
	}
	// Git resolves an unknown subcommand through configured aliases or a
	// git-<name> executable on PATH. Neither authority is bounded by this hook,
	// so only known built-ins may fall through as non-destructive.
	return !knownGitSubcommands[subcommand]
}

var knownGitSubcommands = map[string]bool{
	"add": true, "am": true, "apply": true, "archive": true, "bisect": true,
	"blame": true, "bundle": true, "cat-file": true, "check-attr": true,
	"check-ignore": true, "check-mailmap": true, "check-ref-format": true,
	"checkout": true, "cherry-pick": true, "clone": true, "commit": true,
	"config": true, "count-objects": true, "describe": true, "diagnose": true,
	"diff": true, "difftool": true, "fetch": true, "for-each-ref": true,
	"format-patch": true, "fsck": true, "gc": true, "grep": true,
	"hash-object": true, "help": true, "init": true, "log": true,
	"ls-files": true, "ls-remote": true, "ls-tree": true, "maintenance": true,
	"merge": true, "merge-base": true, "merge-tree": true, "mergetool": true,
	"mv": true, "notes": true, "pull": true, "push": true,
	"range-diff": true, "rebase": true, "reflog": true, "remote": true,
	"repack": true, "replace": true, "request-pull": true, "rerere": true,
	"restore": true, "revert": true, "rev-list": true, "rev-parse": true,
	"rm": true, "shortlog": true, "show": true, "show-branch": true,
	"sparse-checkout": true, "stash": true, "status": true, "submodule": true,
	"switch": true, "tag": true, "verify-commit": true, "verify-pack": true,
	"verify-tag": true, "version": true, "whatchanged": true,
}

func dangerousGitConfigOverride(value string, aliases map[string]string) bool {
	key, expansion, found := strings.Cut(value, "=")
	if !found {
		return false
	}
	key = strings.ToLower(strings.TrimSpace(key))
	if !strings.HasPrefix(key, "alias.") {
		return false
	}
	name := strings.TrimPrefix(key, "alias.")
	if name == "" || strings.ContainsAny(name, " \t\r\n") {
		return true
	}
	aliases[name] = strings.TrimSpace(expansion)
	return false
}

func dangerousGitAlias(expansion string, invocationArguments []string, aliases map[string]string, depth int) bool {
	expansion = strings.TrimSpace(expansion)
	if expansion == "" || strings.HasPrefix(expansion, "!") {
		// Shell aliases can execute arbitrary programs and expand environment
		// state, so their effect is not statically bounded by this parser.
		return true
	}
	commands, err := shellCommandWords(expansion)
	if err != nil || len(commands) != 1 || len(commands[0]) == 0 {
		return true
	}
	arguments := append(append([]string{}, commands[0]...), invocationArguments...)
	return dangerousGitArguments(arguments, aliases, depth)
}

func hasGitOption(arguments []string, option string) bool {
	for _, argument := range arguments {
		if argument == option || strings.HasPrefix(argument, option+"=") {
			return true
		}
		if len(option) == 2 && option[0] == '-' && option[1] != '-' &&
			strings.HasPrefix(argument, "-") && !strings.HasPrefix(argument, "--") &&
			strings.ContainsRune(strings.TrimPrefix(argument, "-"), rune(option[1])) {
			return true
		}
	}
	return false
}

func hasAnyGitOption(arguments []string, options ...string) bool {
	for _, option := range options {
		if hasGitOption(arguments, option) {
			return true
		}
	}
	return false
}

func firstGitAction(arguments []string, fallback string) string {
	for _, argument := range arguments {
		if argument == "--" {
			continue
		}
		if !strings.HasPrefix(argument, "-") {
			return strings.ToLower(argument)
		}
	}
	return fallback
}

func shellCommandWords(command string) ([][]string, error) {
	var commands [][]string
	var words []string
	var word strings.Builder
	var quote rune
	escaped := false
	flushWord := func() {
		if word.Len() > 0 {
			words = append(words, word.String())
			word.Reset()
		}
	}
	flushCommand := func() {
		flushWord()
		if len(words) > 0 {
			commands = append(commands, words)
			words = nil
		}
	}
	for _, character := range command {
		if escaped {
			word.WriteRune(character)
			escaped = false
			continue
		}
		if quote != 0 {
			if character == quote {
				quote = 0
				continue
			}
			if character == '\\' && quote == '"' {
				escaped = true
				continue
			}
			word.WriteRune(character)
			continue
		}
		switch character {
		case '\\':
			escaped = true
		case '\'', '"':
			quote = character
		case ' ', '\t':
			flushWord()
		case ';', '|', '&', '\n', '\r':
			flushCommand()
		case '`':
			return nil, errors.New("shell command substitution is outside the bounded grammar")
		default:
			word.WriteRune(character)
		}
	}
	if escaped || quote != 0 {
		return nil, errors.New("shell command has incomplete quoting")
	}
	flushCommand()
	return commands, nil
}

func lexicalTraversal(path string) bool {
	path = strings.ReplaceAll(path, "\\", "/")
	for _, part := range strings.Split(path, "/") {
		if part == ".." {
			return true
		}
	}
	return false
}

func inside(root, path string) bool {
	root, _ = filepath.Abs(filepath.Clean(root))
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func symlinkEscape(root, path string) bool {
	current := path
	for {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				real, evalErr := filepath.EvalSymlinks(current)
				return evalErr != nil || !inside(root, real)
			}
			return false
		}
		if !errors.Is(err, os.ErrNotExist) {
			return true
		}
		parent := filepath.Dir(current)
		if parent == current || !inside(root, parent) {
			return true
		}
		current = parent
	}
}

func writeHookContext(out io.Writer, runtimeName, semanticEvent, body string, errOut io.Writer) int {
	nativeEvent := "UserPromptSubmit"
	if semanticEvent == "session_start" {
		nativeEvent = "SessionStart"
	}
	payload := map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": nativeEvent, "additionalContext": body}}
	if runtimeName != "codex" {
		payload["runtime"] = runtimeName
		payload["semantic_event"] = semanticEvent
	}
	return writeJSON(out, payload, errOut)
}

func writeHookDenial(out io.Writer, runtimeName, reason string, errOut io.Writer) int {
	payload := map[string]any{"hookSpecificOutput": map[string]any{"hookEventName": "PreToolUse", "permissionDecision": "deny", "permissionDecisionReason": reason}}
	if runtimeName != "codex" {
		payload["runtime"] = runtimeName
		payload["semantic_event"] = "pre_action_guard"
	}
	return writeJSON(out, payload, errOut)
}

func confirmationStore(root, workspaceID string) actionconfirmation.Store {
	return actionconfirmation.Store{Root: filepath.Join(root, "runtime", "action-confirmation", workspaceID)}
}

func localConfirmedOwnerActor(root string) (string, error) {
	profile, err := agentidentity.Load(root)
	if err != nil {
		return "", fmt.Errorf("load confirmed owner enrollment: %w", err)
	}
	current, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("resolve authenticated local OS principal: %w", err)
	}
	if strings.TrimSpace(current.Uid) == "" && strings.TrimSpace(current.Username) == "" {
		return "", errors.New("authenticated local OS principal has no stable identifier")
	}
	sum := sha256.Sum256([]byte("bcgos-local-principal-v1\x00" + current.Uid + "\x00" + current.Username))
	return fmt.Sprintf("%s@actor-local-%x", profile.OwnerID, sum[:16]), nil
}

func challengeDenial(result actionconfirmation.Result) string {
	return fmt.Sprintf("Maestro requires explicit user confirmation for external action %s on target %s. Reply exactly: CONFIRM MAESTRO %s. Challenge expires at %s. Nothing was changed.", result.Action, result.Target, result.ChallengeID, result.ExpiresAt.UTC().Format(time.RFC3339))
}

func writeJSON(out io.Writer, value any, errOut io.Writer) int {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return reportError(errOut, err)
	}
	return ExitOK
}

func reportError(errOut io.Writer, err error) int {
	fmt.Fprintln(errOut, err)
	return ExitFailure
}

// background is a seam for future bounded cancellation without accepting a
// caller-provided context authority in the public CLI shape.
func background() context.Context {
	return context.Background()
}
