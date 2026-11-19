package memory

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

const ModelWeeklyJobID = "memory-weekly"

var ErrModelWeeklyUnchanged = errors.New("current model weekly inputs are already committed")
var ErrModelWeeklySourcesChanged = errors.New("model weekly eligible source cohort changed")

// ModelWeeklyInput is a private, bounded projection of already admitted memory.
// It grants no provider access or publication authority.
type ModelWeeklyInput struct {
	SchemaVersion      int            `json:"schema_version"`
	JobID              string         `json:"job_id"`
	Purpose            string         `json:"purpose"`
	WorkspaceID        string         `json:"workspace_id"`
	Period             string         `json:"period"`
	SourceFingerprint  string         `json:"source_fingerprint"`
	PolicyID           string         `json:"policy_id"`
	MemoryPolicySHA256 string         `json:"memory_policy_sha256"`
	SynthesizerID      string         `json:"synthesizer_id"`
	Sources            []Artifact     `json:"sources"`
	LayerBytes         map[string]int `json:"layer_bytes"`
	InputSHA256        string         `json:"input_sha256"`
}

type ModelWeeklyCandidate struct {
	SchemaVersion int               `json:"schema_version"`
	JobID         string            `json:"job_id"`
	WorkspaceID   string            `json:"workspace_id"`
	Period        string            `json:"period"`
	InputSHA256   string            `json:"input_sha256"`
	Content       map[string]string `json:"content"`
}

func (engine *Engine) InspectModelWeeklyInput(workspace string, at time.Time) (ModelWeeklyInput, error) {
	return engine.inspectModelWeeklyInput(workspace, at, "consolidate")
}

