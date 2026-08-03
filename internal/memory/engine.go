package memory

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

var workspaceIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

var ErrDreamInProgress = errors.New("memory dreaming cycle already in progress")

type Capture struct {
	SchemaVersion int       `json:"schema_version,omitempty"`
	WorkspaceID   string    `json:"workspace_id"`
	RecordedAt    time.Time `json:"recorded_at"`
	Kind          string    `json:"kind"`
	Text          string    `json:"text"`
	Sanitized     bool      `json:"sanitized"`
	ProducerID    string    `json:"producer_id,omitempty"`
	SanitizerID   string    `json:"sanitizer_id,omitempty"`
	SourceDigest  string    `json:"source_digest,omitempty"`
	Attestation   string    `json:"attestation,omitempty"`
}

type SourceDocument struct {
	ID      string
	Content []byte
}

type SourceRef struct {
	ID     string `json:"id"`
	SHA256 string `json:"sha256"`
}

type PromotionRecord struct {
	EligibilityPolicy string    `json:"eligibility_policy"`
	Eligible          bool      `json:"eligible"`
	Reason            string    `json:"reason"`
	EvaluatedAt       time.Time `json:"evaluated_at"`
}

type Artifact struct {
	SchemaVersion     int              `json:"schema_version"`
	WorkspaceID       string           `json:"workspace_id"`
	Layer             string           `json:"layer"`
	Period            string           `json:"period"`
	GeneratedAt       time.Time        `json:"generated_at"`
	SourceFingerprint string           `json:"source_fingerprint"`
	Sources           []SourceRef      `json:"sources"`
	SynthesizerID     string           `json:"synthesizer_id"`
	Content           string           `json:"content"`
	Promotion         *PromotionRecord `json:"promotion,omitempty"`
}

type SynthesisRequest struct {
	Cycle       string
	TargetLayer string
	WorkspaceID string
	Period      string
	Sources     []SourceDocument
}

type Synthesizer interface {
	Synthesize(context.Context, SynthesisRequest) (string, error)
}

type Eligibility interface {
	Evaluate(context.Context, Artifact) (eligible bool, reason string, err error)
}

type EligibilityFunc func(context.Context, Artifact) (bool, string, error)

func (function EligibilityFunc) Evaluate(ctx context.Context, artifact Artifact) (bool, string, error) {
	return function(ctx, artifact)
}

type Engine struct {
	Root                string
	Policy              Policy
	Budgets             map[string]int
	Freshness           map[string]time.Duration
	Synthesizer         Synthesizer
	SynthesizerID       string
	Eligibility         Eligibility
	EligibilityPolicyID string
	Now                 func() time.Time
	FaultPoint          func(string) error
	MaxSourceBytes      int
	contextReadPoint    func(string) error
}

type DreamResult struct {
	Cycle             string
	Period            string
	SourceFingerprint string
	Skipped           bool
	ActivatedLayers   []string
	LifetimeReason    string
}

type ContextSection struct {
	Layer     string `json:"layer"`
	Content   string `json:"content"`
	DrillDown string `json:"drill_down"`
	Truncated bool   `json:"truncated"`
}

type ContextBundle struct {
	Sections    []ContextSection `json:"sections"`
	Diagnostics []string         `json:"diagnostics"`
}

func (engine *Engine) validate() error {
	if err := engine.validateCore(); err != nil {
		return err
	}
	for _, layer := range []string{"L1", "L2", "L3", "lifetime"} {
		if engine.Budgets[layer] <= 0 {
			return fmt.Errorf("positive context budget required for %s", layer)
		}
	}
	return nil
}

func (engine *Engine) validateCore() error {
	if strings.TrimSpace(engine.Root) == "" {
		return errors.New("memory root is required")
	}
	if err := engine.Policy.Validate(); err != nil {
		return fmt.Errorf("memory policy: %w", err)
	}
	if engine.Now == nil {
		engine.Now = time.Now
	}
	return nil
}

func validateWorkspaceID(workspaceID string) error {
	if !workspaceIDPattern.MatchString(workspaceID) {
		return fmt.Errorf("invalid workspace ID %q", workspaceID)
	}
	return nil
}

