package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	baseprofile "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/profile"
	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	baseskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/skills"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/adaptercfg"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/profile"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimecap"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionhook"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionresolve"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionstart"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceagent"
)

const (
	ExitOK          = 0
	ExitFailure     = 1
	ExitUsage       = 2
	ExitUnavailable = 3
)

var Version = "0.0.0-dev"

const maximumOwnerFacetBytes = 1 << 20
const maximumWorkspaceAgentBytes = 1 << 20

func Run(args []string, out, errOut io.Writer) int {
	return RunWithInput(args, strings.NewReader(""), out, errOut)
}

func RunWithInput(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos <init|doctor|status|version|auth|update|profile|owner|workspace-agent|atlas|session|hook|adapter|skills|memory>")
		return ExitUsage
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprintln(out, "usage: bcgos <init|doctor|status|version|auth|update|profile|owner|workspace-agent|atlas|session|hook|adapter|skills|memory>")
		return ExitOK
	case "init":
		return runInit(args[1:], out, errOut, defaultDataRoot)
	case "doctor":
		return runDoctor(args[1:], out, errOut, defaultDataRoot, commandAvailable)
	case "status":
		return runProductStatus(args[1:], out, errOut, defaultDataRoot)
	case "version":
		fmt.Fprintf(out, "bcgos %s\n", Version)
		return ExitOK
	case "auth":
		return runAuth(args[1:], out, errOut, defaultReleaseAuthService())
	case "update":
		return runUpdate(args[1:], out, errOut, defaultReleaseUpdateService())
	case "profile":
		return runProfile(args[1:], out, errOut, defaultDataRoot)
	case "owner":
		return runOwnerWithInput(args[1:], in, out, errOut, defaultDataRoot)
	case "workspace-agent":
		return runWorkspaceAgentWithInput(args[1:], in, out, errOut, defaultDataRoot)
	case "atlas":
		return runAtlas(args[1:], out, errOut, defaultDataRoot)
	case "session":
		return runSession(args[1:], out, errOut, defaultDataRoot)
	case "hook":
		return runHook(args[1:], out, errOut, defaultDataRoot)
	case "adapter":
		return runAdapter(args[1:], out, errOut)
	case "skills":
		return runSkills(args[1:], out, errOut)
	case "memory":
		return runMemory(args[1:], in, out, errOut)
	default:
		fmt.Fprintf(errOut, "unknown command %q\n", args[0])
		return ExitUsage
	}
}

func runInit(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	flags := newFlagSet("init", errOut)
	allowSynchronized := flags.Bool("allow-synced-workspace", false, "confirm initialization inside a synchronized folder")
	requestedProfile := flags.String("profile", "", "interaction profile: standard, advanced or power")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if flags.NArg() > 1 {
		fmt.Fprintln(errOut, "usage: bcgos init [--allow-synced-workspace] [--profile standard|advanced|power] [path]")
		return ExitUsage
	}
	path := "."
	if flags.NArg() == 1 {
		path = flags.Arg(0)
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	result, err := workspace.Initialize(workspace.Options{WorkspacePath: path, DataRoot: root, AllowSynchronizedRoot: *allowSynchronized})
	if errors.Is(err, workspace.ErrSynchronizedWorkspace) {
		fmt.Fprintln(errOut, "workspace appears to be inside OneDrive or another synchronized root; choose a local folder such as ~/Developer, or rerun with --allow-synced-workspace after explicit confirmation")
		return ExitUsage
	}
	if err != nil {
		return reportError(errOut, err)
	}
	agent, err := workspaceagent.Initialize(root, result.WorkspaceID)
	if err != nil {
		return reportError(errOut, err)
	}
	state, err := initializeProfile(root, *requestedProfile)
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, struct {
		workspace.Result
		Profile        profile.State         `json:"profile"`
		WorkspaceAgent workspaceagent.Status `json:"workspace_agent"`
	}{Result: result, Profile: state, WorkspaceAgent: agent}, errOut)
}

func runWorkspaceAgent(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	return runWorkspaceAgentWithInput(args, strings.NewReader(""), out, errOut, dataRoot)
}

func runWorkspaceAgentWithInput(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos workspace-agent <status|interview|brief|research|economic> [options]")
		return ExitUsage
	}
	if args[0] == "research" {
		return runWorkspaceAgentResearch(args[1:], in, out, errOut, dataRoot)
	}
	if args[0] == "brief" {
		return runWorkspaceAgentBrief(args[1:], in, out, errOut, dataRoot)
	}
	if args[0] == "economic" {
		return runWorkspaceAgentEconomic(args[1:], in, out, errOut, dataRoot)
	}
	path, code := oneOptionalPath("workspace-agent "+args[0], args[1:], errOut)
	if code != ExitOK {
		return code
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	inspection, err := workspace.Inspect(path, root)
	if err != nil {
		return reportError(errOut, err)
	}
	if inspection.WorkspaceID == "" {
		fmt.Fprintln(errOut, "workspace is not initialized; run bcgos init first")
		return ExitUsage
	}
	switch args[0] {
	case "interview":
		return writeJSON(out, workspaceagent.ColdStartInterview(), errOut)
	case "status":
		status, err := workspaceagent.Inspect(root, inspection.WorkspaceID)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, status, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos workspace-agent <status|interview|brief|research|economic> [options]")
		return ExitUsage
	}
}

