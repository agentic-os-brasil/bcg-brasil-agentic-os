package memory

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/privatewiki"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/processidentity"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

var ErrPrivateWikiUnavailable = errors.New("private_wiki_unavailable")
var ErrPrivateWikiStale = errors.New("private_wiki_stale")
var ErrPrivateWikiConflict = errors.New("private_wiki_parent_or_source_conflict")
var ErrPrivateWikiRetirement = errors.New("private_wiki_retirement_incomplete")

type PrivateWikiResult struct {
	Generation      string `json:"generation"`
	SourceWatermark string `json:"source_watermark"`
	Changed         bool   `json:"changed"`
	Recovered       bool   `json:"recovered"`
}

type PrivateWikiPage struct {
	Generation string `json:"generation"`
	Content    string `json:"content"`
	NextOffset int    `json:"next_offset"`
	Complete   bool   `json:"complete"`
}

type PrivateWikiStatus struct {
	ImplementationCurrent bool   `json:"implementation_current"`
	ConfigurationCurrent  bool   `json:"configuration_current"`
	State                 string `json:"state"`
	Generation            string `json:"generation,omitempty"`
	SourceWatermark       string `json:"source_watermark,omitempty"`
	Concepts              int    `json:"concepts"`
	Files                 int    `json:"files"`
	DenialGeneration      uint64 `json:"denial_generation"`
}

type privateWikiSnapshot struct {
	input      privatewiki.Input
	key        []byte
	commitName string
}

