package zipmigration_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipmigration"
)

func TestExternalCurrentBrainSurvivesLegacyEvolution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	o := externalFixture(t, "0.2.0")
	externalPut(t, o.SourceRoot, "data/profile/a.md", "old a")
	externalPut(t, o.SourceRoot, "data/profile/b.md", "deleted b")
	if state := zipmigration.Run(o.SourceRoot, zipmigration.Options{}); state.State != "committed" {
		t.Fatal(state)
	}
	externalPut(t, o.SourceRoot, "brain/owner/a.md", "edited a")
	externalPut(t, o.SourceRoot, "brain/owner/c.md", "new c")
	if err := os.Remove(filepath.Join(o.SourceRoot, "brain/owner/b.md")); err != nil {
		t.Fatal(err)
	}
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	status, err := zipmigration.ApplyExternal(p, externalDigest(t, p), nil)
	if err != nil || !status.Current {
		t.Fatalf("%+v %v", status, err)
	}
	for rel, want := range map[string]string{"brain/owner/a.md": "edited a", "brain/owner/c.md": "new c", "lineage/" + p.AttemptID + "/data/profile/b.md": "deleted b"} {
		b, e := os.ReadFile(filepath.Join(o.Home, rel))
		if e != nil || string(b) != want {
			t.Fatalf("%s %q %v", rel, b, e)
		}
	}
	if _, err = os.Stat(filepath.Join(o.Home, "brain/owner/b.md")); !os.IsNotExist(err) {
		t.Fatal("deleted author file resurrected")
	}
	if state := zipmigration.Run(o.SourceRoot, zipmigration.Options{Status: true}); state.State != "committed" {
		t.Fatal("original runtime broken", state)
	}
}

func TestExternalReviewedPlanCannotOmitOrRedirectSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	for _, change := range []string{"omit", "redirect"} {
		t.Run(change, func(t *testing.T) {
			o := externalFixture(t, "0.1.12")
			externalPut(t, o.SourceRoot, "data/profile/identity.md", "must preserve")
			p, err := zipmigration.PlanExternal(o)
			if err != nil {
				t.Fatal(err)
			}
			if change == "omit" {
				p.Entries = p.Entries[:len(p.Entries)-1]
			} else {
				p.Entries[len(p.Entries)-1].Target = "brain/owner/wrong.md"
			}
			if _, err = zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err == nil {
				t.Fatal("noncanonical source mapping admitted")
			}
			if _, err = os.Stat(o.Home); !os.IsNotExist(err) {
				t.Fatal("invalid plan changed destination")
			}
		})
	}
}

func TestExternalControlStagingNeverWritesInsideSource(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	o := externalFixture(t, "0.1.12")
	o.SourceRoot = filepath.Join(o.UserHome, "brain")
	o.ProductRoot = o.SourceRoot
	if err := os.Mkdir(o.SourceRoot, 0700); err != nil {
		t.Fatal(err)
	}
	externalPut(t, o.SourceRoot, "VERSION", "0.1.12\n")
	externalPut(t, o.SourceRoot, "data/profile/identity.md", "same source product\n")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	status, err := zipmigration.ApplyExternal(p, externalDigest(t, p), nil)
	if err != nil || !status.Current {
		t.Fatalf("%+v %v", status, err)
	}
	entries, err := os.ReadDir(o.SourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("control publication changed source: %v", entries)
	}
	if _, err = os.Stat(filepath.Join(o.SourceRoot, ".maestro")); !os.IsNotExist(err) {
		t.Fatal("locator staging wrote inside source")
	}
}

