// Package darwin defines the bounded operational contract for Darwin 🧬.
//
// Darwin is a governance surgeon, not a second user-facing assistant. The
// package owns the closed health packet, prioritized remediation plan, scoped
// tool-call envelope and metadata-only execution receipt. Runtime adapters own
// the native tool implementation.
package darwin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
)

const (
	SchemaVersion    = 1
	AgentID          = "darwin"
	Role             = "governance_analyst"
	DisplayName      = "Darwin"
	Emoji            = "🧬"
	MaintenanceScope = "maestro-system"
	ScopeKind        = "health"
	maxObservations  = 32
	maxActions       = 3
)

type Mode string

const (
	Interactive          Mode = "interactive"
	HeadlessHousekeeping Mode = "headless_housekeeping"
	DeepReview           Mode = "deep_review"
)

type Severity string

const (
	SeverityLow    Severity = "low"
	SeverityMedium Severity = "medium"
	SeverityHigh   Severity = "high"
)

type ObservationCode string

const (
	ObservationCapabilityUnavailable ObservationCode = "capability_unavailable"
	ObservationStateStale            ObservationCode = "state_stale"
	ObservationSchedulerMissed       ObservationCode = "scheduler_missed"
	ObservationContractDrift         ObservationCode = "contract_drift"
	ObservationValidationFailure     ObservationCode = "validation_failure"
	ObservationOperatingFriction     ObservationCode = "operating_friction"
)

type Action string

const (
	ActionRecordCapabilityGap   Action = "record_capability_gap"
	ActionRefreshDerivedState   Action = "refresh_derived_state"
	ActionReconcileScheduler    Action = "reconcile_scheduler_receipt"
	ActionRunContractValidation Action = "run_contract_validation"
)

type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	OutcomeBlocked   Outcome = "blocked"
	OutcomeNoAction  Outcome = "no_action"
	OutcomePartial   Outcome = "partial"
)

type Impact string

const (
	ImpactReliability Impact = "reliability"
	ImpactRecovery    Impact = "recovery"
	ImpactSafety      Impact = "safety"
	ImpactFriction    Impact = "friction"
)

type Effort string

const (
	EffortSmall  Effort = "small"
	EffortMedium Effort = "medium"
)

type Risk string

const (
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
)

type HealthPacket struct {
	SchemaVersion int           `json:"schema_version"`
	WindowID      string        `json:"window_id"`
	Runtime       string        `json:"runtime"`
	Mode          Mode          `json:"mode"`
	Observations  []Observation `json:"observations"`
}

type Observation struct {
	Code     ObservationCode `json:"code"`
	Severity Severity        `json:"severity"`
	Count    int             `json:"count"`
	State    string          `json:"state,omitempty"`
}

type Proposal struct {
	ID         string          `json:"id"`
	Finding    ObservationCode `json:"finding"`
	Priority   int             `json:"priority"`
	Impact     Impact          `json:"impact"`
	Effort     Effort          `json:"effort"`
	Risk       Risk            `json:"risk"`
	Action     Action          `json:"action"`
	Reversible bool            `json:"reversible"`
	Rollback   Action          `json:"rollback"`
}

type Assessment struct {
	SchemaVersion int        `json:"schema_version"`
	AgentID       string     `json:"agent_id"`
	DisplayName   string     `json:"display_name"`
	Emoji         string     `json:"emoji"`
	WindowID      string     `json:"window_id"`
	Mode          Mode       `json:"mode"`
	Proposals     []Proposal `json:"proposals"`
}

type ToolCall struct {
	Tool      string `json:"tool"`
	Operation string `json:"operation"`
	Resource  string `json:"resource"`
}

type Artifact struct {
	SchemaVersion int             `json:"schema_version"`
	AgentID       string          `json:"agent_id"`
	WindowID      string          `json:"window_id"`
	ProposalID    string          `json:"proposal_id"`
	Finding       ObservationCode `json:"finding"`
	Action        Action          `json:"action"`
}

type ToolResult struct {
	Outcome Outcome
}

type Receipt struct {
	SchemaVersion int             `json:"schema_version"`
	AgentID       string          `json:"agent_id"`
	DisplayName   string          `json:"display_name"`
	Emoji         string          `json:"emoji"`
	WindowID      string          `json:"window_id"`
	Mode          Mode            `json:"mode"`
	Outcome       Outcome         `json:"outcome"`
	Actions       []ActionReceipt `json:"actions,omitempty"`
	RecordedAt    time.Time       `json:"recorded_at"`
}

