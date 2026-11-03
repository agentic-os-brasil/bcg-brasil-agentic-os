package scheduler_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

func TestGuardedPrivateUpdateRefusesAtFinalPublicationAndRestoresExactAbsence(t *testing.T) {
	root := filepath.Join(t.TempDir(), "private")
	if e := scheduler.EnsurePrivateDirectory(root); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(root, "current.json")
	denied := errors.New("live grant revoked")
	if e := scheduler.UpdatePrivateFileGuarded(path, 1024, func(func() error) error { return denied }, func([]byte) ([]byte, error) { return []byte("new"), nil }); !errors.Is(e, denied) {
		t.Fatal(e)
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("refused final guard still published")
	}
	allow := func(f func() error) error { return f() }
	if e := scheduler.UpdatePrivateFileGuarded(path, 1024, allow, func([]byte) ([]byte, error) { return []byte("new"), nil }); e != nil {
		t.Fatal(e)
	}
	if e := scheduler.RestorePrivateFileGuarded(path, 1024, []byte("other"), nil, allow); e == nil {
		t.Fatal("compensation deleted another parent")
	}
	body, e := scheduler.ReadPrivateStateFile(path, 1024)
	if e != nil || !bytes.Equal(body, []byte("new")) {
		t.Fatal("stale compensation changed bytes")
	}
	if e := scheduler.RestorePrivateFileGuarded(path, 1024, []byte("new"), nil, allow); e != nil {
		t.Fatal(e)
	}
	if _, e := os.Stat(path); !os.IsNotExist(e) {
		t.Fatal("exact absence not restored")
	}
}
func TestBoundedReceiptReaderValidatesOwningIdentityAndAliases(t *testing.T) {
	root := filepath.Join(t.TempDir(), "scheduler")
	store := scheduler.Store{Root: root}
	at := time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)
	r := scheduler.Receipt{JobID: "runtime-health-check", ScheduledFor: at, AttemptedAt: at, State: scheduler.Succeeded}
	if e := store.AppendReceipt("workspace-a", r); e != nil {
		t.Fatal(e)
	}
	name := "20261103T120000.000000000Z-20261103T120000.000000000Z.json"
	read, e := store.ReadReceipt("workspace-a", r.JobID, name)
	if e != nil || read.JobID != r.JobID {
		t.Fatalf("canonical read failed: %+v %v", read, e)
	}
	if _, e = store.ReadReceipt("workspace-a", r.JobID, "../"+name); e == nil {
		t.Fatal("traversal admitted")
	}
	path := filepath.Join(root, "workspaces", "workspace-a", "receipts", r.JobID, name)
	alias := filepath.Join(filepath.Dir(path), "hard-link")
	if e = os.Link(path, alias); e != nil {
		t.Fatal(e)
	}
	if _, e = store.ReadReceipt("workspace-a", r.JobID, name); e == nil {
		t.Fatal("multi-link source admitted")
	}
}
