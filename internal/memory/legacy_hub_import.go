package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	LegacyHubBridgeSynthesizerID = "legacy-hub-bridge-v1"
	legacyHubHistoryLimit        = 1024
	legacyHubSourceLimit         = 16
	legacyHubSelectedBytesLimit  = 64 << 10
)

var legacyHubLayers = map[string]bool{"recent": true, "weekly": true, "medium-term": true, "lifetime": true}

type LegacyHubSource struct {
	CandidateID string
	LegacyLayer string
	SHA256      string
	Content     []byte
}

type LegacyHubImportRequest struct {
	WorkspaceID string
	ImportedAt  time.Time
	Sources     []LegacyHubSource
}

type LegacyHubImportResult struct {
	State    string `json:"state"`
	ImportID string `json:"import_id"`
	Period   string `json:"period"`
}

func (engine *Engine) ImportLegacyHub(ctx context.Context, request LegacyHubImportRequest) (LegacyHubImportResult, error) {
	if err := engine.validate(); err != nil {
		return LegacyHubImportResult{}, err
	}
	if err := validateWorkspaceID(request.WorkspaceID); err != nil {
		return LegacyHubImportResult{}, err
	}
	if request.ImportedAt.IsZero() {
		request.ImportedAt = engine.Now()
	}
	sources, err := validateLegacyHubSources(request.Sources)
	if err != nil {
		return LegacyHubImportResult{}, err
	}
	if err := ctx.Err(); err != nil {
		return LegacyHubImportResult{}, err
	}
	importID := legacyHubImportID(request.WorkspaceID, sources)
	period := request.ImportedAt.UTC().Format("2006-01-02")
	result := LegacyHubImportResult{ImportID: importID, Period: period}

	release, err := engine.acquireCycleLock(request.WorkspaceID, "activation")
	if err != nil {
		return LegacyHubImportResult{}, err
	}
	defer release()
	if applied, err := engine.legacyHubImportCommitted(request.WorkspaceID, importID); err != nil {
		return LegacyHubImportResult{}, err
	} else if applied {
		result.State = "already_applied"
		return result, nil
	}
	if err := engine.publishLegacyHubSnapshots(request.WorkspaceID, importID, sources); err != nil {
		return LegacyHubImportResult{}, err
	}
	if err := engine.fault("after_publish_legacy_hub_snapshots"); err != nil {
		return LegacyHubImportResult{}, err
	}

	var previous *Artifact
	if active, readErr := engine.latestL1Artifact(request.WorkspaceID); readErr == nil {
		if err := engine.validateArtifact(active); err != nil {
			return LegacyHubImportResult{}, err
		}
		previous = &active
	} else if !errors.Is(readErr, os.ErrNotExist) {
		return LegacyHubImportResult{}, readErr
	}
	artifact := engine.legacyHubArtifact(request.WorkspaceID, period, importID, request.ImportedAt, previous, sources)
	if err := engine.validateArtifact(artifact); err != nil {
		return LegacyHubImportResult{}, err
	}
	if err := engine.activate(request.WorkspaceID, []Artifact{artifact}); err != nil {
		if applied, checkErr := engine.legacyHubImportCommitted(request.WorkspaceID, importID); checkErr == nil && applied {
			result.State = "already_applied"
			return result, nil
		}
		return LegacyHubImportResult{}, err
	}
	result.State = "applied"
	return result, nil
}

func (engine *Engine) latestL1Artifact(workspaceID string) (Artifact, error) {
	manifest, _, err := engine.latestManifest(workspaceID)
	if err != nil {
		return Artifact{}, err
	}
	key, err := latestArtifactKeyInManifest(manifest, "L1/")
	if err != nil {
		return Artifact{}, err
	}
	artifact, _, err := engine.readArtifactFromManifest(workspaceID, manifest, key)
	return artifact, err
}

func legacyHubImportedContinuation(artifact Artifact) (string, []SourceRef) {
	var refs []SourceRef
	for _, source := range artifact.Sources {
		if strings.HasPrefix(source.ID, "legacy-hub/") {
			refs = append(refs, source)
		}
	}
	if len(refs) == 0 {
		return "", nil
	}
	index := strings.Index(artifact.Content, "# Imported Hub memory · ")
	if index < 0 {
		return "", nil
	}
	return strings.TrimSpace(artifact.Content[index:]), refs
}

func appendUniqueSourceRefs(base, additions []SourceRef) []SourceRef {
	seen := make(map[string]bool, len(base)+len(additions))
	result := make([]SourceRef, 0, len(base)+len(additions))
	for _, source := range append(append([]SourceRef(nil), base...), additions...) {
		key := source.ID + "\x00" + source.SHA256
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, source)
	}
	return result
}

