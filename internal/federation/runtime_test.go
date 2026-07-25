package federation

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestPortableSkillUsesDedicatedAutomaticOutboxAndBridgeRoute(t *testing.T) {
	now := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	store := ExportStore{Root: t.TempDir()}
	if err := store.Enroll(Enrollment{InstallationID: "0123456789abcdef", BridgeEndpoint: "https://placeholder.invalid/federation/v1/batches", ContractVersion: PilotContractVersion, AcceptedAt: now, AutomaticExport: true}); err != nil {
		t.Fatal(err)
	}
	portableRoot := t.TempDir()
	writePortableSkill(t, portableRoot, "handoff-guard", "# Handoff guard\n\nGeneralizable checklist.\n")
	packages, err := (PortableSkillCollector{Root: portableRoot}).Collect()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.EnqueuePortable(packages[0], now); err != nil {
		t.Fatal(err)
	}
	inbox := CentralInbox{Root: t.TempDir(), AllowedInstallations: map[string]bool{"0123456789abcdef": true}}
	server := httptest.NewTLSServer(CentralBridge{Inbox: inbox})
	defer server.Close()
	bridge, err := NewHTTPBridge(server.URL+"/federation/v1/batches", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	report, err := store.FlushPortable(context.Background(), bridge, now)
	if err != nil {
		t.Fatal(err)
	}
	if report.Delivered != 1 {
		t.Fatalf("report = %#v", report)
	}
	accepted, err := inbox.PortableSkills()
	if err != nil || len(accepted) != 1 || accepted[0].Content != packages[0].Content {
		t.Fatalf("accepted = %#v, err = %v", accepted, err)
	}
}

func TestWeeklyFederationExecutorCompilesQueuesAndDeliversWithoutPrivateContext(t *testing.T) {
	now := time.Date(2026, 7, 25, 15, 0, 0, 0, time.UTC)
	store := enrolledStore(t, now)
	bridge := &recordingBridge{}
	executor := WeeklyFederationExecutor{
		Store:  store,
		Bridge: bridge,
		Now:    func() time.Time { return now },
		Build: LocalDarwinPacketBuilderFunc(func(context.Context, scheduler.Occurrence) (LocalDarwinPacket, error) {
			packet := validLocalPacket()
			packet.PrivateContext = "client secret CANARY"
			packet.WorkspaceID = "client-workspace-CANARY"
			return packet, nil
		}),
	}
	if err := executor.Execute(context.Background(), scheduler.Occurrence{JobID: WeeklyFederationJobID, ScheduledFor: now}); err != nil {
		t.Fatal(err)
	}
	if len(bridge.batches) != 1 || bridge.batches[0].InstallationID != "0123456789abcdef" {
		t.Fatalf("bridge batches = %#v", bridge.batches)
	}
	encoded, _ := json.Marshal(bridge.batches[0])
	if strings.Contains(string(encoded), "CANARY") {
		t.Fatalf("private context crossed weekly executor: %s", encoded)
	}
}

func TestPortableEndpointCannotBeDerivedFromUnexpectedBatchRoute(t *testing.T) {
	bridge, err := NewHTTPBridge("https://bridge.maestro.example/not-batches", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := bridge.portableEndpoint(); err == nil {
		t.Fatal("unexpected batch route derived a portable endpoint")
	}
}
