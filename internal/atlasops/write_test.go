package atlasops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	dataRoot := filepath.Join(t.TempDir(), "local", "BCGOS")
	if err := os.MkdirAll(dataRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	engine, err := Open(dataRoot, func() time.Time {
		return time.Date(2026, 8, 11, 9, 0, 0, 0, time.UTC)
	})
	if err != nil {
		t.Fatal(err)
	}
	return engine
}

func attended(key string) Provenance {
	return Provenance{Origin: OriginAttended, SessionID: "session-a", IdempotencyKey: key}
}

func TestCreatePageWritesAndIsIdempotent(t *testing.T) {
	engine := testEngine(t)

	first, err := engine.CreatePage(CreatePageRequest{
		Page:       "development/retros/2026-08-11.md",
		Body:       "# Retro — 2026-08-11\n\n## Evidence\n",
		Provenance: attended("retro-2026-W33"),
	})
	if err != nil {
		t.Fatalf("create page: %v", err)
	}
	if first.State != StateWritten {
		t.Fatalf("first create state = %q, want %q", first.State, StateWritten)
	}
	if first.Revision == "" {
		t.Fatal("first create returned no revision")
	}

	second, err := engine.CreatePage(CreatePageRequest{
		Page:       "development/retros/2026-08-11.md",
		Body:       "# Retro — 2026-08-11\n\n## Evidence\n",
		Provenance: attended("retro-2026-W33"),
	})
	if err != nil {
		t.Fatalf("repeat create page: %v", err)
	}
	if second.State != StateUnchanged {
		t.Fatalf("repeat create state = %q, want %q", second.State, StateUnchanged)
	}
	if second.Revision != first.Revision {
		t.Fatalf("repeat create changed the revision: %q then %q", first.Revision, second.Revision)
	}
}

func TestCreatePagePreservesAnExistingPage(t *testing.T) {
	engine := testEngine(t)
	page := "learnings/category-owner-layer.md"
	handWritten := "# Claim\n\nWritten by the owner directly.\n"

	if _, err := engine.CreatePage(CreatePageRequest{Page: page, Body: handWritten, Provenance: attended("k1")}); err != nil {
		t.Fatal(err)
	}

	result, err := engine.CreatePage(CreatePageRequest{
		Page:       page,
		Body:       "# Claim\n\nGenerated replacement.\n",
		Provenance: attended("k2"),
	})
	if err != nil {
		t.Fatalf("create over existing page: %v", err)
	}
	if result.State != StateUnchanged {
		t.Fatalf("create over existing page state = %q, want %q", result.State, StateUnchanged)
	}

	body, err := os.ReadFile(filepath.Join(engine.Root(), filepath.FromSlash(page)))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != handWritten {
		t.Fatalf("create overwrote an existing page:\n got: %q\nwant: %q", body, handWritten)
	}
}

