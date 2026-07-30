package agentdispatch

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentscaffold"
)

const (
	maxExecutionEnvelopeAge = 5 * time.Minute
	maxFailureCodeBytes     = 80
	errandTool              = "errand_store"
)

// Pilot is an intentionally process-local bridge around Dispatcher. Dispatch
// packets and result bodies are ephemeral adapter messages. Only metadata-only
// receipts are inspectable, and a new process cannot complete an old dispatch
// until a separate safe recovery protocol is implemented.
type Pilot struct {
	dispatcher *Dispatcher
	runtime    string
	instances  map[string]Instance
	now        func() time.Time

	mu         sync.Mutex
	records    map[string]pilotRecord
	usedNonces map[string]bool
}

type pilotRecord struct {
	receipt     Receipt
	packet      WorkPacket
	errand      *ErrandContract
	errandState errandState
	pendingTool *ErrandToolEnvelope
}

type Instance struct {
	AgentID       string
	Role          string
	ScopeKind     string
	ScopeID       string
	ParentAgentID string
	Available     bool
}

// InstanceFromScaffold projects user-local registration metadata into the
// deterministic pilot selector. It does not imply native runtime activation.
func InstanceFromScaffold(status agentscaffold.Status) Instance {
	instance := status.Instance
	return Instance{
		AgentID:       instance.AgentID,
		Role:          instance.Role,
		ScopeKind:     instance.ScopeKind,
		ScopeID:       instance.ScopeID,
		ParentAgentID: instance.ParentAgentID,
		Available: status.Initialized && instance.RegistrationState == "scaffolded" &&
			status.Definition.Available && status.State.Available,
	}
}

type Intent struct {
	WorkspaceID   string
	Objective     string
	Pointers      []string
	Constraints   []string
	ReviewTrigger WalterReviewTrigger
	TTL           time.Duration
}

type ErrandOperation string

const (
	// ErrandCreateEphemeralNote is the only pilot errand mutation. Its exact
	// inverse is always ErrandDeleteEphemeralNote on the same resource.
	ErrandCreateEphemeralNote ErrandOperation = "create_ephemeral_note"
	ErrandDeleteEphemeralNote ErrandOperation = "delete_ephemeral_note"
)

type ErrandGrant struct {
	Operation ErrandOperation `json:"operation"`
	Resource  string          `json:"resource"`
}

type ErrandIntent struct {
	ErrandID  string
	Objective string
	Grant     ErrandGrant
	TTL       time.Duration
}

type ErrandContract struct {
	Tool         string      `json:"tool"`
	Grant        ErrandGrant `json:"grant"`
	Compensation ErrandGrant `json:"compensation"`
}

type ErrandToolEvent string

const (
	ErrandToolRequest   ErrandToolEvent = "request"
	ErrandToolSucceeded ErrandToolEvent = "succeeded"
	ErrandToolFailed    ErrandToolEvent = "failed"
)

type ErrandToolEnvelope struct {
	SchemaVersion int             `json:"schema_version"`
	PacketID      string          `json:"packet_id"`
	TargetAgentID string          `json:"target_agent_id"`
	Runtime       string          `json:"runtime"`
	Tool          string          `json:"tool"`
	Operation     ErrandOperation `json:"operation"`
	Resource      string          `json:"resource"`
	Event         ErrandToolEvent `json:"event"`
	RequestNonce  string          `json:"request_nonce,omitempty"`
	Nonce         string          `json:"nonce"`
	IssuedAt      time.Time       `json:"issued_at"`
	Signature     string          `json:"signature"`
}

type errandState string

const (
	errandReady        errandState = ""
	errandGrantPending errandState = "grant_pending"
	errandGranted      errandState = "granted"
	errandUndoPending  errandState = "undo_pending"
	errandCompensated  errandState = "compensated"
)

// Dispatch is the explicit ephemeral adapter message. It may contain work
// bodies and therefore must not be exposed as a receipt or persisted in logs.
type Dispatch struct {
	Runtime string          `json:"runtime"`
	Packet  WorkPacket      `json:"packet"`
	Errand  *ErrandContract `json:"errand,omitempty"`
}

type State string

const (
	StateDelegated State = "delegated"
	// StatePendingReview means the producer returned, but material completion
	// remains blocked until Walter issues an approved verdict.
	StatePendingReview State = "pending_review"
	StateCompleted     State = "completed"
	StateFailed        State = "failed"
	StateUnavailable   State = "unavailable"
)

// Receipt is safe for public status surfaces: it contains identity, timing,
// state and digests only. Work packet, result and failure prose never enter it.
type Receipt struct {
	SchemaVersion int            `json:"schema_version"`
	DelegationID  string         `json:"delegation_id"`
	OwnerAgentID  string         `json:"owner_agent_id"`
	TargetAgentID string         `json:"target_agent_id,omitempty"`
	Runtime       string         `json:"runtime,omitempty"`
	ScopeKind     string         `json:"scope_kind,omitempty"`
	ScopeID       string         `json:"scope_id,omitempty"`
	State         State          `json:"state"`
	PacketSHA256  string         `json:"packet_sha256,omitempty"`
	ResultSHA256  string         `json:"result_sha256,omitempty"`
	FailureCode   string         `json:"failure_code,omitempty"`
	Review        *ReviewSummary `json:"review,omitempty"`
	IssuedAt      time.Time      `json:"issued_at,omitempty"`
	CompletedAt   time.Time      `json:"completed_at,omitempty"`
}

type ReturnBody struct {
	Summary      string   `json:"summary"`
	EvidenceRefs []string `json:"evidence_refs,omitempty"`
	Uncertainty  string   `json:"uncertainty,omitempty"`
}

