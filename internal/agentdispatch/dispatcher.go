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
)

const (
	maxObjectiveBytes  = 1000
	maxPointers        = 12
	maxConstraints     = 8
	maxConstraintBytes = 300
	maxPacketTTL       = 24 * time.Hour
)

type PacketRequest struct {
	TargetAgentID string
	ScopeKind     string
	ScopeID       string
	Objective     string
	Pointers      []string
	Constraints   []string
	TTL           time.Duration
}

type WorkPacket struct {
	SchemaVersion  int       `json:"schema_version"`
	PacketID       string    `json:"packet_id"`
	ParentPacketID string    `json:"parent_packet_id,omitempty"`
	IssuerAgentID  string    `json:"issuer_agent_id"`
	TargetAgentID  string    `json:"target_agent_id"`
	ScopeKind      string    `json:"scope_kind"`
	ScopeID        string    `json:"scope_id"`
	Objective      string    `json:"objective"`
	Pointers       []string  `json:"pointers,omitempty"`
	Constraints    []string  `json:"constraints,omitempty"`
	IssuedAt       time.Time `json:"issued_at"`
	ExpiresAt      time.Time `json:"expires_at"`
	Signature      string    `json:"signature"`
}

type Dispatcher struct {
	gate        *agentorchestration.Adapter
	signingKey  []byte
	credentials map[string]string
	now         func() time.Time
}

func New(gate *agentorchestration.Adapter, signingCapability string, credentials map[string]string) (*Dispatcher, error) {
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
	return &Dispatcher{
		gate: gate, signingKey: []byte(signingCapability),
		credentials: values, now: time.Now,
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
	if err := dispatcher.Verify(parent); err != nil {
		return WorkPacket{}, packetDenied(), err
	}
	if request.ScopeID != parent.ScopeID || request.ScopeKind != parent.ScopeKind {
		return WorkPacket{}, packetDenied(), errors.New("child packet must inherit the parent scope root")
	}
	issuer := parent.TargetAgentID
	capability := dispatcher.credentials[issuer]
	if capability == "" || dispatcher.credentials[request.TargetAgentID] == "" {
		return WorkPacket{}, packetDenied(), errors.New("child packet issuer or target is not authorized")
	}
	packet, err := dispatcher.issue(issuer, parent.PacketID, request)
	if err != nil {
		return WorkPacket{}, packetDenied(), err
	}
	decision := dispatcher.gate.StartChild(
		issuer, capability, request.TargetAgentID, parent.PacketID,
		packet.PacketID, request.ScopeID, request.ScopeKind,
	)
	if !decision.Allowed {
		return WorkPacket{}, decision, nil
	}
	return packet, decision, nil
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

func (dispatcher *Dispatcher) issue(issuer, parentID string, request PacketRequest) (WorkPacket, error) {
	if err := validateRequest(request); err != nil {
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
		SchemaVersion: 1, PacketID: packetID, ParentPacketID: parentID,
		IssuerAgentID: issuer, TargetAgentID: request.TargetAgentID,
		ScopeKind: request.ScopeKind, ScopeID: request.ScopeID,
		Objective: strings.TrimSpace(request.Objective), Pointers: pointers,
		Constraints: append([]string(nil), request.Constraints...),
		IssuedAt:    now, ExpiresAt: now.Add(request.TTL),
	}
	packet.Signature, err = dispatcher.signature(packet)
	return packet, err
}

func validateRequest(request PacketRequest) error {
	objective := strings.TrimSpace(request.Objective)
	if !agentcatalog.ValidAgentID(request.TargetAgentID) || request.ScopeKind == "" || request.ScopeID == "" ||
		objective == "" || len([]byte(objective)) > maxObjectiveBytes ||
		request.TTL <= 0 || request.TTL > maxPacketTTL ||
		len(request.Pointers) > maxPointers || len(request.Constraints) > maxConstraints {
		return errors.New("work packet exceeds its bounded contract")
	}
	for _, constraint := range request.Constraints {
		if strings.TrimSpace(constraint) == "" || len([]byte(constraint)) > maxConstraintBytes {
			return errors.New("work packet constraint is empty or oversized")
		}
	}
	return nil
}

func (dispatcher *Dispatcher) Verify(packet WorkPacket) error {
	if packet.SchemaVersion != 1 || !agentcatalog.ValidAgentID(packet.IssuerAgentID) ||
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
	if err := validateRequest(PacketRequest{
		TargetAgentID: packet.TargetAgentID, ScopeKind: packet.ScopeKind,
		ScopeID: packet.ScopeID, Objective: packet.Objective, Pointers: packet.Pointers,
		Constraints: packet.Constraints, TTL: packet.ExpiresAt.Sub(packet.IssuedAt),
	}); err != nil {
		return err
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
