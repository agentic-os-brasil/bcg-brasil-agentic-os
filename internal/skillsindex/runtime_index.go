package skillsindex

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

type RuntimePointer struct {
	ID      string `json:"id"`
	Pointer string `json:"pointer"`
}
type RuntimeIndexSource struct {
	Implementation   string           `json:"implementation,omitempty"`
	Configuration    string           `json:"configuration,omitempty"`
	WorkspaceID      string           `json:"workspace_id"`
	Runtime          string           `json:"runtime"`
	ProjectionSHA256 string           `json:"projection_sha256"`
	Catalog          Catalog          `json:"catalog"`
	Pointers         []RuntimePointer `json:"pointers"`
}
type RuntimeIndexMetadata struct {
	SchemaVersion  int                `json:"schema_version"`
	Source         RuntimeIndexSource `json:"source"`
	CatalogSHA256  string             `json:"catalog_sha256"`
	MarkdownSHA256 string             `json:"markdown_sha256"`
}
type RuntimeIndex struct {
	Metadata     RuntimeIndexMetadata
	Generation   string
	MetadataPath string
}

func indexDigest(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func indexJSON(v any) []byte      { b, _ := json.Marshal(v); return b }
func validIndexHash(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && strings.ToLower(s) == s
}
func renderRuntimeIndex(source RuntimeIndexSource) ([]byte, []byte, error) {
	if source.WorkspaceID == "" || (source.Runtime != "claude" && source.Runtime != "codex") || !validIndexHash(source.ProjectionSHA256) {
		return nil, nil, errors.New("invalid installed index identity")
	}
	// Historical generations without provenance remain readable as old evidence.
	// New publication requires both digests; current qualification checks them.
	if (source.Implementation != "" || source.Configuration != "") && (!validIndexHash(source.Implementation) || !validIndexHash(source.Configuration)) {
		return nil, nil, errors.New("invalid index compiler provenance")
	}
	if err := source.Catalog.Validate(); err != nil {
		return nil, nil, err
	}
	if len(source.Pointers) != len(source.Catalog.Skills) {
		return nil, nil, errors.New("installed pointer coverage mismatch")
	}
	type entry struct {
		ID              string `json:"id"`
		DisplayName     string `json:"display_name"`
		Trigger         string `json:"trigger"`
		DefaultPrompt   string `json:"default_prompt"`
		UserInvokedOnly bool   `json:"user_invoked_only"`
		Pointer         string `json:"pointer"`
	}
	entries := make([]entry, 0, len(source.Pointers))
	var md strings.Builder
	md.WriteString("# Installed Maestro Skills Index\n\n| Skill | Use when | Workspace-relative pointer |\n|---|---|---|\n")
	for i, p := range source.Pointers {
		skill := source.Catalog.Skills[i]
		if p.ID != skill.ID || filepath.IsAbs(p.Pointer) || filepath.ToSlash(filepath.Clean(p.Pointer)) != p.Pointer || strings.Contains(p.Pointer, "..") || !strings.HasSuffix(p.Pointer, "/"+p.ID+"/SKILL.md") {
			return nil, nil, errors.New("invalid installed skill pointer")
		}
		entries = append(entries, entry{ID: skill.ID, DisplayName: skill.DisplayName, Trigger: skill.Trigger, DefaultPrompt: skill.DefaultPrompt, UserInvokedOnly: skill.UserInvokedOnly, Pointer: p.Pointer})
		fmt.Fprintf(&md, "| %s | %s | \x60%s\x60 |\n", skill.DisplayName, skill.Trigger, p.Pointer)
	}
	return indexJSON(entries), []byte(md.String()), nil
}

// PublishRuntimeIndex owns only an external runtime index, never factory files.
// Its lock precedes the service control/lease guard. Both files are immutable;
// current.json is the single visible publication boundary.
func PublishRuntimeIndex(root string, source RuntimeIndexSource, guard func(func() error) error) (RuntimeIndex, bool, error) {
	var out RuntimeIndex
	changed := false
	if !validIndexHash(source.Implementation) || !validIndexHash(source.Configuration) {
		return out, false, errors.New("index compiler provenance required")
	}
	if guard == nil {
		return out, false, errors.New("index publication guard required")
	}
	err := scheduler.WithPrivateLock(filepath.Join(root, "publication.guard"), func() error {
		jsonBody, mdBody, err := renderRuntimeIndex(source)
		if err != nil {
			return err
		}
		metadata := RuntimeIndexMetadata{1, source, indexDigest(jsonBody), indexDigest(mdBody)}
		body := indexJSON(metadata)
		id := indexDigest(body)
		dir := filepath.Join(root, "generations", id)
		old, err := LoadRuntimeIndex(root, "")
		if err == nil && old.Generation == id {
			out = old
			return nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err := scheduler.EnsurePrivateDirectory(dir); err != nil {
			return err
		}
		for _, f := range []struct {
			name string
			body []byte
		}{{"catalog.json", jsonBody}, {"INDEX.md", mdBody}, {"metadata.json", body}} {
			if err := scheduler.WritePrivateFileAtomic(filepath.Join(dir, f.name), f.body); err != nil {
				return err
			}
		}
		out, err = LoadRuntimeIndex(root, id)
		if err != nil {
			return err
		}
		if err := guard(func() error {
			return scheduler.WritePrivateFileAtomic(filepath.Join(root, "current.json"), indexJSON(struct {
				Generation string `json:"generation"`
			}{id}))
		}); err != nil {
			return err
		}
		changed = true
		return nil
	})
	return out, changed, err
}

// LoadRuntimeIndex verifies the complete immutable pair independently of a
// successful write call. generation empty means the current published version.
func LoadRuntimeIndex(root, generation string) (RuntimeIndex, error) {
	if generation == "" {
		body, err := scheduler.ReadPrivateFile(filepath.Join(root, "current.json"), 1024)
		if err != nil {
			return RuntimeIndex{}, err
		}
		var pointer struct {
			Generation string `json:"generation"`
		}
		if err = json.Unmarshal(body, &pointer); err != nil {
			return RuntimeIndex{}, err
		}
		generation = pointer.Generation
	}
	if !validIndexHash(generation) {
		return RuntimeIndex{}, errors.New("invalid index generation")
	}
	dir := filepath.Join(root, "generations", generation)
	body, err := scheduler.ReadPrivateFile(filepath.Join(dir, "metadata.json"), 2<<20)
	if err != nil {
		return RuntimeIndex{}, err
	}
	if indexDigest(body) != generation {
		return RuntimeIndex{}, errors.New("index metadata digest mismatch")
	}
	var metadata RuntimeIndexMetadata
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err = dec.Decode(&metadata); err != nil {
		return RuntimeIndex{}, err
	}
	if metadata.SchemaVersion != 1 {
		return RuntimeIndex{}, errors.New("invalid index schema")
	}
	expectedJSON, expectedMD, err := renderRuntimeIndex(metadata.Source)
	if err != nil {
		return RuntimeIndex{}, err
	}
	for _, f := range []struct {
		name, hash string
		expected   []byte
	}{{"catalog.json", metadata.CatalogSHA256, expectedJSON}, {"INDEX.md", metadata.MarkdownSHA256, expectedMD}} {
		body, err = scheduler.ReadPrivateFile(filepath.Join(dir, f.name), 2<<20)
		if err != nil {
			return RuntimeIndex{}, err
		}
		if indexDigest(body) != f.hash || !bytes.Equal(body, f.expected) {
			return RuntimeIndex{}, errors.New("index pair failed readback")
		}
	}
	return RuntimeIndex{metadata, generation, filepath.Join(dir, "metadata.json")}, nil
}
func SortRuntimePointers(p []RuntimePointer) {
	sort.Slice(p, func(i, j int) bool { return p[i].ID < p[j].ID })
}
