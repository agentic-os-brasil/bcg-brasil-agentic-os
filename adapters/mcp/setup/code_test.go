package setup_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/setup"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
)

func TestCodeRegistrationPreservesDisabledAndRefusesForeignSource(t *testing.T) {
	testCodeInventory(t, false)
}

func TestCodeRegistrationWithUnrelatedCachedPlugin(t *testing.T) {
	testCodeInventory(t, true)
}

func testCodeInventory(t *testing.T, mixed bool) {
	t.Helper()
	originalExecutable, err := os.Executable()
	must(t, err)
	originalBody := read(t, originalExecutable)
	executable := filepath.Join(t.TempDir(), "controlled-code")
	must(t, os.WriteFile(executable, originalBody, 0700))
	id := "0123456789abcdef0123456789abcdef"
	root := filepath.Join(t.TempDir(), "marketplace")
	record := installer.ClientRecord{Binding: installer.ClientBinding{Workspace: t.TempDir()}, Materialization: installer.ClientMaterialization{CodeRoot: root, Release: "0.1.43"}, Code: installer.CodeObservation{Executable: executable, Marketplace: "maestro-local-" + id, PluginID: "maestro-product@maestro-local-" + id, Scope: "user"}}
	foreign, enabled, current := false, false, false
	version, fault, versionCalls := "2.1.295", "", 0
	var mutations [][]string
	run := func(_ context.Context, path string, args []string, cwd string) ([]byte, error) {
		if path != executable || cwd != record.Binding.Workspace {
			t.Fatal("Code selection drift")
		}
		if reflect.DeepEqual(args, []string{"--version"}) {
			versionCalls++
			if fault == "before_version" && versionCalls == 3 {
				version = "2.1.296"
			}
			return []byte(version + " (Claude Code)\n"), nil
		}
		if args[len(args)-1] == "--help" {
			return []byte("--json --scope\n"), nil
		}
		if reflect.DeepEqual(args, []string{"plugin", "marketplace", "list", "--json"}) {
			source := root
			if foreign {
				source = filepath.Join(root, "foreign")
			}
			return json.Marshal([]any{map[string]any{"name": "maestro-local-" + id, "source": "directory", "path": source}})
		}
		if reflect.DeepEqual(args, []string{"plugin", "list", "--json"}) {
			version := "0.1.42"
			if current {
				version = "0.1.43"
			}
			rows := []any{map[string]any{"id": "maestro-product@maestro-local-" + id, "version": "0.1.42", "scope": "user", "enabled": enabled, "installPath": filepath.Join(root, "plugins", "maestro-product"), "readFromFolder": true, "folderVersion": version}}
			if mixed {
				rows = append(rows, map[string]any{"id": "unrelated@git-market", "version": "1.2.3", "scope": "user", "enabled": true, "installPath": filepath.Join(root, "unrelated-cache")})
			}
			return json.Marshal(rows)
		}
		mutations = append(mutations, append([]string{}, args...))
		switch strings.Join(args[:2], " ") {
		case "plugin marketplace":
		case "plugin update":
			current = true
			enabled = true
		case "plugin disable":
			enabled = false
		default:
			return nil, errors.New("unexpected mutation")
		}
		if len(mutations) == 1 && strings.HasPrefix(fault, "after_first_") || len(mutations) == 3 && strings.HasPrefix(fault, "after_final_") {
			if strings.HasSuffix(fault, "version") {
				version = "2.1.296"
			} else {
				must(t, os.WriteFile(executable, append(append([]byte{}, originalBody...), 1), 0700))
			}
		}
		return []byte("{\"command\":\"controlled-process-witness\",\"outcome\":\"success\"}\n"), nil
	}
	review, err := setup.ReviewCodeRegistration(context.Background(), record, run)
	must(t, err)
	if len(mutations) != 0 || len(review.Actions) != 3 {
		t.Fatalf("review mutation or incomplete disabled preservation: %+v", review)
	}
	observation, err := setup.ApplyCodeRegistration(context.Background(), record, run, review.Digest, true)
	must(t, err)
	want := [][]string{{"plugin", "marketplace", "update", "maestro-local-" + id, "--json"}, {"plugin", "update", "maestro-product@maestro-local-" + id, "--scope", "user", "--json"}, {"plugin", "disable", "maestro-product@maestro-local-" + id, "--scope", "user", "--json"}}
	if !reflect.DeepEqual(mutations, want) || observation.Enabled || observation.State != "registered" || observation.InstalledVersion != "0.1.42" || observation.FolderVersion != "0.1.43" || observation.EvidenceClass != "engineering_process_witness" {
		t.Fatalf("supported argv/readback did not preserve semantics: %+v %+v", mutations, observation)
	}
	foreign = true
	mutations = nil
	if _, err = setup.ReviewCodeRegistration(context.Background(), record, run); err == nil || len(mutations) > 0 {
		t.Fatal("foreign marketplace admitted/mutated")
	}
	// Exercise identity drift inside these existing registration families. These
	// are observations around supported CLI effects, not atomic exec-by-FD proof.
	for _, scenario := range []string{"before_version", "after_first_version", "after_final_version", "after_first_hash", "after_final_hash"} {
		foreign, enabled, current = false, false, false
		version, fault, versionCalls = "2.1.295", scenario, 0
		mutations = nil
		must(t, os.WriteFile(executable, originalBody, 0700))
		driftReview, e := setup.ReviewCodeRegistration(context.Background(), record, run)
		must(t, e)
		observed, e := setup.ApplyCodeRegistration(context.Background(), record, run, driftReview.Digest, true)
		if e == nil || observed.State == "registered" {
			t.Fatalf("%s drift accepted as registered: %+v %v", scenario, observed, e)
		}
		expected := 1
		if scenario == "before_version" {
			expected = 0
		} else if strings.HasPrefix(scenario, "after_final_") {
			expected = 3
		}
		if len(mutations) != expected {
			t.Fatalf("%s continued effects after observable identity drift: got %d want %d", scenario, len(mutations), expected)
		}
	}
}

