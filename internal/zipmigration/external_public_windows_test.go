//go:build windows

package zipmigration

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsExternalPublicReviewedPlanRefusesBeforeEffects(t *testing.T) {
	o := externalNativeTransactionFixture(t)
	original := externalNativeSnapshot(t, o.SourceRoot)
	profile := externalNativeSnapshot(t, o.UserHome)
	plan, digest := externalNativePlan(t, o)
	unreviewed := digest[:len(digest)-1] + "0"
	if unreviewed == digest {
		unreviewed = digest[:len(digest)-1] + "1"
	}
	tampered := plan
	tampered.Entries = append([]ExternalEntry(nil), plan.Entries...)
	tampered.Entries[0].Target = "brain/redirected"
	invalid := ExternalPlan{}
	tamperedBytes, err := tampered.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	invalidBytes, err := invalid.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []struct {
		plan   ExternalPlan
		digest string
	}{{plan, unreviewed}, {tampered, hash(tamperedBytes)}, {invalid, hash(invalidBytes)}} {
		status, err := ApplyExternal(candidate.plan, candidate.digest, nil)
		if err == nil || status.Current {
			t.Fatal("invalid or unreviewed plan acquired authority", status.State, err)
		}
		if _, err = os.Stat(o.Home); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("refusal created HOME", err)
		}
		if _, err = os.Stat(plan.ArchiveRoot); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("refusal created rollback archive", err)
		}
		externalNativeUnchanged(t, o.SourceRoot, original)
		externalNativeUnchanged(t, o.UserHome, profile)
	}
	if status, err := RollbackExternal(o.Home, "unsealed-attempt", digest, nil); err == nil || status.Current {
		t.Fatal("unsealed rollback acquired authority", status.State, err)
	}
	externalNativeUnchanged(t, o.SourceRoot, original)
	externalNativeUnchanged(t, o.UserHome, profile)
}

func TestWindowsExternalPublicCommitPreservesOriginalAndOfflineProof(t *testing.T) {
	o := externalNativeTransactionFixture(t)
	original := externalNativeSnapshot(t, o.SourceRoot)
	plan, digest := externalNativePlan(t, o)
	status, err := ApplyExternal(plan, digest, nil)
	if err != nil || !status.Current || status.State != "committed" {
		t.Fatal("public apply failed", status.State, err)
	}
	inspected, err := InspectExternal(o.Home, plan.AttemptID, digest)
	if err != nil || !inspected.Current || inspected.PlanSHA256 != digest || inspected.SourceRoot != o.SourceRoot {
		t.Fatal("public committed proof failed", inspected.State, err)
	}
	externalNativeRead(t, o.Home, "brain/owner/note.md", "authored before external migration\n")
	externalNativeRead(t, o.Home, "brain/owner/.hidden.md", "hidden authored work\n")
	if info, err := os.Stat(filepath.Join(o.Home, "brain/owner/empty")); err != nil || !info.IsDir() {
		t.Fatal("public migration lost empty directory", err)
	}
	externalNativeUnchanged(t, o.SourceRoot, original)
	externalNativePut(t, o.Home, "brain/owner/note.md", "new authored work after public commit\n")
	offline := o.SourceRoot + "-offline"
	if err = os.Rename(o.SourceRoot, offline); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.Rename(offline, o.SourceRoot); err != nil {
			t.Error(err)
		}
	}()
	if inspected, err = InspectExternal(o.Home, plan.AttemptID, digest); err != nil || !inspected.Current {
		t.Fatal("public inspector required offline original", inspected.State, err)
	}
	if status, err = ApplyExternal(plan, digest, nil); err != nil || !status.Current || status.State != "already_migrated" {
		t.Fatal("public repeat required offline original or recopied", status.State, err)
	}
	externalNativeRead(t, o.Home, "brain/owner/note.md", "new authored work after public commit\n")
	active := externalNativeSnapshot(t, o.Home)
	profile := externalNativeSnapshot(t, o.UserHome)
	if status, err = RollbackExternal(o.Home, plan.AttemptID, digest, nil); err == nil || status.Current {
		t.Fatal("public rollback revoked HOME without its original", status.State, err)
	}
	externalNativeUnchanged(t, o.Home, active)
	externalNativeUnchanged(t, o.UserHome, profile)
	externalNativeUnchanged(t, offline, original)
}

