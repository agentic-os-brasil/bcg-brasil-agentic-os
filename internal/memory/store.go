package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type CommitManifest struct {
	SchemaVersion int               `json:"schema_version"`
	WorkspaceID   string            `json:"workspace_id"`
	TransactionID string            `json:"transaction_id"`
	ParentCommit  string            `json:"parent_commit,omitempty"`
	CommittedAt   time.Time         `json:"committed_at"`
	Artifacts     map[string]string `json:"artifacts"`
}

var ErrNoValidCommit = errors.New("memory commits exist but none is valid")

func artifactKey(artifact Artifact) string {
	switch artifact.Layer {
	case "L1", "L2":
		return artifact.Layer + "/" + artifact.Period
	case "L3":
		return "L3/current"
	case "lifetime":
		return "lifetime/current"
	default:
		return artifact.Layer + "/" + artifact.Period
	}
}

func (engine *Engine) activate(workspaceID string, artifacts []Artifact) error {
	if len(artifacts) == 0 {
		return errors.New("activation requires at least one artifact")
	}
	if err := validateWorkspaceID(workspaceID); err != nil {
		return err
	}
	seen := make(map[string]bool)
	for _, artifact := range artifacts {
		if artifact.WorkspaceID != workspaceID {
			return errors.New("artifact workspace does not match activation workspace")
		}
		if err := engine.validateArtifact(artifact); err != nil {
			return fmt.Errorf("validate %s artifact: %w", artifact.Layer, err)
		}
		key := artifactKey(artifact)
		if seen[key] {
			return fmt.Errorf("duplicate artifact key %s", key)
		}
		seen[key] = true
	}

	workspaceRoot := engine.workspaceRoot(workspaceID)
	stagingRoot := filepath.Join(workspaceRoot, ".transactions")
	if err := os.MkdirAll(stagingRoot, 0o700); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(stagingRoot, "dream-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	transactionID := filepath.Base(staging)

	artifactPaths := make(map[string]string, len(artifacts))
	for index, artifact := range artifacts {
		name := fmt.Sprintf("%02d-%s-%s.json", index, strings.ToLower(artifact.Layer), safeName(artifact.Period))
		path := filepath.Join(staging, name)
		if err := writeJSONFile(path, artifact); err != nil {
			return err
		}
		artifactPaths[artifactKey(artifact)] = name
		if err := engine.fault("after_stage_artifact:" + artifact.Layer); err != nil {
			return err
		}
	}

	versionsRoot := filepath.Join(workspaceRoot, "versions")
	if err := os.MkdirAll(versionsRoot, 0o700); err != nil {
		return err
	}
	versionDir := filepath.Join(versionsRoot, transactionID)
	if err := durableRename(staging, versionDir); err != nil {
		return err
	}
	if err := engine.fault("after_publish_version"); err != nil {
		return err
	}

	parent, parentName, err := engine.latestManifest(workspaceID)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	entries := make(map[string]string)
	if err == nil {
		for key, path := range parent.Artifacts {
			entries[key] = path
		}
	}
	for key, name := range artifactPaths {
		entries[key] = filepath.ToSlash(filepath.Join("versions", transactionID, name))
	}
	committedAt := engine.Now().UTC()
	if err == nil && !committedAt.After(parent.CommittedAt) {
		committedAt = parent.CommittedAt.Add(time.Nanosecond)
	}
	manifest := CommitManifest{SchemaVersion: 1, WorkspaceID: workspaceID, TransactionID: transactionID, ParentCommit: parentName, CommittedAt: committedAt, Artifacts: entries}
	if err := engine.validateManifest(workspaceID, manifest); err != nil {
		return err
	}

	commitsRoot := filepath.Join(workspaceRoot, "commits")
	if err := os.MkdirAll(commitsRoot, 0o700); err != nil {
		return err
	}
	pending := filepath.Join(commitsRoot, ".pending-"+transactionID+".json")
	if err := writeJSONFile(pending, manifest); err != nil {
		return err
	}
	if err := engine.fault("after_stage_manifest"); err != nil {
		return err
	}
	commitName := committedAt.Format("20060102T150405.000000000Z") + "-" + transactionID + ".json"
	if err := durableRename(pending, filepath.Join(commitsRoot, commitName)); err != nil {
		return err
	}
	if err := engine.fault("after_commit"); err != nil {
		return err
	}
	return nil
}

func (engine *Engine) fault(point string) error {
	if engine.FaultPoint == nil {
		return nil
	}
	return engine.FaultPoint(point)
}

func safeName(value string) string {
	replacer := strings.NewReplacer("/", "_", "\\", "_", ":", "_")
	return replacer.Replace(value)
}

func (engine *Engine) latestManifest(workspaceID string) (CommitManifest, string, error) {
	commitsRoot := filepath.Join(engine.workspaceRoot(workspaceID), "commits")
	entries, err := os.ReadDir(commitsRoot)
	if err != nil {
		return CommitManifest{}, "", err
	}
	var names []string
	for _, entry := range entries {
		if !entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") && strings.HasSuffix(entry.Name(), ".json") {
			names = append(names, entry.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(names)))
	for _, name := range names {
		manifest, decodeErr := engine.readManifest(filepath.Join(commitsRoot, name))
		if decodeErr == nil && engine.validateManifest(workspaceID, manifest) == nil {
			return manifest, name, nil
		}
	}
	if len(names) > 0 {
		return CommitManifest{}, "", ErrNoValidCommit
	}
	return CommitManifest{}, "", os.ErrNotExist
}

func (engine *Engine) readManifest(path string) (CommitManifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return CommitManifest{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var manifest CommitManifest
	if err := decoder.Decode(&manifest); err != nil {
		return CommitManifest{}, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return CommitManifest{}, err
	}
	return manifest, nil
}

func (engine *Engine) validateManifest(workspaceID string, manifest CommitManifest) error {
	if manifest.SchemaVersion != 1 || manifest.WorkspaceID != workspaceID || manifest.TransactionID == "" || manifest.CommittedAt.IsZero() || len(manifest.Artifacts) == 0 {
		return errors.New("invalid memory commit manifest")
	}
	workspaceRoot := engine.workspaceRoot(workspaceID)
	for key, relative := range manifest.Artifacts {
		clean := filepath.Clean(filepath.FromSlash(relative))
		if clean == "." || filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
			return fmt.Errorf("manifest artifact %s escapes workspace root", key)
		}
		path := filepath.Join(workspaceRoot, clean)
		artifact, err := engine.readArtifact(path)
		if err != nil || validateArtifactStructure(artifact) != nil || artifact.WorkspaceID != workspaceID || artifactKey(artifact) != key {
			return fmt.Errorf("manifest artifact %s is missing or invalid", key)
		}
	}
	return nil
}

func (engine *Engine) readArtifactByKey(workspaceID, key string) (Artifact, string, error) {
	manifest, _, err := engine.latestManifest(workspaceID)
	if err != nil {
		return Artifact{}, "", err
	}
	return engine.readArtifactFromManifest(workspaceID, manifest, key)
}

func (engine *Engine) readArtifactFromManifest(workspaceID string, manifest CommitManifest, key string) (Artifact, string, error) {
	relative, ok := manifest.Artifacts[key]
	if !ok {
		return Artifact{}, "", os.ErrNotExist
	}
	path := filepath.Join(engine.workspaceRoot(workspaceID), filepath.FromSlash(relative))
	artifact, err := engine.readArtifact(path)
	return artifact, path, err
}

func latestArtifactKeyInManifest(manifest CommitManifest, prefix string) (string, error) {
	var keys []string
	for key := range manifest.Artifacts {
		if strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return "", os.ErrNotExist
	}
	sort.Strings(keys)
	return keys[len(keys)-1], nil
}
