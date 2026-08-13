package atlas

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	managedOKFVersion            = "0.2"
	managedScope                 = "managed"
	maximumManagedAllowlistBytes = 128 << 10
)

// ManagedAllowlist is the reviewed source contract for the product atlas.
// Source paths are intentionally limited to product-facing documentation.
type ManagedAllowlist struct {
	SchemaVersion    int             `json:"schema_version"`
	OKFVersion       string          `json:"okf_version"`
	GeneratorVersion string          `json:"generator_version"`
	PolicyVersion    string          `json:"policy_version"`
	LogDate          string          `json:"log_date"`
	Sources          []ManagedSource `json:"sources"`
}

type ManagedSource struct {
	ID          string   `json:"id"`
	Path        string   `json:"path"`
	Type        string   `json:"type"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Related     []string `json:"related"`
}

// ManagedReport is the deterministic receipt for one reconciliation.
type ManagedReport struct {
	Fingerprint string `json:"fingerprint"`
	Sources     int    `json:"sources"`
	Concepts    int    `json:"concepts"`
}

type conceptHeader struct {
	Type              string      `yaml:"type"`
	Title             string      `yaml:"title,omitempty"`
	Description       string      `yaml:"description,omitempty"`
	Resource          string      `yaml:"resource,omitempty"`
	Tags              []string    `yaml:"tags,omitempty"`
	Sources           []sourceRef `yaml:"sources,omitempty"`
	Status            string      `yaml:"status,omitempty"`
	ProfileVersion    string      `yaml:"x-maestro-profile-version"`
	StableID          string      `yaml:"x-maestro-stable-id"`
	Scope             string      `yaml:"x-maestro-scope"`
	SourceFingerprint string      `yaml:"x-maestro-source-fingerprint"`
	Freshness         string      `yaml:"x-maestro-freshness"`
	BCGOSStatus       string      `yaml:"x-maestro-status"`
	GeneratorVersion  string      `yaml:"x-maestro-generator-version"`
	PolicyVersion     string      `yaml:"x-maestro-policy-version"`
}

type sourceRef struct {
	ID       string `yaml:"id"`
	Resource string `yaml:"resource"`
	Title    string `yaml:"title,omitempty"`
}

var managedMarkdownLinkPattern = regexp.MustCompile(`\]\(([^)\r\n]*)\)`)

type managedLinkDiagnostics struct {
	BrokenLinks    []string `json:"broken_links"`
	OpaqueLinks    []string `json:"opaque_links"`
	RewrittenLinks []string `json:"rewritten_links"`
}

func (diagnostics *managedLinkDiagnostics) addOpaque(source, target string) {
	diagnostics.OpaqueLinks = append(diagnostics.OpaqueLinks, source+":"+target)
}

func (diagnostics *managedLinkDiagnostics) addRewritten(source, from, to string) {
	diagnostics.RewrittenLinks = append(diagnostics.RewrittenLinks, source+":"+from+" -> "+to)
}

func (diagnostics *managedLinkDiagnostics) sort() {
	sort.Strings(diagnostics.BrokenLinks)
	sort.Strings(diagnostics.OpaqueLinks)
	sort.Strings(diagnostics.RewrittenLinks)
}

// ReconcileManaged compiles the reviewed managed source allowlist into a
// deterministic OKF bundle and publishes it with a best-effort atomic
// directory swap. Durable versioned manifests and last-known-good pointer
// recovery remain outside this development-only compiler.
func ReconcileManaged(root, allowlistPath, outputPath string) (ManagedReport, error) {
	allowlist, err := loadManagedAllowlist(allowlistPath)
	if err != nil {
		return ManagedReport{}, err
	}
	if err := validateManagedAllowlist(root, allowlist); err != nil {
		return ManagedReport{}, err
	}

	sources := append([]ManagedSource(nil), allowlist.Sources...)
	sort.Slice(sources, func(left, right int) bool { return sources[left].ID < sources[right].ID })
	fingerprint, err := managedFingerprint(root, sources)
	if err != nil {
		return ManagedReport{}, err
	}

	parent := filepath.Dir(outputPath)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return ManagedReport{}, err
	}
	stage, err := os.MkdirTemp(parent, ".managed-atlas-stage-")
	if err != nil {
		return ManagedReport{}, err
	}
	defer os.RemoveAll(stage)

	byID := make(map[string]ManagedSource, len(sources))
	linkDiagnostics := managedLinkDiagnostics{
		BrokenLinks:    []string{},
		OpaqueLinks:    []string{},
		RewrittenLinks: []string{},
	}
	for _, source := range sources {
		byID[source.ID] = source
	}
	for _, source := range sources {
		if err := writeManagedConcept(stage, root, source, byID, fingerprint, allowlist, &linkDiagnostics); err != nil {
			return ManagedReport{}, err
		}
	}
	if err := writeManagedIndex(stage, sources, fingerprint, allowlist); err != nil {
		return ManagedReport{}, err
	}
	if err := writeManagedLog(stage, sources, fingerprint, allowlist); err != nil {
		return ManagedReport{}, err
	}
	if err := writeManagedReports(stage, sources, byID, linkDiagnostics); err != nil {
		return ManagedReport{}, err
	}
	if err := ValidateManagedBundle(stage); err != nil {
		return ManagedReport{}, fmt.Errorf("validate staged managed bundle: %w", err)
	}
	if err := atomicPublishDirectory(stage, outputPath); err != nil {
		return ManagedReport{}, err
	}
	return ManagedReport{Fingerprint: fingerprint, Sources: len(sources), Concepts: len(sources)}, nil
}

// ValidateManagedBundle checks the OKF core and the BCGOS managed profile.
func ValidateManagedBundle(root string) error {
	index, err := os.ReadFile(filepath.Join(root, "index.md"))
	if err != nil {
		return fmt.Errorf("read root index: %w", err)
	}
	if !strings.Contains(string(index), `okf_version: "`+managedOKFVersion+`"`) {
		return fmt.Errorf("root index must declare okf_version %q", managedOKFVersion)
	}
	if _, err := os.Stat(filepath.Join(root, "log.md")); err != nil {
		return fmt.Errorf("read root log: %w", err)
	}
	if err := validateManagedMarkdownLinks(root); err != nil {
		return err
	}
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed bundle symlink is forbidden: %s", path)
		}
		if filepath.Ext(entry.Name()) != ".md" || entry.Name() == "index.md" || entry.Name() == "log.md" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		header, err := parseConceptHeader(body)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if strings.TrimSpace(header.Type) == "" {
			return fmt.Errorf("%s: type is required", path)
		}
		if header.Scope != managedScope {
			return fmt.Errorf("%s: x-maestro-scope must be %q", path, managedScope)
		}
		if header.ProfileVersion != "1" || header.PolicyVersion == "" || header.GeneratorVersion == "" {
			return fmt.Errorf("%s: incomplete Maestro profile", path)
		}
		return nil
	})
}

// VerifyManagedUpToDate recompiles into an isolated temporary directory and
// compares every generated file without mutating the checked-in bundle.
func VerifyManagedUpToDate(root, allowlistPath, outputPath string) error {
	if err := ValidateManagedBundle(outputPath); err != nil {
		return fmt.Errorf("validate checked-in managed bundle: %w", err)
	}
	temporary, err := os.MkdirTemp("", "bcgos-managed-wiki-verify-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temporary)
	expectedPath := filepath.Join(temporary, "managed")
	if _, err := ReconcileManaged(root, allowlistPath, expectedPath); err != nil {
		return err
	}
	expected, err := directorySnapshot(expectedPath)
	if err != nil {
		return err
	}
	actual, err := directorySnapshot(outputPath)
	if err != nil {
		return err
	}
	if expected != actual {
		return errors.New("managed OKF bundle is stale; run wiki reconcile")
	}
	return nil
}

func loadManagedAllowlist(path string) (ManagedAllowlist, error) {
	file, err := os.Open(path)
	if err != nil {
		return ManagedAllowlist{}, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, maximumManagedAllowlistBytes+1))
	if err != nil {
		return ManagedAllowlist{}, err
	}
	if len(body) > maximumManagedAllowlistBytes {
		return ManagedAllowlist{}, fmt.Errorf("managed allowlist exceeds %d bytes", maximumManagedAllowlistBytes)
	}
	if err := rejectDuplicateManagedJSONKeys(body); err != nil {
		return ManagedAllowlist{}, fmt.Errorf("decode managed allowlist: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var allowlist ManagedAllowlist
	if err := decoder.Decode(&allowlist); err != nil {
		return ManagedAllowlist{}, err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return ManagedAllowlist{}, errors.New("managed allowlist contains multiple JSON values")
		}
		return ManagedAllowlist{}, err
	}
	return allowlist, nil
}

func rejectDuplicateManagedJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := walkManagedJSONValue(decoder, token); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func walkManagedJSONValue(decoder *json.Decoder, token json.Token) error {
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("managed allowlist object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = true
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := walkManagedJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return errors.New("unterminated managed allowlist object")
		}
	case '[':
		for decoder.More() {
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := walkManagedJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("unterminated managed allowlist array")
		}
	default:
		return fmt.Errorf("unexpected managed allowlist delimiter %q", delimiter)
	}
	return nil
}

func validateManagedAllowlist(root string, allowlist ManagedAllowlist) error {
	if allowlist.SchemaVersion != 1 || allowlist.OKFVersion != managedOKFVersion ||
		strings.TrimSpace(allowlist.GeneratorVersion) == "" || strings.TrimSpace(allowlist.PolicyVersion) == "" ||
		!validISODate(allowlist.LogDate) || len(allowlist.Sources) == 0 {
		return errors.New("managed allowlist requires schema 1, OKF 0.2, versions, ISO log_date and sources")
	}
	seen := make(map[string]bool, len(allowlist.Sources))
	for _, source := range allowlist.Sources {
		if !safeManagedID(source.ID) {
			return fmt.Errorf("managed allowlist has unsafe source id: %q", source.ID)
		}
		if seen[source.ID] {
			return fmt.Errorf("managed allowlist has duplicate source id: %q", source.ID)
		}
		seen[source.ID] = true
		if !safeManagedRelative(source.Path) {
			return fmt.Errorf("unsafe managed source path: %s", source.Path)
		}
		if !approvedManagedSource(source.Path) {
			return fmt.Errorf("managed source is outside approved product roots: %s", source.Path)
		}
		symlinked, err := hasSymlinkComponent(root, source.Path)
		if err != nil {
			return fmt.Errorf("inspect managed source %s: %w", source.Path, err)
		}
		if symlinked {
			return fmt.Errorf("symlinked managed source is forbidden: %s", source.Path)
		}
		if strings.TrimSpace(source.Type) == "" || strings.TrimSpace(source.Title) == "" {
			return fmt.Errorf("managed source %s requires type and title", source.ID)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(source.Path))); err != nil {
			return fmt.Errorf("managed source %s: %w", source.Path, err)
		}
	}
	for _, source := range allowlist.Sources {
		for _, related := range source.Related {
			if related == source.ID || !seen[related] {
				return fmt.Errorf("source %s has invalid related id %q", source.ID, related)
			}
		}
	}
	return nil
}

func safeManagedID(id string) bool {
	if id == "" || len(id) > 96 || id[0] == '-' || id[len(id)-1] == '-' {
		return false
	}
	for _, char := range id {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
			return false
		}
	}
	return true
}

func approvedManagedSource(path string) bool {
	if path == "README.md" || path == "ROADMAP.md" || strings.HasPrefix(path, "specs/") {
		return true
	}
	if !strings.HasPrefix(path, "docs/") {
		return false
	}
	for _, blocked := range []string{"docs/decisions/", "docs/onboarding/", "docs/pilot/"} {
		if strings.HasPrefix(path, blocked) {
			return false
		}
	}
	return true
}

func safeManagedRelative(path string) bool {
	if path == "" || strings.Contains(path, `\`) || filepath.IsAbs(path) {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	return clean == path && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../") && !strings.Contains(clean, "/../")
}

func validISODate(value string) bool {
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func managedFingerprint(root string, sources []ManagedSource) (string, error) {
	hash := sha256.New()
	for _, source := range sources {
		body, err := readManagedSource(filepath.Join(root, filepath.FromSlash(source.Path)))
		if err != nil {
			return "", err
		}
		metadata, err := json.Marshal(source)
		if err != nil {
			return "", err
		}
		_, _ = hash.Write(metadata)
		_, _ = io.WriteString(hash, "\x00")
		_, _ = hash.Write(body)
		_, _ = io.WriteString(hash, "\x00")
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func writeManagedConcept(bundleRoot, sourceRoot string, source ManagedSource, byID map[string]ManagedSource, fingerprint string, allowlist ManagedAllowlist, linkDiagnostics *managedLinkDiagnostics) error {
	body, err := readManagedSource(filepath.Join(sourceRoot, filepath.FromSlash(source.Path)))
	if err != nil {
		return err
	}
	header := conceptHeader{
		Type:              source.Type,
		Title:             source.Title,
		Description:       source.Description,
		Resource:          "repo://" + source.Path,
		Tags:              append([]string(nil), source.Tags...),
		Status:            "stable",
		ProfileVersion:    "1",
		StableID:          "managed/" + source.ID,
		Scope:             managedScope,
		SourceFingerprint: fingerprint,
		Freshness:         "fresh",
		BCGOSStatus:       "active",
		GeneratorVersion:  allowlist.GeneratorVersion,
		PolicyVersion:     allowlist.PolicyVersion,
		Sources:           []sourceRef{{ID: source.ID, Resource: "repo://" + source.Path, Title: source.Title}},
	}
	encoded, err := yaml.Marshal(header)
	if err != nil {
		return err
	}
	body, err = rewriteManagedMarkdownLinks(body, sourceRoot, source, byID, linkDiagnostics)
	if err != nil {
		return err
	}
	var builder strings.Builder
	builder.WriteString("---\n")
	builder.Write(encoded)
	builder.WriteString("---\n\n# Source snapshot\n\n")
	builder.WriteString("This managed concept is generated from the reviewed repository source `")
	builder.WriteString(source.Path)
	builder.WriteString("`. The source remains authoritative.\n\n")
	if len(source.Related) > 0 {
		builder.WriteString("## Related\n\n")
		for _, related := range source.Related {
			builder.WriteString("- [")
			builder.WriteString(byID[related].Title)
			builder.WriteString("](/concepts/")
			builder.WriteString(related)
			builder.WriteString(".md)\n")
		}
		builder.WriteString("\n")
	}
	builder.WriteString("## Source content\n\n")
	builder.Write(body)
	path := filepath.Join(bundleRoot, "concepts", source.ID+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(builder.String()), 0o644)
}