// A qualification probe may inspect the actual current eligible cohort even
// when already consolidated, but that purpose can never enter domain commit.
func (engine *Engine) InspectModelWeeklyProbeInput(workspace string, at time.Time) (ModelWeeklyInput, error) {
	return engine.inspectModelWeeklyInput(workspace, at, "native_probe")
}
func (engine *Engine) inspectModelWeeklyInput(workspace string, at time.Time, purpose string) (ModelWeeklyInput, error) {
	var input ModelWeeklyInput
	if err := engine.validateRuntime(workspace, true); err != nil {
		return input, err
	}
	before, err := engine.WeeklyInputIdentity(workspace, at)
	if err != nil {
		return input, err
	}
	monday := startOfISOWeek(at)
	year, week := monday.ISOWeek()
	input = ModelWeeklyInput{SchemaVersion: 1, JobID: ModelWeeklyJobID, WorkspaceID: workspace, Period: fmt.Sprintf("%04d-W%02d", year, week), PolicyID: engine.EligibilityPolicyID, SynthesizerID: engine.SynthesizerID, SourceFingerprint: before, LayerBytes: map[string]int{}}
	input.Purpose = purpose
	input.MemoryPolicySHA256 = modelValueDigest(engine.Policy)
	var daily []SourceDocument
	for i := 0; i < 7; i++ {
		artifact, _, err := engine.readArtifactByKey(workspace, "L1/"+monday.AddDate(0, 0, i).Format("2006-01-02"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return ModelWeeklyInput{}, err
		}
		input.Sources = append(input.Sources, artifact)
		daily = append(daily, artifactSource(artifact))
	}
	if len(daily) == 0 {
		return ModelWeeklyInput{}, ErrNoLiveCaptures
	}
	// The same original weekly fingerprint owns unchanged-input behavior. A
	// model is never invoked merely to rephrase an already committed cohort.
	fingerprint := engine.sourceFingerprint("weekly|"+engine.EligibilityPolicyID, daily)
	if existing, _, err := engine.readArtifactByKey(workspace, "L2/"+input.Period); err == nil {
		if purpose == "consolidate" && existing.SynthesizerID == engine.SynthesizerID && existing.SourceFingerprint == fingerprint {
			return ModelWeeklyInput{}, ErrModelWeeklyUnchanged
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return ModelWeeklyInput{}, err
	}
	for _, key := range []string{"L3/current", "lifetime/current"} {
		artifact, _, err := engine.readArtifactByKey(workspace, key)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return ModelWeeklyInput{}, err
		}
		input.Sources = append(input.Sources, artifact)
	}
	for _, layer := range []string{"L2", "L3", "lifetime"} {
		if engine.Budgets[layer] < 1 {
			return ModelWeeklyInput{}, errors.New("model memory layer budget unavailable")
		}
		// Use the original numerical layer ceiling as a conservative byte
		// ceiling too; the engine independently rechecks its rune budget.
		input.LayerBytes[layer] = engine.Budgets[layer]
	}
	after, err := engine.WeeklyInputIdentity(workspace, at)
	if err != nil || before != after {
		return ModelWeeklyInput{}, errors.Join(ErrCaptureConflict, err)
	}
	input.InputSHA256 = input.digest()
	return input, input.Validate()
}

func ParseModelWeeklyCandidate(input ModelWeeklyInput, body []byte) (ModelWeeklyCandidate, error) {
	var candidate ModelWeeklyCandidate
	if input.Validate() != nil || len(body) > 32768 || !utf8.Valid(body) {
		return candidate, errors.New("bounded model memory candidate required")
	}
	if err := releasecontract.DecodeStrictJSON(body, &candidate); err != nil {
		return candidate, err
	}
	if candidate.SchemaVersion != 1 || candidate.JobID != ModelWeeklyJobID || candidate.WorkspaceID != input.WorkspaceID || candidate.Period != input.Period || candidate.InputSHA256 != input.InputSHA256 || len(candidate.Content) != 3 {
		return ModelWeeklyCandidate{}, errors.New("model memory candidate scope or source changed")
	}
	for _, layer := range []string{"L2", "L3", "lifetime"} {
		text, ok := candidate.Content[layer]
		if !ok || text == "" || text != strings.TrimSpace(text) || !utf8.ValidString(text) || strings.ContainsRune(text, 0) || len(text) > input.LayerBytes[layer] {
			return ModelWeeklyCandidate{}, errors.New("model memory candidate layer exceeds exact bounded contract")
		}
	}
	return candidate, nil
}

func (engine *Engine) CommitModelWeekly(ctx context.Context, input ModelWeeklyInput, candidate ModelWeeklyCandidate, at time.Time) (DreamResult, error) {
	return engine.CommitModelWeeklyWithExecution(ctx, input, candidate, nil, at)
}

// Execution is trusted composition metadata; it never supplies native proof or
// replaces the caller's original live publication guard.
func (engine *Engine) CommitModelWeeklyWithExecution(ctx context.Context, input ModelWeeklyInput, candidate ModelWeeklyCandidate, execution *ModelWeeklyExecution, at time.Time) (DreamResult, error) {
	if input.Purpose != "consolidate" {
		return DreamResult{}, errors.New("native probe input cannot publish memory")
	}
	if err := ctx.Err(); err != nil {
		return DreamResult{}, err
	}
	body, err := json.Marshal(candidate)
	if err != nil {
		return DreamResult{}, err
	}
	owned, err := ParseModelWeeklyCandidate(input, body)
	if err != nil {
		return DreamResult{}, err
	}
	current, err := engine.inspectModelWeeklyInput(input.WorkspaceID, at, input.Purpose)
	if errors.Is(err, ErrModelWeeklyUnchanged) {
		return DreamResult{Cycle: "weekly", Period: input.Period, Skipped: true}, nil
	}
	if err != nil || current.InputSHA256 != input.InputSHA256 {
		return DreamResult{}, errors.Join(ErrCaptureConflict, err)
	}
	copy := *engine
	copy.ExpectedWeeklyFingerprint = input.SourceFingerprint
	copy.Synthesizer = modelWeeklySynthesis{input: input, candidate: owned}
	if execution != nil && execution.Validate(input.WorkspaceID) != nil {
		return DreamResult{}, errors.New("invalid model memory execution metadata")
	}
	copy.modelWeekly = &ModelWeeklyCommit{SchemaVersion: 1, Input: input, Execution: execution, CandidateSHA256: modelValueDigest(owned)}
	eligibility := copy.Eligibility
	copy.Eligibility = EligibilityFunc(func(ctx context.Context, artifact Artifact) (bool, string, error) {
		ok, reason, err := eligibility.Evaluate(ctx, artifact)
		if err != nil {
			return false, "", err
		}
		if !ok {
			artifact.Promotion = &PromotionRecord{EligibilityPolicy: copy.EligibilityPolicyID, Eligible: false, Reason: reason, EvaluatedAt: copy.Now().UTC()}
			copy.modelWeekly.PendingLifetime = &artifact
		}
		return ok, reason, nil
	})
	// DreamWeekly reacquires the original activation lock, revalidates exact
	// inputs, derives all provenance, evaluates lifetime and publishes one
	// validated atomic manifest under the original publication guard.
	return copy.DreamWeekly(ctx, input.WorkspaceID, at)
}

// WithCurrentModelWeeklyInput holds the original memory mutation boundary
// through bounded input release. The callback must acquire control/lease guards
// after this lock, matching the original memory -> control -> installation order.
func (engine *Engine) WithCurrentModelWeeklyInput(ctx context.Context, input ModelWeeklyInput, at time.Time, releaseInput func() error) error {
	if releaseInput == nil {
		return errors.New("model memory input release required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	unlock, err := engine.acquireCycleLock(input.WorkspaceID, "activation")
	if err != nil {
		return err
	}
	defer unlock()
	current, err := engine.inspectModelWeeklyInput(input.WorkspaceID, at, input.Purpose)
	if err != nil || current.InputSHA256 != input.InputSHA256 {
		return errors.Join(ErrCaptureConflict, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return releaseInput()
}

func (input ModelWeeklyInput) digest() string {
	input.InputSHA256 = ""
	body, _ := json.Marshal(input)
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}

func (input ModelWeeklyInput) Validate() error {
	if len(input.MemoryPolicySHA256) != 64 || !isHex(input.MemoryPolicySHA256) {
		return errors.New("invalid model memory policy binding")
	}
	if input.Purpose != "consolidate" && input.Purpose != "native_probe" {
		return errors.New("invalid model memory input purpose")
	}
	if input.SchemaVersion != 1 || input.JobID != ModelWeeklyJobID || validateWorkspaceID(input.WorkspaceID) != nil || len(input.Period) != 8 || input.PolicyID == "" || input.SynthesizerID == "" || len(input.SourceFingerprint) != 64 || !isHex(input.SourceFingerprint) || len(input.Sources) < 1 || len(input.Sources) > 9 || input.InputSHA256 != input.digest() || len(input.LayerBytes) != 3 {
		return errors.New("model memory input binding invalid")
	}
	l1 := 0
	monday, err := modelInputMonday(input.Period)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, artifact := range input.Sources {
		key := artifact.Layer + "/" + artifact.Period
		if validateArtifactCore(artifact) != nil || artifact.WorkspaceID != input.WorkspaceID || artifact.Layer != "L1" && artifact.Layer != "L3" && artifact.Layer != "lifetime" || seen[key] {
			return errors.New("model memory input includes an invalid source")
		}
		seen[key] = true
		if artifact.Layer == "L1" {
			day, e := time.Parse("2006-01-02", artifact.Period)
			if e != nil || day.Before(monday) || !day.Before(monday.AddDate(0, 0, 7)) {
				return errors.New("model memory source outside selected ISO week")
			}
			l1++
		}
	}
	for _, layer := range []string{"L2", "L3", "lifetime"} {
		if input.LayerBytes[layer] < 1 || input.LayerBytes[layer] > 32768 {
			return errors.New("model memory layer bound invalid")
		}
	}
	body, err := json.Marshal(input)
	if err != nil || len(body) > 32768 || l1 == 0 {
		return errors.New("model memory input exceeds bounded eligible cohort")
	}
	return nil
}

func modelInputMonday(period string) (time.Time, error) {
	var year, week int
	if _, err := fmt.Sscanf(period, "%04d-W%02d", &year, &week); err != nil || year < 1 || week < 1 || week > 53 || fmt.Sprintf("%04d-W%02d", year, week) != period {
		return time.Time{}, errors.New("invalid model memory ISO week")
	}
	monday := startOfISOWeek(time.Date(year, 1, 4, 0, 0, 0, 0, time.UTC)).AddDate(0, 0, 7*(week-1))
	y, w := monday.ISOWeek()
	if y != year || w != week {
		return time.Time{}, errors.New("invalid model memory ISO week")
	}
	return monday, nil
}

// This adapter owns no model or filesystem capability. It exposes one already
// parsed batch to the original engine's layer-by-layer validation/commit path.
type modelWeeklySynthesis struct {
	input     ModelWeeklyInput
	candidate ModelWeeklyCandidate
}

func (s modelWeeklySynthesis) Synthesize(ctx context.Context, request SynthesisRequest) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if request.Cycle != "weekly" || request.WorkspaceID != s.input.WorkspaceID || request.Period != s.input.Period || len(request.Sources) == 0 {
		return "", errors.New("model memory synthesis outside its exact batch")
	}
	var expected []SourceDocument
	prior := ""
	switch request.TargetLayer {
	case "L2":
		prior = "L1"
	case "L3":
		prior = "L3"
	case "lifetime":
		prior = "lifetime"
	default:
		return "", errors.New("model memory cannot write another layer")
	}
	for _, artifact := range s.input.Sources {
		if artifact.Layer == prior {
			expected = append(expected, artifactSource(artifact))
		}
	}
	actual := request.Sources
	if request.TargetLayer != "L2" {
		var generated Artifact
		if json.Unmarshal(actual[0].Content, &generated) != nil || generated.WorkspaceID != s.input.WorkspaceID || generated.SynthesizerID != s.input.SynthesizerID || generated.Content != s.candidate.Content[map[string]string{"L3": "L2", "lifetime": "L3"}[request.TargetLayer]] {
			return "", errors.New("model memory intermediate provenance changed")
		}
		actual = actual[1:]
	}
	if len(actual) != len(expected) {
		return "", ErrCaptureConflict
	}
	for i := range expected {
		if actual[i].ID != expected[i].ID || !bytes.Equal(actual[i].Content, expected[i].Content) {
			return "", ErrCaptureConflict
		}
	}
	return s.candidate.Content[request.TargetLayer], nil
}
