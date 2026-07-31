package darwinobservability

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestSchedulerAdapterDropsWorkspaceAndError(t *testing.T) {
	record, err := FromSchedulerReceipt(scheduler.Receipt{
		SchemaVersion: 1, WorkspaceID: "client-workspace-123", JobID: "darwin-housekeeping",
		ScheduledFor: time.Date(2026, 7, 29, 9, 0, 0, 0, time.UTC), AttemptedAt: time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC),
		State: scheduler.Failed, Error: "customer prompt and path must never leave local state",
	}, "week-1")
	if err != nil {
		t.Fatalf("adapter failed: %v", err)
	}
	body, _ := json.Marshal(record)
	text := string(body)
	for _, forbidden := range []string{"client-workspace-123", "customer prompt", "path must never", "workspace_id", "error"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("forbidden value leaked: %q in %s", forbidden, text)
		}
	}
	if record.Health.Recovery != RecoveryFailed || record.Health.Outcome != OutcomeFailed {
		t.Fatalf("unexpected typed mapping: %+v", record.Health)
	}
}

func TestDarwinAdapterIsMetadataOnly(t *testing.T) {
	record, err := FromDarwinReceipt(darwin.Receipt{
		SchemaVersion: darwin.SchemaVersion, AgentID: darwin.AgentID, DisplayName: darwin.DisplayName, Emoji: darwin.Emoji,
		WindowID: "week-1", Mode: darwin.HeadlessHousekeeping, Outcome: darwin.OutcomeSucceeded,
		RecordedAt: time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC), Actions: []darwin.ActionReceipt{{ProposalID: "private-client-proposal", Resource: "bcgos://health/maestro-system/private", Tool: "filesystem", Operation: "read"}},
	})
	if err != nil {
		t.Fatalf("adapter failed: %v", err)
	}
	body, _ := json.Marshal(record)
	text := string(body)
	if strings.Contains(text, "private-client-proposal") || strings.Contains(text, "bcgos://") || strings.Contains(text, "filesystem") {
		t.Fatalf("Darwin action detail leaked: %s", text)
	}
}
