package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
)

const maximumL3EvidenceCommits = 4096

func l3Predecessor(artifact Artifact) (*SourceRef, error) {
	var predecessor *SourceRef
	for _, source := range artifact.Sources {
		if strings.HasPrefix(source.ID, "L3/") {
			if predecessor != nil {
				return nil, errors.New("L3 continuity has ambiguous predecessor evidence")
			}
			copy := source
			predecessor = &copy
		}
	}
	return predecessor, nil
}

// Only the next staged L3 and its SHA-bound, committed ancestry count. Text,
// orphaned versions and commits from an earlier visibility generation do not.
func (engine *Engine) l3GenerationCount(workspaceID string, next Artifact) (int, error) {
	if next.WorkspaceID != workspaceID || next.Layer != "L3" {
		return 0, errors.New("L3 evidence is outside selected workspace/layer")
	}
	if err := engine.validateArtifact(next); err != nil {
		return 0, err
	}
	barrier, err := engine.readVisibility(workspaceID)
	if err != nil {
		return 0, err
	}
	if next.VisibilityRevision != barrier.Generation {
		return 0, ErrCaptureConflict
	}
	wanted, err := l3Predecessor(next)
	if err != nil {
		return 0, err
	}
	if wanted == nil {
		return 1, nil
	}
	manifest, name, err := engine.latestManifest(workspaceID)
	if err != nil {
		return 0, err
	}
	generations := 1
	previousDigest := ""
	seen := map[string]bool{}
	for checked := 0; checked < maximumL3EvidenceCommits; checked++ {
		if seen[name] {
			return 0, errors.New("L3 evidence commit ancestry cycles")
		}
		seen[name] = true
		if manifest.WorkspaceID != workspaceID || manifest.VisibilityRevision != next.VisibilityRevision {
			return 0, errors.New("L3 evidence crosses workspace/visibility generation")
		}
		current, _, err := engine.readArtifactFromManifest(workspaceID, manifest, "L3/current")
		if err != nil {
			return 0, err
		}
		document := artifactSource(current)
		digest := sha256.Sum256(document.Content)
		encoded := hex.EncodeToString(digest[:])
		if encoded != previousDigest {
			if document.ID != wanted.ID || encoded != wanted.SHA256 {
				return 0, errors.New("L3 predecessor does not match committed source digest")
			}
			generations++
			previousDigest = encoded
			wanted, err = l3Predecessor(current)
			if err != nil {
				return 0, err
			}
			if wanted == nil {
				return generations, nil
			}
		}
		parent := manifest.ParentCommit
		if parent == "" {
			return 0, errors.New("L3 predecessor has no committed ancestry")
		}
		if filepath.Base(parent) != parent || strings.ContainsAny(parent, "/\\") || !strings.HasSuffix(parent, ".json") {
			return 0, errors.New("L3 evidence parent escapes memory commits")
		}
		manifest, err = engine.readManifest(filepath.Join(engine.workspaceRoot(workspaceID), "commits", parent))
		if err != nil {
			return 0, err
		}
		if err := engine.validateManifest(workspaceID, manifest); err != nil {
			return 0, err
		}
		name = parent
	}
	return 0, errors.New("L3 evidence exceeds bounded commit ancestry")
}

func validateL3CandidateEvidence(artifact Artifact) (int, error) {
	if artifact.l3Generations == 0 {
		return 0, nil
	}
	if artifact.l3Generations < 0 || artifact.l3Generations > maximumL3EvidenceCommits {
		return 0, errors.New("invalid L3 generation evidence count")
	}
	body := artifactSource(artifact).Content
	if artifact.l3EvidenceDigest != sha256.Sum256(body) {
		return 0, errors.New("L3 generation evidence does not match lifetime candidate")
	}
	return artifact.l3Generations, nil
}