type ActionReceipt struct {
	ProposalID string  `json:"proposal_id"`
	Action     Action  `json:"action"`
	Tool       string  `json:"tool"`
	Operation  string  `json:"operation"`
	Resource   string  `json:"resource"`
	Outcome    Outcome `json:"outcome"`
	Rollback   Action  `json:"rollback"`
}

type ToolGuard interface {
	Authorize(ToolCall) error
}

type ToolInvoker interface {
	Invoke(context.Context, ToolCall, Artifact) (ToolResult, error)
}

type ToolGuardFunc func(ToolCall) error

func (function ToolGuardFunc) Authorize(call ToolCall) error { return function(call) }

type InvokerFunc func(context.Context, ToolCall, Artifact) (ToolResult, error)

func (function InvokerFunc) Invoke(ctx context.Context, call ToolCall, artifact Artifact) (ToolResult, error) {
	return function(ctx, call, artifact)
}

var (
	idPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)
	validRuntimes   = map[string]bool{"claude": true, "codex": true, "runtime-neutral": true}
	validSeverities = map[Severity]bool{SeverityLow: true, SeverityMedium: true, SeverityHigh: true}
	validStates     = map[string]bool{"": true, "native": true, "adapter": true, "configured": true, "derived": true, "stale": true, "missing": true, "blocked": true, "healthy": true, "warning": true, "failed": true, "unavailable": true}
	validCodes      = map[ObservationCode]bool{
		ObservationCapabilityUnavailable: true,
		ObservationStateStale:            true,
		ObservationSchedulerMissed:       true,
		ObservationContractDrift:         true,
		ObservationValidationFailure:     true,
		ObservationOperatingFriction:     true,
	}
)

func (packet HealthPacket) Validate() error {
	if packet.SchemaVersion != SchemaVersion || !idPattern.MatchString(packet.WindowID) || !validRuntimes[packet.Runtime] || !validMode(packet.Mode) {
		return errors.New("Darwin health packet header is invalid")
	}
	if len(packet.Observations) > maxObservations {
		return errors.New("Darwin health packet observations are oversized")
	}
	seen := map[ObservationCode]bool{}
	for _, observation := range packet.Observations {
		if !validCodes[observation.Code] || !validSeverities[observation.Severity] || observation.Count < 1 || observation.Count > 1000 || seen[observation.Code] {
			return errors.New("Darwin health packet contains an invalid or duplicate observation")
		}
		if !validStates[observation.State] || len([]byte(observation.State)) > 64 || strings.ContainsAny(observation.State, "\r\n") {
			return errors.New("Darwin health packet contains an invalid state label")
		}
		seen[observation.Code] = true
	}
	return nil
}

func Plan(packet HealthPacket) (Assessment, error) {
	if err := packet.Validate(); err != nil {
		return Assessment{}, err
	}
	observations := append([]Observation(nil), packet.Observations...)
	sort.SliceStable(observations, func(left, right int) bool {
		leftRank, rightRank := severityRank(observations[left].Severity), severityRank(observations[right].Severity)
		if leftRank != rightRank {
			return leftRank > rightRank
		}
		if observations[left].Count != observations[right].Count {
			return observations[left].Count > observations[right].Count
		}
		return observations[left].Code < observations[right].Code
	})
	proposals := make([]Proposal, 0, min(maxActions, len(observations)))
	for index, observation := range observations {
		if index >= maxActions {
			break
		}
		proposal := proposalFor(observation)
		proposal.Priority = index + 1
		proposal.ID = proposalID(packet.WindowID, observation)
		proposals = append(proposals, proposal)
	}
	return Assessment{SchemaVersion: SchemaVersion, AgentID: AgentID, DisplayName: DisplayName, Emoji: Emoji, WindowID: packet.WindowID, Mode: packet.Mode, Proposals: proposals}, nil
}

