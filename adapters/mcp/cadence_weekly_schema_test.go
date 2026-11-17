package product_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/google/jsonschema-go/jsonschema"
)

func TestWeeklyPublishedSchemasMatchCommandReceiptModes(t *testing.T) {
	schemas := map[string]func(any) error{}
	for _, name := range []string{"maintenance-command", "maintenance-receipt"} {
		body, err := os.ReadFile(filepath.Join("..", "..", "schemas", name+".schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		var schema jsonschema.Schema
		if err := json.Unmarshal(body, &schema); err != nil {
			t.Fatal(err)
		}
		resolved, err := schema.Resolve(nil)
		if err != nil {
			t.Fatal(err)
		}
		schemas[name] = resolved.Validate
	}
	now := time.Date(2026, 11, 2, 10, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, job string
		mode      maintenance.WeeklyModelMode
		trigger   maintenance.Trigger
		proposal  bool
		valid     bool
	}{
		{"working", maintenance.YodaSelfReviewWeeklyJobID, maintenance.WeeklyWorking, maintenance.TriggerWeekly, false, true},
		{"native-probe", maintenance.YodaSelfReviewWeeklyJobID, maintenance.WeeklyNativeProbe, maintenance.TriggerWeekly, false, true},
		{"legacy-weekly", maintenance.YodaSelfReviewWeeklyJobID, "", maintenance.TriggerWeekly, true, true},
		{"legacy-monthly", maintenance.DarwinStructuralProposalJobID, "", maintenance.TriggerMonthly, true, true},
		{"legacy-ordinary", maintenance.MemoryLightDreamJobID, "", maintenance.TriggerDaily, false, true},
		{"working-proposal-denied", maintenance.YodaSelfReviewWeeklyJobID, maintenance.WeeklyWorking, maintenance.TriggerWeekly, true, false},
		{"probe-proposal-denied", maintenance.YodaSelfReviewWeeklyJobID, maintenance.WeeklyNativeProbe, maintenance.TriggerWeekly, true, false},
		{"missing-mode-denied", maintenance.YodaSelfReviewWeeklyJobID, "", maintenance.TriggerWeekly, false, false},
		{"unknown-mode-denied", maintenance.YodaSelfReviewWeeklyJobID, "unknown", maintenance.TriggerWeekly, false, false},
		{"other-job-mode-denied", maintenance.MemoryLightDreamJobID, maintenance.WeeklyWorking, maintenance.TriggerDaily, false, false},
		{"wrong-trigger-denied", maintenance.YodaSelfReviewWeeklyJobID, maintenance.WeeklyWorking, maintenance.TriggerDaily, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			command := maintenance.Command{SchemaVersion: 1, CommandID: "synthetic-command", JobID: tc.job, WorkspaceID: "synthetic-owner", Trigger: tc.trigger, ScheduledFor: now, RequestedAt: now, Deadline: now.Add(time.Minute), ProposalOnly: tc.proposal, WeeklyMode: tc.mode}
			receipt := maintenance.Receipt{SchemaVersion: 1, AttemptID: strings.Repeat("a", 32), OccurrenceDigest: command.OccurrenceDigest(), CommandID: command.CommandID, JobID: command.JobID, WorkspaceID: command.WorkspaceID, Trigger: command.Trigger, State: maintenance.ReceiptSucceeded, RecordedAt: now, Deadline: command.Deadline, ProposalOnly: command.ProposalOnly, WeeklyMode: command.WeeklyMode, ReasonCode: "completed"}
			if tc.proposal {
				receipt.State, receipt.ReasonCode = maintenance.ReceiptUnavailable, "handler_unavailable"
			}
			for name, value := range map[string]any{"maintenance-command": command, "maintenance-receipt": receipt} {
				body, err := json.Marshal(value)
				if err != nil {
					t.Fatal(err)
				}
				var wire any
				if err := json.Unmarshal(body, &wire); err != nil {
					t.Fatal(err)
				}
				if err := schemas[name](wire); (err == nil) != tc.valid {
					t.Errorf("%s schema validity want=%v got=%v", name, tc.valid, err)
				}
				goErr := command.Validate(now)
				if name == "maintenance-receipt" {
					goErr = receipt.Validate()
				}
				if (goErr == nil) != tc.valid {
					t.Errorf("%s Go validity want=%v got=%v", name, tc.valid, goErr)
				}
				if tc.valid {
					wire.(map[string]any)["unreviewed_authority"] = true
					if schemas[name](wire) == nil {
						t.Errorf("%s accepted undeclared authority metadata", name)
					}
				}
			}
		})
	}
}