func runWorkspaceAgentBrief(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 || args[0] != "submit" {
		fmt.Fprintln(errOut, "usage: bcgos workspace-agent brief submit --stdin [workspace-path]")
		return ExitUsage
	}
	flags := newFlagSet("workspace-agent brief submit", errOut)
	stdin := flags.Bool("stdin", false, "read reviewed workspace brief as JSON")
	if err := flags.Parse(args[1:]); err != nil || !*stdin || flags.NArg() > 1 {
		fmt.Fprintln(errOut, "usage: bcgos workspace-agent brief submit --stdin [workspace-path]")
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	workspaceID, code := workspaceAgentID(root, optionalArg(flags.Args()), errOut)
	if code != ExitOK {
		return code
	}
	var brief workspaceagent.Brief
	if err := decodeWorkspaceAgentJSON(in, &brief); err != nil {
		return reportError(errOut, err)
	}
	brief.WorkspaceID, brief.BriefID, brief.CreatedAt = workspaceID, "", time.Time{}
	saved, err := workspaceagent.SaveBrief(root, brief)
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, saved, errOut)
}

func runWorkspaceAgentResearch(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos workspace-agent research <plan|approve|query|record> [options] [workspace-path]")
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	switch args[0] {
	case "plan":
		flags := newFlagSet("workspace-agent research plan", errOut)
		stdin := flags.Bool("stdin", false, "read proposed research plan as JSON")
		if err := flags.Parse(args[1:]); err != nil || !*stdin || flags.NArg() > 1 {
			fmt.Fprintln(errOut, "usage: bcgos workspace-agent research plan --stdin [workspace-path]")
			return ExitUsage
		}
		workspaceID, code := workspaceAgentID(root, optionalArg(flags.Args()), errOut)
		if code != ExitOK {
			return code
		}
		var plan workspaceagent.ResearchPlan
		if err := decodeWorkspaceAgentJSON(in, &plan); err != nil {
			return reportError(errOut, err)
		}
		plan.WorkspaceID, plan.PlanID, plan.State = workspaceID, "", ""
		plan.CreatedAt, plan.Approval = time.Time{}, workspaceagent.Approval{}
		created, err := workspaceagent.CreateResearchPlan(root, plan)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, created, errOut)
	case "approve":
		flags := newFlagSet("workspace-agent research approve", errOut)
		planID := flags.String("plan", "", "research plan ID")
		approvedBy := flags.String("approved-by", "", "approving owner")
		confirm := flags.Bool("confirm", false, "confirm external disclosure scope")
		if err := flags.Parse(args[1:]); err != nil || *planID == "" || *approvedBy == "" || !*confirm || flags.NArg() > 1 {
			fmt.Fprintln(errOut, "usage: bcgos workspace-agent research approve --plan <id> --approved-by <owner> --confirm [workspace-path]")
			return ExitUsage
		}
		workspaceID, code := workspaceAgentID(root, optionalArg(flags.Args()), errOut)
		if code != ExitOK {
			return code
		}
		approved, err := workspaceagent.ApproveResearchPlan(root, workspaceID, *planID, workspaceagent.Approval{ApprovedAt: time.Now().UTC(), ApprovedBy: *approvedBy, DisclosureLevel: "public_only"})
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, approved, errOut)
	case "query":
		flags := newFlagSet("workspace-agent research query", errOut)
		stdin := flags.Bool("stdin", false, "consume one approved external query as JSON")
		if err := flags.Parse(args[1:]); err != nil || !*stdin || flags.NArg() > 1 {
			fmt.Fprintln(errOut, "usage: bcgos workspace-agent research query --stdin [workspace-path]")
			return ExitUsage
		}
		workspaceID, code := workspaceAgentID(root, optionalArg(flags.Args()), errOut)
		if code != ExitOK {
			return code
		}
		var execution workspaceagent.QueryExecution
		if err := decodeWorkspaceAgentJSON(in, &execution); err != nil {
			return reportError(errOut, err)
		}
		execution.WorkspaceID, execution.ExecutedAt, execution.Slot = workspaceID, time.Time{}, 0
		consumed, err := workspaceagent.ConsumeResearchQuery(root, execution)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, consumed, errOut)
	case "record":
		flags := newFlagSet("workspace-agent research record", errOut)
		stdin := flags.Bool("stdin", false, "read sourced evidence as JSON")
		if err := flags.Parse(args[1:]); err != nil || !*stdin || flags.NArg() > 1 {
			fmt.Fprintln(errOut, "usage: bcgos workspace-agent research record --stdin [workspace-path]")
			return ExitUsage
		}
		workspaceID, code := workspaceAgentID(root, optionalArg(flags.Args()), errOut)
		if code != ExitOK {
			return code
		}
		var evidence workspaceagent.Evidence
		if err := decodeWorkspaceAgentJSON(in, &evidence); err != nil {
			return reportError(errOut, err)
		}
		evidence.WorkspaceID = workspaceID
		if err := workspaceagent.RecordEvidence(root, evidence); err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, map[string]string{"state": "recorded", "plan_id": evidence.PlanID}, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos workspace-agent research <plan|approve|query|record> [options] [workspace-path]")
		return ExitUsage
	}
}

