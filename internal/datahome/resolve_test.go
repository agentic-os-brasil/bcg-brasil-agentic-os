package datahome_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
)

func record(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestResolveRejectsUnsafeSelectedPathsAndConflictingLocator(t *testing.T) {
	for _, scenario := range []string{"empty-env", "relative-env", "newline-env", "overlap-product", "overlap-product-case-alias", "overlap-source", "linked-home", "linked-missing-ancestor", "locator-missing", "env-marker-conflict", "populated-unconfirmed", "nondefault-no-marker"} {
		t.Run(scenario, func(t *testing.T) {
			opts := fixture(t)
			home := filepath.Join(opts.UserHome, ".maestro")
			selected := home
			switch scenario {
			case "empty-env":
				selected = ""
			case "relative-env":
				selected = "relative"
			case "newline-env":
				selected = home + "\r\n"
			case "overlap-product":
				selected = filepath.Join(opts.ProductRoot, "state")
			case "overlap-product-case-alias":
				selected = filepath.Join(filepath.Dir(opts.ProductRoot), "PRODUCT", "state")
			case "overlap-source":
				opts.SourceRoot = filepath.Join(opts.UserHome, "legacy")
				if err := os.Mkdir(opts.SourceRoot, 0700); err != nil {
					t.Fatal(err)
				}
				selected = filepath.Join(opts.SourceRoot, "state")
			case "linked-home", "linked-missing-ancestor":
				if runtime.GOOS == "windows" {
					t.Skip("native reparse tests remain separate")
				}
				outside := filepath.Join(opts.UserHome, "outside")
				if err := os.Mkdir(outside, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(outside, home); err != nil {
					t.Fatal(err)
				}
				if scenario == "linked-missing-ancestor" {
					selected = filepath.Join(home, "absent")
				}
			case "locator-missing":
				record(t, filepath.Join(opts.UserHome, ".maestro-home.json"), map[string]any{"schema_version": 1, "home": home, "attempt_id": ""})
			case "env-marker-conflict":
				confirm(t, home)
				record(t, filepath.Join(opts.UserHome, ".maestro-home.json"), map[string]any{"schema_version": 1, "home": home, "attempt_id": ""})
				selected = filepath.Join(opts.UserHome, "different")
			case "populated-unconfirmed":
				record(t, filepath.Join(home, "owner.json"), map[string]string{"keep": "original"})
			case "nondefault-no-marker":
				selected = filepath.Join(opts.UserHome, "elsewhere")
				confirm(t, selected)
			}
			opts.LookupEnv = func(string) (string, bool) { return selected, true }
			got, err := datahome.Resolve(opts)
			if err == nil || got.Writable {
				t.Fatalf("unsafe case accepted: %+v %v", got, err)
			}
			if scenario == "locator-missing" && got.Diagnostic != "marker-target-missing" {
				t.Fatalf("missing marker target fell back: %+v", got)
			}
		})
	}
}

func TestResolveRejectsMalformedRecordsEvenWithEnvironment(t *testing.T) {
	for _, body := range []string{
		`{}`, `null`, `[]`, "\xef\xbb\xbf{}", "{\"schema_version\":1,\"home\":\"/bad\xff\",\"attempt_id\":\"\"}",
		`{"schema_version":1,"home":"/missing","attempt_id":"","extra":0}`,
		`{"schema_version":1,"schema_version":1,"home":"/missing","attempt_id":""}`,
		`{"schema_version":1,"home":"/missing","attempt_id":""} {}`,
		`{"schema_version":1.0,"home":"/missing","attempt_id":""}`,
		`{"schema_version":2,"home":"/missing","attempt_id":""}`,
		`{"schema_version":1,"home":null,"attempt_id":""}`,
		`{"schema_version":1,"home":"/missing","attempt_id":"ABC"}`,
		strings.Repeat(" ", 16<<10) + `{}`,
	} {
		t.Run(fmtName(body), func(t *testing.T) {
			opts := fixture(t)
			opts.LookupEnv = func(string) (string, bool) { return filepath.Join(opts.UserHome, ".maestro"), true }
			if err := os.WriteFile(filepath.Join(opts.UserHome, ".maestro-home.json"), []byte(body), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := datahome.Resolve(opts)
			if err == nil || got.Writable || got.Diagnostic != "marker-invalid" {
				t.Fatalf("malformed marker accepted: %+v %v", got, err)
			}
		})
	}
}

func fmtName(body string) string {
	if len(body) > 90 {
		return "oversized"
	}
	return body
}

func TestResolveMigrationRemainsUnverifiedAndRecordsMustAgree(t *testing.T) {
	for _, scenario := range []string{"valid", "bad-attempt", "bad-digest", "locator-disagrees", "wrong-home", "missing-field", "null-origin"} {
		t.Run(scenario, func(t *testing.T) {
			opts := fixture(t)
			home := filepath.Join(opts.UserHome, ".maestro")
			origin := map[string]any{"schema_version": 1, "home": home, "origin": "migrated", "attempt_id": strings.Repeat("a", 32), "plan_sha256": strings.Repeat("b", 64)}
			switch scenario {
			case "bad-attempt":
				origin["attempt_id"] = "UPPER"
			case "bad-digest":
				origin["plan_sha256"] = strings.Repeat("A", 64)
			case "locator-disagrees":
				record(t, filepath.Join(opts.UserHome, ".maestro-home.json"), map[string]any{"schema_version": 1, "home": home, "attempt_id": strings.Repeat("c", 32)})
			case "wrong-home":
				origin["home"] = opts.UserHome
			case "missing-field":
				delete(origin, "attempt_id")
			case "null-origin":
				origin["origin"] = nil
			}
			record(t, filepath.Join(home, "brain", ".maestro", "origin.json"), origin)
			got, err := datahome.Resolve(opts)
			if scenario == "valid" {
				if err != nil || got.Writable || got.Diagnostic != "migration-unverified" {
					t.Fatalf("unverified migration: %+v %v", got, err)
				}
			} else if err == nil || got.Writable {
				t.Fatalf("invalid origin accepted: %+v %v", got, err)
			}
		})
	}
}

func TestResolveRejectsPubliclyWritableUserHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission contract")
	}
	opts := fixture(t)
	if err := os.Chmod(opts.UserHome, 0777); err != nil {
		t.Fatal(err)
	}
	got, err := datahome.Resolve(opts)
	if err == nil || got.Writable {
		t.Fatalf("writable userhome accepted: %+v %v", got, err)
	}
	info, err := os.Stat(opts.UserHome)
	if err != nil || info.Mode().Perm() != 0777 {
		t.Fatalf("inspection repaired permissions: %v %v", info, err)
	}
}

func confirm(t *testing.T, home string) {
	record(t, filepath.Join(home, "brain", ".maestro", "origin.json"), map[string]any{"schema_version": 1, "home": home, "origin": "fresh-confirmed", "attempt_id": "", "plan_sha256": ""})
}

func TestResolveRediscoversConfirmedNondefaultFromLocator(t *testing.T) {
	opts := fixture(t)
	home := filepath.Join(opts.UserHome, "state with spaces")
	confirm(t, home)
	record(t, filepath.Join(opts.UserHome, ".maestro-home.json"), map[string]any{"schema_version": 1, "home": home, "attempt_id": ""})
	got, err := datahome.Resolve(opts)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		if got.Writable || got.Diagnostic != "ownership-unverified" {
			t.Fatalf("unqualified Windows ownership: %+v", got)
		}
		return
	}
	if got.Home != home || got.SelectedBy != "marker" || got.Origin != "fresh-confirmed" || !got.Writable || got.Diagnostic != "ready" {
		t.Fatalf("unexpected locator result: %+v", got)
	}
}