func TestExternalPhysicalWriteSetAliasesRefuseBeforeEffects(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	for _, target := range []string{"home", "profile-control"} {
		t.Run(target, func(t *testing.T) {
			o := externalFixture(t, "0.1.12")
			externalPut(t, o.SourceRoot, "data/profile/identity.md", "original")
			reviewed, err := zipmigration.PlanExternal(o)
			if err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(filepath.Dir(o.SourceRoot), "SOURCE")
			if target == "profile-control" {
				o.SourceRoot = filepath.Join(o.UserHome, ".MAESTRO-EXTERNAL-MIGRATION")
				if err := os.Mkdir(o.SourceRoot, 0700); err != nil {
					t.Fatal(err)
				}
				externalPut(t, o.SourceRoot, "VERSION", "0.1.12\n")
				externalPut(t, o.SourceRoot, "data/profile/identity.md", "original")
				alias = filepath.Join(o.UserHome, ".maestro-external-migration")
				o.ProductRoot = o.SourceRoot
			}
			a, err := os.Stat(alias)
			if os.IsNotExist(err) {
				t.Skip("filesystem distinguishes case aliases")
			}
			b, otherErr := os.Stat(o.SourceRoot)
			if err != nil || otherErr != nil || !os.SameFile(a, b) {
				t.Fatalf("alias identity was not established: %v %v", err, otherErr)
			}
			if target == "home" {
				o.Home = filepath.Join(alias, "new-home")
			}
			before, err := os.ReadDir(o.SourceRoot)
			if err != nil {
				t.Fatal(err)
			}
			if p, planErr := zipmigration.PlanExternal(o); planErr == nil {
				if status, applyErr := zipmigration.ApplyExternal(p, externalDigest(t, p), nil); applyErr == nil || status.Current {
					t.Errorf("physical overlap activated: %+v %v", status, applyErr)
				}
				t.Error("plan admitted physical write-set overlap")
			}
			reviewed.ExternalOptions = o
			reviewed.ArchiveRoot = filepath.Join(filepath.Dir(o.Home), ".maestro-rollback-"+reviewed.AttemptID)
			if status, applyErr := zipmigration.ApplyExternal(reviewed, externalDigest(t, reviewed), nil); applyErr == nil || status.Current {
				t.Errorf("imported physical overlap activated: %+v %v", status, applyErr)
			}
			after, err := os.ReadDir(o.SourceRoot)
			if err != nil || len(before) != len(after) {
				t.Fatalf("source changed: %v %v", after, err)
			}
			if _, err := os.Stat(o.Home); !os.IsNotExist(err) {
				t.Fatal("overlap created HOME")
			}
		})
	}
}

func TestExternalPrivateControlDirectoriesRefuseWithoutRepair(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	for _, directory := range []string{"control", "attempt", "locator-staging"} {
		t.Run(directory, func(t *testing.T) {
			o := externalFixture(t, "0.1.12")
			externalPut(t, o.SourceRoot, "data/profile/identity.md", "original")
			p, err := zipmigration.PlanExternal(o)
			if err != nil {
				t.Fatal(err)
			}
			name := filepath.Join(o.UserHome, ".maestro-external-migration")
			if directory == "attempt" {
				name = filepath.Join(name, p.AttemptID)
			} else if directory == "locator-staging" {
				name = filepath.Join(name, ".locator-staging")
			}
			if err = os.MkdirAll(name, 0700); err != nil {
				t.Fatal(err)
			}
			if err = os.Chmod(o.UserHome, 0755); err != nil {
				t.Fatal(err)
			}
			if err = os.Chmod(name, 0777); err != nil {
				t.Fatal(err)
			}
			if _, err = zipmigration.PlanExternal(o); err == nil && directory != "attempt" {
				t.Error("plan admitted nonprivate controls")
			}
			if status, err := zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err == nil || status.Current {
				t.Errorf("apply admitted nonprivate controls: %+v %v", status, err)
			}
			info, err := os.Stat(name)
			if err != nil || info.Mode().Perm() != 0777 {
				t.Fatal("unsafe control was repaired", err)
			}
			for _, leaf := range []string{o.Home, filepath.Join(o.UserHome, ".maestro-home.json"), filepath.Join(o.UserHome, ".maestro-external-migration/apply.lock")} {
				if _, err := os.Stat(leaf); !os.IsNotExist(err) {
					t.Error("unsafe controls allowed first effect", leaf)
				}
			}
		})
	}
}

func TestExternalHomeCannotOverlapProfilePublication(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	o := externalFixture(t, "0.1.12")
	externalPut(t, o.SourceRoot, "data/profile/identity.md", "original")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	o.Home = filepath.Join(o.UserHome, ".maestro-external-migration")
	if _, err = zipmigration.PlanExternal(o); err == nil {
		t.Error("plan admitted HOME as profile-control namespace")
	}
	p.ExternalOptions = o
	p.ArchiveRoot = filepath.Join(filepath.Dir(o.Home), ".maestro-rollback-"+p.AttemptID)
	if _, err = zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err == nil {
		t.Error("apply admitted HOME as profile-control namespace")
	}
	if _, err = os.Stat(o.Home); !os.IsNotExist(err) {
		t.Fatal("overlap wrote profile control state before refusing")
	}
}