func (engine *Engine) workspaceRoot(workspaceID string) string {
	return filepath.Join(engine.Root, "workspaces", workspaceID)
}

func (engine *Engine) currentPath(workspaceID, layer, name string) string {
	return filepath.Join(engine.workspaceRoot(workspaceID), strings.ToLower(layer), name)
}

func (engine *Engine) Capture(capture Capture) (string, error) {
	if err := engine.validateCore(); err != nil {
		return "", err
	}
	if err := validateWorkspaceID(capture.WorkspaceID); err != nil {
		return "", err
	}
	if !capture.Sanitized {
		return "", errors.New("capture must be sanitized before persistence")
	}
	if capture.SchemaVersion != 0 && capture.SchemaVersion != AttestedCaptureSchemaVersion {
		return "", errors.New("unsupported capture schema version")
	}
	if capture.SchemaVersion == AttestedCaptureSchemaVersion && len(capture.Attestation) != 64 {
		return "", errors.New("attested capture requires its producer attestation")
	}
	if capture.RecordedAt.IsZero() || strings.TrimSpace(capture.Kind) == "" || strings.TrimSpace(capture.Text) == "" {
		return "", errors.New("capture requires recorded_at, kind and text")
	}
	directory := "captures"
	if capture.SchemaVersion == AttestedCaptureSchemaVersion {
		directory = "attested-captures"
	}
	path := engine.currentPath(capture.WorkspaceID, "l1", filepath.Join(directory, capture.RecordedAt.UTC().Format("2006-01-02")+".jsonl"))
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	defer file.Close()
	encoded, err := json.Marshal(capture)
	if err != nil {
		return "", err
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		return "", err
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	return path, nil
}

func (engine *Engine) DreamDaily(ctx context.Context, workspaceID string, day time.Time) (DreamResult, error) {
	return engine.dreamDaily(ctx, workspaceID, day, "captures")
}

func (engine *Engine) DreamDailyAttested(ctx context.Context, workspaceID string, day time.Time) (DreamResult, error) {
	return engine.dreamDaily(ctx, workspaceID, day, "attested-captures")
}

func (engine *Engine) dreamDaily(ctx context.Context, workspaceID string, day time.Time, captureDirectory string) (DreamResult, error) {
	if err := engine.validateRuntime(workspaceID, false); err != nil {
		return DreamResult{}, err
	}
	period := day.UTC().Format("2006-01-02")
	release, err := engine.acquireCycleLock(workspaceID, "activation")
	if err != nil {
		return DreamResult{}, err
	}
	defer release()
	capturePath := engine.currentPath(workspaceID, "l1", filepath.Join(captureDirectory, period+".jsonl"))
	sources, err := engine.readSources(workspaceID, []string{capturePath})
	if err != nil {
		return DreamResult{}, err
	}
	fingerprint := engine.sourceFingerprint("daily", sources)
	if existing, _, err := engine.readArtifactByKey(workspaceID, "L1/"+period); err == nil && engine.validateArtifact(existing) == nil && existing.WorkspaceID == workspaceID && existing.Layer == "L1" && existing.Period == period && existing.SourceFingerprint == fingerprint && existing.SynthesizerID == engine.SynthesizerID {
		return DreamResult{Cycle: "daily", Period: period, SourceFingerprint: fingerprint, Skipped: true}, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return DreamResult{}, err
	}
	content, err := engine.Synthesizer.Synthesize(ctx, SynthesisRequest{Cycle: "daily", TargetLayer: "L1", WorkspaceID: workspaceID, Period: period, Sources: sources})
	if err != nil {
		return DreamResult{}, fmt.Errorf("synthesize L1: %w", err)
	}
	artifact := engine.newArtifact(workspaceID, "L1", period, fingerprint, sources, content)
	if err := engine.activate(workspaceID, []Artifact{artifact}); err != nil {
		return DreamResult{}, err
	}
	return DreamResult{Cycle: "daily", Period: period, SourceFingerprint: fingerprint, ActivatedLayers: []string{"L1"}}, nil
}

func (engine *Engine) DreamWeekly(ctx context.Context, workspaceID string, anyDay time.Time) (DreamResult, error) {
	if err := engine.validateRuntime(workspaceID, true); err != nil {
		return DreamResult{}, err
	}
	monday := startOfISOWeek(anyDay)
	year, week := monday.ISOWeek()
	period := fmt.Sprintf("%04d-W%02d", year, week)
	release, err := engine.acquireCycleLock(workspaceID, "activation")
	if err != nil {
		return DreamResult{}, err
	}
	defer release()
	var l1Sources []SourceDocument
	for offset := 0; offset < 7; offset++ {
		date := monday.AddDate(0, 0, offset).Format("2006-01-02")
		artifact, _, err := engine.readArtifactByKey(workspaceID, "L1/"+date)
		if err == nil {
			l1Sources = append(l1Sources, artifactSource(artifact))
		} else if !errors.Is(err, os.ErrNotExist) {
			return DreamResult{}, err
		}
	}
	if len(l1Sources) == 0 {
		return DreamResult{}, errors.New("weekly dream requires at least one L1 digest")
	}
	runFingerprint := engine.sourceFingerprint("weekly|"+engine.EligibilityPolicyID, l1Sources)
	if existing, _, err := engine.readArtifactByKey(workspaceID, "L2/"+period); err == nil && engine.validateArtifact(existing) == nil && existing.WorkspaceID == workspaceID && existing.Layer == "L2" && existing.Period == period && existing.SourceFingerprint == runFingerprint && existing.SynthesizerID == engine.SynthesizerID {
		result := DreamResult{Cycle: "weekly", Period: period, SourceFingerprint: runFingerprint, Skipped: true}
		if existing.Promotion != nil {
			result.LifetimeReason = existing.Promotion.Reason
		}
		return result, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return DreamResult{}, err
	}

	l2Content, err := engine.Synthesizer.Synthesize(ctx, SynthesisRequest{Cycle: "weekly", TargetLayer: "L2", WorkspaceID: workspaceID, Period: period, Sources: l1Sources})
	if err != nil {
		return DreamResult{}, fmt.Errorf("synthesize L2: %w", err)
	}
	l2 := engine.newArtifact(workspaceID, "L2", period, runFingerprint, l1Sources, l2Content)

	l3Sources := []SourceDocument{artifactSource(l2)}
	if current, _, err := engine.readArtifactByKey(workspaceID, "L3/current"); err == nil {
		l3Sources = append(l3Sources, artifactSource(current))
	} else if !errors.Is(err, os.ErrNotExist) {
		return DreamResult{}, err
	}
	l3Fingerprint := engine.sourceFingerprint("weekly-l3", l3Sources)
	l3Content, err := engine.Synthesizer.Synthesize(ctx, SynthesisRequest{Cycle: "weekly", TargetLayer: "L3", WorkspaceID: workspaceID, Period: period, Sources: l3Sources})
	if err != nil {
		return DreamResult{}, fmt.Errorf("synthesize L3: %w", err)
	}
	l3 := engine.newArtifact(workspaceID, "L3", "rolling", l3Fingerprint, l3Sources, l3Content)

	lifetimeSources := []SourceDocument{artifactSource(l3)}
	if current, _, err := engine.readArtifactByKey(workspaceID, "lifetime/current"); err == nil {
		lifetimeSources = append(lifetimeSources, artifactSource(current))
	} else if !errors.Is(err, os.ErrNotExist) {
		return DreamResult{}, err
	}
	lifetimeFingerprint := engine.sourceFingerprint("weekly-lifetime|"+engine.EligibilityPolicyID, lifetimeSources)
	lifetimeContent, err := engine.Synthesizer.Synthesize(ctx, SynthesisRequest{Cycle: "weekly", TargetLayer: "lifetime", WorkspaceID: workspaceID, Period: period, Sources: lifetimeSources})
	if err != nil {
		return DreamResult{}, fmt.Errorf("synthesize lifetime: %w", err)
	}
	lifetime := engine.newArtifact(workspaceID, "lifetime", "lifetime", lifetimeFingerprint, lifetimeSources, lifetimeContent)
	if err := engine.validateArtifactCandidate(lifetime); err != nil {
		return DreamResult{}, fmt.Errorf("validate lifetime candidate: %w", err)
	}
	eligible, reason, err := engine.Eligibility.Evaluate(ctx, lifetime)
	if err != nil {
		return DreamResult{}, fmt.Errorf("evaluate lifetime eligibility: %w", err)
	}
	if strings.TrimSpace(reason) == "" {
		return DreamResult{}, errors.New("lifetime eligibility must return a reason")
	}
	lifetime.Promotion = &PromotionRecord{EligibilityPolicy: engine.EligibilityPolicyID, Eligible: eligible, Reason: reason, EvaluatedAt: engine.Now().UTC()}
	l2.Promotion = lifetime.Promotion

	artifacts := []Artifact{l2, l3}
	result := DreamResult{Cycle: "weekly", Period: period, SourceFingerprint: runFingerprint, ActivatedLayers: []string{"L2", "L3"}, LifetimeReason: reason}
	if eligible {
		artifacts = append(artifacts, lifetime)
		result.ActivatedLayers = append(result.ActivatedLayers, "lifetime")
	}
	if err := engine.activate(workspaceID, artifacts); err != nil {
		return DreamResult{}, err
	}
	return result, nil
}

func (engine *Engine) acquireCycleLock(workspaceID, name string) (func(), error) {
	locksRoot := filepath.Join(engine.workspaceRoot(workspaceID), ".locks")
	if err := os.MkdirAll(locksRoot, 0o700); err != nil {
		return nil, err
	}
	lockPath := filepath.Join(locksRoot, name+".lock")
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("%w: %s", ErrDreamInProgress, name)
		}
		return nil, err
	}
	metadata := map[string]any{"cycle": name, "started_at": engine.Now().UTC(), "pid": os.Getpid()}
	if err := writeJSONFile(filepath.Join(lockPath, "owner.json"), metadata); err != nil {
		_ = os.RemoveAll(lockPath)
		return nil, err
	}
	return func() { _ = os.RemoveAll(lockPath) }, nil
}

