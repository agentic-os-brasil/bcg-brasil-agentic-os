package agentdispatch

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
)

// Pilot adds the smallest useful product flow around Dispatcher. It does not
// activate an agent runtime: a runtime adapter still has to execute the packet
// and submit its bounded return. The flow keeps Maestro accountable for the
// delegation record, target selection, terminal state and returned evidence.
type Pilot struct {
	dispatcher *Dispatcher
	instances  map[string]Instance
	mu         sync.Mutex
	records    map[string]Receipt
}

type Instance struct {
	AgentID       string
	Role          string
	ScopeKind     string
	ScopeID       string
	ParentAgentID string
	Available     bool
}

// InstanceFromScaffold is the runtime-neutral bridge from the user-local agent
// registration to the pilot selector. A scaffolded instance is a permitted
// target; native runtime activation remains a separate adapter concern.
func InstanceFromScaffold(status agentscaffold.Status) Instance {
	instance := status.Instance
	return Instance{
		AgentID:       instance.AgentID,
		Role:          instance.Role,
		ScopeKind:     instance.ScopeKind,
		ScopeID:       instance.ScopeID,
		ParentAgentID: instance.ParentAgentID,
		Available:     status.Initialized && instance.RegistrationState == "scaffolded" && status.Definition.Available && status.State.Available,
	}
}

type Intent struct {
	WorkspaceID string
	Objective   string
	Pointers    []string
	Constraints []string
	TTL         time.Duration
}

type ErrandIntent struct {
	ErrandID    string
	Objective   string
	Pointers    []string
	Constraints []string
	TTL         time.Duration
	Reversible  bool
}

type State string

const (
	StateDelegated   State = "delegated"
	StateCompleted   State = "completed"
	StateFailed      State = "failed"
	StateUnavailable State = "unavailable"
)

type Failure struct {
	Code   string `json:"code"`
	Reason string `json:"reason"`
}

type Return struct {
	Summary      string   `json:"summary"`
	EvidenceRefs []string `json:"evidence_refs"`
	Uncertainty  string   `json:"uncertainty,omitempty"`
}

type Receipt struct {
	SchemaVersion int        `json:"schema_version"`
	DelegationID  string     `json:"delegation_id"`
	OwnerAgentID  string     `json:"owner_agent_id"`
	TargetAgentID string     `json:"target_agent_id,omitempty"`
	ScopeKind     string     `json:"scope_kind"`
	ScopeID       string     `json:"scope_id"`
	State         State      `json:"state"`
	Packet        WorkPacket `json:"packet,omitempty"`
	Result        Return     `json:"result,omitempty"`
	Failure       Failure    `json:"failure,omitempty"`
}

func NewPilot(dispatcher *Dispatcher, instances []Instance) (*Pilot, error) {
	if dispatcher == nil {
		return nil, errors.New("pilot orchestration requires a dispatcher")
	}
	registered := make(map[string]Instance, len(instances))
	errands := 0
	for _, instance := range instances {
		if strings.TrimSpace(instance.AgentID) == "" || registered[instance.AgentID].AgentID != "" {
			return nil, errors.New("pilot agent registry is invalid")
		}
		if instance.Role == "errand_helper" {
			errands++
			if errands > 1 {
				return nil, errors.New("pilot permits one errand helper")
			}
		}
		registered[instance.AgentID] = instance
	}
	return &Pilot{dispatcher: dispatcher, instances: registered, records: map[string]Receipt{}}, nil
}

// Delegate selects the one workspace agent registered for the current pilot
// workspace. Selection is deliberately deterministic: no prompt-based router
// can silently widen the catalog or select another domain.
func (pilot *Pilot) Delegate(intent Intent) (Receipt, error) {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()

	targetID := "workspace-agent-" + strings.TrimSpace(intent.WorkspaceID)
	if !validWorkspaceIntent(intent) {
		return pilot.recordFailure("workspace", targetID, intent.WorkspaceID, "intent_invalid", "the workspace intent exceeds the bounded delegation contract")
	}
	instance, exists := pilot.instances[targetID]
	if !exists || !instance.Available || instance.Role != "workspace_agent" || instance.ScopeKind != "workspace" || instance.ScopeID != intent.WorkspaceID || instance.ParentAgentID != "maestro" {
		return pilot.recordFailure("workspace", targetID, intent.WorkspaceID, "target_unavailable", "the permitted workspace agent is not available for this scope")
	}
	packet, decision, err := pilot.dispatcher.StartRoot(PacketRequest{
		TargetAgentID: targetID, ScopeKind: "workspace", ScopeID: intent.WorkspaceID,
		Objective: intent.Objective, Pointers: intent.Pointers, Constraints: intent.Constraints, TTL: intent.TTL,
	})
	if err != nil {
		return pilot.recordFailure("workspace", targetID, intent.WorkspaceID, "dispatch_invalid", err.Error())
	}
	if !decision.Allowed {
		return pilot.recordFailure("workspace", targetID, intent.WorkspaceID, "dispatch_denied", "the shared orchestration guard denied delegation: "+decision.Code)
	}
	receipt := Receipt{
		SchemaVersion: 1, DelegationID: packet.PacketID, OwnerAgentID: "maestro", TargetAgentID: targetID,
		ScopeKind: "workspace", ScopeID: intent.WorkspaceID, State: StateDelegated, Packet: packet,
	}
	pilot.records[receipt.DelegationID] = receipt
	return receipt, nil
}