func privateWikiMAC(key []byte, domain string, bodies ...[]byte) string {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte("maestro-private-memory-wiki-v1\x00" + domain + "\x00"))
	for _, b := range bodies {
		_, _ = h.Write(b)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func (engine *Engine) privateWikiRoot(workspace string) string {
	return filepath.Join(engine.workspaceRoot(workspace), ".private-wiki")
}

func (engine *Engine) privateWikiScope(workspace string) ([]byte, string, string, error) {
	if engine.ownedPreflight(workspace) != nil {
		return nil, "", "", ErrPrivateWikiUnavailable
	}
	root, err := scheduler.CanonicalPrivatePath(engine.Root)
	if err != nil || filepath.Base(root) != "memory" || filepath.Base(filepath.Dir(root)) != "brain" {
		return nil, "", "", ErrPrivateWikiUnavailable
	}
	home := filepath.Dir(filepath.Dir(root))
	if scheduler.ValidatePrivateDirectory(home) != nil {
		return nil, "", "", ErrPrivateWikiUnavailable
	}
	key, err := (CaptureAttestor{Root: engine.Root}).ownedKey(workspace, false)
	if err != nil {
		return nil, "", "", ErrPrivateWikiUnavailable
	}
	homeRef := privateWikiMAC(key, "local-home", []byte(home))
	workspaceRef := privateWikiMAC(key, "registered-workspace", []byte(homeRef), []byte(workspace))
	return key, homeRef, workspaceRef, nil
}

func privateWikiImplementation() (string, error) {
	identity, err := processidentity.Current()
	if err != nil {
		return "", errors.Join(ErrPrivateWikiUnavailable, err)
	}
	return identity.SHA256, nil
}

func (engine *Engine) privateWikiSnapshot(workspace string) (privateWikiSnapshot, error) {
	key, homeRef, workspaceRef, err := engine.privateWikiScope(workspace)
	if err != nil {
		return privateWikiSnapshot{}, err
	}
	before, err := engine.readVisibility(workspace)
	if err != nil {
		return privateWikiSnapshot{}, ErrPrivateWikiUnavailable
	}
	manifest, name, err := engine.latestManifest(workspace)
	if err != nil || name == "" || len(manifest.Artifacts) == 0 {
		return privateWikiSnapshot{}, ErrPrivateWikiUnavailable
	}
	if len(manifest.Artifacts) > privatewiki.MaxSources {
		return privateWikiSnapshot{}, privatewiki.ErrBudget
	}
	policy, _ := json.Marshal(engine.Policy)
	policyRef := privateWikiMAC(key, "source-policy", policy)
	manifestBody, _ := json.Marshal(manifest)
	commitRef := privateWikiMAC(key, "memory-commit", []byte(name), manifestBody)
	implementation, err := privateWikiImplementation()
	if err != nil {
		return privateWikiSnapshot{}, err
	}
	configuration, _ := json.Marshal([]any{privatewiki.Profile, privatewiki.CompilerVersion, privatewiki.MaxSources, privatewiki.MaxSourceBytes, privatewiki.MaxInputBytes, privatewiki.MaxFiles, privatewiki.MaxOutputBytes, engine.Policy, engine.Budgets, engine.Freshness})
	in := privatewiki.Input{HomeRef: homeRef, WorkspaceRef: workspaceRef, CommitRef: commitRef, DenialGeneration: before.Generation, PolicyRef: policyRef, Implementation: implementation, Configuration: privateWikiMAC(key, "configuration", configuration), CommittedAt: manifest.CommittedAt}
	var keys []string
	for k := range manifest.Artifacts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	total := 0
	var versions []string
	for _, k := range keys {
		artifact, _, body, err := engine.readArtifactFromManifestPayload(workspace, manifest, k)
		if err != nil || engine.validateArtifact(artifact) != nil {
			return privateWikiSnapshot{}, ErrPrivateWikiUnavailable
		}
		total += len(body)
		if len(body) > privatewiki.MaxSourceBytes || total > privatewiki.MaxInputBytes {
			return privateWikiSnapshot{}, privatewiki.ErrBudget
		}
		id := privateWikiMAC(key, "stable-concept", []byte(homeRef), []byte(workspaceRef), []byte(k))
		version := privateWikiMAC(key, "rollup-version", []byte(manifest.Artifacts[k]), body)
		reference := privateWikiMAC(key, "source-reference", []byte(id), []byte(version))
		in.Sources = append(in.Sources, privatewiki.Source{ID: id, Version: version, Reference: reference, Layer: artifact.Layer, Period: artifact.Period, Content: artifact.Content})
		versions = append(versions, id+":"+version)
	}
	identity, _ := json.Marshal([]any{homeRef, workspaceRef, commitRef, before.Generation, policyRef, versions})
	in.Watermark = privateWikiMAC(key, "committed-source-watermark", identity)
	after, err := engine.readVisibility(workspace)
	if err != nil || after.Generation != before.Generation {
		return privateWikiSnapshot{}, ErrPrivateWikiStale
	}
	_, afterName, err := engine.latestManifest(workspace)
	if err != nil || afterName != name {
		return privateWikiSnapshot{}, ErrPrivateWikiStale
	}
	return privateWikiSnapshot{in, key, name}, nil
}

func (engine *Engine) privateWikiGuard(ctx context.Context, publish func() error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	fn := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := processidentity.Current(); err != nil {
			return err
		}
		return publish()
	}
	if engine.PublicationGuard != nil {
		return engine.PublicationGuard(fn)
	}
	return fn()
}

