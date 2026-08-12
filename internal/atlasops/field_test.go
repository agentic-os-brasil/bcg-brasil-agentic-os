package atlasops

import (
	"strings"
	"testing"
)

const methodPage = `# Method: stakeholder map before kickoff

## Snapshot
- **Maturity:** draft
- **Last used:** 2026-06-02

## Steps
1. List every decision the engagement needs.
2. Name who signs each one.

## Evidence of use
- 2026-06-02 — used on the procurement diagnostic

## Related
- [Craft index](../index.md)
`

func seedMethod(t *testing.T, engine *Engine) string {
	t.Helper()
	page := "craft/methods/stakeholder-map.md"
	if _, err := engine.CreatePage(CreatePageRequest{Page: page, Body: methodPage, Provenance: attended("seed")}); err != nil {
		t.Fatal(err)
	}
	return page
}

func TestSetFieldReplacesOnlyTheNamedField(t *testing.T) {
	engine := testEngine(t)
	page := seedMethod(t, engine)

	result, err := engine.SetField(SetFieldRequest{
		Page:       page,
		Field:      "Maturity",
		Value:      "working",
		Provenance: attended("maturity-1"),
	})
	if err != nil {
		t.Fatalf("set field: %v", err)
	}
	if result.State != StateWritten {
		t.Fatalf("state = %q, want %q", result.State, StateWritten)
	}

	body, err := engine.read(page)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "- **Maturity:** working") {
		t.Fatalf("field was not set:\n%s", body)
	}
	// Everything the owner wrote by hand survives untouched.
	for _, kept := range []string{
		"- **Last used:** 2026-06-02",
		"1. List every decision the engagement needs.",
		"- 2026-06-02 — used on the procurement diagnostic",
		"# Method: stakeholder map before kickoff",
	} {
		if !strings.Contains(body, kept) {
			t.Fatalf("set-field disturbed hand-authored content, lost %q:\n%s", kept, body)
		}
	}
}

func TestSetFieldIsIdempotentAndReportsAnUnchangedValue(t *testing.T) {
	engine := testEngine(t)
	page := seedMethod(t, engine)
	request := SetFieldRequest{Page: page, Field: "Maturity", Value: "working", Provenance: attended("maturity-1")}

	if _, err := engine.SetField(request); err != nil {
		t.Fatal(err)
	}
	second, err := engine.SetField(request)
	if err != nil {
		t.Fatalf("repeat set field: %v", err)
	}
	if second.State != StateUnchanged {
		t.Fatalf("repeat state = %q, want %q", second.State, StateUnchanged)
	}

	// A different key carrying the value the page already holds is also a no-op:
	// the page is already where the caller wants it.
	same, err := engine.SetField(SetFieldRequest{Page: page, Field: "Maturity", Value: "working", Provenance: attended("maturity-2")})
	if err != nil {
		t.Fatal(err)
	}
	if same.State != StateUnchanged {
		t.Fatalf("setting the value it already holds reported %q, want %q", same.State, StateUnchanged)
	}
}

