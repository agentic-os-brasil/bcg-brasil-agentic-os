package priorwork

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const compilerVersion = "prior-work-okf-v1"

type facetEntry struct {
	Kind  string
	Value string
	Items []Item
	Path  string
}

func Compile(destination string, catalog Catalog) error {
	if strings.TrimSpace(destination) == "" {
		return errors.New("prior-work bundle destination is required")
	}
	if err := ensurePrivateDirectory(destination); err != nil {
		return err
	}
	root, err := openAnchoredRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	return compileAt(root, catalog)
}

func compileAt(root *os.Root, catalog Catalog) error {
	if catalog.SchemaVersion != 1 || catalog.TenantRef == "" || catalog.PolicyVersion == "" ||
		catalog.CollectionSequence == 0 || catalog.Watermark == "" || catalog.GeneratedAt.IsZero() ||
		!digestPattern.MatchString(catalog.SnapshotDigest) || catalog.Roots == nil || catalog.Items == nil {
		return errors.New("invalid prior-work catalog")
	}
	var err error
	catalog, err = canonicalCatalog(catalog)
	if err != nil {
		return err
	}
	fingerprint, err := catalogFingerprint(catalog)
	if err != nil {
		return err
	}
	for _, path := range []string{"items", "facets/clients", "facets/projects", "facets/themes", "facets/years", "facets/audiences"} {
		if err := root.MkdirAll(filepath.FromSlash(path), 0o700); err != nil {
			return err
		}
	}
	if err := writePrivateFileAt(root, "items.json", catalog); err != nil {
		return err
	}

	facets := collectFacets(catalog.Items)
	backlinks := map[string][]string{}
	for _, facet := range facets {
		if err := writeMarkdownAt(root, filepath.FromSlash(facet.Path), renderFacetPage(fingerprint, facet)); err != nil {
			return err
		}
		for _, item := range facet.Items {
			itemPath := "items/" + opaqueFilename(item.key()) + ".md"
			backlinks[itemPath] = append(backlinks[itemPath], facet.Path)
		}
	}
	for _, item := range catalog.Items {
		path := "items/" + opaqueFilename(item.key()) + ".md"
		if err := writeMarkdownAt(root, filepath.FromSlash(path), renderItemPage(fingerprint, item, backlinks[path])); err != nil {
			return err
		}
	}
	for key := range backlinks {
		sort.Strings(backlinks[key])
	}
	if err := writePrivateFileAt(root, "backlinks.json", backlinks); err != nil {
		return err
	}
	diagnostics := struct {
		SchemaVersion int    `json:"schema_version"`
		Compiler      string `json:"compiler"`
		Fingerprint   string `json:"fingerprint"`
		Items         int    `json:"items"`
		Facets        int    `json:"facets"`
		Truncated     bool   `json:"truncated"`
	}{
		SchemaVersion: 1, Compiler: compilerVersion, Fingerprint: fingerprint,
		Items: len(catalog.Items), Facets: len(facets), Truncated: false,
	}
	if err := writePrivateFileAt(root, "diagnostics.json", diagnostics); err != nil {
		return err
	}
	if err := writeMarkdownAt(root, "index.md", renderIndex(fingerprint, catalog, facets)); err != nil {
		return err
	}
	return writeMarkdownAt(root, "log.md", renderLog(fingerprint, catalog))
}

func ValidateBundle(root string, expected Catalog) error {
	anchored, err := openAnchoredRoot(root)
	if err != nil {
		return err
	}
	defer anchored.Close()
	return validateBundleAt(anchored, expected)
}