func (engine *Engine) validateRuntime(workspaceID string, weekly bool) error {
	if err := engine.validate(); err != nil {
		return err
	}
	if err := validateWorkspaceID(workspaceID); err != nil {
		return err
	}
	if engine.Synthesizer == nil || strings.TrimSpace(engine.SynthesizerID) == "" {
		return errors.New("dreaming requires a synthesizer and synthesizer ID")
	}
	if weekly && (engine.Eligibility == nil || strings.TrimSpace(engine.EligibilityPolicyID) == "") {
		return errors.New("weekly dreaming requires an eligibility policy and policy ID")
	}
	return nil
}

func startOfISOWeek(value time.Time) time.Time {
	utc := value.UTC()
	day := int(utc.Weekday())
	if day == 0 {
		day = 7
	}
	date := time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
	return date.AddDate(0, 0, 1-day)
}

func (engine *Engine) readSources(workspaceID string, paths []string) ([]SourceDocument, error) {
	sort.Strings(paths)
	root := engine.workspaceRoot(workspaceID)
	sources := make([]SourceDocument, 0, len(paths))
	maximum := engine.MaxSourceBytes
	if maximum <= 0 {
		maximum = 1 << 20
	}
	total := 0
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		remaining := maximum - total
		if remaining <= 0 {
			_ = file.Close()
			return nil, errors.New("memory dream sources exceed the configured byte limit")
		}
		content, readErr := io.ReadAll(io.LimitReader(file, int64(remaining+1)))
		closeErr := file.Close()
		if readErr != nil {
			return nil, readErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if len(content) > remaining {
			return nil, errors.New("memory dream sources exceed the configured byte limit")
		}
		total += len(content)
		relative, err := filepath.Rel(root, path)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("source outside workspace memory root: %s", path)
		}
		sources = append(sources, SourceDocument{ID: filepath.ToSlash(relative), Content: content})
	}
	return sources, nil
}