func rewriteManagedMarkdownLinks(body []byte, sourceRoot string, source ManagedSource, byID map[string]ManagedSource, diagnostics *managedLinkDiagnostics) ([]byte, error) {
	byPath := make(map[string]ManagedSource, len(byID))
	for _, candidate := range byID {
		byPath[candidate.Path] = candidate
	}
	text := string(body)
	var rewriteErr error
	rewritten := managedMarkdownLinkPattern.ReplaceAllStringFunc(text, func(match string) string {
		if rewriteErr != nil {
			return match
		}
		submatch := managedMarkdownLinkPattern.FindStringSubmatch(match)
		if len(submatch) != 2 {
			return match
		}
		raw := submatch[1]
		destination, suffix := splitManagedMarkdownDestination(raw)
		if destination == "" || isNonFileManagedLink(destination) {
			return match
		}
		resolved, err := resolveManagedSourceLink(source.Path, destination)
		if err != nil {
			rewriteErr = fmt.Errorf("%s: %w", source.Path, err)
			return match
		}
		if target, ok := byPath[resolved]; ok {
			newDestination := "/concepts/" + target.ID + ".md" + managedLinkSuffix(destination, suffix)
			if newDestination != destination+suffix {
				diagnostics.addRewritten(source.Path, raw, newDestination)
			}
			return "](" + newDestination + ")"
		}
		absolute := filepath.Join(sourceRoot, filepath.FromSlash(resolved))
		info, statErr := os.Stat(absolute)
		if statErr != nil || !info.Mode().IsRegular() {
			link := source.Path + ":" + destination
			diagnostics.BrokenLinks = append(diagnostics.BrokenLinks, link)
			rewriteErr = fmt.Errorf("broken markdown link %q in %s", destination, source.Path)
			return match
		}
		newDestination := "repo://" + resolved + managedLinkSuffix(destination, suffix)
		diagnostics.addOpaque(source.Path, newDestination)
		return "](" + newDestination + ")"
	})
	if rewriteErr != nil {
		return nil, rewriteErr
	}
	diagnostics.sort()
	return []byte(rewritten), nil
}

