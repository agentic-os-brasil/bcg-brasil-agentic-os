package maintenance

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const CommandSchemaVersion = 1

type Trigger string

const (
	TriggerContinuous Trigger = "continuous"
	TriggerEvent      Trigger = "event"
	TriggerPresence   Trigger = "presence"
	TriggerDaily      Trigger = "daily"
	TriggerWeekly     Trigger = "weekly"
	TriggerMonthly    Trigger = "monthly"
)

type Command struct {
	SchemaVersion int       `json:"schema_version"`
	CommandID     string    `json:"command_id"`
	JobID         string    `json:"job_id"`
	WorkspaceID   string    `json:"workspace_id"`
	Trigger       Trigger   `json:"trigger"`
	EventID       string    `json:"event_id,omitempty"`
	ScheduledFor  time.Time `json:"scheduled_for"`
	RequestedAt   time.Time `json:"requested_at"`
	Deadline      time.Time `json:"deadline"`
	ProposalOnly  bool      `json:"proposal_only"`
}

type ReceiptState string

const (
	ReceiptAccepted         ReceiptState = "accepted"
	ReceiptBusy             ReceiptState = "busy"
	ReceiptSucceeded        ReceiptState = "succeeded"
	ReceiptReviewedNoChange ReceiptState = "reviewed_no_change"
	ReceiptRecoveryRequired ReceiptState = "recovery_required"
	ReceiptFailed           ReceiptState = "failed"
	ReceiptUnavailable      ReceiptState = "unavailable"
	ReceiptTimedOut         ReceiptState = "timed_out"
	ReceiptProposalEmitted  ReceiptState = "proposal_emitted"
)

type Receipt struct {
	SchemaVersion        int          `json:"schema_version"`
	AttemptID            string       `json:"attempt_id"`
	OccurrenceDigest     string       `json:"occurrence_digest"`
	CommandID            string       `json:"command_id"`
	JobID                string       `json:"job_id"`
	WorkspaceID          string       `json:"workspace_id"`
	Trigger              Trigger      `json:"trigger"`
	State                ReceiptState `json:"state"`
	RecordedAt           time.Time    `json:"recorded_at"`
	Deadline             time.Time    `json:"deadline"`
	ProposalOnly         bool         `json:"proposal_only"`
	ProposalCount        int          `json:"proposal_count,omitempty"`
	ProposalDigest       string       `json:"proposal_digest,omitempty"`
	ProposalArtifactID   string       `json:"proposal_artifact_id,omitempty"`
	RecoveryPhase        string       `json:"recovery_phase,omitempty"`
	RecoveryIntentDigest string       `json:"recovery_intent_digest,omitempty"`
	FenceTokenDigest     string       `json:"fence_token_digest,omitempty"`
	ReasonCode           ReasonCode   `json:"reason_code"`
}

var (
	commandIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)
	attemptIDPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)
	digestPattern    = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

func (command Command) Validate(now time.Time) error {
	if command.SchemaVersion != CommandSchemaVersion || !commandIDPattern.MatchString(command.CommandID) || !commandIDPattern.MatchString(command.JobID) || !commandIDPattern.MatchString(command.WorkspaceID) {
		return errors.New("maintenance command header is invalid")
	}
	if !validTrigger(command.Trigger) || command.RequestedAt.IsZero() || command.ScheduledFor.IsZero() || command.Deadline.IsZero() {
		return errors.New("maintenance command timing or trigger is invalid")
	}
	if now.IsZero() {
		now = command.RequestedAt
	}
	if command.RequestedAt.After(now.Add(time.Second)) || command.ScheduledFor.After(now.Add(time.Second)) || command.Deadline.Before(command.RequestedAt) || command.Deadline.Sub(command.RequestedAt) > 15*time.Minute || command.Deadline.Sub(now) > 15*time.Minute || !command.Deadline.After(now) {
		return errors.New("maintenance command deadline is missing, expired or unbounded")
	}
	if (command.Trigger == TriggerEvent || command.Trigger == TriggerContinuous) && !commandIDPattern.MatchString(command.EventID) {
		return errors.New("event maintenance command requires a bounded event ID")
	}
	if command.ProposalOnly != (command.JobID == "darwin-structural-evolution-proposal") {
		return errors.New("proposal-only flag does not match the maintenance job")
	}
	return nil
}

