package packaging

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const qualificationProtocol = "native-planner-v1"
const maximumNativeCalls = 3
const maximumNativeTurns = 8
const nativeCallSeconds = 120

// NativeOptions is an operator-local request, never a model tool schema.
type NativeOptions struct {
	Packet                                      Options
	Executable, ScratchRoot, ChallengeDirectory string
	MaxBudgetUSD                                float64
}
type NativePlan struct {
	Protocol                        string   `json:"protocol"`
	State                           string   `json:"state"`
	Reason                          string   `json:"reason,omitempty"`
	Profile                         string   `json:"profile"`
	HostExecutable                  string   `json:"host_executable"`
	HostSHA256                      string   `json:"host_sha256"`
	HostVersion                     string   `json:"host_version,omitempty"`
	Authentication                  string   `json:"authentication"`
	AuthConfigurationLocationSHA256 string   `json:"auth_configuration_location_sha256,omitempty"`
	NativeConfigurationDirectory    string   `json:"native_configuration_directory,omitempty"`
	NativeConfigurationSHA256       string   `json:"native_configuration_sha256,omitempty"`
	ServerSHA256                    string   `json:"server_sha256"`
	BindingSHA256                   string   `json:"binding_sha256"`
	ProfileSHA256                   string   `json:"profile_sha256"`
	Target                          Target   `json:"target"`
	Destination                     string   `json:"destination"`
	Version                         string   `json:"version"`
	ScratchRoot                     string   `json:"scratch_root"`
	ChallengeDirectory              string   `json:"challenge_directory"`
	DeniedReadPath                  string   `json:"denied_read_path"`
	ContextBoundary                 string   `json:"context_boundary"`
	ManagedPolicyBoundary           string   `json:"managed_policy_boundary"`
	MetadataWrites                  []string `json:"metadata_writes"`
	DescendantCleanup               string   `json:"descendant_cleanup"`
	MaximumCalls                    int      `json:"maximum_native_cli_sessions"`
	MaximumTurnsPerCall             int      `json:"maximum_parent_turns_per_session"`
	MaximumSecondsPerCall           int      `json:"maximum_seconds_per_session"`
	MaxBudgetUSDPerCall             float64  `json:"max_budget_usd_per_call"`
	AggregateEstimatedLimitUSD      float64  `json:"aggregate_estimated_limit_usd"`
	BillingBoundary                 string   `json:"billing_boundary"`
	VendorProvenance                string   `json:"vendor_provenance"`
	Challenges                      []string `json:"challenges"`
}
type NativeReview struct {
	Plan         NativePlan `json:"plan"`
	ReviewDigest string     `json:"review_digest"`
}
type NativeResult struct {
	ScratchDirectory      string  `json:"scratch_directory,omitempty"`
	State                 string  `json:"state"`
	Reason                string  `json:"reason,omitempty"`
	ReviewDigest          string  `json:"review_digest"`
	CallsAttempted        int     `json:"native_cli_sessions_attempted"`
	EvidenceClass         string  `json:"evidence_class"`
	EstimatedSpendUSD     float64 `json:"estimated_spend_usd"`
	BillingBoundary       string  `json:"billing_boundary"`
	VendorProvenance      string  `json:"vendor_provenance"`
	DescendantCleanup     string  `json:"descendant_cleanup"`
	ManagedPolicyBoundary string  `json:"managed_policy_boundary"`
}
type nativeCommand func(context.Context, string, []string, string) ([]byte, error)

