//go:build !windows

package atlasops

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestExclusiveAtlasPublicationSurvivesProcessExit(t *testing.T) {
	const crashExit = 77
	request := CreatePageRequest{Page: "knowledge/crash.md", Body: "# Recovered knowledge\n\n## Evidence\n", Provenance: attended("exclusive-publication")}
	if base := os.Getenv("MAESTRO_ATLAS_PUBLICATION_CRASH_ROOT"); base != "" {
		inspection, err := workspace.Inspect(filepath.Join(base, "case"), filepath.Join(base, "local"))
		if err != nil {
			t.Fatal(err)
		}
		engine, err := OpenWorkspace(atlas.Options{DataRoot: inspection.DataRoot, WorkspacePath: inspection.WorkspacePath, WorkspaceID: inspection.WorkspaceID})
		if err != nil {
			t.Fatal(err)
		}
		publication := 0
		atlasExclusivePublicationHook = func() {
			publication++
			if (os.Getenv("MAESTRO_ATLAS_PUBLICATION_CRASH_AT") == "prepared" && publication == 1) || publication == 2 {
				os.Exit(crashExit)
			}
		}
		if _, err := engine.CreatePage(request); err != nil {
			t.Fatal(err)
		}
		t.Fatal("exclusive-publication interruption was not reached")
	}
	for _, boundary := range []string{"prepared", "page"} {
		t.Run(boundary, func(t *testing.T) {
			base := t.TempDir()
			registered, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(base, "case"), DataRoot: filepath.Join(base, "local")})
			if err != nil {
				t.Fatal(err)
			}
			command := exec.Command(os.Args[0], "-test.run=^TestExclusiveAtlasPublicationSurvivesProcessExit$")
			command.Env = append(os.Environ(), "MAESTRO_ATLAS_PUBLICATION_CRASH_ROOT="+base, "MAESTRO_ATLAS_PUBLICATION_CRASH_AT="+boundary)
			output, err := command.CombinedOutput()
			var exited *exec.ExitError
			if !errors.As(err, &exited) || exited.ExitCode() != crashExit {
				t.Fatalf("child missed publication boundary: %v %s", err, output)
			}
			restarted, err := OpenWorkspace(atlas.Options{DataRoot: registered.DataRoot, WorkspacePath: registered.WorkspacePath, WorkspaceID: registered.WorkspaceID})
			if err != nil {
				t.Fatal(err)
			}
			result, err := restarted.CreatePage(request)
			if err != nil {
				t.Fatalf("explicit restart/retry could not read interrupted %s publication: %v", boundary, err)
			}
			want := StateWritten
			if boundary == "page" {
				want = StateUnchanged
			}
			if result.State != want {
				t.Fatalf("recovered state = %s, want %s", result.State, want)
			}
			projection, err := restarted.Collect(CollectRequest{Purpose: "read interrupted exclusive publication", Reader: ReaderOwnerSession, Pages: []string{request.Page}})
			if err != nil || len(projection.Pages) != 1 || projection.Pages[0].Content != request.Body {
				t.Fatalf("recovered page unreadable or changed: %+v %v", projection, err)
			}
		})
	}
}

func TestLegacyOwnerPageInOrdinaryDirectoryRemainsReadableAndAppendable(t *testing.T) {
	engine := testEngine(t)
	parent := filepath.Join(engine.Root(), "hand-authored", "nested")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	page := "hand-authored/nested/legacy.md"
	if err := os.WriteFile(filepath.Join(engine.Root(), page), []byte("# Legacy owner page\n\n## Evidence\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	projection, err := engine.Collect(CollectRequest{Purpose: "read existing owner corpus", Reader: ReaderOwnerSession, Pages: []string{page}})
	if err != nil || len(projection.Pages) != 1 {
		t.Fatalf("valid existing owner page became unreadable: %+v %v", projection, err)
	}
	if _, err := engine.AppendEntry(AppendEntryRequest{Page: page, Section: "## Evidence", Entry: "- preserved owner behavior", Provenance: attended("legacy-append")}); err != nil {
		t.Fatalf("valid existing owner page became unwritable: %v", err)
	}
	projection, err = engine.Collect(CollectRequest{Purpose: "read appended owner page", Reader: ReaderOwnerSession, Pages: []string{page}})
	if err != nil || len(projection.Pages) != 1 || projection.Pages[0].Content != "# Legacy owner page\n\n## Evidence\n- preserved owner behavior\n" {
		t.Fatalf("legacy append changed wrong content: %+v %v", projection, err)
	}
	info, err := os.Stat(parent)
	if err != nil || info.Mode().Perm() != 0o755 {
		t.Fatalf("page IO changed owner directory permissions: %v %v", info, err)
	}
}

func TestOwnerPageCompatibilityKeepsRootsControlsAndWorkspacePrivate(t *testing.T) {
	for _, scope := range []string{"owner-root", "owner-controls", "workspace-page"} {
		t.Run(scope, func(t *testing.T) {
			engine := testEngine(t)
			if scope == "workspace-page" {
				base := t.TempDir()
				registered, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(base, "case"), DataRoot: filepath.Join(base, "local")})
				if err != nil {
					t.Fatal(err)
				}
				engine, err = OpenWorkspace(atlas.Options{DataRoot: registered.DataRoot, WorkspacePath: registered.WorkspacePath, WorkspaceID: registered.WorkspaceID})
				if err != nil {
					t.Fatal(err)
				}
			}
			page := "knowledge/private.md"
			if _, err := engine.CreatePage(CreatePageRequest{Page: page, Body: "# Private page\n", Provenance: attended("create")}); err != nil {
				t.Fatal(err)
			}
			path := engine.Root()
			if scope == "owner-controls" {
				path = filepath.Join(engine.Root(), ".atlasops")
			}
			if scope == "workspace-page" {
				path = filepath.Join(engine.Root(), "knowledge")
			}
			if err := os.Chmod(path, 0o755); err != nil {
				t.Fatal(err)
			}
			var err error
			if scope == "owner-controls" {
				_, err = engine.Grants()
			} else {
				_, err = engine.Collect(CollectRequest{Purpose: "refuse relaxed authority directories", Reader: ReaderOwnerSession, Pages: []string{page}})
			}
			if err == nil {
				t.Fatalf("owner-page compatibility relaxed %s", scope)
			}
			info, err := os.Stat(path)
			if err != nil || info.Mode().Perm() != 0o755 {
				t.Fatalf("read repaired directory permissions: %v %v", info, err)
			}
		})
	}
}