func splitManagedMarkdownDestination(raw string) (string, string) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ""
	}
	if strings.HasPrefix(trimmed, "<") {
		if end := strings.IndexByte(trimmed, '>'); end >= 0 {
			return trimmed[1:end], trimmed[end+1:]
		}
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return "", ""
	}
	return fields[0], strings.TrimPrefix(trimmed, fields[0])
}

func isNonFileManagedLink(destination string) bool {
	if strings.HasPrefix(destination, "#") {
		return true
	}
	parsed, err := url.Parse(destination)
	return err == nil && (parsed.Scheme != "" || parsed.Host != "")
}

func resolveManagedSourceLink(sourcePath, destination string) (string, error) {
	parsed, err := url.Parse(destination)
	if err != nil || parsed.Path == "" {
		return "", fmt.Errorf("invalid markdown link %q in %s", destination, sourcePath)
	}
	resolved := pathpkg.Clean(pathpkg.Join(pathpkg.Dir(filepath.ToSlash(sourcePath)), parsed.Path))
	if resolved == "." || resolved == ".." || strings.HasPrefix(resolved, "../") {
		return "", fmt.Errorf("markdown link escapes repository root: %q in %s", destination, sourcePath)
	}
	return resolved, nil
}

func managedLinkSuffix(destination, suffix string) string {
	parsed, err := url.Parse(destination)
	if err != nil {
		return suffix
	}
	var builder strings.Builder
	if parsed.RawQuery != "" {
		builder.WriteByte('?')
		builder.WriteString(parsed.RawQuery)
	}
	if parsed.Fragment != "" {
		builder.WriteByte('#')
		builder.WriteString(parsed.Fragment)
	}
	return builder.String() + suffix
}

