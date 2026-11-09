package atlasops_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlas"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/atlasops"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func standingGrant() atlasops.GrantRequest {
	return atlasops.GrantRequest{Ritual: "reviewed-notes", RitualVersion: "1", Segment: "knowledge/client", Operations: []string{"create-page", "append-entry"}, Cadence: "manual", Reader: atlasops.ReaderOwnerSession}
}

func TestConcurrentWorkspaceAppendsPreserveBothEntriesAndFenceRevisions(t *testing.T) {
	for _, fenced := range []bool{false, true} {
		t.Run(map[bool]string{false: "append-both", true: "same-reviewed-revision"}[fenced], func(t *testing.T) {
			base := t.TempDir()
			options := registeredWorkspace(t, base, filepath.Join(base, "local"), "a")
			var enabled atomic.Bool
			var calls atomic.Int32
			arrived := make(chan struct{}, 2)
			release := make(chan struct{})
			options.Now = func() time.Time {
				if enabled.Load() {
					call := calls.Add(1)
					if call <= 2 {
						arrived <- struct{}{}
					}
					if call == 1 {
						<-release
					}
				}
				return time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
			}
			first, err := atlasops.OpenWorkspace(options)
			if err != nil {
				t.Fatal(err)
			}
			second, err := atlasops.OpenWorkspace(options)
			if err != nil {
				t.Fatal(err)
			}
			page := "knowledge/concurrent.md"
			created, err := first.CreatePage(atlasops.CreatePageRequest{Page: page, Body: "# Notes\n\n## Evidence\n", Provenance: provenance("create")})
			if err != nil {
				t.Fatal(err)
			}
			enabled.Store(true)
			type outcome struct {
				result atlasops.Result
				err    error
			}
			results := make(chan outcome, 2)
			appendEntry := func(engine *atlasops.Engine, entry string) {
				request := atlasops.AppendEntryRequest{Page: page, Section: "## Evidence", Entry: entry, Provenance: provenance(entry)}
				if fenced {
					request.ExpectedRevision = created.Revision
				}
				result, err := engine.AppendEntry(request)
				results <- outcome{result, err}
			}
			go appendEntry(first, "- first reviewed entry")
			<-arrived
			go appendEntry(second, "- second reviewed entry")
			// The unfenced engine reaches its clock after reading the same page;
			// a serialized writer must wait until the first effect finishes.
			select {
			case <-arrived:
			case <-time.After(300 * time.Millisecond):
			}
			close(release)
			written, proposed := 0, 0
			for range 2 {
				got := <-results
				if got.err != nil {
					t.Fatal(got.err)
				}
				if got.result.State == atlasops.StateWritten {
					written++
				}
				if got.result.State == atlasops.StateProposed {
					proposed++
				}
			}
			body := collectPage(t, first, page).Content
			if fenced {
				if written != 1 || proposed != 1 || strings.Count(body, "reviewed entry") != 1 {
					t.Fatalf("concurrent review fence: written=%d proposed=%d body=%q", written, proposed, body)
				}
			} else if written != 2 || !strings.Contains(body, "- first reviewed entry") || !strings.Contains(body, "- second reviewed entry") {
				t.Fatalf("concurrent append lost content: written=%d body=%q", written, body)
			}
		})
	}
}

func registeredWorkspace(t *testing.T, base, dataRoot, name string) atlas.Options {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("new workspace mutation requires native confinement qualification")
	}
	result, err := workspace.Initialize(workspace.Options{WorkspacePath: filepath.Join(base, name), DataRoot: dataRoot})
	if err != nil {
		t.Fatal(err)
	}
	return atlas.Options{DataRoot: dataRoot, WorkspacePath: result.WorkspacePath, WorkspaceID: result.WorkspaceID}
}

