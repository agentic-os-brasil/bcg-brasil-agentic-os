package execution

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

func TestStoreRejectsSymlinkParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("legacy Windows Store is not product-confined")
	}
	store := testStore(t)
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(store.Root, "workspaces")); err != nil {
		t.Skip(err)
	}
	if _, err := store.Create(testCreateInput()); err == nil {
		t.Fatal("created through symlink parent")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("external directory mutated: %v %v", entries, err)
	}
}

func TestStoreRejectsOversizedContract(t *testing.T) {
	store := testStore(t)
	if _, err := store.Create(testCreateInput()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Root, "workspaces", testWorkspaceID, "execution", "items", "item-a", "contract.v1.json")
	file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	_, err = file.Write(bytes.Repeat([]byte(" "), 2<<20))
	if err != nil {
		t.Fatal(err)
	}
	file.Close()
	if _, err := store.Inspect(testWorkspaceID, "item-a"); err == nil {
		t.Fatal("accepted oversized contract")
	}
}

func TestStoreRejectsSymlinkContract(t *testing.T) {
	store := testStore(t)
	if _, err := store.Create(testCreateInput()); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Root, "workspaces", testWorkspaceID, "execution", "items", "item-a", "contract.v1.json")
	outside := filepath.Join(t.TempDir(), "contract.json")
	if err := os.Rename(path, outside); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, path); err != nil {
		t.Fatal("required file-reparse fixture unavailable", err)
	}
	before, external := confinementTreeSnapshot(t, store.Root), confinementTreeSnapshot(t, filepath.Dir(outside))
	if _, err := store.Start(testWorkspaceID, "item-a", 1); err == nil {
		t.Fatal("started using linked contract")
	}
	if !reflect.DeepEqual(before, confinementTreeSnapshot(t, store.Root)) || !reflect.DeepEqual(external, confinementTreeSnapshot(t, filepath.Dir(outside))) {
		t.Fatal("linked-contract refusal mutated ledger or external target")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(outside, path); err != nil {
		t.Fatal(err)
	}
	item, err := store.Inspect(testWorkspaceID, "item-a")
	if err != nil || item.State.StateRevision != 1 {
		t.Fatalf("mutation committed: %+v %v", item.State, err)
	}
}