func readManagedSource(path string) ([]byte, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// Git checkouts may use CRLF on Windows. Normalize source bytes before
	// fingerprinting and compilation so the checked-in managed bundle is
	// deterministic across supported platforms.
	return bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n")), nil
}

func writeManagedIndex(bundleRoot string, sources []ManagedSource, fingerprint string, allowlist ManagedAllowlist) error {
	var builder strings.Builder
	fmt.Fprintf(&builder, "okf_version: %q\n\n# Managed Maestro Atlas\n\n", allowlist.OKFVersion)
	builder.WriteString("Deterministic managed knowledge bundle compiled from an explicit product-source allowlist.\n\n")
	fmt.Fprintf(&builder, "Source watermark: `%s`\n\n## Concepts\n\n", fingerprint)
	for _, source := range sources {
		fmt.Fprintf(&builder, "- [%s](/concepts/%s.md) - %s\n", source.Title, source.ID, source.Description)
	}
	return os.WriteFile(filepath.Join(bundleRoot, "index.md"), []byte(builder.String()), 0o644)
}

func writeManagedLog(bundleRoot string, sources []ManagedSource, fingerprint string, allowlist ManagedAllowlist) error {
	body := fmt.Sprintf("# Directory Update Log\n\n## %s\n\n* **Reconcile**: compiled %d allowlisted concepts with source watermark `%s`.\n* **Policy**: BCGOS managed scope, generator `%s`, policy `%s`.\n", allowlist.LogDate, len(sources), fingerprint, allowlist.GeneratorVersion, allowlist.PolicyVersion)
	return os.WriteFile(filepath.Join(bundleRoot, "log.md"), []byte(body), 0o644)
}

