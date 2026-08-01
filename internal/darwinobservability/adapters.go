package darwinobservability

import (
	"errors"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/darwin"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maestroflow"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const (
	currentLateness = 5 * time.Minute
	agingLateness   = time.Hour
)

// FromDarwinReceipt projects the existing local Darwin receipt into a closed
// health record. Action details and any runtime-specific content are omitted.
// scheduledAt is explicit because a Darwin receipt does not carry its cadence
// occurrence; inventing it from RecordedAt would make freshness meaningless.
func FromDarwinReceipt(receipt darwin.Receipt, scheduledAt time.Time, scopeSHA256 string) (Record, error) {
	if receipt.AgentID != darwin.AgentID || receipt.WindowID == "" || receipt.RecordedAt.IsZero() ||
		scheduledAt.IsZero() || !digestPattern.MatchString(scopeSHA256) {
		return Record{}, errors.New("invalid Darwin receipt for observability")
	}
	outcome := OutcomeSucceeded
	switch receipt.Outcome {
	case darwin.OutcomeSucceeded:
	case darwin.OutcomeNoAction:
		outcome = OutcomeNoAction
	case darwin.OutcomePartial:
		outcome = OutcomePartial
	case darwin.OutcomeFailed:
		outcome = OutcomeFailed
	case darwin.OutcomeBlocked:
		outcome = OutcomeBlocked
	default:
		return Record{}, errors.New("unknown Darwin receipt outcome")
	}
	freshness := freshnessFor(scheduledAt, receipt.RecordedAt)
	recovery := recoveryFor(freshness, outcome)
	windowID := OpaqueWindowID(receipt.WindowID)
	record := Record{
		SchemaVersion: SchemaVersion, Kind: KindHealth,
		WindowID: windowID, ScopeSHA256: scopeSHA256, Authority: AuthorityCallerAssertedShadow,
		RecordedAt: receipt.RecordedAt.UTC(),
		Health: &HealthEvidence{
			JobKind: JobDarwinHousekeeping, ScheduledAt: scheduledAt.UTC(), CapturedAt: receipt.RecordedAt.UTC(),
			Freshness: freshness, Recovery: recovery, Outcome: outcome,
		},
	}
	assignEvidenceID(&record)
	return record, record.Validate()
}

// FromSchedulerReceipt deliberately ignores WorkspaceID and Error. The
// scheduler's local diagnostic detail never crosses this boundary.
func FromSchedulerReceipt(receipt scheduler.Receipt, localWindowID, scopeSHA256 string) (Record, error) {
	if localWindowID == "" || !digestPattern.MatchString(scopeSHA256) || receipt.JobID == "" ||
		receipt.ScheduledFor.IsZero() || receipt.AttemptedAt.IsZero() {
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
	outcome := OutcomeSucceeded
	switch receipt.State {
	case scheduler.Succeeded:
	case scheduler.Failed:
		outcome = OutcomeFailed
	case scheduler.Unavailable:
		outcome = OutcomeUnavailable
	default:
		return Record{}, errors.New("unknown scheduler receipt state")
	}
	freshness := freshnessFor(receipt.ScheduledFor, receipt.AttemptedAt)
	recovery := recoveryFor(freshness, outcome)
	windowID := OpaqueWindowID(localWindowID)
	record := Record{
		SchemaVersion: SchemaVersion, Kind: KindHealth,
		WindowID: windowID, ScopeSHA256: scopeSHA256, Authority: AuthorityCallerAssertedShadow,
		RecordedAt: receipt.AttemptedAt.UTC(),
		Health:     &HealthEvidence{JobKind: jobKind, ScheduledAt: receipt.ScheduledFor.UTC(), CapturedAt: receipt.AttemptedAt.UTC(), Freshness: freshness, Recovery: recovery, Outcome: outcome},
	}
	assignEvidenceID(&record)
	return record, record.Validate()
}

// FromMaestroFlowReceipt projects one canonical Maestro attempt into
// metadata-only Darwin evidence. It never accepts client/workspace content.
func FromMaestroFlowReceipt(receipt maestroflow.Receipt, windowID, scopeSHA256 string) (Record, error) {
	if err := receipt.Validate(); err != nil || !windowPattern.MatchString(windowID) || !digestPattern.MatchString(scopeSHA256) {
		return Record{}, errors.New("invalid Maestro flow receipt for observability")
	}
	evidence := &FlowEvidence{
		AttemptID: receipt.AttemptID, AttemptSHA256: receipt.AttemptSHA256, EntryPath: "",
		PreAccountUsed: receipt.PreAccountUsed, AccountConsultationRequired: receipt.AccountConsultationRequired, PostAccountValidationRequired: receipt.PostAccountValidationRequired,
		AccountSignals:       make([]string, 0, len(receipt.AccountSignals)),
		PostAccountValidated: receipt.PostAccountValidated, WalterRequired: receipt.WalterRequired,
		WalterGate: receipt.WalterGate, WalterSkipped: receipt.WalterSkipped,
		WalterSkipReason: receipt.WalterSkipReason, WalterSkipEvidenceSHA256: receipt.WalterSkipEvidenceSHA256,
		RefinementLoadBearing: receipt.RefinementLoadBearing, RefinementActionable: receipt.RefinementActionable, RefinementKind: receipt.RefinementKind,
		AccountVerdict: string(receipt.AccountVerdict), WalterVerdict: string(receipt.WalterVerdict),
		Cycles: receipt.Cycle, BudgetExhausted: receipt.BudgetExhausted,
		MaterialDelivered: receipt.Event == "material_delivered",
	}
	for _, signal := range receipt.AccountSignals {
		evidence.AccountSignals = append(evidence.AccountSignals, string(signal))
	}
	if receipt.PreAccountUsed {
		evidence.EntryPath = "account_first"
	} else {
		evidence.EntryPath = "case_direct"
	}
	if receipt.Event == "account_refine" || receipt.Event == "walter_refine" || receipt.Invalidated {
		evidence.InvalidationsAfterMutation = 1
	}
	record := Record{SchemaVersion: SchemaVersion, Kind: KindFlow, WindowID: windowID, ScopeSHA256: scopeSHA256, Authority: AuthorityCallerAssertedShadow, RecordedAt: receipt.At.UTC(), Flow: evidence}
	assignEvidenceID(&record)
	return record, record.Validate()
}

// FromDarwinSelfMaintenanceReceipt projects owner-context self maintenance
// without allowing claim or canonical-self content across the observability
// boundary.
func FromDarwinSelfMaintenanceReceipt(receipt darwin.SelfMaintenanceReceipt, windowID, scopeSHA256 string) (Record, error) {
	if err := receipt.Validate(); err != nil || !windowPattern.MatchString(windowID) || !digestPattern.MatchString(scopeSHA256) {
		return Record{}, errors.New("invalid Darwin self maintenance receipt for observability")
	}
	record := Record{
		SchemaVersion: SchemaVersion, Kind: KindSelf, WindowID: windowID, ScopeSHA256: scopeSHA256,
		Authority: AuthorityCallerAssertedShadow, RecordedAt: receipt.RecordedAt.UTC(),
		Self: &SelfEvidence{
			SnapshotVersion: receipt.SnapshotVersion, SnapshotSHA256: receipt.SnapshotSHA256,
			ObservationCount: receipt.ObservationCount, DuplicateCount: receipt.DuplicateCount,
			ContradictionCount: receipt.ContradictionCount, RecheckDue: receipt.RecheckDue,
			DecayCandidates: receipt.DecayCandidates, OwnerConfirmedSignals: receipt.OwnerConfirmedSignals,
			ReevaluationProposals: receipt.ReevaluationProposals, CanonicalMutations: receipt.CanonicalMutations,
		},
	}
	assignEvidenceID(&record)
	return record, record.Validate()
}

// FromActivationObservation adapts the existing closed activation monitor
// without duplicating route, posture or policy definitions.
func FromActivationObservation(observation activationpolicy.Observation, recordedAt time.Time, scopeSHA256 string, paCoverage PACoverage, paExpertCount int, gaps []CapabilityGap) (Record, error) {
	if observation.WindowID == "" || recordedAt.IsZero() || !digestPattern.MatchString(scopeSHA256) {
		return Record{}, errors.New("window, recorded time and opaque scope are required")
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
	gaps = append([]CapabilityGap(nil), gaps...)
	if observation.MissingReceipt {
		outcome = OutcomePartial
		if !containsGap(gaps, GapReceiptCoverage) {
			gaps = append(gaps, GapReceiptCoverage)
		}
	}
	record := Record{
		SchemaVersion: SchemaVersion, Kind: KindSelection,
		WindowID: OpaqueWindowID(observation.WindowID), ScopeSHA256: scopeSHA256, Authority: AuthorityCallerAssertedShadow,
		RecordedAt: recordedAt.UTC(),
		Selection: &SelectionEvidence{
			PlanSHA256: observation.PlanSHA256, PolicyVersion: observation.PolicyVersion, Posture: observation.Posture, Route: observation.Route,
			Outcome: outcome, DurationSeconds: observation.DurationSeconds, BudgetExhausted: observation.BudgetExhausted,
			HumanOverride: observation.HumanOverride, OverrideKind: overrideFor(observation.HumanOverride),
			PACoverage: paCoverage, PAExpertCount: paExpertCount, CapabilityGaps: gaps,
		},
	}
	// The activation observation predates usage counters. Preserve the planned
	// fields as zero only when the route is blocked; callers with runtime
	// counters should use WithUsage below.
	record.Selection.MaxCalls, record.Selection.MaxTokenUnits = routeBudget(record.Selection.Route)
	assignEvidenceID(&record)
	return record, record.Validate()
}

func WithUsage(record Record, maxCalls, callsUsed, maxTokenUnits, tokenUnitsUsed int) (Record, error) {
	if record.Kind != KindSelection || record.Selection == nil {
		return Record{}, errors.New("usage requires selection evidence")
	}
	expectedCalls, expectedTokenUnits := routeBudget(record.Selection.Route)
	if maxCalls != expectedCalls || maxTokenUnits != expectedTokenUnits {
		return Record{}, errors.New("usage budget must match the route policy")
	}
	record.Selection.MaxCalls, record.Selection.CallsUsed = maxCalls, callsUsed
	record.Selection.MaxTokenUnits, record.Selection.TokenUnitsUsed = maxTokenUnits, tokenUnitsUsed
	assignEvidenceID(&record)
	return record, record.Validate()
}

func overrideFor(overridden bool) OverrideKind {
	if overridden {
		return OverrideRoute
	}
	return OverrideNone
}

func freshnessFor(scheduledAt, capturedAt time.Time) Freshness {
	delay := capturedAt.Sub(scheduledAt)
	switch {
	case delay <= currentLateness:
		return FreshnessCurrent
	case delay <= agingLateness:
		return FreshnessAging
	default:
		return FreshnessMissed
	}
}

func recoveryFor(freshness Freshness, outcome Outcome) Recovery {
	switch outcome {
	case OutcomeFailed, OutcomePartial:
		return RecoveryFailed
	case OutcomeBlocked, OutcomeUnavailable:
		return RecoveryBlocked
	case OutcomeSucceeded, OutcomeNoAction:
		if freshness == FreshnessMissed {
			return RecoveryRecovered
		}
		return RecoveryNotNeeded
	default:
		return RecoveryFailed
	}
}

func containsGap(gaps []CapabilityGap, target CapabilityGap) bool {
	for _, gap := range gaps {
		if gap == target {
			return true
		}
	}
	return false
}

func assignEvidenceID(record *Record) {
	record.EvidenceID = canonicalEvidenceID(*record)
}
