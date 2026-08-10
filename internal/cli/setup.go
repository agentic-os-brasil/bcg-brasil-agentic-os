package cli

import (
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

func runSetup(args []string, out, errOut io.Writer, dataRoot func() (string, error), identity func() (setupauth.Identity, error)) int {
	if len(args) == 0 || (args[0] != "status" && args[0] != "authorize") {
		fmt.Fprintln(errOut, "usage: bcgos setup <status|authorize> --workspace PATH [--confirm]")
		return ExitUsage
	}
	action := args[0]
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
