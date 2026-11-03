// Package privatewiki compiles bounded navigation data. It owns no IO or authority.
package privatewiki

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const (
	Profile         = "private-memory-local-v1"
	AuthorityKind   = "local-home-workspace"
	CompilerVersion = "private-memory-compiler-v1"
	MaxSources      = 128
	MaxSourceBytes  = 1 << 20
	MaxInputBytes   = 8 << 20
	MaxFiles        = 1024
	MaxOutputBytes  = 16 << 20
)

var (
	ErrInvalid   = errors.New("private_wiki_invalid_profile_or_graph")
	ErrBudget    = errors.New("private_wiki_budget_exceeded")
	hexID        = regexp.MustCompile(`^[0-9a-f]{64}$`)
	periodID     = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	markdownLink = regexp.MustCompile(`!?\[[^\]]*\]\(([^)]+)\)`)
)

type Source struct {
	ID, Version, Reference, Layer, Period string
	Content                               string `json:"-"`
}

type Input struct {
	HomeRef, WorkspaceRef, CommitRef, Watermark string
	DenialGeneration                            uint64
	Implementation, Configuration, PolicyRef    string
	CommittedAt                                 time.Time
	Sources                                     []Source
}

type Concept struct {
	Reference string `json:"reference"`
	Version   string `json:"version"`
	Layer     string `json:"layer"`
	Period    string `json:"period"`
}

type Manifest struct {
	SchemaVersion    int                `json:"schema_version"`
	Profile          string             `json:"profile"`
	Authority        string             `json:"authority_kind"`
	Generation       string             `json:"generation"`
	HomeRef          string             `json:"home_ref"`
	WorkspaceRef     string             `json:"workspace_ref"`
	CommitRef        string             `json:"commit_ref"`
	SourceWatermark  string             `json:"source_watermark"`
	DenialGeneration uint64             `json:"denial_generation"`
	Compiler         string             `json:"compiler"`
	Implementation   string             `json:"implementation"`
	Configuration    string             `json:"configuration"`
	PolicyRef        string             `json:"policy_ref"`
	Concepts         map[string]Concept `json:"concepts"`
	Files            map[string]string  `json:"files"`
}

type Candidate struct {
	Manifest Manifest
	Files    map[string][]byte
}

func Digest(body []byte) string { h := sha256.Sum256(body); return hex.EncodeToString(h[:]) }
func ValidID(s string) bool     { return hexID.MatchString(s) }

