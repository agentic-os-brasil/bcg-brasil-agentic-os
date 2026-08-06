package cli

import (
	"bytes"
	"context"
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

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	baseprofile "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/profile"
	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	baseskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/skills"
	bundlecatalog "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/catalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/actionconfirmation"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/adaptercfg"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/canary"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/codexadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/continuoususe"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/federation"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/markitdown"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installreadiness"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maestro"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/profile"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimecap"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimeprojection"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionctx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionhook"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionresolve"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionstart"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillrouting"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspaceagent"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspacemigration"
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
const maximumWorkContractBytes = 32 << 10
const maximumOrchestrationStateBytes = agentorchestration.MaximumDurableStateBytes
const installedOrchestrationStatePath = ".bcgos/maestro-orchestration-state.json"

var enqueueHookPresenceWake = func(workspaceID string) error {
	executable, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve bcgos executable for presence wake: %w", err)
	}
	command := exec.Command(executable, "maintenance", "wake", "--trigger", "presence", "--workspace", workspaceID, "--idle-state", "auto")
	if err := command.Start(); err != nil {
		return fmt.Errorf("enqueue maintenance presence wake: %w", err)
	}
	go func() { _ = command.Wait() }()
	return nil
}

type hookOrchestrationState struct {
	configured bool
	digest     string
}

func Run(args []string, out, errOut io.Writer) int {
	return RunWithInput(args, strings.NewReader(""), out, errOut)
}

func RunWithInput(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos <init|doctor|status|version|auth|update|profile|owner|maestro|agent|workspace|workspace-agent|workspace-migration|atlas|prior-work|session|hook|adapter|skills|bundles|memory|maintenance|ingest|federation|canary|work>")
		return ExitUsage
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprintln(out, "usage: bcgos <init|doctor|status|version|auth|update|profile|owner|maestro|agent|workspace|workspace-agent|workspace-migration|atlas|prior-work|session|hook|adapter|skills|bundles|memory|maintenance|ingest|federation|canary|work>")
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
	case "maestro":
		return runMaestroWithInput(args[1:], in, out, errOut, defaultDataRoot)
	case "agent":
		return runAgentWithInput(args[1:], in, out, errOut, defaultDataRoot)
	case "workspace-agent":
		return runWorkspaceAgentWithInput(args[1:], in, out, errOut, defaultDataRoot)
	case "workspace":
		return runWorkspaceImport(args[1:], out, errOut, defaultDataRoot)
	case "workspace-migration":
		return runWorkspaceMigration(args[1:], out, errOut, defaultDataRoot)
	case "atlas":
		return runAtlas(args[1:], out, errOut, defaultDataRoot)
	case "prior-work":
		return runPriorWork(args[1:], in, out, errOut, defaultDataRoot)
	case "session":
		return runSession(args[1:], out, errOut, defaultDataRoot)
	case "hook":
		return runHookWithInput(args[1:], in, out, errOut, defaultDataRoot)
	case "adapter":
		return runAdapterWithDataRoot(args[1:], out, errOut, defaultDataRoot)
	case "skills":
		return runSkills(args[1:], out, errOut)
	case "bundles":
		return runBundles(args[1:], out, errOut)
	case "memory":
		return runMemory(args[1:], in, out, errOut)
	case "maintenance":
		return runMaintenance(args[1:], out, errOut)
	case "ingest":
		return runIngest(args[1:], out, errOut, defaultDataRoot)
	case "federation":
		return runFederation(args[1:], out, errOut, defaultDataRoot)
	case "canary":
		return runCanary(args[1:], out, errOut, defaultDataRoot)
	case "work":
		return runWork(args[1:], in, out, errOut, defaultDataRoot)
	default:
		fmt.Fprintf(errOut, "unknown command %q\n", args[0])
		return ExitUsage
	}
}

func runCanary(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) != 1 || args[0] != "report" {
		fmt.Fprintln(errOut, "usage: bcgos canary report")
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	report, err := (canary.Store{Root: filepath.Join(root, "canary")}).Report()
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, report, errOut)
}

type workCreateRequest struct {
	Objective       string                `json:"objective"`
	InitialNextStep string                `json:"initial_next_step"`
	Criteria        []execution.Criterion `json:"criteria"`
	AllowedRefs     []string              `json:"allowed_refs"`
}

func runWork(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos work <create|start|checkpoint|pause|resume|next|evidence|complete|inspect|export|delete>")
		return ExitUsage
	}
	if args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "usage: bcgos work <create|start|checkpoint|pause|resume|next|evidence|complete|inspect|export|delete>")
		return ExitOK
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	switch args[0] {
	case "create":
		flags := newFlagSet("work create", errOut)
		workspacePath := flags.String("workspace", "", "initialized workspace path")
		stdin := flags.Bool("stdin", false, "read execution contract from standard input")
		if err := flags.Parse(args[1:]); err != nil || rejectPositionals(flags, errOut) || strings.TrimSpace(*workspacePath) == "" || !*stdin {
			fmt.Fprintln(errOut, "usage: bcgos work create --workspace PATH --stdin")
			return ExitUsage
		}
		body, err := io.ReadAll(io.LimitReader(in, maximumWorkContractBytes+1))
		if err != nil || len(body) > maximumWorkContractBytes {
			return reportError(errOut, errors.New("execution contract exceeds 32 KiB limit"))
		}
		decoder := json.NewDecoder(bytes.NewReader(body))
		decoder.DisallowUnknownFields()
		var req workCreateRequest
		if err := decoder.Decode(&req); err != nil {
			return reportError(errOut, err)
		}
		store, workspaceID, err := executionStoreForWorkspace(root, *workspacePath)
		if err != nil {
			return reportError(errOut, err)
		}
		item, err := store.Create(execution.CreateInput{WorkspaceID: workspaceID, Objective: req.Objective, InitialNextStep: req.InitialNextStep, Criteria: req.Criteria, AllowedRefs: req.AllowedRefs})
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, execution.Receipt(item), errOut)
	case "start", "checkpoint", "pause", "resume", "next", "inspect", "export", "delete":
		flags := newFlagSet("work "+args[0], errOut)
		workspacePath := flags.String("workspace", "", "initialized workspace path")
		itemID := flags.String("item", "", "execution item identity")
		revision := flags.Int("revision", 0, "expected state revision")
		confirm := flags.Bool("confirm", false, "confirm deletion")
		attempt := flags.String("attempt", "", "active attempt identity")
		stdin := flags.Bool("stdin", false, "read checkpoint JSON")
		active := flags.Bool("active", false, "resolve the active execution item")
		if err := flags.Parse(args[1:]); err != nil || rejectPositionals(flags, errOut) || strings.TrimSpace(*workspacePath) == "" || (args[0] != "next" && strings.TrimSpace(*itemID) == "") {
			return ExitUsage
		}
		store, workspaceID, err := executionStoreForWorkspace(root, *workspacePath)
		if err != nil {
			return reportError(errOut, err)
		}
		switch args[0] {
		case "start":
			if *revision < 1 {
				return ExitUsage
			}
			item, err := store.Start(workspaceID, *itemID, *revision)
			if err != nil {
				return reportError(errOut, err)
			}
			return writeJSON(out, execution.Receipt(item), errOut)
		case "checkpoint":
			_ = stdin
			body, err := io.ReadAll(io.LimitReader(in, maximumWorkContractBytes+1))
			if err != nil {
				return reportError(errOut, err)
			}
			var input struct {
				Summary      string   `json:"summary"`
				NextStep     string   `json:"next_step"`
				Blocker      string   `json:"blocker"`
				ArtifactRefs []string `json:"artifact_refs"`
			}
			if err := json.Unmarshal(body, &input); err != nil {
				return reportError(errOut, err)
			}
			item, err := store.Checkpoint(workspaceID, *itemID, execution.CheckpointInput{ExpectedRevision: *revision, AttemptID: *attempt, Summary: input.Summary, NextStep: input.NextStep, Blocker: input.Blocker, ArtifactRefs: input.ArtifactRefs})
			if err != nil {
				return reportError(errOut, err)
			}
			return writeJSON(out, execution.Receipt(item), errOut)
		case "pause":
			item, err := store.Pause(workspaceID, *itemID, *revision, *attempt)
			if err != nil {
				return reportError(errOut, err)
			}
			return writeJSON(out, execution.Receipt(item), errOut)
		case "resume":
			item, err := store.Resume(workspaceID, *itemID, *revision)
			if err != nil {
				return reportError(errOut, err)
			}
			return writeJSON(out, execution.Receipt(item), errOut)
		case "next":
			projection, err := store.NextActive(workspaceID)
			if !*active && *itemID != "" {
				projection, err = store.Next(workspaceID, *itemID)
			}
			if err != nil {
				return reportError(errOut, err)
			}
			return writeJSON(out, projection, errOut)
		case "inspect":
			item, err := store.Inspect(workspaceID, *itemID)
			if err != nil {
				return reportError(errOut, err)
			}
			return writeJSON(out, item, errOut)
		case "export":
			item, err := store.Export(workspaceID, *itemID)
			if err != nil {
				return reportError(errOut, err)
			}
			return writeJSON(out, item, errOut)
		default:
			if !*confirm || *revision < 1 {
				return ExitUsage
			}
			if err := store.Delete(workspaceID, *itemID, *revision, true); err != nil {
				return reportError(errOut, err)
			}
			return writeJSON(out, map[string]string{"item_id": *itemID, "state": "deleted", "workspace_id": workspaceID}, errOut)
		}
	case "evidence":
		flags := newFlagSet("work evidence", errOut)
		workspacePath := flags.String("workspace", "", "initialized workspace path")
		itemID := flags.String("item", "", "execution item identity")
		revision := flags.Int("revision", 0, "expected state revision")
		attemptID := flags.String("attempt", "", "current attempt identity")
		criterionID := flags.String("criterion", "", "completion criterion identity")
		if err := flags.Parse(args[1:]); err != nil || rejectPositionals(flags, errOut) ||
			strings.TrimSpace(*workspacePath) == "" || strings.TrimSpace(*itemID) == "" ||
			strings.TrimSpace(*attemptID) == "" || strings.TrimSpace(*criterionID) == "" ||
			*revision < 1 {
			fmt.Fprintln(errOut, "usage: bcgos work evidence --workspace PATH --item ID --revision N --attempt ID --criterion ID")
			return ExitUsage
		}
		store, workspaceID, err := executionStoreForWorkspace(root, *workspacePath)
		if err != nil {
			return reportError(errOut, err)
		}
		inspection, err := workspace.Inspect(*workspacePath, root)
		if err != nil {
			return reportError(errOut, err)
		}
		result, err := store.CollectEvidence(workspaceID, *itemID, execution.EvidenceInput{
			WorkspaceRoot: inspection.WorkspacePath, ExpectedRevision: *revision,
			AttemptID: *attemptID, CriterionID: *criterionID,
		})
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, execution.EvidenceMutationReceipt(result), errOut)
	case "complete":
		flags := newFlagSet("work complete", errOut)
		workspacePath := flags.String("workspace", "", "initialized workspace path")
		itemID := flags.String("item", "", "execution item identity")
		revision := flags.Int("revision", 0, "expected state revision")
		attemptID := flags.String("attempt", "", "current attempt identity")
		if err := flags.Parse(args[1:]); err != nil || rejectPositionals(flags, errOut) ||
			strings.TrimSpace(*workspacePath) == "" || strings.TrimSpace(*itemID) == "" ||
			strings.TrimSpace(*attemptID) == "" || *revision < 1 {
			fmt.Fprintln(errOut, "usage: bcgos work complete --workspace PATH --item ID --revision N --attempt ID")
			return ExitUsage
		}
		store, workspaceID, err := executionStoreForWorkspace(root, *workspacePath)
		if err != nil {
			return reportError(errOut, err)
		}
		inspection, err := workspace.Inspect(*workspacePath, root)
		if err != nil {
			return reportError(errOut, err)
		}
		item, err := store.Complete(workspaceID, *itemID, execution.CompletionInput{
			WorkspaceRoot: inspection.WorkspacePath, ExpectedRevision: *revision,
			AttemptID: *attemptID,
		})
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, execution.Receipt(item), errOut)
	default:
		fmt.Fprintf(errOut, "unknown work command %q\n", args[0])
		return ExitUsage
	}
}

func executionStoreForWorkspace(dataRoot, workspacePath string) (execution.Store, string, error) {
	inspection, err := workspace.Inspect(workspacePath, dataRoot)
	if err != nil {
		return execution.Store{}, "", err
	}
	if inspection.State != "ready" && inspection.State != "warning" {
		return execution.Store{}, "", errors.New("workspace must be initialized and readable before accessing execution items")
	}
	return execution.Store{Root: dataRoot}, inspection.WorkspaceID, nil
}

func runFederation(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos federation <enroll|status|revoke>")
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	store := federation.ExportStore{Root: filepath.Join(root, "federation")}
	switch args[0] {
	case "enroll":
		flags := newFlagSet("federation enroll", errOut)
		accepted := flags.Bool("accept-federated-improvement-contract", false, "accept the one-time automatic pilot reporting contract")
		endpoint := flags.String("bridge-endpoint", "", "managed HTTPS batch bridge endpoint")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			fmt.Fprintln(errOut, "usage: bcgos federation enroll --accept-federated-improvement-contract --bridge-endpoint https://bridge.example/federation/v1/batches")
			return ExitUsage
		}
		if !*accepted {
			fmt.Fprintln(errOut, "--accept-federated-improvement-contract is required; enrollment authorizes automatic typed pilot reporting until revocation")
			return ExitUsage
		}
		installationID, err := federation.NewInstallationID()
		if err != nil {
			return reportError(errOut, err)
		}
		if err := store.Enroll(federation.Enrollment{InstallationID: installationID, BridgeEndpoint: *endpoint, ContractVersion: federation.PilotContractVersion, AcceptedAt: time.Now().UTC(), AutomaticExport: true}); err != nil {
			return reportError(errOut, err)
		}
		return writeFederationStatus(out, "enrolled", store, errOut)
	case "status":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos federation status")
			return ExitUsage
		}
		enrollment, err := store.Enrollment()
		if errors.Is(err, federation.ErrNotEnrolled) {
			return writeJSON(out, struct {
				State string `json:"state"`
			}{State: "not_enrolled"}, errOut)
		}
		if err != nil {
			return reportError(errOut, err)
		}
		state := "enrolled"
		if !enrollment.RevokedAt.IsZero() {
			state = "revoked"
		}
		return writeFederationStatus(out, state, store, errOut)
	case "revoke":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos federation revoke")
			return ExitUsage
		}
		if err := store.Revoke(time.Now().UTC()); err != nil {
			return reportError(errOut, err)
		}
		return writeFederationStatus(out, "revoked", store, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos federation <enroll|status|revoke>")
		return ExitUsage
	}
}

func writeFederationStatus(out io.Writer, state string, store federation.ExportStore, errOut io.Writer) int {
	enrollment, err := store.Enrollment()
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, struct {
		State           string    `json:"state"`
		ContractVersion string    `json:"contract_version"`
		AcceptedAt      time.Time `json:"accepted_at"`
		AutomaticExport bool      `json:"automatic_export"`
		RevokedAt       time.Time `json:"revoked_at,omitempty"`
	}{State: state, ContractVersion: enrollment.ContractVersion, AcceptedAt: enrollment.AcceptedAt, AutomaticExport: enrollment.AutomaticExport, RevokedAt: enrollment.RevokedAt}, errOut)
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
	if _, err := ownerctx.Initialize(root); err != nil {
		return reportError(errOut, fmt.Errorf("bootstrap owner context: %w", err))
	}
	agent, err := workspaceagent.Initialize(root, result.WorkspaceID)
	if err != nil {
		return reportError(errOut, err)
	}
	agentStub, err := agentscaffold.Scaffold(root, agentscaffold.WorkspaceRequest(result.WorkspaceID))
	if err != nil {
		return reportError(errOut, err)
	}
	state, err := initializeProfile(root, *requestedProfile)
	if err != nil {
		return reportError(errOut, err)
	}
	managedIdentities, err := loadManagedIdentities(root)
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, struct {
		workspace.Result
		Profile                profile.State             `json:"profile"`
		CaseAgent              workspaceagent.Status     `json:"case_agent"`
		AgentStub              agentscaffold.Status      `json:"agent_stub"`
		AgentIdentityInterview agentidentity.Interview   `json:"agent_identity_interview"`
		ManagedAgentIdentities []agentidentity.Selection `json:"managed_agent_identities"`
	}{Result: result, Profile: state, CaseAgent: agent, AgentStub: agentStub, AgentIdentityInterview: agentidentity.InitialInterview(), ManagedAgentIdentities: managedIdentities}, errOut)
}