func runWorkspaceAgentEconomic(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos workspace-agent economic <import|attach> [options]")
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	switch args[0] {
	case "import":
		flags := newFlagSet("workspace-agent economic import", errOut)
		stdin := flags.Bool("stdin", false, "read attested public snapshot as JSON")
		attestedPublic := flags.Bool("attested-public", false, "confirm the snapshot contains only independently sourced public information")
		attestedBy := flags.String("attested-by", "", "person responsible for the public-source attestation")
		noWorkspaceDerivation := flags.Bool("confirm-no-workspace-derivation", false, "confirm no workspace or client material contributed to the snapshot")
		if err := flags.Parse(args[1:]); err != nil || !*stdin || !*attestedPublic || strings.TrimSpace(*attestedBy) == "" || !*noWorkspaceDerivation || flags.NArg() != 0 {
			fmt.Fprintln(errOut, "usage: bcgos workspace-agent economic import --stdin --attested-public --attested-by <owner> --confirm-no-workspace-derivation")
			return ExitUsage
		}
		var snapshot workspaceagent.EconomicSnapshot
		if err := decodeWorkspaceAgentJSON(in, &snapshot); err != nil {
			return reportError(errOut, err)
		}
		snapshot.SnapshotID, snapshot.CreatedAt = "", time.Time{}
		snapshot.Attestation = workspaceagent.PublicAttestation{
			AttestedBy:            *attestedBy,
			AttestedAt:            time.Now().UTC(),
			Origin:                "independent_public_sources",
			NoWorkspaceDerivation: true,
		}
		saved, err := workspaceagent.SaveEconomicSnapshot(root, snapshot)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, saved, errOut)
	case "attach":
		flags := newFlagSet("workspace-agent economic attach", errOut)
		snapshotID := flags.String("snapshot", "", "public economic snapshot ID")
		if err := flags.Parse(args[1:]); err != nil || *snapshotID == "" || flags.NArg() > 1 {
			fmt.Fprintln(errOut, "usage: bcgos workspace-agent economic attach --snapshot <id> [workspace-path]")
			return ExitUsage
		}
		workspaceID, code := workspaceAgentID(root, optionalArg(flags.Args()), errOut)
		if code != ExitOK {
			return code
		}
		if err := workspaceagent.AttachEconomicSnapshot(root, workspaceID, *snapshotID); err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, map[string]string{"state": "attached", "snapshot_id": *snapshotID}, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos workspace-agent economic <import|attach> [options]")
		return ExitUsage
	}
}

func workspaceAgentID(dataRoot, workspacePath string, errOut io.Writer) (string, int) {
	inspection, err := workspace.Inspect(workspacePath, dataRoot)
	if err != nil {
		return "", reportError(errOut, err)
	}
	if inspection.WorkspaceID == "" {
		fmt.Fprintln(errOut, "workspace is not initialized; run bcgos init first")
		return "", ExitUsage
	}
	return inspection.WorkspaceID, ExitOK
}

func optionalArg(args []string) string {
	if len(args) == 1 {
		return args[0]
	}
	return "."
}

func decodeWorkspaceAgentJSON(in io.Reader, target any) error {
	decoder := json.NewDecoder(io.LimitReader(in, maximumWorkspaceAgentBytes+1))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("workspace-agent input contains trailing JSON or exceeds 1 MiB")
	}
	return nil
}

