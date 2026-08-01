// Package agentdispatch issues bounded, signed work packets and sequences
// governed agent branches through the shared orchestration guard.
package agentdispatch

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentorchestration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillpolicy"
)

const (
	legacyPacketSchemaVersion = 1
	packetSchemaVersion       = 2
	maxObjectiveBytes         = 1000
	maxPointers               = 12
	maxConstraints            = 8
	maxConstraintBytes        = 300
	maxPacketTTL              = 24 * time.Hour
)

type PacketRequest struct {
	TargetAgentID string
	ScopeKind     string
	ScopeID       string
	Objective     string
	Pointers      []string
	Constraints   []string
	SkillID       string
	// ReviewTrigger is signed into the producer packet so materiality cannot
	// be added only after the producer has already completed.
	ReviewTrigger WalterReviewTrigger
	// ReworkOfPacketID binds a new producer attempt to the prior material
	// packet after Walter requests refinement.
	ReworkOfPacketID string
	Review           *ReviewPacket
	TTL              time.Duration
}

type WorkPacket struct {
	SchemaVersion    int                 `json:"schema_version"`
	PacketID         string              `json:"packet_id"`
	ParentPacketID   string              `json:"parent_packet_id,omitempty"`
	IssuerAgentID    string              `json:"issuer_agent_id"`
	TargetAgentID    string              `json:"target_agent_id"`
	ScopeKind        string              `json:"scope_kind"`
	ScopeID          string              `json:"scope_id"`
	Objective        string              `json:"objective"`
	Pointers         []string            `json:"pointers,omitempty"`
	Constraints      []string            `json:"constraints,omitempty"`
	SkillID          string              `json:"skill_id,omitempty"`
	ReviewTrigger    WalterReviewTrigger `json:"review_trigger,omitempty"`
	ReworkOfPacketID string              `json:"rework_of_packet_id,omitempty"`
	Review           *ReviewPacket       `json:"review,omitempty"`
	IssuedAt         time.Time           `json:"issued_at"`
	ExpiresAt        time.Time           `json:"expires_at"`
	Signature        string              `json:"signature"`
}

type Dispatcher struct {
	gate        *agentorchestration.Adapter
	signingKey  []byte
	credentials map[string]string
	skills      skillpolicy.Registry
	now         func() time.Time
}

func New(gate *agentorchestration.Adapter, signingCapability string, credentials map[string]string, policies ...skillpolicy.Registry) (*Dispatcher, error) {
	if gate == nil || signingCapability == "" || credentials["maestro"] == "" {
		return nil, errors.New("dispatcher requires a gate, signing capability and Maestro credential")
	}
	values := make(map[string]string, len(credentials))
	for id, capability := range credentials {
		if !agentcatalog.ValidAgentID(id) || capability == "" {
			return nil, errors.New("dispatcher credential registry is invalid")
		}
		values[id] = capability
	}
	var skills skillpolicy.Registry
	if len(policies) > 1 {
		return nil, errors.New("dispatcher accepts at most one managed skill policy")
	}
	if len(policies) == 1 {
		skills = policies[0]
	}
	return &Dispatcher{
		gate: gate, signingKey: []byte(signingCapability),
		credentials: values, skills: skills, now: time.Now,
	}, nil
}

func (dispatcher *Dispatcher) StartRoot(request PacketRequest) (WorkPacket, agentorchestration.Decision, error) {
	if dispatcher.credentials[request.TargetAgentID] == "" {
		return WorkPacket{}, packetDenied(), errors.New("root target has no dispatcher credential")
	}
	packet, err := dispatcher.issue("maestro", "", request)
	if err != nil {
		return WorkPacket{}, packetDenied(), err
	}
	decision := dispatcher.gate.StartBranch(
		"maestro", dispatcher.credentials["maestro"], request.TargetAgentID,
		packet.PacketID, request.ScopeID, request.ScopeKind,
	)
	if !decision.Allowed {
		return WorkPacket{}, decision, nil
	}
	return packet, decision, nil
}

func (dispatcher *Dispatcher) StartChild(parent WorkPacket, request PacketRequest) (WorkPacket, agentorchestration.Decision, error) {
	return WorkPacket{}, agentorchestration.Decision{Allowed: false, Code: "nesting_denied"}, errors.New("Maestro delegation is sequential and does not permit nested spokes")
}

// SelectDirectSkill verifies that a vertical agent's role may select a managed
// method inside its own context. It intentionally does not create a packet,
// grant tools or transfer ownership to another agent.
func (dispatcher *Dispatcher) SelectDirectSkill(root WorkPacket, agentID, capability, skillID string) error {
	if root.SchemaVersion != packetSchemaVersion || root.ParentPacketID != "" ||
		root.TargetAgentID != agentID || dispatcher.Verify(root) != nil {
		return errors.New("direct skill selection requires a current signed root packet")
	}
	if decision := dispatcher.gate.AuthorizeActiveRoot(agentID, capability, root.PacketID, root.ScopeID, root.ScopeKind); !decision.Allowed {
		return errors.New("direct skill selection is not authorized for the active root")
	}
	role, ok := dispatcher.gate.RoleForAgent(agentID)
	if !ok || !dispatcher.skills.AllowsDirect(role, skillID) {
		return errors.New("direct skill selection is not allowed for this agent role")
	}
	return nil
}