func runAgent(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	return runAgentWithInput(args, strings.NewReader(""), out, errOut, dataRoot)
}

type activationPlanInput struct {
	Envelope activationpolicy.IntentEnvelope `json:"envelope"`
}

type activationCompletionInput struct {
	Envelope activationpolicy.IntentEnvelope      `json:"envelope"`
	Plan     activationpolicy.RoutePlan           `json:"plan"`
	Receipts []activationpolicy.CompletionReceipt `json:"receipts"`
}

type activationAdvisoryInput struct {
	Envelope activationpolicy.IntentEnvelope  `json:"envelope"`
	Request  activationpolicy.AdvisoryRequest `json:"request"`
}

func runAgentWithInput(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos agent <interview|personalize|hire|scaffold|status|plan|declassify|verify|monitor|darwin> [options]")
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	switch args[0] {
	case "darwin":
		return runDarwin(args[1:], in, out, errOut, root)
	case "interview":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos agent interview")
			return ExitUsage
		}
		return writeJSON(out, agentidentity.GuidedIdentityInterview(root), errOut)
	case "personalize":
		if len(args) == 1 || (args[1] != "draft" && args[1] != "review" && args[1] != "confirm") {
			fmt.Fprintln(errOut, "usage: bcgos agent personalize <draft --stdin --consent --no-client-data|review --id ID|confirm --id ID --digest SHA256 --confirm>")
			return ExitUsage
		}
		if args[1] == "review" {
			flags := newFlagSet("agent personalize review", errOut)
			id := flags.String("id", "", "draft ID")
			if flags.Parse(args[2:]) != nil || rejectPositionals(flags, errOut) || *id == "" {
				fmt.Fprintln(errOut, "usage: bcgos agent personalize review --id ID")
				return ExitUsage
			}
			draft, err := agentidentity.ReviewProfileDraft(root, *id)
			if err != nil {
				return reportError(errOut, err)
			}
			return writeJSON(out, draft, errOut)
		}
		if args[1] == "confirm" {
			flags := newFlagSet("agent personalize confirm", errOut)
			id := flags.String("id", "", "draft ID")
			digest := flags.String("digest", "", "review digest")
			confirmed := flags.Bool("confirm", false, "confirm exact reviewed draft")
			if flags.Parse(args[2:]) != nil || rejectPositionals(flags, errOut) || *id == "" || *digest == "" || !*confirmed {
				fmt.Fprintln(errOut, "usage: bcgos agent personalize confirm --id ID --digest SHA256 --confirm")
				return ExitUsage
			}
			draft, err := agentidentity.ConfirmProfileDraft(root, *id, *digest, true)
			if err != nil {
				return reportError(errOut, err)
			}
			return writeJSON(out, draft, errOut)
		}
		flags := newFlagSet("agent personalize draft", errOut)
		stdin := flags.Bool("stdin", false, "read proposed profile from stdin")
		consent := flags.Bool("consent", false, "record explicit owner consent")
		noClientData := flags.Bool("no-client-data", false, "owner attests that the profile contains no client data")
		if flags.Parse(args[2:]) != nil || rejectPositionals(flags, errOut) || !*stdin || !*consent || !*noClientData {
			fmt.Fprintln(errOut, "usage: bcgos agent personalize draft --stdin --consent --no-client-data")
			return ExitUsage
		}
		body, err := io.ReadAll(io.LimitReader(in, maximumWorkContractBytes+1))
		if err != nil || len(body) > maximumWorkContractBytes {
			return reportError(errOut, errors.New("agent personalization input exceeds 32 KiB"))
		}
		var input agentidentity.Profile
		if err := agentidentity.DecodeStrict(body, &input); err != nil {
			return reportError(errOut, err)
		}
		if len(input.CapabilityTracks) > 0 {
			catalog, catalogErr := bundlecatalog.Catalog()
			if catalogErr != nil {
				return reportError(errOut, catalogErr)
			}
			_, planErr := catalog.PlanForTracks(input.CapabilityTracks)
			if planErr != nil {
				return reportError(errOut, planErr)
			}
		}
		input.UpdatedAt = time.Now().UTC()
		draft, err := agentidentity.DraftProfile(root, input, true, true)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, draft, errOut)
	case "identity":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos agent identity")
			return ExitUsage
		}
		profile, err := agentidentity.Load(root)
		if errors.Is(err, os.ErrNotExist) {
			return writeJSON(out, struct {
				Interview     agentidentity.Interview   `json:"interview"`
				ManagedAgents []agentidentity.Selection `json:"managed_agents"`
			}{Interview: agentidentity.InitialInterview(), ManagedAgents: agentidentity.ResolveManaged(agentidentity.Profile{})}, errOut)
		}
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, struct {
			Profile       agentidentity.Profile     `json:"profile"`
			ManagedAgents []agentidentity.Selection `json:"managed_agents"`
		}{Profile: profile, ManagedAgents: agentidentity.ResolveManaged(profile)}, errOut)
	case "scaffold", "hire":
		command := "agent " + args[0]
		flags := newFlagSet(command, errOut)
		agentID := flags.String("id", "", "path-safe agent ID")
		role := flags.String("role", "", "managed role")
		scopeKind := flags.String("scope-kind", "", "workspace, account, case or practice")
		scopeID := flags.String("scope", "", "immutable scope ID")
		parent := flags.String("parent", "", "registered parent agent ID")
		parentRole := flags.String("parent-role", "", "registered parent role")
		accountAgent := flags.String("account-agent", "", "registered Client Account Agent relation for a case")
		owner := flags.String("owner", "", "accountable owner slug for account/practice roots")
		mandate := flags.String("mandate", "", "bounded mandate for account/practice roots")
		canon := flags.String("canon", "", "data-root-relative practice canon path")
		canonSHA256 := flags.String("canon-sha256", "", "verified practice canon SHA-256")
		expertKind := flags.String("expert-kind", "", "PA expert kind: FPA or IPA")
		expertVersion := flags.String("expert-version", "", "PA expert semantic version")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 ||
			*agentID == "" || *role == "" || *scopeKind == "" || *scopeID == "" ||
			*parent == "" || *parentRole == "" {
			fmt.Fprintf(errOut, "usage: bcgos %s --id <id> --role <role> --scope-kind <kind> --scope <id> --parent <id> --parent-role <role> [--owner <id> --mandate <text> --canon <path> --canon-sha256 <hash> --expert-kind FPA|IPA --expert-version <semver>]\n", command)
			return ExitUsage
		}
		status, err := agentscaffold.Scaffold(root, agentscaffold.Request{
			AgentID: *agentID, Role: *role, ScopeKind: *scopeKind,
			ScopeID: *scopeID, ParentAgent: *parent, ParentRole: *parentRole,
			AccountAgentID: *accountAgent,
			Owner:          *owner, Mandate: *mandate,
			CanonPath: *canon, CanonSHA256: *canonSHA256,
			ExpertKind: *expertKind, ExpertVersion: *expertVersion,
			ExpertLifecycle: func() string {
				if *role == "pa_expert" {
					return "draft"
				}
				return ""
			}(),
		})
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, status, errOut)
	case "status":
		flags := newFlagSet("agent status", errOut)
		agentID := flags.String("id", "", "agent ID")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *agentID == "" {
			fmt.Fprintln(errOut, "usage: bcgos agent status --id <id>")
			return ExitUsage
		}
		status, err := agentscaffold.Inspect(root, *agentID)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, status, errOut)
	case "plan":
		if err := requireAgentStdin(args[1:], errOut); err != nil {
			return ExitUsage
		}
		var input activationPlanInput
		if err := decodeActivationJSON(in, &input); err != nil {
			return reportError(errOut, err)
		}
		experts, err := localPAExpertRegistry(root)
		if err != nil {
			return reportError(errOut, err)
		}
		plan, err := activationpolicy.Plan(input.Envelope, experts)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, plan, errOut)
	case "declassify":
		if err := requireAgentStdin(args[1:], errOut); err != nil {
			return ExitUsage
		}
		var input activationAdvisoryInput
		if err := decodeActivationJSON(in, &input); err != nil {
			return reportError(errOut, err)
		}
		experts, err := localPAExpertRegistry(root)
		if err != nil {
			return reportError(errOut, err)
		}
		plan, err := activationpolicy.Plan(input.Envelope, experts)
		if err != nil {
			return reportError(errOut, err)
		}
		if input.Request.PlanSHA256 != plan.PlanSHA256 ||
			input.Request.EpisodeSHA256 != activationpolicy.SHA256Hex([]byte(input.Envelope.EpisodeID)) ||
			input.Request.Classification != input.Envelope.Sensitivity ||
			(input.Envelope.Sensitivity != activationpolicy.Public &&
				input.Envelope.Sensitivity != activationpolicy.Internal) ||
			!planSelectsExpert(plan, input.Request.Expert) {
			return reportError(errOut, errors.New("advisory request is not bound to the current deterministic plan and hired PA expert"))
		}
		receipt, err := activationpolicy.Declassify(input.Request)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, receipt, errOut)
	case "verify":
		if err := requireAgentStdin(args[1:], errOut); err != nil {
			return ExitUsage
		}
		var input activationCompletionInput
		if err := decodeActivationJSON(in, &input); err != nil {
			return reportError(errOut, err)
		}
		experts, err := localPAExpertRegistry(root)
		if err != nil {
			return reportError(errOut, err)
		}
		expected, err := activationpolicy.Plan(input.Envelope, experts)
		if err != nil {
			return reportError(errOut, err)
		}
		if expected.PlanSHA256 != input.Plan.PlanSHA256 {
			return reportError(errOut, errors.New("completion plan does not match the current deterministic policy and PA Expert registry"))
		}
		if err := activationpolicy.VerifyCompletion(input.Plan, input.Receipts); err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, map[string]any{
			"episode_id":             input.Plan.EpisodeID,
			"plan_sha256":            input.Plan.PlanSHA256,
			"state":                  "shadow_evaluated",
			"evidence_authority":     "unverified_breadcrumb",
			"may_complete_execution": false,
		}, errOut)
	case "monitor":
		if err := requireAgentStdin(args[1:], errOut); err != nil {
			return ExitUsage
		}
		var observations []activationpolicy.Observation
		if err := decodeActivationJSON(in, &observations); err != nil {
			return reportError(errOut, err)
		}
		report, err := activationpolicy.EvaluateObservations(observations)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, report, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos agent <interview|personalize|identity|hire|scaffold|status|plan|declassify|verify|monitor|darwin> [options]")
		return ExitUsage
	}
}

func runDarwin(args []string, in io.Reader, out, errOut io.Writer, root string) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos agent darwin <request|assess|housekeeping> --stdin")
		return ExitUsage
	}
	switch args[0] {
	case "request":
		if err := requireAgentStdin(args[1:], errOut); err != nil {
			return ExitUsage
		}
		var request darwin.HealthRequest
		if err := decodeActivationJSON(in, &request); err != nil {
			return reportError(errOut, err)
		}
		assessment, err := darwin.AssessHealth(context.Background(), request)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, assessment, errOut)
	case "assess":
		if err := requireAgentStdin(args[1:], errOut); err != nil {
			return ExitUsage
		}
		var packet darwin.HealthPacket
		if err := decodeActivationJSON(in, &packet); err != nil {
			return reportError(errOut, err)
		}
		assessment, err := darwin.Plan(packet)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, assessment, errOut)
	case "housekeeping":
		flags := newFlagSet("agent darwin housekeeping", errOut)
		stdin := flags.Bool("stdin", false, "read the closed Darwin health packet from standard input")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || !*stdin {
			fmt.Fprintln(errOut, "usage: bcgos agent darwin housekeeping --stdin")
			return ExitUsage
		}
		maestroCapability := strings.TrimSpace(os.Getenv("BCGOS_MAESTRO_CAPABILITY"))
		darwinCapability := strings.TrimSpace(os.Getenv("BCGOS_DARWIN_CAPABILITY"))
		recoveryCapability := strings.TrimSpace(os.Getenv("BCGOS_RECOVERY_CAPABILITY"))
		if maestroCapability == "" || darwinCapability == "" || recoveryCapability == "" {
			return reportError(errOut, errors.New("Darwin housekeeping requires BCGOS_MAESTRO_CAPABILITY, BCGOS_DARWIN_CAPABILITY and BCGOS_RECOVERY_CAPABILITY"))
		}
		var packet darwin.HealthPacket
		if err := decodeActivationJSON(in, &packet); err != nil {
			return reportError(errOut, err)
		}
		if packet.Mode != darwin.Interactive && packet.Mode != darwin.HeadlessHousekeeping {
			return reportError(errOut, errors.New("Darwin housekeeping requires interactive or headless_housekeeping input mode"))
		}
		packet.Mode = darwin.HeadlessHousekeeping
		catalog, err := baseagents.Catalog()
		if err != nil {
			return reportError(errOut, err)
		}
		state, err := agentorchestration.NewStateStore(recoveryCapability)
		if err != nil {
			return reportError(errOut, err)
		}
		adapter, err := agentorchestration.NewAdapter(packet.Runtime, catalog, []agentorchestration.Authorization{
			{AgentID: "maestro", Role: "hub", ScopeKind: "control", Capability: maestroCapability},
			darwin.Authorization(darwinCapability),
		}, state)
		if err != nil {
			return reportError(errOut, err)
		}
		branchID := "darwin-" + packet.WindowID
		if decision := adapter.StartBranch("maestro", maestroCapability, "darwin", branchID, darwin.MaintenanceScope, darwin.ScopeKind); !decision.Allowed {
			return reportError(errOut, errors.New("Darwin housekeeping branch was denied"))
		}
		guard := darwin.AdapterGuard(adapter, darwinCapability, branchID, "")
		invoker := darwin.FilesystemInvoker{Root: filepath.Join(root, "darwin")}
		assessment, err := darwin.Plan(packet)
		if err != nil {
			_ = adapter.FinishBranch("darwin", darwinCapability, branchID)
			return reportError(errOut, err)
		}
		receipt, executeErr := darwin.Execute(context.Background(), packet, assessment, guard, invoker, time.Now)
		finishDecision := adapter.FinishBranch("darwin", darwinCapability, branchID)
		if !finishDecision.Allowed {
			return reportError(errOut, errors.New("Darwin housekeeping branch could not be closed"))
		}
		if storeErr := (darwin.Store{Root: filepath.Join(root, "darwin")}).Append(receipt); storeErr != nil {
			return reportError(errOut, storeErr)
		}
		if executeErr != nil {
			return reportError(errOut, executeErr)
		}
		if receipt.Outcome == darwin.OutcomeBlocked || receipt.Outcome == darwin.OutcomeFailed {
			if err := writeJSON(out, receipt, errOut); err != ExitOK {
				return err
			}
			return ExitFailure
		}
		return writeJSON(out, receipt, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos agent darwin <request|assess|housekeeping> --stdin")
		return ExitUsage
	}
}