func runProductStatus(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	path, code := oneOptionalPath("status", args, errOut)
	if code != ExitOK {
		return code
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	inspection, err := workspace.Inspect(path, root)
	if err != nil {
		return reportError(errOut, err)
	}
	state, err := resolveProfile(root, "", false)
	if err != nil {
		return reportError(errOut, err)
	}
	releaseCapability := defaultReleaseCapability()
	return writeJSON(out, struct {
		Version      string               `json:"version"`
		Workspace    workspace.Inspection `json:"workspace"`
		Capabilities map[string]string    `json:"capabilities"`
		Profile      profile.State        `json:"profile"`
	}{
		Version:   Version,
		Workspace: inspection,
		Profile:   state,
		Capabilities: map[string]string{
			"bundles":                "supported",
			"human_atlas_bootstrap":  "supported",
			"interaction_profile":    "supported",
			"memory_dreaming":        "unavailable",
			"private_release_auth":   releaseCapability.State,
			"updates":                releaseCapability.State,
			"workspace_agent_setup":  "supported",
			"workspace_research":     "managed_skill_runtime_dependent",
			"public_economic_rollup": "supported",
		},
	}, errOut)
}

type doctorCheck struct {
	ID      string `json:"id"`
	State   string `json:"state"`
	Message string `json:"message"`
}

func runDoctor(args []string, out, errOut io.Writer, dataRoot func() (string, error), available func(string) bool) int {
	path, code := oneOptionalPath("doctor", args, errOut)
	if code != ExitOK {
		return code
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	inspection, err := workspace.Inspect(path, root)
	if err != nil {
		return reportError(errOut, err)
	}
	profileState, err := resolveProfile(root, "", false)
	if err != nil {
		return reportError(errOut, err)
	}
	manifest, err := baseruntime.Manifest()
	if err != nil {
		return reportError(errOut, err)
	}
	runtimeReports := make([]runtimecap.Report, 0, 2)
	for _, runtimeID := range []string{"claude", "codex"} {
		report, err := manifest.Report(runtimeID, available(runtimeID))
		if err != nil {
			return reportError(errOut, err)
		}
		runtimeReports = append(runtimeReports, report)
	}
	state := "ready"
	nextActions := []string{}
	workspaceCheck := doctorCheck{ID: "workspace", State: "pass", Message: "workspace metadata and readable brain are ready"}
	switch inspection.State {
	case "uninitialized":
		state = "action_required"
		workspaceCheck = doctorCheck{ID: "workspace", State: "action_required", Message: "workspace is not initialized"}
		nextActions = append(nextActions, "Run bcgos init <local-workspace-path>.")
	case "invalid", "incomplete":
		state = "action_required"
		workspaceCheck = doctorCheck{ID: "workspace", State: "action_required", Message: "workspace metadata or brain surface needs repair"}
		nextActions = append(nextActions, "Review the workspace path and rerun bcgos init only after confirming it is safe.")
	case "warning":
		state = "warning"
		workspaceCheck = doctorCheck{ID: "workspace", State: "warning", Message: "workspace appears to be synchronized; OneDrive-style sync can cause I/O timeouts"}
		nextActions = append(nextActions, "Move future work to a local folder outside synchronized storage when practical.")
	}
	releaseCapability := defaultReleaseCapability()
	checks := []doctorCheck{
		workspaceCheck,
		{ID: "local_data", State: "pass", Message: "private BCGOS data is separated from the workspace"},
		interactionProfileCheck(profileState),
		runtimeCheck("claude_code", "claude", available),
		runtimeCheck("codex", "codex", available),
		adapterCheck("claude_adapter", "claude", inspection.WorkspacePath),
		adapterCheck("codex_adapter", "codex", inspection.WorkspacePath),
		{ID: "bundles", State: "pass", Message: "signed bundle activation and last-known-good rollback are supported"},
		{
			ID: "private_release_auth", State: releaseCapability.State,
			Message: releaseCapability.Reason,
		},
		{
			ID: "updates", State: releaseCapability.State,
			Message: releaseCapability.Reason,
		},
	}
	if !available("claude") && !available("codex") {
		if state == "ready" {
			state = "action_required"
		}
		nextActions = append(nextActions, "Install or open Claude Code or Codex before starting an assisted session.")
	}
	if len(nextActions) == 0 {
		nextActions = append(nextActions, "Open Claude Code or Codex in this workspace to begin guided onboarding.")
	}
	return writeJSON(out, struct {
		State               string               `json:"state"`
		Workspace           workspace.Inspection `json:"workspace"`
		Checks              []doctorCheck        `json:"checks"`
		RuntimeCapabilities []runtimecap.Report  `json:"runtime_capabilities"`
		Profile             profile.State        `json:"profile"`
		NextActions         []string             `json:"next_actions"`
	}{State: state, Workspace: inspection, Checks: checks, RuntimeCapabilities: runtimeReports, Profile: profileState, NextActions: nextActions}, errOut)
}

func runProfile(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "usage: bcgos profile <show|set standard|advanced|power>")
		return ExitOK
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	switch args[0] {
	case "show":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos profile show")
			return ExitUsage
		}
		state, err := resolveProfile(root, "", false)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, state, errOut)
	case "set":
		if len(args) != 2 {
			fmt.Fprintln(errOut, "usage: bcgos profile set <standard|advanced|power>")
			return ExitUsage
		}
		state, err := resolveProfile(root, args[1], true)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, state, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos profile <show|set standard|advanced|power>")
		return ExitUsage
	}
}

func runSkills(args []string, out, errOut io.Writer) int {
	if len(args) != 1 || args[0] != "index" {
		fmt.Fprintln(errOut, "usage: bcgos skills index")
		return ExitUsage
	}
	catalog, err := baseskills.Catalog()
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, catalog, errOut)
}

