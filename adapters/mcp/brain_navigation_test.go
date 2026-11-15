package product_test

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// This journey belongs to the existing authored-brain public parent: both the
// SDK and the emitted stdio binary must publish and cold-read the same corpus.
func authoredNavigationJourney(t *testing.T, f ownerFixture, cs *mcp.ClientSession, workspace string) {
	t.Helper()
	if err := cs.Close(); err != nil {
		t.Fatal(err)
	}
	approve := func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	}
	cs = f.connect(t, approve)
	const a, b, missing = "projects/navigation.md", "people/navigation.md", "projects/missing.md"
	aText := authoredBrief(workspace, a, "Navegação Ágil") + "\nMarcadorUnicode café. [Person](../people/navigation.md) [Missing](missing.md) [Unchecked](outside.md)\n"
	bText := authoredBrief(workspace, b, "Navigation person")
	brainCall(t, cs, "create", map[string]any{"page": a, "text": aText}, false)
	brainCall(t, cs, "create", map[string]any{"page": b, "text": bText}, false)
	compile := func(session *mcp.ClientSession, pages []string, fail bool) map[string]any {
		return contentCall(t, session, "maestro_apply", map[string]any{"operation": "brain.compile", "pages": pages, "purpose": "Compile only this reviewed authored cohort"}, fail)
	}
	compiled := compile(cs, []string{a, b, missing}, false)
	generation, ok := compiled["generation"].(string)
	if !ok || len(generation) != 64 || compiled["state"] != "written" {
		t.Fatal("immutable authored generation unavailable", compiled)
	}
	if err := cs.Close(); err != nil {
		t.Fatal(err)
	}
	cs = f.connect(t, approve)
	search := brainCall(t, cs, "search", map[string]any{"generation": generation, "query": "CAFÉ", "purpose": "Recover the named authored cohort"}, false)
	encoded, _ := json.Marshal(search)
	if !strings.Contains(string(encoded), a) || strings.Contains(string(encoded), "outside.md") {
		t.Fatal("search lost Unicode match or enumerated unchecked target", search)
	}
	bounded := brainCall(t, cs, "search", map[string]any{"generation": generation, "query": "Navigation", "purpose": "Keep serialized search results bounded", "max_bytes": 1024, "limit": 1}, false)
	boundedBytes, _ := json.Marshal(bounded)
	if len(boundedBytes) > 1024 || len(bounded["matches"].([]any)) != 1 {
		t.Fatal("search ignored its result budget", bounded)
	}
	nav := brainCall(t, cs, "navigate", map[string]any{"generation": generation, "reference": "pages/" + b, "purpose": "Read the person backlinks"}, false)
	if !strings.Contains(nav["text"].(string), a) {
		t.Fatal("backlink missing after restart", nav)
	}
	diagnostics := brainCall(t, cs, "diagnostics", map[string]any{"generation": generation, "purpose": "Inspect bounded cohort diagnostics"}, false)
	raw, _ := json.Marshal(diagnostics)
	if !strings.Contains(string(raw), "broken_link") || strings.Contains(string(raw), "outside.md") {
		t.Fatal("broken link diagnosis exceeded admitted cohort", diagnostics)
	}
	repeated := compile(cs, []string{missing, b, a}, false)
	if repeated["state"] != "unchanged" || repeated["generation"] != generation {
		t.Fatal("same source compilation was not a no-op", repeated)
	}
	current := brainCall(t, cs, "read", map[string]any{"page": a, "purpose": "Read the exact page before correction"}, false)
	brainCall(t, cs, "update", map[string]any{"page": a, "text": strings.ReplaceAll(aText, "MarcadorUnicode café", "Corrected current material"), "expected_page_revision": current["revision"]}, false)
	brainCall(t, cs, "search", map[string]any{"generation": generation, "query": "CAFÉ", "purpose": "Old material must be fenced after correction"}, true)
	fresh := compile(cs, []string{a, b, missing}, false)
	if fresh["generation"] == generation {
		t.Fatal("changed source retained old generation", fresh)
	}
	brainCall(t, cs, "navigate", map[string]any{"generation": generation, "reference": "index.md", "purpose": "Refuse retired generation pointers"}, true)

	freshGeneration := fresh["generation"].(string)
	for offset := 0; ; {
		page := brainCall(t, cs, "navigate", map[string]any{"generation": freshGeneration, "reference": "pages/" + a, "purpose": "Read a bounded UTF-8 navigation page", "offset": offset, "max_bytes": 97}, false)
		text := page["text"].(string)
		if !utf8.ValidString(text) || len(text) > 97 {
			t.Fatal("navigation split a Unicode rune", page)
		}
		next := int(page["next_offset"].(float64))
		if next < 0 {
			break
		}
		if next <= offset {
			t.Fatal("navigation did not advance", page)
		}
		offset = next
	}
	full := brainCall(t, cs, "navigate", map[string]any{"generation": freshGeneration, "reference": "pages/" + a, "purpose": "Inspect Unicode continuation boundary"}, false)
	accent := strings.Index(full["text"].(string), "ã")
	if accent < 0 {
		t.Fatal("Unicode fixture unavailable")
	}
	brainCall(t, cs, "navigate", map[string]any{"generation": freshGeneration, "reference": "pages/" + a, "purpose": "Reject continuation inside a Unicode rune", "offset": accent + 1}, true)
	before := contentTree(t, f.options.WorkspacePath, false)
	cs = f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		return &mcp.ElicitResult{Action: "decline"}, nil
	})
	compile(cs, []string{a}, true)
	for _, path := range []string{"../outside.md", "owner/self/secret.md", "memory/private.md", ".atlasops/private.md", "projects\\alias.md"} {
		compile(cs, []string{path}, true)
	}
	if !reflect.DeepEqual(before, contentTree(t, f.options.WorkspacePath, false)) {
		t.Fatal("declined or forbidden compilation changed files")
	}
	pointerPath := filepath.Join(f.options.WorkspacePath, "brain", ".atlasops", "navigation-current.json")
	pointerBefore, err := os.ReadFile(pointerPath)
	if err != nil {
		t.Fatal(err)
	}
	cs = f.connect(t, func(context.Context, *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		file := filepath.Join(f.options.WorkspacePath, "brain", filepath.FromSlash(a))
		text, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		if err = os.WriteFile(file, append(text, []byte("\nChanged during native review.\n")...), 0600); err != nil {
			return nil, err
		}
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	})
	compile(cs, []string{a}, true)
	pointerAfter, err := os.ReadFile(pointerPath)
	if err != nil || string(pointerBefore) != string(pointerAfter) {
		t.Fatal("stale approved review changed current generation", err)
	}
	brainCall(t, cs, "search", map[string]any{"generation": freshGeneration, "query": "Navigation", "purpose": "Do not disclose changed source snapshots"}, true)
	// Follow actual emitted standard Markdown hrefs, rather than constructing a
	// guessed source selector. Percent-literal and fragment names stay distinct.
	cs = f.connect(t, approve)
	reservedPages := []string{"projects/hash#part.md", "projects/hash%23part.md", "projects/space name.md", "projects/round(name).md", "projects/label[one].md"}
	for _, page := range reservedPages {
		brainCall(t, cs, "create", map[string]any{"page": page, "text": authoredBrief(workspace, page, "Reserved navigation")}, false)
	}
	reservedGeneration := compile(cs, reservedPages, false)["generation"].(string)
	directory := brainCall(t, cs, "navigate", map[string]any{"generation": reservedGeneration, "reference": "indexes/projects/index.md", "purpose": "Follow exact emitted reserved-name links"}, false)
	reached := map[string]bool{}
	for _, match := range regexp.MustCompile(`\]\(([^)\r\n]+)\)`).FindAllStringSubmatch(directory["text"].(string), -1) {
		base := &url.URL{Path: "indexes/projects/index.md"}
		href, err := url.Parse(match[1])
		if err != nil {
			t.Fatal(err)
		}
		resolved := base.ResolveReference(href)
		reference := strings.TrimPrefix(resolved.EscapedPath(), "/")
		document := brainCall(t, cs, "navigate", map[string]any{"generation": reservedGeneration, "reference": reference, "purpose": "Read the virtual document selected by its emitted href"}, false)
		if document["reference"] != reference {
			t.Fatal("virtual selector was silently reinterpreted", document)
		}
		for _, page := range reservedPages {
			if strings.Contains(document["text"].(string), "Source: `"+page+"`") {
				reached[page] = true
			}
		}
	}
	if len(reached) != len(reservedPages) {
		t.Fatal("reserved source documents were merged or unreachable", reached)
	}
	brainCall(t, cs, "navigate", map[string]any{"generation": reservedGeneration, "reference": "pages/projects/hash#part.md", "purpose": "Reject ambiguous unescaped fragment selector"}, true)

}