func loadManagedIdentities(root string) ([]agentidentity.Selection, error) {
	profile, err := agentidentity.Load(root)
	if errors.Is(err, os.ErrNotExist) {
		return agentidentity.ResolveManaged(agentidentity.Profile{}), nil
	}
	if err != nil {
		return nil, err
	}
	return agentidentity.ResolveManaged(profile), nil
}

func localPAExpertRegistry(root string) ([]activationpolicy.PAExpert, error) {
	instances, err := agentscaffold.ListPAExperts(root)
	if err != nil {
		return nil, err
	}
	managed, err := baseagents.ManagedPAExpertRegistry()
	if err != nil {
		return nil, err
	}
	byID := make(map[string]agentscaffold.Instance, len(instances))
	for _, instance := range instances {
		byID[instance.AgentID] = instance
	}
	experts := make([]activationpolicy.PAExpert, 0, len(managed.Experts))
	for _, expert := range managed.Experts {
		instance, ok := byID[expert.ID]
		if !ok || instance.ExpertKind != string(expert.Kind) ||
			instance.ExpertVersion != expert.Version ||
			instance.CanonSHA256 != expert.CanonSHA256 {
			return nil, errors.New("published PA Expert is not installed with its exact signed canon")
		}
		experts = append(experts, expert)
	}
	return experts, nil
}

func planSelectsExpert(plan activationpolicy.RoutePlan, expert activationpolicy.PAExpert) bool {
	for _, selected := range plan.Experts {
		if selected.ID == expert.ID && selected.Kind == expert.Kind &&
			selected.Version == expert.Version && selected.CanonSHA256 == expert.CanonSHA256 &&
			expert.Lifecycle == activationpolicy.Published {
			return true
		}
	}
	return false
}

func requireAgentStdin(args []string, errOut io.Writer) error {
	flags := newFlagSet("agent governed input", errOut)
	stdin := flags.Bool("stdin", false, "read strict JSON from standard input")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 || !*stdin {
		fmt.Fprintln(errOut, "--stdin is required; governed content must not be passed in process arguments")
		return errors.New("agent governed input requires stdin")
	}
	return nil
}

func decodeActivationJSON(in io.Reader, target any) error {
	const maximumActivationBytes = 64 << 10
	body, err := io.ReadAll(io.LimitReader(in, maximumActivationBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maximumActivationBytes {
		return errors.New("activation input exceeds 64 KiB")
	}
	return activationpolicy.DecodeStrict(body, target)
}

func runWorkspaceAgent(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	return runWorkspaceAgentWithInput(args, strings.NewReader(""), out, errOut, dataRoot)
}

func runWorkspaceAgentWithInput(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos workspace-agent <status|interview|brief|research|economic|value> [options]")
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
	if args[0] == "value" {
		return runWorkspaceAgentValue(args[1:], in, out, errOut, dataRoot)
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
		fmt.Fprintln(errOut, "usage: bcgos workspace-agent <status|interview|brief|research|economic|value> [options]")
		return ExitUsage
	}
}

func runWorkspaceAgentValue(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos workspace-agent value <start|intervention|submit|status> [options] [workspace-path]")
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	switch args[0] {
	case "start", "status":
		path, code := oneOptionalPath("workspace-agent value "+args[0], args[1:], errOut)
		if code != ExitOK {
			return code
		}
		workspaceID, code := workspaceAgentID(root, path, errOut)
		if code != ExitOK {
			return code
		}
		if args[0] == "start" {
			receipt, err := workspaceagent.StartFirstValue(root, workspaceID)
			if err != nil {
				return reportError(errOut, err)
			}
			return writeJSON(out, receipt, errOut)
		}
		state, err := workspaceagent.FirstValueStatus(root, workspaceID)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, state, errOut)
	case "intervention":
		flags := newFlagSet("workspace-agent value intervention", errOut)
		runID := flags.String("run", "", "first-value run ID")
		kind := flags.String("kind", "", "brief_correction, plan_correction or artifact_revision")
		if err := flags.Parse(args[1:]); err != nil || *runID == "" || *kind == "" || flags.NArg() > 1 {
			fmt.Fprintln(errOut, "usage: bcgos workspace-agent value intervention --run <id> --kind <brief_correction|plan_correction|artifact_revision> [workspace-path]")
			return ExitUsage
		}
		workspaceID, code := workspaceAgentID(root, optionalArg(flags.Args()), errOut)
		if code != ExitOK {
			return code
		}
		if err := workspaceagent.RecordFirstValueIntervention(root, workspaceID, *runID, *kind); err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, map[string]string{"state": "recorded", "run_id": *runID, "kind": *kind}, errOut)
	case "submit":
		flags := newFlagSet("workspace-agent value submit", errOut)
		runID := flags.String("run", "", "first-value run ID")
		stdin := flags.Bool("stdin", false, "read reviewed first-value submission as JSON")
		if err := flags.Parse(args[1:]); err != nil || !*stdin || *runID == "" || flags.NArg() > 1 {
			fmt.Fprintln(errOut, "usage: bcgos workspace-agent value submit --run <id> --stdin [workspace-path]")
			return ExitUsage
		}
		inspection, err := workspace.Inspect(optionalArg(flags.Args()), root)
		if err != nil {
			return reportError(errOut, err)
		}
		if inspection.WorkspaceID == "" {
			fmt.Fprintln(errOut, "workspace is not initialized; run bcgos init first")
			return ExitUsage
		}
		var submission workspaceagent.FirstValueSubmission
		if err := decodeWorkspaceAgentJSON(in, &submission); err != nil {
			return reportError(errOut, err)
		}
		receipt, err := workspaceagent.CompleteFirstValue(root, inspection.WorkspaceID, *runID, filepath.Join(inspection.WorkspacePath, "brain", "deliverables"), submission)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, receipt, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos workspace-agent value <start|intervention|submit|status> [options] [workspace-path]")
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
	owner, err := ownerctx.Inspect(root)
	if err != nil {
		return reportError(errOut, err)
	}
	continuous, _, err := buildContinuousUseStatus(root, inspection, owner)
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, struct {
		Version       string               `json:"version"`
		Workspace     workspace.Inspection `json:"workspace"`
		Capabilities  map[string]string    `json:"capabilities"`
		Profile       profile.State        `json:"profile"`
		ContinuousUse continuoususe.Status `json:"continuous_use"`
	}{
		Version:       Version,
		Workspace:     inspection,
		Profile:       state,
		ContinuousUse: continuous,
		Capabilities: map[string]string{
			"bundles":                "supported",
			"human_atlas_bootstrap":  "supported",
			"interaction_profile":    "supported",
			"memory_dreaming":        "daily_light_local_contract_weekly_deep_unavailable",
			"continuous_use":         continuous.State,
			"private_release_auth":   releaseCapability.State,
			"updates":                releaseCapability.State,
			"workspace_migration":    workspacemigration.CapabilityStatus().State + "_" + workspacemigration.CapabilityStatus().Execution,
			"case_agent_setup":       "supported",
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

func runtimeDependencyCheck(root string, inspection workspace.Inspection) (doctorCheck, string) {
	if inspection.WorkspaceID == "" {
		return doctorCheck{ID: "runtime_dependencies", State: "action_required", Message: "workspace dependencies are unavailable until bcgos init completes"}, "Run bcgos init <local-workspace-path>."
	}
	if _, err := resolveHookOrchestrationState(inspection, installedOrchestrationStatePath); err != nil {
		return doctorCheck{ID: "runtime_dependencies", State: "action_required", Message: err.Error()}, "Run bcgos init <local-workspace-path> to repair the local runtime bootstrap."
	}
	owner, err := ownerctx.Inspect(root)
	if err != nil || !owner.Initialized {
		if err == nil {
			err = errors.New("owner context is not initialized")
		}
		return doctorCheck{ID: "runtime_dependencies", State: "action_required", Message: err.Error()}, "Run bcgos init <local-workspace-path> to create the owner context."
	}
	workspaceAgent, err := workspaceagent.Inspect(root, inspection.WorkspaceID)
	if err != nil || !workspaceAgent.Initialized {
		if err == nil {
			err = errors.New("workspace agent is not initialized")
		}
		return doctorCheck{ID: "runtime_dependencies", State: "action_required", Message: err.Error()}, "Run bcgos init <local-workspace-path> to create the workspace agent."
	}
	status, err := agentscaffold.Inspect(root, agentscaffold.WorkspaceRequest(inspection.WorkspaceID).AgentID)
	if err != nil || !status.Initialized {
		if err == nil {
			err = errors.New("workspace agent scaffold is not initialized")
		}
		return doctorCheck{ID: "runtime_dependencies", State: "action_required", Message: err.Error()}, "Run bcgos init <local-workspace-path> to create the agent scaffold."
	}
	return doctorCheck{ID: "runtime_dependencies", State: "pass", Message: "durable state, owner context and agent scaffolds are ready"}, ""
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
	dependencyCheck, dependencyAction := runtimeDependencyCheck(root, inspection)
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
		dependencyCheck,
		{ID: "local_data", State: "pass", Message: "private BCGOS data is separated from the workspace"},
		interactionProfileCheck(profileState),
		runtimeCheck("claude_code", "claude", available),
		runtimeCheck("codex", "codex", available),
		adapterCheck("claude_adapter", "claude", inspection.WorkspacePath),
		adapterCheck("codex_adapter", "codex", inspection.WorkspacePath),
		lifecycleReceiptCheck(root, inspection.WorkspaceID),
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
	if dependencyAction != "" {
		if state == "ready" {
			state = "action_required"
		}
		nextActions = append(nextActions, dependencyAction)
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

func runBundles(args []string, out, errOut io.Writer) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		fmt.Fprintln(out, "usage: bcgos bundles <index|plan --track TRACK[,TRACK...]|recommend --function TEXT>")
		return ExitOK
	}
	catalog, err := bundlecatalog.Catalog()
	if err != nil {
		return reportError(errOut, err)
	}
	switch args[0] {
	case "index":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos bundles index")
			return ExitUsage
		}
		return writeJSON(out, catalog, errOut)
	case "plan":
		flags := newFlagSet("bundles plan", errOut)
		tracks := flags.String("track", "", "comma-separated declared capability tracks")
		if err := flags.Parse(args[1:]); err != nil || rejectPositionals(flags, errOut) || strings.TrimSpace(*tracks) == "" {
			fmt.Fprintln(errOut, "usage: bcgos bundles plan --track TRACK[,TRACK...]")
			return ExitUsage
		}
		plan, err := catalog.PlanForTracks(splitTracks(*tracks))
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, plan, errOut)
	case "recommend":
		flags := newFlagSet("bundles recommend", errOut)
		function := flags.String("function", "", "explicitly declared collaborator function")
		if err := flags.Parse(args[1:]); err != nil || rejectPositionals(flags, errOut) || strings.TrimSpace(*function) == "" {
			fmt.Fprintln(errOut, "usage: bcgos bundles recommend --function TEXT")
			return ExitUsage
		}
		return writeJSON(out, ownerctx.RecommendTechCore(*function), errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos bundles <index|plan --track TRACK[,TRACK...]|recommend --function TEXT>")
		return ExitUsage
	}
}

func splitTracks(value string) []string {
	parts := strings.Split(value, ",")
	for index := range parts {
		parts[index] = strings.TrimSpace(parts[index])
	}
	return parts
}

type maestroDispatchRequest struct {
	AuthenticatedOwner bool               `json:"authenticated_owner"`
	OwnerID            string             `json:"owner_id"`
	DispatchID         string             `json:"dispatch_id"`
	OccurrenceID       string             `json:"occurrence_id"`
	Prompt             string             `json:"prompt"`
	Language           string             `json:"language"`
	Source             string             `json:"source"`
	SessionID          string             `json:"session_id"`
	WorkingLanguage    string             `json:"working_language"`
	CurrentLanguage    string             `json:"current_language"`
	DraftOutput        string             `json:"draft_output"`
	Audience           string             `json:"audience"`
	Consequence        string             `json:"consequence"`
	Reversibility      string             `json:"reversibility"`
	RelevanceKeys      []string           `json:"relevance_keys"`
	SelfSignal         *maestroSelfSignal `json:"self_signal,omitempty"`
	Plan               maestro.Input      `json:"plan"`
}

// maestroSelfSignal is optional and deliberately narrow. A prompt or a
// Walter invocation is not evidence about the owner's self; only this closed
// explicit signal can request a material observation.
type maestroSelfSignal struct {
	Signal         ownerctx.SignalClass `json:"signal"`
	Facet          string               `json:"facet"`
	Claim          string               `json:"claim"`
	EvidenceType   string               `json:"evidence_type"`
	Confidence     float64              `json:"confidence"`
	Sensitivity    string               `json:"sensitivity"`
	OwnerConfirmed bool                 `json:"owner_confirmed"`
}

func validateMaestroSelfSignal(signal *maestroSelfSignal) error {
	if signal == nil {
		return nil
	}
	switch signal.Signal {
	case ownerctx.SignalExplicitInstruction, ownerctx.SignalExplicitCorrection, ownerctx.SignalExplicitEndorsement:
	default:
		return errors.New("self_signal must be an explicit instruction, correction or endorsement")
	}
	if !validOwnerSelfFacet(signal.Facet) || !closedSelfSignalToken(signal.Claim) || len(signal.EvidenceType) < 1 || len(signal.EvidenceType) > 64 {
		return errors.New("self_signal facet, claim or evidence is invalid")
	}
	switch signal.EvidenceType {
	case "owner_instruction", "owner_correction", "owner_endorsement", "owner_preference_confirmation":
	default:
		return errors.New("self_signal evidence class is not allowed")
	}
	if signal.Confidence < 0 || signal.Confidence > 1 || signal.Sensitivity != "professional" && signal.Sensitivity != "sensitive" && signal.Sensitivity != "restricted" || !signal.OwnerConfirmed {
		return errors.New("self_signal confidence, sensitivity or confirmation is invalid")
	}
	if signal.Signal == ownerctx.SignalExplicitEndorsement && (strings.EqualFold(signal.Claim, "ok") || strings.EqualFold(signal.Claim, "okay")) {
		return errors.New("generic acknowledgement is not an explicit self endorsement")
	}
	return nil
}

func validOwnerSelfFacet(facet string) bool {
	switch facet {
	case "professional-role", "communication-style", "voice", "preferences", "motivations", "quality-bar", "decision-rules", "working-boundaries":
		return true
	default:
		return false
	}
}

func closedSelfSignalToken(value string) bool {
	if len(value) < 1 || len(value) > 128 {
		return false
	}
	for index := 0; index < len(value); index++ {
		char := value[index]
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '_' || char == '.' || char == ':' || char == '-') || index == 0 && char == '.' {
			return false
		}
	}
	return true
}

var maestroWalterFacetAllowlist = []string{"professional-role", "communication-style", "voice", "preferences", "motivations", "quality-bar", "decision-rules", "working-boundaries"}
var recordUserPromptFunc = ownerctx.RecordUserPrompt

func durableDispatchFence(root, ownerID, sessionID, dispatchID, promptDigest, packetDigest, draftDigest string, plan maestro.Plan, chain maestro.ChainState) (maestro.DispatchBoundaryState, error) {
	state, err := (maestro.DispatchBoundaryInput{Root: root, OwnerID: ownerID, SessionID: sessionID, DispatchID: dispatchID, PromptDigest: promptDigest, PacketDigest: packetDigest, DraftDigest: draftDigest, Plan: plan, Chain: chain}).PersistDispatchBoundary()
	if err != nil {
		return maestro.DispatchBoundaryState{}, err
	}
	return state, nil
}

func finalizeDurableDispatchFence(root, ownerID, sessionID, dispatchID, promptDigest, packetDigest, draftDigest string, plan maestro.Plan, chain maestro.ChainState, prepared maestro.DispatchBoundaryState) (maestro.DispatchBoundaryState, error) {
	return (maestro.DispatchBoundaryInput{Root: root, OwnerID: ownerID, SessionID: sessionID, DispatchID: dispatchID, PromptDigest: promptDigest, PacketDigest: packetDigest, DraftDigest: draftDigest, Plan: plan, Chain: chain}).FinalizeDispatchBoundary(prepared)
}

func mapPlannerObservationScope(scopeKind, scopeID string) (ownerctx.PromptScopeKind, string, error) {
	if scopeID == "" {
		return "", "", errors.New("planner observation scope ID is required")
	}
	switch scopeKind {
	case string(ownerctx.PromptScopeCase):
		return ownerctx.PromptScopeCase, scopeID, nil
	case string(ownerctx.PromptScopeAccount):
		return ownerctx.PromptScopeAccount, scopeID, nil
	case string(ownerctx.PromptScopeWorkspace):
		return ownerctx.PromptScopeWorkspace, scopeID, nil
	case "control", "practice", "review", "health", "errand":
		// These planner scopes are not owner self scopes. Project them into a
		// deterministic workspace metadata scope without granting authority.
		return ownerctx.PromptScopeWorkspace, "planner-" + maestro.SHA256Hex(scopeKind + "\x00" + scopeID)[:32], nil
	default:
		return "", "", fmt.Errorf("planner scope %q cannot be projected to an owner observation scope", scopeKind)
	}
}

func maestroDispatchObservation(request maestroDispatchRequest, plan maestro.Plan, packet maestro.IntentReviewPacket, scopeKind ownerctx.PromptScopeKind, scopeID string, now time.Time) ownerctx.ObservationInput {
	sensitivity := "professional"
	if request.Plan.Sensitivity == maestro.SensitivityConfidential {
		sensitivity = "sensitive"
	} else if request.Plan.Sensitivity == maestro.SensitivityRestricted {
		sensitivity = "restricted"
	}
	input := ownerctx.ObservationInput{
		SchemaVersion: 1, Signal: ownerctx.SignalObservedPattern, Claim: "task_activity", EvidenceType: "interaction_metadata",
		SourceEvent: "maestro.loop_completed", SourceDigest: maestro.SHA256Hex(packet.LiteralRequest), EpisodeID: request.DispatchID,
		ScopeKind: string(scopeKind), ScopeID: scopeID, Confidence: 1, Sensitivity: sensitivity,
		ExpiresAt: now.UTC().Add(30 * 24 * time.Hour), AuthenticatedOwner: request.AuthenticatedOwner,
		Material: false, OwnerConfirmed: false,
	}
	if request.SelfSignal != nil {
		input.Signal = request.SelfSignal.Signal
		input.Facet = request.SelfSignal.Facet
		input.Claim = request.SelfSignal.Claim
		input.EvidenceType = request.SelfSignal.EvidenceType
		input.Confidence = request.SelfSignal.Confidence
		input.Sensitivity = request.SelfSignal.Sensitivity
		input.Material = true
		input.OwnerConfirmed = request.SelfSignal.OwnerConfirmed
	}
	return input
}

func rollbackDispatch(root string, chain maestro.ChainState, chainCreated bool, promptID, reason string) error {
	var rollbackErr error
	if promptID != "" {
		if err := ownerctx.DeletePromptHistory(root, promptID, true); err != nil {
			rollbackErr = err
		}
	}
	if chainCreated {
		if err := maestro.RemoveChainState(root, chain); err != nil && rollbackErr == nil {
			rollbackErr = err
		}
	}
	if rollbackErr == nil {
		return nil
	}
	markerErr := maestro.PersistDispatchRecoveryMarker(root, maestro.DispatchRecoveryMarker{SchemaVersion: 1, PlanDigest: chain.PlanDigest, ArtifactKind: maestro.RecoveryArtifactChainState, TargetRef: filepath.Join(root, "owner", "maestro", "chains", chain.PlanDigest+".json"), Reason: reason + ": " + rollbackErr.Error()})
	if markerErr != nil {
		return fmt.Errorf("dispatch rollback failed: %v; recovery marker failed: %w", rollbackErr, markerErr)
	}
	return fmt.Errorf("dispatch rollback failed; recovery marker recorded: %w", rollbackErr)
}

func runMaestroWithInput(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) >= 1 && args[0] == "status" {
		path, code := oneOptionalPath("maestro status", args[1:], errOut)
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
		owner, err := ownerctx.Inspect(root)
		if err != nil {
			return reportError(errOut, err)
		}
		status, _, err := buildContinuousUseStatus(root, inspection, owner)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, status, errOut)
	}
	if len(args) != 2 || args[0] != "dispatch" || args[1] != "--stdin" {
		fmt.Fprintln(errOut, "usage: bcgos maestro <status [workspace-path]|dispatch --stdin>")
		return ExitUsage
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	body, err := io.ReadAll(io.LimitReader(in, maximumWorkContractBytes+1))
	if err != nil || len(body) > maximumWorkContractBytes {
		return reportError(errOut, errors.New("Maestro dispatch request exceeds 32 KiB limit"))
	}
	var request maestroDispatchRequest
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&request); err != nil {
		return reportError(errOut, errors.New("Maestro dispatch request is not a closed JSON contract"))
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return reportError(errOut, errors.New("Maestro dispatch request must contain exactly one JSON value"))
	}
	if !request.AuthenticatedOwner || strings.TrimSpace(request.OwnerID) == "" || strings.TrimSpace(request.Prompt) == "" {
		return reportError(errOut, errors.New("Maestro dispatch requires a fresh owner attestation and prompt"))
	}
	if request.DispatchID != "" && request.OccurrenceID != "" && request.DispatchID != request.OccurrenceID {
		return reportError(errOut, errors.New("dispatch_id and occurrence_id must match when both are supplied"))
	}
	if request.DispatchID == "" {
		request.DispatchID = request.OccurrenceID
	}
	if request.DispatchID == "" {
		return reportError(errOut, errors.New("Maestro dispatch requires a caller-generated dispatch_id"))
	}
	if request.Language == "" {
		request.Language = "en"
	}
	if request.Source == "" {
		request.Source = "cli"
	}
	if request.SessionID == "" {
		request.SessionID = "maestro-dispatch"
	}
	if request.WorkingLanguage == "" {
		request.WorkingLanguage = request.Language
	}
	if request.CurrentLanguage == "" {
		request.CurrentLanguage = request.Language
	}
	if request.DraftOutput == "" {
		request.DraftOutput = "model_execution_unavailable"
	}
	if request.Audience == "" {
		request.Audience = "owner"
	}
	if request.Consequence == "" {
		request.Consequence = "low"
	}
	if request.Reversibility == "" {
		request.Reversibility = "reversible"
	}
	plan, err := maestro.PlanFor(request.Plan)
	if err != nil {
		return reportError(errOut, err)
	}
	if err := validateMaestroSelfSignal(request.SelfSignal); err != nil {
		return reportError(errOut, err)
	}
	observationScopeKind, observationScopeID, err := mapPlannerObservationScope(request.Plan.ScopeKind, request.Plan.ScopeID)
	if err != nil {
		return reportError(errOut, err)
	}
	chain, err := maestro.NewChain(plan, maestro.DefaultLoopPolicy)
	if err != nil {
		return reportError(errOut, err)
	}
	// Initialization is the only local bootstrap mutation. No prompt or
	// durable chain is written until plan, snapshot and sealed packet checks
	// have all succeeded.
	if _, err := ownerctx.Initialize(root); err != nil {
		return reportError(errOut, err)
	}
	occurrenceAlreadyRecorded, err := ownerctx.PromptHistoryOccurrenceExists(root, request.OwnerID, request.DispatchID)
	if err != nil {
		return reportError(errOut, err)
	}
	excludeOccurrenceID := ""
	if occurrenceAlreadyRecorded {
		excludeOccurrenceID = request.DispatchID
	}
	snapshot, err := ownerctx.ProjectSnapshot(root, append([]string(nil), maestroWalterFacetAllowlist...))
	if err != nil {
		return reportError(errOut, err)
	}
	packet, err := maestro.BuildIntentReviewPacketWithPromptHistory(request.Prompt, plan, request.DraftOutput, nil, snapshot, nil, request.Audience, request.Consequence, request.Reversibility, "", root, ownerctx.PromptHistorySelectionLimits{OwnerID: request.OwnerID, ExcludeOccurrenceID: excludeOccurrenceID, MaxCount: 8, MaxBytes: 32 << 10, MaxAge: 30 * 24 * time.Hour, ScopeKind: observationScopeKind, ScopeID: observationScopeID, CurrentPrompt: request.Prompt, RelevanceKeys: request.RelevanceKeys, CurrentLanguage: request.CurrentLanguage}, request.WorkingLanguage, nil, time.Now().UTC())
	if err != nil {
		return reportError(errOut, err)
	}
	if err := packet.Validate(); err != nil {
		return reportError(errOut, err)
	}
	chainPath := filepath.Join(root, "owner", "maestro", "chains", chain.PlanDigest+".json")
	_, chainStatErr := os.Lstat(chainPath)
	chainExisted := chainStatErr == nil
	if _, err := maestro.PersistChainState(root, chain); err != nil {
		return reportError(errOut, err)
	}
	promptDigest := maestro.SHA256Hex(packet.LiteralRequest)
	draftDigest := maestro.SHA256Hex(packet.DraftOutput)
	durableState, err := durableDispatchFence(root, request.OwnerID, request.SessionID, request.DispatchID, promptDigest, packet.PacketDigest, draftDigest, plan, chain)
	if err != nil {
		if rollbackErr := rollbackDispatch(root, chain, !chainExisted, "", "durable dispatch fence failed"); rollbackErr != nil {
			return reportError(errOut, fmt.Errorf("%w; %v", err, rollbackErr))
		}
		return reportError(errOut, err)
	}
	promptReceipt, err := recordUserPromptFunc(root, ownerctx.PromptHistoryInput{OwnerID: request.OwnerID, OccurrenceID: request.DispatchID, Prompt: request.Prompt, Language: request.Language, Source: request.Source, SessionID: request.SessionID, ScopeKind: observationScopeKind, ScopeID: observationScopeID, ContentKind: "user_prompt"})
	if err != nil {
		if rollbackErr := rollbackDispatch(root, chain, !chainExisted, "", "prompt commit failed"); rollbackErr != nil {
			return reportError(errOut, fmt.Errorf("%w; %v", err, rollbackErr))
		}
		return reportError(errOut, err)
	}
	observationReceipt, _, err := ownerctx.AppendObservation(root, maestroDispatchObservation(request, plan, packet, observationScopeKind, observationScopeID, durableState.StartedAt))
	if err != nil {
		promptID := ""
		if !occurrenceAlreadyRecorded {
			promptID = promptReceipt.ID
		}
		if rollbackErr := rollbackDispatch(root, chain, !chainExisted, promptID, "interaction observation commit failed"); rollbackErr != nil {
			return reportError(errOut, fmt.Errorf("%w; %v", err, rollbackErr))
		}
		return reportError(errOut, err)
	}
	_ = observationReceipt
	durableState, err = finalizeDurableDispatchFence(root, request.OwnerID, request.SessionID, request.DispatchID, promptDigest, packet.PacketDigest, draftDigest, plan, chain, durableState)
	if err != nil {
		if maestro.DispatchBoundaryStateUnknown(err) {
			return reportError(errOut, err)
		}
		promptID := ""
		if !occurrenceAlreadyRecorded {
			promptID = promptReceipt.ID
		}
		if rollbackErr := rollbackDispatch(root, chain, !chainExisted, promptID, "dispatch finalization failed"); rollbackErr != nil {
			return reportError(errOut, fmt.Errorf("%w; %v", err, rollbackErr))
		}
		return reportError(errOut, err)
	}
	chainBody, _ := json.Marshal(chain)
	receipt := maestro.DispatchBoundaryReceipt{SchemaVersion: 1, PlanDigest: plan.PlanDigest, ChainDigest: maestro.SHA256Hex(string(chainBody)), PacketDigest: packet.PacketDigest, PromptDigest: promptDigest, DraftDigest: draftDigest, AccountConsultation: plan.AccountConsultationRequired, WalterRequired: plan.RequiresWalter, HistoryCount: len(packet.PriorPrompts), DispatchID: durableState.DispatchID, DurableDispatchEpoch: durableState.Epoch, BindingChainDigest: durableState.BindingChainDigest, State: chain.Stage, Outcome: "dispatch_boundary_model_unavailable"}
	return writeJSON(out, receipt, errOut)
}