func runOwner(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	return runOwnerWithInput(args, strings.NewReader(""), out, errOut, dataRoot)
}

func runOwnerWithInput(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos owner <init|status|interview|refine>")
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	switch args[0] {
	case "init":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos owner init")
			return ExitUsage
		}
		status, err := ownerctx.Initialize(root)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, status, errOut)
	case "interview":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos owner interview")
			return ExitUsage
		}
		return writeJSON(out, ownerctx.ColdStartInterview(), errOut)
	case "status":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos owner status")
			return ExitUsage
		}
		status, err := ownerctx.Inspect(root)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, status, errOut)
	case "refine":
		return runOwnerRefine(args[1:], in, out, errOut, root)
	default:
		fmt.Fprintln(errOut, "usage: bcgos owner <init|status|interview|refine>")
		return ExitUsage
	}
}

func runOwnerRefine(args []string, in io.Reader, out, errOut io.Writer, root string) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos owner refine <submit|apply|revert>")
		return ExitUsage
	}
	switch args[0] {
	case "submit":
		flags := newFlagSet("owner refine submit", errOut)
		facet := flags.String("facet", "", "owner facet to refine")
		evidence := flags.String("evidence", "", "short provenance summary")
		stdin := flags.Bool("stdin", false, "read proposed facet body from standard input")
		if err := flags.Parse(args[1:]); err != nil {
			return ExitUsage
		}
		if flags.NArg() != 0 || !*stdin || *facet == "" || *evidence == "" {
			fmt.Fprintln(errOut, "usage: bcgos owner refine submit --facet <facet> --evidence <summary> --stdin")
			return ExitUsage
		}
		body, err := io.ReadAll(io.LimitReader(in, maximumOwnerFacetBytes+1))
		if err != nil {
			return reportError(errOut, err)
		}
		if len(body) > maximumOwnerFacetBytes {
			fmt.Fprintln(errOut, "proposed owner facet body exceeds 1 MiB")
			return ExitUsage
		}
		receipt, err := ownerctx.SubmitRefinement(root, ownerctx.RefinementInput{Facet: *facet, Evidence: *evidence, ProposedBody: string(body)})
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, receipt, errOut)
	case "apply":
		flags := newFlagSet("owner refine apply", errOut)
		confirm := flags.Bool("confirm", false, "confirm applying a guarded refinement")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 1 {
			fmt.Fprintln(errOut, "usage: bcgos owner refine apply <proposal-id> --confirm")
			return ExitUsage
		}
		receipt, err := ownerctx.ApplyRefinement(root, flags.Arg(0), *confirm)
		if errors.Is(err, ownerctx.ErrConfirmationRequired) {
			fmt.Fprintln(errOut, "this owner facet requires explicit confirmation; rerun with --confirm")
			return ExitUsage
		}
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, receipt, errOut)
	case "revert":
		flags := newFlagSet("owner refine revert", errOut)
		confirm := flags.Bool("confirm", false, "confirm reverting an owner refinement")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 1 {
			fmt.Fprintln(errOut, "usage: bcgos owner refine revert <audit-id> --confirm")
			return ExitUsage
		}
		receipt, err := ownerctx.RevertRefinement(root, flags.Arg(0), *confirm)
		if errors.Is(err, ownerctx.ErrConfirmationRequired) {
			fmt.Fprintln(errOut, "reverting an owner refinement requires --confirm")
			return ExitUsage
		}
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, receipt, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos owner refine <submit|apply|revert>")
		return ExitUsage
	}
}

func runAtlas(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 || (args[0] != "init" && args[0] != "status") {
		fmt.Fprintln(errOut, "usage: bcgos atlas <init|status> [workspace-path]")
		return ExitUsage
	}
	path, code := oneOptionalPath("atlas "+args[0], args[1:], errOut)
	if code != ExitOK {
		return code
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	inspection, err := workspace.Inspect(path, root)
	if err != nil {
		return reportError(errOut, err)
	}
	if inspection.State == "uninitialized" || inspection.State == "invalid" || inspection.State == "incomplete" {
		fmt.Fprintln(errOut, "workspace must be initialized and readable before atlas bootstrap; run bcgos init <workspace-path>")
		return ExitUsage
	}
	options := atlas.Options{DataRoot: root, WorkspacePath: inspection.WorkspacePath, WorkspaceID: inspection.WorkspaceID}
	if args[0] == "init" {
		status, err := atlas.Initialize(options)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, status, errOut)
	}
	return writeJSON(out, atlas.Inspect(options), errOut)
}

