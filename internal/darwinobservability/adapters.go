package darwinobservability

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// FromDarwinReceipt projects the existing local Darwin receipt into a closed
// health record. Action details and any runtime-specific content are omitted.
func FromDarwinReceipt(receipt darwin.Receipt) (Record, error) {
	if receipt.AgentID != darwin.AgentID || receipt.WindowID == "" || receipt.RecordedAt.IsZero() {
		return Record{}, errors.New("invalid Darwin receipt for observability")
	}
	freshness, recovery, outcome := FreshnessCurrent, RecoveryNotNeeded, OutcomeSucceeded
	switch receipt.Outcome {
	case darwin.OutcomeSucceeded, darwin.OutcomeNoAction:
	case darwin.OutcomePartial:
		freshness, recovery, outcome = FreshnessAging, RecoveryRecovered, OutcomePartial
	case darwin.OutcomeFailed:
		freshness, recovery, outcome = FreshnessStale, RecoveryFailed, OutcomeFailed
	case darwin.OutcomeBlocked:
		freshness, recovery, outcome = FreshnessUnavailable, RecoveryBlocked, OutcomeBlocked
	default:
		return Record{}, errors.New("unknown Darwin receipt outcome")
	}
	record := Record{
		SchemaVersion: SchemaVersion, Kind: KindHealth,
		EvidenceID: EvidenceID(KindHealth, receipt.WindowID, mustJSON(receipt.RecordedAt.UTC())),
		WindowID:   receipt.WindowID, RecordedAt: receipt.RecordedAt.UTC(),
		Health: &HealthEvidence{
			JobKind: JobDarwinHousekeeping, ScheduledAt: receipt.RecordedAt.UTC(), CapturedAt: receipt.RecordedAt.UTC(),
			Freshness: freshness, Recovery: recovery, Outcome: outcome,
		},
	}
	return record, record.Validate()
}

// FromSchedulerReceipt deliberately ignores WorkspaceID and Error. The
// scheduler's local diagnostic detail never crosses this boundary.
func FromSchedulerReceipt(receipt scheduler.Receipt, windowID string) (Record, error) {
	if windowID == "" || receipt.JobID == "" || receipt.ScheduledFor.IsZero() || receipt.AttemptedAt.IsZero() {
		return Record{}, errors.New("invalid scheduler receipt for observability")
	}
	jobKind := JobKind(receipt.JobID)
	switch receipt.JobID {
	case "darwin-housekeeping":
		jobKind = JobDarwinHousekeeping
	case "federation-weekly":
		jobKind = JobFederationWeekly
	case "activation-monitor":
		jobKind = JobActivationMonitor
	default:
		return Record{}, errors.New("scheduler job is not observable")
	}
	freshness, recovery, outcome := FreshnessCurrent, RecoveryNotNeeded, OutcomeSucceeded
	switch receipt.State {
	case scheduler.Succeeded:
	case scheduler.Failed:
		freshness, recovery, outcome = FreshnessStale, RecoveryFailed, OutcomeFailed
	case scheduler.Unavailable:
		freshness, recovery, outcome = FreshnessUnavailable, RecoveryBlocked, OutcomeUnavailable
	default:
		return Record{}, errors.New("unknown scheduler receipt state")
	}
	value, _ := json.Marshal([]any{receipt.JobID, receipt.ScheduledFor.UTC(), receipt.AttemptedAt.UTC(), receipt.State})
	record := Record{
		SchemaVersion: SchemaVersion, Kind: KindHealth,
		EvidenceID: EvidenceID(KindHealth, windowID, value), WindowID: windowID,
		RecordedAt: receipt.AttemptedAt.UTC(),
		Health:     &HealthEvidence{JobKind: jobKind, ScheduledAt: receipt.ScheduledFor.UTC(), CapturedAt: receipt.AttemptedAt.UTC(), Freshness: freshness, Recovery: recovery, Outcome: outcome},
	}
	return record, record.Validate()
}

// FromActivationObservation adapts the existing closed activation monitor
// without duplicating route, posture or policy definitions.
func FromActivationObservation(observation activationpolicy.Observation, recordedAt time.Time, paCoverage PACoverage, paExpertCount int, gaps []CapabilityGap) (Record, error) {
	if recordedAt.IsZero() {
		return Record{}, errors.New("recorded time is required")
	}
	outcome := OutcomeFailed
	switch observation.Outcome {
	case activationpolicy.CompletedOutcome:
		outcome = OutcomeSucceeded
	case activationpolicy.FailedOutcome:
		outcome = OutcomeFailed
	case activationpolicy.BlockedOutcome:
		outcome = OutcomeBlocked
	default:
		return Record{}, errors.New("activation outcome is not observable")
	}
	value, _ := json.Marshal(observation)
	record := Record{
		SchemaVersion: SchemaVersion, Kind: KindSelection,
		EvidenceID: EvidenceID(KindSelection, observation.WindowID, value), WindowID: observation.WindowID, RecordedAt: recordedAt.UTC(),
		Selection: &SelectionEvidence{
			PlanSHA256: observation.PlanSHA256, PolicyVersion: observation.PolicyVersion, Posture: observation.Posture, Route: observation.Route,
			Outcome: outcome, DurationSeconds: observation.DurationSeconds, BudgetExhausted: observation.BudgetExhausted,
			HumanOverride: observation.HumanOverride, OverrideKind: overrideFor(observation.HumanOverride),
			PACoverage: paCoverage, PAExpertCount: paExpertCount, CapabilityGaps: append([]CapabilityGap(nil), gaps...),
		},
	}
	// The activation observation predates usage counters. Preserve the planned
	// fields as zero only when the route is blocked; callers with runtime
	// counters should use WithUsage below.
	switch record.Selection.Route {
	case activationpolicy.D0Direct:
		record.Selection.MaxCalls, record.Selection.MaxTokenUnits = 1, 4000
	case activationpolicy.D1Targeted:
		record.Selection.MaxCalls, record.Selection.MaxTokenUnits = 3, 10000
	case activationpolicy.D2Governed:
		record.Selection.MaxCalls, record.Selection.MaxTokenUnits = 6, 24000
	}
	return record, record.Validate()
}

func WithUsage(record Record, maxCalls, callsUsed, maxTokenUnits, tokenUnitsUsed int) (Record, error) {
	if record.Kind != KindSelection || record.Selection == nil {
		return Record{}, errors.New("usage requires selection evidence")
	}
	record.Selection.MaxCalls, record.Selection.CallsUsed = maxCalls, callsUsed
	record.Selection.MaxTokenUnits, record.Selection.TokenUnitsUsed = maxTokenUnits, tokenUnitsUsed
	return record, record.Validate()
}

func overrideFor(overridden bool) OverrideKind {
	if overridden {
		return OverrideRoute
	}
	return OverrideNone
}

func mustJSON(value any) []byte { body, _ := json.Marshal(value); return body }