func validateBundleAt(root *os.Root, expected Catalog) error {
	for _, required := range []string{"index.md", "log.md", "items.json", "backlinks.json", "diagnostics.json"} {
		info, err := root.Lstat(required)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return fmt.Errorf("invalid prior-work bundle file %s", required)
		}
		file, err := root.Open(required)
		if err != nil {
			return err
		}
		openedInfo, statErr := file.Stat()
		closeErr := file.Close()
		if statErr != nil {
			return statErr
		}
		if !os.SameFile(info, openedInfo) {
			return fmt.Errorf("prior-work bundle file %s changed during secure open", required)
		}
		if closeErr != nil {
			return closeErr
		}
	}
	actual, err := loadJSONRoot[Catalog](root, "items.json")
	if err != nil {
		return err
	}
	expectedFingerprint, err := catalogFingerprint(expected)
	if err != nil {
		return err
	}
	actualFingerprint, err := catalogFingerprint(actual)
	if err != nil {
		return err
	}
	if expectedFingerprint != actualFingerprint {
		return errors.New("compiled prior-work catalog fingerprint mismatch")
	}
	diagnostics, err := loadJSONRoot[struct {
		SchemaVersion int    `json:"schema_version"`
		Compiler      string `json:"compiler"`
		Fingerprint   string `json:"fingerprint"`
		Items         int    `json:"items"`
		Facets        int    `json:"facets"`
		Truncated     bool   `json:"truncated"`
	}](root, "diagnostics.json")
	if err != nil {
		return err
	}
	if diagnostics.SchemaVersion != 1 || diagnostics.Compiler != compilerVersion ||
		diagnostics.Fingerprint != expectedFingerprint || diagnostics.Items != len(expected.Items) {
		return errors.New("invalid prior-work bundle diagnostics")
	}
	return nil
}

func collectFacets(items []Item) []facetEntry {
	byKey := map[string]*facetEntry{}
	add := func(kind, value string, item Item) {
		key := kind + "\x00" + normalize(value)
		entry := byKey[key]
		if entry == nil {
			entry = &facetEntry{Kind: kind, Value: value, Path: facetPath(kind, value)}
			byKey[key] = entry
		}
		entry.Items = append(entry.Items, item)
	}
	for _, item := range items {
		for _, value := range item.Facets.Clients {
			add("clients", value, item)
		}
		for _, value := range item.Facets.Projects {
			add("projects", value, item)
		}
		for _, value := range item.Facets.Themes {
			add("themes", value, item)
		}
		for _, value := range item.Facets.Years {
			add("years", strconv.Itoa(value), item)
		}
		for _, value := range item.Facets.Audiences {
			add("audiences", value, item)
		}
	}
	facets := make([]facetEntry, 0, len(byKey))
	for _, entry := range byKey {
		sort.Slice(entry.Items, func(i, j int) bool { return entry.Items[i].key() < entry.Items[j].key() })
		facets = append(facets, *entry)
	}
	sort.Slice(facets, func(i, j int) bool {
		if facets[i].Kind != facets[j].Kind {
			return facets[i].Kind < facets[j].Kind
		}
		return normalize(facets[i].Value) < normalize(facets[j].Value)
	})
	return facets
}

func facetPath(kind, value string) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + normalize(value)))
	return "facets/" + kind + "/" + slug(value) + "-" + hex.EncodeToString(sum[:4]) + ".md"
}

func slug(value string) string {
	var output strings.Builder
	hyphen := false
	for _, character := range normalize(value) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			output.WriteRune(character)
			hyphen = false
		} else if !hyphen && output.Len() > 0 {
			output.WriteByte('-')
			hyphen = true
		}
	}
	result := strings.Trim(output.String(), "-")
	if result == "" {
		return "facet"
	}
	if len(result) > 48 {
		result = strings.Trim(result[:48], "-")
	}
	return result
}

func frontmatter(fingerprint string) string {
	return "---\n" +
		"okf_version: \"0.2\"\n" +
		"profile: \"bcgos-atlas/v1\"\n" +
		"scope: \"organization\"\n" +
		"sensitivity: \"client_restricted\"\n" +
		"authority: \"derived_sharepoint_metadata\"\n" +
		"source_fingerprint: \"" + fingerprint + "\"\n" +
		"generator: \"" + compilerVersion + "\"\n" +
		"---\n\n"
}

