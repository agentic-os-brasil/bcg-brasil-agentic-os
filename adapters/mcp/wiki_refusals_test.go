package product_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenance"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

func TestPrivateWikiQueryReferencesCannotCrossRegisteredWorkspaces(t *testing.T) {
	f := newOwnerFixture(t, false)
	prepareMemoryFixture(t, f)
	one := f.connect(t, cadenceApprove)
	memoryCall(t, one, "memory.capture", map[string]any{"text": "private-wiki-body: FIRST_REGISTERED_SCOPE"}, false)
	memoryCall(t, one, "memory.consolidate", map[string]any{"cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")}, false)
	cadenceCall(t, one, "qualify", map[string]any{"job_id": "wiki-incremental-sync", "expected_revision": 0}, false)
	first := wikiCall(t, one, "status", nil, false)
	firstGeneration := first["generation"].(string)
	index := wikiCall(t, one, "read", map[string]any{"generation": firstGeneration, "reference": "index.md"}, false)["content"].(string)
	concept := regexp.MustCompile(`\((concepts/[a-f0-9]{64}\.md)\)`).FindStringSubmatch(index)
	if len(concept) != 2 {
		t.Fatal("first concept absent")
	}
	body := wikiCall(t, one, "read", map[string]any{"generation": firstGeneration, "reference": concept[1]}, false)["content"].(string)
	source := regexp.MustCompile(`maestro://memory/[a-f0-9]{64}`).FindString(body)
	other := f
	other.options.WorkspacePath = filepath.Join(filepath.Dir(f.options.WorkspacePath), "other-registered-workspace")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: other.options.WorkspacePath, DataRoot: filepath.Join(filepath.Dir(f.root), "state")}); err != nil {
		t.Fatal(err)
	}
	prepareMemoryFixture(t, other)
	two := other.connect(t, cadenceApprove)
	memoryCall(t, two, "memory.capture", map[string]any{"text": "private-wiki-body: SECOND_REGISTERED_SCOPE"}, false)
	memoryCall(t, two, "memory.consolidate", map[string]any{"cycle": "daily", "date": time.Now().UTC().Format("2006-01-02")}, false)
	cadenceCall(t, two, "qualify", map[string]any{"job_id": "wiki-incremental-sync", "expected_revision": 0}, false)
	second := wikiCall(t, two, "status", nil, false)
	if firstGeneration == second["generation"] {
		t.Fatal("registered scopes shared graph identity")
	}
	before := cadenceTree(t, f)
	wikiCall(t, two, "read", map[string]any{"generation": firstGeneration, "reference": concept[1]}, true)
	wikiCall(t, two, "source", map[string]any{"generation": second["generation"], "reference": source}, true)
	if cadenceTree(t, f) != before {
		t.Fatal("cross-workspace read refusal changed owner state")
	}
}

func TestPrivateWikiQueriesAreClosedAndReadOnly(t *testing.T) {
	for _, transport := range []string{"sdk", "stdio"} {
		t.Run(transport, func(t *testing.T) {
			f := newOwnerFixture(t, transport == "stdio")
			prepareMemoryFixture(t, f)
			cs := f.connect(t, nil)
			before := cadenceTree(t, f)
			wikiCall(t, cs, "status", nil, true)
			generation := strings.Repeat("a", 64)
			for _, fields := range []map[string]any{
				{"generation": generation, "reference": "../source.md"},
				{"generation": generation, "reference": "manifest.json"},
				{"generation": generation, "reference": "index.md", "max_bytes": 4097},
				{"generation": generation, "reference": "index.md", "max_bytes": 0},
				{"generation": generation, "reference": "index.md", "offset": 16777217},
				{"reference": "index.md"},
				{"generation": generation, "reference": "index.md", "workspace_id": "other"},
				{"generation": generation, "reference": "index.md", "root": f.root},
				{"generation": generation, "reference": "index.md", "approved": true},
				{"generation": generation, "reference": "index.md", "event_id": "invented-event"},
			} {
				wikiCall(t, cs, "read", fields, true)
			}
			wikiCall(t, cs, "status", map[string]any{"workspace_id": "other"}, true)
			wikiCall(t, cs, "source", map[string]any{"generation": generation, "reference": "maestro://owner/private"}, true)
			if cadenceTree(t, f) != before {
				t.Fatal("query/schema refusal initialized or repaired private state")
			}
		})
	}
}

