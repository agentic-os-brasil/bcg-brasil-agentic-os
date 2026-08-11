package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/user"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/priorwork"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/setupauth"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

const (
	setupComplete                    = "complete"
	setupCompleteWithExternalPending = "complete_with_external_actions_pending"
	setupBlocked                     = "blocked"
)

type setupStage struct {
	ID     string `json:"id"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

type setupApplyReport struct {
	SchemaVersion int              `json:"schema_version"`
	State         string           `json:"state"`
	WorkspaceID   string           `json:"workspace_id,omitempty"`
	WorkspacePath string           `json:"workspace_path"`
	Runtime       string           `json:"runtime"`
	Authorization setupauth.Status `json:"authorization"`
	Stages        []setupStage     `json:"stages"`
	Error         string           `json:"error,omitempty"`
}

func runSetup(args []string, out, errOut io.Writer, dataRoot func() (string, error), identity func() (setupauth.Identity, error)) int {
	if len(args) == 0 || (args[0] != "status" && args[0] != "authorize" && args[0] != "apply") {
		fmt.Fprintln(errOut, "usage: bcgos setup <status|authorize|apply> --workspace PATH [--runtime claude|codex] [--executable PATH] [--confirm]")
		return ExitUsage
	}
	action := args[0]
	if action == "apply" {
		return runSetupApply(args[1:], out, errOut, dataRoot, identity)
	}
	flags := flag.NewFlagSet("setup "+action, flag.ContinueOnError)
	flags.SetOutput(errOut)
	workspacePath := flags.String("workspace", "", "initialized Maestro workspace path")
	confirm := flags.Bool("confirm", false, "record the one-and-done local setup authorization")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || strings.TrimSpace(*workspacePath) == "" || (action == "status" && *confirm) || (action == "authorize" && !*confirm) {
		fmt.Fprintln(errOut, setupUsage(action))
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	inspection, err := workspace.Inspect(*workspacePath, root)
	if err != nil {
		return reportError(errOut, err)
	}
	if inspection.WorkspaceID == "" || (inspection.State != "ready" && inspection.State != "warning") {
		return reportError(errOut, errors.New("one-and-done setup requires an initialized Maestro workspace"))
	}
	actor, err := identity()
	if err != nil {
		return reportError(errOut, err)
	}
	request, err := setupAuthorizationRequest(root, inspection.WorkspaceID, inspection.WorkspacePath)
	if err != nil {
		return reportError(errOut, err)
	}
	store := setupauth.Store{Root: root}
	var status setupauth.Status
	if action == "authorize" {
		status, err = store.Authorize(request, actor, true)
	} else {
		status, err = store.Status(request, actor)
	}
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, status, errOut)
}

func runSetupApply(args []string, out, errOut io.Writer, dataRoot func() (string, error), identity func() (setupauth.Identity, error)) int {
	flags := flag.NewFlagSet("setup apply", flag.ContinueOnError)
	flags.SetOutput(errOut)
	workspacePath := flags.String("workspace", "", "Maestro workspace path")
	runtimeName := flags.String("runtime", "", "target runtime: claude or codex")
	executable := flags.String("executable", "", "path to the installed bcgos executable")
	confirm := flags.Bool("confirm", false, "record the one-and-done local setup authorization")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || strings.TrimSpace(*workspacePath) == "" || (*runtimeName != "claude" && *runtimeName != "codex") {
		fmt.Fprintln(errOut, "usage: bcgos setup apply --workspace PATH --runtime claude|codex [--executable PATH] [--confirm]")
		return ExitUsage
	}
	if err := ensureUserLevelProcess(); err != nil {
		report := setupApplyReport{
			SchemaVersion: 1, State: setupBlocked, WorkspacePath: *workspacePath, Runtime: *runtimeName,
			Stages: []setupStage{{ID: "process_identity", Status: setupBlocked, Detail: err.Error()}},
			Error:  err.Error(),
		}
		_ = writeJSON(out, report, errOut)
		return ExitFailure
	}
	report := setupApplyReport{SchemaVersion: 1, State: setupBlocked, WorkspacePath: *workspacePath, Runtime: *runtimeName}
	finishBlocked := func(stage, detail string) int {
		report.Stages = append(report.Stages, setupStage{ID: stage, Status: setupBlocked, Detail: detail})
		report.Error = detail
		if code := writeJSON(out, report, errOut); code != ExitOK {
			return code
		}
		return ExitFailure
	}
	root, err := dataRoot()
	if err != nil {
		return finishBlocked("data_root", err.Error())
	}
	actor, err := identity()
	if err != nil {
		return finishBlocked("identity", err.Error())
	}
	inspection, err := workspace.Inspect(*workspacePath, root)
	if err != nil {
		return finishBlocked("workspace_preflight", err.Error())
	}
	workspaceReady := inspection.WorkspaceID != "" && (inspection.State == "ready" || inspection.State == "warning")
	if !workspaceReady && !*confirm {
		return finishBlocked("authorization", "one setup confirmation is required before the first local mutation")
	}
	if workspaceReady {
		request, requestErr := setupAuthorizationRequest(root, inspection.WorkspaceID, inspection.WorkspacePath)
		if requestErr != nil {
			return finishBlocked("authorization", requestErr.Error())
		}
		status, statusErr := (setupauth.Store{Root: root}).Status(request, actor)
		if statusErr != nil {
			return finishBlocked("authorization", statusErr.Error())
		}
		if status.State != setupauth.StateActive && !*confirm {
			report.Authorization = status
			return finishBlocked("authorization", "one setup confirmation is required for this workspace, identity or scope")
		}
	}

	var commandOutput bytes.Buffer
	if code := runInit([]string{*workspacePath}, &commandOutput, &commandOutput, func() (string, error) { return root, nil }); code != ExitOK {
		return finishBlocked("workspace_initialize", strings.TrimSpace(commandOutput.String()))
	}
	initStatus := "completed"
	if workspaceReady {
		initStatus = "already_ready"
	}
	report.Stages = append(report.Stages, setupStage{ID: "workspace_initialize", Status: initStatus, Detail: "workspace metadata and local owner scaffolds are ready"})
	inspection, err = workspace.Inspect(*workspacePath, root)
	if err != nil || inspection.WorkspaceID == "" || (inspection.State != "ready" && inspection.State != "warning") {
		if err == nil {
			err = errors.New("workspace did not become ready")
		}
		return finishBlocked("workspace_readiness", err.Error())
	}
	report.WorkspaceID = inspection.WorkspaceID
	report.WorkspacePath = inspection.WorkspacePath
	request, err := setupAuthorizationRequest(root, inspection.WorkspaceID, inspection.WorkspacePath)
	if err != nil {
		return finishBlocked("authorization", err.Error())
	}
	statusBefore, err := (setupauth.Store{Root: root}).Status(request, actor)
	if err != nil {
		return finishBlocked("authorization", err.Error())
	}
	report.Authorization, err = (setupauth.Store{Root: root}).Authorize(request, actor, *confirm)
	if err != nil {
		return finishBlocked("authorization", err.Error())
	}
	authorizationStatus := "completed"
	if statusBefore.State == setupauth.StateActive {
		authorizationStatus = "already_ready"
	}
	report.Stages = append(report.Stages, setupStage{ID: "authorization", Status: authorizationStatus, Detail: "bounded local setup grant is active"})

	adapterAlreadyReady := false
	commandOutput.Reset()
	if code := runAdapterWithDataRoot([]string{"status", "--runtime", *runtimeName, inspection.WorkspacePath}, &commandOutput, &commandOutput, func() (string, error) { return root, nil }); code == ExitOK {
		var current struct {
			State      string `json:"state"`
			Projection struct {
				State string `json:"state"`
			} `json:"projection"`
		}
		adapterAlreadyReady = json.Unmarshal(commandOutput.Bytes(), &current) == nil && current.State == "installed" && current.Projection.State == "installed"
	}
	installArgs := []string{"install", "--runtime", *runtimeName}
	if strings.TrimSpace(*executable) != "" {
		installArgs = append(installArgs, "--executable", *executable)
	}
	installArgs = append(installArgs, inspection.WorkspacePath)
	commandOutput.Reset()
	if code := runAdapterWithDataRoot(installArgs, &commandOutput, &commandOutput, func() (string, error) { return root, nil }); code != ExitOK {
		return finishBlocked("runtime_adapter", strings.TrimSpace(commandOutput.String()))
	}
	adapterStatus := "completed"
	if adapterAlreadyReady {
		adapterStatus = "already_ready"
	}
	report.Stages = append(report.Stages, setupStage{ID: "runtime_adapter", Status: adapterStatus, Detail: "runtime hooks and local projection are installed; failed writes are rolled back by the adapter transaction"})
	commandOutput.Reset()
	if code := runAdapterWithDataRoot([]string{"status", "--runtime", *runtimeName, inspection.WorkspacePath}, &commandOutput, &commandOutput, func() (string, error) { return root, nil }); code != ExitOK {
		return finishBlocked("local_diagnostics", strings.TrimSpace(commandOutput.String()))
	}
	report.Stages = append(report.Stages, setupStage{ID: "local_diagnostics", Status: "completed", Detail: "workspace and runtime adapter status are readable"})

	release := defaultReleaseCapability()
	if release.State == "configured" {
		report.Stages = append(report.Stages, setupStage{ID: "release_channel", Status: "already_ready", Detail: release.Reason})
		report.State = setupComplete
	} else {
		report.Stages = append(report.Stages, setupStage{ID: "release_channel", Status: "pending_external", Detail: release.Reason})
		report.State = setupCompleteWithExternalPending
	}
	return writeJSON(out, report, errOut)
}

func setupUsage(action string) string {
	if action == "status" {
		return "usage: bcgos setup status --workspace PATH"
	}
	return "usage: bcgos setup authorize --workspace PATH --confirm"
}

func setupAuthorizationRequest(root, workspaceID, workspacePath string) (setupauth.Request, error) {
	request := setupauth.Request{WorkspaceID: workspaceID, WorkspacePath: workspacePath}
	status, err := priorWorkSourceSelectionStore(root).Status(workspaceID)
	if err != nil {
		return setupauth.Request{}, fmt.Errorf("read selected-source scope for setup authorization: %w", err)
	}
	if status.State == priorwork.SourceSelected {
		request.SourceFingerprint = status.Fingerprint
	}
	return request, nil
}

func currentSetupIdentity() (setupauth.Identity, error) {
	principal, err := user.Current()
	if err != nil {
		return setupauth.Identity{}, fmt.Errorf("resolve local setup principal: %w", err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		return setupauth.Identity{}, fmt.Errorf("resolve local setup device: %w", err)
	}
	if strings.TrimSpace(principal.Username) == "" || strings.TrimSpace(hostname) == "" {
		return setupauth.Identity{}, errors.New("local setup identity is unavailable")
	}
	return setupauth.DeriveIdentity(principal.Username, hostname), nil
}

func setupAuthorizationForPacket(root string, inspection workspace.Inspection) setupauth.Status {
	unavailable := setupauth.Status{SchemaVersion: setupauth.SchemaVersion, State: "unavailable", PolicyVersion: setupauth.PolicyVersion, WorkspaceID: inspection.WorkspaceID}
	if inspection.WorkspaceID == "" || inspection.WorkspacePath == "" {
		return unavailable
	}
	identity, err := currentSetupIdentity()
	if err != nil {
		return unavailable
	}
	request, err := setupAuthorizationRequest(root, inspection.WorkspaceID, inspection.WorkspacePath)
	if err != nil {
		return unavailable
	}
	status, err := (setupauth.Store{Root: root}).Status(request, identity)
	if err != nil {
		return unavailable
	}
	return status
}