type FailureBody struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

type ExecutionOutcome string

const (
	ExecutionSucceeded ExecutionOutcome = "succeeded"
	ExecutionFailed    ExecutionOutcome = "failed"
)

// ExecutionEnvelope is authenticated by the target executor capability. Its
// signature binds runtime, packet, target, scope, outcome, result digest,
// random nonce and issue time.
type ExecutionEnvelope struct {
	SchemaVersion int              `json:"schema_version"`
	PacketID      string           `json:"packet_id"`
	TargetAgentID string           `json:"target_agent_id"`
	Runtime       string           `json:"runtime"`
	ScopeKind     string           `json:"scope_kind"`
	ScopeID       string           `json:"scope_id"`
	Outcome       ExecutionOutcome `json:"outcome"`
	ResultSHA256  string           `json:"result_sha256"`
	Nonce         string           `json:"nonce"`
	IssuedAt      time.Time        `json:"issued_at"`
	Signature     string           `json:"signature"`
}

// Executor is the narrow adapter-side signer for one authenticated target and
// runtime. Native wiring must obtain its capability from the approved private
// store; it must never place the capability in a Dispatch or Receipt.
type Executor struct {
	runtime    string
	targetID   string
	capability string
	now        func() time.Time
}

func NewExecutor(runtime, targetID, capability string) (*Executor, error) {
	if (runtime != "claude" && runtime != "codex") || !agentcatalog.ValidAgentID(targetID) || capability == "" {
		return nil, errors.New("executor authorization is incomplete")
	}
	return &Executor{runtime: runtime, targetID: targetID, capability: capability, now: time.Now}, nil
}

func (executor *Executor) SealReturn(dispatch Dispatch, body ReturnBody) (ExecutionEnvelope, error) {
	if dispatch.Runtime != executor.runtime || dispatch.Packet.TargetAgentID != executor.targetID {
		return ExecutionEnvelope{}, errors.New("executor is not authorized for this dispatch")
	}
	if dispatch.Packet.Review != nil {
		return ExecutionEnvelope{}, errors.New("Walter dispatch requires a typed review verdict")
	}
	if err := validateReturnBody(body, dispatch.Packet.ScopeKind, dispatch.Packet.ScopeID); err != nil {
		return ExecutionEnvelope{}, err
	}
	return executor.seal(dispatch, ExecutionSucceeded, digestBody(normalizeReturnBody(body)))
}

// SealWalterReview is the only success envelope Walter may produce. Keeping
// it separate from SealReturn prevents a generic summary from bypassing the
// bounded conversational verdict contract.
func (executor *Executor) SealWalterReview(dispatch Dispatch, body WalterReviewBody) (ExecutionEnvelope, error) {
	if dispatch.Runtime != executor.runtime || dispatch.Packet.TargetAgentID != executor.targetID ||
		executor.targetID != "walter" || dispatch.Packet.Review == nil {
		return ExecutionEnvelope{}, errors.New("executor is not authorized for this Walter review")
	}
	normalized := normalizeWalterReviewBody(body)
	if err := validateWalterReviewBody(normalized, *dispatch.Packet.Review); err != nil {
		return ExecutionEnvelope{}, err
	}
	return executor.seal(dispatch, ExecutionSucceeded, digestBody(normalized))
}

func (executor *Executor) SealFailure(dispatch Dispatch, body FailureBody) (ExecutionEnvelope, error) {
	if dispatch.Runtime != executor.runtime || dispatch.Packet.TargetAgentID != executor.targetID {
		return ExecutionEnvelope{}, errors.New("executor is not authorized for this dispatch")
	}
	if err := validateFailureBody(body); err != nil {
		return ExecutionEnvelope{}, err
	}
	return executor.seal(dispatch, ExecutionFailed, digestBody(normalizeFailureBody(body)))
}

func (executor *Executor) SealErrandToolRequest(dispatch Dispatch, operation ErrandOperation, resource string) (ErrandToolEnvelope, error) {
	if dispatch.Runtime != executor.runtime ||
		dispatch.Packet.TargetAgentID != executor.targetID ||
		dispatch.Errand == nil {
		return ErrandToolEnvelope{}, errors.New("executor is not authorized for this errand")
	}
	grant := dispatch.Errand.Grant
	compensation := dispatch.Errand.Compensation
	if !((operation == grant.Operation && resource == grant.Resource) ||
		(operation == compensation.Operation && resource == compensation.Resource)) {
		return ErrandToolEnvelope{}, errors.New("tool request is outside the exact errand contract")
	}
	nonce, err := randomID()
	if err != nil {
		return ErrandToolEnvelope{}, err
	}
	envelope := ErrandToolEnvelope{
		SchemaVersion: 1, PacketID: dispatch.Packet.PacketID,
		TargetAgentID: executor.targetID, Runtime: executor.runtime,
		Tool: dispatch.Errand.Tool, Operation: operation, Resource: resource,
		Event: ErrandToolRequest, Nonce: nonce, IssuedAt: executor.now().UTC(),
	}
	envelope.Signature, err = signErrandToolEnvelope(envelope, executor.capability)
	return envelope, err
}