func runOwner(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	return runOwnerWithInput(args, strings.NewReader(""), out, errOut, dataRoot)
}

func runOwnerWithInput(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos owner <init|status|interview [quick|complete]|onboarding|expand|refine|self|prompt-history>")
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
		if len(args) > 2 {
			fmt.Fprintln(errOut, "usage: bcgos owner interview [quick|complete]")
			return ExitUsage
		}
		if len(args) == 2 && args[1] == ownerctx.OnboardingTrackQuick {
			return writeJSON(out, ownerctx.QuickStartInterview(), errOut)
		}
		if len(args) == 1 || args[1] == ownerctx.OnboardingTrackComplete {
			return writeJSON(out, ownerctx.ColdStartInterview(), errOut)
		}
		fmt.Fprintln(errOut, "usage: bcgos owner interview [quick|complete]")
		return ExitUsage
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
	case "onboarding":
		return runOwnerOnboarding(args[1:], in, out, errOut, root)
	case "expand":
		return runOwnerExpand(args[1:], in, out, errOut, root)
	case "refine":
		return runOwnerRefine(args[1:], in, out, errOut, root)
	case "self":
		return runOwnerSelf(args[1:], in, out, errOut, root)
	case "prompt-history":
		return runOwnerPromptHistory(args[1:], in, out, errOut, root)
	default:
		fmt.Fprintln(errOut, "usage: bcgos owner <init|status|interview [quick|complete]|onboarding|expand|refine|self|prompt-history>")
		return ExitUsage
	}
}

func runOwnerExpand(args []string, in io.Reader, out, errOut io.Writer, root string) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos owner expand <status|next|draft|review|confirm>")
		return ExitUsage
	}
	switch args[0] {
	case "status":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos owner expand status")
			return ExitUsage
		}
		status, err := ownerctx.Inspect(root)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, status.Expansion, errOut)
	case "next":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos owner expand next")
			return ExitUsage
		}
		question, err := ownerctx.NextExpansionQuestion(root)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, question, errOut)
	case "draft":
		flags := newFlagSet("owner expand draft", errOut)
		token := flags.String("question-token", "", "digest-bound token returned by owner expand next")
		stdin := flags.Bool("stdin", false, "read the proposed canonical facet body from stdin")
		consent := flags.Bool("consent", false, "record explicit owner consent for this interview answer")
		noClientData := flags.Bool("no-client-data", false, "owner attests that the answer contains no client data")
		if flags.Parse(args[1:]) != nil || rejectPositionals(flags, errOut) || !*stdin || !*consent || !*noClientData || strings.TrimSpace(*token) == "" {
			fmt.Fprintln(errOut, "usage: bcgos owner expand draft --question-token SHA256 --stdin --consent --no-client-data")
			return ExitUsage
		}
		body, err := io.ReadAll(io.LimitReader(in, maximumWorkContractBytes+1))
		if err != nil || len(body) > maximumWorkContractBytes {
			return reportError(errOut, errors.New("SELF expansion draft exceeds 32 KiB"))
		}
		draft, err := ownerctx.DraftExpansion(root, *token, string(body), true, true)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, draft, errOut)
	case "review":
		flags := newFlagSet("owner expand review", errOut)
		id := flags.String("id", "", "draft ID")
		if flags.Parse(args[1:]) != nil || rejectPositionals(flags, errOut) || *id == "" {
			fmt.Fprintln(errOut, "usage: bcgos owner expand review --id ID")
			return ExitUsage
		}
		draft, err := ownerctx.ReviewExpansion(root, *id)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, draft, errOut)
	case "confirm":
		flags := newFlagSet("owner expand confirm", errOut)
		id := flags.String("id", "", "draft ID")
		digest := flags.String("digest", "", "review digest")
		confirm := flags.Bool("confirm", false, "confirm the exact reviewed draft")
		if flags.Parse(args[1:]) != nil || rejectPositionals(flags, errOut) || *id == "" || *digest == "" || !*confirm {
			fmt.Fprintln(errOut, "usage: bcgos owner expand confirm --id ID --digest SHA256 --confirm")
			return ExitUsage
		}
		draft, err := ownerctx.ConfirmExpansion(root, *id, *digest, true)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, draft, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos owner expand <status|next|draft|review|confirm>")
		return ExitUsage
	}
}