func TestExternalMissingReservedProfileAliasesRefuseBeforeEffects(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	for _, reserved := range []string{".MAESTRO-EXTERNAL-MIGRATION", ".MAESTRO-HOME.JSON"} {
		t.Run(reserved, func(t *testing.T) {
			o := externalFixture(t, "0.1.12")
			externalPut(t, o.SourceRoot, "data/profile/identity.md", "original")
			reviewed, err := zipmigration.PlanExternal(o)
			if err != nil {
				t.Fatal(err)
			}
			o.Home = filepath.Join(o.UserHome, reserved)
			if _, err := zipmigration.PlanExternal(o); err == nil {
				t.Error("plan admitted missing reserved alias")
			}
			reviewed.ExternalOptions = o
			reviewed.ArchiveRoot = filepath.Join(o.UserHome, ".maestro-rollback-"+reviewed.AttemptID)
			if _, err = zipmigration.ApplyExternal(reviewed, externalDigest(t, reviewed), nil); err == nil {
				t.Error("apply admitted missing reserved alias")
			}
			entries, err := os.ReadDir(o.UserHome)
			if err != nil || len(entries) != 0 {
				t.Fatal("reserved prospective overlap changed profile", entries, err)
			}
		})
	}
}

func TestExternalDistinctCaseSensitiveRootRemainsSupported(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	o := externalFixture(t, "0.1.12")
	externalPut(t, o.SourceRoot, "data/profile/identity.md", "original")
	o.Home = filepath.Join(filepath.Dir(o.SourceRoot), "SOURCE")
	if _, err := os.Stat(o.Home); err == nil {
		t.Skip("filesystem aliases the source spelling")
	} else if !os.IsNotExist(err) {
		t.Fatal(err)
	}
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal("distinct case-sensitive root was refused", err)
	}
	if status, err := zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err != nil || !status.Current {
		t.Fatal("distinct case-sensitive root unusable", status, err)
	}
}

func TestExternalRollbackRetainedPathsUseArchive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	o := externalFixture(t, "0.1.12")
	externalPut(t, o.SourceRoot, "data/workspaces/project/note.md", "retained namespace")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	digest := externalDigest(t, p)
	if _, err = zipmigration.ApplyExternal(p, digest, nil); err != nil {
		t.Fatal(err)
	}
	stop := errors.New("interrupted after archive")
	for _, operation := range []string{"interrupt", "inspect-revoked", "rollback", "repeat", "inspect"} {
		var status zipmigration.ExternalStatus
		if operation == "inspect" || operation == "inspect-revoked" {
			status, err = zipmigration.InspectExternal(o.Home, p.AttemptID, digest)
		} else if operation == "interrupt" {
			status, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, func(progress zipmigration.ExternalProgress) error {
				if progress.Phase == "archived" {
					return stop
				}
				return nil
			})
		} else {
			status, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, nil)
		}
		if operation == "interrupt" && errors.Is(err, stop) {
			err = nil
		}
		if err != nil || status.Current || status.RestoredRuntime {
			t.Errorf("%s: %+v %v", operation, status, err)
			continue
		}
		retained := status.RetainedPaths["workspaces"]
		want := filepath.Join(p.ArchiveRoot, "lineage", p.AttemptID, "data/workspaces")
		body, readErr := os.ReadFile(filepath.Join(retained, "project/note.md"))
		if retained != want || readErr != nil || string(body) != "retained namespace" {
			t.Errorf("%s returned unreadable archived lineage: %q %q %v", operation, retained, body, readErr)
		}
	}
}

func TestExternalReservedAuthorityAliasesAreInactive(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	o := externalFixture(t, "0.2.0")
	names := []string{"memory/workspaces/a/.ATTESTATION/capture-v2.key", "memory/.DREAM-REQUESTED", "ACCOUNTS/a/CASES/c/OWNER/MAESTRO/dispatch/current.json"}
	for _, name := range names {
		externalPut(t, o.SourceRoot, "brain/"+name, "old authority")
	}
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(o.Home, "brain", name)); !os.IsNotExist(err) {
			t.Error("reserved alias retained active authority", name)
		}
		b, err := os.ReadFile(filepath.Join(o.Home, "lineage", p.AttemptID, "brain", name))
		if err != nil || string(b) != "old authority" {
			t.Error("reserved alias was not retained inactive", name, err)
		}
	}
}

