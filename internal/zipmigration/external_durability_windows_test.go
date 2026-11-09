//go:build windows

package zipmigration

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWindowsExternalAdapterDurabilityRetriesHeldObjectsWithoutSourceEffects(t *testing.T) {
	tree := externalNativeTree(t)
	name := "brain/owner/note.md"
	body := []byte("durable private destination\n")
	if err := externalEnsure(tree, name, body); err != nil {
		t.Fatal(err)
	}
	before, err := tree.stat(name)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.RemoveAll(filepath.Join(tree.project, filepath.FromSlash(externalControl))); err != nil {
		t.Fatal(err)
	}
	if err = externalEnsure(tree, name, body); err != nil {
		t.Fatal("equal-byte durability retry failed", err)
	}
	after, err := tree.stat(name)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("durability retry changed the published object", err)
	}
	externalNativeRead(t, tree.project, name, string(body))
	if _, err = os.Stat(filepath.Join(tree.project, filepath.FromSlash(externalControl))); !externalMissing(err) {
		t.Fatal("retry recreated missing staging ancestry", err)
	}
	o := externalNativeTransactionFixture(t)
	original := externalNativeSnapshot(t, o.SourceRoot)
	source, err := externalOpenSource(o.SourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if err = externalSyncPath(source, "VERSION"); err == nil {
		t.Fatal("source admission acquired sync authority")
	}
	if err = source.close(); err != nil {
		t.Fatal(err)
	}
	externalNativeUnchanged(t, o.SourceRoot, original)
}

func TestWindowsExternalAdapterDurabilityRetainsParentCustodyAcrossRollback(t *testing.T) {
	o := externalNativeTransactionFixture(t)
	original := externalNativeSnapshot(t, o.SourceRoot)
	custom := filepath.Join(filepath.Dir(o.Home), "custom-parent")
	if err := externalCreateHome(custom); err != nil {
		t.Fatal(err)
	}
	o.Home = filepath.Join(custom, "selected-home")
	if err := externalCreateHome(o.Home); err != nil {
		t.Fatal(err)
	}
	plan, digest := externalNativePlan(t, o)
	if !plan.PriorHome {
		t.Fatal("prior empty custom HOME was not sealed")
	}
	if status, err := applyExternal(plan, digest, nil); err != nil || !status.Current {
		t.Fatal("custom HOME apply failed", status.State, err)
	}
	externalNativePut(t, o.Home, "brain/owner/note.md", "new retained work\n")
	stop := errors.New("stop after actual durable revocation")
	if _, err := rollbackExternal(o.Home, plan.AttemptID, digest, func(event ExternalProgress) error {
		if event.Phase == "revoked" {
			return stop
		}
		return nil
	}); !errors.Is(err, stop) {
		t.Fatal("revocation interruption not observed", err)
	}
	_, mover, err := externalLoadTree(o.Home, plan.AttemptID, digest, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { mover.close() })
	before, err := mover.stat(".")
	if err != nil {
		t.Fatal(err)
	}
	if mover.homeParent == nil {
		t.Fatal("initial private parent custody missing")
	}
	if err = mover.homeParent.File.Close(); err != nil {
		t.Fatal(err)
	}
	moved, err := mover.move(plan.ArchiveRoot)
	if !moved || err == nil || mover.project != plan.ArchiveRoot {
		t.Fatal("closed parent flush lost the actual move", moved, mover.project, err)
	}
	after, err := mover.stat(".")
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("late failure lost archive identity", err)
	}
	if err = externalCheckRecord(mover, plan, digest, "revoked.json", "revoked"); err != nil {
		t.Fatal("same admitted archive proof became inaccessible", err)
	}
	externalNativeRead(t, plan.ArchiveRoot, "brain/owner/note.md", "new retained work\n")
	if err = mover.close(); !errors.Is(err, os.ErrClosed) {
		t.Fatal("owned parent close failure was discarded", err)
	}
	if status, err := rollbackExternal(o.Home, plan.AttemptID, digest, nil); err != nil || status.State != "rolled_back" || status.Current {
		t.Fatal("archived retry failed", status.State, err)
	}
	if entries, err := os.ReadDir(o.Home); err != nil || len(entries) != 0 {
		t.Fatal("prior empty custom HOME was not restored", err)
	}
	if status, err := rollbackExternal(o.Home, plan.AttemptID, digest, nil); err != nil || status.State != "rolled_back" {
		t.Fatal("locator-absent durability retry failed", status.State, err)
	}
	externalNativeRead(t, plan.ArchiveRoot, "brain/owner/note.md", "new retained work\n")
	externalNativeUnchanged(t, o.SourceRoot, original)
}