func Compile(in Input) (Candidate, error) {
	if err := validateInput(in); err != nil {
		return Candidate{}, err
	}
	sources := append([]Source(nil), in.Sources...)
	sort.Slice(sources, func(i, j int) bool {
		return sources[i].Layer+"/"+sources[i].Period+"/"+sources[i].ID < sources[j].Layer+"/"+sources[j].Period+"/"+sources[j].ID
	})
	identity, _ := json.Marshal([]any{Profile, CompilerVersion, in.HomeRef, in.WorkspaceRef, in.CommitRef, in.Watermark, in.DenialGeneration, in.Implementation, in.Configuration, in.PolicyRef})
	m := Manifest{SchemaVersion: 1, Profile: Profile, Authority: AuthorityKind, Generation: Digest(identity), HomeRef: in.HomeRef, WorkspaceRef: in.WorkspaceRef, CommitRef: in.CommitRef, SourceWatermark: in.Watermark, DenialGeneration: in.DenialGeneration, Compiler: CompilerVersion, Implementation: in.Implementation, Configuration: in.Configuration, PolicyRef: in.PolicyRef, Concepts: map[string]Concept{}, Files: map[string]string{}}
	files := map[string][]byte{}
	layers := map[string][]Source{}
	backlinks := map[string][]string{}
	index := commonFrontmatter(m, "Memory Index", "Memory navigation", in.CommittedAt) + "okf_version: \"0.2\"\n---\n\n# Memory navigation\n\nDerived routes over the current authorized committed rollups.\n\n"
	for _, s := range sources {
		name := "concepts/" + s.ID + ".md"
		layerIndex := "layers/" + strings.ToLower(s.Layer) + "/index.md"
		m.Concepts[name] = Concept{s.Reference, s.Version, s.Layer, s.Period}
		layers[s.Layer] = append(layers[s.Layer], s)
		backlinks[name] = []string{"index.md", layerIndex}
		index += fmt.Sprintf("- [%s %s](%s)\n", s.Layer, s.Period, name)
		body := commonFrontmatter(m, "Memory Rollup", s.Layer+" "+s.Period, in.CommittedAt)
		body += "resource: " + strconv.Quote("maestro://memory/"+s.Reference) + "\n"
		body += "x-maestro-stable-id: " + strconv.Quote(s.ID) + "\n"
		body += "x-maestro-memory-layer: " + strconv.Quote(s.Layer) + "\n"
		body += "x-maestro-rollup-version: " + strconv.Quote(s.Version) + "\n---\n\n# " + s.Layer + " " + s.Period + "\n\n## Summary\n\n"
		runes := []rune(s.Content)
		truncated := len(runes) > 2048
		if truncated {
			runes = runes[:2048]
		}
		summary := string(runes)
		fence := strings.Repeat("`", maxBacktickRun(summary)+3)
		body += fence + "text\n" + summary + "\n" + fence + "\n"
		if truncated {
			body += "\nSummary is bounded; use the source pointer for the complete rollup.\n"
		}
		body += "\n[Source rollup](maestro://memory/" + s.Reference + ")\n\n## Backlinks\n\n- [Memory index](../index.md)\n- [Layer index](../" + layerIndex + ")\n"
		files[name] = []byte(body)
	}
	var layerNames []string
	for l := range layers {
		layerNames = append(layerNames, l)
	}
	sort.Strings(layerNames)
	index += "\n## Layers\n\n"
	for _, l := range layerNames {
		name := "layers/" + strings.ToLower(l) + "/index.md"
		index += "- [" + l + "](" + name + ")\n"
		body := commonFrontmatter(m, "Layer Index", l+" navigation", in.CommittedAt) + "---\n\n# " + l + " navigation\n\n[Memory index](../../index.md)\n\n"
		for _, s := range layers[l] {
			body += fmt.Sprintf("- [%s %s](../../concepts/%s.md)\n", s.Layer, s.Period, s.ID)
		}
		files[name] = []byte(body)
	}
	files["index.md"] = []byte(index)
	files["log.md"] = []byte(commonFrontmatter(m, "Update Log", "Private compilation metadata", in.CommittedAt) + "---\n\n# Update metadata\n\nProfile: " + Profile + "\n\nGeneration: " + m.Generation + "\n\nSource watermark: " + m.SourceWatermark + "\n\n[Memory index](index.md)\n")
	b, err := json.MarshalIndent(backlinks, "", "  ")
	if err != nil {
		return Candidate{}, err
	}
	files["backlinks.json"] = append(b, '\n')
	for n, b := range files {
		m.Files[n] = Digest(b)
	}
	b, err = json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Candidate{}, err
	}
	files["manifest.json"] = append(b, '\n')
	c := Candidate{m, files}
	if err := Validate(c); err != nil {
		return Candidate{}, err
	}
	return c, nil
}

func validateInput(in Input) error {
	for _, id := range []string{in.HomeRef, in.WorkspaceRef, in.CommitRef, in.Watermark, in.Implementation, in.Configuration, in.PolicyRef} {
		if !ValidID(id) {
			return ErrInvalid
		}
	}
	if in.CommittedAt.IsZero() || len(in.Sources) == 0 {
		return ErrInvalid
	}
	if len(in.Sources) > MaxSources {
		return ErrBudget
	}
	total := 0
	seen := map[string]bool{}
	refs := map[string]bool{}
	for _, s := range in.Sources {
		if !ValidID(s.ID) || !ValidID(s.Version) || !ValidID(s.Reference) || seen[s.ID] || refs[s.Reference] || !periodID.MatchString(s.Period) || !validLayer(s.Layer) || s.Content == "" || !utf8.ValidString(s.Content) {
			return ErrInvalid
		}
		seen[s.ID] = true
		refs[s.Reference] = true
		total += len(s.Content)
		if len(s.Content) > MaxSourceBytes || total > MaxInputBytes {
			return ErrBudget
		}
	}
	return nil
}

func commonFrontmatter(m Manifest, kind, title string, at time.Time) string {
	fields := [][2]string{{"type", kind}, {"title", title}, {"timestamp", at.UTC().Format(time.RFC3339Nano)}, {"x-maestro-profile", Profile}, {"x-maestro-profile-version", "1"}, {"x-maestro-authority-kind", AuthorityKind}, {"x-maestro-scope", "workspace"}, {"x-maestro-owner-ref", m.HomeRef}, {"x-maestro-workspace-ref", m.WorkspaceRef}, {"x-maestro-memory-commit", m.CommitRef}, {"x-maestro-source-fingerprint", m.SourceWatermark}, {"x-maestro-sensitivity", "client_restricted"}, {"x-maestro-freshness", "source-current"}, {"x-maestro-status", "active"}, {"x-maestro-generator-version", CompilerVersion}, {"x-maestro-policy-version", "1"}, {"x-maestro-source-policy", m.PolicyRef}, {"x-maestro-implementation", m.Implementation}, {"x-maestro-configuration", m.Configuration}}
	b := "---\n"
	for _, f := range fields {
		b += f[0] + ": " + strconv.Quote(f[1]) + "\n"
	}
	return b + "x-maestro-denial-generation: " + strconv.FormatUint(m.DenialGeneration, 10) + "\n"
}