func TestExternalProofIsSelfContainedAndStrict(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	o := externalFixture(t, "0.2.0")
	externalPut(t, o.SourceRoot, "brain/owner/identity.md", "owner")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	digest := externalDigest(t, p)
	if _, err = zipmigration.ApplyExternal(p, digest, nil); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(o.SourceRoot, o.SourceRoot+"-offline"); err != nil {
		t.Fatal(err)
	}
	if err = os.Rename(o.ProductRoot, o.ProductRoot+"-offline"); err != nil {
		t.Fatal(err)
	}
	// Retired roots can change independently after commit; inspection must not
	// acquire identity or permission facts from either historical pathname.
	if err = os.Symlink(o.SourceRoot+"-offline", o.SourceRoot); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(o.ProductRoot+"-offline", o.ProductRoot); err != nil {
		t.Fatal(err)
	}
	status, err := zipmigration.InspectExternal(o.Home, p.AttemptID, digest)
	if err != nil || !status.Current {
		t.Fatalf("proof depends on original: %+v %v", status, err)
	}
	commit := filepath.Join(o.Home, "brain/.maestro/external-migration", p.AttemptID, "commit.json")
	body, err := os.ReadFile(commit)
	if err != nil {
		t.Fatal(err)
	}
	for _, invalid := range [][]byte{[]byte(`{"schema_version":1,"schema_version":1}`), []byte(`null`), []byte(`{"schema_version":1}`)} {
		if err = os.WriteFile(commit, invalid, 0600); err != nil {
			t.Fatal(err)
		}
		if state, err := zipmigration.InspectExternal(o.Home, p.AttemptID, digest); err == nil || state.Current {
			t.Fatal("bad proof admitted", state, err)
		}
	}
	if err = os.WriteFile(commit, body, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, nil); err == nil {
		t.Fatal("missing original revoked active HOME")
	}
	status, err = zipmigration.InspectExternal(o.Home, p.AttemptID, digest)
	if err != nil || !status.Current {
		t.Fatal("failed rollback changed authority")
	}
	// Even a digest-concordant document cannot omit a required zero-valued field.
	planName := filepath.Join(o.Home, "brain/.maestro/external-migration", p.AttemptID, "plan.json")
	planBody, err := os.ReadFile(planName)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err = json.Unmarshal(planBody, &object); err != nil {
		t.Fatal(err)
	}
	delete(object, "prior_home")
	altered, _ := json.Marshal(object)
	if err = os.WriteFile(planName, altered, 0600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(altered)
	if state, err := zipmigration.InspectExternal(o.Home, p.AttemptID, hex.EncodeToString(sum[:])); err == nil || state.Current {
		t.Fatal("missing required field admitted")
	}
}

func TestExternalForeignLocatorDoesNotRevokeOrMoveHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	o := externalFixture(t, "0.1.12")
	externalPut(t, o.SourceRoot, "data/profile/note.md", "preserve")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	digest := externalDigest(t, p)
	if _, err = zipmigration.ApplyExternal(p, digest, nil); err != nil {
		t.Fatal(err)
	}
	locator := filepath.Join(o.UserHome, ".maestro-home.json")
	body, err := os.ReadFile(locator)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(locator, []byte("foreign locator\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, nil); err == nil {
		t.Fatal("foreign locator accepted")
	}
	if _, err = os.Stat(p.ArchiveRoot); !os.IsNotExist(err) {
		t.Fatal("conflict moved HOME")
	}
	if err = os.WriteFile(locator, body, 0600); err != nil {
		t.Fatal(err)
	}
	if state, err := zipmigration.InspectExternal(o.Home, p.AttemptID, digest); err != nil || !state.Current {
		t.Fatal("conflict revoked valid authority")
	}
}

func TestExternalPreflightConflictsHaveNoProfileOrSourceEffects(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	for _, conflict := range []string{"populated", "foreign-locator", "hardlink-source", "alias-home", "stale-source", "intent-overlap"} {
		t.Run(conflict, func(t *testing.T) {
			o := externalFixture(t, "0.1.12")
			externalPut(t, o.SourceRoot, "data/profile/identity.md", "untouched")
			if conflict == "hardlink-source" {
				if err := os.Link(filepath.Join(o.SourceRoot, "data/profile/identity.md"), filepath.Join(o.SourceRoot, "data/alias.md")); err != nil {
					t.Fatal(err)
				}
				if _, err := zipmigration.PlanExternal(o); err == nil {
					t.Fatal("source alias accepted")
				}
				return
			}
			if conflict == "intent-overlap" {
				o.SourceRoot = filepath.Join(o.UserHome, ".maestro-external-migration", "source")
				o.ProductRoot = o.SourceRoot
				if err := os.MkdirAll(o.SourceRoot, 0700); err != nil {
					t.Fatal(err)
				}
				externalPut(t, o.SourceRoot, "VERSION", "0.1.12\n")
				externalPut(t, o.SourceRoot, "data/profile/a.md", "safe")
				if _, err := zipmigration.PlanExternal(o); err == nil {
					t.Fatal("control overlap accepted")
				}
				return
			}
			p, err := zipmigration.PlanExternal(o)
			if err != nil {
				t.Fatal(err)
			}
			switch conflict {
			case "populated":
				externalPut(t, o.Home, "personal.md", "foreign destination")
			case "foreign-locator":
				externalPut(t, o.UserHome, ".maestro-home.json", "foreign selection")
			case "alias-home":
				if err = os.Symlink(o.SourceRoot, o.Home); err != nil {
					t.Fatal(err)
				}
			case "stale-source":
				externalPut(t, o.SourceRoot, "data/profile/identity.md", "new source revision")
			}
			if _, err = zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err == nil {
				t.Fatal("conflict accepted")
			}
			if _, err = os.Stat(filepath.Join(o.UserHome, ".maestro-external-migration")); !os.IsNotExist(err) {
				t.Fatal("failed preflight created profile control")
			}
			want := "untouched"
			if conflict == "stale-source" {
				want = "new source revision"
			}
			b, err := os.ReadFile(filepath.Join(o.SourceRoot, "data/profile/identity.md"))
			if err != nil || string(b) != want {
				t.Fatal("source changed")
			}
		})
	}
}

func TestExternalConflictingPartialAndConcurrentAttemptStayOwned(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	o := externalFixture(t, "0.1.11")
	externalPut(t, o.SourceRoot, "data/profile/note.md", "source")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	digest := externalDigest(t, p)
	_, err = zipmigration.ApplyExternal(p, digest, func(event zipmigration.ExternalProgress) error {
		if event.Phase == "pending" {
			if _, nestedErr := zipmigration.ApplyExternal(p, digest, nil); nestedErr == nil {
				t.Fatal("concurrent attempt admitted")
			}
			return errors.New("stopped")
		}
		return nil
	})
	if err == nil {
		t.Fatal("stop ignored")
	}
	externalPut(t, o.Home, "brain/owner/note.md", "unowned partial change")
	if _, err = zipmigration.ApplyExternal(p, digest, nil); err == nil {
		t.Fatal("conflicting partial overwritten")
	}
	b, err := os.ReadFile(filepath.Join(o.Home, "brain/owner/note.md"))
	if err != nil || string(b) != "unowned partial change" {
		t.Fatal("partial work lost")
	}
	if state, err := zipmigration.InspectExternal(o.Home, p.AttemptID, digest); err == nil || state.Current {
		t.Fatal("partial home admitted")
	}
}

func TestExternalWindowsMutationRefusesBeforeEffects(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows refusal boundary")
	}
	o := externalFixture(t, "0.1.12")
	externalPut(t, o.SourceRoot, "data/profile/note.md", "source")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err == nil {
		t.Fatal("Windows mutation admitted")
	}
	if _, err = os.Stat(o.Home); !os.IsNotExist(err) {
		t.Fatal("Windows refusal created HOME")
	}
}

