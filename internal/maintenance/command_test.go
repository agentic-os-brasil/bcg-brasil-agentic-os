package maintenance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testCommand(now time.Time) Command {
	return Command{
		SchemaVersion: CommandSchemaVersion,
		CommandID:     "cmd-daily-1",
		JobID:         "memory-daily",
		WorkspaceID:   "workspace-1",
		Trigger:       TriggerDaily,
		ScheduledFor:  now,
		RequestedAt:   now,
		Deadline:      now.Add(time.Minute),
	}
}

func TestCommandRequiresExplicitBoundedDeadline(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	command := testCommand(now)
	if err := command.Validate(now); err != nil {
		t.Fatal(err)
	}
	command.Deadline = now.Add(16 * time.Minute)
	if err := command.Validate(now); err == nil {
		t.Fatal("unbounded command deadline was accepted")
	}
	command = testCommand(now)
	command.Deadline = now
	if err := command.Validate(now); err == nil {
		t.Fatal("expired command deadline was accepted")
	}
}

func TestContinuousGateMapsOnlyToEventJobs(t *testing.T) {
	catalog := Catalog{SchemaVersion: 1, CatalogState: CatalogOnly, Jobs: []Job{{
		ID: "wiki-incremental-sync", Category: "wiki", Trigger: "event", Executor: "deterministic", Scope: "workspace",
		Availability: Unavailable, AvailabilityReason: "not installed", DefaultEnabled: true, Unattended: "deterministic_only",
		Writes: []string{"wiki_private"}, SuccessBoundary: "watermark committed",
	}}}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	command := testCommand(now)
	command.CommandID = "cmd-event-1"
	command.JobID = "wiki-incremental-sync"
	command.Trigger = TriggerContinuous
	command.EventID = "source-change-1"
	decision, err := Gate(catalog, command, now)
	if err != nil || decision.State != "accepted" || len(decision.PlannedJobs) != 1 {
		t.Fatalf("decision=%#v err=%v", decision, err)
	}
}

func TestContinuousGateRejectsMissingEventID(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	command := testCommand(now)
	command.CommandID = "cmd-event-invalid"
	command.JobID = "wiki-incremental-sync"
	command.Trigger = TriggerContinuous
	if _, err := Gate(Catalog{}, command, now); err == nil {
		t.Fatal("continuous event without event ID was accepted")
	}
}

func TestProposalReceiptCannotClaimApplication(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	receipt := Receipt{SchemaVersion: CommandSchemaVersion, CommandID: "cmd-monthly-1", JobID: "darwin-structural-evolution-proposal", WorkspaceID: "workspace-1", Trigger: TriggerMonthly, State: ReceiptProposalEmitted, RecordedAt: now, Deadline: now.Add(time.Minute), ProposalOnly: true, ProposalCount: 2, ProposalDigest: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}
	if err := receipt.Validate(); err != nil {
		t.Fatal(err)
	}
	receipt.State = ReceiptSucceeded
	if err := receipt.Validate(); err == nil {
		t.Fatal("proposal-only receipt claimed applied success")
	}
}

func TestReceiptStoreKeepsOnlyTypedMetadata(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	store := Store{Root: t.TempDir()}
	receipt := Receipt{SchemaVersion: CommandSchemaVersion, CommandID: "cmd-store-1", JobID: "memory-daily", WorkspaceID: "workspace-1", Trigger: TriggerDaily, State: ReceiptSucceeded, RecordedAt: now, Deadline: now.Add(time.Minute)}
	if err := store.AppendReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	got, err := store.Receipts("workspace-1", "memory-daily")
	if err != nil || len(got) != 1 || got[0].CommandID != receipt.CommandID {
		t.Fatalf("receipts=%#v err=%v", got, err)
	}
	if err := store.AppendReceipt(receipt); err != nil {
		t.Fatalf("identical duplicate receipt was not idempotent: %v", err)
	}
}

func TestPublishedCommandAndReceiptSchemasAreStrictlyPresent(t *testing.T) {
	for _, name := range []string{"maintenance-command.schema.json", "maintenance-receipt.schema.json"} {
		body, err := os.ReadFile(filepath.Join("../../schemas", name))
		if err != nil {
			t.Fatal(err)
		}
		var schema map[string]any
		if err := json.Unmarshal(body, &schema); err != nil {
			t.Fatal(err)
		}
		if schema["additionalProperties"] != false || schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
			t.Fatalf("schema %s is not strict: %#v", name, schema)
		}
	}
}