func writeManagedReports(bundleRoot string, sources []ManagedSource, byID map[string]ManagedSource, linkDiagnostics managedLinkDiagnostics) error {
	backlinks := make(map[string][]string, len(sources))
	for _, source := range sources {
		for _, related := range source.Related {
			backlinks[related] = append(backlinks[related], source.ID)
		}
	}
	for key := range backlinks {
		sort.Strings(backlinks[key])
	}
	for _, source := range sources {
		if _, ok := backlinks[source.ID]; !ok {
			backlinks[source.ID] = []string{}
		}
	}
	backlinkJSON, err := json.MarshalIndent(backlinks, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(bundleRoot, "backlinks.json"), append(backlinkJSON, '\n'), 0o644); err != nil {
		return err
	}
	linkDiagnostics.sort()
	diagnostics := map[string]any{
		"broken_links":    linkDiagnostics.BrokenLinks,
		"opaque_links":    linkDiagnostics.OpaqueLinks,
		"rewritten_links": linkDiagnostics.RewrittenLinks,
		"orphans":         []string{},
	}
	for _, source := range sources {
		if len(source.Related) == 0 && len(backlinks[source.ID]) == 0 && len(byID) > 1 {
			diagnostics["orphans"] = append(diagnostics["orphans"].([]string), source.ID)
		}
	}
	diagnosticJSON, err := json.MarshalIndent(diagnostics, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(bundleRoot, "diagnostics.json"), append(diagnosticJSON, '\n'), 0o644)
}