func TestExternalRetirementIsFixedDenyOnlyAndInterruptedRollbackResumes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	o := externalFixture(t, "0.1.12")
	externalPut(t, o.SourceRoot, "data/profile/identity.md", "preserve")
	if retired, err := zipmigration.RetiredExternalHome(o.UserHome, o.Home); err != nil || retired {
		t.Fatal("fresh home retired", retired, err)
	}
	if _, err := os.Stat(filepath.Join(o.UserHome, ".maestro-external-migration")); !os.IsNotExist(err) {
		t.Fatal("retirement read created control state")
	}
	for i := 0; i < 2; i++ {
		p, err := zipmigration.PlanExternal(o)
		if err != nil {
			t.Fatal(err)
		}
		digest := externalDigest(t, p)
		if _, err = zipmigration.ApplyExternal(p, digest, nil); err != nil {
			t.Fatal(err)
		}
		_, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, func(event zipmigration.ExternalProgress) error {
			if event.Phase == "retired" {
				return errors.New("operator interrupted")
			}
			return nil
		})
		if err == nil {
			t.Fatal("stop ignored")
		}
		if retired, err := zipmigration.RetiredExternalHome(o.UserHome, o.Home); err == nil || retired {
			t.Fatal("incomplete retirement not closed", retired, err)
		}
		if _, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, nil); err != nil {
			t.Fatal("rollback resume", err)
		}
		if retired, err := zipmigration.RetiredExternalHome(o.UserHome, o.Home); err != nil || !retired {
			t.Fatal("retired binding absent", retired, err)
		}
	}
	homeHash := sha256.Sum256([]byte(o.Home))
	pointer := filepath.Join(o.UserHome, ".maestro-external-migration/retired", hex.EncodeToString(homeHash[:])+".json")
	if err := os.WriteFile(pointer, []byte(`{"schema_version":1}`), 0600); err != nil {
		t.Fatal(err)
	}
	if retired, err := zipmigration.RetiredExternalHome(o.UserHome, o.Home); err == nil || retired {
		t.Fatal("malformed retirement treated as fresh", retired, err)
	}
}