func (executor *Executor) SealErrandToolOutcome(request ErrandToolEnvelope, event ErrandToolEvent) (ErrandToolEnvelope, error) {
	if request.TargetAgentID != executor.targetID ||
		request.Runtime != executor.runtime ||
		request.Event != ErrandToolRequest ||
		(event != ErrandToolSucceeded && event != ErrandToolFailed) {
		return ErrandToolEnvelope{}, errors.New("executor cannot attest this errand tool outcome")
	}
	expected, err := signErrandToolEnvelope(request, executor.capability)
	if err != nil || !hmac.Equal([]byte(expected), []byte(request.Signature)) {
		return ErrandToolEnvelope{}, errors.New("errand tool request was not authenticated by this executor")
	}
	nonce, err := randomID()
	if err != nil {
		return ErrandToolEnvelope{}, err
	}
	envelope := request
	envelope.Event = event
	envelope.RequestNonce = request.Nonce
	envelope.Nonce = nonce
	envelope.IssuedAt = executor.now().UTC()
	envelope.Signature = ""
	envelope.Signature, err = signErrandToolEnvelope(envelope, executor.capability)
	return envelope, err
}

func (executor *Executor) seal(dispatch Dispatch, outcome ExecutionOutcome, resultSHA256 string) (ExecutionEnvelope, error) {
	nonce, err := randomID()
	if err != nil {
		return ExecutionEnvelope{}, err
	}
	envelope := ExecutionEnvelope{
		SchemaVersion: 1,
		PacketID:      dispatch.Packet.PacketID, TargetAgentID: executor.targetID,
		Runtime: executor.runtime, ScopeKind: dispatch.Packet.ScopeKind, ScopeID: dispatch.Packet.ScopeID,
		Outcome: outcome, ResultSHA256: resultSHA256, Nonce: nonce, IssuedAt: executor.now().UTC(),
	}
	envelope.Signature, err = signExecutionEnvelope(envelope, executor.capability)
	return envelope, err
}