func validateLegacyHubSources(input []LegacyHubSource) ([]LegacyHubSource, error) {
	if len(input) == 0 || len(input) > legacyHubSourceLimit {
		return nil, fmt.Errorf("legacy Hub import requires between 1 and %d sources", legacyHubSourceLimit)
	}
	sources := append([]LegacyHubSource(nil), input...)
	sort.Slice(sources, func(i, j int) bool { return sources[i].CandidateID < sources[j].CandidateID })
	total := 0
	for index := range sources {
		source := &sources[index]
		if len(source.CandidateID) != 32 || !isHex(source.CandidateID) || strings.ToLower(source.CandidateID) != source.CandidateID {
			return nil, errors.New("legacy Hub source has an invalid candidate ID")
		}
		if index > 0 && sources[index-1].CandidateID == source.CandidateID {
			return nil, errors.New("legacy Hub import contains a duplicate candidate")
		}
		if !legacyHubLayers[source.LegacyLayer] {
			return nil, errors.New("legacy Hub source has an invalid layer")
		}
		if len(source.Content) == 0 || !utf8.Valid(source.Content) {
			return nil, errors.New("legacy Hub source content is empty or invalid UTF-8")
		}
		digest := sha256.Sum256(source.Content)
		if hex.EncodeToString(digest[:]) != source.SHA256 {
			return nil, errors.New("legacy Hub source digest does not match content")
		}
		total += len(source.Content)
		if total > legacyHubSelectedBytesLimit {
			return nil, errors.New("legacy Hub selected content exceeds its byte limit")
		}
	}
	return sources, nil
}

func legacyHubImportID(workspaceID string, sources []LegacyHubSource) string {
	hash := sha256.New()
	_, _ = hash.Write([]byte("legacy-hub-import-v1\x00" + workspaceID + "\x00"))
	for _, source := range sources {
		_, _ = hash.Write([]byte(source.CandidateID + "\x00" + source.LegacyLayer + "\x00" + source.SHA256 + "\x00"))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func (engine *Engine) publishLegacyHubSnapshots(workspaceID, importID string, sources []LegacyHubSource) error {
	root := filepath.Join(engine.workspaceRoot(workspaceID), "imports", "legacy-hub")
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	destination := filepath.Join(root, importID)
	if info, err := os.Lstat(destination); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("legacy Hub snapshot authority is invalid")
		}
		return validateLegacyHubSnapshotDirectory(destination, sources)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	staging, err := os.MkdirTemp(root, ".pending-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	for _, source := range sources {
		path := filepath.Join(staging, source.CandidateID+".md")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		if _, err = file.Write(source.Content); err == nil {
			err = file.Sync()
		}
		closeErr := file.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return durableRename(staging, destination)
}

func validateLegacyHubSnapshotDirectory(root string, sources []LegacyHubSource) error {
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != len(sources) {
		return errors.New("legacy Hub snapshot is incomplete")
	}
	for _, source := range sources {
		path := filepath.Join(root, source.CandidateID+".md")
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return errors.New("legacy Hub snapshot is invalid")
		}
		body, err := os.ReadFile(path)
		if err != nil || string(body) != string(source.Content) {
			return errors.New("legacy Hub snapshot content conflicts with immutable source")
		}
	}
	return nil
}

func (engine *Engine) legacyHubArtifact(workspaceID, period, importID string, importedAt time.Time, previous *Artifact, sources []LegacyHubSource) Artifact {
	parts := []string{"# L1 continuity · " + period}
	refs := make([]SourceRef, 0, len(sources)+4)
	if previous != nil {
		parts = []string{previous.Content}
		refs = append(refs, previous.Sources...)
	}
	parts = append(parts, "# Imported Hub memory · "+importID)
	for _, source := range sources {
		parts = append(parts, "## "+source.LegacyLayer+" · "+source.CandidateID, strings.TrimSpace(string(source.Content)))
		refs = append(refs, SourceRef{ID: "legacy-hub/" + source.LegacyLayer + "/" + source.CandidateID, SHA256: source.SHA256})
	}
	fingerprint := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return Artifact{
		SchemaVersion: 1, WorkspaceID: workspaceID, Layer: "L1", Period: period, GeneratedAt: importedAt.UTC(),
		SourceFingerprint: hex.EncodeToString(fingerprint[:]), Sources: refs,
		SynthesizerID: LegacyHubBridgeSynthesizerID + "/" + importID, Content: strings.Join(parts, "\n\n"),
	}
}

func (engine *Engine) legacyHubImportCommitted(workspaceID, importID string) (bool, error) {
	commitsRoot := filepath.Join(engine.workspaceRoot(workspaceID), "commits")
	entries, err := os.ReadDir(commitsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") && strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}
	if len(names) > legacyHubHistoryLimit {
		return false, errors.New("legacy Hub import history exceeds its validation limit")
	}
	want := LegacyHubBridgeSynthesizerID + "/" + importID
	for _, name := range names {
		manifest, readErr := engine.readManifest(filepath.Join(commitsRoot, name))
		if readErr != nil || engine.validateManifest(workspaceID, manifest) != nil {
			continue
		}
		for key := range manifest.Artifacts {
			if !strings.HasPrefix(key, "L1/") {
				continue
			}
			artifact, _, readErr := engine.readArtifactFromManifest(workspaceID, manifest, key)
			if readErr == nil && artifact.SynthesizerID == want {
				return true, nil
			}
		}
	}
	return false, nil
}
