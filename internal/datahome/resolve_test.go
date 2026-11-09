package datahome_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
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
				if got, err := datahome.Resolve(opts); err != nil || got.Diagnostic != "fresh-unconfirmed" {
					t.Fatalf("safe HOME control refused before alias fixture: %+v %v", got, err)
				}
				outside := filepath.Join(opts.UserHome, "outside")
				if err := os.Mkdir(outside, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(outside, "keep.txt"), []byte("untouched outside bytes"), 0600); err != nil {
					t.Fatal(err)
				}
				initializeDirectoryAlias(t, home, outside)
				aliasInfo, aliasErr := os.Stat(home)
				outsideInfo, outsideErr := os.Stat(outside)
				if aliasErr != nil || outsideErr != nil || !os.SameFile(aliasInfo, outsideInfo) {
					t.Fatalf("required directory alias not established: %v %v", aliasErr, outsideErr)
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
			before := homeSnapshot(t, opts.UserHome)
			got, err := datahome.Resolve(opts)
			if err == nil || got.Writable {
				t.Fatalf("unsafe case accepted: %+v %v", got, err)
			}
			if after := homeSnapshot(t, opts.UserHome); !reflect.DeepEqual(before, after) {
				t.Fatalf("unsafe HOME inspection changed the profile or outside tree: before=%v after=%v", before, after)
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
	opts := fixture(t)
	if err := os.WriteFile(filepath.Join(opts.UserHome, "keep.txt"), []byte("preserve user home"), 0600); err != nil {
		t.Fatal(err)
	}
	if got, err := datahome.Resolve(opts); err != nil || got.Diagnostic != "fresh-unconfirmed" || got.Writable {
		t.Fatalf("safe userhome fixture was not admitted read-only: %+v %v", got, err)
	}
	if runtime.GOOS == "windows" {
		initializePublicDirectory(t, opts.UserHome)
	} else if err := os.Chmod(opts.UserHome, 0777); err != nil {
		t.Fatal(err)
	}
	before := homeSnapshot(t, opts.UserHome)
	got, err := datahome.Resolve(opts)
	if err == nil || got.Writable {
		t.Fatalf("writable userhome accepted: %+v %v", got, err)
	}
	if !reflect.DeepEqual(before, homeSnapshot(t, opts.UserHome)) {
		t.Fatal("unsafe-userhome inspection mutated content, metadata or permissions")
	}
}

func confirm(t *testing.T, home string) {
	record(t, filepath.Join(home, "brain", ".maestro", "origin.json"), map[string]any{"schema_version": 1, "home": home, "origin": "fresh-confirmed", "attempt_id": "", "plan_sha256": ""})
}

func TestResolveRediscoversConfirmedNondefaultFromLocator(t *testing.T) {
	opts := fixture(t)
	home := filepath.Join(opts.UserHome, "state with spaces")
	opts.LookupEnv = func(string) (string, bool) { return home, true }
	if _, err := datahome.InitializeFresh(opts); err != nil {
		t.Fatal(err)
	}
	opts.LookupEnv = func(string) (string, bool) { return "", false }
	root := filepath.Dir(opts.UserHome)
	before := homeSnapshot(t, root)
	got, err := datahome.Resolve(opts)
	if err != nil {
		t.Fatal(err)
	}
	if got.Home != home || got.SelectedBy != "marker" || got.Origin != "fresh-confirmed" || !got.Writable || got.Diagnostic != "ready" {
		t.Fatalf("unexpected locator result: %+v", got)
	}
	metadata := filepath.Join(got.BrainRoot, ".maestro")
	for _, path := range []string{home, got.BrainRoot, metadata, filepath.Join(metadata, "origin.json"), filepath.Join(opts.UserHome, ".maestro-home.json")} {
		initializePrivateObject(t, path)
	}
	if after := homeSnapshot(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("read-only resolution changed original tree: before=%v after=%v", before, after)
	}
}

func TestResolveCaseDistinctNondefaultCannotBypassLocator(t *testing.T) {
	opts := fixture(t)
	defaultHome, selected, distinct := caseDistinctHomes(t, opts.UserHome)
	if !distinct {
		t.Run("same-object-case-contract", func(t *testing.T) { assertCaseAliasLocatorContract(t, opts, defaultHome, selected) })
		t.Skip("filesystem resolves case variants to the same object")
	}
	confirm(t, selected)
	opts.LookupEnv = func(string) (string, bool) { return selected, true }
	got, err := datahome.Resolve(opts)
	if err == nil || got.Writable || got.Diagnostic != "marker-required" {
		t.Fatalf("case-distinct nondefault %q bypassed locator for %q: %+v %v", selected, defaultHome, got, err)
	}
}

func TestResolveCaseDistinctLocatorCannotAuthorizeDifferentHome(t *testing.T) {
	opts := fixture(t)
	markerHome, selected, distinct := caseDistinctHomes(t, opts.UserHome)
	if !distinct {
		t.Run("same-object-case-contract", func(t *testing.T) { assertCaseAliasLocatorContract(t, opts, markerHome, selected) })
		t.Skip("filesystem resolves case variants to the same object")
	}
	confirm(t, markerHome)
	confirm(t, selected)
	record(t, filepath.Join(opts.UserHome, ".maestro-home.json"), map[string]any{"schema_version": 1, "home": markerHome, "attempt_id": ""})
	opts.LookupEnv = func(string) (string, bool) { return selected, true }
	got, err := datahome.Resolve(opts)
	if err == nil || got.Writable || got.Diagnostic != "marker-conflict" {
		t.Fatalf("locator for %q authorized distinct %q: %+v %v", markerHome, selected, got, err)
	}
}

func caseDistinctHomes(t *testing.T, userHome string) (string, string, bool) {
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
	return lower, upper, !os.SameFile(lowerInfo, upperInfo)
}

func recordCasePairEvidence(t *testing.T, selected, counterpart string, same bool) {
	t.Helper()
	a, err := os.Stat(selected)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(counterpart)
	if err != nil {
		t.Fatal(err)
	}
	selectedID, counterpartID := initializeDirectoryIdentity(t, selected), initializeDirectoryIdentity(t, counterpart)
	if selected == counterpart || !filepath.IsAbs(selected) || !filepath.IsAbs(counterpart) || os.SameFile(a, b) != same || (selectedID == counterpartID) != same {
		t.Fatal("selected case-pair native identity was not established")
	}
	body, err := json.Marshal(map[string]any{"kind": "selected-directory-case-pair-v1", "scope": t.Name(), "selected": selected, "counterpart": counterpart, "selected_identity": selectedID, "counterpart_identity": counterpartID, "same_object": same})
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("case_pair_evidence=%s", body)
}

func assertCaseAliasLocatorContract(t *testing.T, opts datahome.Options, lower, upper string) {
	t.Helper()
	if got, err := datahome.InitializeFresh(opts); err != nil || !got.Writable {
		t.Fatalf("ordinary HOME positive control failed: %+v %v", got, err)
	}
	opts.LookupEnv = func(string) (string, bool) { return upper, true }
	before := homeSnapshot(t, opts.UserHome)
	if got, err := datahome.Resolve(opts); err != nil || !got.Writable || got.Diagnostic != "ready" || got.Home != upper {
		t.Fatalf("same-object default case spelling lost authority: %+v %v", got, err)
	}
	if after := homeSnapshot(t, opts.UserHome); !reflect.DeepEqual(before, after) {
		t.Fatal("case alias read changed HOME state")
	}
	record(t, filepath.Join(opts.UserHome, ".maestro-home.json"), map[string]any{"schema_version": 1, "home": lower, "attempt_id": ""})
	before = homeSnapshot(t, opts.UserHome)
	if got, err := datahome.Resolve(opts); err != nil || !got.Writable || got.Diagnostic != "ready" {
		t.Fatalf("same-object locator spelling was not recognized: %+v %v", got, err)
	}
	if after := homeSnapshot(t, opts.UserHome); !reflect.DeepEqual(before, after) {
		t.Fatal("case alias locator read changed HOME state")
	}
	other := filepath.Join(opts.UserHome, "different actual HOME")
	confirm(t, other)
	record(t, filepath.Join(opts.UserHome, ".maestro-home.json"), map[string]any{"schema_version": 1, "home": other, "attempt_id": ""})
	before = homeSnapshot(t, opts.UserHome)
	if got, err := datahome.Resolve(opts); err == nil || got.Writable || got.Diagnostic != "marker-conflict" {
		t.Fatalf("locator authorized a different actual HOME: %+v %v", got, err)
	}
	if after := homeSnapshot(t, opts.UserHome); !reflect.DeepEqual(before, after) {
		t.Fatal("conflicting locator refusal changed profile or HOME")
	}
	recordCasePairEvidence(t, upper, lower, true)
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