func (dispatcher *Dispatcher) FinishChild(packet WorkPacket) agentorchestration.Decision {
	if err := dispatcher.Verify(packet); err != nil || packet.ParentPacketID == "" {
		return packetDenied()
	}
	capability := dispatcher.credentials[packet.TargetAgentID]
	if capability == "" {
		return packetDenied()
	}
	return dispatcher.gate.FinishChild(packet.TargetAgentID, capability, packet.ParentPacketID, packet.PacketID)
}

func (dispatcher *Dispatcher) FinishRoot(packet WorkPacket) agentorchestration.Decision {
	if err := dispatcher.Verify(packet); err != nil || packet.ParentPacketID != "" {
		return packetDenied()
	}
	capability := dispatcher.credentials[packet.TargetAgentID]
	if capability == "" {
		return packetDenied()
	}
	return dispatcher.gate.FinishBranch(packet.TargetAgentID, capability, packet.PacketID)
}

func (dispatcher *Dispatcher) guardRootTool(packet WorkPacket, tool, operation, resource string) agentorchestration.Decision {
	if err := dispatcher.Verify(packet); err != nil || packet.ParentPacketID != "" {
		return packetDenied()
	}
	capability := dispatcher.credentials[packet.TargetAgentID]
	if capability == "" {
		return packetDenied()
	}
	return dispatcher.gate.GuardTool(
		packet.TargetAgentID, capability, packet.PacketID, "",
		packet.ScopeID, packet.ScopeKind, tool, operation, resource,
	)
}

func (dispatcher *Dispatcher) issue(issuer, parentID string, request PacketRequest) (WorkPacket, error) {
	child := parentID != ""
	if err := validateRequest(request, child); err != nil {
		return WorkPacket{}, err
	}
	if err := dispatcher.validateSkillSelection(issuer, request.TargetAgentID, request.SkillID, child); err != nil {
		return WorkPacket{}, err
	}
	packetID, err := randomID()
	if err != nil {
		return WorkPacket{}, err
	}
	now := dispatcher.now().UTC()
	pointers := make([]string, 0, len(request.Pointers))
	seen := make(map[string]bool, len(request.Pointers))
	for _, pointer := range request.Pointers {
		normalized, valid := agentorchestration.NormalizeResource(pointer)
		if !valid || !agentorchestration.ResourceWithinScope(normalized, request.ScopeKind, request.ScopeID) || seen[normalized] {
			return WorkPacket{}, errors.New("work packet contains an invalid, duplicate or cross-scope pointer")
		}
		seen[normalized] = true
		pointers = append(pointers, normalized)
	}
	packet := WorkPacket{
		SchemaVersion: packetSchemaVersion, PacketID: packetID, ParentPacketID: parentID,
		IssuerAgentID: issuer, TargetAgentID: request.TargetAgentID,
		ScopeKind: request.ScopeKind, ScopeID: request.ScopeID,
		Objective: strings.TrimSpace(request.Objective), Pointers: pointers,
		Constraints: append([]string(nil), request.Constraints...), SkillID: request.SkillID,
		ReviewTrigger:    request.ReviewTrigger,
		ReworkOfPacketID: request.ReworkOfPacketID,
		Review:           cloneReviewPacket(request.Review),
		IssuedAt:         now, ExpiresAt: now.Add(request.TTL),
	}
	if err := validateReviewPacket(packet.Review, packet.PacketID, packet.Objective); err != nil {
		return WorkPacket{}, err
	}
	packet.Signature, err = dispatcher.signature(packet)
	return packet, err
}

func validateRequest(request PacketRequest, child bool) error {
	objective := strings.TrimSpace(request.Objective)
	if !agentcatalog.ValidAgentID(request.TargetAgentID) || request.ScopeKind == "" || request.ScopeID == "" ||
		objective == "" || len([]byte(objective)) > maxObjectiveBytes ||
		request.TTL <= 0 || request.TTL > maxPacketTTL ||
		len(request.Pointers) > maxPointers || len(request.Constraints) > maxConstraints {
		return errors.New("work packet exceeds its bounded contract")
	}
	if (request.TargetAgentID == "walter") != (request.Review != nil) {
		return errors.New("Walter dispatch requires exactly one sealed review packet")
	}
	if request.ReviewTrigger != "" && !request.ReviewTrigger.valid() {
		return errors.New("work packet has an invalid Walter review trigger")
	}
	if request.ReworkOfPacketID != "" && (!validPacketID(request.ReworkOfPacketID) || child) {
		return errors.New("work packet has an invalid rework binding")
	}
	if request.Review != nil && request.ReworkOfPacketID != "" {
		return errors.New("Walter review packet cannot carry a rework binding")
	}
	if request.Review != nil && request.ReviewTrigger != "" {
		return errors.New("Walter review packet cannot carry a producer review trigger")
	}
	if request.Review != nil && (child || request.ScopeKind != "review") {
		return errors.New("Walter review must be a direct review root")
	}
	if child && request.ReviewTrigger != "" {
		return errors.New("child packet cannot define a material review trigger")
	}
	if !child && request.SkillID != "" {
		return errors.New("work packet has an invalid skill selection boundary")
	}
	for _, constraint := range request.Constraints {
		if strings.TrimSpace(constraint) == "" || len([]byte(constraint)) > maxConstraintBytes {
			return errors.New("work packet constraint is empty or oversized")
		}
	}
	return nil
}

