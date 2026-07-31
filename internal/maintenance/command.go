package maintenance

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
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
	ReceiptAccepted        ReceiptState = "accepted"
	ReceiptBusy            ReceiptState = "busy"
	ReceiptSucceeded       ReceiptState = "succeeded"
	ReceiptFailed          ReceiptState = "failed"
	ReceiptUnavailable     ReceiptState = "unavailable"
	ReceiptTimedOut        ReceiptState = "timed_out"
	ReceiptProposalEmitted ReceiptState = "proposal_emitted"
)

type Receipt struct {
	SchemaVersion  int          `json:"schema_version"`
	CommandID      string       `json:"command_id"`
	JobID          string       `json:"job_id"`
	WorkspaceID    string       `json:"workspace_id"`
	Trigger        Trigger      `json:"trigger"`
	State          ReceiptState `json:"state"`
	RecordedAt     time.Time    `json:"recorded_at"`
	Deadline       time.Time    `json:"deadline"`
	ProposalOnly   bool         `json:"proposal_only"`
	ProposalCount  int          `json:"proposal_count,omitempty"`
	ProposalDigest string       `json:"proposal_digest,omitempty"`
	Diagnostic     string       `json:"diagnostic,omitempty"`
}

var (
	commandIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)
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
	if command.RequestedAt.After(now.Add(time.Second)) || command.Deadline.Before(command.RequestedAt) || command.Deadline.Sub(command.RequestedAt) > 15*time.Minute || command.Deadline.Sub(now) > 15*time.Minute || !command.Deadline.After(now) {
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
	if receipt.SchemaVersion != CommandSchemaVersion || !commandIDPattern.MatchString(receipt.CommandID) || !commandIDPattern.MatchString(receipt.JobID) || !commandIDPattern.MatchString(receipt.WorkspaceID) || !validTrigger(receipt.Trigger) || !validReceiptState(receipt.State) || receipt.RecordedAt.IsZero() || receipt.Deadline.IsZero() {
		return errors.New("maintenance receipt is invalid")
	}
	if len([]byte(receipt.Diagnostic)) > 256 || strings.ContainsAny(receipt.Diagnostic, "\r\n") {
		return errors.New("maintenance receipt diagnostic is not bounded")
	}
	if receipt.ProposalOnly != (receipt.JobID == "darwin-structural-evolution-proposal") {
		return errors.New("maintenance receipt proposal flag does not match the job")
	}
	if receipt.ProposalOnly && receipt.State == ReceiptSucceeded {
		return errors.New("proposal-only maintenance cannot report an applied success")
	}
	if receipt.State == ReceiptProposalEmitted && receipt.ProposalCount < 0 {
		return errors.New("maintenance proposal count is invalid")
	}
	if receipt.ProposalDigest != "" && !digestPattern.MatchString(receipt.ProposalDigest) {
		return errors.New("maintenance proposal digest is invalid")
	}
	if receipt.State == ReceiptProposalEmitted && receipt.ProposalCount > 0 && receipt.ProposalDigest == "" {
		return errors.New("maintenance proposal receipt requires a digest")
	}
	return nil
}

type GateDecision struct {
	State       string   `json:"state"`
	Reason      string   `json:"reason,omitempty"`
	EventID     string   `json:"event_id,omitempty"`
	PlannedJobs []string `json:"planned_jobs,omitempty"`
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
	case ReceiptAccepted, ReceiptBusy, ReceiptSucceeded, ReceiptFailed, ReceiptUnavailable, ReceiptTimedOut, ReceiptProposalEmitted:
		return true
	default:
		return false
	}
}
