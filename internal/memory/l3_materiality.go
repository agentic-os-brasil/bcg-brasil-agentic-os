package memory

import (
	"errors"
	"path/filepath"
	"sort"
	"strings"
)

// The original SHA-bound ancestry walker still authenticates every transition.
// This additional lifetime-only veto counts distinct canonical L1 cohorts from
// their exact L3->L2 source edges. A weekly provider/format change may reprocess
// output under its real producer ID, without manufacturing new lifetime evidence.
func weeklySourceCohort(l3, l2 Artifact) (string, error) {
	// Original DreamWeekly constructs the L3 source before attaching the
	// lifetime decision metadata to L2. Reconstruct only that exact source view;
	// all content, producer, source refs, visibility and timestamps stay pinned.
	source := l2
	source.Promotion = nil
	document := artifactSource(source)
	matched := false
	for _, ref := range l3.Sources {
		if strings.HasPrefix(ref.ID, "L2/") {
			if matched || ref.ID != document.ID || ref.SHA256 != modelArtifactDigest(source) {
				return "", errors.New("L3 materiality has ambiguous or changed L2 source")
			}
			matched = true
		}
	}
	if !matched || l2.WorkspaceID != l3.WorkspaceID || l2.Layer != "L2" || len(l2.Sources) < 1 || len(l2.Sources) > 7 {
		return "", errors.New("L3 materiality lacks exact canonical L2 evidence")
	}
	refs := append([]SourceRef(nil), l2.Sources...)
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	for i, ref := range refs {
		if !strings.HasPrefix(ref.ID, "L1/") || len(ref.SHA256) != 64 || !isHex(ref.SHA256) || i > 0 && refs[i-1].ID == ref.ID {
			return "", errors.New("lifetime continuity requires a distinct eligible L1 cohort")
		}
	}
	return modelValueDigest(refs), nil
}

func (engine *Engine) l3MaterialGenerationCount(workspace string, next, l2 Artifact, committed bool) (int, error) {
	if err := engine.validateArtifact(next); err != nil {
		return 0, err
	}
	if err := engine.validateArtifact(l2); err != nil {
		return 0, err
	}
	cohort, err := weeklySourceCohort(next, l2)
	if err != nil {
		return 0, err
	}
	seenCohorts := map[string]bool{cohort: true}
	wanted, err := l3Predecessor(next)
	if err != nil {
		return 0, err
	}
	if wanted == nil {
		return 1, nil
	}
	manifest, name, err := engine.latestManifest(workspace)
	if err != nil {
		return 0, err
	}
	seenCommits := map[string]bool{}
	previousDigest := ""
	nextDigest := modelArtifactDigest(next)
	for checked := 0; checked < maximumL3EvidenceCommits; checked++ {
		if seenCommits[name] || manifest.WorkspaceID != workspace || manifest.VisibilityRevision != next.VisibilityRevision {
			return 0, errors.New("invalid lifetime materiality ancestry")
		}
		seenCommits[name] = true
		current, _, e := engine.readArtifactFromManifest(workspace, manifest, "L3/current")
		if e != nil {
			return 0, e
		}
		encoded := modelArtifactDigest(current)
		if committed && checked == 0 {
			if encoded != nextDigest {
				return 0, errors.New("materiality start is not current L3")
			}
			previousDigest = encoded
		}
		if encoded != previousDigest {
			if artifactSource(current).ID != wanted.ID || encoded != wanted.SHA256 {
				return 0, errors.New("materiality predecessor digest changed")
			}
			var previousL2 Artifact
			found := false
			for _, ref := range current.Sources {
				if strings.HasPrefix(ref.ID, "L2/") {
					if found {
						return 0, errors.New("ambiguous materiality L2 source")
					}
					previousL2, _, e = engine.readArtifactFromManifest(workspace, manifest, ref.ID)
					if e != nil {
						return 0, e
					}
					found = true
				}
			}
			if !found {
				return 0, errors.New("committed L3 lacks canonical L2 materiality")
			}
			if e = engine.validateArtifact(previousL2); e != nil {
				return 0, e
			}
			material, e := weeklySourceCohort(current, previousL2)
			if e != nil {
				return 0, e
			}
			seenCohorts[material] = true
			previousDigest = encoded
			wanted, e = l3Predecessor(current)
			if e != nil {
				return 0, e
			}
			if wanted == nil {
				return len(seenCohorts), nil
			}
		}
		parent := manifest.ParentCommit
		if parent == "" || filepath.Base(parent) != parent || strings.ContainsAny(parent, "/\\") || !strings.HasSuffix(parent, ".json") {
			return 0, errors.New("materiality predecessor lacks confined committed ancestry")
		}
		manifest, err = engine.readManifest(filepath.Join(engine.workspaceRoot(workspace), "commits", parent))
		if err != nil {
			return 0, err
		}
		if err = engine.validateManifest(workspace, manifest); err != nil {
			return 0, err
		}
		name = parent
	}
	return 0, errors.New("lifetime materiality exceeds original ancestry bound")
}
