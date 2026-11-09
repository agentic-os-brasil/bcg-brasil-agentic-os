package atlas

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// AuthoredPage is a fixed preparation input, never a model-selected root.
type AuthoredPage struct {
	Page string
	Text string
}

// WorkspacePages supplies the existing canonical workspace templates with the
// authored contract. Existing files remain owned by the person who edits them.
func WorkspacePages(workspaceID string, at time.Time) []AuthoredPage {
	bodies := make(map[string]string, len(workspaceFiles)+1)
	for page, body := range workspaceFiles {
		bodies[page] = body
	}
	day := at.Format("2006-01-02")
	bodies["daily/"+day+".md"] = strings.Replace(dailyTemplate, "YYYY-MM-DD", day, 1)
	pages := make([]AuthoredPage, 0, len(bodies))
	for page, body := range bodies {
		title := strings.TrimPrefix(strings.SplitN(body, "\n", 2)[0], "# ")
		kind := "index"
		switch {
		case strings.Contains(page, "template-client"):
			kind = "account"
		case strings.Contains(page, "template-project"):
			kind = "project-brief"
		case strings.Contains(page, "template-person"):
			kind = "person"
		case strings.HasPrefix(page, "daily/") && !strings.HasSuffix(page, "index.md"):
			kind = "daily"
		}
		header := fmt.Sprintf("---\nid: %s\ntitle: %s\nsummary: %s\ntype: %s\nscope: workspace/%s\nstatus: draft\nsensitivity: internal\nupdated: %s\n---\n",
			strconv.Quote(strings.TrimSuffix(page, ".md")), strconv.Quote(title), strconv.Quote("Professional workspace "+kind+" page."), kind, workspaceID, day)
		pages = append(pages, AuthoredPage{Page: page, Text: header + body})
	}
	sort.Slice(pages, func(i, j int) bool { return pages[i].Page < pages[j].Page })
	return pages
}

func AuthoredScope(page, workspaceID string) string {
	parts := strings.Split(page, "/")
	if len(parts) >= 3 && parts[0] == "accounts" {
		if len(parts) >= 5 && parts[2] == "cases" {
			return "account/" + parts[1] + "/case/" + parts[3]
		}
		return "account/" + parts[1]
	}
	return "workspace/" + workspaceID
}

// AuthoredDiagnostics reads metadata without modifying legacy pages.
func AuthoredDiagnostics(page, workspaceID, text string) []string {
	_, err := ValidateAuthoredPage(page, workspaceID, text)
	if err == nil {
		return []string{}
	}
	return []string{err.Error()}
}

// ValidateAuthoredPage validates only the declared source profile. Optional
// fields remain in the original Markdown; this reader does not rewrite them.
func ValidateAuthoredPage(page, workspaceID, text string) (map[string]string, error) {
	fail := func(code string) (map[string]string, error) { return nil, errors.New(code) }
	if len(text) == 0 || len(text) > 256<<10 || !utf8.ValidString(text) || strings.ContainsRune(text, 0) {
		return fail("authored_body_invalid")
	}
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return fail("authored_frontmatter_missing")
	}
	end := strings.Index(normalized[4:], "\n---\n")
	if end < 0 || end > 16<<10 {
		return fail("authored_frontmatter_invalid")
	}
	var document yaml.Node
	if yaml.Unmarshal([]byte(normalized[4:4+end]), &document) != nil || len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return fail("authored_frontmatter_invalid")
	}
	fields := map[string]string{}
	nodes := document.Content[0].Content
	for i := 0; i+1 < len(nodes); i += 2 {
		key, value := nodes[i], nodes[i+1]
		if _, exists := fields[key.Value]; exists {
			return fail("authored_frontmatter_duplicate")
		}
		if value.Kind == yaml.ScalarNode {
			fields[key.Value] = value.Value
		}
	}
	for _, name := range []string{"id", "title", "summary", "type", "scope", "status", "sensitivity", "updated"} {
		if strings.TrimSpace(fields[name]) == "" || strings.ContainsAny(fields[name], "\r\n") {
			return fail("authored_frontmatter_required")
		}
	}
	if fields["id"] != strings.TrimSuffix(page, ".md") || fields["scope"] != AuthoredScope(page, workspaceID) {
		return fail("authored_scope_invalid")
	}
	if utf8.RuneCountInString(fields["summary"]) > 150 || len(fields["title"]) > 512 {
		return fail("authored_metadata_size")
	}
	types := map[string]bool{"daily": true, "learning": true, "craft-method": true, "craft-style": true, "person": true, "objectives": true, "cdc": true, "retro": true, "project-feedback": true, "upward-feedback": true, "operating": true, "index": true, "account": true, "project-brief": true, "decision-log": true, "task-list": true, "deliverable": true, "source": true, "canon": true, "data": true, "framework": true, "hypothesis": true, "benchmark": true, "interview": true}
	if !types[fields["type"]] {
		return fail("authored_type_invalid")
	}
	if !map[string]bool{"active": true, "closed": true, "draft": true, "superseded": true}[fields["status"]] {
		return fail("authored_status_invalid")
	}
	if !map[string]bool{"owner-private": true, "client-confidential": true, "internal": true}[fields["sensitivity"]] || strings.HasPrefix(page, "accounts/") && fields["sensitivity"] != "client-confidential" {
		return fail("authored_sensitivity_invalid")
	}
	if _, err := time.Parse("2006-01-02", fields["updated"]); err != nil {
		return fail("authored_updated_invalid")
	}
	body := normalized[4+end+5:]
	if !strings.Contains("\n"+body, "\n# "+fields["title"]+"\n") {
		return fail("authored_title_mismatch")
	}
	return fields, nil
}
