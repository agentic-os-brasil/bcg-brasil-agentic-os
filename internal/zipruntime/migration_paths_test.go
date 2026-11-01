package zipruntime_test

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipmigration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipruntime"
)

func migrationPathsFixture(t *testing.T) (zipmigration.ExternalPlan, string, zipruntime.Options) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	source, user := filepath.Join(base, "old product"), filepath.Join(base, "user")
	for _, path := range []string{source, user} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	migrationPathsPut(t, source, "VERSION", "0.2.0\n")
	migrationPathsPut(t, source, "brain/owner/note.md", "original authored note")
	plan, err := zipmigration.PlanExternal(zipmigration.ExternalOptions{SourceRoot: source, ProductRoot: source, UserHome: user, Home: filepath.Join(user, ".maestro")})
	if err != nil {
		t.Fatal(err)
	}
	body, err := plan.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return plan, fmt.Sprintf("%x", sha256.Sum256(body)), zipruntime.Options{UserHome: user, LookupEnv: func(string) (string, bool) { return "", false }}
}

func migrationPathsPut(t *testing.T, root, relative, body string) {
	t.Helper()
	name := filepath.Join(root, relative)
	if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
}

func migrationPathsSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		result[rel] = info.Mode().String()
		if !entry.IsDir() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			result[rel] += string(body)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestMigratedPathsAcceptPhysicalSourceCaseIdentity(t *testing.T) {
	plan, digest, options := migrationPathsFixture(t)
	alternate := filepath.Join(filepath.Dir(plan.SourceRoot), strings.ToUpper(filepath.Base(plan.SourceRoot)))
	originalInfo, err := os.Stat(plan.SourceRoot)
	if err != nil {
		t.Fatal(err)
	}
	alternateInfo, err := os.Stat(alternate)
	if os.IsNotExist(err) {
		t.Skip("fixture filesystem distinguishes uppercase and lowercase directory names")
	}
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(originalInfo, alternateInfo) {
		t.Fatal("alternate fixture spelling names a different directory")
	}
	if status, err := zipmigration.ApplyExternal(plan, digest, nil); err != nil || !status.Current {
		t.Fatalf("apply: %+v %v", status, err)
	}
	before := migrationPathsSnapshot(t, filepath.Dir(plan.SourceRoot))
	for _, product := range []string{plan.SourceRoot, alternate} {
		if paths := zipruntime.ResolvePaths(product, options); !paths.Writable || paths.Mode != "external" || paths.OwnerScopeRoot != plan.Home {
			t.Fatalf("physical source spelling %q rejected: %+v", product, paths)
		}
	}
	if after := migrationPathsSnapshot(t, filepath.Dir(plan.SourceRoot)); !reflect.DeepEqual(before, after) {
		t.Fatal("source identity resolution changed retained state")
	}
}