func runSession(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 || (args[0] != "packet" && args[0] != "bridge" && args[0] != "resolve") {
		fmt.Fprintln(errOut, "usage: bcgos session <packet|bridge|resolve> [workspace-path]")
		return ExitUsage
	}
	command := args[0]
	if command == "resolve" {
		return runSessionResolve(args[1:], out, errOut, dataRoot)
	}
	runtimeName := ""
	remaining := args[1:]
	if command == "bridge" {
		flags := newFlagSet("session bridge", errOut)
		runtime := flags.String("runtime", "", "target runtime: claude or codex")
		if err := flags.Parse(remaining); err != nil {
			return ExitUsage
		}
		runtimeName = *runtime
		remaining = flags.Args()
	}
	path, code := oneOptionalPath("session "+command, remaining, errOut)
	if code != ExitOK {
		return code
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	inspection, err := workspace.Inspect(path, root)
	if err != nil {
		return reportError(errOut, err)
	}
	profileState, err := resolveProfile(root, "", false)
	if err != nil {
		return reportError(errOut, err)
	}
	owner, err := ownerctx.Inspect(root)
	if err != nil {
		return reportError(errOut, err)
	}
	packet := sessionctx.Build(sessionctx.Sources{
		Profile: profileState, Workspace: inspection, Owner: owner,
		Atlas: atlas.Inspect(atlas.Options{DataRoot: root, WorkspacePath: inspection.WorkspacePath, WorkspaceID: inspection.WorkspaceID}),
	})
	if err := packet.Validate(); err != nil {
		return reportError(errOut, err)
	}
	if command == "bridge" {
		envelope, err := sessionstart.Build(runtimeName, packet)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, envelope, errOut)
	}
	return writeJSON(out, packet, errOut)
}

func runSessionResolve(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	flags := newFlagSet("session resolve", errOut)
	pointer := flags.String("pointer", "", "packet pointer to resolve")
	purpose := flags.String("purpose", "", "authorized purpose")
	budget := flags.Int("budget-bytes", 0, "maximum body bytes")
	if err := flags.Parse(args); err != nil || *pointer == "" || *purpose == "" || flags.NArg() > 1 {
		fmt.Fprintln(errOut, "usage: bcgos session resolve --pointer <pointer> --purpose session --budget-bytes <1..8192> [workspace-path]")
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	path := optionalArg(flags.Args())
	inspection, err := workspace.Inspect(path, root)
	if err != nil {
		return reportError(errOut, err)
	}
	profileState, err := resolveProfile(root, "", false)
	if err != nil {
		return reportError(errOut, err)
	}
	owner, err := ownerctx.Inspect(root)
	if err != nil {
		return reportError(errOut, err)
	}
	packet := sessionctx.Build(sessionctx.Sources{Profile: profileState, Workspace: inspection, Owner: owner, Atlas: atlas.Inspect(atlas.Options{DataRoot: root, WorkspacePath: inspection.WorkspacePath, WorkspaceID: inspection.WorkspaceID})})
	result, err := sessionresolve.Resolve(root, *pointer, *purpose, packet, *budget)
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, result, errOut)
}

func runHook(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 || args[0] != "session-start" {
		fmt.Fprintln(errOut, "usage: bcgos hook session-start --runtime claude|codex [workspace-path]")
		return ExitUsage
	}
	flags := newFlagSet("hook session-start", errOut)
	runtimeName := flags.String("runtime", "", "target runtime: claude or codex")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() > 1 {
		fmt.Fprintln(errOut, "usage: bcgos hook session-start --runtime claude|codex [workspace-path]")
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	path := optionalArg(flags.Args())
	inspection, err := workspace.Inspect(path, root)
	if err != nil {
		return reportError(errOut, err)
	}
	profileState, err := resolveProfile(root, "", false)
	if err != nil {
		return reportError(errOut, err)
	}
	owner, err := ownerctx.Inspect(root)
	if err != nil {
		return reportError(errOut, err)
	}
	packet := sessionctx.Build(sessionctx.Sources{
		Profile: profileState, Workspace: inspection, Owner: owner,
		Atlas: atlas.Inspect(atlas.Options{DataRoot: root, WorkspacePath: inspection.WorkspacePath, WorkspaceID: inspection.WorkspaceID}),
	})
	var output any
	switch *runtimeName {
	case "claude":
		output, err = sessionhook.BuildClaude(packet)
	case "codex":
		output, err = sessionhook.BuildCodex(packet)
	default:
		err = fmt.Errorf("unsupported runtime %q", *runtimeName)
	}
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, output, errOut)
}

func runAdapter(args []string, out, errOut io.Writer) int {
	if len(args) == 0 || (args[0] != "install" && args[0] != "uninstall" && args[0] != "status") {
		fmt.Fprintln(errOut, "usage: bcgos adapter <install|uninstall|status> --runtime claude|codex [workspace-path]")
		return ExitUsage
	}
	flags := newFlagSet("adapter "+args[0], errOut)
	runtimeName := flags.String("runtime", "", "target runtime: claude or codex")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() > 1 {
		fmt.Fprintln(errOut, "usage: bcgos adapter <install|uninstall|status> --runtime claude|codex [workspace-path]")
		return ExitUsage
	}
	path := optionalArg(flags.Args())
	var (
		status adaptercfg.Status
		err    error
	)
	switch args[0] {
	case "install":
		status, err = adaptercfg.Install(*runtimeName, path)
	case "uninstall":
		status, err = adaptercfg.Uninstall(*runtimeName, path)
	case "status":
		status, err = adaptercfg.Inspect(*runtimeName, path)
	}
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, status, errOut)
}

