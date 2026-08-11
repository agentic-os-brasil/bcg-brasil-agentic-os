package atlasops

import (
	"strings"
	"testing"
)

func seedPages(t *testing.T, engine *Engine, pages map[string]string) {
	t.Helper()
	index := 0
	for page, body := range pages {
		index++
		if _, err := engine.CreatePage(CreatePageRequest{
			Page:       page,
			Body:       body,
			Provenance: attended("seed-" + page),
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCollectReturnsNamedPagesForTheOwnerSession(t *testing.T) {
	engine := testEngine(t)
	seedPages(t, engine, map[string]string{
		"development/objectives.md": "# Objectives\n\n## Evidence\n- led the steering committee\n",
		"learnings/index.md":        "# Learnings\n",
	})

	projection, err := engine.Collect(CollectRequest{
		Purpose: "compose the weekly retrospective",
		Reader:  ReaderOwnerSession,
		Pages:   []string{"development/objectives.md"},
	})
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(projection.Pages) != 1 {
		t.Fatalf("projection carried %d pages, want exactly the one that was named", len(projection.Pages))
	}
	page := projection.Pages[0]
	if !strings.Contains(page.Content, "led the steering committee") {
		t.Fatalf("owner session did not receive the page body: %+v", page)
	}
	if page.Revision == "" {
		t.Fatal("projection carried no revision, so a later write cannot detect a conflict")
	}
	if page.Attenuated {
		t.Fatal("the owner session received an attenuated projection of their own atlas")
	}
}

func TestCollectRefusesAnUnnamedWholeRootRequest(t *testing.T) {
	engine := testEngine(t)
	seedPages(t, engine, map[string]string{"learnings/index.md": "# Learnings\n"})

	if _, err := engine.Collect(CollectRequest{
		Purpose: "everything",
		Reader:  ReaderOwnerSession,
	}); err == nil {
		t.Fatal("collect returned a projection for an unnamed request; a whole-root dump must be refused")
	}
}

func TestCollectRequiresADeclaredPurposeAndKnownReader(t *testing.T) {
	engine := testEngine(t)
	seedPages(t, engine, map[string]string{"learnings/index.md": "# Learnings\n"})
	pages := []string{"learnings/index.md"}

	if _, err := engine.Collect(CollectRequest{Reader: ReaderOwnerSession, Pages: pages}); err == nil {
		t.Fatal("collect accepted a request with no declared purpose")
	}
	if _, err := engine.Collect(CollectRequest{Purpose: "p", Reader: "auditor", Pages: pages}); err == nil {
		t.Fatal("collect accepted an unknown reader")
	}
}

func TestCollectAttenuatesForADelegateAndRequiresAuthorization(t *testing.T) {
	engine := testEngine(t)
	body := "# Claim\n\nProcurement transformations stall at the category-owner layer.\n"
	seedPages(t, engine, map[string]string{"learnings/category-owner-layer.md": body})
	pages := []string{"learnings/category-owner-layer.md"}

	if _, err := engine.Collect(CollectRequest{
		Purpose: "frame the client conversation",
		Reader:  ReaderDelegate,
		Pages:   pages,
	}); err == nil {
		t.Fatal("a delegate reader received owner content without explicit authorization")
	}

	projection, err := engine.Collect(CollectRequest{
		Purpose:    "frame the client conversation",
		Reader:     ReaderDelegate,
		Pages:      pages,
		Authorized: true,
	})
	if err != nil {
		t.Fatalf("authorized delegate collect: %v", err)
	}
	page := projection.Pages[0]
	if !page.Attenuated {
		t.Fatal("an authorized delegate received an unattenuated projection")
	}
	if page.Content != "" {
		t.Fatalf("a delegate received the page body: %q", page.Content)
	}
	if page.Pointer == "" {
		t.Fatal("the delegate received neither a body nor a pointer, so the projection is useless")
	}
}

func TestCollectReportsAMissingPageAsAnOmission(t *testing.T) {
	engine := testEngine(t)
	seedPages(t, engine, map[string]string{"learnings/index.md": "# Learnings\n"})

	projection, err := engine.Collect(CollectRequest{
		Purpose: "compose the weekly retrospective",
		Reader:  ReaderMaestro,
		Pages:   []string{"learnings/index.md", "development/objectives.md"},
	})
	if err != nil {
		t.Fatalf("collect with a missing page returned an error instead of an omission: %v", err)
	}
	if len(projection.Pages) != 1 {
		t.Fatalf("projection carried %d pages, want the one that exists", len(projection.Pages))
	}
	if len(projection.Omissions) != 1 || projection.Omissions[0].Page != "development/objectives.md" {
		t.Fatalf("the missing page was not reported as an omission: %+v", projection.Omissions)
	}
}

func TestCollectRefusesToReachOutsideTheOwnerRoot(t *testing.T) {
	engine := testEngine(t)
	for _, page := range []string{"../escape.md", "/absolute.md", ".atlasops/journal.json"} {
		if _, err := engine.Collect(CollectRequest{
			Purpose: "p",
			Reader:  ReaderOwnerSession,
			Pages:   []string{page},
		}); err == nil {
			t.Fatalf("collect accepted an out-of-root page: %q", page)
		}
	}
}