func TestWorkspaceGrantRevocationOrdersWithPageEffects(t *testing.T) {
	base := t.TempDir()
	options := registeredWorkspace(t, base, filepath.Join(base, "local"), "case")
	var block atomic.Bool
	arrived, release := make(chan struct{}), make(chan struct{})
	options.Now = func() time.Time {
		if block.CompareAndSwap(true, false) {
			close(arrived)
			<-release
		}
		return time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)
	}
	writer, err := atlasops.OpenWorkspace(options)
	if err != nil {
		t.Fatal(err)
	}
	otherOptions := options
	otherOptions.Now = nil
	controller, err := atlasops.OpenWorkspace(otherOptions)
	if err != nil {
		t.Fatal(err)
	}
	page := "knowledge/client/granted.md"
	if _, err := writer.CreatePage(atlasops.CreatePageRequest{Page: page, Body: "# Notes\n\n## Evidence\n", Provenance: provenance("create")}); err != nil {
		t.Fatal(err)
	}
	grant, err := writer.CreateGrant(standingGrant())
	if err != nil {
		t.Fatal(err)
	}
	request := atlasops.AppendEntryRequest{Page: page, Section: "## Evidence", Entry: "- granted work", Provenance: atlasops.Provenance{Origin: atlasops.OriginGrant, GrantID: grant.GrantID, OccurrenceID: "once", IdempotencyKey: "append"}}
	if err := controller.PauseGrant(grant.GrantID); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.AppendEntry(request); err == nil {
		t.Fatal("paused grant produced a page effect")
	}
	if err := controller.ResumeGrant(grant.GrantID); err != nil {
		t.Fatal(err)
	}
	block.Store(true)
	written, revoked := make(chan error, 1), make(chan error, 1)
	go func() { _, err := writer.AppendEntry(request); written <- err }()
	<-arrived
	go func() { revoked <- controller.RevokeGrant(grant.GrantID) }()
	select {
	case err := <-revoked:
		close(release)
		t.Fatalf("revocation completed before the locked page effect: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-written; err != nil {
		t.Fatal(err)
	}
	if err := <-revoked; err != nil {
		t.Fatal(err)
	}
	request.Provenance.IdempotencyKey = "after-revocation"
	request.Entry = "- refused after revocation"
	if _, err := writer.AppendEntry(request); err == nil {
		t.Fatal("revoked grant produced a later effect")
	}
	if body := collectPage(t, writer, page).Content; !strings.Contains(body, "- granted work") || strings.Contains(body, "refused after revocation") {
		t.Fatalf("grant transition changed the wrong content: %q", body)
	}
	expired := standingGrant()
	expiry := options.Now().Add(-time.Minute)
	expired.ExpiresAt = &expiry
	grant, err = writer.CreateGrant(expired)
	if err != nil {
		t.Fatal(err)
	}
	request.Provenance.GrantID = grant.GrantID
	if _, err := writer.AppendEntry(request); err == nil {
		t.Fatal("expired grant produced a page effect")
	}
}

