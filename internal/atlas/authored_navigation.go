package atlas

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// NavigationSource is an explicitly admitted snapshot, never a discovery root.
// A missing named source is retained so that absence and broken links are real
// observations of the cohort rather than guesses about an unvisited filesystem.
type NavigationSource struct {
	Page     string `json:"page"`
	Revision string `json:"revision,omitempty"`
	Pointer  string `json:"pointer,omitempty"`
	Text     string `json:"text,omitempty"`
	Missing  bool   `json:"missing,omitempty"`
}
type NavigationDiagnostic struct {
	Code   string `json:"code"`
	Page   string `json:"page"`
	Target string `json:"target,omitempty"`
}
type NavigationPage struct {
	Page        string   `json:"page"`
	Revision    string   `json:"revision"`
	Pointer     string   `json:"pointer"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Scope       string   `json:"scope"`
	Sensitivity string   `json:"sensitivity"`
	Body        string   `json:"body"`
	Links       []string `json:"links"`
	Backlinks   []string `json:"backlinks"`
}

// AuthoredNavigation is a complete immutable derived view. Canonical pages are
// never a destination of this compiler; even curated/generated markers remain
// byte-for-byte owned by their author.
type AuthoredNavigation struct {
	Schema      int                    `json:"schema"`
	WorkspaceID string                 `json:"workspace_id"`
	Generation  string                 `json:"generation"`
	Sources     []NavigationSource     `json:"sources"`
	Pages       []NavigationPage       `json:"pages"`
	Diagnostics []NavigationDiagnostic `json:"diagnostics"`
	Files       map[string]string      `json:"files"`
}

var authoredLink = regexp.MustCompile(`\]\(([^)\r\n]+)\)`)
var inlineAuthoredCode = regexp.MustCompile("`[^`]*`")

func navigationDigest(body []byte) string {
	value := sha256.Sum256(body)
	return hex.EncodeToString(value[:])
}
func navigationName(page string) bool {
	if !utf8.ValidString(page) || len(page) > 256 || path.Clean(page) != page || path.IsAbs(page) || !strings.HasSuffix(page, ".md") || strings.ContainsAny(page, "\\\x00\r\n") {
		return false
	}
	for _, part := range strings.Split(page, "/") {
		if part == "" || strings.HasPrefix(part, ".") {
			return false
		}
	}
	return true
}

// CompileAuthoredNavigation is pure and deterministic. Its byte and page bounds
// match the owning Atlas collector. Out-of-cohort references are not traversed,
// disclosed or mislabeled as broken.
func CompileAuthoredNavigation(workspaceID string, sources []NavigationSource) (AuthoredNavigation, error) {
	out := AuthoredNavigation{Schema: 1, WorkspaceID: workspaceID, Pages: []NavigationPage{}, Diagnostics: []NavigationDiagnostic{}, Files: map[string]string{}}
	if len(sources) == 0 || len(sources) > 24 || workspaceID == "" {
		return out, errors.New("authored_navigation_cohort_invalid")
	}
	out.Sources = append([]NavigationSource{}, sources...)
	sort.Slice(out.Sources, func(i, j int) bool { return out.Sources[i].Page < out.Sources[j].Page })
	named := map[string]NavigationSource{}
	bytes := 0
	for _, source := range out.Sources {
		if !navigationName(source.Page) {
			return out, errors.New("authored_navigation_page_invalid")
		}
		if _, found := named[source.Page]; found {
			return out, errors.New("authored_navigation_duplicate_page")
		}
		if source.Missing {
			if source.Text != "" || source.Revision != "" || source.Pointer != "" {
				return out, errors.New("authored_navigation_missing_invalid")
			}
		} else {
			bytes += len(source.Text)
			if bytes > 128<<10 || !utf8.ValidString(source.Text) || strings.ContainsRune(source.Text, 0) || source.Revision != navigationDigest([]byte(source.Text)) {
				return out, errors.New("authored_navigation_snapshot_invalid")
			}
		}
		named[source.Page] = source
	}
	fingerprint, _ := json.Marshal([]any{"authored-navigation-v1", workspaceID, out.Sources})
	out.Generation = navigationDigest(fingerprint)
	active := map[string]int{}
	for _, source := range out.Sources {
		if source.Missing {
			out.Diagnostics = append(out.Diagnostics, NavigationDiagnostic{Code: "missing_page", Page: source.Page})
			continue
		}
		metadata, err := ValidateAuthoredPage(source.Page, workspaceID, source.Text)
		if err != nil {
			out.Diagnostics = append(out.Diagnostics, NavigationDiagnostic{Code: "metadata_invalid", Page: source.Page})
			continue
		}
		if metadata["status"] == "superseded" {
			continue
		}
		body, valid := authoredCuratedBody(source.Text)
		if !valid {
			out.Diagnostics = append(out.Diagnostics, NavigationDiagnostic{Code: "generated_region_invalid", Page: source.Page})
			continue
		}
		page := NavigationPage{Page: source.Page, Revision: source.Revision, Pointer: source.Pointer, Title: metadata["title"], Summary: metadata["summary"], Scope: metadata["scope"], Sensitivity: metadata["sensitivity"], Body: body, Links: []string{}, Backlinks: []string{}}
		active[page.Page] = len(out.Pages)
		out.Pages = append(out.Pages, page)
	}
	for i := range out.Pages {
		page := &out.Pages[i]
		seen := map[string]bool{}
		for _, match := range authoredLink.FindAllStringSubmatch(authoredLinkText(page.Body), -1) {
			target := authoredLinkTarget(page.Page, match[1])
			if target == "" || seen[target] {
				continue
			}
			seen[target] = true
			source, admitted := named[target]
			if !admitted {
				continue
			}
			if source.Missing {
				out.Diagnostics = append(out.Diagnostics, NavigationDiagnostic{Code: "broken_link", Page: page.Page, Target: target})
				continue
			}
			if _, valid := active[target]; valid {
				page.Links = append(page.Links, target)
			}
		}
		sort.Strings(page.Links)
	}
	for _, page := range out.Pages {
		for _, target := range page.Links {
			j := active[target]
			out.Pages[j].Backlinks = append(out.Pages[j].Backlinks, page.Page)
		}
	}
	directories := map[string][]int{}
	for i := range out.Pages {
		page := &out.Pages[i]
		sort.Strings(page.Backlinks)
		if len(page.Backlinks) == 0 && path.Base(page.Page) != "index.md" {
			out.Diagnostics = append(out.Diagnostics, NavigationDiagnostic{Code: "orphan", Page: page.Page})
		}
		for dir := path.Dir(page.Page); dir != "."; dir = path.Dir(dir) {
			directories[dir] = append(directories[dir], i)
		}
		var body strings.Builder
		fmt.Fprintf(&body, "# %s\n\nSource: `%s`\nRevision: `%s`\nScope: `%s`\n\n%s\n\n## Backlinks\n", page.Title, page.Page, page.Revision, page.Scope, page.Summary)
		for _, backlink := range page.Backlinks {
			fmt.Fprintf(&body, "- %s\n", navigationMarkdownLink(backlink, navigationRelative("pages/"+page.Page, "pages/"+backlink)))
		}
		body.WriteString("\n## Related\n")
		for _, target := range page.Links {
			fmt.Fprintf(&body, "- %s\n", navigationMarkdownLink(target, navigationRelative("pages/"+page.Page, "pages/"+target)))
		}
		out.Files["pages/"+page.Page] = body.String()
	}
	var root strings.Builder
	root.WriteString("# Authored navigation\n\nExplicit admitted cohort; source Markdown remains authoritative.\n\n")
	dirs := make([]string, 0, len(directories))
	for dir := range directories {
		dirs = append(dirs, dir)
	}
	sort.Strings(dirs)
	for _, dir := range dirs {
		ref := "indexes/" + dir + "/index.md"
		fmt.Fprintf(&root, "- %s\n", navigationMarkdownLink(dir, ref))
		var index strings.Builder
		fmt.Fprintf(&index, "# %s\n\n", dir)
		for _, i := range directories[dir] {
			page := out.Pages[i]
			fmt.Fprintf(&index, "- %s — %s\n", navigationMarkdownLink(page.Title, navigationRelative(ref, "pages/"+page.Page)), page.Summary)
		}
		out.Files[ref] = index.String()
	}
	for _, page := range out.Pages {
		if path.Dir(page.Page) == "." {
			fmt.Fprintf(&root, "- %s\n", navigationMarkdownLink(page.Title, "pages/"+page.Page))
		}
	}
	out.Files["index.md"] = root.String()
	sort.Slice(out.Diagnostics, func(i, j int) bool {
		a, b := out.Diagnostics[i], out.Diagnostics[j]
		return a.Page+"\x00"+a.Code+"\x00"+a.Target < b.Page+"\x00"+b.Code+"\x00"+b.Target
	})
	diagnostics, _ := json.MarshalIndent(out.Diagnostics, "", "  ")
	out.Files["diagnostics.json"] = string(diagnostics) + "\n"
	backlinks := map[string][]string{}
	for _, page := range out.Pages {
		backlinks[page.Page] = page.Backlinks
	}
	encoded, _ := json.MarshalIndent(backlinks, "", "  ")
	out.Files["backlinks.json"] = string(encoded) + "\n"
	out.Files["log.md"] = fmt.Sprintf("# Authored navigation publication\n\nForeground explicit-cohort compilation: %d named sources, %d navigable pages. Source names, bodies and source digests are omitted from this free-text log. The owning private generation record retains its exact cohort, fingerprint, attended provenance and publication timestamp.\n", len(out.Sources), len(out.Pages))
	aggregateSensitivity := "internal"
	for _, page := range out.Pages {
		if page.Sensitivity == "client-confidential" {
			aggregateSensitivity = "client-confidential"
			break
		}
		if page.Sensitivity == "owner-private" {
			aggregateSensitivity = "owner-private"
		}
	}
	for reference, body := range out.Files {
		if !strings.HasSuffix(reference, ".md") {
			continue
		}
		title, scope, sensitivity, pointer, revision := "Authored navigation", "workspace/"+workspaceID, aggregateSensitivity, "", out.Generation
		for _, page := range out.Pages {
			if reference == "pages/"+page.Page {
				title, scope, sensitivity, pointer, revision = page.Title, page.Scope, page.Sensitivity, page.Pointer, page.Revision
				break
			}
		}
		var header strings.Builder
		header.WriteString("---\n")
		if reference == "index.md" {
			header.WriteString("okf_version: \"0.2\"\n")
		}
		fmt.Fprintf(&header, "type: Authored Navigation\ntitle: %s\nx-maestro-profile-version: \"1\"\nx-maestro-stable-id: %s\nx-maestro-scope: %s\nx-maestro-workspace-ref: %s\nx-maestro-sensitivity: %s\nx-maestro-freshness: fresh\nx-maestro-status: active\nx-maestro-generator-version: authored-navigation-v1\nx-maestro-policy-version: \"1\"\n", strconv.Quote(title), navigationDigest([]byte(workspaceID+"\x00"+reference)), strconv.Quote(scope), strconv.Quote(workspaceID), strconv.Quote(sensitivity))
		// Metadata-safe update history omits sensitive paths and source fingerprints;
		// the immutable owning envelope remains the audit/provenance authority.
		if reference != "log.md" {
			fmt.Fprintf(&header, "x-maestro-source-fingerprint: %s\n", revision)
		}
		if pointer != "" {
			fmt.Fprintf(&header, "resource: %s\n", strconv.Quote(pointer))
		}
		header.WriteString("---\n")
		out.Files[reference] = header.String() + body
	}
	return out, nil
}
func authoredCuratedBody(text string) (string, bool) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if strings.HasPrefix(text, "---\n") {
		if end := strings.Index(text[4:], "\n---\n"); end >= 0 {
			text = text[4+end+5:]
		}
	}
	const start = "<!-- maestro:generated:start -->"
	const end = "<!-- maestro:generated:end -->"
	for strings.Contains(text, start) || strings.Contains(text, end) {
		a, b := strings.Index(text, start), strings.Index(text, end)
		if a < 0 || b < a || strings.Contains(text[a+len(start):b], start) {
			return "", false
		}
		text = text[:a] + text[b+len(end):]
	}
	return text, true
}
func authoredLinkText(text string) string {
	lines := strings.Split(text, "\n")
	fence := ""
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			mark := trimmed[:3]
			if fence == "" {
				fence = mark
			} else if fence == mark {
				fence = ""
			}
			lines[i] = ""
			continue
		}
		if fence != "" {
			lines[i] = ""
		} else {
			lines[i] = inlineAuthoredCode.ReplaceAllString(line, "")
		}
	}
	return strings.Join(lines, "\n")
}
func authoredLinkTarget(source, raw string) string {
	raw = strings.TrimSpace(raw)
	if strings.HasPrefix(raw, "<") {
		if end := strings.Index(raw, ">"); end >= 0 {
			raw = raw[1:end]
		}
	} else {
		fields := strings.Fields(raw)
		if len(fields) == 0 {
			return ""
		}
		raw = fields[0]
	}
	u, err := url.Parse(raw)
	if err != nil || u.IsAbs() || u.Host != "" || u.Path == "" || strings.HasPrefix(u.Path, "/") {
		return ""
	}
	target := path.Clean(path.Join(path.Dir(source), u.Path))
	if !navigationName(target) {
		return ""
	}
	return target
}

