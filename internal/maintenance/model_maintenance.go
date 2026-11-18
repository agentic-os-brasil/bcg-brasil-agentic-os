package maintenance

import (
	"errors"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

// WeeklyModelDescriptor is closed, trusted configuration, never qualification or
// authority. No default provider, model, profile, or credential mode is selected.
// Its exact digest must be owner accepted, genuinely probed, and bound by the
// existing product qualification and private occurrence grant before invocation.
type WeeklyModelDescriptor struct {
	RuntimeIdentity        *processidentity.WeeklyNativeDeclaration `json:"runtime_identity,omitempty"`
	SchemaVersion          int                                      `json:"schema_version"`
	JobID                  string                                   `json:"job_id"`
	Runtime                string                                   `json:"runtime"`
	RuntimeVersion         string                                   `json:"runtime_version"`
	Executable             string                                   `json:"executable"`
	ExecutableSHA256       string                                   `json:"executable_sha256"`
	ConfigurationSHA256    string                                   `json:"configuration_sha256"`
	ManagedPolicySHA256    string                                   `json:"managed_policy_sha256"`
	ProfileReferenceSHA256 string                                   `json:"profile_reference_sha256"`
	TransportSHA256        string                                   `json:"transport_sha256"`
	Provider               string                                   `json:"provider"`
	Model                  string                                   `json:"model"`
	CredentialMode         string                                   `json:"credential_mode"`
	ValidatorVersion       string                                   `json:"validator_version"`
	Limits                 WeeklyModelLimits                        `json:"limits"`
}

type WeeklyModelLimits struct {
	MaxInputBytes   int `json:"max_input_bytes"`
	MaxOutputBytes  int `json:"max_output_bytes"`
	MaxStderrBytes  int `json:"max_stderr_bytes"`
	MaxOutputTokens int `json:"max_output_tokens"`
	TimeoutSeconds  int `json:"timeout_seconds"`
	Attempts        int `json:"attempts"`
	AgenticTurns    int `json:"agentic_turns"`
}

func (d WeeklyModelDescriptor) Validate() error {
	if d.RuntimeIdentity != nil {
		if err := d.RuntimeIdentity.Validate(); err != nil {
			return err
		}
	}
	if d.SchemaVersion != 1 || d.JobID != YodaSelfReviewWeeklyJobID || d.Runtime != "claude_code_cli" || d.CredentialMode != "runtime_owned_session" || d.ValidatorVersion != "weekly-self-v1" {
		return errors.New("weekly model descriptor is unsupported")
	}
	for _, v := range []string{d.RuntimeVersion, d.Provider, d.Model} {
		if v == "" || len(v) > 128 || !utf8.ValidString(v) || strings.ContainsAny(v, "\x00\r\n") || strings.TrimSpace(v) != v || strings.HasPrefix(v, "-") {
			return errors.New("weekly model exact runtime/provider/model is undeclared")
		}
	}
	if !filepath.IsAbs(d.Executable) || filepath.Clean(d.Executable) != d.Executable {
		return errors.New("weekly model executable must be an exact absolute path")
	}
	for _, v := range []string{d.ExecutableSHA256, d.ConfigurationSHA256, d.ManagedPolicySHA256, d.ProfileReferenceSHA256, d.TransportSHA256} {
		if !digestPattern.MatchString(v) {
			return errors.New("weekly model tuple digest is missing or invalid")
		}
	}
	l := d.Limits
	if l.MaxInputBytes < 1 || l.MaxInputBytes > 32768 || l.MaxOutputBytes < 1 || l.MaxOutputBytes > 32768 || l.MaxStderrBytes < 1 || l.MaxStderrBytes > 4096 || l.MaxOutputTokens < 1 || l.MaxOutputTokens > 8192 || l.TimeoutSeconds < 1 || l.TimeoutSeconds > 120 || l.Attempts != 1 || l.AgenticTurns != 1 {
		return errors.New("weekly model bounds exceed the fixed one-call tool-free contract")
	}
	return nil
}
func (d WeeklyModelDescriptor) Digest() string { return productDigest(d) }
func ParseWeeklyModelDescriptor(body []byte) (WeeklyModelDescriptor, error) {
	var d WeeklyModelDescriptor
	if len(body) > 16384 {
		return d, errors.New("weekly descriptor exceeds bound")
	}
	if err := releasecontract.DecodeStrictJSON(body, &d); err != nil {
		return d, err
	}
	return d, d.Validate()
}

// weeklyModelBinding is an optional private extension of the EXISTING grant.
// Only the maintenance owner may attach it after checking actual qualification.
// This declaration alone issues no grant and cannot make an ordinary job eligible.
type WeeklyModelMode string

const (
	WeeklyNativeProbe WeeklyModelMode = "native_probe"
	WeeklyWorking     WeeklyModelMode = "weekly_working"
)

type weeklyModelBinding struct {
	Mode                WeeklyModelMode
	OwnerScope          string
	ProbeIntentSHA256   string
	TerminalOperation   string
	DescriptorSHA256    string
	InputSHA256         string
	QualificationSHA256 string
	ActivationSHA256    string
	PolicySHA256        string
}
type weeklyModelBoundGrant interface{ weeklyModelBinding() weeklyModelBinding }

// ValidateWeeklyModelExecution does not issue or broaden an ExecutionGrant.
// Old grants have no model binding and remain unavailable for model execution.
func ValidateWeeklyModelExecution(grant ExecutionGrant, command Command, descriptor WeeklyModelDescriptor, inputSHA256, policySHA256 string) error {
	if err := descriptor.Validate(); err != nil {
		return err
	}
	if command.JobID != YodaSelfReviewWeeklyJobID || command.Trigger != TriggerWeekly || (!digestPattern.MatchString(inputSHA256) || !digestPattern.MatchString(policySHA256)) {
		return errors.New("weekly model command/input binding is invalid")
	}
	if err := ValidateExecutionGrant(grant, command); err != nil {
		return err
	}
	bound, ok := grant.(weeklyModelBoundGrant)
	if !ok {
		return errors.New("weekly model qualification binding is unavailable")
	}
	b := bound.weeklyModelBinding()
	if command.WeeklyMode != b.Mode || command.ProposalOnly || b.OwnerScope != command.WorkspaceID || b.DescriptorSHA256 != descriptor.Digest() || b.InputSHA256 != inputSHA256 || b.PolicySHA256 != policySHA256 {
		return errors.New("weekly model exact owner/descriptor/input/policy binding is unavailable")
	}
	switch b.Mode {
	case WeeklyNativeProbe:
		if b.TerminalOperation != "qualification_evidence_only" || !digestPattern.MatchString(b.ProbeIntentSHA256) || b.QualificationSHA256 != "" || b.ActivationSHA256 != "" {
			return errors.New("weekly evidence-only probe binding is invalid")
		}
	case WeeklyWorking:
		if b.TerminalOperation != "working_projection_only" || b.ProbeIntentSHA256 != "" || !digestPattern.MatchString(b.QualificationSHA256) || !digestPattern.MatchString(b.ActivationSHA256) {
			return errors.New("weekly working activation/qualification binding is unavailable")
		}
	default:
		return errors.New("weekly model mode is unavailable")
	}

	return nil
}

// WeeklyModelTransportDigest binds the exact nonsecret working-directory and
// closed environment values. It confers no trust or runtime authority.
func WeeklyModelTransportDigest(directory string, environment []string) string {
	return productDigest(struct {
		Directory   string   `json:"directory"`
		Environment []string `json:"environment"`
	}{directory, environment})
}

// WeeklyModelExecutionMode exposes no selector or issuer. It reads only an
// existing live, private worker binding and is unavailable on ordinary grants.
func WeeklyModelExecutionMode(grant ExecutionGrant, command Command) (WeeklyModelMode, error) {
	if err := ValidateExecutionGrant(grant, command); err != nil {
		return "", err
	}
	bounded, ok := grant.(executionGrant)
	if !ok || bounded.weekly == nil || command.JobID != YodaSelfReviewWeeklyJobID {
		return "", errors.New("weekly private grant is unavailable")
	}
	mode := bounded.weekly.Mode
	if mode != WeeklyNativeProbe && mode != WeeklyWorking {
		return "", errors.New("weekly private mode is invalid")
	}
	return mode, nil
}

// ValidateWeeklyModelEnvironment is shared by declaration and invocation so
// credentials and command overrides are rejected before review or persistence.
func ValidateWeeklyModelEnvironment(values []string) error {
	allowed := map[string]bool{"HOME": true, "USERPROFILE": true, "APPDATA": true, "LOCALAPPDATA": true, "SystemRoot": true, "SYSTEMROOT": true, "TEMP": true, "TMP": true, "PATH": true, "CLAUDE_CONFIG_DIR": true, "LANG": true, "LC_ALL": true}
	seen := map[string]bool{}
	for _, entry := range values {
		key, value, ok := strings.Cut(entry, "=")
		fold := strings.ToUpper(key)
		if !ok || !allowed[key] || seen[fold] || value == "" || strings.ContainsAny(value, "\x00\r\n") || !utf8.ValidString(value) {
			return errors.New("weekly environment contains an unsupported, duplicate or invalid entry")
		}
		seen[fold] = true
	}
	return nil
}
