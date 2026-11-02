package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// RetentionState contains metadata only and never performs source deletion.
type RetentionState struct {
	WorkspaceID        string `json:"workspace_id"`
	VisibilityRevision uint64 `json:"visibility_revision"`
	DenialGeneration   uint64 `json:"denial_generation"`
	LiveCaptures       int    `json:"live_captures"`
	DeniedCaptures     int    `json:"denied_captures"`
	ActiveArtifacts    int    `json:"active_artifacts"`
	SourcesSHA256      string `json:"sources_sha256"`
	PreserveSources    bool   `json:"preserve_sources"`
}

func (engine *Engine) RetentionSnapshot(workspace string) (RetentionState, error) {
	if err := engine.ownedPreflight(workspace); err != nil {
		return RetentionState{}, err
	}
	before, err := engine.readVisibility(workspace)
	if err != nil {
		return RetentionState{}, err
	}
	captures, err := engine.allCaptures(workspace)
	if err != nil {
		return RetentionState{}, err
	}
	if len(captures) > maximumVisibilityIdentities {
		return RetentionState{}, ErrCaptureCapacity
	}
	manifest, _, err := engine.latestManifest(workspace)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return RetentionState{}, err
	}
	after, err := engine.readVisibility(workspace)
	if err != nil {
		return RetentionState{}, err
	}
	if before.Revision != after.Revision {
		return RetentionState{}, ErrCaptureConflict
	}
	sort.Slice(captures, func(i, j int) bool { return captures[i].ID < captures[j].ID })
	body, err := json.Marshal([]any{before, captures, manifest})
	if err != nil {
		return RetentionState{}, err
	}
	hash := sha256.Sum256(body)
	return RetentionState{workspace, before.Revision, before.Generation, len(captures), len(before.Denied), len(manifest.Artifacts), hex.EncodeToString(hash[:]), true}, nil
}

// WithRetentionSnapshot keeps the canonical activation/visibility lock outside
// the caller's publication guard. No control transition takes this lock.
func (engine *Engine) WithRetentionSnapshot(workspace string, fn func(RetentionState) error) error {
	release, err := engine.acquireCycleLock(workspace, "activation")
	if err != nil {
		return err
	}
	defer release()
	snapshot, err := engine.RetentionSnapshot(workspace)
	if err != nil {
		return err
	}
	return fn(snapshot)
}
func (engine *Engine) WeeklyInputIdentity(workspace string, at time.Time) (string, error) {
	if err := engine.validateRuntime(workspace, true); err != nil {
		return "", err
	}
	before, err := engine.readVisibility(workspace)
	if err != nil {
		return "", err
	}
	monday := startOfISOWeek(at)
	var sources []SourceDocument
	for i := 0; i < 7; i++ {
		artifact, _, err := engine.readArtifactByKey(workspace, "L1/"+monday.AddDate(0, 0, i).Format("2006-01-02"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		sources = append(sources, artifactSource(artifact))
	}
	for _, key := range []string{"L3/current", "lifetime/current"} {
		artifact, _, err := engine.readArtifactByKey(workspace, key)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		sources = append(sources, artifactSource(artifact))
	}
	after, err := engine.readVisibility(workspace)
	if err != nil {
		return "", err
	}
	if before.Revision != after.Revision {
		return "", ErrCaptureConflict
	}
	return engine.sourceFingerprint(fmt.Sprintf("weekly-review|%s|%s|%d", monday.Format("2006-01-02"), engine.EligibilityPolicyID, before.Revision), sources), nil
}

// WeeklyCommitEvidence validates both output layers through the canonical
// manifest. name is a confined immutable commit filename, or empty for current.
func (engine *Engine) WeeklyCommitEvidence(workspace, period, name string) (string, error) {
	var manifest CommitManifest
	var err error
	if name == "" {
		manifest, name, err = engine.latestManifest(workspace)
	} else {
		if filepath.Base(name) != name || filepath.Ext(name) != ".json" {
			return "", errors.New("invalid commit reference")
		}
		manifest, err = engine.readManifest(filepath.Join(engine.workspaceRoot(workspace), "commits", name))
	}
	if err != nil {
		return "", err
	}
	if err = engine.validateManifest(workspace, manifest); err != nil {
		return "", err
	}
	for _, key := range []string{"L2/" + period, "L3/current"} {
		artifact, _, err := engine.readArtifactFromManifest(workspace, manifest, key)
		if err != nil {
			return "", err
		}
		if artifact.SynthesizerID != DeterministicWeeklySynthesizerID || artifact.Content == "" || len(artifact.Sources) == 0 {
			return "", errors.New("weekly commit lacks deterministic source evidence")
		}
	}
	return filepath.Join(engine.workspaceRoot(workspace), "commits", name), nil
}

type WeeklyOutputStatus struct {
	Period         string
	GeneratedAt    time.Time
	LifetimeStatus string
}

func (engine *Engine) WeeklyOutput(workspace string) (WeeklyOutputStatus, error) {
	manifest, _, err := engine.latestManifest(workspace)
	if err != nil {
		return WeeklyOutputStatus{}, err
	}
	key, err := latestArtifactKeyInManifest(manifest, "L2/")
	if err != nil {
		return WeeklyOutputStatus{}, err
	}
	l2, _, err := engine.readArtifactFromManifest(workspace, manifest, key)
	if err != nil {
		return WeeklyOutputStatus{}, err
	}
	l3, _, err := engine.readArtifactFromManifest(workspace, manifest, "L3/current")
	if err != nil {
		return WeeklyOutputStatus{}, err
	}
	if l2.SynthesizerID != DeterministicWeeklySynthesizerID || l3.SynthesizerID != DeterministicWeeklySynthesizerID || l2.Promotion == nil {
		return WeeklyOutputStatus{}, errors.New("weekly output lacks deterministic provenance")
	}
	status := "prerequisites_unmet"
	if strings.Contains(l2.Promotion.Reason, "confirmation unavailable") {
		status = "confirmation_unavailable"
	}
	if l2.Promotion.Eligible {
		status = "exact_candidate_approved"
	}
	return WeeklyOutputStatus{l2.Period, l3.GeneratedAt, status}, nil
}