// NavigationMatches performs Unicode case/accent folding without a separate
// search index or nondeterministic ranking. The caller bounds serialized hits.
func NavigationMatches(page NavigationPage, query string) bool {
	fold := func(text string) string {
		var b strings.Builder
		for _, r := range norm.NFD.String(cases.Fold().String(text)) {
			if !unicode.Is(unicode.Mn, r) {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	return strings.Contains(fold(page.Title+"\n"+page.Summary+"\n"+page.Body), fold(query))
}

func navigationRelative(from, to string) string {
	left := strings.Split(path.Dir(from), "/")
	if len(left) == 1 && left[0] == "." {
		left = nil
	}
	right := strings.Split(to, "/")
	common := 0
	for common < len(left) && common < len(right) && left[common] == right[common] {
		common++
	}
	return strings.Repeat("../", len(left)-common) + strings.Join(right[common:], "/")
}

// NavigationReference is the single wire/href representation of a virtual
// document. Escaping each segment keeps slashes structural and makes literal
// percent names distinct from already escaped fragment characters.
func NavigationReference(reference string) string {
	parts := strings.Split(reference, "/")
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return strings.Join(parts, "/")
}

// ResolveNavigationReference decodes exactly once and accepts only canonical
// bundle-relative references. A fragment, query, encoded slash, traversal or
// second spelling never becomes an alias of a document selected by its owner.
func ResolveNavigationReference(reference string) (string, error) {
	parsed, err := url.Parse(reference)
	if err != nil || len(reference) > 1024 || strings.ContainsAny(parsed.Path, "\\\x00") || parsed.IsAbs() || parsed.Host != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.ContainsAny(reference, "?#") || !utf8.ValidString(parsed.Path) || !fs.ValidPath(parsed.Path) || NavigationReference(parsed.Path) != reference {
		return "", errors.New("authored_navigation_reference_invalid")
	}
	return parsed.Path, nil
}
func navigationMarkdownLink(label, reference string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "[", "\\[", "]", "\\]", "*", "\\*", "_", "\\_", "`", "\\`", "<", "\\<", ">", "\\>")
	return "[" + replacer.Replace(label) + "](" + NavigationReference(reference) + ")"
}