func Execute(ctx context.Context, packet HealthPacket, assessment Assessment, guard ToolGuard, invoker ToolInvoker, now func() time.Time) (Receipt, error) {
	if err := packet.Validate(); err != nil {
		return Receipt{}, err
	}
	if assessment.SchemaVersion != SchemaVersion || assessment.AgentID != AgentID || assessment.DisplayName != DisplayName || assessment.WindowID != packet.WindowID || assessment.Mode != packet.Mode || assessment.Emoji != Emoji {
		return Receipt{}, errors.New("Darwin assessment is not bound to the health packet")
	}
	expected, err := Plan(packet)
	if err != nil || !reflect.DeepEqual(assessment.Proposals, expected.Proposals) {
		return Receipt{}, errors.New("Darwin assessment does not match the deterministic remediation plan")
	}
	if guard == nil || invoker == nil {
		return Receipt{}, errors.New("Darwin execution requires a tool guard and invoker")
	}
	if now == nil {
		now = time.Now
	}
	receipt := Receipt{SchemaVersion: SchemaVersion, AgentID: AgentID, DisplayName: DisplayName, Emoji: Emoji, WindowID: packet.WindowID, Mode: packet.Mode, Outcome: OutcomeNoAction, RecordedAt: now().UTC()}
	for _, proposal := range assessment.Proposals {
		if err := ctx.Err(); err != nil {
			receipt.Outcome = summarize(receipt.Actions)
			return receipt, err
		}
		call := callFor(packet, proposal)
		entry := ActionReceipt{ProposalID: proposal.ID, Action: proposal.Action, Tool: call.Tool, Operation: call.Operation, Resource: call.Resource, Outcome: OutcomeBlocked, Rollback: proposal.Rollback}
		if !proposal.Reversible {
			receipt.Actions = append(receipt.Actions, entry)
			continue
		}
		if err := guard.Authorize(call); err != nil {
			receipt.Actions = append(receipt.Actions, entry)
			continue
		}
		if err := ctx.Err(); err != nil {
			receipt.Actions = append(receipt.Actions, entry)
			receipt.Outcome = summarize(receipt.Actions)
			return receipt, err
		}
		result, err := invoker.Invoke(ctx, call, Artifact{SchemaVersion: SchemaVersion, AgentID: AgentID, WindowID: packet.WindowID, ProposalID: proposal.ID, Finding: proposal.Finding, Action: proposal.Action})
		if err != nil || result.Outcome != OutcomeSucceeded {
			entry.Outcome = OutcomeFailed
			receipt.Actions = append(receipt.Actions, entry)
			continue
		}
		entry.Outcome = OutcomeSucceeded
		receipt.Actions = append(receipt.Actions, entry)
	}
	receipt.Outcome = summarize(receipt.Actions)
	return receipt, nil
}

func Authorization(capability string) agentorchestration.Authorization {
	return agentorchestration.Authorization{
		AgentID: AgentID, Role: Role, Scope: MaintenanceScope, ScopeKind: ScopeKind, Capability: capability,
		Tools: []agentorchestration.ToolGrant{
			{Tool: "filesystem", Operation: "read", ResourcePrefix: "bcgos://health/maestro-system/"},
			{Tool: "filesystem", Operation: "write", ResourcePrefix: "bcgos://health/maestro-system/"},
			{Tool: "filesystem", Operation: "edit", ResourcePrefix: "bcgos://health/maestro-system/"},
			{Tool: "probe", Operation: "execute", ResourcePrefix: "bcgos://health/maestro-system/"},
			{Tool: "validation", Operation: "run", ResourcePrefix: "bcgos://health/maestro-system/"},
		},
	}
}

func AdapterGuard(adapter *agentorchestration.Adapter, capability, branchID, dispatchID string) ToolGuard {
	return ToolGuardFunc(func(call ToolCall) error {
		if adapter == nil {
			return errors.New("Darwin adapter guard is unavailable")
		}
		decision := adapter.GuardTool(AgentID, capability, branchID, dispatchID, MaintenanceScope, ScopeKind, call.Tool, call.Operation, call.Resource)
		if !decision.Allowed {
			return fmt.Errorf("Darwin tool denied: %s", decision.Code)
		}
		return nil
	})
}

type FilesystemInvoker struct {
	Root string
}

