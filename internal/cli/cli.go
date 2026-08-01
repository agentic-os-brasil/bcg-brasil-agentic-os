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
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/adaptercfg"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/canary"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/codexadapter"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/federation"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/markitdown"
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
const maximumWorkContractBytes = 32 << 10

func Run(args []string, out, errOut io.Writer) int {
	return RunWithInput(args, strings.NewReader(""), out, errOut)
}

func RunWithInput(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos <init|doctor|status|version|auth|update|profile|owner|agent|workspace-agent|atlas|prior-work|session|hook|adapter|skills|bundles|memory|maintenance|ingest|federation|canary|work>")
		return ExitUsage
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprintln(out, "usage: bcgos <init|doctor|status|version|auth|update|profile|owner|agent|workspace-agent|atlas|prior-work|session|hook|adapter|skills|bundles|memory|maintenance|ingest|federation|canary|work>")
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
	case "agent":
		return runAgentWithInput(args[1:], in, out, errOut, defaultDataRoot)
	case "workspace-agent":
		return runWorkspaceAgentWithInput(args[1:], in, out, errOut, defaultDataRoot)
	case "atlas":
		return runAtlas(args[1:], out, errOut, defaultDataRoot)
	case "prior-work":
		return runPriorWork(args[1:], in, out, errOut, defaultDataRoot)
	case "session":
		return runSession(args[1:], out, errOut, defaultDataRoot)
	case "hook":
		return runHookWithInput(args[1:], in, out, errOut, defaultDataRoot)
	case "adapter":
		return runAdapter(args[1:], out, errOut)
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
		return writeJSON(out, agentidentity.InitialInterview(), errOut)
	case "personalize":
		if err := requireAgentStdin(args[1:], errOut); err != nil {
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
		input.UpdatedAt = time.Now().UTC()
		if err := agentidentity.Save(root, input); err != nil {
			return reportError(errOut, err)
		}
		saved, err := agentidentity.Load(root)
		if err != nil {
			return reportError(errOut, err)
		}
		return writeJSON(out, saved, errOut)
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
		fmt.Fprintln(errOut, "usage: bcgos agent darwin <assess|housekeeping> --stdin")
		return ExitUsage
	}
	switch args[0] {
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
		fmt.Fprintln(errOut, "usage: bcgos agent darwin <assess|housekeeping> --stdin")
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
		fmt.Fprintln(out, "usage: bcgos bundles <index|plan --track TRACK[,TRACK...]>")
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
	default:
		fmt.Fprintln(errOut, "usage: bcgos bundles <index|plan --track TRACK[,TRACK...]>")
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

func runOwner(args []string, out, errOut io.Writer, dataRoot func() (string, error)) int {
	return runOwnerWithInput(args, strings.NewReader(""), out, errOut, dataRoot)
}

func runOwnerWithInput(args []string, in io.Reader, out, errOut io.Writer, dataRoot func() (string, error)) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos owner <init|status|interview|refine|self>")
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
	case "self":
		return runOwnerSelf(args[1:], in, out, errOut, root)
	default:
		fmt.Fprintln(errOut, "usage: bcgos owner <init|status|interview|refine|self>")
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
		confirm := flags.Bool("confirm", false, "confirm that this is an authenticated owner signal")
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
			fmt.Fprintln(errOut, "usage: bcgos owner self observation <reject|contradict|expire|redact> <id> --confirm")
			return ExitUsage
		}
		stateByName := map[string]ownerctx.ObservationState{"reject": ownerctx.ObservationRejected, "contradict": ownerctx.ObservationContradicted, "expire": ownerctx.ObservationExpired, "redact": ownerctx.ObservationRedacted}
		state, ok := stateByName[args[1]]
		if !ok || args[2] == "" || len(args) != 4 || args[3] != "--confirm" {
			fmt.Fprintln(errOut, "usage: bcgos owner self observation <reject|contradict|expire|redact> <id> --confirm")
			return ExitUsage
		}
		receipt, err := ownerctx.RejectObservation(root, args[2], state, true)
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
	activeExecution := execution.ActivePointer{State: execution.ActivePointerUnavailable}
	if inspection.WorkspaceID != "" {
		activeExecution, err = (execution.Store{Root: root}).ActivePointer(inspection.WorkspaceID)
		if err != nil {
			return reportError(errOut, fmt.Errorf("inspect active execution pointer: %w", err))
		}
	}
	packet := sessionctx.Build(sessionctx.Sources{
		Profile: profileState, Workspace: inspection, Owner: owner,
		Atlas:     atlas.Inspect(atlas.Options{DataRoot: root, WorkspacePath: inspection.WorkspacePath, WorkspaceID: inspection.WorkspaceID}),
		Execution: activeExecution,
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
	_ = orchestrationState
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
	if err := evaluateAdapterInteraction(root, *runtimeName, string(lifecycle.SessionStart), lifecycle.IdempotencyKey(*runtimeName, string(lifecycle.SessionStart), inspection.WorkspaceID), inspection.WorkspaceID); err != nil {
		return reportError(errOut, fmt.Errorf("evaluate adapter interaction: %w", err))
	}
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
	_ = orchestrationState
	switch action {
	case "session-start", "context-injection", "pre-action-guard", "post-action-receipt", "stop-finalization":
	default:
		fmt.Fprintln(errOut, usage)
		return ExitUsage
	}
	var native codexadapter.NativeInput
	if action == "pre-action-guard" || action == "post-action-receipt" || action == "stop-finalization" {
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
		response, err := codexadapter.Guard(native)
		if err != nil {
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
	if action == "session-start" || action == "context-injection" {
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
		if err := evaluateAdapterInteraction(root, "codex", event, lifecycle.IdempotencyKey("codex", event, inspection.WorkspaceID), inspection.WorkspaceID); err != nil {
			return reportError(errOut, fmt.Errorf("evaluate adapter interaction: %w", err))
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
	_ = orchestrationState
	switch action {
	case "session-start", "context-injection", "pre-action-guard", "post-action-receipt", "stop-finalization":
	default:
		fmt.Fprintln(errOut, usage)
		return ExitUsage
	}

	var native claudeadapter.NativeInput
	if action == "pre-action-guard" || action == "post-action-receipt" || action == "stop-finalization" {
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
		response, err := claudeadapter.Guard(native)
		if err != nil {
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
		packet := sessionctx.Build(sessionctx.Sources{
			Profile: profileState, Workspace: inspection, Owner: owner,
			Atlas: atlas.Inspect(atlas.Options{DataRoot: root, WorkspacePath: inspection.WorkspacePath, WorkspaceID: inspection.WorkspaceID}),
		})
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
		if err := evaluateAdapterInteraction(root, "claude", event, lifecycle.IdempotencyKey("claude", event, inspection.WorkspaceID), inspection.WorkspaceID); err != nil {
			return reportError(errOut, fmt.Errorf("evaluate adapter interaction: %w", err))
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
		SourceEvent:   "adapter-" + event,
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
	if len(args) == 0 || (args[0] != "install" && args[0] != "uninstall" && args[0] != "status") {
		fmt.Fprintln(errOut, "usage: bcgos adapter <install|uninstall|status> --runtime claude|codex [workspace-path]")
		return ExitUsage
	}
	flags := newFlagSet("adapter "+args[0], errOut)
	runtimeName := flags.String("runtime", "", "target runtime: claude or codex")
	executable := flags.String("executable", "", "path to the installed bcgos executable")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() > 1 {
		fmt.Fprintln(errOut, "usage: bcgos adapter <install|uninstall|status> --runtime claude|codex [workspace-path]")
		return ExitUsage
	}
	path := optionalArg(flags.Args())
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
		if err = runtimeprojection.ValidateInstall(*runtimeName, path); err != nil {
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
		result.Projection, err = runtimeprojection.Install(*runtimeName, path)
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
