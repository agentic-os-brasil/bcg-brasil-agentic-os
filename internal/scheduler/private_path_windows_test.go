//go:build windows

package scheduler

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWindowsSecureSchedulerStateWorksAsStandardUser(t *testing.T) {
	root := t.TempDir()
	store := Store{Root: root}
	now := time.Now().UTC().Truncate(time.Second)
	if _, err := store.EnsureEnrollment("workspace-a", now); err != nil {
		t.Fatalf("enrollment failed under the current user: %v", err)
	}
	if err := store.AppendReceipt("workspace-a", Receipt{
		JobID:        "darwin-housekeeping-daily",
		ScheduledFor: now,
		AttemptedAt:  now,
		State:        Succeeded,
	}); err != nil {
		t.Fatalf("receipt failed under the current user: %v", err)
	}
	lease, err := store.TryAcquireLease("workspace-a", "darwin-housekeeping-daily", "occurrence", "windows-test", now, time.Minute)
	if err != nil {
		t.Fatalf("lease acquisition failed under the current user: %v", err)
	}
	if err := store.ArmLease(lease); err != nil {
		t.Fatalf("lease arm failed under the current user: %v", err)
	}
	if err := store.ReleaseLease(lease); err != nil {
		t.Fatalf("lease release failed under the current user: %v", err)
	}
}

func TestWindowsSecureSchedulerRejectsReparseAncestor(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	alias := filepath.Join(root, "alias")
	if err := os.Symlink(outside, alias); err != nil {
		t.Fatalf("Windows CI must permit the reparse-point test fixture: %v", err)
	}
	if _, err := (Store{Root: filepath.Join(alias, "scheduler")}).EnsureEnrollment("workspace-a", time.Now().UTC()); err == nil {
		t.Fatal("scheduler accepted a reparse-point ancestor")
	}
	if entries, err := os.ReadDir(outside); err != nil || len(entries) != 0 {
		t.Fatalf("scheduler touched the reparse target: entries=%#v err=%v", entries, err)
	}
}
