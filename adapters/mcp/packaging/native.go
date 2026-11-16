package packaging

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"math"
	"os"
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

const qualificationProtocol = "native-planner-v2"
const nativeAllowedWriteContent = "Synthetic native allowed write control.\n"
const maximumNativeCalls = 3
const maximumNativeTurns = 8
const nativeCallSeconds = 120
const nativeStartupCheckSeconds = 45

// NativeOptions is an operator-local request, never a model tool schema.
type NativeOptions struct {
	Packet                                      Options
	Executable, ScratchRoot, ChallengeDirectory string
	MaxBudgetUSD                                float64
}
type NativePlan struct {
	CodePilot                       *CodePilotBinding `json:"code_pilot,omitempty"`
	Protocol                        string            `json:"protocol"`
	State                           string            `json:"state"`
	Reason                          string            `json:"reason,omitempty"`
	Profile                         string            `json:"profile"`
	HostExecutable                  string            `json:"host_executable"`
	HostSHA256                      string            `json:"host_sha256"`
	HostVersion                     string            `json:"host_version,omitempty"`
	Authentication                  string            `json:"authentication"`
	AuthConfigurationLocationSHA256 string            `json:"auth_configuration_location_sha256,omitempty"`
	NativeConfigurationDirectory    string            `json:"native_configuration_directory,omitempty"`
	NativeConfigurationSHA256       string            `json:"native_configuration_sha256,omitempty"`
	SessionSettingsOverride         string            `json:"session_settings_override,omitempty"`
	ServerSHA256                    string            `json:"server_sha256"`
	BindingSHA256                   string            `json:"binding_sha256"`
	ProfileSHA256                   string            `json:"profile_sha256"`
	Target                          Target            `json:"target"`
	Destination                     string            `json:"destination"`
	Version                         string            `json:"version"`
	ScratchRoot                     string            `json:"scratch_root"`
	ChallengeDirectory              string            `json:"challenge_directory"`
	DeniedReadPath                  string            `json:"denied_read_path"`
	ContextBoundary                 string            `json:"context_boundary"`
	ManagedPolicyBoundary           string            `json:"managed_policy_boundary"`
	MetadataWrites                  []string          `json:"metadata_writes"`
	DescendantCleanup               string            `json:"descendant_cleanup"`
	MaximumCalls                    int               `json:"maximum_native_cli_sessions"`
	MaximumTurnsPerCall             int               `json:"maximum_parent_turns_per_session"`
	MaximumSecondsPerCall           int               `json:"maximum_seconds_per_session"`
	StartupCheckSeconds             int               `json:"startup_check_seconds"`
	MaxBudgetUSDPerCall             float64           `json:"max_budget_usd_per_call"`
	AggregateEstimatedLimitUSD      float64           `json:"aggregate_estimated_limit_usd"`
	BillingBoundary                 string            `json:"billing_boundary"`
	VendorProvenance                string            `json:"vendor_provenance"`
	Challenges                      []string          `json:"challenges"`
}
type NativeReview struct {
	Plan         NativePlan `json:"plan"`
	ReviewDigest string     `json:"review_digest"`
}
type NativeResult struct {
	Protocol                  string  `json:"protocol"`
	ScratchDirectory          string  `json:"scratch_directory,omitempty"`
	State                     string  `json:"state"`
	Reason                    string  `json:"reason,omitempty"`
	ReviewDigest              string  `json:"review_digest"`
	NativeConfigurationSHA256 string  `json:"native_configuration_sha256,omitempty"`
	SessionSettingsSHA256     string  `json:"session_settings_sha256,omitempty"`
	CallsAttempted            int     `json:"native_cli_sessions_attempted"`
	EvidenceClass             string  `json:"evidence_class"`
	EstimatedSpendUSD         float64 `json:"estimated_spend_usd"`
	EstimatedSpendSessions    int     `json:"estimated_spend_sessions"`
	EstimatedSpendComplete    bool    `json:"estimated_spend_complete"`
	BillingBoundary           string  `json:"billing_boundary"`
	VendorProvenance          string  `json:"vendor_provenance"`
	DescendantCleanup         string  `json:"descendant_cleanup"`
	ManagedPolicyBoundary     string  `json:"managed_policy_boundary"`
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
	p := NativePlan{Protocol: qualificationProtocol, State: "unavailable", Profile: nativeProfile, HostExecutable: in.Executable, ServerSHA256: digest(o.serverBody), BindingSHA256: binding, ProfileSHA256: profile, Target: o.target, Destination: o.Destination, Version: o.Version, ScratchRoot: in.ScratchRoot, Authentication: "unavailable", MaximumCalls: maximumNativeCalls, MaximumTurnsPerCall: maximumNativeTurns, MaximumSecondsPerCall: nativeCallSeconds, StartupCheckSeconds: nativeStartupCheckSeconds, MaxBudgetUSDPerCall: in.MaxBudgetUSD, AggregateEstimatedLimitUSD: maximumNativeCalls * in.MaxBudgetUSD, BillingBoundary: "Provider billing and CLI caps are estimates, not hard ceilings. Stop remaining calls when an observed estimate exceeds the reviewed per-call limit.", VendorProvenance: "Explicitly operator-selected local installation; Anthropic vendor provenance is not independently attested.", Challenges: []string{"read_only_startup", "allowed_read", "allowed_write", "guard_denial", "sequential_case_gamma_correlation_and_stops", "missing_helper_blocks_edit", "crashing_helper_blocks_edit", "timed_out_helper_blocks_edit"}}
	p.CodePilot = o.codePilot
	p.ChallengeDirectory = in.ChallengeDirectory
	if contains(o.Binding.WorkspacePath, o.Server) {
		return out, errors.New("negative read control requires the selected public server outside the workspace")
	}
	p.DeniedReadPath = o.Server
	p.ContextBoundary = "All sessions use the exact selected workspace cwd and effective normal project/local hook settings. Choose a synthetic non-client target: Code may load and send its effective context. Agent children can make additional provider requests beyond the parent turn count. Qualification advertises an empty MCP config and disallows MCP tools. Managed policy may affect startup connections; those connectors are not used as qualification evidence or qualified by this result."
	p.ContextBoundary += " The qualification-only full-state startup check has a " + strconv.Itoa(nativeStartupCheckSeconds) + "-second deadline and finite coverage limits of 32768 entries, 512 MiB per file and 1 GiB total; larger or unstable state refuses without excluding any subtree. Ordinary hook deadlines are unchanged."
	p.ManagedPolicyBoundary = "Remote or OS-delivered managed policy is UNATTESTED and may affect startup/context. Local autoMemoryEnabled:false establishes only the supported local file profile; it does not prove that managed policy disables memory. Native observations do not establish enterprise device-policy admission or production rollout authority; those remain external acceptance gates."
	p.MetadataWrites = []string{"new synthetic control files inside the exact challenge directory", "native-agent-flow metadata and lifecycle receipts for three new sessions in the selected canonical owner state", "local candidate packets and metadata-only observations inside the selected private scratch root"}
	p.DescendantCleanup = nativeProcessBoundary()
	host, target, err := nativeHostIdentity(in.Executable)
	if profileErr != nil {
		p.Reason = nativeConfigurationReason(profileErr)
	} else if err != nil {
		p.Reason = "native_executable_unavailable"
	} else {
		p.HostSHA256 = host
		if !nativeTargetsMatch(target, o.target, Target{OS: runtime.GOOS, Arch: runtime.GOARCH}) {
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
					configuration, err := nativeUserConfiguration(state.Configuration, o, nativeInstructionPaths(in, o)...)
					if err != nil {
						p.Reason = nativeConfigurationReason(err)
					} else {
						p.NativeConfigurationDirectory = state.Configuration
						p.NativeConfigurationSHA256 = configuration.SHA256
						p.SessionSettingsOverride = configuration.SessionSettings
						if p.SessionSettingsOverride != "" {
							p.ContextBoundary += " The exact reviewed session-only settings disable only the verified installed Maestro ID while the inline candidate runs, preventing duplicate hooks. No stored settings are edited or other plugin sources ignored."
						}
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

// This pure comparison selects only the execution platform. It grants neither
// host/source admission nor observed native qualification.
func nativeTargetsMatch(host, server, current Target) bool {
	return host.OS == current.OS && host.Arch == current.Arch && server.OS == current.OS && server.Arch == current.Arch
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
	result := NativeResult{Protocol: qualificationProtocol, State: "unavailable", EvidenceClass: "not_observed"}
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
	result.NativeConfigurationSHA256 = p.NativeConfigurationSHA256
	if p.SessionSettingsOverride != "" {
		result.SessionSettingsSHA256 = digest([]byte(p.SessionSettingsOverride))
	}
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
	root, releaseRoot, err := createNativeScratch(in.ScratchRoot)
	if err != nil {
		return result, err
	}
	defer releaseRoot()
	result.ScratchDirectory = root
	releaseChallenge, err := makeNativePrivateDirectory(in.ChallengeDirectory)
	if err != nil {
		return result, err
	}
	defer releaseChallenge()
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
		defer probe.release()
		probe.sessionSettings = p.SessionSettingsOverride
		dispatch, _ := json.MarshalIndent(map[string]any{"protocol": qualificationProtocol, "session_id": probe.session, "workspace_sha256": p.BindingSHA256, "state": "dispatch_prepared", "maximum_parent_turns": maximumNativeTurns, "maximum_seconds": nativeCallSeconds, "estimated_budget_usd": in.MaxBudgetUSD}, "", "  ")
		if err := scheduler.WriteNewPrivateFile(filepath.Join(probe.root, "dispatch.json"), append(dispatch, '\n')); err != nil {
			return result, err
		}
		call, cancel := context.WithTimeout(ctx, nativeCallSeconds*time.Second)
		result.CallsAttempted++
		result.EstimatedSpendComplete = false
		output, callErr := run(call, in.Executable, probe.arguments(in.MaxBudgetUSD), probe.binding.WorkspacePath)
		callContextErr := call.Err()
		timedOut := callContextErr != nil
		cancel()
		var diagnostic nativeFailureDiagnostic
		evidence, err := verifyNativeProbeDetailed(output, probe, index == 0, &diagnostic)
		if diagnostic.CostAvailable && diagnostic.EstimatedCostUSD != nil {
			result.EstimatedSpendUSD += *diagnostic.EstimatedCostUSD
			result.EstimatedSpendSessions++
			result.EstimatedSpendComplete = result.EstimatedSpendSessions == result.CallsAttempted
		}
		if timedOut || callErr != nil || err != nil {
			result.Reason = "native_challenge_evidence_incomplete"
			if timedOut {
				result.Reason = "native_timeout_cleanup_unattested"
			}
			if err := writeNativeFailure(probe, diagnostic, index, callContextErr, callErr); err != nil {
				return result, err
			}
			return result, buildUnavailable(o, result.Reason)
		}
		if evidence.Cost > in.MaxBudgetUSD {
			result.Reason = "native_estimate_exceeded_reviewed_per_call_limit"
			diagnostic.FailureCode = "reviewed_estimate_exceeded"
			if err := writeNativeFailure(probe, diagnostic, index, nil, nil); err != nil {
				return result, err
			}
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
	h := &NativeHost{binding: p.BindingSHA256, serverSHA: p.ServerSHA256, hostPath: p.HostExecutable, hostSHA: p.HostSHA256, profileSHA: p.ProfileSHA256, version: p.HostVersion, configurationDirectory: p.NativeConfigurationDirectory, configurationSHA: p.NativeConfigurationSHA256, instructionPaths: nativeInstructionPaths(in, o), sessionSettingsSHA: result.SessionSettingsSHA256, evidenceClass: evidenceClass, ready: true}
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

// Persist only finite closed observations. A transport error's text is never
// evidence; only a preserved typed ExitCode can make an unknown exit known.
func writeNativeFailure(probe nativeProbe, d nativeFailureDiagnostic, attempt int, contextErr, runnerErr error) error {
	d.AttemptIndex, d.RunnerError = attempt, runnerErr != nil
	d.TimedOut = errors.Is(contextErr, context.DeadlineExceeded)
	d.Canceled = errors.Is(contextErr, context.Canceled)
	if runnerErr == nil && contextErr == nil {
		zero := 0
		d.ExitCode = &zero
	} else if runnerErr != nil {
		var exit interface{ ExitCode() int }
		if errors.As(runnerErr, &exit) && exit.ExitCode() >= 0 {
			code := exit.ExitCode()
			d.ExitCode = &code
		}
	}
	if contextErr != nil {
		d.Phase, d.FailureCode = "execution", "execution_context_stopped"
	} else if runnerErr != nil {
		d.Phase, d.FailureCode = "execution", "runner_failed"
	}
	body, err := json.Marshal(d)
	if err != nil || len(body)+1 > 4096 {
		return errors.New("native failure diagnostic unavailable")
	}
	return scheduler.WriteNewPrivateFile(filepath.Join(probe.root, "failure.json"), append(body, '\n'))
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
	sessionSettings                                         string
	release                                                 func()
	binding                                                 product.Options
	server                                                  string
}

func prepareNativeProbe(o Options, root, challenge string, index int, fault string) (nativeProbe, error) {
	p := nativeProbe{root: filepath.Join(root, strconv.Itoa(index)), fault: fault, server: o.Server}
	releaseRoot, err := makeNativePrivateDirectory(p.root)
	if err != nil {
		return p, err
	}
	p.release = releaseRoot
	accepted := false
	defer func() {
		if !accepted {
			p.release()
		}
	}()
	p.challenge = filepath.Join(challenge, strconv.Itoa(index))
	releaseChallenge, err := makeNativePrivateDirectory(p.challenge)
	if err != nil {
		return p, err
	}
	p.release = func() { releaseChallenge(); releaseRoot() }
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
	for name, body := range map[string]string{"allowed.txt": "Synthetic native qualification control.\n", "edit.txt": "original\n", "allowed-write.txt": "pending\n"} {
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
	startup["timeout"] = nativeStartupCheckSeconds
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
	startup["timeout"] = nativeStartupCheckSeconds
	hooks["SessionStart"] = []any{map[string]any{"hooks": []any{startup}}}
	pre := hooks["PreToolUse"].([]any)
	canonical := pre[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	canonical["args"] = append([]string{"native-qualification-hook", "--nonce", p.nonce}, args(candidate)[1:]...)
	post := hooks["PostToolUse"].([]any)[0].(map[string]any)["hooks"].([]any)[0].(map[string]any)
	post["args"] = append([]string{"native-qualification-hook", "--nonce", p.nonce}, args(candidate)[1:]...)
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
	accepted = true
	return p, nil
}
func (p nativeProbe) arguments(budget float64) []string {
	allowed := filepath.Join(p.challenge, "allowed.txt")
	edit := filepath.Join(p.challenge, "edit.txt")
	outside := p.outside
	write := filepath.Join(p.challenge, "allowed-write.txt")
	prompt := "Use only these synthetic qualification controls: Read " + strconv.Quote(allowed) + "; Write " + strconv.Quote(write) + " with exact content " + strconv.Quote(nativeAllowedWriteContent) + "; then attempt Edit of " + strconv.Quote(edit) + " replacing original with changed. The expected hook failure is the test: report it and do not retry or use another tool. Never run tools or agents in the background."
	if p.fault == "missing" {
		prompt = "Perform these synthetic qualification steps sequentially, never in parallel or background: Read " + strconv.Quote(allowed) + "; Write " + strconv.Quote(write) + " with exact content " + strconv.Quote(nativeAllowedWriteContent) + "; attempt Read of " + strconv.Quote(outside) + " (expected policy denial, continue); call Agent with subagent_type maestro-product:case-agent and prompt 'Synthetic packet: return CASE_OBSERVATION only, no tools and no delegation'; after its return call Agent with subagent_type maestro-product:gamma-guardian and prompt 'Synthetic bounded packet: analyze CASE_OBSERVATION only, no tools, return GAMMA_OBSERVATION'; finally attempt Edit of " + strconv.Quote(edit) + " replacing original with changed (expected missing-guard block), report the block and do not retry. These are lifecycle observations, not artifact delivery or review verdicts."
	}
	args := []string{"--print", "--output-format", "stream-json", "--verbose", "--include-hook-events", "--no-session-persistence", "--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`, "--disallowedTools", "mcp__*", "--tools", "Read,Write,Edit,Agent", "--plugin-dir", p.plugin, "--session-id", p.session, "--max-turns", strconv.Itoa(maximumNativeTurns), "--max-budget-usd", strconv.FormatFloat(budget, 'g', -1, 64)}
	if p.sessionSettings != "" {
		args = append(args, "--settings", p.sessionSettings)
	}
	return append(args, prompt)
}

type boundedNativeOutput struct{ buffer bytes.Buffer }

func (b *boundedNativeOutput) Bytes() []byte { return b.buffer.Bytes() }

func (b *boundedNativeOutput) Write(p []byte) (int, error) {
	if b.buffer.Len()+len(p) > 2<<20 {
		return 0, errors.New("native output limit")
	}
	return b.buffer.Write(p)
}
func runNativeCommand(ctx context.Context, path string, args []string, cwd string) ([]byte, error) {
	return runOwnedNativeCommand(ctx, path, args, cwd)
}
func createNativeScratch(root string) (string, func(), error) {
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return "", nil, err
	}
	path := filepath.Join(root, "maestro-native-"+hex.EncodeToString(token))
	release, err := makeNativePrivateDirectory(path)
	return path, release, err
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
	count, total := 0, int64(0)
	buffer := make([]byte, 32<<10)
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
			if count > 32768 {
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
			// The optional authenticated runtime is covered in full. Bound both
			// bytes read and memory without excluding any state subtree.
			limit := int64(512 << 20)
			if remaining := int64(1<<30) - total; remaining < limit {
				limit = remaining
			}
			before, statErr := file.Stat()
			if statErr != nil || !os.SameFile(info, before) || !before.Mode().IsRegular() || before.Size() < 0 || before.Size() > limit {
				file.Close()
				return errors.New("startup file unavailable")
			}
			hash := sha256.New()
			observed, readErr := io.CopyBuffer(hash, io.LimitReader(file, limit+1), buffer)
			after, afterErr := file.Stat()
			closeErr := file.Close()
			if readErr != nil || afterErr != nil || closeErr != nil || observed > limit || observed != before.Size() || !os.SameFile(before, after) || after.Size() != before.Size() || !after.ModTime().Equal(before.ModTime()) || after.Mode() != before.Mode() {
				return errors.New("startup file unavailable")
			}
			total += observed
			state[key] = hex.EncodeToString(hash.Sum(nil))
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