func TestCodeRegistrationDoesNotTrustExitAcknowledgement(t *testing.T) {
	executable, err := os.Executable()
	must(t, err)
	root := t.TempDir()
	record := installer.ClientRecord{Binding: installer.ClientBinding{Workspace: root}, Materialization: installer.ClientMaterialization{CodeRoot: root, Release: "0.1.43"}, Code: installer.CodeObservation{Executable: executable, Marketplace: "maestro-local-0123456789abcdef0123456789abcdef", PluginID: "maestro-product@maestro-local-0123456789abcdef0123456789abcdef", Scope: "user"}}
	run := func(_ context.Context, _ string, args []string, _ string) ([]byte, error) {
		if args[0] == "--version" {
			return []byte("2.1.289"), nil
		}
		if args[len(args)-1] == "--help" {
			return []byte("--json"), nil
		}
		if args[len(args)-2] == "list" {
			return []byte("[]"), nil
		}
		return []byte("{\"outcome\":\"success\"}"), nil
	}
	review, err := setup.ReviewCodeRegistration(context.Background(), record, run)
	must(t, err)
	if _, err = setup.ApplyCodeRegistration(context.Background(), record, run, review.Digest, true); err == nil {
		t.Fatal("successful command without installed readback became registration")
	}
}

func TestCodeReadbackRequiresExplicitEnabledState(t *testing.T) {
	executable, err := os.Executable()
	must(t, err)
	root := t.TempDir()
	id := "0123456789abcdef0123456789abcdef"
	r := installer.ClientRecord{Binding: installer.ClientBinding{Workspace: root}, Materialization: installer.ClientMaterialization{CodeRoot: root, Release: "0.1.43"}, Code: installer.CodeObservation{Executable: executable, Marketplace: "maestro-local-" + id, PluginID: "maestro-product@maestro-local-" + id, Scope: "user"}}
	run := func(_ context.Context, _ string, args []string, _ string) ([]byte, error) {
		if args[0] == "--version" {
			return []byte("2.1.295"), nil
		}
		if args[len(args)-1] == "--help" {
			return []byte("--json"), nil
		}
		if len(args) > 2 && args[1] == "marketplace" {
			return json.Marshal([]any{map[string]any{"name": r.Code.Marketplace, "source": "directory", "path": root}})
		}
		return json.Marshal([]any{map[string]any{"id": r.Code.PluginID, "version": "0.1.43", "scope": "user", "installPath": filepath.Join(root, "plugins", "maestro-product"), "folderVersion": "0.1.43", "readFromFolder": true}})
	}
	if _, err = setup.ReviewCodeRegistration(context.Background(), r, run); err == nil {
		t.Fatal("missing enabled field was interpreted as an observed disabled installation")
	}
}
