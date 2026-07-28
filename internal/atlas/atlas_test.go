package atlas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestInitializeCreatesSeparateOwnerAndWorkspaceHumanAtlasWithoutTasks(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "Developer", "case-a")
	registered, err := workspace.Initialize(workspace.Options{DataRoot: dataRoot, WorkspacePath: workspacePath})
	if err != nil {
		t.Fatal(err)
	}
	status, err := Initialize(Options{DataRoot: dataRoot, WorkspacePath: workspacePath, WorkspaceID: registered.WorkspaceID})
	if err != nil {
		t.Fatal(err)
	}
	if !status.Owner.Available || !status.Workspace.Available || status.Managed.State != "unavailable" {
		t.Fatalf("status = %#v", status)
	}
	for _, path := range []string{
		filepath.Join(dataRoot, "atlas", "owner", "index.md"),
		filepath.Join(dataRoot, "atlas", "owner", "learnings", "index.md"),
		filepath.Join(dataRoot, "atlas", "owner", "development", "index.md"),
		filepath.Join(workspacePath, "brain", "index.md"),
		filepath.Join(workspacePath, "brain", "clients", "index.md"),
		filepath.Join(workspacePath, "brain", "projects", "index.md"),
		filepath.Join(workspacePath, "brain", "people", "index.md"),
		filepath.Join(workspacePath, "brain", "daily", "index.md"),
		filepath.Join(workspacePath, "brain", "clients", "template-client.md"),
		filepath.Join(workspacePath, "brain", "projects", "template-project.md"),
		filepath.Join(workspacePath, "brain", "people", "template-person.md"),
		filepath.Join(workspacePath, "brain", "daily", "template-daily.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	projectTemplate, err := os.ReadFile(filepath.Join(workspacePath, "brain", "projects", "template-project.md"))
	if err != nil || !strings.Contains(string(projectTemplate), "## Current truth") || !strings.Contains(string(projectTemplate), "## Decisions") {
		t.Fatalf("project template = %q, err = %v", projectTemplate, err)
	}
	dailyTemplate, err := os.ReadFile(filepath.Join(workspacePath, "brain", "daily", "template-daily.md"))
	if err != nil || !strings.Contains(string(dailyTemplate), "sanitization") {
		t.Fatalf("daily template = %q, err = %v", dailyTemplate, err)
	}
	if _, err := os.Stat(filepath.Join(workspacePath, "brain", "tasks")); !os.IsNotExist(err) {
		t.Fatalf("tasks directory = %v; task source is not decided", err)
	}
}

func TestInitializeDoesNotOverwriteOwnerAtlasContent(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "Developer", "case-a")
	registered, err := workspace.Initialize(workspace.Options{DataRoot: dataRoot, WorkspacePath: workspacePath})
	if err != nil {
		t.Fatal(err)
	}
	options := Options{DataRoot: dataRoot, WorkspacePath: workspacePath, WorkspaceID: registered.WorkspaceID}
	if _, err := Initialize(options); err != nil {
		t.Fatal(err)
	}
	index := filepath.Join(options.DataRoot, "atlas", "owner", "index.md")
	if err := os.WriteFile(index, []byte("# My owner atlas\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(options); err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(index)
	if err != nil || string(body) != "# My owner atlas\n" {
		t.Fatalf("owner index = %q, err = %v", body, err)
	}
}

func TestInitializeRejectsForgedWorkspaceIDWithoutWritingOwnerOrWorkspaceAtlas(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "Developer", "case-a")
	if _, err := workspace.Initialize(workspace.Options{DataRoot: dataRoot, WorkspacePath: workspacePath}); err != nil {
		t.Fatal(err)
	}
	if _, err := Initialize(Options{DataRoot: dataRoot, WorkspacePath: workspacePath, WorkspaceID: "forged-workspace"}); err == nil {
		t.Fatal("atlas bootstrap accepted a forged workspace identity")
	}
	for _, path := range []string{
		filepath.Join(dataRoot, "atlas", "owner"),
		filepath.Join(workspacePath, "brain", "clients"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("forged workspace identity wrote atlas content at %s: %v", path, err)
		}
	}
}