func TestMigratedPathsRetainSourceAndContinueAcrossProducts(t *testing.T) {
	plan, digest, options := migrationPathsFixture(t)
	original := migrationPathsSnapshot(t, plan.SourceRoot)
	if status, err := zipmigration.ApplyExternal(plan, digest, nil); err != nil || !status.Current {
		t.Fatalf("apply: %+v %v", status, err)
	}
	if err := os.Chmod(plan.SourceRoot, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(plan.SourceRoot, 0700) })
	for i := 0; i < 2; i++ {
		paths := zipruntime.ResolvePaths(plan.SourceRoot, options)
		if !paths.Writable || paths.Mode != "external" || paths.OwnerScopeRoot != plan.Home {
			t.Fatalf("retained original conflicts with migrated HOME: %+v", paths)
		}
	}
	if err := os.Chmod(plan.SourceRoot, 0700); err != nil {
		t.Fatal(err)
	}
	next := filepath.Join(filepath.Dir(plan.SourceRoot), "new product")
	if err := os.Mkdir(next, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(next, 0500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(next, 0700) })
	migrationPathsPut(t, plan.Home, "brain/owner/note.md", "edited after migration")
	if status, err := zipmigration.ApplyExternal(plan, digest, nil); err != nil || !status.Current {
		t.Fatalf("repeat: %+v %v", status, err)
	}
	var out bytes.Buffer
	if err := zipruntime.RunWithOptions([]string{"context-cap", "--root", next, "--max", "100"}, strings.NewReader("owner context"), &out, options); err != nil || out.String() != "owner context" {
		t.Fatalf("next product: %s %v", &out, err)
	}
	if _, err := os.Stat(filepath.Join(plan.Home, "brain/.maestro/context-packet.json")); err != nil {
		t.Fatal(err)
	}
	if got := migrationPathsSnapshot(t, plan.SourceRoot); !reflect.DeepEqual(original, got) {
		t.Fatal("migration or runtime changed original")
	}
	if entries, err := os.ReadDir(next); err != nil || len(entries) != 0 {
		t.Fatalf("runtime changed product: %v %v", entries, err)
	}
	if body, err := os.ReadFile(filepath.Join(plan.Home, "brain/owner/note.md")); err != nil || string(body) != "edited after migration" {
		t.Fatalf("new work overwritten: %q %v", body, err)
	}
	migrationPathsPut(t, plan.SourceRoot, "data/profile/retired.md", "retired layout may drift")
	if paths := zipruntime.ResolvePaths(plan.SourceRoot, options); !paths.Writable || paths.Mode != "external" {
		t.Fatalf("retired v1 drift stranded HOME: %+v", paths)
	}
	if err := os.RemoveAll(plan.SourceRoot); err != nil {
		t.Fatal(err)
	}
	if paths := zipruntime.ResolvePaths(next, options); !paths.Writable || paths.Mode != "external" {
		t.Fatalf("offline original stranded HOME: %+v", paths)
	}
	if err := os.Chmod(next, 0700); err != nil {
		t.Fatal(err)
	}
	migrationPathsPut(t, next, "brain/owner/unrelated.md", "separate owner state")
	if paths := zipruntime.ResolvePaths(next, options); paths.Writable || paths.Diagnostic != "legacy-home-conflict" {
		t.Fatalf("unrelated legacy admitted: %+v", paths)
	}
}

func TestUnverifiedMigrationPathsHaveNoRuntimeEffects(t *testing.T) {
	for _, scenario := range []string{"pending", "plan-tamper", "commit-tamper", "revoked"} {
		t.Run(scenario, func(t *testing.T) {
			plan, digest, options := migrationPathsFixture(t)
			var stop func(zipmigration.ExternalProgress) error
			if scenario == "pending" {
				stop = func(progress zipmigration.ExternalProgress) error {
					if progress.Phase == "pending" {
						return errors.New("operator stopped")
					}
					return nil
				}
			}
			_, err := zipmigration.ApplyExternal(plan, digest, stop)
			if (err != nil) != (scenario == "pending") {
				t.Fatalf("apply: %v", err)
			}
			switch scenario {
			case "plan-tamper":
				migrationPathsPut(t, plan.Home, "brain/.maestro/external-migration/"+plan.AttemptID+"/plan.json", "{}\n")
			case "commit-tamper":
				migrationPathsPut(t, plan.Home, "brain/.maestro/external-migration/"+plan.AttemptID+"/commit.json", "{}\n")
			case "revoked":
				_, err := zipmigration.RollbackExternal(plan.Home, plan.AttemptID, digest, func(progress zipmigration.ExternalProgress) error {
					if progress.Phase == "revoked" {
						return errors.New("operator stopped")
					}
					return nil
				})
				if err == nil {
					t.Fatal("rollback stop ignored")
				}
			}
			root := filepath.Dir(plan.SourceRoot)
			before := migrationPathsSnapshot(t, root)
			if paths := zipruntime.ResolvePaths(plan.SourceRoot, options); paths.Writable || paths.Mode != "blocked" {
				t.Fatalf("unverified migration admitted: %+v", paths)
			}
			for _, args := range [][]string{{"context-cap", "--root", plan.SourceRoot, "--max", "100"}, {"daily-stop", "--root", plan.SourceRoot}, {"codex-hook", "SessionStart", "--root", plan.SourceRoot}} {
				var out bytes.Buffer
				_ = zipruntime.RunWithOptions(args, strings.NewReader(`{"session_id":"isolated"}`), &out, options)
			}
			if after := migrationPathsSnapshot(t, root); !reflect.DeepEqual(before, after) {
				t.Fatal("unverified migration changed owner or original state")
			}
		})
	}
}