func TestSetFieldRefusesAnAbsentOrAmbiguousField(t *testing.T) {
	engine := testEngine(t)
	page := seedMethod(t, engine)

	if _, err := engine.SetField(SetFieldRequest{
		Page: page, Field: "Owner", Value: "someone", Provenance: attended("k1"),
	}); err == nil {
		t.Fatal("set-field invented a field the page does not declare")
	}

	ambiguous := "craft/methods/ambiguous.md"
	if _, err := engine.CreatePage(CreatePageRequest{
		Page:       ambiguous,
		Body:       "# M\n\n## A\n- **Status:** one\n\n## B\n- **Status:** two\n",
		Provenance: attended("k2"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SetField(SetFieldRequest{
		Page: ambiguous, Field: "Status", Value: "three", Provenance: attended("k3"),
	}); err == nil {
		t.Fatal("set-field resolved a field that appears twice; it must refuse rather than guess")
	}
}

func TestSetFieldReturnsAProposalOnRevisionConflict(t *testing.T) {
	engine := testEngine(t)
	page := seedMethod(t, engine)
	stale, err := engine.Collect(CollectRequest{Purpose: "read", Reader: ReaderOwnerSession, Pages: []string{page}})
	if err != nil {
		t.Fatal(err)
	}
	before := stale.Pages[0].Revision

	if _, err := engine.AppendEntry(AppendEntryRequest{
		Page: page, Section: "## Evidence of use", Entry: "- 2026-08-11 — the owner edited this", Provenance: attended("hand"),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := engine.SetField(SetFieldRequest{
		Page: page, Field: "Maturity", Value: "working",
		ExpectedRevision: before, Provenance: attended("maturity-1"),
	})
	if err != nil {
		t.Fatalf("conflict returned an error instead of a proposal: %v", err)
	}
	if result.State != StateProposed || result.Proposal == "" {
		t.Fatalf("conflict state = %q with proposal %q", result.State, result.Proposal)
	}
	body, err := engine.read(page)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(body, "**Maturity:** working") {
		t.Fatalf("conflicting set-field wrote anyway:\n%s", body)
	}
}

func TestLinkAddsAReferenceAndIsANoOpWhenRepeated(t *testing.T) {
	engine := testEngine(t)
	page := seedMethod(t, engine)
	target := "learnings/category-owner-layer.md"

	first, err := engine.Link(LinkRequest{
		Page:       page,
		Section:    "## Related",
		Target:     target,
		Label:      "Category owner layer",
		Provenance: attended("link-1"),
	})
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	if first.State != StateWritten {
		t.Fatalf("state = %q, want %q", first.State, StateWritten)
	}

	body, err := engine.read(page)
	if err != nil {
		t.Fatal(err)
	}
	// The reference is relative to the page holding it, not to the root, so it
	// resolves when someone opens that page directly in an editor.
	reference := "../../learnings/category-owner-layer.md"
	if !strings.Contains(body, "[Category owner layer]("+reference+")") {
		t.Fatalf("link was not written in the resolvable form:\n%s", body)
	}

	repeated, err := engine.Link(LinkRequest{
		Page: page, Section: "## Related", Target: target,
		Label: "A different label", Provenance: attended("link-2"),
	})
	if err != nil {
		t.Fatalf("repeat link: %v", err)
	}
	if repeated.State != StateUnchanged {
		t.Fatalf("repeat link state = %q, want %q", repeated.State, StateUnchanged)
	}
	after, err := engine.read(page)
	if err != nil {
		t.Fatal(err)
	}
	if occurrences := strings.Count(after, reference); occurrences != 1 {
		t.Fatalf("target appears %d times after a repeat, want 1:\n%s", occurrences, after)
	}
}

func TestLinkRefusesATargetOutsideTheOwnerRoot(t *testing.T) {
	engine := testEngine(t)
	page := seedMethod(t, engine)
	for _, target := range []string{"../escape.md", "/absolute.md", "https://example.com/page"} {
		if _, err := engine.Link(LinkRequest{
			Page: page, Section: "## Related", Target: target, Label: "x", Provenance: attended("k"),
		}); err == nil {
			t.Fatalf("link accepted a target outside the owner root: %q", target)
		}
	}
}

func TestFieldAndLinkAreEnforcedByAGrant(t *testing.T) {
	engine := testEngine(t)
	request := weeklyRetro()
	request.Operations = []string{"create-page", "append-entry", "set-field", "link"}
	grant, err := engine.CreateGrant(request)
	if err != nil {
		t.Fatal(err)
	}
	granted := func(key string) Provenance {
		return Provenance{Origin: OriginGrant, GrantID: grant.GrantID, OccurrenceID: "w33", IdempotencyKey: key}
	}

	page := "development/retros/2026-08-11.md"
	if _, err := engine.CreatePage(CreatePageRequest{
		Page: page, Body: "# Retro\n\n## Snapshot\n- **Status:** open\n\n## Related\n", Provenance: granted("k1"),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SetField(SetFieldRequest{
		Page: page, Field: "Status", Value: "closed", Provenance: granted("k2"),
	}); err != nil {
		t.Fatalf("granted set-field inside the segment was refused: %v", err)
	}

	if err := engine.RevokeGrant(grant.GrantID); err != nil {
		t.Fatal(err)
	}
	if _, err := engine.SetField(SetFieldRequest{
		Page: page, Field: "Status", Value: "reopened", Provenance: granted("k3"),
	}); err == nil {
		t.Fatal("set-field citing a revoked grant was accepted")
	}
	if _, err := engine.Link(LinkRequest{
		Page: page, Section: "## Related", Target: "learnings/x.md", Label: "x", Provenance: granted("k4"),
	}); err == nil {
		t.Fatal("link citing a revoked grant was accepted")
	}
}