func (receipt Receipt) Validate() error {
	if receipt.SchemaVersion != CommandSchemaVersion || !attemptIDPattern.MatchString(receipt.AttemptID) || !digestPattern.MatchString(receipt.OccurrenceDigest) || !commandIDPattern.MatchString(receipt.CommandID) || !commandIDPattern.MatchString(receipt.JobID) || !commandIDPattern.MatchString(receipt.WorkspaceID) || !validTrigger(receipt.Trigger) || !validReceiptState(receipt.State) || receipt.RecordedAt.IsZero() || receipt.Deadline.IsZero() {
		return errors.New("maintenance receipt is invalid")
	}
	if !validReasonCode(receipt.ReasonCode) {
		return errors.New("maintenance receipt reason code is not allowlisted")
	}
	if receipt.ProposalOnly != (receipt.JobID == "darwin-structural-evolution-proposal") {
		return errors.New("maintenance receipt proposal flag does not match the job")
	}
	if receipt.ProposalOnly && receipt.State == ReceiptSucceeded {
		return errors.New("proposal-only maintenance cannot report an applied success")
	}
	if receipt.State == ReceiptProposalEmitted && receipt.ProposalCount < 1 {
		return errors.New("proposal-emitted maintenance receipt requires at least one proposal")
	}
	if receipt.ProposalDigest != "" && !digestPattern.MatchString(receipt.ProposalDigest) {
		return errors.New("maintenance proposal digest is invalid")
	}
	if receipt.State == ReceiptProposalEmitted && receipt.ProposalDigest == "" {
		return errors.New("maintenance proposal receipt requires a digest")
	}
	if receipt.ProposalArtifactID != "" && !digestPattern.MatchString(receipt.ProposalArtifactID) {
		return errors.New("maintenance proposal artifact ID is invalid")
	}
	if receipt.State == ReceiptProposalEmitted && receipt.ProposalArtifactID == "" {
		return errors.New("maintenance proposal receipt requires a durable artifact ID")
	}
	if receipt.State != ReceiptProposalEmitted && (receipt.ProposalCount != 0 || receipt.ProposalDigest != "") {
		return errors.New("non-proposal maintenance receipt cannot carry proposal evidence")
	}
	if receipt.State != ReceiptProposalEmitted && receipt.ProposalArtifactID != "" {
		return errors.New("non-proposal maintenance receipt cannot carry a proposal artifact ID")
	}
	if receipt.RecoveryPhase != "" && receipt.RecoveryPhase != "intent" && receipt.RecoveryPhase != "completed" && receipt.RecoveryPhase != "failed" && receipt.RecoveryPhase != "audit_incomplete" {
		return errors.New("maintenance recovery phase is invalid")
	}
	if receipt.RecoveryPhase == "intent" {
		if receipt.RecoveryIntentDigest == "" || receipt.FenceTokenDigest == "" || receipt.ReasonCode != ReasonRecoveryIntent {
			return errors.New("maintenance recovery intent is incomplete")
		}
	}
	if receipt.RecoveryPhase == "completed" || receipt.RecoveryPhase == "failed" || receipt.RecoveryPhase == "audit_incomplete" {
		if receipt.RecoveryIntentDigest == "" || receipt.FenceTokenDigest == "" {
			return errors.New("maintenance recovery outcome is incomplete")
		}
		wantReason := map[string]ReasonCode{"completed": ReasonRecoveryCompleted, "failed": ReasonRecoveryFailed, "audit_incomplete": ReasonRecoveryAuditIncomplete}[receipt.RecoveryPhase]
		if receipt.ReasonCode != wantReason {
			return errors.New("maintenance recovery outcome reason is invalid")
		}
	}
	if receipt.RecoveryPhase == "" && (receipt.RecoveryIntentDigest != "" || receipt.FenceTokenDigest != "") {
		return errors.New("maintenance recovery binding requires a phase")
	}
	return nil
}

// OccurrenceKey identifies the work occurrence rather than a caller's command
// attempt. Retries for the same scheduled/event work therefore contend on the
// same lease even when they use different command IDs.
func (command Command) OccurrenceKey() string {
	return occurrenceKey(command.JobID, command.Trigger, command.EventID, command.ScheduledFor)
}

func (command Command) OccurrenceDigest() string {
	digest := sha256.Sum256([]byte(command.OccurrenceKey()))
	return hex.EncodeToString(digest[:])
}

type GateDecision struct {
	State       string   `json:"state"`
	Reason      string   `json:"reason,omitempty"`
	EventID     string   `json:"event_id,omitempty"`
	PlannedJobs []string `json:"planned_jobs,omitempty"`
}