func validateManagedMarkdownLinks(root string) error {
	return filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".md" {
			return nil
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var linkErr error
		managedMarkdownLinkPattern.ReplaceAllStringFunc(string(body), func(match string) string {
			if linkErr != nil {
				return match
			}
			submatch := managedMarkdownLinkPattern.FindStringSubmatch(match)
			if len(submatch) != 2 {
				return match
			}
			destination, _ := splitManagedMarkdownDestination(submatch[1])
			if destination == "" || isNonFileManagedLink(destination) {
				return match
			}
			parsed, parseErr := url.Parse(destination)
			if parseErr != nil || parsed.Path == "" {
				linkErr = fmt.Errorf("invalid markdown link %q in %s", destination, path)
				return match
			}
			var target string
			if strings.HasPrefix(parsed.Path, "/") {
				target = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(parsed.Path, "/")))
			} else {
				target = filepath.Join(filepath.Dir(path), filepath.FromSlash(parsed.Path))
			}
			info, statErr := os.Stat(target)
			if statErr != nil || !info.Mode().IsRegular() {
				linkErr = fmt.Errorf("broken generated markdown link %q in %s", destination, path)
			}
			return match
		})
		return linkErr
	})
}

func parseConceptHeader(body []byte) (conceptHeader, error) {
	text := string(body)
	if !strings.HasPrefix(text, "---\n") {
		return conceptHeader{}, errors.New("missing YAML frontmatter")
	}
	remainder := text[len("---\n"):]
	end := strings.Index(remainder, "\n---\n")
	if end < 0 {
		return conceptHeader{}, errors.New("unterminated YAML frontmatter")
	}
	var header conceptHeader
	if err := yaml.Unmarshal([]byte(remainder[:end]), &header); err != nil {
		return conceptHeader{}, fmt.Errorf("parse YAML frontmatter: %w", err)
	}
	return header, nil
}

func atomicPublishDirectory(stage, output string) error {
	if info, err := os.Lstat(output); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("managed output cannot be a symlink: %s", output)
	}
	backup := output + ".previous"
	_ = os.RemoveAll(backup)
	if _, err := os.Stat(output); err == nil {
		if err := os.Rename(output, backup); err != nil {
			return err
		}
	}
	if err := os.Rename(stage, output); err != nil {
		_ = os.Rename(backup, output)
		return err
	}
	return os.RemoveAll(backup)
}

func directorySnapshot(root string) (string, error) {
	var builder strings.Builder
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// A Windows checkout may materialize tracked Markdown as CRLF while
		// reconciliation writes LF. Compare semantic bytes, not checkout EOL.
		body = bytes.ReplaceAll(body, []byte("\r\n"), []byte("\n"))
		builder.WriteString(filepath.ToSlash(relative))
		builder.WriteByte('\n')
		builder.Write(body)
		builder.WriteString("\n---\n")
		return nil
	})
	return builder.String(), err
}

func hasSymlinkComponent(root, relative string) (bool, error) {
	current := root
	for _, component := range strings.Split(relative, "/") {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if err != nil {
			return false, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return true, nil
		}
	}
	return false, nil
}