func renderIndex(fingerprint string, catalog Catalog, facets []facetEntry) string {
	var body strings.Builder
	body.WriteString(frontmatter(fingerprint))
	body.WriteString("# SharePoint prior-work index\n\n")
	body.WriteString("Dedicated retrieval metadata for explicitly requested prior professional work. It is not part of the general Maestro wiki.\n\n")
	body.WriteString("- Catalog sequence: " + strconv.FormatUint(catalog.CollectionSequence, 10) + "\n")
	body.WriteString("- Items: " + strconv.Itoa(len(catalog.Items)) + "\n")
	body.WriteString("- Facets: " + strconv.Itoa(len(facets)) + "\n\n")
	body.WriteString("## Facets\n\n")
	for _, facet := range facets {
		body.WriteString("- [" + markdownText(facet.Value) + "](" + facet.Path + ") — " + strconv.Itoa(len(facet.Items)) + "\n")
	}
	return body.String()
}

func renderFacetPage(fingerprint string, facet facetEntry) string {
	var body strings.Builder
	body.WriteString(frontmatter(fingerprint))
	body.WriteString("# " + markdownText(facet.Value) + "\n\n")
	body.WriteString("Facet: `" + facet.Kind + "` · " + strconv.Itoa(len(facet.Items)) + " item(s)\n\n")
	limit := len(facet.Items)
	if limit > 500 {
		limit = 500
	}
	for _, item := range facet.Items[:limit] {
		itemPath := "../../items/" + opaqueFilename(item.key()) + ".md"
		body.WriteString("- [" + markdownText(item.Name) + "](" + itemPath + ")\n")
	}
	if len(facet.Items) > limit {
		body.WriteString("\nResult list truncated; use the explicit query surface for the full catalog.\n")
	}
	return body.String()
}

func renderItemPage(fingerprint string, item Item, backlinks []string) string {
	var body strings.Builder
	body.WriteString(frontmatter(fingerprint))
	body.WriteString("# " + markdownText(item.Name) + "\n\n")
	body.WriteString("- Kind: `" + item.Kind + "`\n")
	body.WriteString("- Modified: `" + item.ModifiedAt.UTC().Format("2006-01-02T15:04:05Z") + "`\n")
	body.WriteString("- Sensitivity: `" + item.Sensitivity + "`\n")
	body.WriteString("- Source: [Open in SharePoint](<" + markdownURL(item.SourceURL) + ">)\n")
	body.WriteString("- Authorization: opening the source rechecks current SharePoint access.\n")
	if len(backlinks) > 0 {
		body.WriteString("\n## Facets\n\n")
		for _, link := range backlinks {
			body.WriteString("- [" + markdownText(filepath.Base(link)) + "](../" + link + ")\n")
		}
	}
	return body.String()
}

func renderLog(fingerprint string, catalog Catalog) string {
	return frontmatter(fingerprint) +
		"# Catalog log\n\n" +
		"- Collection sequence: " + strconv.FormatUint(catalog.CollectionSequence, 10) + "\n" +
		"- Generated at: `" + catalog.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z") + "`\n" +
		"- Item count: " + strconv.Itoa(len(catalog.Items)) + "\n" +
		"- Snapshot content and credentials are not copied into this bundle.\n"
}

func markdownText(value string) string {
	replacer := strings.NewReplacer("\\", "\\\\", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "`", "\\`")
	return replacer.Replace(value)
}

func markdownURL(value string) string {
	return strings.NewReplacer("<", "%3C", ">", "%3E", " ", "%20").Replace(value)
}

func writeMarkdownAt(root *os.Root, path, body string) error {
	if strings.ContainsRune(body, '\x00') {
		return errors.New("generated Markdown contains a NUL byte")
	}
	file, err := root.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.WriteString(body); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}
