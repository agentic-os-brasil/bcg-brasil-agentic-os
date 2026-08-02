package darwin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
)

// ProductSurface is a bounded state/count pair. It intentionally has no
// workspace, client, path, prompt, person, raw-error or credential fields.
type ProductSurface struct {
	State string `json:"state"`
	Count int    `json:"count"`
}

type HealthSurfaces struct {
	Doctor         ProductSurface `json:"doctor"`
	Capability     ProductSurface `json:"capability"`
	Validation     ProductSurface `json:"validation"`
	Scheduler      ProductSurface `json:"scheduler"`
	ManagedState   ProductSurface `json:"managed_state"`
	StateDocuments ProductSurface `json:"state_documents"`
	FrictionCodes  []string       `json:"friction_codes"`
}

type HealthRequest struct {
	SchemaVersion int            `json:"schema_version"`
	WindowID      string         `json:"window_id"`
	Runtime       string         `json:"runtime"`
	Mode          Mode           `json:"mode"`
	Surfaces      HealthSurfaces `json:"surfaces"`
}

type HealthAssessment struct {
	Packet       HealthPacket `json:"packet"`
	Assessment   Assessment   `json:"assessment"`
	PacketSHA256 string       `json:"packet_sha256"`
}

var validSurfaceStates = map[string]bool{
	"": true, "healthy": true, "warning": true, "failed": true,
	"stale": true, "missing": true, "blocked": true, "unavailable": true,
}

var validFrictionCodes = map[string]bool{
	"catalog_only":       true,
	"native_unqualified": true,
	"receipt_missing":    true,
	"lease_recovery":     true,
	"hook_signal_only":   true,
	"validation_summary": true,
}

func (surface ProductSurface) validate() error {
	if !validSurfaceStates[surface.State] || surface.Count < 0 || surface.Count > 1000 {
		return errors.New("invalid Darwin product surface")
	}
	return nil
}

func (surfaces HealthSurfaces) Validate() error {
	for _, surface := range []ProductSurface{surfaces.Doctor, surfaces.Capability, surfaces.Validation, surfaces.Scheduler, surfaces.ManagedState, surfaces.StateDocuments} {
		if err := surface.validate(); err != nil {
			return err
		}
	}
	if len(surfaces.FrictionCodes) > 16 {
		return errors.New("too many Darwin friction codes")
	}
	seen := map[string]bool{}
	for _, code := range surfaces.FrictionCodes {
		if !validFrictionCodes[code] || seen[code] {
			return errors.New("invalid or duplicate Darwin friction code")
		}
		seen[code] = true
	}
	return nil
}

func BuildHealthPacket(request HealthRequest) (HealthPacket, error) {
	if request.SchemaVersion != SchemaVersion || !idPattern.MatchString(request.WindowID) || !validRuntimes[request.Runtime] || !validMode(request.Mode) {
		return HealthPacket{}, errors.New("invalid Darwin health request")
	}
	if err := request.Surfaces.Validate(); err != nil {
		return HealthPacket{}, err
	}
	observations := make([]Observation, 0, 6)
	add := func(code ObservationCode, severity Severity, surface ProductSurface) {
		if surface.Count > 0 && surface.State != "healthy" && surface.State != "" {
			observations = append(observations, Observation{Code: code, Severity: severity, Count: surface.Count, State: surface.State})
		}
	}
	add(ObservationCapabilityUnavailable, SeverityHigh, request.Surfaces.Capability)
	add(ObservationStateStale, SeverityMedium, request.Surfaces.ManagedState)
	add(ObservationSchedulerMissed, SeverityHigh, request.Surfaces.Scheduler)
	add(ObservationValidationFailure, SeverityHigh, request.Surfaces.Validation)
	add(ObservationContractDrift, SeverityMedium, request.Surfaces.Doctor)
	add(ObservationStateDocumentsOversized, SeverityMedium, request.Surfaces.StateDocuments)
	if len(request.Surfaces.FrictionCodes) > 0 {
		observations = append(observations, Observation{Code: ObservationOperatingFriction, Severity: SeverityLow, Count: len(request.Surfaces.FrictionCodes), State: "derived"})
	}
	sort.Slice(observations, func(i, j int) bool { return observations[i].Code < observations[j].Code })
	return HealthPacket{SchemaVersion: SchemaVersion, WindowID: request.WindowID, Runtime: request.Runtime, Mode: request.Mode, Observations: observations}, nil
}

func AssessHealth(ctx context.Context, request HealthRequest) (HealthAssessment, error) {
	if err := ctx.Err(); err != nil {
		return HealthAssessment{}, err
	}
	packet, err := BuildHealthPacket(request)
	if err != nil {
		return HealthAssessment{}, err
	}
	assessment, err := Plan(packet)
	if err != nil {
		return HealthAssessment{}, err
	}
	return HealthAssessment{Packet: packet, Assessment: assessment, PacketSHA256: DigestHealthPacket(packet)}, nil
}

func DigestHealthPacket(packet HealthPacket) string {
	body, _ := json.Marshal(packet)
	digest := sha256.Sum256(body)
	return hex.EncodeToString(digest[:])
}