func (dispatcher *Dispatcher) validateSkillSelection(issuer, target, skillID string, child bool) error {
	if !child {
		return nil
	}
	_, issuerOK := dispatcher.gate.RoleForAgent(issuer)
	_, targetOK := dispatcher.gate.RoleForAgent(target)
	if !issuerOK || !targetOK {
		return errors.New("delegated skill selection is not allowed for these agent roles")
	}
	if skillID != "" {
		return errors.New("nested skill selection is unavailable because spokes cannot delegate")
	}
	return nil
}

func (dispatcher *Dispatcher) Verify(packet WorkPacket) error {
	if (packet.SchemaVersion != legacyPacketSchemaVersion && packet.SchemaVersion != packetSchemaVersion) || !agentcatalog.ValidAgentID(packet.IssuerAgentID) ||
		!agentcatalog.ValidAgentID(packet.TargetAgentID) || !validPacketID(packet.PacketID) ||
		(packet.ParentPacketID != "" && !validPacketID(packet.ParentPacketID)) || packet.Signature == "" {
		return errors.New("work packet header is invalid")
	}
	expected, err := dispatcher.signature(packet)
	if err != nil {
		return err
	}
	provided, err := hex.DecodeString(packet.Signature)
	if err != nil || !hmac.Equal(provided, mustDecodeHex(expected)) {
		return errors.New("work packet signature is invalid")
	}
	now := dispatcher.now().UTC()
	if packet.IssuedAt.After(now) || !packet.ExpiresAt.After(now) || packet.ExpiresAt.Sub(packet.IssuedAt) > maxPacketTTL {
		return errors.New("work packet is not within its validity window")
	}
	child := packet.ParentPacketID != ""
	request := PacketRequest{
		TargetAgentID: packet.TargetAgentID, ScopeKind: packet.ScopeKind,
		ScopeID: packet.ScopeID, Objective: packet.Objective, Pointers: packet.Pointers,
		Constraints: packet.Constraints, SkillID: packet.SkillID, ReviewTrigger: packet.ReviewTrigger,
		ReworkOfPacketID: packet.ReworkOfPacketID,
		Review:           cloneReviewPacket(packet.Review), TTL: packet.ExpiresAt.Sub(packet.IssuedAt),
	}
	if packet.SchemaVersion == legacyPacketSchemaVersion {
		if packet.SkillID != "" || validateLegacyRequest(request) != nil {
			return errors.New("legacy work packet violates its completion-only contract")
		}
	} else if err := validateRequest(request, child); err != nil {
		return err
	}
	if err := validateReviewPacket(packet.Review, packet.PacketID, packet.Objective); err != nil {
		return err
	}
	if packet.SchemaVersion == packetSchemaVersion {
		if err := dispatcher.validateSkillSelection(packet.IssuerAgentID, packet.TargetAgentID, packet.SkillID, child); err != nil {
			return err
		}
	}
	seen := make(map[string]bool, len(packet.Pointers))
	for _, pointer := range packet.Pointers {
		normalized, valid := agentorchestration.NormalizeResource(pointer)
		if !valid || normalized != pointer ||
			!agentorchestration.ResourceWithinScope(pointer, packet.ScopeKind, packet.ScopeID) ||
			seen[pointer] {
			return errors.New("work packet pointer contract is invalid")
		}
		seen[pointer] = true
	}
	return nil
}

// validateLegacyRequest accepts only schema-v1 packets already in flight so
// they can finish safely after the schema-v2 skill boundary is deployed.
func validateLegacyRequest(request PacketRequest) error {
	if request.SkillID != "" {
		return errors.New("legacy work packet cannot carry a skill selection")
	}
	return validateRequest(request, false)
}

func (dispatcher *Dispatcher) signature(packet WorkPacket) (string, error) {
	packet.Signature = ""
	body, err := json.Marshal(packet)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, dispatcher.signingKey)
	if _, err := mac.Write(body); err != nil {
		return "", err
	}
	return hex.EncodeToString(mac.Sum(nil)), nil
}

func randomID() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func validPacketID(value string) bool {
	if len(value) != 64 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32
}

func mustDecodeHex(value string) []byte {
	decoded, _ := hex.DecodeString(value)
	return decoded
}

func packetDenied() agentorchestration.Decision {
	return agentorchestration.Decision{Allowed: false, Code: "packet_denied"}
}