func resolveProfile(dataRoot, requested string, explicit bool) (profile.State, error) {
	policy, err := baseprofile.Policy()
	if err != nil {
		return profile.State{}, err
	}
	store := profile.Store{Root: dataRoot, Policy: policy}
	if explicit {
		return store.Set(requested)
	}
	return store.Get()
}

func initializeProfile(dataRoot, requested string) (profile.State, error) {
	policy, err := baseprofile.Policy()
	if err != nil {
		return profile.State{}, err
	}
	store := profile.Store{Root: dataRoot, Policy: policy}
	if requested != "" {
		return store.Set(requested)
	}
	return store.Ensure()
}

func oneOptionalPath(command string, args []string, errOut io.Writer) (string, int) {
	if len(args) > 1 {
		fmt.Fprintf(errOut, "usage: bcgos %s [path]\n", command)
		return "", ExitUsage
	}
	if len(args) == 1 {
		return args[0], ExitOK
	}
	return ".", ExitOK
}

func runtimeCheck(id, executable string, available func(string) bool) doctorCheck {
	if available(executable) {
		return doctorCheck{ID: id, State: "available", Message: executable + " was found"}
	}
	return doctorCheck{ID: id, State: "unavailable", Message: executable + " was not found; this is not a BCGOS installation failure"}
}

func adapterCheck(id, runtime, workspacePath string) doctorCheck {
	status, err := adaptercfg.Inspect(runtime, workspacePath)
	if err != nil {
		return doctorCheck{ID: id, State: "warning", Message: "adapter configuration cannot be inspected safely: " + err.Error()}
	}
	if status.State == "installed" {
		return doctorCheck{ID: id, State: "configured", Message: "workspace-local adapter is configured; runtime trust and execution remain separate checks"}
	}
	return doctorCheck{ID: id, State: "unavailable", Message: "workspace-local adapter is not configured"}
}

func interactionProfileCheck(state profile.State) doctorCheck {
	if state.Source == "fallback" {
		return doctorCheck{ID: "interaction_profile", State: "warning", Message: state.Warning}
	}
	return doctorCheck{ID: "interaction_profile", State: "pass", Message: "active profile is " + state.Profile}
}

func commandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func defaultDataRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return workspace.DefaultDataRoot(runtime.GOOS, home, os.Getenv("LOCALAPPDATA"), os.Getenv("XDG_STATE_HOME"))
}

func runMemory(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos memory <capture|status|context|dream>")
		return ExitUsage
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprintln(out, "usage: bcgos memory <capture|status|context|dream>")
		return ExitOK
	case "capture":
		return runCapture(args[1:], in, out, errOut)
	case "status":
		return runStatus(args[1:], out, errOut)
	case "context":
		return runContext(args[1:], out, errOut)
	case "dream":
		return runDream(args[1:], out, errOut)
	default:
		fmt.Fprintf(errOut, "unknown memory command %q\n", args[0])
		return ExitUsage
	}
}

func runCapture(args []string, in io.Reader, out, errOut io.Writer) int {
	flags := newFlagSet("memory capture", errOut)
	dataDir := flags.String("data-dir", "", "local BCGOS data directory")
	workspace := flags.String("workspace", "", "workspace identity")
	kind := flags.String("kind", "", "sanitized signal kind")
	stdin := flags.Bool("stdin", false, "read sanitized signal text from standard input")
	sanitized := flags.Bool("sanitized", false, "attest that adapter sanitization has completed")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	if missing := required(map[string]string{"--data-dir": *dataDir, "--workspace": *workspace, "--kind": *kind}); missing != "" {
		fmt.Fprintf(errOut, "%s is required\n", missing)
		return ExitUsage
	}
	if !*sanitized {
		fmt.Fprintln(errOut, "--sanitized is required; raw input must not be persisted")
		return ExitUsage
	}
	if !*stdin {
		fmt.Fprintln(errOut, "--stdin is required; professional content must not be passed in process arguments")
		return ExitUsage
	}
	const maximumCaptureBytes = 1 << 20
	content, err := io.ReadAll(io.LimitReader(in, maximumCaptureBytes+1))
	if err != nil {
		return reportError(errOut, err)
	}
	if len(content) > maximumCaptureBytes {
		return reportError(errOut, errors.New("capture exceeds 1 MiB limit"))
	}
	text := strings.TrimSpace(string(content))
	if text == "" {
		fmt.Fprintln(errOut, "standard input is empty")
		return ExitUsage
	}
	policy, err := basememory.Policy()
	if err != nil {
		return reportError(errOut, err)
	}
	engine := memory.Engine{Root: filepath.Join(*dataDir, "memory"), Policy: policy}
	path, err := engine.Capture(memory.Capture{WorkspaceID: *workspace, RecordedAt: time.Now().UTC(), Kind: *kind, Text: text, Sanitized: true})
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, map[string]any{"workspace_id": *workspace, "state": "captured", "path": path}, errOut)
}