// NewRecoveryReceipt records an operator-attested quarantine recovery without
// claiming scheduler success. The reason is an allowlisted code, never free
// text from a shell or process.
func NewRecoveryReceipt(workspaceID, jobID string, trigger Trigger, scheduledFor, now time.Time) (Receipt, error) {
	attempt, err := attemptID()
	if err != nil {
		return Receipt{}, err
	}
	return Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: attempt, OccurrenceDigest: digest(scheduler.ScheduledOccurrenceKey(jobID, scheduledFor)), CommandID: "recovery-" + digestPrefix(jobID+scheduledFor.UTC().Format(time.RFC3339Nano)), JobID: jobID, WorkspaceID: workspaceID, Trigger: trigger, State: ReceiptUnavailable, RecordedAt: now.UTC(), Deadline: now.UTC(), ProposalOnly: jobID == "darwin-structural-evolution-proposal", ReasonCode: ReasonReceiptPersisted}, nil
}

func NewRecoveryIntentReceipt(workspaceID, jobID string, trigger Trigger, scheduledFor, now time.Time, fenceToken string) (Receipt, error) {
	attempt, err := attemptID()
	if err != nil {
		return Receipt{}, err
	}
	occurrence := digest(scheduler.ScheduledOccurrenceKey(jobID, scheduledFor))
	fenceDigest := digest(fenceToken)
	intentDigest := digest(workspaceID + "\x00" + jobID + "\x00" + occurrence + "\x00" + fenceDigest)
	return Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: attempt, OccurrenceDigest: occurrence, CommandID: "recovery-intent-" + digestPrefix(intentDigest), JobID: jobID, WorkspaceID: workspaceID, Trigger: trigger, State: ReceiptUnavailable, RecordedAt: now.UTC(), Deadline: now.UTC(), ProposalOnly: jobID == "darwin-structural-evolution-proposal", RecoveryPhase: "intent", RecoveryIntentDigest: intentDigest, FenceTokenDigest: fenceDigest, ReasonCode: ReasonRecoveryIntent}, nil
}

func NewRecoveryOutcomeReceipt(intent Receipt, now time.Time, phase string, reason ReasonCode) (Receipt, error) {
	if intent.RecoveryPhase != "intent" || intent.RecoveryIntentDigest == "" || intent.FenceTokenDigest == "" || (phase != "completed" && phase != "failed" && phase != "audit_incomplete") {
		return Receipt{}, errors.New("invalid maintenance recovery intent")
	}
	attempt, err := attemptID()
	if err != nil {
		return Receipt{}, err
	}
	return Receipt{SchemaVersion: CommandSchemaVersion, AttemptID: attempt, OccurrenceDigest: intent.OccurrenceDigest, CommandID: "recovery-" + phase + "-" + digestPrefix(intent.RecoveryIntentDigest), JobID: intent.JobID, WorkspaceID: intent.WorkspaceID, Trigger: intent.Trigger, State: ReceiptUnavailable, RecordedAt: now.UTC(), Deadline: now.UTC(), ProposalOnly: intent.ProposalOnly, RecoveryPhase: phase, RecoveryIntentDigest: intent.RecoveryIntentDigest, FenceTokenDigest: intent.FenceTokenDigest, ReasonCode: reason}, nil
}

func Gate(catalog Catalog, command Command, now time.Time) (GateDecision, error) {
	if err := command.Validate(now); err != nil {
		return GateDecision{}, err
	}
	trigger := command.Trigger
	if trigger == TriggerContinuous {
		trigger = TriggerEvent
	}
	jobs, err := catalog.ForTrigger(string(trigger))
	if err != nil {
		return GateDecision{}, err
	}
	for _, job := range jobs {
		if job.ID == command.JobID {
			if job.Availability != Available || !job.DefaultEnabled || job.Unattended == "never" {
				return GateDecision{State: "unavailable", Reason: fmt.Sprintf("job %q is not qualified and enabled for worker execution", command.JobID), EventID: command.EventID}, nil
			}
			return GateDecision{State: "accepted", EventID: command.EventID, PlannedJobs: []string{job.ID}}, nil
		}
	}
	return GateDecision{State: "unavailable", Reason: fmt.Sprintf("job %q is not eligible for trigger %q", command.JobID, command.Trigger), EventID: command.EventID}, nil
}

func validTrigger(trigger Trigger) bool {
	switch trigger {
	case TriggerContinuous, TriggerEvent, TriggerPresence, TriggerDaily, TriggerWeekly, TriggerMonthly:
		return true
	default:
		return false
	}
}

func validReceiptState(state ReceiptState) bool {
	switch state {
	case ReceiptAccepted, ReceiptBusy, ReceiptSucceeded, ReceiptReviewedNoChange, ReceiptRecoveryRequired, ReceiptFailed, ReceiptUnavailable, ReceiptTimedOut, ReceiptProposalEmitted:
		return true
	default:
		return false
	}
}