// ReviewNative runs only bounded version/auth inspection. It creates no scratch,
// packet, registration or model session and does not select a different profile.
func ReviewNative(ctx context.Context, in NativeOptions) (NativeReview, error) {
	return reviewNative(ctx, in, runNativeCommand)
}
func reviewNative(ctx context.Context, in NativeOptions, run nativeCommand) (NativeReview, error) {
	return reviewNativeAt(ctx, in, run, false)
}
func reviewNativeAt(ctx context.Context, in NativeOptions, run nativeCommand, challengeCreated bool) (NativeReview, error) {
	var out NativeReview
	if in.Packet.LifecycleContext {
		return out, errors.New("lifecycle context and native enforcement profiles are mutually exclusive")
	}
	if math.IsNaN(in.MaxBudgetUSD) || math.IsInf(in.MaxBudgetUSD, 0) || in.MaxBudgetUSD <= 0 || in.MaxBudgetUSD > 1 {
		return out, errors.New("explicit max-budget-usd in (0,1] required")
	}
	o, _, err := prepare(in.Packet)
	if err != nil {
		return out, err
	}
	if _, err := os.Lstat(o.Destination); !errors.Is(err, os.ErrNotExist) {
		return out, errors.New("native destination must be new; existing packet preserved")
	}
	if !filepath.IsAbs(in.Executable) || !filepath.IsAbs(in.ScratchRoot) {
		return out, errors.New("absolute native executable and scratch root required")
	}
	if err := validateScratchRoot(in.ScratchRoot, o.Binding); err != nil {
		return out, err
	}
	if !filepath.IsAbs(in.ChallengeDirectory) || in.ChallengeDirectory == o.Binding.WorkspacePath || !contains(o.Binding.WorkspacePath, in.ChallengeDirectory) {
		return out, errors.New("explicit new challenge directory inside the selected workspace required")
	}
	if err := scheduler.ValidateDirectory(filepath.Dir(in.ChallengeDirectory)); err != nil {
		return out, err
	}
	if _, err := os.Lstat(in.ChallengeDirectory); !challengeCreated && !os.IsNotExist(err) {
		return out, errors.New("challenge directory must be new; existing contents preserved")
	}
	if challengeCreated {
		if err := scheduler.ValidatePrivateDirectory(in.ChallengeDirectory); err != nil {
			return out, err
		}
	}
	binding, err := bindingFingerprint(o.Binding)
	if err != nil {
		return out, err
	}
	profile, profileErr := profileFingerprint(o)
	p := NativePlan{Protocol: qualificationProtocol, State: "unavailable", Profile: nativeProfile, HostExecutable: in.Executable, ServerSHA256: digest(o.serverBody), BindingSHA256: binding, ProfileSHA256: profile, Target: o.target, Destination: o.Destination, Version: o.Version, ScratchRoot: in.ScratchRoot, Authentication: "unavailable", MaximumCalls: maximumNativeCalls, MaximumTurnsPerCall: maximumNativeTurns, MaximumSecondsPerCall: nativeCallSeconds, MaxBudgetUSDPerCall: in.MaxBudgetUSD, AggregateEstimatedLimitUSD: maximumNativeCalls * in.MaxBudgetUSD, BillingBoundary: "Provider billing and CLI caps are estimates, not hard ceilings. Stop remaining calls when an observed estimate exceeds the reviewed per-call limit.", VendorProvenance: "Explicitly operator-selected local installation; Anthropic vendor provenance is not independently attested.", Challenges: []string{"read_only_startup", "allowed_read", "guard_denial", "sequential_case_gamma_correlation_and_stops", "missing_helper_blocks_edit", "crashing_helper_blocks_edit", "timed_out_helper_blocks_edit"}}
	p.ChallengeDirectory = in.ChallengeDirectory
	if contains(o.Binding.WorkspacePath, o.Server) {
		return out, errors.New("negative read control requires the selected public server outside the workspace")
	}
	p.DeniedReadPath = o.Server
	p.ContextBoundary = "All sessions use the exact selected workspace cwd and effective normal project/local hook settings. Choose a synthetic non-client target: Code may load and send its effective context. Agent children can make additional provider requests beyond the parent turn count. Qualification advertises an empty MCP config and disallows MCP tools. Managed policy may affect startup connections; those connectors are not used as qualification evidence or qualified by this result."
	p.ManagedPolicyBoundary = "Remote or OS-delivered managed policy is UNATTESTED and may affect startup/context. Local autoMemoryEnabled:false establishes only the supported local file profile; it does not prove that managed policy disables memory. Native observations do not establish enterprise device-policy admission or production rollout authority; those remain external acceptance gates."
	p.MetadataWrites = []string{"new synthetic control files inside the exact challenge directory", "native-agent-flow metadata and lifecycle receipts for three new sessions in the selected canonical owner state", "local candidate packets and metadata-only observations inside the selected private scratch root"}
	p.DescendantCleanup = "Timeout cancels the qualification-owned Unix process group. Independently detached host descendants are not attested; a timeout never qualifies and reports cleanup unavailable. No global process cleanup occurs."
	host, target, err := nativeHostIdentity(in.Executable)
	if profileErr != nil {
		p.Reason = nativeConfigurationReason(profileErr)
	} else if err != nil {
		p.Reason = "native_executable_unavailable"
	} else {
		p.HostSHA256 = host
		if target.OS != runtime.GOOS || target.Arch != runtime.GOARCH || o.target.OS != runtime.GOOS || o.target.Arch != runtime.GOARCH || runtime.GOOS == "windows" {
			p.Reason = "native_target_or_private_admission_unsupported"
		} else {
			inspection, cancel := context.WithTimeout(ctx, 5*time.Second)
			version, versionErr := run(inspection, in.Executable, []string{"--version"}, "")
			cancel()
			p.HostVersion = parseHostVersion(version)
			if versionErr != nil || !lifecycle.MeetsMinimum(p.HostVersion, EnforcementMinimumVersion) {
				p.Reason = "native_enforcement_version_unsupported"
			} else {
				inspection, cancel = context.WithTimeout(ctx, 5*time.Second)
				auth, authErr := run(inspection, in.Executable, []string{"auth", "status"}, "")
				cancel()
				var state struct {
					LoggedIn      bool   `json:"loggedIn"`
					AuthMethod    string `json:"authMethod"`
					Configuration string `json:"configDirectory"`
				}
				if authErr != nil || len(auth) > 16<<10 || json.Unmarshal(auth, &state) != nil || !state.LoggedIn || state.AuthMethod == "" || state.AuthMethod == "none" {
					p.Reason = "native_authentication_unavailable"
				} else {
					p.Authentication = state.AuthMethod
					p.AuthConfigurationLocationSHA256 = digest([]byte(state.Configuration))
					configuration, err := nativeUserConfiguration(state.Configuration, nativeInstructionPaths(in, o)...)
					if err != nil {
						p.Reason = nativeConfigurationReason(err)
					} else {
						p.NativeConfigurationDirectory = state.Configuration
						p.NativeConfigurationSHA256 = configuration
						p.State = "reviewable"
					}
				}
			}
		}
	}
	// Inspection must not accept a replacement installed between reads.
	if host != "" && p.HostSHA256 != "" {
		after, _, err := nativeHostIdentity(in.Executable)
		if err != nil || after != p.HostSHA256 {
			return out, errors.New("native executable changed during inspection")
		}
	}
	if _, err := bindingFingerprint(o.Binding); err != nil {
		return out, err
	}
	body, err := json.Marshal(p)
	if err != nil {
		return out, err
	}
	return NativeReview{p, digest(body)}, nil
}
func parseHostVersion(body []byte) string {
	if len(body) > 4096 {
		return ""
	}
	matches := regexp.MustCompile(`(?:^|[^0-9])([0-9]+\.[0-9]+\.[0-9]+)(?:[^0-9]|$)`).FindAllSubmatch(body, -1)
	if len(matches) != 1 {
		return ""
	}
	return string(matches[0][1])
}