func runStatus(args []string, out, errOut io.Writer) int {
	flags := newFlagSet("memory status", errOut)
	dataDir := flags.String("data-dir", "", "local BCGOS data directory")
	workspace := flags.String("workspace", "", "workspace identity")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	if missing := required(map[string]string{"--data-dir": *dataDir, "--workspace": *workspace}); missing != "" {
		fmt.Fprintf(errOut, "%s is required\n", missing)
		return ExitUsage
	}
	engine := memory.Engine{Root: filepath.Join(*dataDir, "memory")}
	report, err := engine.Status(*workspace)
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, struct {
		memory.StatusReport
		Dreaming string `json:"dreaming"`
	}{StatusReport: report, Dreaming: "unavailable"}, errOut)
}

func runContext(args []string, out, errOut io.Writer) int {
	flags := newFlagSet("memory context", errOut)
	dataDir := flags.String("data-dir", "", "local BCGOS data directory")
	workspace := flags.String("workspace", "", "workspace identity")
	l1 := flags.Int("budget-l1", 0, "maximum L1 characters")
	l2 := flags.Int("budget-l2", 0, "maximum L2 characters")
	l3 := flags.Int("budget-l3", 0, "maximum L3 characters")
	lifetime := flags.Int("budget-lifetime", 0, "maximum lifetime characters")
	if err := flags.Parse(args); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	if missing := required(map[string]string{"--data-dir": *dataDir, "--workspace": *workspace}); missing != "" {
		fmt.Fprintf(errOut, "%s is required\n", missing)
		return ExitUsage
	}
	budgets := map[string]int{"L1": *l1, "L2": *l2, "L3": *l3, "lifetime": *lifetime}
	for layer, budget := range budgets {
		if budget <= 0 {
			fmt.Fprintf(errOut, "positive budget required for %s\n", layer)
			return ExitUsage
		}
	}
	policy, err := basememory.Policy()
	if err != nil {
		return reportError(errOut, err)
	}
	engine := memory.Engine{Root: filepath.Join(*dataDir, "memory"), Policy: policy, Budgets: budgets}
	bundle, err := engine.AssembleContext(*workspace)
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, bundle, errOut)
}

func runDream(args []string, out, errOut io.Writer) int {
	if len(args) == 0 || (args[0] != "daily" && args[0] != "weekly") {
		fmt.Fprintln(errOut, "usage: bcgos memory dream <daily|weekly> --data-dir PATH --workspace ID")
		return ExitUsage
	}
	cycle := args[0]
	flags := newFlagSet("memory dream "+cycle, errOut)
	dataDir := flags.String("data-dir", "", "local BCGOS data directory")
	workspace := flags.String("workspace", "", "workspace identity")
	if err := flags.Parse(args[1:]); err != nil {
		return ExitUsage
	}
	if rejectPositionals(flags, errOut) {
		return ExitUsage
	}
	if missing := required(map[string]string{"--data-dir": *dataDir, "--workspace": *workspace}); missing != "" {
		fmt.Fprintf(errOut, "%s is required\n", missing)
		return ExitUsage
	}
	code := writeJSON(out, map[string]any{
		"capability":   "memory_dreaming",
		"cycle":        cycle,
		"state":        "unavailable",
		"workspace_id": *workspace,
		"reason":       "no synthesis and eligibility adapter is installed",
	}, errOut)
	if code != ExitOK {
		return code
	}
	return ExitUnavailable
}

func newFlagSet(name string, errOut io.Writer) *flag.FlagSet {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(errOut)
	return flags
}

func required(values map[string]string) string {
	for _, key := range []string{"--data-dir", "--workspace", "--kind"} {
		if value, exists := values[key]; exists && strings.TrimSpace(value) == "" {
			return key
		}
	}
	return ""
}

func rejectPositionals(flags *flag.FlagSet, errOut io.Writer) bool {
	if flags.NArg() == 0 {
		return false
	}
	fmt.Fprintf(errOut, "unexpected positional argument %q; professional content must enter through standard input\n", flags.Arg(0))
	return true
}

func reportError(errOut io.Writer, err error) int {
	if err == nil {
		return ExitOK
	}
	_ = json.NewEncoder(errOut).Encode(map[string]string{"state": "error", "error": err.Error()})
	return ExitFailure
}

func writeJSON(out io.Writer, value any, errOut io.Writer) int {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		if !errors.Is(err, io.ErrClosedPipe) {
			fmt.Fprintln(errOut, err)
		}
		return ExitFailure
	}
	return ExitOK
}
