package atlasops

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestInterruptedWorkspaceAppendRetriesAfterRestartWithoutDuplicate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("workspace mutation requires native confinement qualification")
	}
	base := t.TempDir()
	registered, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(base, "case"), DataRoot: filepath.Join(base, "local")})
	if err != nil {
		t.Fatal(err)
	}
	options := atlas.Options{DataRoot: registered.DataRoot, WorkspacePath: registered.WorkspacePath, WorkspaceID: registered.WorkspaceID}
	startedAt := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	clock := startedAt
	options.Now = func() time.Time { return clock }
	engine, err := OpenWorkspace(options)
	if err != nil {
		t.Fatal(err)
	}
	page := "knowledge/recovery.md"
	if _, err := engine.CreatePage(CreatePageRequest{Page: page, Body: "# Notes\n\n## Evidence\n", Provenance: attended("create")}); err != nil {
		t.Fatal(err)
	}
	request := AppendEntryRequest{Page: page, Section: "## Evidence", Entry: "- exactly once after interruption", Provenance: attended("append-once")}
	interrupted := errors.New("synthetic interruption after the real page effect")
	engine.afterPageEffect = func() error { return interrupted }
	if _, err := engine.AppendEntry(request); !errors.Is(err, interrupted) {
		t.Fatalf("missing injected interruption: %v", err)
	}
	projection, err := engine.Collect(CollectRequest{Purpose: "confirm actual effect", Reader: ReaderOwnerSession, Pages: []string{page}})
	if err != nil || len(projection.Pages) != 1 || strings.Count(projection.Pages[0].Content, "exactly once") != 1 {
		t.Fatalf("injection did not reach the page effect: %+v, %v", projection, err)
	}
	clock = clock.Add(24 * time.Hour)
	restarted, err := OpenWorkspace(options)
	if err != nil {
		t.Fatal(err)
	}
	retried, err := restarted.AppendEntry(request)
	if err != nil {
		t.Fatal(err)
	}
	if !retried.RecordedAt.Equal(startedAt) {
		t.Fatalf("recovered effect lost its prepared timestamp: %v", retried.RecordedAt)
	}
	projection, err = restarted.Collect(CollectRequest{Purpose: "read restarted knowledge", Reader: ReaderOwnerSession, Pages: []string{page}})
	if err != nil || len(projection.Pages) != 1 || strings.Count(projection.Pages[0].Content, "exactly once") != 1 || retried.State != StateUnchanged {
		t.Fatalf("interrupted retry duplicated effect: %+v, %+v, %v", retried, projection, err)
	}
}

func TestPendingWorkspaceAppendPreservesNewerContentAndExactRequest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("workspace mutation requires native confinement qualification")
	}
	for _, change := range []string{"newer-page", "changed-request", "before-page", "corrupt-journal"} {
		t.Run(change, func(t *testing.T) {
			base := t.TempDir()
			registered, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(base, "case"), DataRoot: filepath.Join(base, "local")})
			if err != nil {
				t.Fatal(err)
			}
			options := atlas.Options{DataRoot: registered.DataRoot, WorkspacePath: registered.WorkspacePath, WorkspaceID: registered.WorkspaceID}
			clock := time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
			options.Now = func() time.Time { return clock }
			engine, err := OpenWorkspace(options)
			if err != nil {
				t.Fatal(err)
			}
			page, original := "knowledge/recovery.md", "# Notes\n\n## Evidence\n"
			if _, err := engine.CreatePage(CreatePageRequest{Page: page, Body: original, Provenance: attended("create")}); err != nil {
				t.Fatal(err)
			}
			request := AppendEntryRequest{Page: page, Section: "## Evidence", Entry: "- original reviewed entry", Provenance: attended("interrupted")}
			engine.afterPageEffect = func() error { return errors.New("synthetic interruption") }
			if _, err := engine.AppendEntry(request); err == nil {
				t.Fatal("missing interruption")
			}
			clock = clock.Add(24 * time.Hour)
			restarted, err := OpenWorkspace(options)
			if err != nil {
				t.Fatal(err)
			}
			switch change {
			case "newer-page":
				if _, err := restarted.AppendEntry(AppendEntryRequest{Page: page, Section: "## Evidence", Entry: "- newer reviewed work", Provenance: attended("newer")}); err != nil {
					t.Fatal(err)
				}
			case "changed-request":
				request.Entry = "- substituted unreviewed entry"
			case "before-page":
				if err := os.WriteFile(filepath.Join(restarted.Root(), page), []byte(original), 0o600); err != nil {
					t.Fatal(err)
				}
			case "corrupt-journal":
				entries, err := os.ReadDir(filepath.Join(restarted.Root(), ".atlasops"))
				if err != nil {
					t.Fatal(err)
				}
				corrupted := false
				for _, entry := range entries {
					path := filepath.Join(restarted.Root(), ".atlasops", entry.Name())
					body, err := os.ReadFile(path)
					if err != nil {
						t.Fatal(err)
					}
					if strings.Contains(string(body), `"schema_version":2`) {
						if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
							t.Fatal(err)
						}
						corrupted = true
					}
				}
				if !corrupted {
					t.Fatal("missing actual prepared journal to interrupt")
				}
			}
			result, retryErr := restarted.AppendEntry(request)
			projection, err := restarted.Collect(CollectRequest{Purpose: "confirm safe retry", Reader: ReaderOwnerSession, Pages: []string{page}})
			if err != nil || len(projection.Pages) != 1 {
				t.Fatalf("readback: %+v %v", projection, err)
			}
			body := projection.Pages[0].Content
			if strings.Count(body, "original reviewed entry") != 1 {
				t.Fatalf("retry lost or duplicated original entry: %q", body)
			}
			switch change {
			case "newer-page":
				if retryErr != nil || result.State != StateProposed || !strings.Contains(body, "newer reviewed work") {
					t.Fatalf("retry overwrote newer page: %+v %q %v", result, body, retryErr)
				}
			case "changed-request":
				if retryErr == nil || strings.Contains(body, "substituted") {
					t.Fatalf("retry accepted a changed request: %+v %q %v", result, body, retryErr)
				}
			case "before-page":
				if retryErr != nil || result.State != StateWritten {
					t.Fatalf("retry did not reapply the exact pending effect: %+v %v", result, retryErr)
				}
			case "corrupt-journal":
				if retryErr == nil {
					t.Fatal("corrupt journal authorized a repeated effect")
				}
			}
		})
	}
}

