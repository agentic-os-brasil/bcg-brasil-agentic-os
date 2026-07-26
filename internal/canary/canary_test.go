package canary

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestStoreAggregatesOnlyClosedLocalMetrics(t *testing.T) {
	store := Store{Root: t.TempDir()}
	now := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	receipts := []Receipt{
		{RecordedAt: now, Event: EventFirstValue, Outcome: OutcomeSucceeded, Duration: DurationUnderThirtyMinutes},
		{RecordedAt: now.Add(time.Second), Event: EventGoalResume, Outcome: OutcomeSucceeded},
		{RecordedAt: now.Add(2 * time.Second), Event: EventGoalResume, Outcome: OutcomeBlocked},
		{RecordedAt: now.Add(3 * time.Second), Event: EventInstallation, Outcome: OutcomeSucceeded},
		{RecordedAt: now.Add(4 * time.Second), Event: EventManualIntervention, Outcome: OutcomeNeutral, Count: CountTwoToThree},
		{RecordedAt: now.Add(5 * time.Second), Event: EventCapabilityFailure, Outcome: OutcomeFailed, Capability: CapabilitySessionContext},
	}
	for _, receipt := range receipts {
		if err := store.Append(receipt); err != nil {
			t.Fatal(err)
		}
	}
	report, err := store.Report()
	if err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != 1 || report.ReceiptCount != len(receipts) {
		t.Fatalf("unexpected report header: %#v", report)
	}
	if !reflect.DeepEqual(report.FirstValue, []DurationCount{{Duration: DurationUnderThirtyMinutes, Count: 1}}) {
		t.Fatalf("unexpected first-value aggregate: %#v", report.FirstValue)
	}
	if !reflect.DeepEqual(report.Resume, []OutcomeCount{{Outcome: OutcomeSucceeded, Count: 1}, {Outcome: OutcomeBlocked, Count: 1}}) {
		t.Fatalf("unexpected resume aggregate: %#v", report.Resume)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{"workspace", "client", "prompt", "path", "error", "objective", "summary"} {
		if strings.Contains(strings.ToLower(string(encoded)), prohibited) {
			t.Fatalf("aggregate exposed prohibited field %q: %s", prohibited, encoded)
		}
	}
}

func TestReceiptValidationRejectsCrossEventFields(t *testing.T) {
	now := time.Now().UTC()
	invalid := []Receipt{
		{SchemaVersion: 1, RecordedAt: now, Event: EventFirstValue, Outcome: OutcomeSucceeded},
		{SchemaVersion: 1, RecordedAt: now, Event: EventGoalResume, Outcome: OutcomeSucceeded, Count: CountOnce},
		{SchemaVersion: 1, RecordedAt: now, Event: EventManualIntervention, Outcome: OutcomeSucceeded, Count: CountOnce},
		{SchemaVersion: 1, RecordedAt: now, Event: EventCapabilityFailure, Outcome: OutcomeFailed, Capability: "arbitrary"},
		{SchemaVersion: 1, RecordedAt: now, Event: "custom", Outcome: OutcomeNeutral},
	}
	for _, receipt := range invalid {
		if err := receipt.Validate(); err == nil {
			t.Fatalf("invalid receipt accepted: %#v", receipt)
		}
	}
}

func TestReportRejectsUnknownFieldsAndUnexpectedEntries(t *testing.T) {
	root := t.TempDir()
	receipts := filepath.Join(root, "receipts")
	if err := os.MkdirAll(receipts, 0o700); err != nil {
		t.Fatal(err)
	}
	body := `{"schema_version":1,"recorded_at":"2026-07-26T09:00:00Z","event":"goal_resume","outcome":"succeeded","workspace":"secret"}`
	if err := os.WriteFile(filepath.Join(receipts, "receipt.json"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Root: root}).Report(); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field did not fail closed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(receipts, "unexpected.txt"), []byte("ignored?"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Root: root}).Report(); err == nil {
		t.Fatal("unexpected receipt directory entry was ignored")
	}
}

