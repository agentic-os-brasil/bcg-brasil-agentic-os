package datahome_test

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
)

func TestInitializeFreshCreatesReadyHome(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(map[bool]string{false: "default", true: "nondefault with spaces"}[custom], func(t *testing.T) {
			opts := fixture(t)
			if custom {
				selected := filepath.Join(opts.UserHome, "state with spaces")
				opts.LookupEnv = func(string) (string, bool) { return selected, true }
			}
			got, err := datahome.InitializeFresh(opts)
			if runtime.GOOS == "windows" {
				if err == nil || got.Writable || got.Diagnostic != "ownership-unverified" {
					t.Fatalf("Windows must fail closed: %+v %v", got, err)
				}
				entries, _ := os.ReadDir(opts.UserHome)
				if len(entries) != 0 {
					t.Fatal("unverified bootstrap wrote state")
				}
				return
			}
			if err != nil || !got.Writable || got.Diagnostic != "ready" {
				t.Fatalf("bootstrap: %+v %v", got, err)
			}
			origin := filepath.Join(got.BrainRoot, ".maestro", "origin.json")
			before, err := os.Stat(origin)
			if err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(origin)
			if err != nil {
				t.Fatal(err)
			}
			again, err := datahome.InitializeFresh(opts)
			if err != nil || !again.Writable {
				t.Fatalf("idempotent: %+v %v", again, err)
			}
			after, _ := os.Stat(origin)
			afterBody, _ := os.ReadFile(origin)
			if !os.SameFile(before, after) || before.ModTime() != after.ModTime() || string(body) != string(afterBody) {
				t.Fatal("idempotent bootstrap rewrote origin")
			}
			opts.LookupEnv = func(string) (string, bool) { return "", false }
			reopened, err := datahome.Resolve(opts)
			if err != nil || !reopened.Writable || reopened.Home != got.Home {
				t.Fatalf("reopen: %+v %v", reopened, err)
			}
			for _, path := range []string{got.Home, got.BrainRoot, filepath.Dir(origin)} {
				info, err := os.Stat(path)
				if err != nil || info.Mode().Perm() != 0700 {
					t.Fatalf("directory mode: %s %v %v", path, info, err)
				}
			}
			if after.Mode().Perm() != 0600 {
				t.Fatal("origin must be private")
			}
			if custom {
				info, err := os.Stat(filepath.Join(opts.UserHome, ".maestro-home.json"))
				if err != nil || info.Mode().Perm() != 0600 {
					t.Fatalf("locator: %v %v", info, err)
				}
			}
		})
	}
}

func TestInitializeFreshRefusesExistingAmbiguousStateWithoutChanges(t *testing.T) {
	for _, scenario := range []string{"populated", "empty-home", "origin-conflict", "migrated", "locator-conflict", "locator-missing-origin", "symlink", "ancestor-symlink", "public-home"} {
		t.Run(scenario, func(t *testing.T) {
			opts := fixture(t)
			home := filepath.Join(opts.UserHome, ".maestro")
			switch scenario {
			case "populated":
				record(t, filepath.Join(home, "keep.json"), map[string]string{"keep": "original"})
			case "empty-home":
				if err := os.Mkdir(home, 0700); err != nil {
					t.Fatal(err)
				}
			case "origin-conflict":
				record(t, filepath.Join(home, "brain", ".maestro", "origin.json"), map[string]any{"schema_version": 1, "home": opts.UserHome, "origin": "fresh-confirmed", "attempt_id": "", "plan_sha256": ""})
			case "migrated":
				record(t, filepath.Join(home, "brain", ".maestro", "origin.json"), map[string]any{"schema_version": 1, "home": home, "origin": "migrated", "attempt_id": "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "plan_sha256": "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"})
			case "locator-conflict":
				confirm(t, home)
				record(t, filepath.Join(opts.UserHome, ".maestro-home.json"), map[string]any{"schema_version": 1, "home": home, "attempt_id": ""})
				selected := filepath.Join(opts.UserHome, "different")
				opts.LookupEnv = func(string) (string, bool) { return selected, true }
			case "locator-missing-origin":
				if err := os.Mkdir(home, 0700); err != nil {
					t.Fatal(err)
				}
				record(t, filepath.Join(opts.UserHome, ".maestro-home.json"), map[string]any{"schema_version": 1, "home": home, "attempt_id": ""})
			case "symlink", "ancestor-symlink":
				if runtime.GOOS == "windows" {
					t.Skip("native reparse qualification separate")
				}
				outside := filepath.Join(opts.UserHome, "outside")
				if err := os.Mkdir(outside, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, home); err != nil {
					t.Fatal(err)
				}
				if scenario == "ancestor-symlink" {
					selected := filepath.Join(home, "new-home")
					opts.LookupEnv = func(string) (string, bool) { return selected, true }
				}
			case "public-home":
				if runtime.GOOS == "windows" {
					t.Skip("Unix modes")
				}
				if err := os.Mkdir(home, 0755); err != nil {
					t.Fatal(err)
				}
			}
			before := homeSnapshot(t, opts.UserHome)
			got, err := datahome.InitializeFresh(opts)
			if scenario == "empty-home" && runtime.GOOS != "windows" {
				if err != nil || !got.Writable {
					t.Fatalf("empty home: %+v %v", got, err)
				}
				return
			}
			if err == nil || got.Writable {
				t.Fatalf("unsafe state promoted: %+v %v", got, err)
			}
			after := homeSnapshot(t, opts.UserHome)
			if !reflect.DeepEqual(before, after) {
				t.Fatalf("refused bootstrap changed original tree: before=%v after=%v", before, after)
			}
		})
	}
}

func homeSnapshot(t *testing.T, root string) map[string]string {
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
		value := info.Mode().String()
		// NTFS can defer directory timestamps from fixture creation until inspection.
		if runtime.GOOS != "windows" || !info.IsDir() {
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