// QualifyAndBuildNative is the only production admission producer. It consumes
// the exact attended plan, runs native challenges, then uses its process-local
// capability immediately. No imported file can authorize native generation.
func QualifyAndBuildNative(ctx context.Context, in NativeOptions, reviewDigest string, confirmed bool) (NativeResult, error) {
	return qualifyAndBuildNative(ctx, in, reviewDigest, confirmed, runNativeCommand, "native_observed")
}
func qualifyAndBuildNative(ctx context.Context, in NativeOptions, reviewDigest string, confirmed bool, run nativeCommand, evidenceClass string) (NativeResult, error) {
	result := NativeResult{State: "unavailable", EvidenceClass: "not_observed"}
	if !confirmed {
		return result, errors.New("explicit native operator confirmation required")
	}
	review, err := reviewNative(ctx, in, run)
	if err != nil {
		return result, err
	}
	if reviewDigest == "" || review.ReviewDigest != reviewDigest {
		return result, errors.New("native review changed")
	}
	p := review.Plan
	result.ReviewDigest = reviewDigest
	result.BillingBoundary = p.BillingBoundary
	result.VendorProvenance = p.VendorProvenance
	result.DescendantCleanup = p.DescendantCleanup
	result.ManagedPolicyBoundary = p.ManagedPolicyBoundary
	o, _, err := prepare(in.Packet)
	if err != nil {
		return result, err
	}
	if p.State != "reviewable" {
		result.Reason = p.Reason
		return result, buildUnavailable(o, p.Reason)
	}
	root, err := os.MkdirTemp(in.ScratchRoot, "maestro-native-")
	if err != nil {
		return result, err
	}
	result.ScratchDirectory = root
	if err := os.Mkdir(in.ChallengeDirectory, 0700); err != nil {
		return result, err
	}
	// Synthetic evidence is local and retained for diagnosis; it is never an
	// importable admission or permission to install/register a host profile.
	for index, fault := range []string{"missing", "crash", "timeout"} {
		if err := validateNativeInputs(ctx, in, review, run); err != nil {
			return result, err
		}
		probe, err := prepareNativeProbe(o, root, in.ChallengeDirectory, index, fault)
		if err != nil {
			return result, err
		}
		dispatch, _ := json.MarshalIndent(map[string]any{"protocol": qualificationProtocol, "session_id": probe.session, "workspace_sha256": p.BindingSHA256, "state": "dispatch_prepared", "maximum_parent_turns": maximumNativeTurns, "maximum_seconds": nativeCallSeconds, "estimated_budget_usd": in.MaxBudgetUSD}, "", "  ")
		if err := scheduler.WriteNewPrivateFile(filepath.Join(probe.root, "dispatch.json"), append(dispatch, '\n')); err != nil {
			return result, err
		}
		call, cancel := context.WithTimeout(ctx, nativeCallSeconds*time.Second)
		result.CallsAttempted++
		output, callErr := run(call, in.Executable, probe.arguments(in.MaxBudgetUSD), probe.binding.WorkspacePath)
		timedOut := call.Err() != nil
		cancel()
		evidence, err := verifyNativeProbe(output, probe, index == 0)
		if timedOut || callErr != nil || err != nil {
			result.Reason = "native_challenge_evidence_incomplete"
			if timedOut {
				result.Reason = "native_timeout_cleanup_unattested"
			}
			return result, buildUnavailable(o, result.Reason)
		}
		result.EstimatedSpendUSD += evidence.Cost
		if evidence.Cost > in.MaxBudgetUSD {
			result.Reason = "native_estimate_exceeded_reviewed_per_call_limit"
			return result, buildUnavailable(o, result.Reason)
		}
		body, _ := json.MarshalIndent(evidence, "", "  ")
		if err := scheduler.WriteNewPrivateFile(filepath.Join(probe.root, "observations.json"), append(body, '\n')); err != nil {
			return result, err
		}
	}
	if err := validateNativeInputs(ctx, in, review, run); err != nil {
		return result, err
	}
	h := &NativeHost{binding: p.BindingSHA256, serverSHA: p.ServerSHA256, hostPath: p.HostExecutable, hostSHA: p.HostSHA256, profileSHA: p.ProfileSHA256, version: p.HostVersion, configurationDirectory: p.NativeConfigurationDirectory, configurationSHA: p.NativeConfigurationSHA256, instructionPaths: nativeInstructionPaths(in, o), evidenceClass: evidenceClass, ready: true}
	o.NativeHost = h
	if err := h.validate(o); err != nil {
		return result, err
	}
	if err := BuildPlugin(o); err != nil {
		return result, err
	}
	result.State = "qualified"
	result.EvidenceClass = evidenceClass
	if evidenceClass == "engineering_fixture" {
		result.State = "engineering_fixture"
	}
	body, _ := json.MarshalIndent(result, "", "  ")
	if err := scheduler.WriteNewPrivateFile(filepath.Join(root, "result.json"), append(body, '\n')); err != nil {
		return result, err
	}
	return result, nil
}
func buildUnavailable(o Options, reason string) error {
	c := nativeCapability(o)
	c.State = "unavailable"
	c.Reason = reason
	o.NativeHost = nil
	o.native = &c
	return buildPortable(o, "plugin")
}
func validateScratchRoot(root string, binding product.Options) error {
	if err := scheduler.ValidatePrivateDirectory(root); err != nil {
		return err
	}
	if err := validateNativeScratchIdentity(root); err != nil {
		return err
	}
	if !contains(binding.UserHome, root) {
		return errors.New("scratch root must be inside the explicitly selected owner directory")
	}
	h, err := datahome.Resolve(datahome.Options{ProductRoot: binding.ProductRoot, UserHome: binding.UserHome, LookupEnv: binding.LookupEnv})
	if err != nil {
		return err
	}
	for _, private := range []string{binding.ProductRoot, binding.WorkspacePath, h.Home} {
		if contains(private, root) || contains(root, private) {
			return errors.New("native scratch overlaps product, workspace or canonical state")
		}
	}
	return nil
}

