package atlas

import (
	"encoding/json"
	"fmt"
	"gopkg.in/yaml.v3"
	"net/url"
	"path"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func navigationTestSource(page, body string) NavigationSource {
	text := fmt.Sprintf("---\nid: %s\ntitle: %s\nsummary: Curated authored source.\ntype: project-brief\nscope: workspace/test-workspace\nstatus: draft\nsensitivity: internal\nupdated: 2026-11-14\n---\n# %s\n\n%s", strconv.Quote(strings.TrimSuffix(page, ".md")), strconv.Quote(page), page, body)
	return NavigationSource{Page: page, Text: text, Revision: navigationDigest([]byte(text)), Pointer: "bcgos://atlas/workspace/test-workspace/" + page}
}
func TestCompileAuthoredNavigationPreservesExplicitCohort(t *testing.T) {
	a := navigationTestSource("projects/deep/a.md", "Curated café. [B](../../people/b.md) [Missing](missing.md) [Unvisited](unrelated.md)\n```md\n[Example](missing.md)\n```\n<!-- maestro:generated:start -->\nDerivedMarker [Feedback](../../people/b.md)\n<!-- maestro:generated:end -->\n")
	b := navigationTestSource("people/b.md", "A person.\n")
	missing := NavigationSource{Page: "projects/deep/missing.md", Missing: true}
	sources := []NavigationSource{a, b, missing}
	before, _ := json.Marshal(sources)
	compiled, err := CompileAuthoredNavigation("test-workspace", sources)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(sources)
	if string(before) != string(after) {
		t.Fatal("pure compilation changed canonical snapshots")
	}
	reordered, err := CompileAuthoredNavigation("test-workspace", []NavigationSource{missing, b, a})
	if err != nil || !reflect.DeepEqual(compiled, reordered) {
		t.Fatalf("input order affected generation: %v", err)
	}
	if len(compiled.Pages) != 2 || !NavigationMatches(compiled.Pages[1], "CAFE") || NavigationMatches(compiled.Pages[1], "DerivedMarker") {
		t.Fatal("Unicode search or curated-region separation failed", compiled.Pages)
	}
	if !reflect.DeepEqual(compiled.Pages[0].Backlinks, []string{a.Page}) {
		t.Fatal("backlinks not deterministic", compiled.Pages[0])
	}
	broken := 0
	for _, diagnostic := range compiled.Diagnostics {
		if diagnostic.Code == "broken_link" {
			broken++
			if diagnostic.Target != missing.Page {
				t.Fatal("unvisited target diagnosed", diagnostic)
			}
		}
	}
	if broken != 1 {
		t.Fatal("broken references not deduplicated", compiled.Diagnostics)
	}
	for reference, body := range compiled.Files {
		if !strings.HasSuffix(reference, ".md") {
			continue
		}
		parts := strings.SplitN(body, "---\n", 3)
		if len(parts) != 3 || parts[0] != "" {
			t.Fatal("generated concept has no OKF frontmatter", reference)
		}
		var metadata map[string]any
		if err := yaml.Unmarshal([]byte(parts[1]), &metadata); err != nil || metadata["type"] != "Authored Navigation" || metadata["x-maestro-profile-version"] != "1" || metadata["x-maestro-sensitivity"] != "internal" {
			t.Fatal("generated security profile invalid", reference, metadata, err)
		}
		if reference == "index.md" && metadata["okf_version"] != "0.2" {
			t.Fatal("root OKF version missing", metadata)
		}
		for _, match := range authoredLink.FindAllStringSubmatch(body, -1) {
			target := path.Clean(path.Join(path.Dir(reference), match[1]))
			if _, found := compiled.Files[target]; !found {
				t.Fatalf("generated hierarchical reference %s -> %s is unresolved", reference, target)
			}
		}
	}
	if strings.Contains(compiled.Files["log.md"], a.Page) || strings.Contains(compiled.Files["log.md"], a.Revision) {
		t.Fatal("log disclosed sensitive source identity")
	}
	if _, err = CompileAuthoredNavigation("test-workspace", append(sources, a)); err == nil {
		t.Fatal("duplicate source accepted")
	}
	for _, bad := range []NavigationSource{{Page: "../private.md", Missing: true}, {Page: "owner.md", Text: "stale", Revision: strings.Repeat("a", 64)}} {
		if _, err = CompileAuthoredNavigation("test-workspace", []NavigationSource{bad}); err == nil {
			t.Fatal("invalid snapshot accepted")
		}
	}
	reserved := []NavigationSource{}
	for _, name := range []string{"projects/hash#part.md", "projects/hash%23part.md", "projects/space name.md", "projects/round(name).md", "projects/label[one].md"} {
		reserved = append(reserved, navigationTestSource(name, "Reserved source."))
	}
	reservedView, err := CompileAuthoredNavigation("test-workspace", reserved)
	if err != nil || len(reservedView.Pages) != len(reserved) {
		t.Fatal("reserved-name source fixture invalid", err, reservedView.Diagnostics)
	}
	reached := map[string]bool{}
	for reference, body := range reservedView.Files {
		if !strings.HasSuffix(reference, ".md") {
			continue
		}
		for _, match := range authoredLink.FindAllStringSubmatch(body, -1) {
			base := &url.URL{Path: reference}
			link, err := url.Parse(match[1])
			if err != nil {
				t.Fatal("generated href invalid", match[1], err)
			}
			target := strings.TrimPrefix(path.Clean(base.ResolveReference(link).Path), "/")
			if _, found := reservedView.Files[target]; !found {
				t.Fatalf("reserved generated href %q from %q resolves %q instead of a virtual document", match[1], reference, target)
			}
			reached[target] = true
		}
	}
	for _, source := range reserved {
		if !reached["pages/"+source.Page] {
			t.Fatal("reserved-name document unreachable", source.Page)
		}
	}
	for _, source := range reserved {
		wire := NavigationReference("pages/" + source.Page)
		decoded, err := ResolveNavigationReference(wire)
		if err != nil || decoded != "pages/"+source.Page {
			t.Fatal("canonical virtual reference did not roundtrip", source.Page, wire, decoded, err)
		}
	}
	for _, bad := range []string{"pages/hash#part.md", "pages/hash%2fpart.md", "../pages/a.md", "/pages/a.md", "pages/a.md?x=1", "pages/%FF.md", "pages/%00.md", "pages/%5Ca.md"} {
		if _, err := ResolveNavigationReference(bad); err == nil {
			t.Fatal("noncanonical or unsafe virtual reference accepted", bad)
		}
	}
	if NavigationReference("pages/projects/hash#part.md") == NavigationReference("pages/projects/hash%23part.md") {
		t.Fatal("literal percent confused with fragment encoding")
	}
	if !strings.Contains(reservedView.Files["indexes/projects/index.md"], `label\[one\]`) {
		t.Fatal("Markdown label was not escaped")
	}
	tooLarge := navigationTestSource("projects/large.md", strings.Repeat("x", 128<<10))
	if _, err = CompileAuthoredNavigation("test-workspace", []NavigationSource{tooLarge}); err == nil {
		t.Fatal("collection byte bound was exceeded")
	}
	malformed := navigationTestSource("projects/legacy.md", "<!-- maestro:generated:start -->\nUnclosed\n")
	result, err := CompileAuthoredNavigation("test-workspace", []NavigationSource{malformed})
	if err != nil || len(result.Pages) != 0 || len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "generated_region_invalid" {
		t.Fatal("unclosed generated content claimed valid", result, err)
	}
}
