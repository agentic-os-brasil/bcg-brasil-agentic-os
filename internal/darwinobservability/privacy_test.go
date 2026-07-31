package darwinobservability

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestSchedulerAdapterDropsWorkspaceAndError(t *testing.T) {
	record, err := FromSchedulerReceipt(scheduler.Receipt{
		SchemaVersion: 1, WorkspaceID: "client-workspace-123", JobID: "darwin-housekeeping",
		ScheduledFor: time.Date(2026, 7, 29, 9, 0, 0, 0, time.UTC), AttemptedAt: time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC),
		State: scheduler.Failed, Error: "customer prompt and path must never leave local state",
	}, "week-1", strings.Repeat("f", 64))
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
	}, time.Date(2026, 7, 30, 8, 55, 0, 0, time.UTC), strings.Repeat("f", 64))
	if err != nil {
		t.Fatalf("adapter failed: %v", err)
	}
	body, _ := json.Marshal(record)
	text := string(body)
	if strings.Contains(text, "private-client-proposal") || strings.Contains(text, "bcgos://") || strings.Contains(text, "filesystem") {
		t.Fatalf("Darwin action detail leaked: %s", text)
	}
}

func TestActivationAdapterCannotHideMissingReceipt(t *testing.T) {
	record, err := FromActivationObservation(activationpolicy.Observation{
		SchemaVersion: 1, WindowID: "week-1", PlanSHA256: strings.Repeat("a", 64),
		PolicyVersion: activationpolicy.PolicyVersion, Posture: activationpolicy.Balanced,
		Route: activationpolicy.D1Targeted, Outcome: activationpolicy.CompletedOutcome,
		MissingReceipt: true,
	}, time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC), strings.Repeat("f", 64), PACoverageCovered, 1, nil)
	if err != nil {
		t.Fatalf("adapter failed: %v", err)
	}
	if record.Selection.Outcome != OutcomePartial || !containsGap(record.Selection.CapabilityGaps, GapReceiptCoverage) {
		t.Fatalf("missing receipt was hidden: %+v", record.Selection)
	}
	emptyWindow := activationpolicy.Observation{
		SchemaVersion: 1, PlanSHA256: strings.Repeat("a", 64), PolicyVersion: activationpolicy.PolicyVersion,
		Posture: activationpolicy.Balanced, Route: activationpolicy.D0Direct, Outcome: activationpolicy.CompletedOutcome,
	}
	if _, err := FromActivationObservation(emptyWindow, time.Now().UTC(), strings.Repeat("f", 64), PACoverageNotRequired, 0, nil); err == nil {
		t.Fatal("empty source window was converted into apparently valid evidence")
	}
}

func TestUsageCannotInflateRouteBudget(t *testing.T) {
	record, err := FromActivationObservation(activationpolicy.Observation{
		SchemaVersion: 1, WindowID: "week-1", PlanSHA256: strings.Repeat("a", 64),
		PolicyVersion: activationpolicy.PolicyVersion, Posture: activationpolicy.Direct,
		Route: activationpolicy.D0Direct, Outcome: activationpolicy.CompletedOutcome,
	}, time.Date(2026, 7, 30, 9, 0, 0, 0, time.UTC), strings.Repeat("f", 64), PACoverageNotRequired, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WithUsage(record, 2, 1, 8000, 1000); err == nil {
		t.Fatal("inflated D0 budget accepted")
	}
	if _, err := WithUsage(record, 1, 1, 4000, 1000); err != nil {
		t.Fatalf("exact D0 usage rejected: %v", err)
	}
}
