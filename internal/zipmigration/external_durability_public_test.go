//go:build !windows

package zipmigration_test

import (
	"errors"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipmigration"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExternalCustomHomeDurablePhasesRetainEmptyDirectoriesAndPriorHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	for _, prior := range []bool{false, true} {
		t.Run(map[bool]string{false: "new-home", true: "prior-empty-home"}[prior], func(t *testing.T) {
			o := externalFixture(t, "0.1.12")
			custom := filepath.Join(filepath.Dir(o.Home), "custom-parent")
			if err := os.Mkdir(custom, 0700); err != nil {
				t.Fatal(err)
			}
			o.Home = filepath.Join(custom, "selected-home")
			if prior {
				if err := os.Mkdir(o.Home, 0700); err != nil {
					t.Fatal(err)
				}
			}
			externalPut(t, o.SourceRoot, "data/profile/note.md", "original source\n")
			externalPut(t, o.SourceRoot, "data/workspaces/project/note.md", "retained namespace\n")
			original := filepath.Join(o.SourceRoot, "data/profile/note.md")
			if err := os.Chmod(original, 0400); err != nil {
				t.Fatal(err)
			}
			before, err := os.Stat(original)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Join(o.SourceRoot, "data/profile/empty/deep"), 0700); err != nil {
				t.Fatal(err)
			}
			plan, err := zipmigration.PlanExternal(o)
			if err != nil {
				t.Fatal(err)
			}
			digest := externalDigest(t, plan)
			phases := map[string]bool{}
			status, err := zipmigration.ApplyExternal(plan, digest, func(event zipmigration.ExternalProgress) error {
				phases[event.Phase] = true
				return nil
			})
			if err != nil || !status.Current || !phases["prepared"] || !phases["pending"] || !phases["copied"] || !phases["committed"] {
				t.Fatal("custom HOME did not pass durable phase barriers", status.State, phases, err)
			}
			if entries, err := os.ReadDir(filepath.Join(o.Home, "brain/owner/empty/deep")); err != nil || len(entries) != 0 {
				t.Fatal("empty directory entry was not retained", err)
			}
			externalPut(t, o.Home, "brain/owner/note.md", "new authored work\n")
			stop := errors.New("stop after durable archive")
			status, err = zipmigration.RollbackExternal(o.Home, plan.AttemptID, digest, func(event zipmigration.ExternalProgress) error {
				if event.Phase == "archived" {
					return stop
				}
				return nil
			})
			if !errors.Is(err, stop) || status.State != "rollback_pending" || status.Current || status.RetainedPaths["workspaces"] != filepath.Join(plan.ArchiveRoot, "lineage", plan.AttemptID, "data/workspaces") {
				t.Fatal("archived effect was not retained on interruption", status, err)
			}
			status, err = zipmigration.RollbackExternal(o.Home, plan.AttemptID, digest, nil)
			if err != nil || status.State != "rolled_back" || status.Current {
				t.Fatal("archived/locator-absent retry failed", status.State, err)
			}
			if actual, err := os.ReadFile(filepath.Join(plan.ArchiveRoot, "brain/owner/note.md")); err != nil || string(actual) != "new authored work\n" {
				t.Fatal("rollback lost later work", err)
			}
			if entries, err := os.ReadDir(o.Home); (prior && (err != nil || len(entries) != 0)) || (!prior && !os.IsNotExist(err)) {
				t.Fatal("prior HOME state was not restored exactly", prior, err)
			}
			after, err := os.Stat(original)
			if err != nil || !os.SameFile(before, after) || before.Mode() != after.Mode() || !before.ModTime().Equal(after.ModTime()) {
				t.Fatal("source identity or metadata changed", err)
			}
			if actual, err := os.ReadFile(original); err != nil || string(actual) != "original source\n" {
				t.Fatal("source bytes changed", err)
			}
		})
	}
}