func validLayer(l string) bool { return l == "L1" || l == "L2" || l == "L3" || l == "lifetime" }
func maxBacktickRun(s string) int {
	n, best := 0, 0
	for _, r := range s {
		if r == '`' {
			n++
			if n > best {
				best = n
			}
		} else {
			n = 0
		}
	}
	return best
}

// Validate checks actual bytes, profile values and graph relations independently
// of publication. It neither grants access nor reads a filesystem.
func Validate(c Candidate) error {
	m := c.Manifest
	if m.SchemaVersion != 1 || m.Profile != Profile || m.Authority != AuthorityKind || m.Compiler != CompilerVersion || len(m.Concepts) == 0 || len(m.Concepts) > MaxSources {
		return ErrInvalid
	}
	for _, id := range []string{m.Generation, m.HomeRef, m.WorkspaceRef, m.CommitRef, m.SourceWatermark, m.Implementation, m.Configuration, m.PolicyRef} {
		if !ValidID(id) {
			return ErrInvalid
		}
	}
	identity, _ := json.Marshal([]any{Profile, CompilerVersion, m.HomeRef, m.WorkspaceRef, m.CommitRef, m.SourceWatermark, m.DenialGeneration, m.Implementation, m.Configuration, m.PolicyRef})
	if Digest(identity) != m.Generation {
		return ErrInvalid
	}
	if len(c.Files) != len(m.Files)+1 || len(c.Files) > MaxFiles {
		return ErrBudget
	}
	total := 0
	refs := map[string]bool{}
	for name, concept := range m.Concepts {
		if !strings.HasPrefix(name, "concepts/") || !strings.HasSuffix(name, ".md") || !ValidID(strings.TrimSuffix(strings.TrimPrefix(name, "concepts/"), ".md")) || !ValidID(concept.Reference) || !ValidID(concept.Version) || !validLayer(concept.Layer) || !periodID.MatchString(concept.Period) {
			return ErrInvalid
		}
		refs["maestro://memory/"+concept.Reference] = true
	}
	actualBacklinks := map[string][]string{}
	for name, body := range c.Files {
		if !safeName(name) || !utf8.Valid(body) {
			return ErrInvalid
		}
		total += len(body)
		if total > MaxOutputBytes {
			return ErrBudget
		}
		if name == "manifest.json" {
			continue
		}
		if m.Files[name] != Digest(body) {
			return ErrInvalid
		}
		if !strings.HasSuffix(name, ".md") {
			if name != "backlinks.json" {
				return ErrInvalid
			}
			continue
		}
		meta, text, err := frontmatter(body)
		if err != nil {
			return err
		}
		if err = validateMetadata(meta, m, name); err != nil {
			return err
		}
		targets, err := markdownTargets(name, text, c.Files, refs)
		if err != nil {
			return err
		}
		for _, target := range targets {
			if _, ok := m.Concepts[target]; ok {
				actualBacklinks[target] = append(actualBacklinks[target], name)
			}
		}
	}
	var stored Manifest
	if err := strictJSON(c.Files["manifest.json"], &stored); err != nil {
		return ErrInvalid
	}
	a, _ := json.Marshal(m)
	b, _ := json.Marshal(stored)
	if !bytes.Equal(a, b) {
		return ErrInvalid
	}
	if _, ok := c.Files["index.md"]; !ok {
		return ErrInvalid
	}
	if _, ok := c.Files["log.md"]; !ok {
		return ErrInvalid
	}
	var declared map[string][]string
	if strictJSON(c.Files["backlinks.json"], &declared) != nil || len(declared) != len(m.Concepts) {
		return ErrInvalid
	}
	for name := range m.Concepts {
		actual := uniqueSorted(actualBacklinks[name])
		expected := uniqueSorted(declared[name])
		if strings.Join(actual, "\x00") != strings.Join(expected, "\x00") || len(actual) == 0 {
			return ErrInvalid
		}
	}
	return nil
}

func safeName(n string) bool {
	return n != "" && n == path.Clean(n) && !strings.HasPrefix(n, "/") && !strings.Contains(n, "\\") && !strings.HasPrefix(n, "../") && !strings.ContainsRune(n, 0) && len(n) <= 192
}
func strictJSON(b []byte, out any) error {
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if e := d.Decode(out); e != nil {
		return e
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return ErrInvalid
	}
	return nil
}
func DecodeManifest(b []byte) (Manifest, error) { var m Manifest; e := strictJSON(b, &m); return m, e }