func runOwnerOnboarding(args []string, in io.Reader, out, errOut io.Writer, root string) int {
	if len(args) == 0 || (args[0] != "status" && args[0] != "review" && args[0] != "select" && args[0] != "answer" && args[0] != "confirm") {
		fmt.Fprintln(errOut, "usage: bcgos owner onboarding <status|review|select --track quick|complete --confirm|answer --facet ID (--body TEXT|--stdin) --confirm|confirm --digest SHA256 --confirm>")
		return ExitUsage
	}
	switch args[0] {
	case "status", "review":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos owner onboarding status")
			return ExitUsage
		}
		status, err := ownerctx.Inspect(root)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, status.Onboarding, errOut)
	case "select":
		flags := newFlagSet("owner onboarding select", errOut)
		track := flags.String("track", "", "onboarding track: quick or complete")
		confirmed := flags.Bool("confirm", false, "record the owner's selected onboarding track")
		if flags.Parse(args[1:]) != nil || rejectPositionals(flags, errOut) || !*confirmed || strings.TrimSpace(*track) == "" {
			fmt.Fprintln(errOut, "usage: bcgos owner onboarding select --track quick|complete --confirm")
			return ExitUsage
		}
		status, err := ownerctx.SelectOnboardingTrack(root, *track)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, status.Onboarding, errOut)
	case "answer":
		flags := newFlagSet("owner onboarding answer", errOut)
		facet := flags.String("facet", "", "owner facet being answered")
		evidence := flags.String("evidence", "owner onboarding answer", "short provenance summary")
		body := flags.String("body", "", "concise reviewed Markdown body")
		stdin := flags.Bool("stdin", false, "read the concise reviewed Markdown body from standard input")
		confirmed := flags.Bool("confirm", false, "confirm that the owner approved this facet body")
		if flags.Parse(args[1:]) != nil || rejectPositionals(flags, errOut) || strings.TrimSpace(*facet) == "" || !*confirmed || (*stdin && strings.TrimSpace(*body) != "") || (!*stdin && strings.TrimSpace(*body) == "") {
			fmt.Fprintln(errOut, "usage: bcgos owner onboarding answer --facet ID (--body TEXT|--stdin) --confirm")
			return ExitUsage
		}
		proposedBody := *body
		if *stdin {
			readBody, err := io.ReadAll(io.LimitReader(in, maximumOwnerFacetBytes+1))
			if err != nil || len(readBody) > maximumOwnerFacetBytes {
				return reportError(errOut, errors.New("owner onboarding answer exceeds 1 MiB"))
			}
			proposedBody = string(readBody)
		}
		if strings.TrimSpace(proposedBody) == "" {
			return reportError(errOut, errors.New("owner onboarding answer body is required"))
		}
		proposal, err := ownerctx.SubmitRefinement(root, ownerctx.RefinementInput{Facet: *facet, Evidence: *evidence, ProposedBody: proposedBody})
		if err != nil {
			return reportError(errOut, err)
		}
		if _, err := ownerctx.ApplyRefinement(root, proposal.ID, true); err != nil {
			return reportError(errOut, err)
		}
		status, err := ownerctx.Inspect(root)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, status.Onboarding, errOut)
	case "confirm":
		flags := newFlagSet("owner onboarding confirm", errOut)
		reviewDigest := flags.String("digest", "", "SHA-256 digest shown by owner onboarding status")
		confirmed := flags.Bool("confirm", false, "confirm the reviewed onboarding profile")
		if flags.Parse(args[1:]) != nil || rejectPositionals(flags, errOut) || !*confirmed || strings.TrimSpace(*reviewDigest) == "" {
			fmt.Fprintln(errOut, "usage: bcgos owner onboarding confirm --digest SHA256 --confirm")
			if *confirmed && strings.TrimSpace(*reviewDigest) == "" {
				fmt.Fprintln(errOut, "run bcgos owner onboarding status and pass its review_digest; the digest prevents confirming a profile that changed after review")
			}
			return ExitUsage
		}
		status, err := ownerctx.ConfirmOnboarding(root, *reviewDigest)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, status.Onboarding, errOut)
	}
	return ExitUsage
}

func isReadOnlyInstalledBCGOSDiagnostic(toolName string, raw json.RawMessage) bool {
	executable, err := os.Executable()
	return err == nil && actionconfirmation.IsReadOnlyBCGOSDiagnostic(toolName, raw, executable)
}

func isReadOnlyBoundedDiagnostic(toolName string, raw json.RawMessage) bool {
	return isReadOnlyInstalledBCGOSDiagnostic(toolName, raw) || actionconfirmation.IsReadOnlyBoundedDiagnostic(toolName, raw)
}

func runOwnerPromptHistory(args []string, in io.Reader, out, errOut io.Writer, root string) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos owner prompt-history <config|add|inspect|export|delete|reset>")
		return ExitUsage
	}
	switch args[0] {
	case "config":
		flags := newFlagSet("owner prompt-history config", errOut)
		maxEntries := flags.Int("max-entries", 0, "maximum retained user prompts")
		maxBytes := flags.Int("max-bytes", 0, "maximum retained prompt bytes")
		maxAgeDays := flags.Int("max-age-days", 0, "maximum prompt age in days")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			fmt.Fprintln(errOut, "usage: bcgos owner prompt-history config [--max-entries N --max-bytes N --max-age-days N]")
			return ExitUsage
		}
		config, err := ownerctx.LoadPromptHistoryConfig(root)
		if err != nil {
			return reportError(errOut, err)
		}
		if *maxEntries != 0 {
			config.MaxEntries = *maxEntries
		}
		if *maxBytes != 0 {
			config.MaxBytes = *maxBytes
		}
		if *maxAgeDays != 0 {
			config.MaxAgeSeconds = int64(*maxAgeDays) * 24 * 60 * 60
		}
		if *maxEntries != 0 || *maxBytes != 0 || *maxAgeDays != 0 {
			if err := ownerctx.ConfigurePromptHistory(root, config); err != nil {
				return reportError(errOut, err)
			}
		}
		return writeJSON(out, config, errOut)
	case "add":
		flags := newFlagSet("owner prompt-history add", errOut)
		ownerID := flags.String("owner-id", "owner", "owner identity")
		scopeKind := flags.String("scope-kind", "", "global, workspace, account or case")
		scopeID := flags.String("scope-id", "", "opaque scope ID")
		language := flags.String("language", "", "prompt language, for example pt-BR")
		source := flags.String("source", "owner", "claude, codex, cli or owner")
		sessionID := flags.String("session-id", "", "opaque source session ID")
		stdin := flags.Bool("stdin", false, "read only the user prompt body from stdin")
		confirm := flags.Bool("confirm", false, "confirm local user-prompt retention")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || !*stdin || !*confirm || *scopeKind == "" || *scopeID == "" || *language == "" || *sessionID == "" {
			fmt.Fprintln(errOut, "usage: bcgos owner prompt-history add --scope-kind K --scope-id ID --language LANG --session-id ID --stdin --confirm")
			return ExitUsage
		}
		body, err := io.ReadAll(io.LimitReader(in, 64<<10+1))
		if err != nil {
			return reportError(errOut, err)
		}
		if len(body) > 64<<10 {
			fmt.Fprintln(errOut, "user prompt exceeds 64 KiB")
			return ExitUsage
		}
		receipt, err := ownerctx.RecordUserPrompt(root, ownerctx.PromptHistoryInput{OwnerID: *ownerID, Prompt: string(body), Language: *language, Source: *source, SessionID: *sessionID, ScopeKind: ownerctx.PromptScopeKind(*scopeKind), ScopeID: *scopeID, ContentKind: "user_prompt"})
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, receipt, errOut)
	case "inspect":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos owner prompt-history inspect")
			return ExitUsage
		}
		value, err := ownerctx.InspectPromptHistory(root)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, value, errOut)
	case "export":
		if len(args) != 2 || args[1] != "--confirm" {
			fmt.Fprintln(errOut, "usage: bcgos owner prompt-history export --confirm")
			return ExitUsage
		}
		value, err := ownerctx.ExportPromptHistory(root)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, value, errOut)
	case "delete":
		if len(args) != 3 || args[2] != "--confirm" {
			fmt.Fprintln(errOut, "usage: bcgos owner prompt-history delete ID --confirm")
			return ExitUsage
		}
		if err := ownerctx.DeletePromptHistory(root, args[1], true); err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, map[string]string{"deleted": args[1]}, errOut)
	case "reset":
		if len(args) != 2 || args[1] != "--confirm" {
			fmt.Fprintln(errOut, "usage: bcgos owner prompt-history reset --confirm")
			return ExitUsage
		}
		if err := ownerctx.ResetPromptHistory(root, true); err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, map[string]string{"reset": "prompt_history"}, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos owner prompt-history <config|add|inspect|export|delete|reset>")
		return ExitUsage
	}
}

func runOwnerSelf(args []string, in io.Reader, out, errOut io.Writer, root string) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos owner self <snapshot|observations|observe|observation|reset>")
		return ExitUsage
	}
	switch args[0] {
	case "snapshot":
		if len(args) >= 2 && args[1] == "delete" {
			if len(args) != 4 || args[3] != "--confirm" {
				fmt.Fprintln(errOut, "usage: bcgos owner self snapshot delete <version> --confirm")
				return ExitUsage
			}
			if err := ownerctx.DeleteSnapshot(root, args[2], true); err != nil {
				return reportError(errOut, err)
			}
			return writeJSON(out, map[string]string{"deleted": args[2]}, errOut)
		}
		flags := newFlagSet("owner self snapshot", errOut)
		export := flags.Bool("export", false, "include bounded facet content in this local export")
		facets := flags.String("facets", "", "comma-separated facet IDs; defaults to all registered facets")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
			fmt.Fprintln(errOut, "usage: bcgos owner self snapshot [--facets id,id] [--export]")
			return ExitUsage
		}
		var requested []string
		if strings.TrimSpace(*facets) != "" {
			requested = splitTracks(*facets)
		}
		snapshot, err := ownerctx.ProjectSnapshot(root, requested)
		if err != nil {
			return reportError(errOut, err)
		}
		if err := ownerctx.PersistSnapshot(root, snapshot); err != nil {
			return reportError(errOut, err)
		}
		if !*export {
			for id, facet := range snapshot.Facets {
				facet.Content = ""
				snapshot.Facets[id] = facet
			}
		}
		return writeJSON(out, snapshot, errOut)
	case "observations":
		if len(args) != 1 {
			fmt.Fprintln(errOut, "usage: bcgos owner self observations")
			return ExitUsage
		}
		observations, err := ownerctx.ListObservations(root)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, observations, errOut)
	case "observe":
		flags := newFlagSet("owner self observe", errOut)
		stdin := flags.Bool("stdin", false, "read a metadata-only observation JSON object")
		confirm := flags.Bool("confirm", false, "confirm that this is an owner-attested signal under the local data-root boundary")
		if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || !*stdin || !*confirm {
			fmt.Fprintln(errOut, "usage: bcgos owner self observe --stdin --confirm")
			return ExitUsage
		}
		var input ownerctx.ObservationInput
		decoder := json.NewDecoder(io.LimitReader(in, 16<<10))
		if err := decoder.Decode(&input); err != nil {
			return reportError(errOut, err)
		}
		input.AuthenticatedOwner = true
		input.OwnerConfirmed = true
		receipt, evaluation, err := ownerctx.AppendObservation(root, input)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, struct {
			Receipt    ownerctx.ObservationReceipt    `json:"receipt"`
			Evaluation ownerctx.InteractionEvaluation `json:"evaluation"`
		}{receipt, evaluation}, errOut)
	case "observation":
		if len(args) < 3 {
			fmt.Fprintln(errOut, "usage: bcgos owner self observation <reject|contradict|expire|redact> <id> --transition-id ID --expected-state STATE --expected-revision DIGEST --confirm")
			return ExitUsage
		}
		stateByName := map[string]ownerctx.ObservationState{"reject": ownerctx.ObservationRejected, "contradict": ownerctx.ObservationContradicted, "expire": ownerctx.ObservationExpired, "redact": ownerctx.ObservationRedacted}
		state, ok := stateByName[args[1]]
		flags := newFlagSet("owner self observation", errOut)
		transitionID := flags.String("transition-id", "", "caller-generated transition occurrence")
		expectedState := flags.String("expected-state", "", "current observation state")
		expectedRevision := flags.String("expected-revision", "", "current observation revision")
		confirm := flags.Bool("confirm", false, "confirm owner action")
		if !ok || args[2] == "" || len(args) < 4 || flags.Parse(args[3:]) != nil || rejectPositionals(flags, errOut) || *transitionID == "" || *expectedState == "" || *expectedRevision == "" || !*confirm {
			fmt.Fprintln(errOut, "usage: bcgos owner self observation <reject|contradict|expire|redact> <id> --transition-id ID --expected-state STATE --expected-revision DIGEST --confirm")
			return ExitUsage
		}
		receipt, err := ownerctx.RejectObservation(root, ownerctx.ObservationTransitionInput{ObservationID: args[2], TransitionID: *transitionID, Next: state, ExpectedState: ownerctx.ObservationState(*expectedState), ExpectedRevision: *expectedRevision, OwnerAction: true})
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, receipt, errOut)
	case "reset":
		if len(args) != 2 || args[1] != "--confirm" {
			fmt.Fprintln(errOut, "usage: bcgos owner self reset --confirm")
			return ExitUsage
		}
		if err := ownerctx.ResetDerivedSelf(root, true); err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, map[string]string{"reset": "derived_self"}, errOut)
	default:
		fmt.Fprintln(errOut, "usage: bcgos owner self <snapshot|observations|observe|observation|reset>")
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
	sharePointSource, err := priorWorkSourceStatus(root, inspection.WorkspaceID)
	if err != nil {
		return reportError(errOut, fmt.Errorf("inspect guided SharePoint source selection: %w", err))
	}
	continuous, activeExecution, err := buildContinuousUseStatus(root, inspection, owner)
	if err != nil {
		return reportError(errOut, fmt.Errorf("build continuous-use status: %w", err))
	}
	packet := sessionctx.Build(sessionctx.Sources{
		Profile: profileState, Workspace: inspection, Owner: owner, OwnerContextRoot: root,
		Atlas:            atlas.Inspect(atlas.Options{DataRoot: root, WorkspacePath: inspection.WorkspacePath, WorkspaceID: inspection.WorkspaceID}),
		Execution:        activePointerFromContinuity(activeExecution),
		Memory:           sessionMemorySource(root, inspection.WorkspaceID),
		SharePointSource: sharePointSource,
		ContinuousUse:    continuous,
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
	sharePointSource, err := priorWorkSourceStatus(root, inspection.WorkspaceID)
	if err != nil {
		return reportError(errOut, fmt.Errorf("inspect guided SharePoint source selection: %w", err))
	}
	continuous, activeExecution, err := buildContinuousUseStatus(root, inspection, owner)
	if err != nil {
		return reportError(errOut, fmt.Errorf("build continuous-use status: %w", err))
	}
	packet := sessionctx.Build(sessionctx.Sources{Profile: profileState, Workspace: inspection, Owner: owner, OwnerContextRoot: root, Atlas: atlas.Inspect(atlas.Options{DataRoot: root, WorkspacePath: inspection.WorkspacePath, WorkspaceID: inspection.WorkspaceID}), Execution: activePointerFromContinuity(activeExecution), Memory: sessionMemorySource(root, inspection.WorkspaceID), SharePointSource: sharePointSource, ContinuousUse: continuous})
	result, err := sessionresolve.Resolve(root, *pointer, *purpose, packet, *budget)
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, result, errOut)
}

