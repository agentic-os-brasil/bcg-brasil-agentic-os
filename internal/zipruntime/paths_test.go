package zipruntime_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/lifecycle"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipmigration"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/zipruntime"
)

func TestExternalHomeContinuesAcrossProductVersions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	user, first, next := filepath.Join(base, "user"), filepath.Join(base, "core N"), filepath.Join(base, "core N+1")
	for _, p := range []string{user, first, next} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	home := filepath.Join(user, "owner state")
	lookup := func(key string) (string, bool) { return home, key == "MAESTRO_HOME" }
	if _, err := datahome.InitializeFresh(datahome.Options{UserHome: user, ProductRoot: first, LookupEnv: lookup}); err != nil {
		t.Fatal(err)
	}
	invoke := func(product, command, input string, extra ...string) string {
		t.Helper()
		var out bytes.Buffer
		args := append([]string{command, "--root", product}, extra...)
		// CLI override isolates the real public Run without changing host HOME.
		args = append(args, "--user-home", user)
		if err := zipruntime.Run(args, strings.NewReader(input), &out); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	if got := invoke(first, "context-cap", "owner packet", "--max", "30000"); got != "owner packet" {
		t.Fatal(got)
	}
	if _, err := os.Stat(filepath.Join(home, "brain/.maestro/context-packet.json")); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	checkpoint := map[string]any{"schema_version": 1, "id": "across-versions", "session_id": "session", "workspace": home, "scope": "owner", "captured_at": now.Format(time.RFC3339), "local_date": now.Format("2006-01-02"), "summary": "Continue owner work across versions", "decisions": []string{}, "next_actions": []string{}, "provenance": "agent-authored"}
	q := filepath.Join(home, "brain/.maestro/daily-pending")
	if err := os.MkdirAll(q, 0700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(checkpoint)
	if err := os.WriteFile(filepath.Join(q, "across-versions.json"), b, 0600); err != nil {
		t.Fatal(err)
	}
	if got := invoke(next, "daily-stop", ""); !strings.Contains(got, `"saved":1`) {
		t.Fatal(got)
	}
	if got := invoke(next, "daily-context", ""); !strings.Contains(got, "Continue owner work across versions") {
		t.Fatal(got)
	}
	for _, event := range []string{"SessionStart", "PostToolUse", "Stop"} {
		var out bytes.Buffer
		if err := zipruntime.RunWithOptions([]string{"codex-hook", event, "--root", next}, strings.NewReader(`{"session_id":"external-session","tool_name":"Write","tool_use_id":"event"}`), &out, zipruntime.Options{UserHome: user, LookupEnv: lookup}); err != nil {
			t.Fatal(err)
		}
		if event == "SessionStart" && !strings.Contains(out.String(), home) {
			t.Fatal("context omits admitted HOME", out.String())
		}
	}
	summary, err := lifecycle.DiagnoseRuntime(filepath.Join(home, "brain/.maestro"), lifecycle.IdempotencyKey(home), "codex")
	if err != nil || summary.Observed != 2 {
		t.Fatalf("owner lifecycle identity: %+v %v", summary, err)
	}
	accounts := filepath.Join(home, "brain/accounts")
	for _, rel := range []string{"a/cases/one", "b/cases/two"} {
		if err := os.MkdirAll(filepath.Join(accounts, rel), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(accounts, ".active"), []byte("a/one"), 0600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(base, "case alias")
	if err := os.Symlink(filepath.Join(accounts, "b/cases/two"), alias); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		cwd, path string
		deny      bool
	}{
		{filepath.Join(accounts, "a/cases/one"), "allowed.md", false},
		{filepath.Join(accounts, "b/cases/two"), "forbidden.md", true},
		{base, "case alias/forbidden.md", true},
	} {
		payload, _ := json.Marshal(map[string]any{"cwd": tc.cwd, "tool_name": "Write", "tool_input": map[string]string{"file_path": tc.path}})
		var out bytes.Buffer
		if err := zipruntime.RunWithOptions([]string{"codex-hook", "PreToolUse", "--root", next}, bytes.NewReader(payload), &out, zipruntime.Options{UserHome: user, LookupEnv: lookup}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), `"permissionDecision":"deny"`) != tc.deny {
			t.Fatalf("cwd=%s target=%s output=%s", tc.cwd, tc.path, &out)
		}
	}
	for _, p := range []string{first, next} {
		entries, err := os.ReadDir(p)
		if err != nil || len(entries) != 0 {
			t.Fatalf("product changed: %s %v %v", p, entries, err)
		}
	}
}

func TestUnavailableAuthorityPreservesBothTrees(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	for _, scenario := range []string{"fresh", "legacy-source", "conflict", "malformed", "migrated", "pending", "revoked"} {
		t.Run(scenario, func(t *testing.T) {
			base, err := filepath.EvalSymlinks(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			user, product := filepath.Join(base, "user"), filepath.Join(base, "core")
			for _, p := range []string{user, product} {
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			lookup := func(string) (string, bool) { return "", false }
			put := func(p, s string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(s), 0600); err != nil {
					t.Fatal(err)
				}
			}
			home := filepath.Join(user, ".maestro")
			if scenario != "fresh" && scenario != "legacy-source" && scenario != "pending" && scenario != "revoked" {
				if _, err := datahome.InitializeFresh(datahome.Options{UserHome: user, ProductRoot: product, LookupEnv: lookup}); err != nil {
					t.Fatal(err)
				}
			}
			switch scenario {
			case "legacy-source":
				put(filepath.Join(product, "data/profile/identity.md"), "retained legacy")
			case "conflict":
				put(filepath.Join(product, "brain/owner/identity.md"), "unbound owner")
			case "malformed":
				put(filepath.Join(home, "brain/.maestro/origin.json"), "{")
			case "migrated":
				attempt := strings.Repeat("a", 32)
				origin, _ := json.Marshal(map[string]any{"schema_version": 1, "home": home, "origin": "migrated", "attempt_id": attempt, "plan_sha256": strings.Repeat("b", 64)})
				locator, _ := json.Marshal(map[string]any{"schema_version": 1, "home": home, "attempt_id": attempt})
				put(filepath.Join(home, "brain/.maestro/origin.json"), string(origin))
				put(filepath.Join(user, ".maestro-home.json"), string(locator))
			case "pending", "revoked":
				put(filepath.Join(product, "data/profile/identity.md"), "retained legacy")
				result := zipmigration.Run(product, zipmigration.Options{})
				if result.State != "committed" {
					t.Fatal(result)
				}
				if scenario == "revoked" {
					if result := zipmigration.Run(product, zipmigration.Options{Rollback: result.AttemptID}); result.State != "rolled_back" {
						t.Fatal(result)
					}
				} else {
					state := filepath.Join(product, "brain/.maestro/migration/state.json")
					b, err := os.ReadFile(state)
					if err != nil {
						t.Fatal(err)
					}
					put(state, strings.Replace(string(b), `"committed"`, `"partial"`, 1))
				}
			}
			snapshot := func() map[string]string {
				entries := map[string]string{}
				if err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
					if err != nil {
						return err
					}
					info, err := d.Info()
					if err != nil {
						return err
					}
					entries[path] = info.Mode().String()
					if !d.IsDir() {
						b, err := os.ReadFile(path)
						if err != nil {
							return err
						}
						entries[path] += string(b)
					}
					return nil
				}); err != nil {
					t.Fatal(err)
				}
				return entries
			}
			before := snapshot()
			options := zipruntime.Options{UserHome: user, LookupEnv: lookup}
			for _, args := range [][]string{{"context-cap", "--root", product, "--max", "100"}, {"daily-context", "--root", product}, {"daily-stop", "--root", product}, {"daily-dream-ack", "--root", product}, {"codex-hook", "SessionStart", "--root", product}, {"codex-hook", "PostToolUse", "--root", product}, {"codex-hook", "Stop", "--root", product}} {
				var out bytes.Buffer
				_ = zipruntime.RunWithOptions(args, strings.NewReader(`{"session_id":"test"}`), &out, options)
			}
			if after := snapshot(); !reflect.DeepEqual(before, after) {
				t.Fatal("unavailable authority changed retained trees")
			}
		})
	}
}

func TestExternalUnsafeStateBlocksBeforeContextReceipt(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("external ownership unavailable")
	}
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	user, product, outside := filepath.Join(base, "user"), filepath.Join(base, "core"), filepath.Join(base, "outside")
	for _, p := range []string{user, product, outside} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	lookup := func(string) (string, bool) { return "", false }
	home, err := datahome.InitializeFresh(datahome.Options{UserHome: user, ProductRoot: product, LookupEnv: lookup})
	if err != nil {
		t.Fatal(err)
	}
	state := filepath.Join(home.Home, "state")
	if err := os.Remove(state); err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, state); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = zipruntime.RunWithOptions([]string{"context-cap", "--root", product, "--max", "100"}, strings.NewReader("private context"), &out, zipruntime.Options{UserHome: user, LookupEnv: lookup})
	if err == nil || out.Len() != 0 {
		t.Fatalf("unsafe state admitted: %v %s", err, &out)
	}
	if _, err := os.Stat(filepath.Join(home.BrainRoot, ".maestro/context-packet.json")); !os.IsNotExist(err) {
		t.Fatal("blocked admission wrote receipt", err)
	}
}
