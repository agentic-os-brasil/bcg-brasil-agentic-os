package darwin

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestBuildHealthPacketUsesOnlyDeterministicProductSurfaces(t *testing.T) {
	request := HealthRequest{SchemaVersion: SchemaVersion, WindowID: "health-window", Runtime: "claude", Mode: Interactive, Surfaces: HealthSurfaces{
		Doctor: ProductSurface{State: "healthy", Count: 1}, Capability: ProductSurface{State: "unavailable", Count: 2},
		Validation: ProductSurface{State: "failed", Count: 1}, Scheduler: ProductSurface{State: "stale", Count: 3},
		ManagedState: ProductSurface{State: "stale", Count: 1}, FrictionCodes: []string{"native_unqualified", "receipt_missing"},
	}}
	packet, err := BuildHealthPacket(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(packet.Observations) != 5 || packet.Observations[0].Code != ObservationCapabilityUnavailable {
		t.Fatalf("packet = %#v", packet)
	}
	if err := packet.Validate(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mustJSON(t, packet)), "unavailable") == false {
		t.Fatal("closed product state was not retained")
	}
}

func TestAssessHealthIsBoundedAndDeterministic(t *testing.T) {
	request := HealthRequest{SchemaVersion: SchemaVersion, WindowID: "health-empty", Runtime: "codex", Mode: DeepReview, Surfaces: HealthSurfaces{Doctor: ProductSurface{State: "healthy", Count: 1}}}
	one, err := AssessHealth(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	two, err := AssessHealth(context.Background(), request)
	if err != nil || one.PacketSHA256 != two.PacketSHA256 || len(one.Assessment.Proposals) != 0 {
		t.Fatalf("assessment parity = %#v %#v %v", one, two, err)
	}
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return body
}
