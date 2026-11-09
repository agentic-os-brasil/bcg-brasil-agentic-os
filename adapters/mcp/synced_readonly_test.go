package product_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

func TestProductOpenRefusesLegacySourceStateWithoutMigration(t *testing.T) {
	for _, guardProcess := range []bool{false, true} {
		name := "library"
		if guardProcess {
			name = "qualified_process"
		}
		t.Run(name, func(t *testing.T) {
			base := physicalTempDir(t)
			user, managed, workspace := filepath.Join(base, "owner"), filepath.Join(base, "managed"), filepath.Join(base, "workspace")
			stateRoot := filepath.Join(user, ".maestro-install", "config")
			for _, path := range []string{managed, workspace, stateRoot} {
				if err := os.MkdirAll(path, 0700); err != nil {
					t.Fatal(err)
				}
			}
			// This is the valid historical wire used by the canonical migration
			// test, not an unsupported/malformed stand-in for a real installation.
			body := []byte("{\n  \"schema_version\": 1,\n  \"release\": \"0.1.0\",\n  \"channel\": \"canary\",\n  \"cli_version\": \"0.1.0\",\n  \"bundle_version\": \"0.1.0\",\n  \"target_os\": \"darwin\",\n  \"target_arch\": \"arm64\",\n  \"activated_at\": \"2026-07-01T00:00:00Z\"\n}\n")
			if err := os.WriteFile(filepath.Join(stateRoot, "install-state.json"), body, 0600); err != nil {
				t.Fatal(err)
			}
			before := treeSnapshot(t, base)
			_, err := product.Open(product.Options{ProductRoot: managed, UserHome: user, WorkspacePath: workspace, GuardProcess: guardProcess, CLIVersion: "0.1.42", LookupEnv: func(string) (string, bool) { return "", false }})
			if err == nil {
				t.Fatal("historical schema1 received source-aware product admission")
			}
			if !reflect.DeepEqual(before, treeSnapshot(t, base)) {
				t.Fatalf("read-only product.Open migrated valid schema1 before refusal (guard_process=%t)", guardProcess)
			}
		})
	}
}
