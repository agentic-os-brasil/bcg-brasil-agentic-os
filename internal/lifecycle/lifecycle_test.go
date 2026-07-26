package lifecycle

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordIsIdempotentAndMetadataOnly(t *testing.T) {
	root := t.TempDir()
	receipt := Receipt{Runtime: "claude", Event: PostActionObserve, State: "observed", OccurredAt: time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC), IdempotencyKey: IdempotencyKey("session", "tool")}
	if _, err := Record(root, "workspace-1", receipt); err != nil {
		t.Fatal(err)
	}
	if _, err := Record(root, "workspace-1", receipt); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "runtime", "receipts", "workspace-1"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %d, %v", len(entries), err)
	}
	summary, err := Diagnose(root, "workspace-1")
	if err != nil || summary.State != "observed" || summary.Observed != 1 || len(summary.Events) != 1 {
		t.Fatalf("summary = %#v, %v", summary, err)
	}
}

func TestDiagnoseAbsentIsExplicitlyUnavailable(t *testing.T) {
	summary, err := Diagnose(t.TempDir(), "workspace-1")
	if err != nil || summary.State != "unavailable" {
		t.Fatalf("summary = %#v, %v", summary, err)
	}
}

func TestRecordDoesNotOverwriteAnExistingReceipt(t *testing.T) {
	root := t.TempDir()
	workspaceRoot := filepath.Join(root, "runtime", "receipts", "workspace-1")
	if err := os.MkdirAll(workspaceRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	key := IdempotencyKey("a")
	path := filepath.Join(workspaceRoot, StopFinalize+"-"+key+".json")
	if err := os.WriteFile(path, []byte(`{"preserved":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Record(root, "workspace-1", Receipt{Runtime: "claude", Event: StopFinalize, State: "observed", IdempotencyKey: key}); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != `{"preserved":true}` {
		t.Fatalf("receipt was overwritten: %q, %v", body, err)
	}
}