func TestPrivateWikiQueriesHideCorrectedDeletedAndInterruptedViews(t *testing.T) {
	for _, transport := range []string{"sdk", "stdio"} {
		t.Run(transport, func(t *testing.T) {
			f := newOwnerFixture(t, transport == "stdio")
			prepareMemoryFixture(t, f)
			cs := f.connect(t, cadenceApprove)
			date := time.Now().UTC().Format("2006-01-02")
			capture := memoryCall(t, cs, "memory.capture", map[string]any{"text": "private-wiki-body: ORIGINAL_DENIED_VIEW"}, false)
			memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": date}, false)
			cadenceCall(t, cs, "qualify", map[string]any{"job_id": "wiki-incremental-sync", "expected_revision": 0}, false)
			old := wikiCall(t, cs, "status", nil, false)
			oldGeneration := old["generation"].(string)
			index := wikiCall(t, cs, "read", map[string]any{"generation": oldGeneration, "reference": "index.md"}, false)["content"].(string)
			match := regexp.MustCompile(`\((concepts/[a-f0-9]{64}\.md)\)`).FindStringSubmatch(index)
			if len(match) != 2 {
				t.Fatal("concept route absent")
			}
			concept := wikiCall(t, cs, "read", map[string]any{"generation": oldGeneration, "reference": match[1]}, false)["content"].(string)
			sourceReference := regexp.MustCompile(`maestro://memory/[a-f0-9]{64}`).FindString(concept)
			refuseOld := func(generation, concept, source string) {
				t.Helper()
				before := cadenceTree(t, f)
				for _, ref := range []string{"index.md", "backlinks.json", concept} {
					out := wikiCall(t, cs, "read", map[string]any{"generation": generation, "reference": ref}, true)
					if strings.Contains(reportJSON(t, out), "private-wiki-body") {
						t.Fatal("refused query leaked a source body")
					}
				}
				wikiCall(t, cs, "source", map[string]any{"generation": generation, "reference": source}, true)
				if cadenceTree(t, f) != before {
					t.Fatal("refused stale queries changed owner state")
				}
			}
			corrected := memoryCall(t, cs, "memory.correct", map[string]any{"capture_id": capture["id"], "expected_revision": 1, "text": "private-wiki-body: REPLACEMENT_CURRENT_VIEW"}, false)
			refuseOld(oldGeneration, match[1], sourceReference)
			memoryCall(t, cs, "memory.consolidate", map[string]any{"cycle": "daily", "date": date}, false)
			e, _, err := maintenance.NewProductMemoryEngine(filepath.Dir(f.root))
			if err != nil {
				t.Fatal(err)
			}
			workspace := old["workspace_id"]
			if workspace == nil {
				workspace = cadenceCall(t, cs, "status", nil, false)["workspace_id"]
			}
			id := workspace.(string)
			e.FaultPoint = func(point string) error {
				if point == "wiki_before_pointer" {
					return errors.New("controlled compile interruption")
				}
				return nil
			}
			if _, err := e.CompilePrivateWiki(context.Background(), id, ""); err == nil {
				t.Fatal("controlled compile failure did not run")
			}
			refuseOld(oldGeneration, match[1], sourceReference)
			e.FaultPoint = nil
			cadenceCall(t, cs, "qualify", map[string]any{"job_id": "wiki-reconcile", "expected_revision": 0}, false)
			current := wikiCall(t, cs, "status", nil, false)
			generation := current["generation"].(string)
			index = wikiCall(t, cs, "read", map[string]any{"generation": generation, "reference": "index.md"}, false)["content"].(string)
			match = regexp.MustCompile(`\((concepts/[a-f0-9]{64}\.md)\)`).FindStringSubmatch(index)
			if len(match) != 2 {
				t.Fatal("current concept route absent")
			}
			concept = wikiCall(t, cs, "read", map[string]any{"generation": generation, "reference": match[1]}, false)["content"].(string)
			if strings.Contains(concept, "ORIGINAL_DENIED_VIEW") || !strings.Contains(concept, "REPLACEMENT_CURRENT_VIEW") {
				t.Fatal("reconciliation recovered corrected text")
			}
			sourceReference = regexp.MustCompile(`maestro://memory/[a-f0-9]{64}`).FindString(concept)
			e.FaultPoint = func(point string) error {
				if point == "wiki_before_retirement" {
					return errors.New("controlled cleanup interruption")
				}
				return nil
			}
			if _, err := e.DeleteCapture(id, capture["id"].(string), uint64(corrected["revision"].(float64))); err == nil {
				t.Fatal("controlled physical retirement failure did not run")
			}
			refuseOld(generation, match[1], sourceReference)
			status := wikiCall(t, cs, "status", nil, false)
			if status["state"] != "retirement_pending" || status["implementation_current"] != false {
				t.Fatal("pending cleanup became a current view")
			}
			e.FaultPoint = nil
			if _, err := e.CompilePrivateWiki(context.Background(), id, ""); err == nil {
				t.Fatal("source-free retirement became a successful compilation")
			}
			if _, err := os.Lstat(filepath.Join(f.root, "memory", "workspaces", id, ".private-wiki")); !os.IsNotExist(err) {
				t.Fatal("owning retry retained old private graph bodies")
			}
			refuseOld(generation, match[1], sourceReference)
		})
	}
}
