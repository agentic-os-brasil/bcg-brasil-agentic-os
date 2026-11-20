package memory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ModelWeeklyExecution records an effect already authorized by the product's
// private native session. Reading it alone never qualifies or enrolls a job.
type ModelWeeklyExecution struct {
	RegistrationSHA256   string    `json:"registration_sha256"`
	QualificationSHA256  string    `json:"qualification_sha256"`
	ActivationSHA256     string    `json:"activation_sha256"`
	ImplementationSHA256 string    `json:"implementation_sha256"`
	WorkspaceID          string    `json:"workspace_id"`
	CommandID            string    `json:"command_id"`
	AttemptID            string    `json:"attempt_id"`
	OccurrenceSHA256     string    `json:"occurrence_sha256"`
	ScheduledFor         time.Time `json:"scheduled_for"`
	RequestedAt          time.Time `json:"requested_at"`
	Deadline             time.Time `json:"deadline"`
}

var modelExecutionCommandID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,63}$`)

func (e ModelWeeklyExecution) Validate(workspace string) error {
	if e.WorkspaceID != workspace || !modelExecutionCommandID.MatchString(e.CommandID) || len(e.AttemptID) != 32 || !isHex(e.AttemptID) || e.ScheduledFor.IsZero() || e.RequestedAt.Before(e.ScheduledFor) || !e.Deadline.After(e.RequestedAt) || e.Deadline.Sub(e.RequestedAt) > 2*time.Minute {
		return errors.New("invalid model memory execution")
	}
	for _, d := range []string{e.RegistrationSHA256, e.QualificationSHA256, e.ActivationSHA256, e.ImplementationSHA256, e.OccurrenceSHA256} {
		if len(d) != 64 || !isHex(d) {
			return errors.New("invalid model memory execution binding")
		}
	}
	return nil
}

// One bounded sidecar lives beside L2/L3 in the original atomic version. Pending lifetime
// is private candidate data, not an active layer, and survives unchanged-input
// skips until independently reviewed or invalidated by source visibility/drift.
type ModelWeeklyCommit struct {
	SchemaVersion   int                   `json:"schema_version"`
	Input           ModelWeeklyInput      `json:"input"`
	CandidateSHA256 string                `json:"candidate_sha256"`
	L2SHA256        string                `json:"l2_sha256"`
	L3SHA256        string                `json:"l3_sha256"`
	PendingLifetime *Artifact             `json:"pending_lifetime,omitempty"`
	Execution       *ModelWeeklyExecution `json:"execution,omitempty"`
}

func modelValueDigest(v any) string {
	body, _ := json.Marshal(v)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
func modelArtifactDigest(a Artifact) string {
	sum := sha256.Sum256(artifactSource(a).Content)
	return hex.EncodeToString(sum[:])
}
func (r ModelWeeklyCommit) Digest() string { return modelValueDigest(r) }
func cloneModelWeeklyCommit(r *ModelWeeklyCommit) (*ModelWeeklyCommit, error) {
	body, err := json.Marshal(r)
	if err != nil {
		return nil, err
	}
	var owned ModelWeeklyCommit
	if err = json.Unmarshal(body, &owned); err != nil {
		return nil, err
	}
	return &owned, nil
}
func (r ModelWeeklyCommit) validate(workspace string) error {
	if r.SchemaVersion != 1 || r.Input.WorkspaceID != workspace || r.Input.Purpose != "consolidate" || r.Input.Validate() != nil {
		return errors.New("invalid model memory commit input")
	}
	for _, d := range []string{r.CandidateSHA256, r.L2SHA256, r.L3SHA256} {
		if len(d) != 64 || !isHex(d) {
			return errors.New("invalid model memory commit digest")
		}
	}
	if r.Execution != nil && r.Execution.Validate(workspace) != nil {
		return errors.New("invalid model memory commit execution")
	}
	if p := r.PendingLifetime; p != nil && (p.WorkspaceID != workspace || p.Layer != "lifetime" || p.SynthesizerID != r.Input.SynthesizerID || validateArtifactCore(*p) != nil || p.Promotion == nil || p.Promotion.Eligible || p.Promotion.EligibilityPolicy != r.Input.PolicyID) {
		return errors.New("invalid pending lifetime candidate")
	}
	body, err := json.Marshal(r)
	if err != nil || len(body) > 64<<10 {
		return errors.New("model memory commit exceeds bound")
	}
	return nil
}

// ReadModelWeeklyCommit requires canonical current output/visibility and source
// validity. Historical metadata or a model report cannot substitute for it.
func (engine *Engine) ReadModelWeeklyCommit(workspace string) (ModelWeeklyCommit, string, error) {
	manifest, name, err := engine.latestManifest(workspace)
	if err != nil {
		return ModelWeeklyCommit{}, "", err
	}
	r, err := engine.readModelWeeklyRecord(manifest)
	if err != nil {
		return ModelWeeklyCommit{}, "", err
	}
	if err = r.validate(workspace); err != nil {
		return ModelWeeklyCommit{}, "", err
	}
	if r.Input.PolicyID != engine.EligibilityPolicyID || r.Input.MemoryPolicySHA256 != modelValueDigest(engine.Policy) {
		return ModelWeeklyCommit{}, "", ErrCaptureConflict
	}
	layers := map[string]Artifact{}
	for _, item := range []struct{ key, digest string }{{"L2/" + r.Input.Period, r.L2SHA256}, {"L3/current", r.L3SHA256}} {
		artifact, _, e := engine.readArtifactFromManifest(workspace, manifest, item.key)
		if e != nil || engine.validateArtifact(artifact) != nil || modelArtifactDigest(artifact) != item.digest || artifact.SynthesizerID != r.Input.SynthesizerID {
			return ModelWeeklyCommit{}, "", errors.Join(ErrCaptureConflict, e)
		}
		layers[artifact.Layer] = artifact
	}
	content := map[string]string{"L2": layers["L2"].Content, "L3": layers["L3"].Content}
	if pending := r.PendingLifetime; pending != nil {
		if err = engine.validateArtifactCandidate(*pending); err != nil {
			return ModelWeeklyCommit{}, "", err
		}
		l3source := artifactSource(layers["L3"])
		sources := []SourceDocument{l3source}
		if pending.VisibilityRevision != layers["L3"].VisibilityRevision || len(pending.Sources) < 1 || pending.Sources[0].ID != l3source.ID || pending.Sources[0].SHA256 != r.L3SHA256 {
			return ModelWeeklyCommit{}, "", ErrCaptureConflict
		}
		previous, _, e := engine.readArtifactFromManifest(workspace, manifest, "lifetime/current")
		if e == nil {
			sources = append(sources, artifactSource(previous))
			if engine.validateArtifact(previous) != nil || len(pending.Sources) != 2 || pending.Sources[1] != sourceRefs(sources)[1] {
				return ModelWeeklyCommit{}, "", ErrCaptureConflict
			}
		} else if !errors.Is(e, os.ErrNotExist) {
			return ModelWeeklyCommit{}, "", e
		} else if len(pending.Sources) != 1 {
			return ModelWeeklyCommit{}, "", ErrCaptureConflict
		}
		original := *engine
		original.SynthesizerID = r.Input.SynthesizerID
		if pending.SourceFingerprint != original.sourceFingerprint("weekly-lifetime|"+r.Input.PolicyID, sources) || layers["L2"].Promotion == nil || layers["L2"].Promotion.Eligible || layers["L2"].Promotion.Reason != pending.Promotion.Reason {
			return ModelWeeklyCommit{}, "", ErrCaptureConflict
		}
		content["lifetime"] = pending.Content
	} else {
		lifetime, _, e := engine.readArtifactFromManifest(workspace, manifest, "lifetime/current")
		if e != nil || engine.validateArtifact(lifetime) != nil || lifetime.SynthesizerID != r.Input.SynthesizerID || len(lifetime.Sources) < 1 || lifetime.Sources[0].SHA256 != r.L3SHA256 {
			return ModelWeeklyCommit{}, "", errors.Join(ErrCaptureConflict, e)
		}
		content["lifetime"] = lifetime.Content
	}
	candidate := ModelWeeklyCandidate{SchemaVersion: 1, JobID: ModelWeeklyJobID, WorkspaceID: workspace, Period: r.Input.Period, InputSHA256: r.Input.InputSHA256, Content: content}
	if modelValueDigest(candidate) != r.CandidateSHA256 {
		return ModelWeeklyCommit{}, "", errors.New("model weekly candidate differs from atomic output")
	}
	monday, err := modelInputMonday(r.Input.Period)
	if err != nil {
		return ModelWeeklyCommit{}, "", err
	}
	expected := map[string]string{}
	for _, artifact := range r.Input.Sources {
		if artifact.Layer == "L1" {
			expected[artifactSource(artifact).ID] = modelArtifactDigest(artifact)
		}
	}
	changed := false
	for day := 0; day < 7; day++ {
		key := "L1/" + monday.AddDate(0, 0, day).Format("2006-01-02")
		artifact, _, e := engine.readArtifactFromManifest(workspace, manifest, key)
		if errors.Is(e, os.ErrNotExist) {
			changed = changed || expected[key] != ""
			continue
		}
		if e != nil {
			return ModelWeeklyCommit{}, "", e
		}
		if engine.validateArtifact(artifact) != nil {
			return ModelWeeklyCommit{}, "", ErrCaptureConflict
		}
		changed = changed || expected[key] == "" || modelArtifactDigest(artifact) != expected[key]
	}
	if changed {
		return ModelWeeklyCommit{}, "", errors.Join(ErrCaptureConflict, ErrModelWeeklySourcesChanged)
	}
	owned, err := cloneModelWeeklyCommit(&r)
	if err != nil {
		return ModelWeeklyCommit{}, "", err
	}
	return *owned, filepath.Join(engine.workspaceRoot(workspace), "commits", name), nil
}

// An explicit foreground weekly request may reuse a still-current exact
// candidate. A different period is ordinary new work; source drift is surfaced
// separately so callers never promote the stale candidate.
func (engine *Engine) PendingModelWeeklyLifetime(workspace string, at time.Time) (ModelWeeklyCommit, error) {
	manifest, _, err := engine.latestManifest(workspace)
	if err != nil {
		return ModelWeeklyCommit{}, err
	}
	year, week := startOfISOWeek(at).ISOWeek()
	r, err := engine.readModelWeeklyRecord(manifest)
	if err != nil {
		return ModelWeeklyCommit{}, err
	}
	if r.PendingLifetime == nil || r.Input.Period != fmt.Sprintf("%04d-W%02d", year, week) {
		return ModelWeeklyCommit{}, os.ErrNotExist
	}
	r, _, err = engine.ReadModelWeeklyCommit(workspace)
	return r, err
}

// PromoteModelWeeklyLifetime uses the exact stored candidate and the original
// eligibility callback. It never synthesizes again or alters calendar ancestry.
func (engine *Engine) PromoteModelWeeklyLifetime(ctx context.Context, workspace, expected string) (DreamResult, error) {
	if err := engine.validateRuntime(workspace, true); err != nil {
		return DreamResult{}, err
	}
	unlock, err := engine.acquireCycleLock(workspace, "activation")
	if err != nil {
		return DreamResult{}, err
	}
	defer unlock()
	r, _, err := engine.ReadModelWeeklyCommit(workspace)
	if err != nil {
		return DreamResult{}, err
	}
	if r.Digest() != expected {
		return DreamResult{}, ErrCaptureConflict
	}
	if r.PendingLifetime == nil {
		return DreamResult{Cycle: "weekly", Period: r.Input.Period, Skipped: true}, nil
	}
	candidate := *r.PendingLifetime
	l3, _, err := engine.readArtifactByKey(workspace, "L3/current")
	if err != nil {
		return DreamResult{}, err
	}
	candidate.l3Generations, err = engine.committedL3GenerationCount(workspace, l3)
	if err != nil {
		return DreamResult{}, err
	}
	l2, _, err := engine.readArtifactByKey(workspace, "L2/"+r.Input.Period)
	if err != nil {
		return DreamResult{}, err
	}
	candidate.l3Generations, err = engine.l3MaterialGenerationCount(workspace, l3, l2, true)
	if err != nil {
		return DreamResult{}, err
	}
	candidate.l3EvidenceDigest = sha256.Sum256(artifactSource(candidate).Content)
	eligible, reason, err := engine.Eligibility.Evaluate(ctx, candidate)
	if err != nil {
		return DreamResult{}, err
	}
	if strings.TrimSpace(reason) == "" {
		return DreamResult{}, errors.New("lifetime eligibility reason required")
	}
	out := DreamResult{Cycle: "weekly", Period: r.Input.Period, LifetimeReason: reason}
	if !eligible {
		return out, nil
	}
	if err := ctx.Err(); err != nil {
		return DreamResult{}, err
	}
	current, _, err := engine.ReadModelWeeklyCommit(workspace)
	if err != nil || current.Digest() != expected {
		return DreamResult{}, errors.Join(ErrCaptureConflict, err)
	}
	candidate.Promotion = &PromotionRecord{EligibilityPolicy: engine.EligibilityPolicyID, Eligible: true, Reason: reason, EvaluatedAt: engine.Now().UTC()}
	r.PendingLifetime = nil
	owned := *engine
	owned.modelWeekly = &r
	if err = owned.activate(workspace, []Artifact{l2, l3, candidate}); err != nil {
		return DreamResult{}, err
	}
	out.ActivatedLayers = []string{"lifetime"}
	return out, nil
}

// The original canonical L3 filename pins the exact sidecar bytes. The sidecar
// carries data and execution provenance only, never standalone native authority.
func (engine *Engine) readModelWeeklyRecord(manifest CommitManifest) (ModelWeeklyCommit, error) {
	_, path, err := engine.readArtifactFromManifest(manifest.WorkspaceID, manifest, "L3/current")
	if err != nil {
		return ModelWeeklyCommit{}, err
	}
	name := filepath.Base(path)
	marker := strings.LastIndex(name, "-model-")
	if marker < 0 {
		return ModelWeeklyCommit{}, os.ErrNotExist
	}
	hash := strings.TrimSuffix(name[marker+len("-model-"):], ".json")
	if !strings.HasSuffix(name, ".json") || len(hash) != 64 || !isHex(hash) {
		return ModelWeeklyCommit{}, ErrCaptureConflict
	}
	body, err := memoryReadFile(filepath.Join(filepath.Dir(path), "model-weekly.json"), 64<<10)
	if err != nil {
		return ModelWeeklyCommit{}, errors.Join(ErrCaptureConflict, err)
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != hash {
		return ModelWeeklyCommit{}, errors.New("model weekly sidecar differs from canonical L3 anchor")
	}
	var record ModelWeeklyCommit
	if err = decodeMemoryJSON(body, &record); err != nil {
		return record, err
	}
	return record, record.validate(manifest.WorkspaceID)
}

func (engine *Engine) stageModelWeeklyRecord(staging, workspace string, artifacts []Artifact) (string, error) {
	if engine.modelWeekly == nil {
		return "", nil
	}
	owned, err := cloneModelWeeklyCommit(engine.modelWeekly)
	if err != nil {
		return "", err
	}
	l2, l3 := false, false
	for _, artifact := range artifacts {
		if artifact.Layer == "L2" {
			if l2 {
				return "", errors.New("duplicate model weekly L2")
			}
			owned.L2SHA256 = modelArtifactDigest(artifact)
			l2 = true
		}
		if artifact.Layer == "L3" {
			if l3 {
				return "", errors.New("duplicate model weekly L3")
			}
			owned.L3SHA256 = modelArtifactDigest(artifact)
			l3 = true
		}
	}
	if !l2 || !l3 {
		return "", errors.New("model weekly sidecar requires the exact atomic L2/L3 pair")
	}
	if err = owned.validate(workspace); err != nil {
		return "", err
	}
	path := filepath.Join(staging, "model-weekly.json")
	if err = writeJSONFile(path, owned); err != nil {
		return "", err
	}
	body, err := memoryReadFile(path, 64<<10)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:]), nil
}

// Cleanup classification grants no read/publication authority. It preserves a
// well-formed current-generation sidecar only with its exact L3 filename anchor;
// original denial removes the entire old version, including this private body.
func modelWeeklySidecarInGeneration(workspace string, generation uint64, body []byte, names []string) bool {
	if len(body) > 64<<10 {
		return false
	}
	var record ModelWeeklyCommit
	if decodeMemoryJSON(body, &record) != nil || record.validate(workspace) != nil {
		return false
	}
	for _, source := range record.Input.Sources {
		if source.VisibilityRevision != generation {
			return false
		}
	}
	if record.PendingLifetime != nil && record.PendingLifetime.VisibilityRevision != generation {
		return false
	}
	sum := sha256.Sum256(body)
	suffix := "-l3-rolling-model-" + hex.EncodeToString(sum[:]) + ".json"
	for _, name := range names {
		if len(name) == 2+len(suffix) && name[0] >= '0' && name[0] <= '9' && name[1] >= '0' && name[1] <= '9' && strings.HasSuffix(name, suffix) {
			return true
		}
	}
	return false
}
