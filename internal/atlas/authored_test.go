package atlas

import (
	"strings"
	"testing"
)

func TestAuthoredFrontmatterRejectsDuplicateKeysOfEveryValueKind(t *testing.T) {
	const workspace = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const page = "projects/metadata.md"
	base := "---\nid: projects/metadata\ntitle: Metadata\nsummary: Reviewed metadata.\ntype: project-brief\nscope: workspace/" + workspace + "\nstatus: draft\nsensitivity: internal\nupdated: 2026-11-09\n---\n# Metadata\n"
	for _, tc := range []struct{ name, extra string }{
		{"required-list-first", "id: [ignored]\n"},
		{"required-object-first", "id: {ignored: value}\n"},
		{"required-scalar-first", "id: ignored\n"},
		{"optional-object-scalar", "context: {source: first}\ncontext: second\n"},
		{"optional-object-object", "context: {source: first}\ncontext: {source: second}\n"},
		{"optional-list-list", "tags: [first]\ntags: [second]\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := strings.Replace(base, "---\n", "---\n"+tc.extra, 1)
			if _, err := ValidateAuthoredPage(page, workspace, text); err == nil || err.Error() != "authored_frontmatter_duplicate" {
				t.Fatalf("ambiguous frontmatter was accepted or misclassified: %v", err)
			}
		})
	}
	legitimate := strings.Replace(base, "---\n", "---\ncontext: {source: reviewed}\ntags: [synthetic, reviewed]\n", 1)
	if _, err := ValidateAuthoredPage(page, workspace, legitimate); err != nil {
		t.Fatalf("single optional structured values must remain valid: %v", err)
	}
}