// DelegateErrand uses the single registered errand helper for a basic,
// reversible task. It intentionally has a separate input contract so a
// workspace or practice intent cannot be routed into this bounded lane.
func (pilot *Pilot) DelegateErrand(intent ErrandIntent) (Receipt, error) {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()

	if !validErrandIntent(intent) {
		return pilot.recordFailure("errand", "", intent.ErrandID, "intent_invalid", "the errand must be explicitly reversible and bounded")
	}
	var helper Instance
	for _, instance := range pilot.instances {
		if instance.Role == "errand_helper" {
			helper = instance
			break
		}
	}
	if helper.AgentID == "" || !helper.Available || helper.ScopeKind != "errand" || helper.ScopeID != intent.ErrandID || helper.ParentAgentID != "maestro" {
		return pilot.recordFailure("errand", helper.AgentID, intent.ErrandID, "target_unavailable", "the single permitted errand helper is not available for this scope")
	}
	packet, decision, err := pilot.dispatcher.StartRoot(PacketRequest{
		TargetAgentID: helper.AgentID, ScopeKind: "errand", ScopeID: intent.ErrandID,
		Objective: intent.Objective, Pointers: intent.Pointers, Constraints: intent.Constraints, TTL: intent.TTL,
	})
	if err != nil {
		return pilot.recordFailure("errand", helper.AgentID, intent.ErrandID, "dispatch_invalid", err.Error())
	}
	if !decision.Allowed {
		return pilot.recordFailure("errand", helper.AgentID, intent.ErrandID, "dispatch_denied", "the shared orchestration guard denied errand delegation: "+decision.Code)
	}
	receipt := Receipt{
		SchemaVersion: 1, DelegationID: packet.PacketID, OwnerAgentID: "maestro", TargetAgentID: helper.AgentID,
		ScopeKind: "errand", ScopeID: intent.ErrandID, State: StateDelegated, Packet: packet,
	}
	pilot.records[receipt.DelegationID] = receipt
	return receipt, nil
}

// Return closes a successful root branch and exposes only a bounded summary,
// evidence pointers and uncertainty to the Maestro flow.
func (pilot *Pilot) Return(packet WorkPacket, result Return) (Receipt, error) {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()
	receipt, err := pilot.activeReceipt(packet)
	if err != nil {
		return failureFromPacket(packet, "packet_invalid", err.Error()), err
	}
	if err := validateReturn(result, receipt.ScopeKind, receipt.ScopeID); err != nil {
		return failureFromPacket(packet, "return_invalid", err.Error()), err
	}
	if decision := pilot.dispatcher.FinishRoot(packet); !decision.Allowed {
		err := fmt.Errorf("shared orchestration guard denied completion: %s", decision.Code)
		return failureFromPacket(packet, "completion_denied", err.Error()), err
	}
	receipt.State, receipt.Result, receipt.Failure = StateCompleted, normalizeReturn(result), Failure{}
	pilot.records[receipt.DelegationID] = receipt
	return receipt, nil
}

// Fail records a runtime or execution failure explicitly and releases the
// branch. It cannot be used to mask an unverified packet.
func (pilot *Pilot) Fail(packet WorkPacket, failure Failure) (Receipt, error) {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()
	receipt, err := pilot.activeReceipt(packet)
	if err != nil {
		return failureFromPacket(packet, "packet_invalid", err.Error()), err
	}
	if strings.TrimSpace(failure.Code) == "" || strings.TrimSpace(failure.Reason) == "" || len(failure.Code) > 80 || len(failure.Reason) > maxObjectiveBytes {
		err := errors.New("failure report is incomplete or oversized")
		return failureFromPacket(packet, "failure_invalid", err.Error()), err
	}
	if decision := pilot.dispatcher.FinishRoot(packet); !decision.Allowed {
		err := fmt.Errorf("shared orchestration guard denied failure close: %s", decision.Code)
		return failureFromPacket(packet, "completion_denied", err.Error()), err
	}
	receipt.State, receipt.Failure = StateFailed, Failure{Code: strings.TrimSpace(failure.Code), Reason: strings.TrimSpace(failure.Reason)}
	pilot.records[receipt.DelegationID] = receipt
	return receipt, nil
}