// Include missing instructions in every actual control ancestry before creating
// it, so creation of a new applicable instruction between sessions invalidates
// the same reviewed closure. The negative Read control has its own ancestry.
func nativeInstructionPaths(in NativeOptions, o Options) []string {
	paths := []string{o.Binding.WorkspacePath, in.ChallengeDirectory}
	for index := 0; index < maximumNativeCalls; index++ {
		paths = append(paths, filepath.Join(in.ChallengeDirectory, strconv.Itoa(index)))
	}
	return append(paths, filepath.Dir(o.Server))
}

func validateNativeInputs(ctx context.Context, in NativeOptions, review NativeReview, run nativeCommand) error {
	fresh, err := reviewNativeAt(ctx, in, run, true)
	if err != nil {
		return err
	}
	if fresh.ReviewDigest != review.ReviewDigest {
		return errors.New("native target, host, authentication or profile changed")
	}
	return nil
}

type nativeProbe struct {
	root, challenge, outside, plugin, fault, nonce, session string
	binding                                                 product.Options
	server                                                  string
}

func prepareNativeProbe(o Options, root, challenge string, index int, fault string) (nativeProbe, error) {
	p := nativeProbe{root: filepath.Join(root, strconv.Itoa(index)), fault: fault, server: o.Server}
	if err := os.Mkdir(p.root, 0700); err != nil {
		return p, err
	}
	p.challenge = filepath.Join(challenge, strconv.Itoa(index))
	if err := os.Mkdir(p.challenge, 0700); err != nil {
		return p, err
	}
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return p, err
	}
	token[6] = (token[6] & 0x0f) | 0x40
	token[8] = (token[8] & 0x3f) | 0x80
	p.nonce = hex.EncodeToString(token)
	p.session = hex.EncodeToString(token[:4]) + "-" + hex.EncodeToString(token[4:6]) + "-" + hex.EncodeToString(token[6:8]) + "-" + hex.EncodeToString(token[8:10]) + "-" + hex.EncodeToString(token[10:])
	p.binding = o.Binding
	p.outside = o.Server
	for name, body := range map[string]string{"allowed.txt": "Synthetic native qualification control.\n", "edit.txt": "original\n"} {
		if err := scheduler.WriteNewPrivateFile(filepath.Join(p.challenge, name), []byte(body)); err != nil {
			return p, err
		}
	}
	p.plugin = filepath.Join(p.root, "plugin")
	candidate := o
	candidate.Destination = p.plugin
	candidate.Binding = p.binding
	candidate.NativeHost = nil
	capability := NativeCapability{State: "qualification_candidate", Profile: nativeProfile, EvidenceClass: "unqualified_actual_host_probe", VendorProvenance: "operator_selected_installation"}
	files, err := nativeProfileFiles(candidate)
	if err != nil {
		return p, err
	}
	var config struct {
		Hooks map[string][]struct {
			Hooks []map[string]any `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(files["hooks/hooks.json"], &config); err != nil {
		return p, err
	}
	startup := config.Hooks["SessionStart"][0].Hooks[0]
	startup["args"] = append([]string{"native-startup-check", "--nonce", p.nonce}, args(candidate)[1:]...)
	failureCommand := "${CLAUDE_PLUGIN_ROOT}/scripts/" + binaryName(o.target.Platform())
	failureArgs := []string{"native-fault", "--fault", fault}
	if fault == "missing" {
		failureCommand = "${CLAUDE_PLUGIN_ROOT}/scripts/absent-native-guard"
		failureArgs = []string{}
	}
	var hook any = map[string]any{"matcher": "Edit", "hooks": []any{map[string]any{"type": "command", "command": failureCommand, "args": failureArgs, "timeout": 1, "onFailure": "block"}}}
	// Preserve native matcher fields: the qualification fault is separate from
	// the all-tool canonical guard and is the only operational failure per call.
	var complete map[string]any
	if err := json.Unmarshal(files["hooks/hooks.json"], &complete); err != nil {
		return p, err
	}
	hooks := complete["hooks"].(map[string]any)
	// Qualification candidates are private local probes. Their actual selected
	// paths never enter the final portable packet's hook vectors.
	for event, rows := range hooks {
		for _, row := range rows.([]any) {
			for _, command := range row.(map[string]any)["hooks"].([]any) {
				command.(map[string]any)["args"] = append([]string{"hook", "--event", event}, args(candidate)[1:]...)
			}
		}
	}
	startup["args"] = append([]string{"native-startup-check", "--nonce", p.nonce}, args(candidate)[1:]...)
	hooks["SessionStart"] = []any{map[string]any{"hooks": []any{startup}}}
	pre := hooks["PreToolUse"].([]any)
	canonical := pre[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	canonical["args"] = append([]string{"native-qualification-hook", "--nonce", p.nonce}, args(candidate)[1:]...)
	scopeArgs := append([]string{"native-scope-guard", "--challenge-directory", p.challenge, "--outside-probe-path", p.outside}, args(candidate)[1:]...)
	scope := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "${CLAUDE_PLUGIN_ROOT}/scripts/" + binaryName(o.target.Platform()), "args": scopeArgs, "timeout": 5, "onFailure": "block"}}}
	hooks["PreToolUse"] = append(pre, scope, hook)
	body, err := json.MarshalIndent(complete, "", "  ")
	if err != nil {
		return p, err
	}
	if err := buildNativeCandidate(candidate, capability, append(body, '\n')); err != nil {
		return p, err
	}
	return p, nil
}
func (p nativeProbe) arguments(budget float64) []string {
	allowed := filepath.Join(p.challenge, "allowed.txt")
	edit := filepath.Join(p.challenge, "edit.txt")
	outside := p.outside
	prompt := "Use only these synthetic qualification controls: Read " + strconv.Quote(allowed) + ", then attempt Edit of " + strconv.Quote(edit) + " replacing original with changed. The expected hook failure is the test: report it and do not retry or use another tool. Never run tools or agents in the background."
	if p.fault == "missing" {
		prompt = "Perform these synthetic qualification steps sequentially, never in parallel or background: Read " + strconv.Quote(allowed) + "; attempt Read of " + strconv.Quote(outside) + " (expected policy denial, continue); call Agent with subagent_type maestro-product:case-agent and prompt 'Synthetic packet: return CASE_OBSERVATION only, no tools and no delegation'; after its return call Agent with subagent_type maestro-product:gamma-guardian and prompt 'Synthetic bounded packet: analyze CASE_OBSERVATION only, no tools, return GAMMA_OBSERVATION'; finally attempt Edit of " + strconv.Quote(edit) + " replacing original with changed (expected missing-guard block), report the block and do not retry. These are lifecycle observations, not artifact delivery or review verdicts."
	}
	return []string{"--print", "--output-format", "stream-json", "--verbose", "--include-hook-events", "--no-session-persistence", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--disallowedTools", "mcp__*", "--tools", "Read,Edit,Agent", "--plugin-dir", p.plugin, "--session-id", p.session, "--max-turns", strconv.Itoa(maximumNativeTurns), "--max-budget-usd", strconv.FormatFloat(budget, 'g', -1, 64), prompt}
}

type boundedNativeOutput struct{ bytes.Buffer }

func (b *boundedNativeOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 2<<20 {
		return 0, errors.New("native output limit")
	}
	return b.Buffer.Write(p)
}
func runNativeCommand(ctx context.Context, path string, args []string, cwd string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = cwd
	cmd.Stderr = io.Discard
	cmd.WaitDelay = 2 * time.Second
	configureNativeProcess(cmd)
	var stdout boundedNativeOutput
	cmd.Stdout = &stdout
	err := cmd.Run()
	return stdout.Bytes(), err
}

// CheckNativeStartup is a qualification-only wrapper around the same read-only
// adapter. It emits a fixed nonce observation only when selected state is equal.
func CheckNativeStartup(binding product.Options, nonce string, in io.Reader, out io.Writer) error {
	if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(nonce) {
		return errors.New("invalid native startup nonce")
	}
	bound, err := product.Open(binding)
	if err != nil {
		return err
	}
	before, err := startupSnapshot(binding)
	if err != nil {
		return err
	}
	var body bytes.Buffer
	if err := bound.HandleClaudeHook("SessionStart", in, &body); err != nil {
		return err
	}
	after, err := startupSnapshot(binding)
	if err != nil || before != after {
		return errors.New("native startup changed selected state")
	}
	var value map[string]any
	if json.Unmarshal(body.Bytes(), &value) != nil {
		return errors.New("native startup output invalid")
	}
	value["systemMessage"] = "maestro_native_startup_unchanged:" + nonce
	return json.NewEncoder(out).Encode(value)
}
func startupSnapshot(binding product.Options) (string, error) {
	state := map[string]string{}
	count, total := 0, 0
	h, err := datahome.Resolve(datahome.Options{ProductRoot: binding.ProductRoot, UserHome: binding.UserHome, LookupEnv: binding.LookupEnv})
	if err != nil {
		return "", err
	}
	for _, root := range []string{filepath.Join(h.Home, "state"), filepath.Join(binding.WorkspacePath, ".bcgos")} {
		err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			count++
			if count > 4096 {
				return errors.New("startup snapshot limit")
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			key := root + "/" + filepath.ToSlash(relative)
			if entry.IsDir() {
				state[key] = "directory:" + strconv.FormatUint(uint64(info.Mode().Perm()), 8)
				return nil
			}
			if !entry.Type().IsRegular() {
				return errors.New("startup alias refused")
			}
			file, err := scheduler.OpenRegularFile(path)
			if err != nil {
				return err
			}
			body, readErr := io.ReadAll(io.LimitReader(file, (1<<20)+1))
			closeErr := file.Close()
			if readErr != nil || closeErr != nil || len(body) > 1<<20 {
				return errors.New("startup file unavailable")
			}
			total += len(body)
			if total > 8<<20 {
				return errors.New("startup snapshot bytes")
			}
			state[key] = digest(body)
			return nil
		})
		if err != nil {
			return "", err
		}
	}
	body, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return digest(body), nil
}

// NativeFault has two fixed qualification behaviors and no command callback.
func NativeFault(ctx context.Context, mode string) error {
	switch mode {
	case "crash":
		return errors.New("synthetic native guard failure")
	case "timeout":
		select {
		case <-time.After(3 * time.Second):
			return errors.New("synthetic timeout guard completed")
		case <-ctx.Done():
			return ctx.Err()
		}
	default:
		return errors.New("unknown native fault")
	}
}
