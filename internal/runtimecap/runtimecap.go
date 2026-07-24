// Package runtimecap validates the canonical runtime capability manifest and
// produces the same observable report for every supported agent runtime.
package runtimecap

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var (
	validStates        = map[string]bool{"native": true, "emulated": true, "degraded": true, "unavailable": true}
	validCriticality   = map[string]bool{"required": true, "optional": true}
	validRuntime       = map[string]bool{"claude": true, "codex": true}
	validSemanticEvent = map[string]bool{"": true, "session_start": true, "pre_action_guard": true, "post_action_observe": true, "stop_finalize": true, "context_inject": true}
)

type Manifest struct {
	SchemaVersion int          `json:"schema_version"`
	Capabilities  []Capability `json:"capabilities"`
}

type Capability struct {
	ID            string                       `json:"id"`
	Criticality   string                       `json:"criticality"`
	SemanticEvent string                       `json:"semantic_event"`
	Fallback      string                       `json:"fallback"`
	Runtimes      map[string]RuntimeCapability `json:"runtimes"`
}

type RuntimeCapability struct {
	State     string `json:"state"`
	Mechanism string `json:"mechanism"`
	Reason    string `json:"reason"`
}

type Report struct {
	Runtime      string             `json:"runtime"`
	Detected     bool               `json:"detected"`
	State        string             `json:"state"`
	Capabilities []CapabilityReport `json:"capabilities"`
}

type CapabilityReport struct {
	ID            string `json:"id"`
	Criticality   string `json:"criticality"`
	SemanticEvent string `json:"semantic_event,omitempty"`
	State         string `json:"state"`
	Mechanism     string `json:"mechanism"`
	Reason        string `json:"reason,omitempty"`
}

func Parse(reader io.Reader) (Manifest, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var manifest Manifest
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Manifest{}, errors.New("runtime capability manifest contains multiple JSON values")
		}
		return Manifest{}, err
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func (manifest Manifest) Validate() error {
	if manifest.SchemaVersion != 1 {
		return fmt.Errorf("runtime capability schema version %d is unsupported", manifest.SchemaVersion)
	}
	if len(manifest.Capabilities) == 0 {
		return errors.New("runtime capability manifest has no capabilities")
	}
	seen := map[string]bool{}
	for _, capability := range manifest.Capabilities {
		if capability.ID == "" || seen[capability.ID] {
			return fmt.Errorf("invalid or duplicate capability id %q", capability.ID)
		}
		seen[capability.ID] = true
		if !validCriticality[capability.Criticality] {
			return fmt.Errorf("capability %s has invalid criticality %q", capability.ID, capability.Criticality)
		}
		if !validSemanticEvent[capability.SemanticEvent] {
			return fmt.Errorf("capability %s has invalid semantic event %q", capability.ID, capability.SemanticEvent)
		}
		if !validStates[capability.Fallback] {
			return fmt.Errorf("capability %s has invalid fallback %q", capability.ID, capability.Fallback)
		}
		for runtime := range validRuntime {
			value, exists := capability.Runtimes[runtime]
			if !exists {
				return fmt.Errorf("capability %s is missing runtime %s", capability.ID, runtime)
			}
			if !validStates[value.State] || value.Mechanism == "" {
				return fmt.Errorf("capability %s has invalid runtime contract for %s", capability.ID, runtime)
			}
			if (value.State == "unavailable" || value.State == "degraded") && value.Reason == "" {
				return fmt.Errorf("capability %s requires a reason for %s on %s", capability.ID, value.State, runtime)
			}
		}
	}
	return nil
}

func (manifest Manifest) Report(runtime string, detected bool) (Report, error) {
	if !validRuntime[runtime] {
		return Report{}, fmt.Errorf("unsupported runtime %q", runtime)
	}
	report := Report{Runtime: runtime, Detected: detected, State: "ready"}
	if !detected {
		report.State = "unavailable"
	}
	for _, capability := range manifest.Capabilities {
		contract := capability.Runtimes[runtime]
		entry := CapabilityReport{
			ID: capability.ID, Criticality: capability.Criticality, SemanticEvent: capability.SemanticEvent,
			State: contract.State, Mechanism: contract.Mechanism, Reason: contract.Reason,
		}
		if !detected {
			entry.State = "unavailable"
			entry.Reason = "runtime executable was not detected"
		}
		report.Capabilities = append(report.Capabilities, entry)
	}
	return report, nil
}