func TestInterruptedAtlasCreateFieldAndLinkRecoverThroughPublicMethods(t *testing.T) {
	for _, operation := range []string{"create-page", "set-field", "link", "update-page"} {
		t.Run(operation, func(t *testing.T) {
			engine := testEngine(t)
			page := "knowledge/notes.md"
			initial, err := engine.CreatePage(CreatePageRequest{Page: page, Body: "# Notes\n\n- **Status:** draft\n\n## Evidence\n", Provenance: attended("initial")})
			if err != nil {
				t.Fatal(err)
			}
			apply := func(engine *Engine) (Result, error) {
				switch operation {
				case "create-page":
					return engine.CreatePage(CreatePageRequest{Page: "knowledge/created.md", Body: "# Created exactly once\n", Provenance: attended("effect")})
				case "update-page":
					return engine.UpdatePage(UpdatePageRequest{Page: page, Body: "# Updated exactly once\n", ExpectedRevision: initial.Revision, Provenance: attended("effect")})
				case "set-field":
					return engine.SetField(SetFieldRequest{Page: page, Field: "Status", Value: "reviewed", Provenance: attended("effect")})
				default:
					return engine.Link(LinkRequest{Page: page, Section: "## Evidence", Target: "knowledge/target.md", Label: "Reviewed target", Provenance: attended("effect")})
				}
			}
			engine.afterPageEffect = func() error { return errors.New("synthetic interruption") }
			if _, err := apply(engine); err == nil {
				t.Fatal("missing real effect interruption")
			}
			restarted, err := Open(engine.dataRoot, engine.now)
			if err != nil {
				t.Fatal(err)
			}
			result, err := apply(restarted)
			if err != nil || result.State != StateUnchanged {
				t.Fatalf("retry did not recover %s: %+v %v", operation, result, err)
			}
			projection, err := restarted.Collect(CollectRequest{Purpose: "read recovered operation", Reader: ReaderOwnerSession, Pages: []string{result.Page}})
			if err != nil || len(projection.Pages) != 1 {
				t.Fatalf("missing recovered page: %+v %v", projection, err)
			}
			expected := map[string]string{"create-page": "# Created exactly once", "set-field": "- **Status:** reviewed", "link": "[Reviewed target](target.md)", "update-page": "# Updated exactly once"}[operation]
			if strings.Count(projection.Pages[0].Content, expected) != 1 {
				t.Fatalf("wrong recovered content: %q", projection.Pages[0].Content)
			}
		})
	}
}

func TestOwnerReplaysHistoricalV1EnvelopeWithoutWorkspaceOrRequestDigest(t *testing.T) {
	engine := testEngine(t)
	request := CreatePageRequest{Page: "knowledge/historical.md", Body: "# Historical owner knowledge\n", Provenance: attended("historical")}
	created, err := engine.CreatePage(request)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(engine.Root(), ".atlasops", created.OperationID+".json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var record map[string]any
	if err := json.Unmarshal(body, &record); err != nil {
		t.Fatal(err)
	}
	if _, exists := record["workspace"]; exists {
		t.Fatal("owner record acquired a workspace envelope")
	}
	delete(record, "request_digest")
	body, err = json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	restarted, err := Open(engine.dataRoot, engine.now)
	if err != nil {
		t.Fatal(err)
	}
	result, err := restarted.CreatePage(request)
	if err != nil || result.State != StateUnchanged || result.Revision != created.Revision {
		t.Fatalf("historical owner receipt stopped replaying: %+v %v", result, err)
	}
	projection, err := restarted.Collect(CollectRequest{Purpose: "read historical owner knowledge", Reader: ReaderOwnerSession, Pages: []string{request.Page}})
	if err != nil || len(projection.Pages) != 1 || projection.Pages[0].Content != request.Body || projection.Pages[0].Pointer != "bcgos://atlas/owner/"+request.Page {
		t.Fatalf("historical owner behavior changed: %+v %v", projection, err)
	}
}