func TestReportRejectsDuplicateKeysAndSymlinkedReceipts(t *testing.T) {
	root := t.TempDir()
	receipts := filepath.Join(root, "receipts")
	if err := os.MkdirAll(receipts, 0o700); err != nil {
		t.Fatal(err)
	}
	duplicate := `{"schema_version":1,"recorded_at":"2026-07-26T09:00:00Z","event":"goal_resume","outcome":"succeeded","outcome":"failed"}`
	duplicatePath := filepath.Join(receipts, "duplicate.json")
	if err := os.WriteFile(duplicatePath, []byte(duplicate), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (Store{Root: root}).Report(); err == nil ||
		!strings.Contains(err.Error(), "duplicate JSON key") {
		t.Fatalf("duplicate key did not fail closed: %v", err)
	}
	if err := os.Remove(duplicatePath); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "external.json")
	valid := `{"schema_version":1,"recorded_at":"2026-07-26T09:00:00Z","event":"goal_resume","outcome":"succeeded"}`
	if err := os.WriteFile(target, []byte(valid), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(receipts, "linked.json")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if _, err := (Store{Root: root}).Report(); err == nil {
		t.Fatal("symlinked receipt entry was followed")
	}
}

func TestStoreRejectsSymlinkedReceiptRootAndRepairsPermissions(t *testing.T) {
	root := t.TempDir()
	receipts := filepath.Join(root, "receipts")
	if err := os.Mkdir(receipts, 0o755); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: root}
	now := time.Date(2026, 7, 26, 9, 0, 0, 0, time.UTC)
	if err := store.Append(Receipt{
		RecordedAt: now, Event: EventGoalResume, Outcome: OutcomeSucceeded,
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(receipts)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("receipt directory mode = %o", info.Mode().Perm())
	}

	symlinkRoot := t.TempDir()
	target := t.TempDir()
	if err := os.Symlink(target, filepath.Join(symlinkRoot, "receipts")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	symlinkStore := Store{Root: symlinkRoot}
	if err := symlinkStore.Append(Receipt{
		RecordedAt: now, Event: EventGoalResume, Outcome: OutcomeSucceeded,
	}); err == nil {
		t.Fatal("append accepted a symlinked receipt root")
	}
	if _, err := symlinkStore.Report(); err == nil {
		t.Fatal("report accepted a symlinked receipt root")
	}
}

func TestSchemasCompileAndRejectArbitraryReceiptContent(t *testing.T) {
	root := filepath.Join("..", "..", "schemas")
	receiptSchema := compileSchema(t, filepath.Join(root, "canary-receipt.schema.json"))
	reportSchema := compileSchema(t, filepath.Join(root, "canary-report.schema.json"))

	validReceipt := decodeJSON(t, `{"schema_version":1,"recorded_at":"2026-07-26T09:00:00Z","event":"goal_resume","outcome":"succeeded"}`)
	if err := receiptSchema.Validate(validReceipt); err != nil {
		t.Fatalf("valid receipt rejected by schema: %v", err)
	}
	invalidReceipt := decodeJSON(t, `{"schema_version":1,"recorded_at":"2026-07-26T09:00:00Z","event":"goal_resume","outcome":"succeeded","prompt":"secret"}`)
	if err := receiptSchema.Validate(invalidReceipt); err == nil {
		t.Fatal("schema accepted arbitrary receipt content")
	}
	validReport := decodeJSON(t, `{"schema_version":1,"receipt_count":0,"first_value":[],"resume":[],"lifecycle":[],"manual_interventions":[],"capability_failures":[]}`)
	if err := reportSchema.Validate(validReport); err != nil {
		t.Fatalf("valid report rejected by schema: %v", err)
	}
}

func compileSchema(t *testing.T, path string) *jsonschema.Schema {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource(filepath.Base(path), document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	return schema
}

func decodeJSON(t *testing.T, raw string) any {
	t.Helper()
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		t.Fatal(err)
	}
	return value
}