// CompilePrivateWiki owns the source fence and immutable graph transaction.
// expectedParent is empty only for a genuinely absent current pointer.
func (engine *Engine) CompilePrivateWiki(ctx context.Context, workspace, expectedParent string) (PrivateWikiResult, error) {
	if err := ctx.Err(); err != nil {
		return PrivateWikiResult{}, err
	}
	if _, err := processidentity.Current(); err != nil {
		return PrivateWikiResult{}, err
	}
	if _, _, _, err := engine.privateWikiScope(workspace); err != nil {
		return PrivateWikiResult{}, err
	}
	prepareRelease, err := engine.acquireCycleLock(workspace, "activation")
	if err != nil {
		return PrivateWikiResult{}, err
	}
	err = engine.privateWikiGuard(ctx, func() error {
		if _, err := engine.preparePrivateWikiCohort(workspace); err != nil {
			return err
		}
		return engine.retirePrivateWiki(workspace)
	})
	prepareRelease()
	if err != nil {
		return PrivateWikiResult{}, err
	}
	snapshot, err := engine.privateWikiSnapshot(workspace)
	if err != nil {
		return PrivateWikiResult{}, err
	}
	if err = engine.fault("wiki_after_snapshot"); err != nil {
		return PrivateWikiResult{}, err
	}
	candidate, err := privatewiki.Compile(snapshot.input)
	if err != nil {
		return PrivateWikiResult{}, err
	}
	release, err := engine.acquireCycleLock(workspace, "activation")
	if err != nil {
		return PrivateWikiResult{}, err
	}
	defer release()
	result := PrivateWikiResult{}
	err = engine.privateWikiGuard(ctx, func() error {
		current, err := engine.privateWikiSnapshot(workspace)
		if err != nil {
			return err
		}
		if current.input.Watermark != snapshot.input.Watermark || current.input.Implementation != snapshot.input.Implementation || current.input.Configuration != snapshot.input.Configuration {
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
		recovered, err := engine.recoverPrivateWikiEvent(workspace, snapshot)
		if err != nil {
			return err
		}
		if parent == candidate.Manifest.Generation {
			if !samePrivateWikiFiles(prior.Files, candidate.Files) {
				return ErrPrivateWikiUnavailable
			}
			result = PrivateWikiResult{Generation: parent, SourceWatermark: snapshot.input.Watermark, Recovered: recovered}
			return nil
		}
		if err = engine.publishPrivateWiki(ctx, workspace, snapshot, candidate, expectedParent); err != nil {
			return err
		}
		readback, err := engine.loadPrivateWiki(workspace, snapshot)
		if err != nil || !samePrivateWikiFiles(readback.Files, candidate.Files) {
			return ErrPrivateWikiUnavailable
		}
		result = PrivateWikiResult{Generation: candidate.Manifest.Generation, SourceWatermark: snapshot.input.Watermark, Changed: true, Recovered: recovered}
		return nil
	})
	if err == nil {
		err = ctx.Err()
	}
	return result, err
}

func samePrivateWikiFiles(a, b map[string][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for n, body := range a {
		if !bytes.Equal(body, b[n]) {
			return false
		}
	}
	return true
}

// Source authority is independent of which compatible executable reads a
// producer's sealed graph. Stored provenance remains evidence, never a grant.
func privateWikiSourcesMatch(m privatewiki.Manifest, in privatewiki.Input) bool {
	if m.SourceWatermark != in.Watermark || m.CommitRef != in.CommitRef || m.DenialGeneration != in.DenialGeneration || m.HomeRef != in.HomeRef || m.WorkspaceRef != in.WorkspaceRef || m.PolicyRef != in.PolicyRef || len(m.Concepts) != len(in.Sources) {
		return false
	}
	for _, source := range in.Sources {
		concept, ok := m.Concepts["concepts/"+source.ID+".md"]
		if !ok || concept.Reference != source.Reference || concept.Version != source.Version || concept.Layer != source.Layer || concept.Period != source.Period {
			return false
		}
	}
	return true
}

func (engine *Engine) currentPrivateWiki(workspace string) (privateWikiSnapshot, privatewiki.Candidate, error) {
	snapshot, err := engine.privateWikiSnapshot(workspace)
	if err != nil {
		return privateWikiSnapshot{}, privatewiki.Candidate{}, err
	}
	c, err := engine.loadPrivateWiki(workspace, snapshot)
	if err != nil {
		return privateWikiSnapshot{}, privatewiki.Candidate{}, err
	}
	if !privateWikiSourcesMatch(c.Manifest, snapshot.input) {
		return privateWikiSnapshot{}, privatewiki.Candidate{}, ErrPrivateWikiStale
	}
	return snapshot, c, nil
}

func (engine *Engine) PrivateWikiStatus(workspace string) (PrivateWikiStatus, error) {
	snapshot, err := engine.privateWikiSnapshot(workspace)
	if err != nil {
		return PrivateWikiStatus{State: "unavailable"}, err
	}
	status := PrivateWikiStatus{State: "absent", SourceWatermark: snapshot.input.Watermark, DenialGeneration: snapshot.input.DenialGeneration}
	c, err := engine.loadPrivateWiki(workspace, snapshot)
	if errors.Is(err, os.ErrNotExist) {
		return status, nil
	}
	if err != nil {
		return PrivateWikiStatus{State: "unavailable"}, err
	}
	status.Generation = c.Manifest.Generation
	if !privateWikiSourcesMatch(c.Manifest, snapshot.input) {
		status.State = "stale"
		return status, nil
	}
	after, err := engine.privateWikiSnapshot(workspace)
	if err != nil || after.input.Watermark != snapshot.input.Watermark {
		return PrivateWikiStatus{State: "unavailable"}, ErrPrivateWikiStale
	}
	status.State = "current"
	status.ImplementationCurrent = c.Manifest.Implementation == after.input.Implementation
	status.ConfigurationCurrent = c.Manifest.Configuration == after.input.Configuration
	status.Concepts = len(c.Manifest.Concepts)
	status.Files = len(c.Files)
	return status, nil
}

func privateWikiPage(generation string, body []byte, offset, limit int) (PrivateWikiPage, error) {
	if offset < 0 || offset > len(body) || limit < 1 || limit > 65536 || offset < len(body) && !utf8.RuneStart(body[offset]) {
		return PrivateWikiPage{}, ErrPrivateWikiUnavailable
	}
	end := offset + limit
	if end > len(body) {
		end = len(body)
	}
	for end < len(body) && !utf8.RuneStart(body[end]) {
		end--
	}
	if end == offset && end < len(body) {
		return PrivateWikiPage{}, ErrPrivateWikiUnavailable
	}
	next := end
	if end == len(body) {
		next = -1
	}
	return PrivateWikiPage{Generation: generation, Content: string(body[offset:end]), NextOffset: next, Complete: next == -1}, nil
}

// ReadPrivateWiki returns only a file in the current authenticated generation.
func (engine *Engine) ReadPrivateWiki(workspace, generation, reference string, offset, limit int) (PrivateWikiPage, error) {
	if generation == "" && offset != 0 {
		return PrivateWikiPage{}, ErrPrivateWikiUnavailable
	}
	snapshot, c, err := engine.currentPrivateWiki(workspace)
	if err != nil {
		return PrivateWikiPage{}, err
	}
	if generation != "" && generation != c.Manifest.Generation {
		return PrivateWikiPage{}, ErrPrivateWikiStale
	}
	if reference == "manifest.json" {
		return PrivateWikiPage{}, ErrPrivateWikiUnavailable
	}
	body, ok := c.Files[reference]
	if !ok {
		return PrivateWikiPage{}, ErrPrivateWikiUnavailable
	}
	page, err := privateWikiPage(c.Manifest.Generation, body, offset, limit)
	if err != nil {
		return PrivateWikiPage{}, err
	}
	if err = engine.fault("wiki_after_read"); err != nil {
		return PrivateWikiPage{}, err
	}
	after, err := engine.privateWikiSnapshot(workspace)
	if err != nil || after.input.Watermark != snapshot.input.Watermark {
		return PrivateWikiPage{}, ErrPrivateWikiStale
	}
	return page, nil
}

func (engine *Engine) ReadPrivateWikiSource(workspace, generation, reference string, offset, limit int) (PrivateWikiPage, error) {
	snapshot, c, err := engine.currentPrivateWiki(workspace)
	if err != nil {
		return PrivateWikiPage{}, err
	}
	if generation != c.Manifest.Generation {
		return PrivateWikiPage{}, ErrPrivateWikiStale
	}
	reference = strings.TrimPrefix(reference, "maestro://memory/")
	var body []byte
	for _, s := range snapshot.input.Sources {
		if s.Reference == reference {
			body = []byte(s.Content)
			break
		}
	}
	if body == nil {
		return PrivateWikiPage{}, ErrPrivateWikiUnavailable
	}
	page, err := privateWikiPage(generation, body, offset, limit)
	if err != nil {
		return PrivateWikiPage{}, err
	}
	after, err := engine.privateWikiSnapshot(workspace)
	if err != nil || after.input.Watermark != snapshot.input.Watermark {
		return PrivateWikiPage{}, ErrPrivateWikiStale
	}
	return page, nil
}
