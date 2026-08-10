// Package sessionstart creates the runtime-neutral input that a future
// lifecycle adapter consumes at Session Start. It never dereferences packet
// pointers or claims that native context injection is installed.
package sessionstart

import (
	"fmt"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/sessionctx"
)

type Envelope struct {
	SchemaVersion        int               `json:"schema_version"`
	Event                string            `json:"event"`
	Runtime              string            `json:"runtime"`
	State                string            `json:"state"`
	Packet               sessionctx.Packet `json:"packet"`
	AvailabilityState    string            `json:"availability_state"`
	AdapterDeliveryState string            `json:"adapter_delivery_state"`
	NativeEvidenceState  string            `json:"native_evidence_state"`
	InjectionState       string            `json:"injection_state"`
	Message              string            `json:"message"`
}

func Build(runtime string, packet sessionctx.Packet) (Envelope, error) {
	if runtime != "claude" && runtime != "codex" {
		return Envelope{}, fmt.Errorf("unsupported runtime %q", runtime)
	}
	if err := packet.Validate(); err != nil {
		return Envelope{}, fmt.Errorf("invalid session context packet: %w", err)
	}
	return Envelope{
		SchemaVersion:        1,
		Event:                "session_start",
		Runtime:              runtime,
		State:                packet.State,
		Packet:               packet,
		AvailabilityState:    "enabled",
		AdapterDeliveryState: "operational",
		NativeEvidenceState:  "native_qualification_pending",
		InjectionState:       "unavailable",
		Message:              "bounded session envelope is operational; native evidence is tracked separately",
	}, nil
}