func TestExternalCaseAuthorityIsArchivedOutsideActiveReaders(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	o := externalFixture(t, "0.2.0")
	for _, name := range []string{"accounts/a/cases/c/.maestro/daily-generation", "accounts/a/cases/c/memory/.dream-requested", "accounts/a/cases/c/owner/maestro/dispatch/current.json", "memory/workspaces/id/.attestation/capture-v2.key"} {
		externalPut(t, o.SourceRoot, "brain/"+name, "historical authority")
	}
	externalPut(t, o.SourceRoot, "brain/accounts/a/cases/c/notes.md", "live case notes")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = zipmigration.ApplyExternal(p, externalDigest(t, p), nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"accounts/a/cases/c/.maestro/daily-generation", "accounts/a/cases/c/memory/.dream-requested", "accounts/a/cases/c/owner/maestro/dispatch/current.json", "memory/workspaces/id/.attestation/capture-v2.key"} {
		if _, err = os.Stat(filepath.Join(o.Home, "brain", name)); !os.IsNotExist(err) {
			t.Fatal("old control remained active", name)
		}
		b, err := os.ReadFile(filepath.Join(o.Home, "lineage", p.AttemptID, "brain", name))
		if err != nil || string(b) != "historical authority" {
			t.Fatal("old control lost", name, err)
		}
	}
	b, err := os.ReadFile(filepath.Join(o.Home, "brain/accounts/a/cases/c/notes.md"))
	if err != nil || string(b) != "live case notes" {
		t.Fatal("authored case content lost")
	}
}

