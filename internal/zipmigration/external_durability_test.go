//go:build !windows

package zipmigration

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func externalDurabilityTree(t *testing.T) *externalTree {
	t.Helper()
	if err := externalMutationAllowed(); err != nil {
		t.Skip(err)
	}
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	e, err := externalOpen(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.close() })
	return e
}

func TestExternalPublicationFlushesActualParentsAndRetriesWithoutRepair(t *testing.T) {
	e := externalDurabilityTree(t)
	for _, name := range []string{"from/nested", "to/nested"} {
		if err := externalMkdirAll(e, name); err != nil {
			t.Fatal(err)
		}
	}
	body := []byte("preserved authored bytes\n")
	from, to := "from/nested/staged.json", "to/nested/published.json"
	if err := os.WriteFile(filepath.Join(e.project, from), body, 0600); err != nil {
		t.Fatal(err)
	}
	if err := externalSyncPath(e, from); err != nil {
		t.Fatal(err)
	}
	before, err := e.root.Lstat(from)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := externalPublish(e, from, to)
	if err != nil || !moved {
		t.Fatal("actual containing-directory publication did not complete", moved, err)
	}
	if _, err := e.root.Lstat(from); !os.IsNotExist(err) {
		t.Fatal("staged name remained", err)
	}
	if err := externalEnsure(e, to, body); err != nil {
		t.Fatal("equal-byte retry did not flush existing publication", err)
	}
	after, err := e.root.Lstat(to)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("retry replaced or changed the published object", err)
	}
	actual, err := os.ReadFile(filepath.Join(e.project, to))
	if err != nil || !bytes.Equal(actual, body) {
		t.Fatal("published bytes changed", err)
	}
	if _, err := e.root.Lstat(externalControl); !os.IsNotExist(err) {
		t.Fatal("durability retry recreated a missing staging tree", err)
	}
}

func TestExternalPublicationRetainsMoveAfterActualDirectorySyncFailure(t *testing.T) {
	e := externalDurabilityTree(t)
	for _, name := range []string{"from", "to"} {
		if err := externalMkdirAll(e, name); err != nil {
			t.Fatal(err)
		}
	}
	from, to := "from/home", "to/archive"
	if err := os.WriteFile(filepath.Join(e.project, from), []byte("retain newer work\n"), 0600); err != nil {
		t.Fatal(err)
	}
	before, err := e.root.Lstat(from)
	if err != nil {
		t.Fatal(err)
	}
	parents := map[string]bool{}
	moved, err := externalPublishAfterRename(e, from, to, func(source, target *os.File) {
		for _, directory := range []*os.File{source, target} {
			info, err := directory.Stat()
			if err != nil || !info.IsDir() {
				t.Fatal("publication did not retain the actual containing directory", err)
			}
			parents[directory.Name()] = false
			if err := directory.Close(); err != nil {
				t.Fatal(err)
			}
		}
	})
	if !moved || !errors.Is(err, os.ErrClosed) {
		t.Fatal("late directory-sync failure lost the namespace effect", moved, err)
	}
	var visit func(error)
	visit = func(err error) {
		if failure, ok := err.(*os.PathError); ok && failure.Op == "sync" {
			parents[failure.Path] = true
		}
		switch wrapped := err.(type) {
		case interface{ Unwrap() []error }:
			for _, child := range wrapped.Unwrap() {
				visit(child)
			}
		case interface{ Unwrap() error }:
			visit(wrapped.Unwrap())
		}
	}
	visit(err)
	if len(parents) != 2 {
		t.Fatal("source and destination parents were not distinct", parents)
	}
	for parent, synced := range parents {
		if !synced {
			t.Fatal("actual native directory Sync error was discarded", parent, err)
		}
	}
	if _, err := e.root.Lstat(from); !os.IsNotExist(err) {
		t.Fatal("late failure recreated the original name", err)
	}
	after, err := e.root.Lstat(to)
	if err != nil || !os.SameFile(before, after) {
		t.Fatal("late failure lost the archived object", err)
	}
	actual, err := os.ReadFile(filepath.Join(e.project, to))
	if err != nil || string(actual) != "retain newer work\n" {
		t.Fatal("late failure changed retained bytes", err)
	}
}

func TestExternalSourceTreeCannotFlushOrRetireOriginal(t *testing.T) {
	e := externalDurabilityTree(t)
	name := "original.md"
	body := []byte("untouched original\n")
	if err := os.WriteFile(filepath.Join(e.project, name), body, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := e.stat(name)
	if err != nil {
		t.Fatal(err)
	}
	source, err := externalOpenSource(e.project)
	if err != nil {
		t.Fatal(err)
	}
	defer source.close()
	if err = externalSyncPath(source, name); err == nil {
		t.Fatal("source tree acquired sync authority")
	}
	if marked, err := source.removeExact(name, body); err == nil || marked {
		t.Fatal("source tree acquired retirement authority", marked, err)
	}
	after, err := e.stat(name)
	if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
		t.Fatal("source metadata changed", err)
	}
	actual, err := os.ReadFile(filepath.Join(e.project, name))
	if err != nil || !bytes.Equal(actual, body) {
		t.Fatal("source bytes changed", err)
	}
}