func (engine *Engine) sourceFingerprint(namespace string, sources []SourceDocument) string {
	hash := sha256.New()
	_, _ = io.WriteString(hash, namespace)
	_, _ = io.WriteString(hash, "\x00"+engine.SynthesizerID+"\x00")
	policy, _ := json.Marshal(engine.Policy)
	_, _ = hash.Write(policy)
	_, _ = hash.Write([]byte{0})
	for _, source := range sources {
		_, _ = io.WriteString(hash, source.ID)
		_, _ = hash.Write([]byte{0})
		_, _ = hash.Write(source.Content)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func sourceRefs(sources []SourceDocument) []SourceRef {
	references := make([]SourceRef, 0, len(sources))
	for _, source := range sources {
		sum := sha256.Sum256(source.Content)
		references = append(references, SourceRef{ID: source.ID, SHA256: hex.EncodeToString(sum[:])})
	}
	return references
}

func artifactSource(artifact Artifact) SourceDocument {
	content, _ := json.Marshal(artifact)
	return SourceDocument{ID: artifact.Layer + "/" + artifact.Period, Content: content}
}

func (engine *Engine) newArtifact(workspaceID, layer, period, fingerprint string, sources []SourceDocument, content string) Artifact {
	return Artifact{
		SchemaVersion:     1,
		WorkspaceID:       workspaceID,
		Layer:             layer,
		Period:            period,
		GeneratedAt:       engine.Now().UTC(),
		SourceFingerprint: fingerprint,
		Sources:           sourceRefs(sources),
		SynthesizerID:     engine.SynthesizerID,
		Content:           strings.TrimSpace(content),
	}
}

func (engine *Engine) validateArtifact(artifact Artifact) error {
	if err := validateArtifactStructure(artifact); err != nil {
		return err
	}
	return engine.validateArtifactBudget(artifact)
}

func (engine *Engine) validateArtifactCandidate(artifact Artifact) error {
	if err := validateArtifactCore(artifact); err != nil {
		return err
	}
	return engine.validateArtifactBudget(artifact)
}

func (engine *Engine) validateArtifactBudget(artifact Artifact) error {
	budget := engine.Budgets[artifact.Layer]
	if budget <= 0 {
		return fmt.Errorf("missing budget for %s", artifact.Layer)
	}
	if contentLength := len([]rune(artifact.Content)); contentLength > budget {
		return fmt.Errorf("%s content length %d exceeds budget %d", artifact.Layer, contentLength, budget)
	}
	return nil
}

func validateArtifactStructure(artifact Artifact) error {
	if err := validateArtifactCore(artifact); err != nil {
		return err
	}
	if artifact.Layer == "lifetime" {
		if artifact.Promotion == nil || !artifact.Promotion.Eligible || artifact.Promotion.EligibilityPolicy == "" || strings.TrimSpace(artifact.Promotion.Reason) == "" || artifact.Promotion.EvaluatedAt.IsZero() {
			return errors.New("lifetime artifact requires a successful eligibility record")
		}
	}
	return nil
}

func validateArtifactCore(artifact Artifact) error {
	if artifact.SchemaVersion != 1 || artifact.WorkspaceID == "" || artifact.Period == "" || artifact.GeneratedAt.IsZero() {
		return errors.New("artifact requires schema, workspace, period and generated_at")
	}
	if err := validateWorkspaceID(artifact.WorkspaceID); err != nil {
		return err
	}
	if artifact.Layer != "L1" && artifact.Layer != "L2" && artifact.Layer != "L3" && artifact.Layer != "lifetime" {
		return fmt.Errorf("unknown memory layer %q", artifact.Layer)
	}
	if len([]rune(artifact.Content)) == 0 {
		return fmt.Errorf("%s content is empty", artifact.Layer)
	}
	if len(artifact.SourceFingerprint) != 64 || !isHex(artifact.SourceFingerprint) || len(artifact.Sources) == 0 || strings.TrimSpace(artifact.SynthesizerID) == "" {
		return errors.New("artifact requires source fingerprint, provenance and synthesizer ID")
	}
	for _, source := range artifact.Sources {
		if strings.TrimSpace(source.ID) == "" || len(source.SHA256) != 64 || !isHex(source.SHA256) {
			return errors.New("artifact contains invalid source provenance")
		}
	}
	return nil
}

func isHex(value string) bool {
	_, err := hex.DecodeString(value)
	return err == nil
}

func writeJSONFile(path string, value any) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func (engine *Engine) readArtifact(path string) (Artifact, error) {
	file, err := os.Open(path)
	if err != nil {
		return Artifact{}, err
	}
	defer file.Close()
	decoder := json.NewDecoder(bufio.NewReader(file))
	decoder.DisallowUnknownFields()
	var artifact Artifact
	if err := decoder.Decode(&artifact); err != nil {
		return Artifact{}, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Artifact{}, err
	}
	return artifact, nil
}

func (engine *Engine) AssembleContext(workspaceID string) (ContextBundle, error) {
	if err := engine.validate(); err != nil {
		return ContextBundle{}, err
	}
	if err := validateWorkspaceID(workspaceID); err != nil {
		return ContextBundle{}, err
	}
	var bundle ContextBundle
	manifest, _, err := engine.latestManifest(workspaceID)
	if errors.Is(err, os.ErrNotExist) {
		for _, layer := range engine.Policy.ContextInjection.Order {
			bundle.Diagnostics = append(bundle.Diagnostics, layer+": missing; skipped")
		}
		return bundle, nil
	}
	if err != nil {
		return ContextBundle{}, err
	}
	if engine.contextReadPoint != nil {
		if err := engine.contextReadPoint("after_snapshot"); err != nil {
			return ContextBundle{}, err
		}
	}
	for _, layer := range engine.Policy.ContextInjection.Order {
		key, err := contextArtifactKey(manifest, layer)
		if errors.Is(err, os.ErrNotExist) {
			bundle.Diagnostics = append(bundle.Diagnostics, layer+": missing; skipped")
			continue
		}
		if err != nil {
			return ContextBundle{}, err
		}
		artifact, path, err := engine.readArtifactFromManifest(workspaceID, manifest, key)
		if errors.Is(err, os.ErrNotExist) {
			bundle.Diagnostics = append(bundle.Diagnostics, layer+": missing; skipped")
			continue
		}
		if err != nil {
			bundle.Diagnostics = append(bundle.Diagnostics, layer+": invalid; skipped")
			continue
		}
		if artifact.Layer != layer || artifact.WorkspaceID != workspaceID || validateArtifactStructure(artifact) != nil {
			bundle.Diagnostics = append(bundle.Diagnostics, layer+": invalid; skipped")
			continue
		}
		if maximumAge := engine.Freshness[layer]; maximumAge > 0 && engine.Now().UTC().Sub(artifact.GeneratedAt.UTC()) > maximumAge {
			bundle.Diagnostics = append(bundle.Diagnostics, layer+": stale; skipped")
			continue
		}
		content, truncated := truncateRunes(artifact.Content, engine.Budgets[layer])
		relative, _ := filepath.Rel(engine.workspaceRoot(workspaceID), path)
		bundle.Sections = append(bundle.Sections, ContextSection{Layer: layer, Content: content, DrillDown: filepath.ToSlash(relative), Truncated: truncated})
	}
	return bundle, nil
}

func contextArtifactKey(manifest CommitManifest, layer string) (string, error) {
	switch layer {
	case "lifetime":
		if _, exists := manifest.Artifacts["lifetime/current"]; !exists {
			return "", os.ErrNotExist
		}
		return "lifetime/current", nil
	case "L3":
		if _, exists := manifest.Artifacts["L3/current"]; !exists {
			return "", os.ErrNotExist
		}
		return "L3/current", nil
	case "L2":
		return latestArtifactKeyInManifest(manifest, "L2/")
	case "L1":
		return latestArtifactKeyInManifest(manifest, "L1/")
	default:
		return "", fmt.Errorf("unknown context layer %q", layer)
	}
}

func truncateRunes(value string, limit int) (string, bool) {
	runes := []rune(value)
	if len(runes) <= limit {
		return value, false
	}
	return string(runes[:limit]), true
}
