package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializeCreatesInspectableHumanWorkspace(t *testing.T) {
	root := t.TempDir()
	workspacePath := filepath.Join(root, "Developer", "marcelo-brain")
	dataRoot := filepath.Join(root, "AppData", "BCGOS")

	result, err := Initialize(Options{WorkspacePath: workspacePath, DataRoot: dataRoot})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if result.State != "initialized" || result.WorkspaceID == "" {
		t.Fatalf("Initialize() result = %#v", result)
	}
	for _, path := range []string{
		filepath.Join(workspacePath, ".bcgos", "workspace.json"),
		filepath.Join(workspacePath, "README.md"),
		filepath.Join(workspacePath, "onboarding", "README.md"),
		filepath.Join(workspacePath, "brain", "README.md"),
		filepath.Join(workspacePath, "brain", "clients", "README.md"),
		filepath.Join(workspacePath, "brain", "projects", "README.md"),
		filepath.Join(workspacePath, "brain", "organization", "bcg", "README.md"),
		filepath.Join(workspacePath, "brain", "organization", "bcg", "people", "README.md"),
		filepath.Join(workspacePath, "brain", "organization", "bcg", "practices", "README.md"),
		filepath.Join(workspacePath, "brain", "tasks", "README.md"),
		filepath.Join(workspacePath, "agents", "maestro.md"),
		filepath.Join(workspacePath, "agents", "bcg-workspace.md"),
		filepath.Join(workspacePath, "agents", "client-accounts", "README.md"),
		filepath.Join(workspacePath, "agents", "client-accounts", "acme-example.md"),
		filepath.Join(workspacePath, "agents", "cases", "README.md"),
		filepath.Join(dataRoot, "memory"),
		filepath.Join(dataRoot, "config"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	contents, err := os.ReadFile(filepath.Join(workspacePath, "brain", "README.md"))
	if err != nil || !strings.Contains(string(contents), "navegável") {
		t.Fatalf("brain README = %q, error = %v", contents, err)
	}

	inspection, err := Inspect(workspacePath, dataRoot)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	if inspection.State != "ready" || inspection.WorkspaceID != result.WorkspaceID || !inspection.BrainReadable {
		t.Fatalf("Inspect() = %#v", inspection)
	}

	readmePath := filepath.Join(workspacePath, "brain", "README.md")
	if err := os.WriteFile(readmePath, []byte("# Meu conteúdo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(Options{WorkspacePath: workspacePath, DataRoot: dataRoot}); err != nil {
		t.Fatalf("second Initialize() error = %v", err)
	}
	contents, err = os.ReadFile(readmePath)
	if err != nil || string(contents) != "# Meu conteúdo\n" {
		t.Fatalf("re-initialization overwrote brain README = %q, error = %v", contents, err)
	}
	rootReadmePath := filepath.Join(workspacePath, "README.md")
	if err := os.WriteFile(rootReadmePath, []byte("# Meu workspace\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(Options{WorkspacePath: workspacePath, DataRoot: dataRoot}); err != nil {
		t.Fatalf("third Initialize() error = %v", err)
	}
	contents, err = os.ReadFile(rootReadmePath)
	if err != nil || string(contents) != "# Meu workspace\n" {
		t.Fatalf("re-initialization overwrote root README = %q, error = %v", contents, err)
	}
}

func TestInitializeRejectsSyncedWorkspaceUntilExplicitlyConfirmed(t *testing.T) {
	root := t.TempDir()
	_, err := Initialize(Options{
		WorkspacePath: filepath.Join(root, "OneDrive", "brain"),
		DataRoot:      filepath.Join(root, "AppData", "BCGOS"),
	})
	if !errors.Is(err, ErrSynchronizedWorkspace) {
		t.Fatalf("Initialize() error = %v, want ErrSynchronizedWorkspace", err)
	}
}

func TestInspectRejectsManifestCopiedFromAnotherWorkspace(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "AppData", "BCGOS")
	workspaceA := filepath.Join(root, "Developer", "case-a")
	workspaceB := filepath.Join(root, "Developer", "case-b")
	if _, err := Initialize(Options{WorkspacePath: workspaceA, DataRoot: dataRoot}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspaceB, ".bcgos"), 0o700); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(workspaceA, ".bcgos", "workspace.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspaceB, ".bcgos", "workspace.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}
	inspection, err := Inspect(workspaceB, dataRoot)
	if err != nil {
		t.Fatal(err)
	}
	if inspection.State != "invalid" || inspection.MetadataStatus != "path_mismatch" || inspection.WorkspaceID != "" {
		t.Fatalf("Inspect() = %#v, want fail-closed path mismatch", inspection)
	}
}

func TestDefaultDataRootUsesPerUserApplicationStorage(t *testing.T) {
	tests := []struct {
		name     string
		platform string
		home     string
		appData  string
		xdg      string
		want     string
	}{
		{name: "windows", platform: "windows", appData: `C:\Users\Marcelo\AppData\Local`, want: `C:\Users\Marcelo\AppData\Local\BCGOS`},
		{name: "macos", platform: "darwin", home: "/Users/marcelo", want: "/Users/marcelo/Library/Application Support/BCGOS"},
		{name: "linux xdg", platform: "linux", home: "/home/marcelo", xdg: "/state", want: "/state/bcgos"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := DefaultDataRoot(test.platform, test.home, test.appData, test.xdg)
			if err != nil || got != test.want {
				t.Fatalf("DefaultDataRoot() = %q, %v; want %q", got, err, test.want)
			}
		})
	}
}