func (pilot *Pilot) Inspect(delegationID string) (Receipt, bool) {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()
	receipt, ok := pilot.records[delegationID]
	return receipt, ok
}

func (pilot *Pilot) activeReceipt(packet WorkPacket) (Receipt, error) {
	if err := pilot.dispatcher.Verify(packet); err != nil {
		return Receipt{}, err
	}
	receipt, exists := pilot.records[packet.PacketID]
	if !exists || receipt.State != StateDelegated || receipt.Packet.Signature != packet.Signature || receipt.Packet.TargetAgentID != packet.TargetAgentID {
		return Receipt{}, errors.New("delegation is not active for this sealed packet")
	}
	return receipt, nil
}

func (pilot *Pilot) recordFailure(scopeKind, targetID, scopeID, code, reason string) (Receipt, error) {
	id, err := randomID()
	if err != nil {
		return Receipt{}, err
	}
	receipt := Receipt{
		SchemaVersion: 1, DelegationID: id, OwnerAgentID: "maestro", TargetAgentID: targetID,
		ScopeKind: scopeKind, ScopeID: scopeID, State: failureState(code), Failure: Failure{Code: code, Reason: reason},
	}
	pilot.records[id] = receipt
	return receipt, errors.New(reason)
}

func failureState(code string) State {
	if code == "target_unavailable" {
		return StateUnavailable
	}
	return StateFailed
}

func failureFromPacket(packet WorkPacket, code, reason string) Receipt {
	return Receipt{
		SchemaVersion: 1, DelegationID: packet.PacketID, OwnerAgentID: "maestro", TargetAgentID: packet.TargetAgentID,
		ScopeKind: packet.ScopeKind, ScopeID: packet.ScopeID, State: StateFailed, Failure: Failure{Code: code, Reason: reason},
	}
}

func validWorkspaceIntent(intent Intent) bool {
	return strings.TrimSpace(intent.WorkspaceID) != "" && !strings.ContainsAny(intent.WorkspaceID, "/\\") &&
		strings.TrimSpace(intent.Objective) != "" && len([]byte(strings.TrimSpace(intent.Objective))) <= maxObjectiveBytes &&
		intent.TTL > 0 && intent.TTL <= maxPacketTTL && len(intent.Pointers) <= maxPointers && len(intent.Constraints) <= maxConstraints
}

func validErrandIntent(intent ErrandIntent) bool {
	return agentcatalog.ValidAgentID(strings.TrimSpace(intent.ErrandID)) && intent.Reversible &&
		strings.TrimSpace(intent.Objective) != "" && len([]byte(strings.TrimSpace(intent.Objective))) <= maxObjectiveBytes &&
		intent.TTL > 0 && intent.TTL <= maxPacketTTL && len(intent.Pointers) <= maxPointers && len(intent.Constraints) <= maxConstraints
}

func validateReturn(result Return, scopeKind, scopeID string) error {
	if strings.TrimSpace(result.Summary) == "" || len([]byte(result.Summary)) > maxObjectiveBytes || len([]byte(result.Uncertainty)) > maxConstraintBytes || len(result.EvidenceRefs) > maxPointers {
		return errors.New("delegated return exceeds its bounded contract")
	}
	seen := map[string]bool{}
	for _, ref := range result.EvidenceRefs {
		normalized, valid := agentorchestration.NormalizeResource(ref)
		if !valid || !agentorchestration.ResourceWithinScope(normalized, scopeKind, scopeID) || seen[normalized] {
			return errors.New("delegated return contains an invalid, duplicate or cross-scope evidence pointer")
		}
		seen[normalized] = true
	}
	return nil
}

func normalizeReturn(result Return) Return {
	result.Summary, result.Uncertainty = strings.TrimSpace(result.Summary), strings.TrimSpace(result.Uncertainty)
	refs := append([]string(nil), result.EvidenceRefs...)
	sort.Strings(refs)
	result.EvidenceRefs = refs
	return result
}
