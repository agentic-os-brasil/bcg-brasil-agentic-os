package memory

import (
	"bytes"
	"context"
	"crypto/hmac"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatewiki"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

type privateWikiPointer struct {
	SchemaVersion  int    `json:"schema_version"`
	Generation     string `json:"generation"`
	ManifestDigest string `json:"manifest_digest"`
	Seal           string `json:"seal"`
}

func wikiPointerSeal(key []byte, p privateWikiPointer) string {
	p.Seal = ""
	b, _ := json.Marshal(p)
	return privateWikiMAC(key, "current-pointer", b)
}

func privateWikiSafeName(name string) bool {
	return name != "" && name != "." && name != ".." && name == path.Clean(name) && !path.IsAbs(name) && !strings.HasPrefix(name, "../") && !strings.ContainsAny(name, "\\\x00") && len(name) <= 192
}

func (engine *Engine) readPrivateWikiDirectory(directory string) (privatewiki.Candidate, error) {
	if scheduler.ValidatePrivateDirectory(directory) != nil {
		return privatewiki.Candidate{}, ErrPrivateWikiUnavailable
	}
	b, err := memoryReadFile(filepath.Join(directory, "manifest.json"), 1<<20)
	if err != nil {
		return privatewiki.Candidate{}, ErrPrivateWikiUnavailable
	}
	m, err := privatewiki.DecodeManifest(b)
	if err != nil {
		return privatewiki.Candidate{}, ErrPrivateWikiUnavailable
	}
	if len(m.Files)+1 > privatewiki.MaxFiles {
		return privatewiki.Candidate{}, privatewiki.ErrBudget
	}
	files := map[string][]byte{"manifest.json": b}
	directories := map[string]bool{"": true}
	total := len(b)
	for name := range m.Files {
		if !privateWikiSafeName(name) {
			return privatewiki.Candidate{}, ErrPrivateWikiUnavailable
		}
		body, err := memoryReadFile(filepath.Join(directory, filepath.FromSlash(name)), privatewiki.MaxOutputBytes)
		if err != nil {
			return privatewiki.Candidate{}, ErrPrivateWikiUnavailable
		}
		total += len(body)
		if total > privatewiki.MaxOutputBytes {
			return privatewiki.Candidate{}, privatewiki.ErrBudget
		}
		files[name] = body
		for parent := path.Dir(name); parent != "."; parent = path.Dir(parent) {
			directories[parent] = true
		}
	}
	var visit func(string, string) error
	entriesSeen := 0
	visit = func(dir, relative string) error {
		if scheduler.ValidatePrivateDirectory(dir) != nil {
			return ErrPrivateWikiUnavailable
		}
		entries, err := memoryReadDir(dir)
		if err != nil {
			return ErrPrivateWikiUnavailable
		}
		for _, entry := range entries {
			entriesSeen++
			if entriesSeen > privatewiki.MaxFiles*2 {
				return privatewiki.ErrBudget
			}
			name := path.Join(relative, entry.Name())
			if !privateWikiSafeName(name) {
				return ErrPrivateWikiUnavailable
			}
			if entry.IsDir() {
				if !directories[name] {
					return ErrPrivateWikiUnavailable
				}
				if err := visit(filepath.Join(dir, entry.Name()), name); err != nil {
					return err
				}
			} else if _, ok := files[name]; !ok {
				return ErrPrivateWikiUnavailable
			}
		}
		return nil
	}
	if err := visit(directory, ""); err != nil {
		return privatewiki.Candidate{}, err
	}
	c := privatewiki.Candidate{Manifest: m, Files: files}
	if privatewiki.Validate(c) != nil {
		return privatewiki.Candidate{}, ErrPrivateWikiUnavailable
	}
	return c, nil
}

func (engine *Engine) loadPrivateWiki(workspace string, snapshot privateWikiSnapshot) (privatewiki.Candidate, error) {
	barrier, err := engine.readVisibility(workspace)
	requirement, required, requirementErr := engine.readVisibilityRequirement(workspace)
	if err != nil || requirementErr != nil || !required || requirement.SchemaVersion != 2 || barrier.PrivateWiki == nil || barrier.PrivateWiki.Cohort != privateWikiCohort {
		return privatewiki.Candidate{}, ErrPrivateWikiCohort
	}
	if barrier.PrivateWiki.RetiredThrough != barrier.Generation {
		return privatewiki.Candidate{}, ErrPrivateWikiRetirement
	}
	root := engine.privateWikiRoot(workspace)
	b, err := memoryReadFile(filepath.Join(root, "current.json"), 4096)
	if errors.Is(err, os.ErrNotExist) {
		return privatewiki.Candidate{}, os.ErrNotExist
	}
	if err != nil {
		return privatewiki.Candidate{}, ErrPrivateWikiUnavailable
	}
	var p privateWikiPointer
	if decodeMemoryJSON(b, &p) != nil || p.SchemaVersion != 1 || !privatewiki.ValidID(p.Generation) || !privatewiki.ValidID(p.ManifestDigest) || !hmac.Equal([]byte(p.Seal), []byte(wikiPointerSeal(snapshot.key, p))) {
		return privatewiki.Candidate{}, ErrPrivateWikiUnavailable
	}
	c, err := engine.readPrivateWikiDirectory(filepath.Join(root, "versions", p.Generation))
	if err != nil {
		return privatewiki.Candidate{}, err
	}
	if c.Manifest.Generation != p.Generation || privatewiki.Digest(c.Files["manifest.json"]) != p.ManifestDigest || c.Manifest.HomeRef != snapshot.input.HomeRef || c.Manifest.WorkspaceRef != snapshot.input.WorkspaceRef {
		return privatewiki.Candidate{}, ErrPrivateWikiUnavailable
	}
	return c, nil
}

func (engine *Engine) publishPrivateWiki(ctx context.Context, workspace string, snapshot privateWikiSnapshot, c privatewiki.Candidate, expectedParent string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if privatewiki.Validate(c) != nil {
		return ErrPrivateWikiUnavailable
	}
	root := engine.privateWikiRoot(workspace)
	for _, dir := range []string{root, filepath.Join(root, ".staging"), filepath.Join(root, "versions")} {
		if err := memoryMkdirAll(dir, 0700); err != nil {
			return ErrPrivateWikiUnavailable
		}
		if scheduler.ValidatePrivateDirectory(dir) != nil {
			return ErrPrivateWikiUnavailable
		}
	}
	stage, err := memoryMkdirTemp(filepath.Join(root, ".staging"), "graph-")
	if err != nil {
		return ErrPrivateWikiUnavailable
	}
	defer memoryRemoveAll(stage)
	var names []string
	for name := range c.Files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !privateWikiSafeName(name) {
			return ErrPrivateWikiUnavailable
		}
		p := filepath.Join(stage, filepath.FromSlash(name))
		if memoryMkdirAll(filepath.Dir(p), 0700) != nil {
			return ErrPrivateWikiUnavailable
		}
		if memoryPublish(p, c.Files[name], true) != nil {
			return ErrPrivateWikiUnavailable
		}
	}
	staged, err := engine.readPrivateWikiDirectory(stage)
	if err != nil || !samePrivateWikiFiles(staged.Files, c.Files) {
		return ErrPrivateWikiUnavailable
	}
	if err = engine.fault("wiki_after_stage"); err != nil {
		return err
	}
	// Reserve a generation exclusively, then create only immutable files. The
	// pointer remains unchanged until the complete graph has been read back.
	version := filepath.Join(root, "versions", c.Manifest.Generation)
	intent := privateWikiPointer{SchemaVersion: 1, Generation: c.Manifest.Generation, ManifestDigest: privatewiki.Digest(c.Files["manifest.json"])}
	intent.Seal = wikiPointerSeal(snapshot.key, intent)
	intentBody, _ := json.Marshal(intent)
	intentPath := filepath.Join(root, ".staging", "intent-"+c.Manifest.Generation+".json")
	if err = memoryPublish(intentPath, intentBody, true); errors.Is(err, os.ErrExist) {
		old, e := memoryReadFile(intentPath, 4096)
		if e != nil || !bytes.Equal(old, intentBody) {
			return ErrPrivateWikiUnavailable
		}
	} else if err != nil {
		return ErrPrivateWikiUnavailable
	}
	if err = memoryMkdir(version, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return ErrPrivateWikiUnavailable
	}
	if scheduler.ValidatePrivateDirectory(version) != nil {
		return ErrPrivateWikiUnavailable
	}
	for _, name := range names {
		if err := ctx.Err(); err != nil {
			return err
		}
		p := filepath.Join(version, filepath.FromSlash(name))
		if memoryMkdirAll(filepath.Dir(p), 0700) != nil {
			return ErrPrivateWikiUnavailable
		}
		if err = memoryPublish(p, c.Files[name], true); errors.Is(err, os.ErrExist) {
			old, e := memoryReadFile(p, privatewiki.MaxOutputBytes)
			if e != nil || !bytes.Equal(old, c.Files[name]) {
				return ErrPrivateWikiUnavailable
			}
		} else if err != nil {
			return ErrPrivateWikiUnavailable
		}
		if err = engine.fault("wiki_after_version_file"); err != nil {
			return err
		}
	}
	actual, err := engine.readPrivateWikiDirectory(version)
	if err != nil || !samePrivateWikiFiles(actual.Files, c.Files) {
		return ErrPrivateWikiUnavailable
	}
	current, err := engine.privateWikiSnapshot(workspace)
	if err != nil || current.input.Watermark != snapshot.input.Watermark || current.input.Implementation != snapshot.input.Implementation || current.input.Configuration != snapshot.input.Configuration {
		return ErrPrivateWikiStale
	}
	prior, err := engine.loadPrivateWiki(workspace, snapshot)
	parent := ""
	if err == nil {
		parent = prior.Manifest.Generation
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if parent != expectedParent {
		return ErrPrivateWikiConflict
	}
	if err = engine.fault("wiki_before_pointer"); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if _, err = processidentity.Current(); err != nil {
		return err
	}
	return memoryPublish(filepath.Join(root, "current.json"), append(intentBody, '\n'), false)
}
