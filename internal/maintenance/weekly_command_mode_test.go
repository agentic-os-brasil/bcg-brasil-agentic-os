package maintenance

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestWeeklyClosedCommandModePreservesLegacyAndRequiresPrivateBinding(t *testing.T) {
	now := time.Now().UTC()
	command := Command{SchemaVersion: 1, CommandID: "synthetic-weekly-mode", JobID: YodaSelfReviewWeeklyJobID, WorkspaceID: "owner-synthetic", Trigger: TriggerWeekly, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute), ProposalOnly: true}
	body, _ := json.Marshal(command)
	if strings.Contains(string(body), "weekly_mode") {
		t.Fatal("legacy command changed")
	}
	if err := command.Validate(now); err != nil {
		t.Fatal(err)
	}
	command.WeeklyMode = WeeklyWorking
	command.ProposalOnly = false
	if err := command.Validate(now); err != nil {
		t.Fatal("closed working command denied", err)
	}
	grant, err := newExecutionGrant(command)
	if err != nil {
		t.Fatal(err)
	}
	if ValidateExecutionGrant(grant, command) == nil {
		t.Fatal("mode text without private weekly binding accepted")
	}
	bound := grant.(executionGrant)
	bound.weekly = &weeklyModelBinding{Mode: WeeklyWorking}
	if err := ValidateExecutionGrant(bound, command); err != nil {
		t.Fatal(err)
	}
	command.JobID = MemoryCaptureJobID
	if command.Validate(now) == nil {
		t.Fatal("weekly mode on another job accepted")
	}
}
func TestWeeklyClosedSucceededReceiptMode(t *testing.T) {
	now := time.Now().UTC()
	r := Receipt{SchemaVersion: 1, AttemptID: strings.Repeat("a", 32), OccurrenceDigest: strings.Repeat("b", 64), CommandID: "synthetic-receipt", JobID: YodaSelfReviewWeeklyJobID, WorkspaceID: "owner-synthetic", Trigger: TriggerWeekly, State: ReceiptSucceeded, RecordedAt: now, Deadline: now.Add(time.Minute), ReasonCode: ReasonCompleted, WeeklyMode: WeeklyWorking}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	r.WeeklyMode = ""
	if r.Validate() == nil {
		t.Fatal("legacy proposal-only job falsely succeeded")
	}
}