func TestOpenConfinedStoreAdmission(t *testing.T) {
	root := t.TempDir()
	if runtime.GOOS != "windows" {
		if err := os.Chmod(root, 0700); err != nil {
			t.Fatal(err)
		}
	}
	store, err := OpenConfinedStore(root)
	if err != nil {
		t.Fatal(err)
	}
	item, err := store.Create(testCreateInput())
	if err != nil {
		t.Fatal(err)
	}
	reopened, err := OpenConfinedStore(root)
	if err != nil {
		t.Fatal(err)
	}
	started, err := reopened.Start(testWorkspaceID, item.State.ItemID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start(testWorkspaceID, item.State.ItemID, 1); !errors.Is(err, ErrRevisionConflict) {
		t.Fatalf("stale revision admitted: %v", err)
	}
	observed, err := reopened.Inspect(testWorkspaceID, item.State.ItemID)
	if err != nil || observed.State != started.State {
		t.Fatalf("reopened authority changed: %+v %v", observed.State, err)
	}
	link := filepath.Join(t.TempDir(), "home")
	confinementDirectoryAlias(t, link, root)
	if _, err := OpenConfinedStore(link); err == nil {
		t.Fatal("admitted linked root")
	}
	confinementPublicDirectory(t, root)
	if _, err := OpenConfinedStore(root); err == nil {
		t.Fatal("admitted public root")
	}
}

func TestEvidenceRejectsLinkedAndOversizedArtifactsWithoutMutation(t *testing.T) {
	for _, kind := range []string{"leaf", "parent", "oversize"} {
		t.Run(kind, func(t *testing.T) {
			store := evidenceStore(t)
			input := evidenceContract()
			input.Criteria = input.Criteria[:1]
			workspace := t.TempDir()
			externalRoot := t.TempDir()
			path := filepath.Join(workspace, "result.txt")
			if kind == "parent" {
				input.Criteria[0].TargetRef = "bcgos://workspace/linked/result.txt"
				input.AllowedRefs = []string{input.Criteria[0].TargetRef}
				outside := externalRoot
				if err := os.WriteFile(filepath.Join(outside, "result.txt"), []byte("outside"), 0600); err != nil {
					t.Fatal(err)
				}
				confinementDirectoryAlias(t, filepath.Join(workspace, "linked"), outside)
			} else if kind == "leaf" {
				outside := filepath.Join(externalRoot, "outside")
				if err := os.WriteFile(outside, []byte("outside"), 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, path); err != nil {
					t.Fatal("required file-reparse fixture unavailable", err)
				}
			} else {
				f, err := os.Create(path)
				if err != nil {
					t.Fatal(err)
				}
				if err := f.Truncate(101 << 20); err != nil {
					t.Fatal(err)
				}
				f.Close()
			}
			item, err := store.Create(input)
			if err != nil {
				t.Fatal(err)
			}
			item, err = store.Start(testWorkspaceID, item.State.ItemID, 1)
			if err != nil {
				t.Fatal(err)
			}
			before, external := confinementTreeSnapshot(t, store.Root), confinementTreeSnapshot(t, externalRoot)
			var workspaceBefore map[string]string
			if kind != "oversize" {
				workspaceBefore = confinementTreeSnapshot(t, workspace)
			}
			_, err = store.CollectEvidence(testWorkspaceID, item.State.ItemID, EvidenceInput{WorkspaceRoot: workspace, ExpectedRevision: 2, AttemptID: item.State.ActiveAttemptID, CriterionID: "artifact"})
			if err == nil {
				t.Fatal("accepted unsafe artifact")
			}
			if !reflect.DeepEqual(before, confinementTreeSnapshot(t, store.Root)) || !reflect.DeepEqual(external, confinementTreeSnapshot(t, externalRoot)) {
				t.Fatal("unsafe-artifact refusal mutated ledger or external target")
			}
			if kind != "oversize" && !reflect.DeepEqual(workspaceBefore, confinementTreeSnapshot(t, workspace)) {
				t.Fatal("unsafe-artifact refusal mutated workspace aliases")
			}
			observed, err := store.Inspect(testWorkspaceID, item.State.ItemID)
			if err != nil || observed.State.StateRevision != 2 {
				t.Fatalf("mutated on failed evidence: %+v %v", observed.State, err)
			}
		})
	}
}

func TestMutationRejectsSwappedRevisionParent(t *testing.T) {
	store := testStore(t)
	if _, err := store.Create(testCreateInput()); err != nil {
		t.Fatal(err)
	}
	revisions := filepath.Join(store.Root, "workspaces", testWorkspaceID, "execution", "items", "item-a", "revisions")
	outside := t.TempDir()
	before, external := confinementTreeSnapshot(t, store.Root), confinementTreeSnapshot(t, outside)
	swapped := false
	store.FaultPoint = func(point string) error {
		if point != "before_revision_commit" {
			return nil
		}
		if err := os.Rename(revisions, revisions+".saved"); err != nil {
			return err
		}
		confinementDirectoryAlias(t, revisions, outside)
		swapped = true
		return nil
	}
	if _, err := store.Start(testWorkspaceID, "item-a", 1); err == nil {
		t.Fatal("published through swapped revision parent")
	}
	if !swapped {
		t.Fatal("mutation refused before the adversarial revision-parent swap executed")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("external directory changed: %v %v", entries, err)
	}
	if err := os.Remove(revisions); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(revisions+".saved", revisions); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, confinementTreeSnapshot(t, store.Root)) || !reflect.DeepEqual(external, confinementTreeSnapshot(t, outside)) {
		t.Fatal("swapped-parent refusal mutated ledger or outside tree")
	}
	item, err := store.Inspect(testWorkspaceID, "item-a")
	if err != nil || item.State.StateRevision != 1 {
		t.Fatalf("failed mutation advanced state: %+v %v", item.State, err)
	}
}

func TestStoreRejectsOversizedRevisionWithoutFallback(t *testing.T) {
	store := testStore(t)
	if _, err := store.Create(testCreateInput()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start(testWorkspaceID, "item-a", 1); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Root, "workspaces", testWorkspaceID, "execution", "items", "item-a", "revisions", "00000000000000000002.json")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write(bytes.Repeat([]byte(" "), 2<<20)); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := store.Inspect(testWorkspaceID, "item-a"); err == nil {
		t.Fatal("unsafe revision silently rolled back")
	}
}

func TestPublishedMalformedRevisionCannotRestoreOldAttempt(t *testing.T) {
	store := testStore(t)
	if _, err := store.Create(testCreateInput()); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Start(testWorkspaceID, "item-a", 1); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(store.Root, "workspaces", testWorkspaceID, "execution", "items", "item-a", "revisions", "00000000000000000002.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Inspect(testWorkspaceID, "item-a"); err == nil {
		t.Fatal("malformed published revision restored old authority")
	}
	if _, err := store.Start(testWorkspaceID, "item-a", 1); err == nil {
		t.Fatal("stale writer regained authority")
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
	item, err := store.Inspect(testWorkspaceID, "item-a")
	if err != nil || item.State.StateRevision != 2 || item.State.ActiveAttemptID != "attempt-a" {
		t.Fatalf("published authority changed: %+v %v", item.State, err)
	}
}

func confinementTreeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	snapshot := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		value := info.Mode().String() + confinementSecuritySnapshot(t, path)
		// Directory timestamps reflect transient locks and deferred NTFS updates.
		if !info.IsDir() {
			value += " " + info.ModTime().String()
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			value += " " + target
		} else if info.Mode().IsRegular() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += " " + string(body)
		}
		snapshot[path] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}