func TestResolveCaseDistinctNondefaultCannotBypassLocator(t *testing.T) {
	opts := fixture(t)
	defaultHome, selected := caseDistinctHomes(t, opts.UserHome)
	confirm(t, selected)
	opts.LookupEnv = func(string) (string, bool) { return selected, true }
	got, err := datahome.Resolve(opts)
	if err == nil || got.Writable || got.Diagnostic != "marker-required" {
		t.Fatalf("case-distinct nondefault %q bypassed locator for %q: %+v %v", selected, defaultHome, got, err)
	}
}

func TestResolveCaseDistinctLocatorCannotAuthorizeDifferentHome(t *testing.T) {
	opts := fixture(t)
	markerHome, selected := caseDistinctHomes(t, opts.UserHome)
	confirm(t, markerHome)
	confirm(t, selected)
	record(t, filepath.Join(opts.UserHome, ".maestro-home.json"), map[string]any{"schema_version": 1, "home": markerHome, "attempt_id": ""})
	opts.LookupEnv = func(string) (string, bool) { return selected, true }
	got, err := datahome.Resolve(opts)
	if err == nil || got.Writable || got.Diagnostic != "marker-conflict" {
		t.Fatalf("locator for %q authorized distinct %q: %+v %v", markerHome, selected, got, err)
	}
}

func caseDistinctHomes(t *testing.T, userHome string) (string, string) {
	t.Helper()
	lower, upper := filepath.Join(userHome, ".maestro"), filepath.Join(userHome, ".MAESTRO")
	for _, path := range []string{lower, upper} {
		if err := os.Mkdir(path, 0700); err != nil && !os.IsExist(err) {
			t.Fatal(err)
		}
	}
	lowerInfo, err := os.Stat(lower)
	if err != nil {
		t.Fatal(err)
	}
	upperInfo, err := os.Stat(upper)
	if err != nil {
		t.Fatal(err)
	}
	if os.SameFile(lowerInfo, upperInfo) {
		t.Skip("filesystem resolves case variants to the same object")
	}
	return lower, upper
}

func fixture(t *testing.T) datahome.Options {
	t.Helper()
	root := t.TempDir()
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	user := filepath.Join(resolved, "user")
	product := filepath.Join(resolved, "product")
	for _, path := range []string{user, product} {
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return datahome.Options{UserHome: user, ProductRoot: product, LookupEnv: func(string) (string, bool) { return "", false }}
}

func TestResolveInspectsAbsentDefaultWithoutCreatingState(t *testing.T) {
	opts := fixture(t)
	got, err := datahome.Resolve(opts)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(opts.UserHome, ".maestro")
	if got.Home != want || got.BrainRoot != filepath.Join(want, "brain") || got.SelectedBy != "default" || got.Origin != "fresh-unconfirmed" || got.Writable || got.Diagnostic != "fresh-unconfirmed" {
		t.Fatalf("unexpected resolution: %+v", got)
	}
	entries, err := os.ReadDir(opts.UserHome)
	if err != nil || len(entries) != 0 {
		t.Fatalf("inspection changed userhome: %v %v", entries, err)
	}
}
