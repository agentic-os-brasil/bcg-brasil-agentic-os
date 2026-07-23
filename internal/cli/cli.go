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
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/memory"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/profile"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/runtimecap"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

const (
	ExitOK          = 0
	ExitFailure     = 1
	ExitUsage       = 2
	ExitUnavailable = 3
)

var Version = "0.0.0-dev"

func Run(args []string, out, errOut io.Writer) int {
	return RunWithInput(args, strings.NewReader(""), out, errOut)
}

func RunWithInput(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "usage: bcgos <init|doctor|status|version|profile|skills|memory>")
		return ExitUsage
	}
	switch args[0] {
	case "help", "--help", "-h":
		fmt.Fprintln(out, "usage: bcgos <init|doctor|status|version|profile|skills|memory>")
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
	case "profile":
		return runProfile(args[1:], out, errOut, defaultDataRoot)
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
	state, err := initializeProfile(root, *requestedProfile)
	if err != nil {
		return reportError(errOut, err)
	}
	return writeJSON(out, struct {
		workspace.Result
		Profile profile.State `json:"profile"`
	}{Result: result, Profile: state}, errOut)
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
			"bundles":             "unavailable",
			"interaction_profile": "supported",
			"memory_dreaming":     "unavailable",
			"updates":             "unavailable",
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
	checks := []doctorCheck{
		workspaceCheck,
		{ID: "local_data", State: "pass", Message: "private BCGOS data is separated from the workspace"},
		interactionProfileCheck(profileState),
		runtimeCheck("claude_code", "claude", available),
		runtimeCheck("codex", "codex", available),
		{ID: "bundles", State: "unavailable", Message: "bundle installation is not implemented in this build"},
		{ID: "updates", State: "unavailable", Message: "update and rollback are not implemented in this build"},
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