func TestAppendEntryInsertsUnderTheNamedSection(t *testing.T) {
	engine := testEngine(t)
	page := "development/objectives.md"
	if _, err := engine.CreatePage(CreatePageRequest{
		Page:       page,
		Body:       "# Objectives\n\n## Evidence\n\n## Closed\n- nothing yet\n",
		Provenance: attended("k1"),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := engine.AppendEntry(AppendEntryRequest{
		Page:       page,
		Section:    "## Evidence",
		Entry:      "- 2026-08-11 — led the steering committee without the partner",
		Provenance: attended("evidence-1"),
	})
	if err != nil {
		t.Fatalf("append entry: %v", err)
	}
	if result.State != StateWritten {
		t.Fatalf("append state = %q, want %q", result.State, StateWritten)
	}

	body, err := os.ReadFile(filepath.Join(engine.Root(), filepath.FromSlash(page)))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	evidence := strings.Index(text, "## Evidence")
	entry := strings.Index(text, "led the steering committee")
	closed := strings.Index(text, "## Closed")
	if entry < evidence || entry > closed {
		t.Fatalf("entry landed outside its section:\n%s", text)
	}
	if !strings.Contains(text, "- nothing yet") {
		t.Fatalf("append dropped content from another section:\n%s", text)
	}
}

func TestAppendEntryIsIdempotentPerKey(t *testing.T) {
	engine := testEngine(t)
	page := "development/objectives.md"
	if _, err := engine.CreatePage(CreatePageRequest{Page: page, Body: "# Objectives\n\n## Evidence\n", Provenance: attended("k1")}); err != nil {
		t.Fatal(err)
	}
	request := AppendEntryRequest{
		Page:       page,
		Section:    "## Evidence",
		Entry:      "- 2026-08-11 — one occurrence only",
		Provenance: attended("evidence-1"),
	}

	if _, err := engine.AppendEntry(request); err != nil {
		t.Fatal(err)
	}
	second, err := engine.AppendEntry(request)
	if err != nil {
		t.Fatalf("repeat append: %v", err)
	}
	if second.State != StateUnchanged {
		t.Fatalf("repeat append state = %q, want %q", second.State, StateUnchanged)
	}

	body, err := os.ReadFile(filepath.Join(engine.Root(), filepath.FromSlash(page)))
	if err != nil {
		t.Fatal(err)
	}
	if occurrences := strings.Count(string(body), "one occurrence only"); occurrences != 1 {
		t.Fatalf("entry appears %d times, want 1:\n%s", occurrences, body)
	}
}

func TestAppendEntryReturnsAProposalOnRevisionConflict(t *testing.T) {
	engine := testEngine(t)
	page := "development/objectives.md"
	created, err := engine.CreatePage(CreatePageRequest{Page: page, Body: "# Objectives\n\n## Evidence\n", Provenance: attended("k1")})
	if err != nil {
		t.Fatal(err)
	}

	// The owner edits the page by hand after the caller read it.
	full := filepath.Join(engine.Root(), filepath.FromSlash(page))
	if err := os.WriteFile(full, []byte("# Objectives\n\n## Evidence\n- edited by hand\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := engine.AppendEntry(AppendEntryRequest{
		Page:             page,
		Section:          "## Evidence",
		Entry:            "- appended against a stale revision",
		ExpectedRevision: created.Revision,
		Provenance:       attended("evidence-2"),
	})
	if err != nil {
		t.Fatalf("conflicting append returned an error instead of a proposal: %v", err)
	}
	if result.State != StateProposed {
		t.Fatalf("conflict state = %q, want %q", result.State, StateProposed)
	}
	if result.Proposal == "" {
		t.Fatal("conflict returned no reviewable proposal")
	}

	body, err := os.ReadFile(full)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "stale revision") {
		t.Fatalf("conflicting append clobbered the hand edit:\n%s", body)
	}
	if !strings.Contains(string(body), "edited by hand") {
		t.Fatalf("hand edit was lost:\n%s", body)
	}
}

// A page may legitimately repeat a heading — an objectives page with an
// evidence section under each objective is the obvious case. Landing on the
// first match would put the entry under the wrong objective and report success,
// which is worse than refusing.
func TestAppendEntryRefusesAnAmbiguousSection(t *testing.T) {
	engine := testEngine(t)
	page := "development/objectives.md"
	if _, err := engine.CreatePage(CreatePageRequest{
		Page: page,
		Body: "# Objectives\n\n## Lead without the partner\n\n### Evidence\n\n## Sharpen the recommendation\n\n### Evidence\n",
		Provenance: attended("k1"),
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := engine.AppendEntry(AppendEntryRequest{
		Page:       page,
		Section:    "### Evidence",
		Entry:      "- which objective is this?",
		Provenance: attended("evidence-1"),
	}); err == nil {
		t.Fatal("append resolved a heading that appears twice; it must refuse rather than guess an objective")
	}

	// The page is untouched: an ambiguous target writes nothing.
	body, err := engine.read(page)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "which objective") {
		t.Fatalf("refused append still wrote:\n%s", body)
	}
}

func TestAppendEntryRequiresAnExistingSection(t *testing.T) {
	engine := testEngine(t)
	page := "development/objectives.md"
	if _, err := engine.CreatePage(CreatePageRequest{Page: page, Body: "# Objectives\n\n## Evidence\n", Provenance: attended("k1")}); err != nil {
		t.Fatal(err)
	}

	if _, err := engine.AppendEntry(AppendEntryRequest{
		Page:       page,
		Section:    "## Absent",
		Entry:      "- nowhere to go",
		Provenance: attended("evidence-3"),
	}); err == nil {
		t.Fatal("append invented a section that the page does not declare")
	}
}

func TestWritesRejectPathsOutsideTheOwnerRoot(t *testing.T) {
	engine := testEngine(t)
	for _, page := range []string{"../escape.md", "/absolute.md", "development/../../escape.md", ""} {
		if _, err := engine.CreatePage(CreatePageRequest{Page: page, Body: "x\n", Provenance: attended("k")}); err == nil {
			t.Fatalf("create accepted an out-of-root page path: %q", page)
		}
	}
}

func TestWritesRequireProvenance(t *testing.T) {
	engine := testEngine(t)
	if _, err := engine.CreatePage(CreatePageRequest{Page: "index.md", Body: "x\n"}); err == nil {
		t.Fatal("create accepted a write with no provenance")
	}
}