func frontmatter(b []byte) (map[string]any, string, error) {
	s := string(b)
	if !strings.HasPrefix(s, "---\n") {
		return nil, "", ErrInvalid
	}
	i := strings.Index(s[4:], "\n---\n")
	if i < 0 || i > 16384 {
		return nil, "", ErrInvalid
	}
	header := s[4 : 4+i]
	var node yaml.Node
	if yaml.Unmarshal([]byte(header), &node) != nil {
		return nil, "", ErrInvalid
	}
	count := 0
	var check func(*yaml.Node) bool
	check = func(n *yaml.Node) bool {
		count++
		if count > 512 || n.Kind == yaml.AliasNode {
			return false
		}
		for _, c := range n.Content {
			if !check(c) {
				return false
			}
		}
		return true
	}
	if !check(&node) {
		return nil, "", ErrInvalid
	}
	var meta map[string]any
	if node.Decode(&meta) != nil || meta == nil {
		return nil, "", ErrInvalid
	}
	return meta, s[4+i+5:], nil
}

func validateMetadata(meta map[string]any, m Manifest, name string) error {
	common := map[string]string{"x-maestro-profile": Profile, "x-maestro-profile-version": "1", "x-maestro-authority-kind": AuthorityKind, "x-maestro-scope": "workspace", "x-maestro-owner-ref": m.HomeRef, "x-maestro-workspace-ref": m.WorkspaceRef, "x-maestro-memory-commit": m.CommitRef, "x-maestro-source-fingerprint": m.SourceWatermark, "x-maestro-sensitivity": "client_restricted", "x-maestro-freshness": "source-current", "x-maestro-status": "active", "x-maestro-generator-version": CompilerVersion, "x-maestro-policy-version": "1", "x-maestro-source-policy": m.PolicyRef, "x-maestro-implementation": m.Implementation, "x-maestro-configuration": m.Configuration}
	if kind, ok := meta["type"].(string); !ok || strings.TrimSpace(kind) == "" {
		return ErrInvalid
	}
	for k, v := range common {
		if meta[k] != v {
			return ErrInvalid
		}
	}
	gen := fmt.Sprint(meta["x-maestro-denial-generation"])
	if gen != strconv.FormatUint(m.DenialGeneration, 10) {
		return ErrInvalid
	}
	allowed := map[string]bool{"x-maestro-denial-generation": true, "x-maestro-stable-id": true, "x-maestro-memory-layer": true, "x-maestro-rollup-version": true}
	for k := range common {
		allowed[k] = true
	}
	for k := range meta {
		if strings.HasPrefix(k, "x-maestro-") && !allowed[k] {
			return ErrInvalid
		}
	}
	if name == "index.md" && meta["okf_version"] != "0.2" {
		return ErrInvalid
	}
	if concept, ok := m.Concepts[name]; ok {
		if meta["x-maestro-stable-id"] != strings.TrimSuffix(strings.TrimPrefix(name, "concepts/"), ".md") || meta["x-maestro-memory-layer"] != concept.Layer || meta["x-maestro-rollup-version"] != concept.Version || meta["resource"] != "maestro://memory/"+concept.Reference {
			return ErrInvalid
		}
	} else if path.Base(name) != "index.md" && name != "log.md" {
		return ErrInvalid
	}
	return nil
}

func markdownTargets(name, text string, files map[string][]byte, refs map[string]bool) ([]string, error) {
	var targets []string
	fence := 0
	for _, line := range strings.Split(text, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "```") {
			n := 0
			for n < len(t) && t[n] == '`' {
				n++
			}
			if fence == 0 {
				fence = n
			} else if n >= fence && strings.TrimSpace(t[n:]) == "" {
				fence = 0
			}
			continue
		}
		if fence > 0 {
			continue
		}
		if strings.ContainsAny(line, "<>") || strings.Contains(line, "][") || strings.HasPrefix(t, "[") && strings.Contains(t, "]: ") {
			return nil, ErrInvalid
		}
		for _, match := range markdownLink.FindAllStringSubmatch(line, -1) {
			raw := match[1]
			if strings.ContainsAny(raw, " \t\r\n\\\x00") {
				return nil, ErrInvalid
			}
			if strings.Contains(raw, ":") {
				if !refs[raw] {
					return nil, ErrInvalid
				}
				continue
			}
			u, err := url.PathUnescape(raw)
			if err != nil || u != raw || strings.ContainsAny(raw, "?#") {
				return nil, ErrInvalid
			}
			target := path.Clean(path.Join(path.Dir(name), raw))
			if !safeName(target) {
				return nil, ErrInvalid
			}
			if _, ok := files[target]; !ok {
				return nil, ErrInvalid
			}
			targets = append(targets, target)
		}
		if strings.Contains(markdownLink.ReplaceAllString(line, ""), "://") {
			return nil, ErrInvalid
		}
	}
	if fence != 0 {
		return nil, ErrInvalid
	}
	return uniqueSorted(targets), nil
}

func uniqueSorted(in []string) []string {
	seen := map[string]bool{}
	for _, s := range in {
		seen[s] = true
	}
	out := make([]string, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
