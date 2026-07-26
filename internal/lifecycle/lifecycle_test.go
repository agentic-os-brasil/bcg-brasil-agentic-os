package lifecycle

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testWorkspaceID = "0123456789abcdef0123456789abcdef"

func TestRecordIsIdempotentAndMetadataOnly(t *testing.T) {
	root := t.TempDir()
	receipt := Receipt{
		SchemaVersion: 1,
		Runtime:       "claude",
		Event:         PostActionObserve,
		State:         "observed",
		OccurredAt:    time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC),
		IdempotencyKey: IdempotencyKey(
			"session-secret",
			"tool-secret",
		),
		ToolName: "Bash",
	}
	if _, err := Record(root, testWorkspaceID, receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := Record(root, testWorkspaceID, receipt); err != nil {
		t.Fatal(err)
	}
	receiptRoot := filepath.Join(root, "runtime", "receipts", testWorkspaceID)
	entries, err := os.ReadDir(receiptRoot)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %d, %v", len(entries), err)
	}
	body, err := os.ReadFile(filepath.Join(receiptRoot, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	for _, prohibited := range []string{"session-secret", "tool-secret", "command", "workspace_path"} {
		if strings.Contains(string(body), prohibited) {
			t.Fatalf("receipt exposed %q: %s", prohibited, body)
		}
	}
	summary, err := Diagnose(root, testWorkspaceID)
	if err != nil || summary.State != "observed" || summary.Observed != 1 {
		t.Fatalf("summary = %#v, %v", summary, err)
	}
}

func TestRecordRejectsPathShapedWorkspaceAndReceiptIdentifiers(t *testing.T) {
	root := t.TempDir()
	valid := Receipt{
		SchemaVersion:  1,
		Runtime:        "claude",
		Event:          StopFinalize,
		State:          "observed",
		IdempotencyKey: IdempotencyKey("session"),
	}
	if _, err := Record(root, "../escape", valid); err == nil {
		t.Fatal("Record accepted a path-shaped workspace ID")
	}
	valid.IdempotencyKey = "../escape"
	if _, err := Record(root, testWorkspaceID, valid); err == nil {
		t.Fatal("Record accepted a path-shaped idempotency key")
	}
	valid.IdempotencyKey = IdempotencyKey("session")
	valid.Diagnostic = "free-form client content"
	if _, err := Record(root, testWorkspaceID, valid); err == nil {
		t.Fatal("Record accepted a free-form diagnostic")
	}
	if _, err := os.Stat(filepath.Join(root, "runtime", "escape")); !os.IsNotExist(err) {
		t.Fatalf("invalid receipt escaped its root: %v", err)
	}
}

func TestDiagnoseAbsentIsExplicitlyUnavailable(t *testing.T) {
	summary, err := Diagnose(t.TempDir(), testWorkspaceID)
	if err != nil || summary.State != "unavailable" {
		t.Fatalf("summary = %#v, %v", summary, err)
	}
}