func TestWindowsExternalPublicInterruptionResumesAndRollbackRetainsWork(t *testing.T) {
	o := externalNativeTransactionFixture(t)
	original := externalNativeSnapshot(t, o.SourceRoot)
	plan, digest := externalNativePlan(t, o)
	stop := errors.New("synthetic public transaction interruption")
	status, err := ApplyExternal(plan, digest, func(event ExternalProgress) error {
		if event.Phase == "copying" {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || status.Current {
		t.Fatal("public copy interruption was not retained", status.State, err)
	}
	if inspected, err := InspectExternal(o.Home, plan.AttemptID, digest); err == nil && inspected.Current {
		t.Fatal("public partial migration became current")
	}
	if status, err = ApplyExternal(plan, digest, nil); err != nil || !status.Current {
		t.Fatal("public same-attempt resume failed", status.State, err)
	}
	externalNativePut(t, o.Home, "brain/owner/note.md", "evolved public HOME work\n")
	externalNativePut(t, o.Home, "brain/owner/new.md", "new public HOME page\n")
	if err = os.Remove(filepath.Join(o.Home, "brain/owner/.hidden.md")); err != nil {
		t.Fatal(err)
	}
	if status, err = ApplyExternal(plan, digest, nil); err != nil || status.State != "already_migrated" {
		t.Fatal("public repeat did not preserve authored work", status.State, err)
	}
	status, err = RollbackExternal(o.Home, plan.AttemptID, digest, func(event ExternalProgress) error {
		if event.Phase == "archived" {
			return stop
		}
		return nil
	})
	if !errors.Is(err, stop) || status.Current || status.State != "rollback_pending" || status.ArchiveRoot != plan.ArchiveRoot {
		t.Fatal("public archived interruption lost effect", status.State, err)
	}
	if inspected, err := InspectExternal(o.Home, plan.AttemptID, digest); err != nil || inspected.Current || inspected.State != "revoked" {
		t.Fatal("public archive revocation proof failed", inspected.State, err)
	}
	if status, err = RollbackExternal(o.Home, plan.AttemptID, digest, nil); err != nil || status.Current || status.State != "rolled_back" || status.RestoredRuntime {
		t.Fatal("public rollback resume failed", status.State, err)
	}
	if status, err = RollbackExternal(o.Home, plan.AttemptID, digest, nil); err != nil || status.Current || status.State != "rolled_back" {
		t.Fatal("public rollback replay failed", status.State, err)
	}
	externalNativeRead(t, plan.ArchiveRoot, "brain/owner/note.md", "evolved public HOME work\n")
	externalNativeRead(t, plan.ArchiveRoot, "brain/owner/new.md", "new public HOME page\n")
	if _, err = os.Stat(filepath.Join(plan.ArchiveRoot, "brain/owner/.hidden.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("public rollback resurrected deleted work", err)
	}
	if _, err = os.Stat(filepath.Join(o.UserHome, ".maestro-home.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("public rollback retained owned locator", err)
	}
	if retired, err := RetiredExternalHome(o.UserHome, o.Home); err != nil || !retired {
		t.Fatal("public retirement proof failed", retired, err)
	}
	if inspected, err := InspectExternal(o.Home, plan.AttemptID, digest); err != nil || inspected.Current || inspected.State != "rolled_back" {
		t.Fatal("public archived receipt inspection failed", inspected.State, err)
	}
	externalNativeUnchanged(t, o.SourceRoot, original)
}
