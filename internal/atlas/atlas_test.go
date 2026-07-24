package atlas

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInitializeCreatesSeparateOwnerAndWorkspaceHumanAtlasWithoutTasks(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "Developer", "case-a")
	status, err := Initialize(Options{DataRoot: dataRoot, WorkspacePath: workspacePath, WorkspaceID: "workspace-a"})
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
	options := Options{DataRoot: filepath.Join(root, "local", "BCGOS"), WorkspacePath: filepath.Join(root, "Developer", "case-a"), WorkspaceID: "workspace-a"}
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