func NewPilot(dispatcher *Dispatcher, instances []Instance) (*Pilot, error) {
	if dispatcher == nil {
		return nil, errors.New("pilot orchestration requires a dispatcher")
	}
	registered := make(map[string]Instance, len(instances))
	errands := 0
	for _, instance := range instances {
		if !agentcatalog.ValidAgentID(instance.AgentID) || registered[instance.AgentID].AgentID != "" {
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
	// Walter is a managed core leaf, not a user-scaffolded workspace instance.
	// Material review remains unavailable unless the dispatcher also carries his
	// authenticated capability; the synthetic registration only preserves the
	// canonical fixed review scope and does not grant authority.
	if _, exists := registered["walter"]; !exists && dispatcher.credentials["walter"] != "" {
		registered["walter"] = Instance{
			AgentID: "walter", Role: "reviewer", ScopeKind: "review", ScopeID: "review",
			ParentAgentID: "maestro", Available: true,
		}
	}
	return &Pilot{
		dispatcher: dispatcher, runtime: dispatcher.gate.Runtime(), instances: registered,
		now: time.Now, records: make(map[string]pilotRecord), usedNonces: make(map[string]bool),
	}, nil
}

// RequireWalterReview opens exactly one direct Walter branch for a producer
// that is already pending review. The review packet binds the source packet
// digest and keeps the projection to IDs, digests, trigger and verdict metadata.
func (pilot *Pilot) RequireWalterReview(sourceDelegationID string, request WalterReviewRequest) (Dispatch, Receipt, error) {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()

	source, exists := pilot.records[sourceDelegationID]
	if !exists || source.receipt.State != StatePendingReview || source.packet.TargetAgentID == "walter" ||
		source.packet.ReviewTrigger == "" || source.packet.ReviewTrigger != request.Trigger {
		receipt, err := pilot.recordReviewFailure(source, request, "source_not_complete", StateFailed)
		return Dispatch{}, receipt, err
	}
	if err := pilot.dispatcher.Verify(source.packet); err != nil {
		receipt, failure := pilot.recordReviewFailure(source, request, "source_packet_invalid", StateFailed)
		return Dispatch{}, receipt, failure
	}
	if err := validateReviewRequest(request); err != nil {
		receipt, failure := pilot.recordReviewFailure(source, request, "review_invalid", StateFailed)
		return Dispatch{}, receipt, failure
	}
	review := ReviewPacket{
		SourcePacketID: source.packet.PacketID, SourcePacketSHA256: digestBody(source.packet),
		SourceScopeKind: source.packet.ScopeKind, SourceScopeID: source.packet.ScopeID,
		Trigger: request.Trigger, Audience: strings.TrimSpace(request.Audience),
		Recommendation:   strings.TrimSpace(request.Recommendation),
		DefinitionOfDone: strings.TrimSpace(request.DefinitionOfDone),
		ArtifactRefs:     append([]string(nil), request.ArtifactRefs...),
		EvidenceRefs:     append([]string(nil), request.EvidenceRefs...),
		Uncertainties:    append([]string(nil), request.Uncertainties...),
	}
	if err := validateReviewPacket(&review, "", request.ReviewObjective); err != nil {
		receipt, failure := pilot.recordReviewFailure(source, request, "review_invalid", StateFailed)
		return Dispatch{}, receipt, failure
	}
	instance, exists := pilot.instances["walter"]
	if !exists || !instance.Available || instance.Role != "reviewer" || instance.ScopeKind != "review" ||
		instance.ScopeID != "review" || instance.ParentAgentID != "maestro" {
		receipt, failure := pilot.recordReviewFailure(source, request, "target_unavailable", StateUnavailable)
		return Dispatch{}, receipt, failure
	}
	dispatch, receipt, err := pilot.start(instance, PacketRequest{
		TargetAgentID: "walter", ScopeKind: "review", ScopeID: "review",
		Objective: request.ReviewObjective, Review: &review, TTL: request.TTL,
	}, nil)
	if err == nil {
		producer := pilot.records[source.receipt.DelegationID]
		producer.receipt.Review = reviewSummary(&review, ReviewDispatched)
		pilot.records[source.receipt.DelegationID] = producer
	}
	return dispatch, receipt, err
}

func (pilot *Pilot) Delegate(intent Intent) (Dispatch, Receipt, error) {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()

	targetID := "workspace-agent-" + strings.TrimSpace(intent.WorkspaceID)
	if !validWorkspaceIntent(intent) {
		receipt, err := pilot.recordFailure("workspace", targetID, intent.WorkspaceID, "intent_invalid", StateFailed)
		return Dispatch{}, receipt, err
	}
	instance, exists := pilot.instances[targetID]
	if !exists || !instance.Available || (instance.Role != "case_agent" && instance.Role != "workspace_agent") ||
		instance.ScopeKind != "workspace" || instance.ScopeID != intent.WorkspaceID ||
		instance.ParentAgentID != "maestro" {
		receipt, err := pilot.recordFailure("workspace", targetID, intent.WorkspaceID, "target_unavailable", StateUnavailable)
		return Dispatch{}, receipt, err
	}
	return pilot.start(instance, PacketRequest{
		TargetAgentID: targetID, ScopeKind: "workspace", ScopeID: intent.WorkspaceID,
		Objective: intent.Objective, Pointers: intent.Pointers, Constraints: intent.Constraints,
		ReviewTrigger: intent.ReviewTrigger, TTL: intent.TTL,
	}, nil)
}

// Rework starts a new signed producer attempt only after Walter returned a
// bounded refinement or missing-the-mark verdict. The new packet carries the
// prior packet ID so the retry cannot be detached from the reviewed material.
func (pilot *Pilot) Rework(sourceDelegationID string, intent Intent) (Dispatch, Receipt, error) {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()

	source, exists := pilot.records[sourceDelegationID]
	if !exists || source.receipt.State != StatePendingReview || source.packet.ReviewTrigger == "" ||
		source.receipt.Review == nil ||
		(source.receipt.Review.State != ReviewRefineReturn && source.receipt.Review.State != ReviewMissingMark) {
		receipt, err := pilot.recordFailure("workspace", "", intent.WorkspaceID, "rework_not_authorized", StateFailed)
		return Dispatch{}, receipt, err
	}
	if !validWorkspaceIntent(intent) || intent.WorkspaceID != source.packet.ScopeID ||
		intent.ReviewTrigger != source.packet.ReviewTrigger {
		receipt, err := pilot.recordFailure("workspace", source.packet.TargetAgentID, source.packet.ScopeID, "rework_invalid", StateFailed)
		return Dispatch{}, receipt, err
	}
	instance, exists := pilot.instances[source.packet.TargetAgentID]
	if !exists || !instance.Available || instance.ParentAgentID != "maestro" ||
		instance.ScopeKind != source.packet.ScopeKind || instance.ScopeID != source.packet.ScopeID {
		receipt, err := pilot.recordFailure("workspace", source.packet.TargetAgentID, source.packet.ScopeID, "target_unavailable", StateUnavailable)
		return Dispatch{}, receipt, err
	}
	return pilot.start(instance, PacketRequest{
		TargetAgentID: source.packet.TargetAgentID, ScopeKind: source.packet.ScopeKind,
		ScopeID: source.packet.ScopeID, Objective: intent.Objective, Pointers: intent.Pointers,
		Constraints: intent.Constraints, ReviewTrigger: intent.ReviewTrigger,
		ReworkOfPacketID: source.packet.PacketID, TTL: intent.TTL,
	}, nil)
}

func (pilot *Pilot) DelegateErrand(intent ErrandIntent) (Dispatch, Receipt, error) {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()

	contract, err := errandContract(intent)
	if err != nil {
		receipt, failure := pilot.recordFailure("errand", "", intent.ErrandID, "intent_invalid", StateFailed)
		return Dispatch{}, receipt, failure
	}
	var helper Instance
	for _, instance := range pilot.instances {
		if instance.Role == "errand_helper" {
			helper = instance
			break
		}
	}
	if helper.AgentID == "" || !helper.Available || helper.ScopeKind != "errand" ||
		helper.ScopeID != intent.ErrandID || helper.ParentAgentID != "maestro" {
		receipt, failure := pilot.recordFailure("errand", helper.AgentID, intent.ErrandID, "target_unavailable", StateUnavailable)
		return Dispatch{}, receipt, failure
	}
	return pilot.start(helper, PacketRequest{
		TargetAgentID: helper.AgentID, ScopeKind: "errand", ScopeID: intent.ErrandID,
		Objective: intent.Objective, Pointers: []string{contract.Grant.Resource},
		Constraints: []string{
			"Execute only the exact typed grant in the errand contract.",
			"On rollback, execute only the deterministic compensation on the same resource.",
		},
		TTL: intent.TTL,
	}, &contract)
}

func (pilot *Pilot) start(instance Instance, request PacketRequest, errand *ErrandContract) (Dispatch, Receipt, error) {
	packet, decision, err := pilot.dispatcher.StartRoot(request)
	if err != nil {
		receipt, failure := pilot.recordFailure(instance.ScopeKind, instance.AgentID, instance.ScopeID, "dispatch_invalid", StateFailed)
		return Dispatch{}, receipt, fmt.Errorf("%w: %v", failure, err)
	}
	if !decision.Allowed {
		receipt, failure := pilot.recordFailure(instance.ScopeKind, instance.AgentID, instance.ScopeID, "dispatch_denied", StateFailed)
		return Dispatch{}, receipt, fmt.Errorf("%w: %s", failure, decision.Code)
	}
	packetSHA256 := digestBody(packet)
	receipt := Receipt{
		SchemaVersion: 1, DelegationID: packet.PacketID, OwnerAgentID: "maestro",
		TargetAgentID: instance.AgentID, Runtime: pilot.runtime,
		ScopeKind: instance.ScopeKind, ScopeID: instance.ScopeID,
		State: StateDelegated, PacketSHA256: packetSHA256, IssuedAt: packet.IssuedAt,
	}
	receipt.Review = reviewSummary(request.Review, ReviewDispatched)
	var privateErrand *ErrandContract
	if errand != nil {
		copy := *errand
		privateErrand = &copy
	}
	pilot.records[receipt.DelegationID] = pilotRecord{
		receipt: receipt, packet: packet, errand: privateErrand,
	}
	return Dispatch{Runtime: pilot.runtime, Packet: packet, Errand: errand}, receipt, nil
}

// GuardErrandTool verifies a fresh target-authenticated native tool request
// before narrowing the static catalog boundary to the exact active grant.
func (pilot *Pilot) GuardErrandTool(envelope ErrandToolEnvelope) agentorchestration.Decision {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()
	record, exists := pilot.records[envelope.PacketID]
	if !exists || record.receipt.State != StateDelegated || record.errand == nil {
		return agentorchestration.Decision{Allowed: false, Code: "dispatch_denied"}
	}
	if err := pilot.verifyErrandToolEnvelope(record, envelope, ErrandToolRequest); err != nil {
		return agentorchestration.Decision{Allowed: false, Code: "tool_envelope_denied"}
	}
	grant := record.errand.Grant
	compensation := record.errand.Compensation
	nextState := errandGrantPending
	if envelope.Operation == compensation.Operation &&
		envelope.Resource == compensation.Resource &&
		record.errandState == errandGranted {
		nextState = errandUndoPending
	} else if envelope.Operation != grant.Operation ||
		envelope.Resource != grant.Resource ||
		record.errandState != errandReady {
		return agentorchestration.Decision{Allowed: false, Code: "resource_denied"}
	}
	decision := pilot.dispatcher.guardRootTool(
		record.packet, record.errand.Tool, string(envelope.Operation), envelope.Resource,
	)
	if !decision.Allowed {
		return decision
	}
	copy := envelope
	record.pendingTool = &copy
	record.errandState = nextState
	pilot.usedNonces[envelope.Nonce] = true
	pilot.records[envelope.PacketID] = record
	return decision
}

// ObserveErrandTool records only a target-authenticated terminal outcome. A
// compensation cannot be authorized until the exact grant succeeded, and it
// cannot be repeated after one successful undo.
func (pilot *Pilot) ObserveErrandTool(envelope ErrandToolEnvelope) agentorchestration.Decision {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()
	record, exists := pilot.records[envelope.PacketID]
	if !exists || record.receipt.State != StateDelegated ||
		record.errand == nil || record.pendingTool == nil {
		return agentorchestration.Decision{Allowed: false, Code: "dispatch_denied"}
	}
	if envelope.Event != ErrandToolSucceeded && envelope.Event != ErrandToolFailed {
		return agentorchestration.Decision{Allowed: false, Code: "tool_envelope_denied"}
	}
	if err := pilot.verifyErrandToolEnvelope(record, envelope, envelope.Event); err != nil ||
		envelope.RequestNonce != record.pendingTool.Nonce ||
		envelope.Tool != record.pendingTool.Tool ||
		envelope.Operation != record.pendingTool.Operation ||
		envelope.Resource != record.pendingTool.Resource {
		return agentorchestration.Decision{Allowed: false, Code: "tool_envelope_denied"}
	}
	if envelope.Event == ErrandToolSucceeded {
		if record.errandState == errandGrantPending {
			record.errandState = errandGranted
		} else if record.errandState == errandUndoPending {
			record.errandState = errandCompensated
		} else {
			return agentorchestration.Decision{Allowed: false, Code: "tool_sequence_denied"}
		}
	} else {
		if record.errandState == errandGrantPending {
			record.errandState = errandReady
		} else if record.errandState == errandUndoPending {
			record.errandState = errandGranted
		} else {
			return agentorchestration.Decision{Allowed: false, Code: "tool_sequence_denied"}
		}
	}
	record.pendingTool = nil
	pilot.usedNonces[envelope.Nonce] = true
	pilot.records[envelope.PacketID] = record
	return agentorchestration.Decision{Allowed: true, Code: "allowed"}
}

func (pilot *Pilot) Return(envelope ExecutionEnvelope, body ReturnBody) (Receipt, error) {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()
	if record, ok := pilot.records[envelope.PacketID]; ok && record.packet.Review != nil {
		return pilot.rejectEnvelope(envelope, "review_verdict_required", errors.New("Walter dispatch requires a typed review verdict"))
	}
	normalized := normalizeReturnBody(body)
	if err := validateReturnBody(normalized, envelope.ScopeKind, envelope.ScopeID); err != nil {
		return pilot.rejectEnvelope(envelope, "return_invalid", err)
	}
	record, err := pilot.verifyEnvelope(envelope, ExecutionSucceeded, digestBody(normalized))
	if err != nil {
		return pilot.rejectEnvelope(envelope, envelopeFailureCode(err), err)
	}
	if record.errand != nil && record.errandState != errandGranted {
		return pilot.rejectEnvelope(envelope, "errand_incomplete", errors.New("errand grant has not completed successfully"))
	}
	if decision := pilot.dispatcher.FinishRoot(record.packet); !decision.Allowed {
		return pilot.rejectEnvelope(envelope, "completion_denied", fmt.Errorf("orchestration guard denied completion: %s", decision.Code))
	}
	state := StateCompleted
	if record.packet.ReviewTrigger != "" {
		state = StatePendingReview
	}
	return pilot.complete(record, envelope, "", state), nil
}

// ReturnWalterReview authenticates and closes the Walter leaf with a bounded
// conversational verdict. A refine or missing verdict closes only the review
// branch; it never becomes execution-ledger approval or final business
// completion authority.
func (pilot *Pilot) ReturnWalterReview(envelope ExecutionEnvelope, body WalterReviewBody) (Receipt, error) {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()
	normalized := normalizeWalterReviewBody(body)
	record, exists := pilot.records[envelope.PacketID]
	if !exists || record.packet.Review == nil {
		return pilot.rejectEnvelope(envelope, "review_not_found", errors.New("Walter review packet is not active"))
	}
	if err := validateWalterReviewBody(normalized, *record.packet.Review); err != nil {
		return pilot.rejectEnvelope(envelope, "review_invalid", err)
	}
	verified, err := pilot.verifyEnvelope(envelope, ExecutionSucceeded, digestBody(normalized))
	if err != nil {
		return pilot.rejectEnvelope(envelope, envelopeFailureCode(err), err)
	}
	if decision := pilot.dispatcher.FinishRoot(verified.packet); !decision.Allowed {
		return pilot.rejectEnvelope(envelope, "completion_denied", fmt.Errorf("Walter review close denied: %s", decision.Code))
	}
	state := ReviewState(ReviewMissingMark)
	switch normalized.Verdict {
	case WalterApproved:
		state = ReviewApproved
	case WalterRefineAndReturn:
		state = ReviewRefineReturn
	}
	receipt := pilot.complete(verified, envelope, "", StateCompleted, state)
	if receipt.Review != nil {
		receipt.Review.ObjectionCount = len(normalized.Objections)
	}
	updated := pilot.records[receipt.DelegationID]
	updated.receipt = receipt
	pilot.records[receipt.DelegationID] = updated
	if sourceID := verified.packet.Review.SourcePacketID; sourceID != "" {
		for delegationID, producer := range pilot.records {
			if producer.packet.PacketID != sourceID {
				continue
			}
			producer.receipt.Review = reviewSummary(verified.packet.Review, state)
			if normalized.Verdict == WalterApproved {
				producer.receipt.State = StateCompleted
				producer.receipt.CompletedAt = pilot.now().UTC()
			}
			pilot.records[delegationID] = producer
			break
		}
	}
	return receipt, nil
}

func (pilot *Pilot) Fail(envelope ExecutionEnvelope, body FailureBody) (Receipt, error) {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()
	normalized := normalizeFailureBody(body)
	if err := validateFailureBody(normalized); err != nil {
		return pilot.rejectEnvelope(envelope, "failure_invalid", err)
	}
	record, err := pilot.verifyEnvelope(envelope, ExecutionFailed, digestBody(normalized))
	if err != nil {
		return pilot.rejectEnvelope(envelope, envelopeFailureCode(err), err)
	}
	if record.errand != nil &&
		record.errandState != errandReady &&
		record.errandState != errandCompensated {
		return pilot.rejectEnvelope(envelope, "compensation_required", errors.New("errand mutation requires successful compensation before failure close"))
	}
	if decision := pilot.dispatcher.FinishRoot(record.packet); !decision.Allowed {
		return pilot.rejectEnvelope(envelope, "completion_denied", fmt.Errorf("orchestration guard denied failure close: %s", decision.Code))
	}
	if record.packet.Review != nil {
		return pilot.complete(record, envelope, normalized.Code, StateFailed, ReviewUnavailable), nil
	}
	return pilot.complete(record, envelope, normalized.Code, StateFailed), nil
}

func (pilot *Pilot) Inspect(delegationID string) (Receipt, bool) {
	pilot.mu.Lock()
	defer pilot.mu.Unlock()
	record, ok := pilot.records[delegationID]
	return record.receipt, ok
}

func (pilot *Pilot) verifyEnvelope(envelope ExecutionEnvelope, outcome ExecutionOutcome, resultSHA256 string) (pilotRecord, error) {
	if pilot.usedNonces[envelope.Nonce] {
		return pilotRecord{}, errEnvelopeReplayed
	}
	record, exists := pilot.records[envelope.PacketID]
	if !exists {
		return pilotRecord{}, errDelegationUnavailable
	}
	if record.receipt.State != StateDelegated {
		return pilotRecord{}, errEnvelopeReplayed
	}
	if envelope.SchemaVersion != 1 || !validPacketID(envelope.Nonce) ||
		envelope.TargetAgentID != record.receipt.TargetAgentID ||
		envelope.Runtime != record.receipt.Runtime || envelope.Runtime != pilot.runtime ||
		envelope.ScopeKind != record.receipt.ScopeKind || envelope.ScopeID != record.receipt.ScopeID ||
		envelope.Outcome != outcome || envelope.ResultSHA256 != resultSHA256 ||
		!validSHA256(envelope.ResultSHA256) || envelope.Signature == "" {
		return pilotRecord{}, errEnvelopeDenied
	}
	now := pilot.now().UTC()
	if envelope.IssuedAt.Before(record.packet.IssuedAt) || envelope.IssuedAt.After(now) ||
		now.Sub(envelope.IssuedAt) > maxExecutionEnvelopeAge ||
		!envelope.IssuedAt.Before(record.packet.ExpiresAt) {
		return pilotRecord{}, errEnvelopeDenied
	}
	capability := pilot.dispatcher.credentials[record.receipt.TargetAgentID]
	if capability == "" {
		return pilotRecord{}, errEnvelopeDenied
	}
	expected, err := signExecutionEnvelope(envelope, capability)
	if err != nil {
		return pilotRecord{}, errEnvelopeDenied
	}
	provided, err := hex.DecodeString(envelope.Signature)
	expectedBytes, expectedErr := hex.DecodeString(expected)
	if err != nil || expectedErr != nil || subtle.ConstantTimeCompare(provided, expectedBytes) != 1 {
		return pilotRecord{}, errEnvelopeDenied
	}
	if digestBody(record.packet) != record.receipt.PacketSHA256 {
		return pilotRecord{}, errEnvelopeDenied
	}
	return record, nil
}

func (pilot *Pilot) verifyErrandToolEnvelope(record pilotRecord, envelope ErrandToolEnvelope, event ErrandToolEvent) error {
	if pilot.usedNonces[envelope.Nonce] ||
		envelope.SchemaVersion != 1 ||
		envelope.Event != event ||
		envelope.PacketID != record.packet.PacketID ||
		envelope.TargetAgentID != record.receipt.TargetAgentID ||
		envelope.Runtime != record.receipt.Runtime ||
		envelope.Runtime != pilot.runtime ||
		envelope.Tool != record.errand.Tool ||
		!validPacketID(envelope.Nonce) ||
		envelope.Signature == "" {
		return errEnvelopeDenied
	}
	normalized, valid := agentorchestration.NormalizeResource(envelope.Resource)
	if !valid || normalized != envelope.Resource {
		return errEnvelopeDenied
	}
	now := pilot.now().UTC()
	if envelope.IssuedAt.Before(record.packet.IssuedAt) ||
		envelope.IssuedAt.After(now) ||
		now.Sub(envelope.IssuedAt) > maxExecutionEnvelopeAge ||
		!envelope.IssuedAt.Before(record.packet.ExpiresAt) {
		return errEnvelopeDenied
	}
	capability := pilot.dispatcher.credentials[record.receipt.TargetAgentID]
	expected, err := signErrandToolEnvelope(envelope, capability)
	if err != nil {
		return errEnvelopeDenied
	}
	provided, decodeErr := hex.DecodeString(envelope.Signature)
	expectedBytes, expectedErr := hex.DecodeString(expected)
	if decodeErr != nil || expectedErr != nil ||
		subtle.ConstantTimeCompare(provided, expectedBytes) != 1 {
		return errEnvelopeDenied
	}
	return nil
}

func (pilot *Pilot) complete(record pilotRecord, envelope ExecutionEnvelope, failureCode string, state State, reviewState ...ReviewState) Receipt {
	receipt := record.receipt
	receipt.State = state
	receipt.ResultSHA256 = envelope.ResultSHA256
	receipt.FailureCode = failureCode
	if state != StatePendingReview {
		receipt.CompletedAt = pilot.now().UTC()
	}
	if receipt.Review != nil && len(reviewState) > 0 && reviewState[0] != "" {
		receipt.Review.State = reviewState[0]
	}
	pilot.records[receipt.DelegationID] = pilotRecord{receipt: receipt, packet: record.packet}
	pilot.usedNonces[envelope.Nonce] = true
	return receipt
}

func (pilot *Pilot) rejectEnvelope(envelope ExecutionEnvelope, code string, cause error) (Receipt, error) {
	if record, ok := pilot.records[envelope.PacketID]; ok {
		receipt := record.receipt
		receipt.State = StateFailed
		receipt.FailureCode = code
		return receipt, cause
	}
	return Receipt{
		SchemaVersion: 1, OwnerAgentID: "maestro",
		State: StateFailed, FailureCode: code,
	}, cause
}

func (pilot *Pilot) recordFailure(scopeKind, targetID, scopeID, code string, state State) (Receipt, error) {
	id, err := randomID()
	if err != nil {
		return Receipt{}, err
	}
	// Invalid input must not turn public failure receipts into a storage
	// channel. Keep identity only after it passes the same opaque-ID boundary
	// used by successful delegations.
	if !agentcatalog.ValidAgentID(scopeID) {
		scopeID = ""
	}
	if targetID != "" && !agentcatalog.ValidAgentID(targetID) {
		targetID = ""
	}
	if scopeKind != "workspace" && scopeKind != "errand" {
		scopeKind = ""
	}
	receipt := Receipt{
		SchemaVersion: 1, DelegationID: id, OwnerAgentID: "maestro", TargetAgentID: targetID,
		Runtime: pilot.runtime, ScopeKind: scopeKind, ScopeID: scopeID, State: state,
		FailureCode: code, CompletedAt: pilot.now().UTC(),
	}
	pilot.records[id] = pilotRecord{receipt: receipt}
	return receipt, errors.New(code)
}

// recordReviewFailure keeps the unavailable/failed projection as useful as a
// successful review without persisting any review body. The source binding is
// copied from the producer receipt, never from untrusted request prose.
func (pilot *Pilot) recordReviewFailure(source pilotRecord, request WalterReviewRequest, code string, state State) (Receipt, error) {
	receipt, err := pilot.recordFailure("review", "walter", "review", code, state)
	if request.Trigger.valid() {
		receipt.Review = &ReviewSummary{Trigger: request.Trigger, State: ReviewUnavailable}
		if source.receipt.DelegationID != "" {
			receipt.Review.SourcePacketID = source.receipt.DelegationID
			receipt.Review.SourcePacketSHA256 = source.receipt.PacketSHA256
		}
		record := pilot.records[receipt.DelegationID]
		record.receipt = receipt
		pilot.records[receipt.DelegationID] = record
	}
	return receipt, err
}

var (
	errEnvelopeDenied        = errors.New("execution envelope is not authorized for the active dispatch")
	errEnvelopeReplayed      = errors.New("execution envelope was already consumed")
	errDelegationUnavailable = errors.New("delegation body is process-local and unavailable after restart")
)

func envelopeFailureCode(err error) string {
	if errors.Is(err, errEnvelopeReplayed) {
		return "envelope_replayed"
	}
	if errors.Is(err, errDelegationUnavailable) {
		return "delegation_unavailable_after_restart"
	}
	return "envelope_denied"
}

func validWorkspaceIntent(intent Intent) bool {
	return agentcatalog.ValidAgentID(strings.TrimSpace(intent.WorkspaceID)) &&
		strings.TrimSpace(intent.Objective) != "" &&
		len([]byte(strings.TrimSpace(intent.Objective))) <= maxObjectiveBytes &&
		intent.TTL > 0 && intent.TTL <= maxPacketTTL &&
		len(intent.Pointers) <= maxPointers && len(intent.Constraints) <= maxConstraints
}

func errandContract(intent ErrandIntent) (ErrandContract, error) {
	if !agentcatalog.ValidAgentID(strings.TrimSpace(intent.ErrandID)) ||
		strings.TrimSpace(intent.Objective) == "" ||
		len([]byte(strings.TrimSpace(intent.Objective))) > maxObjectiveBytes ||
		intent.TTL <= 0 || intent.TTL > maxPacketTTL ||
		intent.Grant.Operation != ErrandCreateEphemeralNote ||
		!validEphemeralNoteResource(intent.Grant.Resource, intent.ErrandID) {
		return ErrandContract{}, errors.New("errand grant is outside the closed pilot allowlist")
	}
	normalized, _ := agentorchestration.NormalizeResource(intent.Grant.Resource)
	grant := ErrandGrant{Operation: ErrandCreateEphemeralNote, Resource: normalized}
	return ErrandContract{
		Tool: errandTool, Grant: grant,
		Compensation: ErrandGrant{Operation: ErrandDeleteEphemeralNote, Resource: normalized},
	}, nil
}

func validEphemeralNoteResource(resource, errandID string) bool {
	normalized, valid := agentorchestration.NormalizeResource(resource)
	if !valid || normalized != resource || !agentorchestration.ResourceWithinScope(resource, "errand", errandID) {
		return false
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return false
	}
	prefix := "/" + errandID + "/ephemeral-notes/"
	name := strings.TrimPrefix(parsed.Path, prefix)
	return strings.HasPrefix(parsed.Path, prefix) && strings.Count(name, "/") == 0 &&
		strings.HasSuffix(name, ".md") && agentcatalog.ValidAgentID(strings.TrimSuffix(name, ".md"))
}

func validateReturnBody(result ReturnBody, scopeKind, scopeID string) error {
	if strings.TrimSpace(result.Summary) == "" ||
		len([]byte(strings.TrimSpace(result.Summary))) > maxObjectiveBytes ||
		len([]byte(strings.TrimSpace(result.Uncertainty))) > maxConstraintBytes ||
		len(result.EvidenceRefs) > maxPointers {
		return errors.New("delegated return exceeds its bounded contract")
	}
	seen := make(map[string]bool, len(result.EvidenceRefs))
	for _, ref := range result.EvidenceRefs {
		normalized, valid := agentorchestration.NormalizeResource(ref)
		if !valid || normalized != ref ||
			!agentorchestration.ResourceWithinScope(ref, scopeKind, scopeID) || seen[ref] {
			return errors.New("delegated return contains an invalid, duplicate or cross-scope evidence pointer")
		}
		seen[ref] = true
	}
	return nil
}

func normalizeReturnBody(result ReturnBody) ReturnBody {
	result.Summary = strings.TrimSpace(result.Summary)
	result.Uncertainty = strings.TrimSpace(result.Uncertainty)
	result.EvidenceRefs = append([]string(nil), result.EvidenceRefs...)
	sort.Strings(result.EvidenceRefs)
	return result
}

func validateFailureBody(body FailureBody) error {
	code := strings.TrimSpace(body.Code)
	if code == "" || len([]byte(code)) > maxFailureCodeBytes ||
		strings.TrimSpace(body.Detail) == "" ||
		len([]byte(strings.TrimSpace(body.Detail))) > maxObjectiveBytes {
		return errors.New("failure body is incomplete or oversized")
	}
	for _, character := range code {
		if (character < 'a' || character > 'z') && (character < '0' || character > '9') && character != '_' {
			return errors.New("failure code is invalid")
		}
	}
	return nil
}

func normalizeFailureBody(body FailureBody) FailureBody {
	body.Code = strings.TrimSpace(body.Code)
	body.Detail = strings.TrimSpace(body.Detail)
	return body
}

func digestBody(value any) string {
	body, _ := json.Marshal(value)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}

func validSHA256(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}

func signExecutionEnvelope(envelope ExecutionEnvelope, capability string) (string, error) {
	if capability == "" {
		return "", errors.New("execution capability is required")
	}
	envelope.Signature = ""
	body, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte("bcgos-execution-envelope-v1\x00" + capability))
	mac := hmac.New(sha256.New, key[:])
	if _, err := mac.Write(body); err != nil {
		return "", err
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func signErrandToolEnvelope(envelope ErrandToolEnvelope, capability string) (string, error) {
	if capability == "" {
		return "", errors.New("execution capability is required")
	}
	envelope.Signature = ""
	body, err := json.Marshal(envelope)
	if err != nil {
		return "", err
	}
	key := sha256.Sum256([]byte("bcgos-errand-tool-envelope-v1\x00" + capability))
	mac := hmac.New(sha256.New, key[:])
	if _, err := mac.Write(body); err != nil {
		return "", err
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}