func (invoker FilesystemInvoker) Invoke(ctx context.Context, call ToolCall, artifact Artifact) (ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return ToolResult{}, err
	}
	if call.Tool != "filesystem" || (call.Operation != "write" && call.Operation != "edit") {
		return ToolResult{}, errors.New("filesystem invoker accepts only bounded write or edit calls")
	}
	if strings.TrimSpace(invoker.Root) == "" {
		return ToolResult{}, errors.New("Darwin filesystem root is required")
	}
	parsed, err := url.Parse(call.Resource)
	if err != nil || parsed.Scheme != "bcgos" || parsed.Host != ScopeKind || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.User != nil || parsed.Opaque != "" {
		return ToolResult{}, errors.New("Darwin filesystem resource is invalid")
	}
	const scopePrefix = "/maestro-system/"
	if !strings.HasPrefix(parsed.Path, scopePrefix) || strings.Contains(parsed.Path, "..") || pathpkg.Clean(parsed.Path) != parsed.Path {
		return ToolResult{}, errors.New("Darwin filesystem resource escapes the maintenance scope")
	}
	relative := strings.TrimPrefix(parsed.Path, scopePrefix)
	if relative == "" || !idPattern.MatchString(pathpkg.Base(relative)) {
		return ToolResult{}, errors.New("Darwin filesystem resource has an unsafe artifact name")
	}
	if artifact.SchemaVersion != SchemaVersion || artifact.AgentID != AgentID || artifact.WindowID == "" || artifact.ProposalID == "" {
		return ToolResult{}, errors.New("Darwin artifact is invalid")
	}
	body := fmt.Sprintf("{\"schema_version\":%d,\"agent_id\":%q,\"window_id\":%q,\"proposal_id\":%q,\"finding\":%q,\"action\":%q}\n", artifact.SchemaVersion, artifact.AgentID, artifact.WindowID, artifact.ProposalID, artifact.Finding, artifact.Action)
	destination := filepath.Join(invoker.Root, relative)
	if err := os.MkdirAll(invoker.Root, 0o700); err != nil {
		return ToolResult{}, err
	}
	if err := rejectSymlinkPath(invoker.Root, relative); err != nil {
		return ToolResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return ToolResult{}, err
	}
	if err := rejectSymlinkPath(invoker.Root, relative); err != nil {
		return ToolResult{}, err
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		existing, readErr := os.ReadFile(destination)
		if readErr == nil && bytes.Equal(existing, []byte(body)) {
			return ToolResult{Outcome: OutcomeSucceeded}, nil
		}
		return ToolResult{}, errors.New("Darwin metadata artifact already exists with different content")
	}
	if err != nil {
		return ToolResult{}, err
	}
	if _, err := file.WriteString(body); err != nil {
		_ = file.Close()
		return ToolResult{}, err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return ToolResult{}, err
	}
	if err := file.Close(); err != nil {
		return ToolResult{}, err
	}
	return ToolResult{Outcome: OutcomeSucceeded}, nil
}

func rejectSymlinkPath(root, relative string) error {
	rootInfo, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return errors.New("Darwin filesystem root must not be a symlink")
	}
	current := root
	for _, part := range strings.Split(pathpkg.Clean(relative), "/") {
		if part == "." || part == "" {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("Darwin filesystem resource cannot traverse a symlink")
		}
	}
	return nil
}

func callFor(packet HealthPacket, proposal Proposal) ToolCall {
	base := "bcgos://health/maestro-system/"
	suffix := string(proposal.Action) + "-" + proposal.ID + ".json"
	tool, operation := "filesystem", "write"
	if proposal.Action == ActionRunContractValidation {
		tool, operation = "validation", "run"
	}
	return ToolCall{Tool: tool, Operation: operation, Resource: base + "derived/" + suffix}
}

func proposalFor(observation Observation) Proposal {
	proposal := Proposal{Finding: observation.Code, Reversible: true, Rollback: ActionRefreshDerivedState, Effort: EffortSmall, Risk: RiskLow}
	switch observation.Code {
	case ObservationCapabilityUnavailable:
		proposal.Action, proposal.Impact = ActionRecordCapabilityGap, ImpactReliability
	case ObservationStateStale:
		proposal.Action, proposal.Impact = ActionRefreshDerivedState, ImpactRecovery
	case ObservationSchedulerMissed:
		proposal.Action, proposal.Impact = ActionReconcileScheduler, ImpactRecovery
	case ObservationContractDrift, ObservationValidationFailure:
		proposal.Action, proposal.Impact, proposal.Effort = ActionRunContractValidation, ImpactSafety, EffortMedium
	case ObservationOperatingFriction:
		proposal.Action, proposal.Impact = ActionRefreshDerivedState, ImpactFriction
	}
	if observation.Severity == SeverityHigh {
		proposal.Risk = RiskMedium
	}
	return proposal
}

func proposalID(windowID string, observation Observation) string {
	digest := sha256.Sum256([]byte(windowID + "\x00" + string(observation.Code) + "\x00" + string(observation.Severity)))
	return hex.EncodeToString(digest[:8])
}

func severityRank(value Severity) int {
	switch value {
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	default:
		return 1
	}
}

func summarize(actions []ActionReceipt) Outcome {
	if len(actions) == 0 {
		return OutcomeNoAction
	}
	succeeded, failed, blocked := 0, 0, 0
	for _, action := range actions {
		switch action.Outcome {
		case OutcomeSucceeded:
			succeeded++
		case OutcomeFailed:
			failed++
		default:
			blocked++
		}
	}
	if succeeded == len(actions) {
		return OutcomeSucceeded
	}
	if succeeded > 0 {
		return OutcomePartial
	}
	if failed > 0 {
		return OutcomeFailed
	}
	return OutcomeBlocked
}

func validMode(value Mode) bool {
	return value == Interactive || value == HeadlessHousekeeping || value == DeepReview
}

func min(left, right int) int {
	if left < right {
		return left
	}
	return right
}
