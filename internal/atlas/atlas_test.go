package atlas

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestInitializeRejectsSymlinkedAtlasBoundaries(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires privileges on some Windows runners")
	}
	for _, scenario := range []string{"owner", "daily"} {
		t.Run(scenario, func(t *testing.T) {
			root := t.TempDir()
			dataRoot := filepath.Join(root, "local", "BCGOS")
			workspacePath := filepath.Join(root, "Developer", "case-a")
			registered, err := workspace.Initialize(workspace.Options{DataRoot: dataRoot, WorkspacePath: workspacePath})
			if err != nil {
				t.Fatal(err)
			}
			external := filepath.Join(root, "external")
			if err := os.MkdirAll(external, 0o700); err != nil {
				t.Fatal(err)
			}
			var boundary string
			switch scenario {
			case "owner":
				boundary = filepath.Join(dataRoot, "atlas", "owner")
				if err := os.MkdirAll(filepath.Dir(boundary), 0o700); err != nil {
					t.Fatal(err)
				}
			case "daily":
				boundary = filepath.Join(workspacePath, "brain", "daily")
				if err := os.RemoveAll(boundary); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Symlink(external, boundary); err != nil {
				t.Fatal(err)
			}
			_, err = Initialize(Options{DataRoot: dataRoot, WorkspacePath: workspacePath, WorkspaceID: registered.WorkspaceID, Now: func() time.Time {
				return time.Date(2026, 8, 11, 8, 0, 0, 0, time.UTC)
			}})
			if err == nil {
				t.Fatal("atlas bootstrap followed a symlinked boundary")
			}
			entries, readErr := os.ReadDir(external)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("external target was modified: entries=%v err=%v", entries, readErr)
			}
		})
	}
}

func TestInitializeCreatesSeparateOwnerAndWorkspaceHumanAtlasWithVisibleTaskStub(t *testing.T) {
	root := t.TempDir()
	dataRoot := filepath.Join(root, "local", "BCGOS")
	workspacePath := filepath.Join(root, "Developer", "case-a")
	registered, err := workspace.Initialize(workspace.Options{DataRoot: dataRoot, WorkspacePath: workspacePath})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 8, 11, 8, 0, 0, 0, time.FixedZone("BRT", -3*60*60))
	status, err := Initialize(Options{DataRoot: dataRoot, WorkspacePath: workspacePath, WorkspaceID: registered.WorkspaceID, Now: func() time.Time { return now }})
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
		filepath.Join(workspacePath, "brain", "daily", "2026-08-11.md"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s: %v", path, err)
		}
	}
	daily, err := os.ReadFile(filepath.Join(workspacePath, "brain", "daily", "2026-08-11.md"))
	if err != nil || !strings.Contains(string(daily), "# Daily — 2026-08-11") || !strings.Contains(string(daily), "## Carry forward") {
		t.Fatalf("initial daily log = %q, err = %v", daily, err)
	}
	projectTemplate, err := os.ReadFile(filepath.Join(workspacePath, "brain", "projects", "template-project.md"))
	if err != nil || !strings.Contains(string(projectTemplate), "## Current truth") || !strings.Contains(string(projectTemplate), "## Decisions") {
		t.Fatalf("project template = %q, err = %v", projectTemplate, err)
	}
	dailyTemplate, err := os.ReadFile(filepath.Join(workspacePath, "brain", "daily", "template-daily.md"))
	if err != nil || !strings.Contains(string(dailyTemplate), "sanitization") {
		t.Fatalf("daily template = %q, err = %v", dailyTemplate, err)
	}
	if info, err := os.Stat(filepath.Join(workspacePath, "brain", "tasks", "README.md")); err != nil || info.IsDir() {
		t.Fatalf("visible task stub = %v", err)
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
		filepath.Join(workspacePath, "brain", "clients", "index.md"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("forged workspace identity wrote atlas content at %s: %v", path, err)
		}
	}
}