func TestExternalInterruptionAndRollbackRetainNewWork(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	for _, phase := range []string{"prepared", "pending", "copying", "copied", "committed"} {
		t.Run(phase, func(t *testing.T) {
			o := externalFixture(t, "0.1.11")
			externalPut(t, o.SourceRoot, "data/profile/note.md", "before")
			p, err := zipmigration.PlanExternal(o)
			if err != nil {
				t.Fatal(err)
			}
			digest := externalDigest(t, p)
			_, err = zipmigration.ApplyExternal(p, digest, func(p zipmigration.ExternalProgress) error {
				if p.Phase == phase {
					return errors.New("operator stopped")
				}
				return nil
			})
			if err == nil {
				t.Fatal("stop ignored")
			}
			status, err := zipmigration.ApplyExternal(p, digest, nil)
			if err != nil || !status.Current {
				t.Fatalf("restart: %+v %v", status, err)
			}
			externalPut(t, o.Home, "brain/owner/note.md", "after migration")
			_, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, func(p zipmigration.ExternalProgress) error {
				if p.Phase == "archived" {
					return errors.New("operator stopped")
				}
				return nil
			})
			if err == nil {
				t.Fatal("rollback stop ignored")
			}
			status, err = zipmigration.RollbackExternal(o.Home, p.AttemptID, digest, nil)
			if err != nil || status.State != "rolled_back" || status.Current || status.RestoredRuntime {
				t.Fatalf("rollback: %+v %v", status, err)
			}
			b, err := os.ReadFile(filepath.Join(status.ArchiveRoot, "brain/owner/note.md"))
			if err != nil || string(b) != "after migration" {
				t.Fatalf("new work lost: %q %v", b, err)
			}
			b, err = os.ReadFile(filepath.Join(o.SourceRoot, "data/profile/note.md"))
			if err != nil || string(b) != "before" {
				t.Fatal("original changed")
			}
			if _, err = os.Stat(filepath.Join(o.UserHome, ".maestro-home.json")); !os.IsNotExist(err) {
				t.Fatal("owned locator remains")
			}
			if _, err = os.Stat(o.Home); !os.IsNotExist(err) {
				t.Fatal("revoked default home blocks legacy selection")
			}
		})
	}
}

func externalFixture(t *testing.T, version string) zipmigration.ExternalOptions {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	o := zipmigration.ExternalOptions{SourceRoot: filepath.Join(base, "source"), ProductRoot: filepath.Join(base, "product"), UserHome: filepath.Join(base, "profile"), Home: filepath.Join(base, "profile", ".maestro")}
	for _, p := range []string{o.SourceRoot, o.ProductRoot, o.UserHome} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	externalPut(t, o.SourceRoot, "VERSION", version+"\n")
	return o
}

func externalDigest(t *testing.T, p zipmigration.ExternalPlan) string {
	t.Helper()
	b, err := p.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TestExternalLegacyAuthoredStateAndInactiveAuthority(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	o := externalFixture(t, "0.1.12")
	externalPut(t, o.SourceRoot, "data/profile/identity.md", "original identity\n")
	externalPut(t, o.SourceRoot, "data/workspaces/project/note.md", "retained namespace\n")
	externalPut(t, o.SourceRoot, "data/.maestro/runtime/observed.json", "historical receipt\n")
	p, err := zipmigration.PlanExternal(o)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(o.Home); !os.IsNotExist(err) {
		t.Fatal("plan created HOME")
	}
	digest := externalDigest(t, p)
	status, err := zipmigration.ApplyExternal(p, digest, nil)
	if err != nil || !status.Current {
		t.Fatalf("apply: %+v %v", status, err)
	}
	for rel, want := range map[string]string{"brain/owner/identity.md": "original identity\n", "lineage/" + p.AttemptID + "/data/workspaces/project/note.md": "retained namespace\n", "lineage/" + p.AttemptID + "/data/.maestro/runtime/observed.json": "historical receipt\n"} {
		b, e := os.ReadFile(filepath.Join(o.Home, rel))
		if e != nil || string(b) != want {
			t.Fatalf("%s: %q %v", rel, b, e)
		}
	}
	if _, err = os.Stat(filepath.Join(o.Home, "brain/.maestro/runtime/observed.json")); !os.IsNotExist(err) {
		t.Fatal("historical authority became live")
	}
	externalPut(t, o.Home, "brain/owner/identity.md", "new owner edit\n")
	status, err = zipmigration.ApplyExternal(p, digest, nil)
	if err != nil || !status.Current || status.State != "already_migrated" {
		t.Fatalf("repeat %+v %v", status, err)
	}
	b, _ := os.ReadFile(filepath.Join(o.Home, "brain/owner/identity.md"))
	if string(b) != "new owner edit\n" {
		t.Fatal("repeat overwrote new work")
	}
	b, _ = os.ReadFile(filepath.Join(o.SourceRoot, "data/profile/identity.md"))
	if string(b) != "original identity\n" {
		t.Fatal("source changed")
	}
}

func externalPut(t *testing.T, root, name, body string) {
	t.Helper()
	p := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}