func runIngest(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	flags := newFlagSet("ingest", errOut)
	workspacePath := flags.String("workspace", "", "initialized workspace path")
	sourcePath := flags.String("source", "", "local source file")
	adapterName := flags.String("adapter", "markitdown", "local adapter: markitdown")
	if err := flags.Parse(args); err != nil || rejectPositionals(flags, errOut) {
		fmt.Fprintln(errOut, "usage: bcgos ingest --workspace PATH --source PATH [--adapter markitdown]")
		return ExitUsage
	}
	if strings.TrimSpace(*workspacePath) == "" || strings.TrimSpace(*sourcePath) == "" {
		fmt.Fprintln(errOut, "usage: bcgos ingest --workspace PATH --source PATH [--adapter markitdown]")
		return ExitUsage
	}
	if *adapterName != "markitdown" {
		fmt.Fprintln(errOut, "only the markitdown local adapter is available in this release")
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
	if inspection.State != "ready" && inspection.State != "warning" {
		fmt.Fprintln(errOut, "workspace must be initialized and readable before ingestion; run bcgos init <workspace-path>")
		return ExitUsage
	}
	policy := ingest.DefaultPolicy()
	// Docling remains the primary substrate. Until its managed runtime pack is
	// installed, the core makes the explicit unavailable -> fallback decision.
	decision, err := ingest.SelectFallback(ingest.PrimaryUnavailable, *adapterName)
	if err != nil {
		return reportError(errOut, err)
	}
	// The signed pack verifier is injected by the future managed installer.
	// Keeping it nil here is intentional: an unsigned or locally forged pack
	// must not become executable merely because its files exist.
	pack, err := markitdown.ResolvePack(root, nil)
	if err != nil {
		return reportError(errOut, err)
	}
	result, conversionErr := (markitdown.Adapter{
		Command:      pack.Command,
		ArtifactRoot: filepath.Join(root, "ingestion", "artifacts"),
		WorkspaceID:  inspection.WorkspaceID,
		Route:        decision.Route,
		Policy:       policy,
	}).Convert(context.Background(), ingest.Request{
		SourcePath:    *sourcePath,
		WorkspacePath: inspection.WorkspacePath,
		Policy:        policy,
	})
	if pack.State != "ready" {
		result.Warnings = append(result.Warnings, pack.Reason)
	}
	if conversionErr != nil && result.Status == ingest.StatusBlocked {
		_ = writeJSON(out, result, errOut)
		return ExitUsage
	}
	if code := writeJSON(out, result, errOut); code != ExitOK {
		return code
	}
	if result.Status == ingest.StatusUnavailable {
		return ExitUnavailable
	}
	if result.Status == ingest.StatusBlocked {
		return ExitUsage
	}
	return ExitOK
}

func runHook(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	return runHookWithInput(args, strings.NewReader(""), out, errOut, dataRoot)
}

func resolveHookOrchestrationState(inspection workspace.Inspection, pointer string) (hookOrchestrationState, error) {
	pointer = strings.TrimSpace(pointer)
	if pointer == "" {
		return hookOrchestrationState{}, nil
	}
	if pointer != installedOrchestrationStatePath || filepath.IsAbs(pointer) || filepath.Clean(pointer) != pointer {
		return hookOrchestrationState{}, errors.New("orchestration state must use the installed workspace-local pointer " + installedOrchestrationStatePath)
	}
	workspaceRoot, err := filepath.Abs(filepath.Clean(inspection.WorkspacePath))
	if err != nil {
		return hookOrchestrationState{}, fmt.Errorf("resolve orchestration workspace: %w", err)
	}
	workspaceInfo, err := os.Lstat(workspaceRoot)
	if err != nil {
		return hookOrchestrationState{}, fmt.Errorf("inspect orchestration workspace: %w", err)
	}
	if workspaceInfo.Mode()&os.ModeSymlink != 0 || !workspaceInfo.IsDir() {
		return hookOrchestrationState{}, errors.New("orchestration workspace must be a non-symlink directory")
	}
	physicalWorkspace, err := filepath.EvalSymlinks(workspaceRoot)
	if err != nil || !samePathCLI(workspaceRoot, physicalWorkspace) {
		return hookOrchestrationState{}, errors.New("orchestration workspace path cannot traverse symlinked components")
	}
	statePath := filepath.Join(workspaceRoot, pointer)
	relative, err := filepath.Rel(workspaceRoot, statePath)
	if err != nil || relative != pointer {
		return hookOrchestrationState{}, errors.New("orchestration state escaped its canonical workspace")
	}
	metadataRoot := filepath.Dir(statePath)
	metadataInfo, err := os.Lstat(metadataRoot)
	if err != nil || metadataInfo.Mode()&os.ModeSymlink != 0 || !metadataInfo.IsDir() {
		return hookOrchestrationState{}, errors.New("orchestration state parent must be the regular workspace .bcgos directory")
	}
	physicalMetadata, err := filepath.EvalSymlinks(metadataRoot)
	if err != nil || !samePathCLI(metadataRoot, physicalMetadata) {
		return hookOrchestrationState{}, errors.New("orchestration state parent cannot traverse symlinked components")
	}
	stateInfo, statErr := os.Lstat(statePath)
	if statErr == nil {
		if stateInfo.Mode()&os.ModeSymlink != 0 || !stateInfo.Mode().IsRegular() {
			return hookOrchestrationState{}, errors.New("orchestration state must be a regular non-symlink file")
		}
		if stateInfo.Size() <= 0 || stateInfo.Size() > maximumOrchestrationStateBytes {
			return hookOrchestrationState{}, errors.New("orchestration state must be a non-empty bounded JSON file")
		}
		if stateInfo.Mode().Perm()&0o077 != 0 {
			return hookOrchestrationState{}, errors.New("orchestration state must be owner-only (0600 or stricter)")
		}
		file, openErr := os.Open(statePath)
		if openErr != nil {
			return hookOrchestrationState{}, fmt.Errorf("open orchestration state: %w", openErr)
		}
		body, readErr := io.ReadAll(io.LimitReader(file, maximumOrchestrationStateBytes+1))
		decodeErr := readErr
		if decodeErr == nil {
			if int64(len(body)) > maximumOrchestrationStateBytes {
				decodeErr = errors.New("orchestration state exceeds the bounded JSON limit")
			} else {
				_, decodeErr = agentorchestration.DecodeStateSnapshot(body)
			}
		}
		closeErr := file.Close()
		if decodeErr != nil {
			return hookOrchestrationState{}, fmt.Errorf("decode orchestration state: %w", decodeErr)
		}
		if closeErr != nil {
			return hookOrchestrationState{}, fmt.Errorf("close orchestration state: %w", closeErr)
		}
	} else if errors.Is(statErr, os.ErrNotExist) {
		return hookOrchestrationState{}, errors.New("orchestration state is missing; run bcgos init for this workspace before opening the runtime")
	} else {
		return hookOrchestrationState{}, fmt.Errorf("inspect orchestration state: %w", statErr)
	}
	store, err := agentorchestration.NewDurableStateStore(statePath, lifecycle.IdempotencyKey("hook-store-reader", inspection.WorkspaceID))
	if err != nil {
		return hookOrchestrationState{}, err
	}
	snapshotBody, err := json.Marshal(store.Snapshot())
	if err != nil {
		return hookOrchestrationState{}, fmt.Errorf("encode orchestration snapshot identity: %w", err)
	}
	return hookOrchestrationState{configured: true, digest: maestro.SHA256Hex(inspection.WorkspaceID + "\x00" + pointer + "\x00" + string(snapshotBody))}, nil
}

func orchestrationBoundKey(base string, state hookOrchestrationState) string {
	if !state.configured {
		return base
	}
	return lifecycle.IdempotencyKey(base, state.digest)
}

func signalSessionPresence(state hookOrchestrationState, workspaceID string) {
	if state.configured {
		_ = enqueueHookPresenceWake(workspaceID)
	}
}

func runHookWithInput(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) > 0 && args[0] == "claude" {
		return runClaudeHook(args[1:], in, out, errOut, dataRoot)
	}
	if len(args) > 0 && args[0] == "codex" {
		return runCodexHook(args[1:], in, out, errOut, dataRoot)
	}
	if len(args) == 0 || args[0] != "session-start" {
		fmt.Fprintln(errOut, "usage: bcgos hook session-start --runtime claude|codex [workspace-path]\n       bcgos hook <claude|codex> <session-start|context-injection|pre-action-guard|post-action-receipt|stop-finalization> [workspace-path]")
		return ExitUsage
	}
	flags := newFlagSet("hook session-start", errOut)
	runtimeName := flags.String("runtime", "", "target runtime: claude or codex")
	adapterSource := flags.String("adapter-source", "", "internal adapter ownership marker")
	orchestrationState := flags.String("orchestration-state", "", "shared Maestro orchestration state path")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() > 1 || (*adapterSource != "" && *adapterSource != "maestro") {
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
	state, err := resolveHookOrchestrationState(inspection, *orchestrationState)
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
	sharePointSource, err := priorWorkSourceStatus(root, inspection.WorkspaceID)
	if err != nil {
		return reportError(errOut, fmt.Errorf("inspect guided SharePoint source selection: %w", err))
	}
	continuous, activeExecution, err := buildContinuousUseStatus(root, inspection, owner)
	if err != nil {
		return reportError(errOut, fmt.Errorf("build continuous-use status: %w", err))
	}
	packet := sessionctx.Build(sessionctx.Sources{
		Profile: profileState, Workspace: inspection, Owner: owner, OwnerContextRoot: root,
		Atlas:            atlas.Inspect(atlas.Options{DataRoot: root, WorkspacePath: inspection.WorkspacePath, WorkspaceID: inspection.WorkspaceID}),
		Execution:        activePointerFromContinuity(activeExecution),
		Memory:           sessionMemorySource(root, inspection.WorkspaceID),
		SharePointSource: sharePointSource,
		ContinuousUse:    continuous,
	})
	if err := enrichOnboardingGuide(&packet, *runtimeName, inspection.WorkspacePath); err != nil {
		return reportError(errOut, err)
	}
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
	interactionKey := orchestrationBoundKey(lifecycle.IdempotencyKey(*runtimeName, string(lifecycle.SessionStart), inspection.WorkspaceID), state)
	if err := evaluateAdapterInteraction(root, *runtimeName, string(lifecycle.SessionStart), interactionKey, inspection.WorkspaceID); err != nil {
		return reportError(errOut, fmt.Errorf("evaluate adapter interaction: %w", err))
	}
	signalSessionPresence(state, inspection.WorkspaceID)
	return writeJSON(out, output, errOut)
}

func runCodexHook(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	const usage = "usage: bcgos hook codex <session-start|context-injection|pre-action-guard|post-action-receipt|stop-finalization> [workspace-path]"
	if len(args) == 0 {
		fmt.Fprintln(errOut, usage)
		return ExitUsage
	}
	action := args[0]
	flags := newFlagSet("hook codex "+action, errOut)
	adapterSource := flags.String("adapter-source", "", "internal adapter ownership marker")
	orchestrationState := flags.String("orchestration-state", "", "shared Maestro orchestration state path")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() > 1 || (*adapterSource != "" && *adapterSource != "maestro") {
		fmt.Fprintln(errOut, usage)
		return ExitUsage
	}
	switch action {
	case "session-start", "context-injection", "pre-action-guard", "post-action-receipt", "stop-finalization":
	default:
		fmt.Fprintln(errOut, usage)
		return ExitUsage
	}
	var native codexadapter.NativeInput
	if action == "context-injection" || action == "pre-action-guard" || action == "post-action-receipt" || action == "stop-finalization" {
		parsed, err := codexadapter.ParseReader(in)
		if err != nil {
			if action == "pre-action-guard" {
				return writeJSON(out, codexadapter.FailClosedDenial(), errOut)
			}
			return reportError(errOut, err)
		}
		native = parsed
	}
	if action == "pre-action-guard" {
		if isReadOnlyBoundedDiagnostic(native.ToolName, native.ToolInputJSON()) {
			return writeJSON(out, codexadapter.GuardOutput{}, errOut)
		}
		response, err := codexadapter.Guard(native)
		if err != nil {
			return writeJSON(out, codexadapter.FailClosedDenial(), errOut)
		}
		if response.HookSpecificOutput != nil {
			return writeJSON(out, response, errOut)
		}
		protected, canonicalErr := actionconfirmation.Canonicalize(native.ToolName, native.ToolInputJSON())
		if canonicalErr != nil {
			return writeJSON(out, codexadapter.ExternalActionDenial(noncanonicalExternalDenial), errOut)
		}
		if protected == nil && !actionconfirmation.LooksLikeBCGOSDiagnostic(native.ToolName, native.ToolInputJSON()) {
			return writeJSON(out, response, errOut)
		}
		root, inspection, inspectErr := inspectProtectedActionWorkspace(optionalArg(flags.Args()), dataRoot)
		if inspectErr != nil {
			if protected != nil {
				return writeJSON(out, codexadapter.ExternalActionDenial(unavailableConfirmationDenial), errOut)
			}
			return writeJSON(out, codexadapter.ExternalActionDenial(unverifiedDiagnosticDenial), errOut)
		}
		state, stateErr := resolveHookOrchestrationState(inspection, *orchestrationState)
		if stateErr != nil {
			if protected != nil {
				return writeJSON(out, codexadapter.ExternalActionDenial(unavailableConfirmationDenial), errOut)
			}
			return writeJSON(out, codexadapter.ExternalActionDenial(unverifiedDiagnosticDenial), errOut)
		}
		if protected != nil {
			actorID, actorErr := localConfirmedOwnerActor(root)
			if actorErr != nil {
				return writeJSON(out, codexadapter.ExternalActionDenial(unavailableConfirmationDenial), errOut)
			}
			result, authorizeErr := confirmationStore(root, inspection.WorkspaceID).Authorize(actionconfirmation.Binding{Runtime: "codex", WorkspaceID: inspection.WorkspaceID, ActorID: actorID, SessionID: native.SessionID, Action: *protected})
			if authorizeErr != nil {
				return writeJSON(out, codexadapter.ExternalActionDenial(unavailableConfirmationDenial), errOut)
			}
			if result.State != actionconfirmation.Authorized {
				return writeJSON(out, codexadapter.ExternalActionDenial(challengeDenial(result)), errOut)
			}
		}
		interactionKey := orchestrationBoundKey(lifecycle.IdempotencyKey("codex", string(lifecycle.PreActionGuard), inspection.WorkspaceID, native.ToolName, native.ToolUseID), state)
		if err := evaluateAdapterInteraction(root, "codex", string(lifecycle.PreActionGuard), interactionKey, inspection.WorkspaceID); err != nil {
			return writeJSON(out, codexadapter.FailClosedDenial(), errOut)
		}
		return writeJSON(out, response, errOut)
	}
	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	inspection, err := workspace.Inspect(optionalArg(flags.Args()), root)
	if err != nil {
		return reportError(errOut, err)
	}
	state, err := resolveHookOrchestrationState(inspection, *orchestrationState)
	if err != nil {
		return reportError(errOut, err)
	}
	if action == "session-start" || action == "context-injection" {
		profileState, err := resolveProfile(root, "", false)
		if err != nil {
			return reportError(errOut, err)
		}
		owner, err := ownerctx.Inspect(root)
		if err != nil {
			return reportError(errOut, err)
		}
		sharePointSource, err := priorWorkSourceStatus(root, inspection.WorkspaceID)
		if err != nil {
			return reportError(errOut, fmt.Errorf("inspect guided SharePoint source selection: %w", err))
		}
		continuous, activeExecution, err := buildContinuousUseStatus(root, inspection, owner)
		if err != nil {
			return reportError(errOut, fmt.Errorf("build continuous-use status: %w", err))
		}
		packet := sessionctx.Build(sessionctx.Sources{
			Profile: profileState, Workspace: inspection, Owner: owner, OwnerContextRoot: root,
			Atlas:            atlas.Inspect(atlas.Options{DataRoot: root, WorkspacePath: inspection.WorkspacePath, WorkspaceID: inspection.WorkspaceID}),
			Execution:        activePointerFromContinuity(activeExecution),
			Memory:           sessionMemorySource(root, inspection.WorkspaceID),
			SharePointSource: sharePointSource,
			ContinuousUse:    continuous,
		})
		if action == "context-injection" {
			if err := enrichContextPacket(&packet, "codex", inspection.WorkspacePath, root, native.SessionID, native.Prompt); err != nil {
				return reportError(errOut, err)
			}
		} else if err := enrichOnboardingGuide(&packet, "codex", inspection.WorkspacePath); err != nil {
			return reportError(errOut, err)
		}
		eventName := "SessionStart"
		if action == "context-injection" {
			eventName = "UserPromptSubmit"
		}
		response, err := sessionhook.BuildCodexEvent(packet, eventName)
		if err != nil {
			return reportError(errOut, err)
		}
		event := lifecycle.SessionStart
		if action == "context-injection" {
			event = lifecycle.ContextInject
		}
		interactionKey := orchestrationBoundKey(lifecycle.IdempotencyKey("codex", event, inspection.WorkspaceID), state)
		if err := evaluateAdapterInteraction(root, "codex", event, interactionKey, inspection.WorkspaceID); err != nil {
			return reportError(errOut, fmt.Errorf("evaluate adapter interaction: %w", err))
		}
		if action == "session-start" {
			signalSessionPresence(state, inspection.WorkspaceID)
		}
		return writeJSON(out, response, errOut)
	}
	event := lifecycle.PostActionObserve
	if action == "stop-finalization" {
		event = lifecycle.StopFinalize
	}
	receipt, err := codexadapter.Receipt(event, native)
	if err != nil {
		return reportError(errOut, fmt.Errorf("build lifecycle receipt: %w", err))
	}
	receipt.IdempotencyKey = orchestrationBoundKey(receipt.IdempotencyKey, state)
	if _, err := lifecycle.Record(root, inspection.WorkspaceID, receipt); err != nil {
		return reportError(errOut, fmt.Errorf("record lifecycle receipt: %w", err))
	}
	if err := evaluateAdapterInteraction(root, "codex", string(event), receipt.IdempotencyKey, inspection.WorkspaceID); err != nil {
		return reportError(errOut, fmt.Errorf("evaluate adapter interaction: %w", err))
	}
	return writeJSON(out, codexadapter.FinalizationOutput{Continue: true}, errOut)
}

func runClaudeHook(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	const usage = "usage: bcgos hook claude <session-start|context-injection|pre-action-guard|post-action-receipt|stop-finalization> [workspace-path]"
	if len(args) == 0 {
		fmt.Fprintln(errOut, usage)
		return ExitUsage
	}
	action := args[0]
	flags := newFlagSet("hook claude "+action, errOut)
	adapterSource := flags.String("adapter-source", "", "internal adapter ownership marker")
	orchestrationState := flags.String("orchestration-state", "", "shared Maestro orchestration state path")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() > 1 || (*adapterSource != "" && *adapterSource != "maestro") {
		fmt.Fprintln(errOut, usage)
		return ExitUsage
	}
	switch action {
	case "session-start", "context-injection", "pre-action-guard", "post-action-receipt", "stop-finalization":
	default:
		fmt.Fprintln(errOut, usage)
		return ExitUsage
	}

	var native claudeadapter.NativeInput
	if action == "context-injection" || action == "pre-action-guard" || action == "post-action-receipt" || action == "stop-finalization" {
		parsed, err := claudeadapter.ParseReader(in)
		if err != nil {
			if action == "pre-action-guard" {
				return writeJSON(out, claudeadapter.FailClosedDenial(), errOut)
			}
			return reportError(errOut, fmt.Errorf("parse bounded Claude hook input: %w", err))
		}
		native = parsed
	}

	// The safety decision depends only on the bounded native payload. It must
	// not be weakened by missing, malformed or slow workspace state.
	if action == "pre-action-guard" {
		if isReadOnlyBoundedDiagnostic(native.ToolName, native.ToolInputJSON()) {
			return writeJSON(out, claudeadapter.GuardOutput{}, errOut)
		}
		response, err := claudeadapter.Guard(native)
		if err != nil {
			return writeJSON(out, claudeadapter.FailClosedDenial(), errOut)
		}
		if response.HookSpecificOutput != nil {
			return writeJSON(out, response, errOut)
		}
		protected, canonicalErr := actionconfirmation.Canonicalize(native.ToolName, native.ToolInputJSON())
		if canonicalErr != nil {
			return writeJSON(out, claudeadapter.ExternalActionDenial(noncanonicalExternalDenial), errOut)
		}
		if protected == nil && !actionconfirmation.LooksLikeBCGOSDiagnostic(native.ToolName, native.ToolInputJSON()) {
			return writeJSON(out, response, errOut)
		}
		root, inspection, inspectErr := inspectProtectedActionWorkspace(optionalArg(flags.Args()), dataRoot)
		if inspectErr != nil {
			if protected != nil {
				return writeJSON(out, claudeadapter.ExternalActionDenial(unavailableConfirmationDenial), errOut)
			}
			return writeJSON(out, claudeadapter.ExternalActionDenial(unverifiedDiagnosticDenial), errOut)
		}
		state, stateErr := resolveHookOrchestrationState(inspection, *orchestrationState)
		if stateErr != nil {
			if protected != nil {
				return writeJSON(out, claudeadapter.ExternalActionDenial(unavailableConfirmationDenial), errOut)
			}
			return writeJSON(out, claudeadapter.ExternalActionDenial(unverifiedDiagnosticDenial), errOut)
		}
		if protected != nil {
			actorID, actorErr := localConfirmedOwnerActor(root)
			if actorErr != nil {
				return writeJSON(out, claudeadapter.ExternalActionDenial(unavailableConfirmationDenial), errOut)
			}
			result, authorizeErr := confirmationStore(root, inspection.WorkspaceID).Authorize(actionconfirmation.Binding{Runtime: "claude", WorkspaceID: inspection.WorkspaceID, ActorID: actorID, SessionID: native.SessionID, Action: *protected})
			if authorizeErr != nil {
				return writeJSON(out, claudeadapter.ExternalActionDenial(unavailableConfirmationDenial), errOut)
			}
			if result.State != actionconfirmation.Authorized {
				return writeJSON(out, claudeadapter.ExternalActionDenial(challengeDenial(result)), errOut)
			}
		}
		interactionKey := orchestrationBoundKey(lifecycle.IdempotencyKey("claude", string(lifecycle.PreActionGuard), inspection.WorkspaceID, native.ToolName, native.ToolUseID), state)
		if err := evaluateAdapterInteraction(root, "claude", string(lifecycle.PreActionGuard), interactionKey, inspection.WorkspaceID); err != nil {
			return writeJSON(out, claudeadapter.FailClosedDenial(), errOut)
		}
		return writeJSON(out, response, errOut)
	}

	root, err := dataRoot()
	if err != nil {
		return reportError(errOut, err)
	}
	inspection, err := workspace.Inspect(optionalArg(flags.Args()), root)
	if err != nil {
		return reportError(errOut, err)
	}
	state, err := resolveHookOrchestrationState(inspection, *orchestrationState)
	if err != nil {
		return reportError(errOut, err)
	}
	switch action {
	case "session-start", "context-injection":
		profileState, err := resolveProfile(root, "", false)
		if err != nil {
			return reportError(errOut, err)
		}
		owner, err := ownerctx.Inspect(root)
		if err != nil {
			return reportError(errOut, err)
		}
		sharePointSource, err := priorWorkSourceStatus(root, inspection.WorkspaceID)
		if err != nil {
			return reportError(errOut, fmt.Errorf("inspect guided SharePoint source selection: %w", err))
		}
		continuous, activeExecution, err := buildContinuousUseStatus(root, inspection, owner)
		if err != nil {
			return reportError(errOut, fmt.Errorf("build continuous-use status: %w", err))
		}
		packet := sessionctx.Build(sessionctx.Sources{
			Profile: profileState, Workspace: inspection, Owner: owner, OwnerContextRoot: root,
			Atlas:            atlas.Inspect(atlas.Options{DataRoot: root, WorkspacePath: inspection.WorkspacePath, WorkspaceID: inspection.WorkspaceID}),
			Execution:        activePointerFromContinuity(activeExecution),
			Memory:           sessionMemorySource(root, inspection.WorkspaceID),
			SharePointSource: sharePointSource,
			ContinuousUse:    continuous,
		})
		if action == "context-injection" {
			if err := enrichContextPacket(&packet, "claude", inspection.WorkspacePath, root, native.SessionID, native.Prompt); err != nil {
				return reportError(errOut, err)
			}
		} else if err := enrichOnboardingGuide(&packet, "claude", inspection.WorkspacePath); err != nil {
			return reportError(errOut, err)
		}
		eventName := "SessionStart"
		if action == "context-injection" {
			eventName = "UserPromptSubmit"
		}
		response, err := sessionhook.BuildClaudeEvent(packet, eventName)
		if err != nil {
			return reportError(errOut, err)
		}
		event := lifecycle.SessionStart
		if action == "context-injection" {
			event = lifecycle.ContextInject
		}
		interactionKey := orchestrationBoundKey(lifecycle.IdempotencyKey("claude", event, inspection.WorkspaceID), state)
		if err := evaluateAdapterInteraction(root, "claude", event, interactionKey, inspection.WorkspaceID); err != nil {
			return reportError(errOut, fmt.Errorf("evaluate adapter interaction: %w", err))
		}
		if action == "session-start" {
			signalSessionPresence(state, inspection.WorkspaceID)
		}
		return writeJSON(out, response, errOut)
	case "post-action-receipt", "stop-finalization":
		event := lifecycle.PostActionObserve
		if action == "stop-finalization" {
			event = lifecycle.StopFinalize
		}
		receipt, err := claudeadapter.Receipt(event, native)
		if err != nil {
			return reportError(errOut, fmt.Errorf("build lifecycle receipt: %w", err))
		}
		receipt.IdempotencyKey = orchestrationBoundKey(receipt.IdempotencyKey, state)
		if _, err := lifecycle.Record(root, inspection.WorkspaceID, receipt); err != nil {
			return reportError(errOut, fmt.Errorf("record lifecycle receipt: %w", err))
		}
		if err := evaluateAdapterInteraction(root, "claude", string(event), receipt.IdempotencyKey, inspection.WorkspaceID); err != nil {
			return reportError(errOut, fmt.Errorf("evaluate adapter interaction: %w", err))
		}
		return writeJSON(out, claudeadapter.FinalizationOutput{Continue: true}, errOut)
	}
	panic("unreachable Claude hook action")
}

const (
	noncanonicalExternalDenial    = "Maestro denied this external mutation because the request is outside the bounded canonical grammar. Nothing was changed. Use an explicit action and target, then retry."
	unavailableConfirmationDenial = "Maestro denied this external mutation because a user-bound confirmation challenge could not be evaluated. Nothing was changed. Retry from an identified native session."
	unverifiedDiagnosticDenial    = "Maestro did not run this bcgos diagnostic because the executable is not the installed Maestro CLI. Nothing was changed. Use the exact path from SessionStart or run the diagnostic through the Maestro skill."
)

func inspectProtectedActionWorkspace(path string, dataRoot func() (string, error)) (string, workspace.Inspection, error) {
	root, err := dataRoot()
	if err != nil {
		return "", workspace.Inspection{}, err
	}
	inspection, err := workspace.Inspect(path, root)
	if err != nil {
		return "", workspace.Inspection{}, err
	}
	return root, inspection, nil
}

func confirmationStore(root, workspaceID string) actionconfirmation.Store {
	return actionconfirmation.Store{Root: filepath.Join(root, "runtime", "action-confirmation", workspaceID)}
}

func localConfirmedOwnerActor(root string) (string, error) {
	profile, err := agentidentity.Load(root)
	if err != nil {
		return "", fmt.Errorf("load confirmed owner enrollment: %w", err)
	}
	principal, err := localPriorWorkActorRef()
	if err != nil {
		return "", err
	}
	return profile.OwnerID + "@" + principal, nil
}

func challengeDenial(result actionconfirmation.Result) string {
	return fmt.Sprintf("Maestro requires explicit user confirmation for external action %s on target %s. Reply exactly: CONFIRM MAESTRO %s. Challenge expires at %s. Nothing was changed.", result.Action, result.Target, result.ChallengeID, result.ExpiresAt.UTC().Format(time.RFC3339))
}

func enrichContextPacket(packet *sessionctx.Packet, runtimeName, workspacePath, root, sessionID, prompt string) error {
	actorID, _ := localConfirmedOwnerActor(root)
	confirmed, err := confirmationStore(root, packet.Workspace.ID).Confirm(runtimeName, packet.Workspace.ID, actorID, sessionID, prompt)
	if err != nil {
		return fmt.Errorf("confirm external action: %w", err)
	}
	if confirmed {
		packet.ActionConfirmation = &sessionctx.ActionConfirmation{State: "confirmed"}
		return nil
	}
	projection, err := runtimeprojection.Inspect(runtimeName, workspacePath)
	if err != nil {
		return fmt.Errorf("inspect runtime projection for contextual routing: %w", err)
	}
	if projection.State != "installed" {
		return nil
	}
	if packet.Owner.Onboarding.State != "complete" {
		return enrichOnboardingGuide(packet, runtimeName, workspacePath)
	}
	catalog, policy, installed, err := runtimeprojection.RoutingInputs(runtimeName, workspacePath)
	if err != nil {
		return fmt.Errorf("load governed contextual routing inputs: %w", err)
	}
	selected, err := skillrouting.Route(skillrouting.Request{Prompt: prompt, Role: "case_agent", Catalog: catalog, Policy: policy, Installed: installed})
	if err != nil {
		return fmt.Errorf("route contextual skills: %w", err)
	}
	for _, item := range selected {
		packet.Skills.Selected = append(packet.Skills.Selected, sessionctx.SkillSelection{ID: item.ID, Reason: item.Reason, Pointer: item.Pointer})
	}
	// Continuity is intentionally best-effort and metadata-only: a prompt hook
	// must never block because its local checkpoint could not be persisted.
	_ = recordAttestedSkillRoute(root, runtimeName, packet.Workspace.ID, sessionID, selected)
	return nil
}

// enrichOnboardingGuide is a lifecycle-owned startup rule, not an agent skill
// grant. While the deterministic owner state is incomplete, it points the
// runtime to the exact integrity-checked onboarding guide and suppresses
// unrelated contextual methods. The guide does not grant tools, data access or
// native runtime authority.
func enrichOnboardingGuide(packet *sessionctx.Packet, runtimeName, workspacePath string) error {
	// Native hooks invoke this process by the installed absolute path. Carry it
	// into the human-facing directive so skills never have to rely on the
	// runtime's PATH, which is often different inside a desktop app.
	if executable, err := os.Executable(); err == nil && filepath.IsAbs(executable) {
		packet.MaestroCLIPath = executable
	}
	if packet.Owner.Onboarding.State == "complete" {
		return nil
	}
	projection, err := runtimeprojection.Inspect(runtimeName, workspacePath)
	if err != nil {
		return fmt.Errorf("inspect runtime projection for onboarding guide: %w", err)
	}
	if projection.State != "installed" {
		return nil
	}
	catalog, _, installed, err := runtimeprojection.RoutingInputs(runtimeName, workspacePath)
	if err != nil {
		return fmt.Errorf("load governed onboarding guide: %w", err)
	}
	known := false
	for _, skill := range catalog.Skills {
		if skill.ID == "maestro-onboarding" {
			known = true
			break
		}
	}
	if !known {
		return errors.New("managed onboarding guide is absent from the active skill catalog")
	}
	for _, skill := range installed {
		if skill.ID == "maestro-onboarding" {
			packet.Skills.Selected = []sessionctx.SkillSelection{{ID: skill.ID, Reason: "deterministic_onboarding_state", Pointer: skill.Pointer}}
			return nil
		}
	}
	return errors.New("managed onboarding guide is absent from the integrity-checked runtime projection")
}

func recordAttestedSkillRoute(root, runtimeName, workspaceID, sessionID string, selected []skillrouting.Selection) error {
	if len(selected) == 0 {
		return nil
	}
	ids := make([]string, 0, len(selected))
	for _, item := range selected {
		ids = append(ids, item.ID)
	}
	memoryRoot := filepath.Join(root, "memory")
	attestor := memory.CaptureAttestor{Root: memoryRoot}
	capture, err := attestor.Seal(memory.Capture{
		WorkspaceID:  workspaceID,
		RecordedAt:   time.Now().UTC(),
		Kind:         "skill_route",
		Text:         strings.Join(ids, ","),
		Sanitized:    true,
		ProducerID:   runtimeName + ".context-injection",
		SanitizerID:  memory.SkillRouteSanitizerID,
		SourceDigest: maestro.SHA256Hex(runtimeName + "\x00" + sessionID + "\x00" + strings.Join(ids, "\x00")),
	})
	if err != nil {
		return err
	}
	policy, err := basememory.Policy()
	if err != nil {
		return err
	}
	_, err = (&memory.Engine{Root: memoryRoot, Policy: policy}).Capture(capture)
	return err
}

// evaluateAdapterInteraction is deliberately metadata-only. Adapter hooks
// evaluate every observed interaction, but their unauthenticated/non-material
// lifecycle signal cannot create a self observation. An attended owner command
// is the separate path that may append an authenticated material signal.
func evaluateAdapterInteraction(root, runtimeName, event, sourceEvent, workspaceID string) error {
	_, _, err := ownerctx.AppendObservation(root, ownerctx.ObservationInput{
		SchemaVersion: 1,
		Signal:        ownerctx.SignalInferredHypothesis,
		Claim:         "adapter_lifecycle_observed",
		EvidenceType:  "adapter_receipt",
		SourceEvent:   "interaction.completed",
		SourceDigest:  maestro.SHA256Hex(runtimeName + "\x00" + event + "\x00" + sourceEvent),
		EpisodeID:     runtimeName + "-" + event,
		ScopeKind:     "workspace",
		ScopeID:       workspaceID,
		Confidence:    0,
		Sensitivity:   "professional",
		ExpiresAt:     time.Now().UTC().Add(24 * time.Hour),
		Material:      false,
	})
	return err
}

func runAdapter(args []string, out, errOut io.Writer) int {
	return runAdapterWithDataRoot(args, out, errOut, defaultDataRoot)
}

// bootstrapAdapterDependencies makes the standalone adapter command safe to
// use as an installation entry point too. It is idempotent and data-free: the
// workspace state, owner registry and agent scaffolds are created, but no
// onboarding answer or external content is inferred or ingested.
func bootstrapAdapterDependencies(root, workspacePath string) error {
	inspection, err := workspace.Inspect(workspacePath, root)
	if err != nil {
		return fmt.Errorf("inspect workspace before bootstrap: %w", err)
	}
	allowSynchronized := inspection.State == "warning" && inspection.WorkspaceID != "" && inspection.MetadataStatus == "valid"
	result, err := workspace.Initialize(workspace.Options{WorkspacePath: workspacePath, DataRoot: root, AllowSynchronizedRoot: allowSynchronized})
	if err != nil {
		return fmt.Errorf("bootstrap workspace: %w", err)
	}
	if _, err := ownerctx.Initialize(root); err != nil {
		return fmt.Errorf("bootstrap owner context: %w", err)
	}
	if _, err := workspaceagent.Initialize(root, result.WorkspaceID); err != nil {
		return fmt.Errorf("bootstrap workspace agent: %w", err)
	}
	if _, err := agentscaffold.Scaffold(root, agentscaffold.WorkspaceRequest(result.WorkspaceID)); err != nil {
		return fmt.Errorf("bootstrap agent scaffold: %w", err)
	}
	return nil
}

func runAdapterWithDataRoot(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 || (args[0] != "install" && args[0] != "uninstall" && args[0] != "status" && args[0] != "verify") {
		fmt.Fprintln(errOut, "usage: bcgos adapter <install|uninstall|status|verify> --runtime claude|codex [workspace-path]")
		return ExitUsage
	}
	flags := newFlagSet("adapter "+args[0], errOut)
	runtimeName := flags.String("runtime", "", "target runtime: claude or codex")
	executable := flags.String("executable", "", "path to the installed bcgos executable")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() > 1 {
		fmt.Fprintln(errOut, "usage: bcgos adapter <install|uninstall|status|verify> --runtime claude|codex [workspace-path]")
		return ExitUsage
	}
	if args[0] == "verify" && ((*runtimeName != "claude" && *runtimeName != "codex") || *executable != "") {
		fmt.Fprintln(errOut, "usage: bcgos adapter verify --runtime claude|codex [workspace-path]")
		return ExitUsage
	}
	path := optionalArg(flags.Args())
	root, rootErr := dataRoot()
	if rootErr != nil {
		return reportError(errOut, rootErr)
	}
	tracks := []string(nil)
	if profile, loadErr := agentidentity.Load(root); loadErr == nil {
		tracks = profile.CapabilityTracks
	} else if !errors.Is(loadErr, os.ErrNotExist) {
		return reportError(errOut, loadErr)
	}
	if args[0] == "verify" {
		resolvedExecutable, err := os.Executable()
		if err != nil {
			return reportError(errOut, fmt.Errorf("locate installed bcgos executable: %w", err))
		}
		report, verifyErr := installreadiness.Verify(installreadiness.Options{
			Runtime: *runtimeName, WorkspacePath: path, DataRoot: root, ExecutablePath: resolvedExecutable,
			CLIVersion: Version, CapabilityTracks: tracks,
		})
		if verifyErr != nil {
			if code := writeJSON(errOut, report, errOut); code != ExitOK {
				return code
			}
			return ExitFailure
		}
		return writeJSON(out, report, errOut)
	}
	type adapterResult struct {
		adaptercfg.Status
		Projection runtimeprojection.Status `json:"projection"`
	}
	var result adapterResult
	var err error
	type fileSnapshot struct {
		path   string
		exists bool
		mode   os.FileMode
		body   []byte
	}
	snapshotFile := func(path string) (fileSnapshot, error) {
		if path == "" {
			return fileSnapshot{}, nil
		}
		info, statErr := os.Lstat(path)
		if errors.Is(statErr, os.ErrNotExist) {
			return fileSnapshot{path: path}, nil
		}
		if statErr != nil {
			return fileSnapshot{}, statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fileSnapshot{}, fmt.Errorf("refusing to snapshot adapter symlink %s", path)
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return fileSnapshot{}, readErr
		}
		return fileSnapshot{path: path, exists: true, mode: info.Mode().Perm(), body: body}, nil
	}
	restoreFile := func(snapshot fileSnapshot) error {
		if snapshot.path == "" {
			return nil
		}
		if !snapshot.exists {
			if err := os.Remove(snapshot.path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
			return nil
		}
		if err := os.WriteFile(snapshot.path, snapshot.body, snapshot.mode); err != nil {
			return err
		}
		return nil
	}
	switch args[0] {
	case "install":
		resolvedExecutable := *executable
		if resolvedExecutable == "" {
			resolvedExecutable, err = os.Executable()
			if err != nil {
				return reportError(errOut, fmt.Errorf("locate installed bcgos executable: %w", err))
			}
		}
		// Preflight both local surfaces before either one writes. This keeps a
		// malformed/tracked hook config or a user-file projection conflict from
		// producing a known half-installed state.
		if err = adaptercfg.ValidateInstall(*runtimeName, path, resolvedExecutable); err != nil {
			return reportError(errOut, err)
		}
		if err = runtimeprojection.ValidateInstallForTracks(*runtimeName, path, tracks); err != nil {
			return reportError(errOut, err)
		}
		if err = bootstrapAdapterDependencies(root, path); err != nil {
			return reportError(errOut, err)
		}
		priorAdapter, inspectErr := adaptercfg.Inspect(*runtimeName, path)
		if inspectErr != nil {
			return reportError(errOut, inspectErr)
		}
		excludePath, excludeErr := adaptercfg.LocalConfigExcludePath(*runtimeName, path)
		if excludeErr != nil {
			return reportError(errOut, excludeErr)
		}
		snapshot, snapshotErr := snapshotFile(priorAdapter.Path)
		if snapshotErr != nil {
			return reportError(errOut, snapshotErr)
		}
		excludeSnapshot, snapshotErr := snapshotFile(excludePath)
		if snapshotErr != nil {
			return reportError(errOut, snapshotErr)
		}
		restoreAdapterState := func() error {
			if restoreErr := restoreFile(snapshot); restoreErr != nil {
				return restoreErr
			}
			return restoreFile(excludeSnapshot)
		}
		previousProjection, _ := runtimeprojection.Inspect(*runtimeName, path)
		result.Status, err = adaptercfg.Install(*runtimeName, path, resolvedExecutable)
		if err != nil {
			if restoreErr := restoreAdapterState(); restoreErr != nil {
				return reportError(errOut, fmt.Errorf("adapter install failed and rollback failed: %w (original: %v)", restoreErr, err))
			}
			return reportError(errOut, err)
		}
		result.Projection, err = runtimeprojection.InstallForTracks(*runtimeName, path, tracks)
		if err != nil {
			if restoreErr := restoreAdapterState(); restoreErr != nil {
				return reportError(errOut, fmt.Errorf("projection failed and adapter rollback failed: %w (original: %v)", restoreErr, err))
			}
			if previousProjection.State == "absent" {
				_, _ = runtimeprojection.Uninstall(*runtimeName, path)
			}
		}
	case "uninstall":
		if err = adaptercfg.ValidateUninstall(*runtimeName, path); err != nil {
			return reportError(errOut, err)
		}
		if err = runtimeprojection.ValidateUninstall(*runtimeName, path); err != nil {
			return reportError(errOut, err)
		}
		priorAdapter, inspectErr := adaptercfg.Inspect(*runtimeName, path)
		if inspectErr != nil {
			return reportError(errOut, inspectErr)
		}
		excludePath, excludeErr := adaptercfg.LocalConfigExcludePath(*runtimeName, path)
		if excludeErr != nil {
			return reportError(errOut, excludeErr)
		}
		snapshot, snapshotErr := snapshotFile(priorAdapter.Path)
		if snapshotErr != nil {
			return reportError(errOut, snapshotErr)
		}
		excludeSnapshot, snapshotErr := snapshotFile(excludePath)
		if snapshotErr != nil {
			return reportError(errOut, snapshotErr)
		}
		restoreAdapterState := func() error {
			if restoreErr := restoreFile(snapshot); restoreErr != nil {
				return restoreErr
			}
			return restoreFile(excludeSnapshot)
		}
		result.Status, err = adaptercfg.Uninstall(*runtimeName, path)
		if err != nil {
			if restoreErr := restoreAdapterState(); restoreErr != nil {
				return reportError(errOut, fmt.Errorf("adapter uninstall failed and rollback failed: %w (original: %v)", restoreErr, err))
			}
			return reportError(errOut, err)
		}
		result.Projection, err = runtimeprojection.Uninstall(*runtimeName, path)
		if err != nil {
			if restoreErr := restoreAdapterState(); restoreErr != nil {
				return reportError(errOut, fmt.Errorf("projection uninstall failed and adapter rollback failed: %w (original: %v)", restoreErr, err))
			}
		}
	case "status":
		result.Status, err = adaptercfg.Inspect(*runtimeName, path)
		if err == nil {
			result.Projection, err = runtimeprojection.Inspect(*runtimeName, path)
		}
	}
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, result, errOut)
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

func lifecycleReceiptCheck(dataRoot, workspaceID string) doctorCheck {
	if workspaceID == "" {
		return doctorCheck{ID: "lifecycle_receipts", State: "unavailable", Message: "no initialized workspace exists for lifecycle receipt evidence"}
	}
	summary, err := lifecycle.Diagnose(dataRoot, workspaceID)
	if err != nil {
		return doctorCheck{ID: "lifecycle_receipts", State: "warning", Message: "lifecycle receipt diagnostics could not be read: " + err.Error()}
	}
	switch summary.State {
	case "observed":
		return doctorCheck{ID: "lifecycle_receipts", State: "pass", Message: fmt.Sprintf("%d metadata-safe adapter-command lifecycle receipt(s) observed; native-session conformance is still required for capability promotion", summary.Observed)}
	default:
		return doctorCheck{ID: "lifecycle_receipts", State: "unavailable", Message: "no adapter-command lifecycle receipt has been recorded; native conformance is separately required"}
	}
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
	}{StatusReport: report, Dreaming: "daily_light_available_weekly_deep_unavailable"}, errOut)
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
	if cycle == "weekly" {
		code := writeJSON(out, map[string]any{"capability": "memory_deep_dreaming", "cycle": cycle, "state": "unavailable", "workspace_id": *workspace, "reason": "no qualified deep synthesis and lifetime eligibility adapter is installed"}, errOut)
		if code != ExitOK {
			return code
		}
		return ExitUnavailable
	}
	policy, err := basememory.Policy()
	if err != nil {
		return reportError(errOut, err)
	}
	config, err := basememory.Runtime()
	if err != nil {
		return reportError(errOut, err)
	}
	memoryRoot := filepath.Join(*dataDir, "memory")
	attestor := memory.CaptureAttestor{Root: memoryRoot}
	engine := memory.Engine{Root: memoryRoot, Policy: policy, Budgets: map[string]int{"L1": config.L1MaxRunes, "L2": 1, "L3": 1, "lifetime": 1}, MaxSourceBytes: config.L1MaxInputBytes, Synthesizer: memory.DeterministicL1Synthesizer{MaxRunes: config.L1MaxRunes, MaxEntries: config.L1MaxEntries, MaxInputBytes: config.L1MaxInputBytes, MaxInputEntries: config.L1MaxInputEntries, Attestor: attestor}, SynthesizerID: memory.DeterministicL1SynthesizerID}
	result, err := engine.DreamDailyAttested(context.Background(), *workspace, time.Now().UTC())
	if errors.Is(err, os.ErrNotExist) {
		return writeJSON(out, map[string]any{"capability": "memory_light_dreaming", "cycle": cycle, "state": "reviewed_no_change", "workspace_id": *workspace, "reason": "no trusted capture-v2 L1 input is available for the current period"}, errOut)
	}
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, map[string]any{"capability": "memory_light_dreaming", "cycle": cycle, "state": map[bool]string{true: "reviewed_no_change", false: "succeeded"}[result.Skipped], "workspace_id": *workspace, "result": result}, errOut)
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