func TestMigrationRollbackRequiresActualHomeSelection(t *testing.T) {
	plan, digest, options := migrationPathsFixture(t)
	if status, err := zipmigration.ApplyExternal(plan, digest, nil); err != nil || !status.Current {
		t.Fatalf("apply: %+v %v", status, err)
	}
	migrationPathsPut(t, plan.Home, "brain/owner/note.md", "new work retained")
	status, err := zipmigration.RollbackExternal(plan.Home, plan.AttemptID, digest, nil)
	if err != nil || status.Current || status.RestoredRuntime {
		t.Fatalf("rollback: %+v %v", status, err)
	}
	if paths := zipruntime.ResolvePaths(plan.SourceRoot, options); !paths.Writable || paths.Mode != "legacy" {
		t.Fatalf("default selection cannot use preserved legacy: %+v", paths)
	}
	options.LookupEnv = func(key string) (string, bool) { return plan.Home, key == "MAESTRO_HOME" }
	next := filepath.Join(filepath.Dir(plan.SourceRoot), "new product")
	if err := os.Mkdir(next, 0700); err != nil {
		t.Fatal(err)
	}
	before := migrationPathsSnapshot(t, filepath.Dir(plan.SourceRoot))
	for _, product := range []string{plan.SourceRoot, next} {
		if paths := zipruntime.ResolvePaths(product, options); paths.Writable || paths.Mode != "blocked" {
			t.Fatalf("inherited override silently changed binding: %+v", paths)
		}
	}
	if _, err := datahome.InitializeFresh(datahome.Options{UserHome: plan.UserHome, ProductRoot: next, LookupEnv: options.LookupEnv}); err == nil {
		t.Fatal("inherited retired binding was adopted as fresh")
	}
	if body, err := os.ReadFile(filepath.Join(status.ArchiveRoot, "brain/owner/note.md")); err != nil || string(body) != "new work retained" {
		t.Fatalf("rollback lost work: %q %v", body, err)
	}
	if after := migrationPathsSnapshot(t, filepath.Dir(plan.SourceRoot)); !reflect.DeepEqual(before, after) {
		t.Fatal("ordinary selection repaired rollback state")
	}
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(plan.Home)))
	migrationPathsPut(t, plan.UserHome, ".maestro-external-migration/retired/"+key+".json", "{}\n")
	if paths := zipruntime.ResolvePaths(next, options); paths.Writable || paths.Mode != "blocked" || paths.Diagnostic != "migration-unverified" {
		t.Fatalf("malformed retirement record fell back to fresh: %+v", paths)
	}
	if _, err := datahome.InitializeFresh(datahome.Options{UserHome: plan.UserHome, ProductRoot: next, LookupEnv: func(string) (string, bool) { return "", false }}); err != nil {
		t.Fatal(err)
	}
	if paths := zipruntime.ResolvePaths(next, options); !paths.Writable || paths.Mode != "external" {
		t.Fatalf("new confirmed origin was vetoed by historical retirement: %+v", paths)
	}
}

func TestNewCustomHomeStillRequiresExplicitInitialization(t *testing.T) {
	plan, _, options := migrationPathsFixture(t)
	product := filepath.Join(filepath.Dir(plan.SourceRoot), "fresh product")
	if err := os.Mkdir(product, 0700); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(plan.UserHome, "new custom HOME")
	options.LookupEnv = func(key string) (string, bool) { return home, key == "MAESTRO_HOME" }
	paths := zipruntime.ResolvePaths(product, options)
	if paths.Writable || paths.Mode != "initialization_required" {
		t.Fatalf("new explicit HOME lost fresh bootstrap: %+v", paths)
	}
	if _, err := datahome.InitializeFresh(datahome.Options{UserHome: plan.UserHome, ProductRoot: product, LookupEnv: options.LookupEnv}); err != nil {
		t.Fatal(err)
	}
	if paths := zipruntime.ResolvePaths(product, options); !paths.Writable || paths.Mode != "external" {
		t.Fatalf("explicit fresh init did not admit custom HOME: %+v", paths)
	}
}