func TestWorkspacePagesRefuseAliasedLeavesAndKeepPageBudget(t *testing.T) {
	for _, alias := range []string{"symlink", "hardlink"} {
		t.Run(alias, func(t *testing.T) {
			base := t.TempDir()
			options := registeredWorkspace(t, base, filepath.Join(base, "local"), "case")
			engine, err := atlasops.OpenWorkspace(options)
			if err != nil {
				t.Fatal(err)
			}
			outside := filepath.Join(base, "outside.md")
			body := "# Outside\n\n## Evidence\n"
			if err := os.WriteFile(outside, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(engine.Root(), "alias.md")
			link := os.Symlink
			if alias == "hardlink" {
				link = os.Link
			}
			if err := link(outside, path); err != nil {
				t.Skipf("links unavailable: %v", err)
			}
			if _, err := engine.Collect(atlasops.CollectRequest{Purpose: "refuse aliases", Reader: atlasops.ReaderOwnerSession, Pages: []string{"alias.md"}}); err == nil {
				t.Fatal("read followed an aliased leaf")
			}
			if _, err := engine.AppendEntry(atlasops.AppendEntryRequest{Page: "alias.md", Section: "## Evidence", Entry: "- refused", Provenance: provenance("append")}); err == nil {
				t.Fatal("write followed an aliased leaf")
			}
			got, err := os.ReadFile(outside)
			if err != nil || string(got) != body {
				t.Fatalf("alias effect changed outside content: %q %v", got, err)
			}
		})
	}
	t.Run("page-limit", func(t *testing.T) {
		base := t.TempDir()
		options := registeredWorkspace(t, base, filepath.Join(base, "local"), "case")
		engine, err := atlasops.OpenWorkspace(options)
		if err != nil {
			t.Fatal(err)
		}
		prefix := "# Notes\n\n## Evidence\n"
		body := prefix + strings.Repeat("x", (256<<10)-len(prefix))
		if _, err := engine.CreatePage(atlasops.CreatePageRequest{Page: "full.md", Body: body, Provenance: provenance("full")}); err != nil {
			t.Fatal(err)
		}
		if _, err := engine.AppendEntry(atlasops.AppendEntryRequest{Page: "full.md", Section: "## Evidence", Entry: "- overflow", Provenance: provenance("overflow")}); err == nil {
			t.Fatal("append exceeded the page budget")
		}
		got, err := os.ReadFile(filepath.Join(engine.Root(), "full.md"))
		if err != nil || string(got) != body {
			t.Fatal("refused overflow changed the page")
		}
	})
}

func provenance(key string) atlasops.Provenance {
	return atlasops.Provenance{Origin: atlasops.OriginAttended, SessionID: "synthetic-session", IdempotencyKey: key}
}

func collectPage(t *testing.T, engine *atlasops.Engine, page string) atlasops.PageProjection {
	t.Helper()
	projection, err := engine.Collect(atlasops.CollectRequest{Purpose: "read reviewed workspace knowledge", Reader: atlasops.ReaderOwnerSession, Pages: []string{page}})
	if err != nil || len(projection.Pages) != 1 {
		t.Fatalf("collect page: %+v, %v", projection, err)
	}
	return projection.Pages[0]
}

func TestWorkspaceKnowledgeIsSeparateAndSurvivesRestart(t *testing.T) {
	base := t.TempDir()
	dataRoot := filepath.Join(base, "local")
	a := registeredWorkspace(t, base, dataRoot, "case-a")
	b := registeredWorkspace(t, base, dataRoot, "case-b")
	first, err := atlasops.OpenWorkspace(a)
	if err != nil {
		t.Fatal(err)
	}
	second, err := atlasops.OpenWorkspace(b)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := atlasops.Open(dataRoot, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"index.md", ".atlasops"} {
		if _, err := os.Lstat(filepath.Join(first.Root(), name)); !os.IsNotExist(err) {
			t.Fatalf("ordinary workspace open created %s: %v", name, err)
		}
	}
	page := "knowledge/reviewed.md"
	for _, item := range []struct {
		engine  *atlasops.Engine
		body    string
		pointer string
	}{
		{first, "# Case A\n\n## Evidence\n", "bcgos://atlas/workspace/" + a.WorkspaceID + "/" + page},
		{second, "# Case B\n\n## Evidence\n", "bcgos://atlas/workspace/" + b.WorkspaceID + "/" + page},
		{owner, "# Owner\n\n## Evidence\n", "bcgos://atlas/owner/" + page},
	} {
		written, err := item.engine.CreatePage(atlasops.CreatePageRequest{Page: page, Body: item.body, Provenance: provenance("same-key")})
		if err != nil || written.State != atlasops.StateWritten {
			t.Fatalf("create in separate corpus: %+v, %v", written, err)
		}
		got := collectPage(t, item.engine, page)
		if got.Content != item.body || got.Pointer != item.pointer || got.Revision != written.Revision {
			t.Fatalf("wrong corpus projection: %+v", got)
		}
	}
	restarted, err := atlasops.OpenWorkspace(a)
	if err != nil {
		t.Fatal(err)
	}
	if got := collectPage(t, restarted, page); got.Content != "# Case A\n\n## Evidence\n" {
		t.Fatalf("restart lost workspace knowledge: %+v", got)
	}
}

func TestCopiedAtlasGrantsCannotCrossWorkspaceOrOwnerScope(t *testing.T) {
	for _, direction := range []string{"workspace-to-workspace", "owner-to-workspace", "workspace-to-owner"} {
		t.Run(direction, func(t *testing.T) {
			base := t.TempDir()
			dataRoot := filepath.Join(base, "local")
			a := registeredWorkspace(t, base, dataRoot, "a")
			b := registeredWorkspace(t, base, dataRoot, "b")
			first, err := atlasops.OpenWorkspace(a)
			if err != nil {
				t.Fatal(err)
			}
			second, err := atlasops.OpenWorkspace(b)
			if err != nil {
				t.Fatal(err)
			}
			owner, err := atlasops.Open(dataRoot, nil)
			if err != nil {
				t.Fatal(err)
			}
			source, target := first, second
			if direction == "owner-to-workspace" {
				source, target = owner, first
			}
			if direction == "workspace-to-owner" {
				source, target = first, owner
			}
			created, err := source.CreatePage(atlasops.CreatePageRequest{Page: "knowledge/client/source.md", Body: "# Scoped source\n", Provenance: provenance("copied-operation")})
			if err != nil {
				t.Fatal(err)
			}
			grant, err := source.CreateGrant(standingGrant())
			if err != nil {
				t.Fatal(err)
			}
			sourceJournal := filepath.Join(source.Root(), ".atlasops")
			targetJournal := filepath.Join(target.Root(), ".atlasops")
			if err := os.MkdirAll(targetJournal, 0o700); err != nil {
				t.Fatal(err)
			}
			body, err := os.ReadFile(filepath.Join(sourceJournal, "grants.json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(targetJournal, "grants.json"), body, 0o600); err != nil {
				t.Fatal(err)
			}
			body, err = os.ReadFile(filepath.Join(sourceJournal, created.OperationID+".json"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(targetJournal, created.OperationID+".json"), body, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := target.CreatePage(atlasops.CreatePageRequest{Page: "knowledge/client/source.md", Body: "# Scoped source\n", Provenance: provenance("copied-operation")}); err == nil {
				t.Fatal("copied journal replayed an operation in another scope")
			}
			if _, err := target.Grants(); err == nil {
				t.Fatal("copied grants became inspectable authority in another scope")
			}
			if _, err := target.CreatePage(atlasops.CreatePageRequest{Page: "knowledge/client/new.md", Body: "# Refused\n", Provenance: atlasops.Provenance{Origin: atlasops.OriginGrant, GrantID: grant.GrantID, OccurrenceID: "once", IdempotencyKey: "copy"}}); err == nil {
				t.Fatal("copied grant authorized a page effect")
			}
		})
	}
}

func TestWorkspaceBindingIsRevalidatedBeforeEveryReadAndEffect(t *testing.T) {
	for _, mutation := range []string{"removed-registry", "rebound-registry", "aliased-manifest", "replaced-brain", "aliased-journal"} {
		t.Run(mutation, func(t *testing.T) {
			base := t.TempDir()
			options := registeredWorkspace(t, base, filepath.Join(base, "local"), "case")
			engine, err := atlasops.OpenWorkspace(options)
			if err != nil {
				t.Fatal(err)
			}
			page := "knowledge/client/page.md"
			if _, err := engine.CreatePage(atlasops.CreatePageRequest{Page: page, Body: "# Notes\n\n- **Status:** active\n\n## Evidence\n", Provenance: provenance("create")}); err != nil {
				t.Fatal(err)
			}
			grant, err := engine.CreateGrant(standingGrant())
			if err != nil {
				t.Fatal(err)
			}
			binding := filepath.Join(options.DataRoot, "workspaces", options.WorkspaceID, "binding.json")
			switch mutation {
			case "removed-registry":
				if err := os.Remove(binding); err != nil {
					t.Fatal(err)
				}
			case "rebound-registry":
				body, err := json.Marshal(map[string]any{"schema_version": 1, "workspace_id": options.WorkspaceID, "workspace_path": filepath.Join(base, "different-case")})
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(binding, body, 0o600); err != nil {
					t.Fatal(err)
				}
			case "aliased-manifest":
				manifest := filepath.Join(options.WorkspacePath, ".bcgos", "workspace.json")
				if err := os.Rename(manifest, manifest+".kept"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(manifest+".kept", manifest); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			case "replaced-brain":
				if err := os.Rename(engine.Root(), engine.Root()+".kept"); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(engine.Root(), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(engine.Root(), "README.md"), []byte("# Replacement\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			case "aliased-journal":
				journal := filepath.Join(engine.Root(), ".atlasops")
				if err := os.Rename(journal, journal+".kept"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(journal+".kept", journal); err != nil {
					t.Skipf("symlinks unavailable: %v", err)
				}
			}
			checks := []struct {
				name string
				run  func() error
			}{
				{"collect", func() error {
					_, err := engine.Collect(atlasops.CollectRequest{Purpose: "refuse changed binding", Reader: atlasops.ReaderOwnerSession, Pages: []string{page}})
					return err
				}},
				{"create", func() error {
					_, err := engine.CreatePage(atlasops.CreatePageRequest{Page: "knowledge/client/new.md", Body: "# Refused\n", Provenance: provenance("new")})
					return err
				}},
				{"append", func() error {
					_, err := engine.AppendEntry(atlasops.AppendEntryRequest{Page: page, Section: "## Evidence", Entry: "- refused", Provenance: provenance("append")})
					return err
				}},
				{"set-field", func() error {
					_, err := engine.SetField(atlasops.SetFieldRequest{Page: page, Field: "Status", Value: "refused", Provenance: provenance("field")})
					return err
				}},
				{"link", func() error {
					_, err := engine.Link(atlasops.LinkRequest{Page: page, Section: "## Evidence", Target: "knowledge/client/target.md", Label: "refused", Provenance: provenance("link")})
					return err
				}},
				{"create-grant", func() error { _, err := engine.CreateGrant(standingGrant()); return err }},
				{"grants", func() error { _, err := engine.Grants(); return err }},
				{"grant", func() error { _, _, err := engine.Grant(grant.GrantID); return err }},
				{"authorize", func() error { return engine.AuthorizeGrant(grant.GrantID, "append-entry", page) }},
				{"pause", func() error { return engine.PauseGrant(grant.GrantID) }},
				{"resume", func() error { return engine.ResumeGrant(grant.GrantID) }},
				{"revoke", func() error { return engine.RevokeGrant(grant.GrantID) }},
			}
			for _, check := range checks {
				if mutation == "aliased-journal" && check.name == "collect" {
					continue
				}
				if err := check.run(); err == nil {
					t.Errorf("%s accepted %s", check.name, mutation)
				}
			}
		})
	}
}

func TestWorkspaceOpenRequiresExistingExactRegistrationAndBrain(t *testing.T) {
	base := t.TempDir()
	options := registeredWorkspace(t, base, filepath.Join(base, "local"), "case")
	for _, invalid := range []atlas.Options{
		{WorkspacePath: options.WorkspacePath, WorkspaceID: options.WorkspaceID},
		{DataRoot: options.DataRoot, WorkspaceID: options.WorkspaceID},
		{DataRoot: options.DataRoot, WorkspacePath: options.WorkspacePath},
		{DataRoot: options.DataRoot, WorkspacePath: filepath.Join(base, "another-case"), WorkspaceID: options.WorkspaceID},
	} {
		if _, err := atlasops.OpenWorkspace(invalid); err == nil {
			t.Fatal("workspace open accepted an incomplete or different binding")
		}
	}
	if err := os.RemoveAll(filepath.Join(options.WorkspacePath, "brain")); err != nil {
		t.Fatal(err)
	}
	if _, err := atlasops.OpenWorkspace(options); err == nil {
		t.Fatal("workspace open bootstrapped a missing brain")
	}
	if _, err := os.Lstat(filepath.Join(options.WorkspacePath, "brain")); !os.IsNotExist(err) {
		t.Fatalf("workspace open recreated missing brain: %v", err)
	}
}

func TestStandingGrantCoverageUsesWholePathComponents(t *testing.T) {
	base := t.TempDir()
	options := registeredWorkspace(t, base, filepath.Join(base, "local"), "a")
	engine, err := atlasops.OpenWorkspace(options)
	if err != nil {
		t.Fatal(err)
	}
	grant, err := engine.CreateGrant(standingGrant())
	if err != nil {
		t.Fatal(err)
	}
	// Historical controls can omit the directory's trailing slash.
	path := filepath.Join(engine.Root(), ".atlasops", "grants.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	document["grants"].([]any)[0].(map[string]any)["segment"] = "knowledge/client"
	body, err = json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := engine.AuthorizeGrant(grant.GrantID, "create-page", "knowledge/client/note.md"); err != nil {
		t.Fatal(err)
	}
	if err := engine.AuthorizeGrant(grant.GrantID, "create-page", "knowledge/client-other/note.md"); err == nil {
		t.Fatal("segment authorized a similarly prefixed sibling")
	}
}
