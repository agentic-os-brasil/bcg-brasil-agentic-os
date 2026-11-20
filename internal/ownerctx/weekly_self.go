package ownerctx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

const weeklySelfMaximumStateBytes = 256 << 10

var (
	ErrWeeklySelfUnavailable = errors.New("weekly self core requires an owning publication boundary")
	ErrWeeklySelfChanged     = errors.New("weekly self selected sources or policy changed")
	ErrWeeklySelfExpired     = errors.New("weekly self input or working projection expired")
	ErrWeeklySelfBoundary    = errors.New("weekly self publication boundary did not execute exactly once synchronously")
	ErrWeeklySelfCompacted   = errors.New("weekly self occurrence already compacted")
)

// WeeklySelfLimits are hard ceilings, not permission to execute a model or job.
type WeeklySelfLimits struct {
	MaxPromptEntries        int   `json:"max_prompt_entries"`
	MaxPromptBytes          int   `json:"max_prompt_bytes"`
	MaxObservations         int   `json:"max_observations"`
	MaxInputBytes           int   `json:"max_input_bytes"`
	MaxEvidenceAgeSeconds   int64 `json:"max_evidence_age_seconds"`
	MaxWorkingFacetBytes    int   `json:"max_working_facet_bytes"`
	RecoverySeconds         int64 `json:"recovery_seconds"`
	WorkingRetentionSeconds int64 `json:"working_retention_seconds"`
}

func DefaultWeeklySelfLimits() WeeklySelfLimits {
	return WeeklySelfLimits{8, 16 << 10, 32, 32 << 10, 90 * 24 * 3600, 2000, 24 * 3600, 30 * 24 * 3600}
}

type WeeklySelfOptions struct {
	OccurrenceID             string           `json:"occurrence_id"`
	OwnerID                  string           `json:"owner_id"`
	ScheduledFor             time.Time        `json:"scheduled_for"`
	Facets                   []string         `json:"facets"`
	Limits                   WeeklySelfLimits `json:"limits"`
	WorkingLanguage          string           `json:"working_language"`
	TranslatorID             string           `json:"translator_id"`
	TranslatorVersion        string           `json:"translator_version"`
	SensitivePurposeSHA256   string           `json:"sensitive_purpose_sha256,omitempty"`
	SensitiveOwnerAuthorized bool             `json:"sensitive_owner_authorized,omitempty"`
}

type WeeklySelfPrompt struct {
	Source             PromptHistoryEntry `json:"source"`
	SourceRecordSHA256 string             `json:"source_record_sha256"`
	Normalized         string             `json:"normalized"`
	NormalizedSHA256   string             `json:"normalized_sha256"`
}

type WeeklySelfWatermark struct {
	Count      int    `json:"count"`
	LastSHA256 string `json:"last_sha256"`
}

// WeeklySelfInput is bounded private data. It is never a diagnostic or authority.
type WeeklySelfInput struct {
	PreviousWorking      *WeeklySelfWorking   `json:"previous_working,omitempty"`
	SchemaVersion        int                  `json:"schema_version"`
	ExpectedRevision     uint64               `json:"expected_revision"`
	Options              WeeklySelfOptions    `json:"options"`
	FrozenAt             time.Time            `json:"frozen_at"`
	RecoverUntil         time.Time            `json:"recover_until"`
	Facets               UserSelfSnapshot     `json:"facets"`
	PromptHistory        []WeeklySelfPrompt   `json:"prompt_history"`
	Observations         []ObservationReceipt `json:"observations"`
	ObservationSupport   map[string]string    `json:"observation_support"`
	HistoryWatermark     WeeklySelfWatermark  `json:"history_watermark"`
	ObservationWatermark WeeklySelfWatermark  `json:"observation_watermark"`
	PromptConfigSHA256   string               `json:"prompt_config_sha256"`
	PromptHistorySHA256  string               `json:"prompt_history_sha256"`
	PolicySHA256         string               `json:"policy_sha256"`
	TranslationSHA256    string               `json:"translation_sha256"`
	InputSHA256          string               `json:"input_sha256"`
}

type WeeklySelfCandidate struct {
	SchemaVersion       int               `json:"schema_version"`
	InputSHA256         string            `json:"input_sha256"`
	CanonicalSHA256     string            `json:"canonical_sha256"`
	PromptHistorySHA256 string            `json:"prompt_history_sha256"`
	PolicySHA256        string            `json:"policy_sha256"`
	TranslationSHA256   string            `json:"translation_sha256"`
	Facets              map[string]string `json:"facets"`
}

func (input WeeklySelfInput) Validate() error {
	if err := validatePreviousWeeklyWorking(input); err != nil {
		return err
	}
	if err := validateWeeklyDependencyBounds(sourceFromWeeklyInput(input)); err != nil {
		return err
	}
	if err := validateWeeklyOptions(input.Options, input.FrozenAt); err != nil {
		return err
	}
	if input.SchemaVersion != 1 || !input.RecoverUntil.Equal(input.FrozenAt.Add(time.Duration(input.Options.Limits.RecoverySeconds)*time.Second)) || input.Facets.Validate() != nil || input.PolicySHA256 != weeklyPolicyDigest(input.Options, input.Facets) || input.PromptHistorySHA256 != digestJSON(input.PromptHistory) || input.InputSHA256 != weeklyInputDigest(input) || weeklyJSONBytes(input) > input.Options.Limits.MaxInputBytes || len(input.PromptHistory) > input.Options.Limits.MaxPromptEntries || len(input.Observations) > input.Options.Limits.MaxObservations || !validDigest(input.PromptConfigSHA256) {
		return errors.New("weekly self frozen bindings invalid")
	}
	translation := digestJSON(struct {
		ID, Version, Language string
		History               []WeeklySelfPrompt
	}{input.Options.TranslatorID, input.Options.TranslatorVersion, input.Options.WorkingLanguage, input.PromptHistory})
	if input.TranslationSHA256 != translation {
		return errors.New("weekly self translation binding invalid")
	}
	if len(input.Facets.Facets) != len(input.Options.Facets) {
		return ErrWeeklySelfChanged
	}
	for _, id := range input.Options.Facets {
		facet, ok := input.Facets.Facets[id]
		if !ok || !containsWeekly(facet.Readers, "yoda") {
			return ErrWeeklySelfChanged
		}
		if facet.Sensitivity == "sensitive" && (!input.Options.SensitiveOwnerAuthorized || !validDigest(input.Options.SensitivePurposeSHA256)) {
			return ErrConfirmationRequired
		}
	}
	seen := map[string]bool{}
	total := 0
	for _, p := range input.PromptHistory {
		if seen[p.Source.ID] || validatePromptHistoryEntry(p.Source) != nil || p.SourceRecordSHA256 != digestJSON(p.Source) || p.Source.OwnerID != input.Options.OwnerID || p.Source.ScopeKind != PromptScopeGlobal || p.Source.ScopeID != "owner" || p.Source.RecordedAt.After(input.Options.ScheduledFor) || input.FrozenAt.Sub(p.Source.RecordedAt) > time.Duration(input.Options.Limits.MaxEvidenceAgeSeconds)*time.Second || strings.TrimSpace(p.Normalized) == "" || !utf8.ValidString(p.Normalized) || p.NormalizedSHA256 != digest(p.Normalized) || len(p.Normalized) > 4*len(p.Source.Prompt)+256 {
			return errors.New("weekly self selected prompt invalid")
		}
		seen[p.Source.ID] = true
		total += len(p.Source.Prompt) + len(p.Normalized)
	}
	if total > input.Options.Limits.MaxPromptBytes {
		return errors.New("weekly self selected prompt bytes exceeded")
	}
	seen = map[string]bool{}
	for _, o := range input.Observations {
		if seen[o.ID] || !IsYodaWeeklyEligible(o) || !containsWeekly(input.Options.Facets, o.Facet) || o.ScopeKind != "global" || o.ScopeID != "owner" || !validDigest(o.Revision) {
			return errors.New("weekly self selected observation invalid")
		}
		seen[o.ID] = true
	}
	for id, h := range input.ObservationSupport {
		if !observationIdentifier.MatchString(id) || !validDigest(h) {
			return errors.New("weekly self observation support invalid")
		}
	}
	return nil
}

// ParseWeeklySelfCandidate treats the whole model response as one closed object.
func ParseWeeklySelfCandidate(input WeeklySelfInput, body []byte) (WeeklySelfCandidate, error) {
	var candidate WeeklySelfCandidate
	if err := input.Validate(); err != nil {
		return candidate, err
	}
	if len(body) == 0 || len(body) > weeklySelfMaximumStateBytes {
		return candidate, errors.New("weekly candidate response exceeds bound")
	}
	if err := decodePrivateJSON(body, &candidate); err != nil {
		return candidate, err
	}
	return candidate, validateWeeklyCandidate(input, candidate)
}

// WeeklySelfReceipt contains no source or working text.
type WeeklySelfReceipt struct {
	State         string `json:"state"`
	Revision      uint64 `json:"revision"`
	OccurrenceID  string `json:"occurrence_id"`
	InputSHA256   string `json:"input_sha256"`
	WorkingSHA256 string `json:"working_sha256"`
	Verified      bool   `json:"verified"`
}

type weeklySelfSource struct {
	Options             WeeklySelfOptions `json:"options"`
	FacetDigests        map[string]string `json:"facet_digests"`
	Prompts             map[string]string `json:"prompts"`
	Observations        map[string]string `json:"observations"`
	ObservationSupport  map[string]string `json:"observation_support"`
	PromptConfigSHA256  string            `json:"prompt_config_sha256"`
	CanonicalSHA256     string            `json:"canonical_sha256"`
	PromptHistorySHA256 string            `json:"prompt_history_sha256"`
	PolicySHA256        string            `json:"policy_sha256"`
	TranslationSHA256   string            `json:"translation_sha256"`
}

type WeeklySelfWorking struct {
	Execution *WeeklySelfExecution `json:"execution,omitempty"`
	Receipt   WeeklySelfReceipt    `json:"receipt"`
	Facets    map[string]string    `json:"facets"`
	ExpiresAt time.Time            `json:"expires_at"`
	Source    weeklySelfSource     `json:"source"`
}

type weeklySelfState struct {
	SchemaVersion    int                `json:"schema_version"`
	OwnerID          string             `json:"owner_id"`
	Revision         uint64             `json:"revision"`
	LastScheduledFor time.Time          `json:"last_scheduled_for"`
	LastOccurrenceID string             `json:"last_occurrence_id"`
	LastInputSHA256  string             `json:"last_input_sha256"`
	Pending          *weeklySelfPending `json:"pending,omitempty"`
	Working          *WeeklySelfWorking `json:"working,omitempty"`
}

// The pending manifest reconstructs originals from their owners instead of caching raw SELF/prompts.
type weeklySelfPending struct {
	Input          WeeklySelfInput `json:"input"`
	ManifestSHA256 string          `json:"manifest_sha256"`
}

func weeklyManifestFromInput(input WeeklySelfInput) (weeklySelfPending, error) {
	var copy WeeklySelfInput
	body, err := json.Marshal(input)
	if err != nil {
		return weeklySelfPending{}, err
	}
	if err := json.Unmarshal(body, &copy); err != nil {
		return weeklySelfPending{}, err
	}
	for id, facet := range copy.Facets.Facets {
		facet.Content = ""
		copy.Facets.Facets[id] = facet
	}
	for index, prompt := range copy.PromptHistory {
		if prompt.Normalized == prompt.Source.Prompt {
			prompt.Normalized = ""
		}
		prompt.Source.Prompt = ""
		copy.PromptHistory[index] = prompt
	}
	if copy.PreviousWorking != nil {
		copy.PreviousWorking.Facets = nil
	}
	return weeklySelfPending{copy, digestJSON(copy)}, nil
}

func (pending weeklySelfPending) Validate() error {
	if pending.Input.PreviousWorking != nil && len(pending.Input.PreviousWorking.Facets) != 0 {
		return errors.New("weekly manifest duplicates previous working text")
	}
	input := pending.Input
	if err := validateWeeklyOptions(input.Options, input.FrozenAt); err != nil {
		return err
	}
	if input.SchemaVersion != 1 || !validDigest(input.InputSHA256) || pending.ManifestSHA256 != digestJSON(input) || weeklyJSONBytes(pending) > input.Options.Limits.MaxInputBytes+1024 {
		return errors.New("weekly reconstruction manifest invalid")
	}
	for _, facet := range input.Facets.Facets {
		if facet.Content != "" {
			return errors.New("weekly manifest contains duplicate canonical content")
		}
	}
	for _, prompt := range input.PromptHistory {
		if prompt.Source.Prompt != "" || !validDigest(prompt.SourceRecordSHA256) {
			return errors.New("weekly manifest contains duplicate raw prompt or invalid source")
		}
	}
	return nil
}

func hydrateWeeklySelfInput(manifest WeeklySelfInput, sources weeklySelfSources, previous *WeeklySelfWorking) (WeeklySelfInput, error) {
	var input WeeklySelfInput
	body, err := json.Marshal(manifest)
	if err != nil {
		return input, err
	}
	if err := json.Unmarshal(body, &input); err != nil {
		return input, err
	}
	if err := validateWeeklySource(sourceFromWeeklyInput(manifest), sources, input.FrozenAt); err != nil {
		return input, err
	}
	for id := range input.Facets.Facets {
		input.Facets.Facets[id] = sources.facets.Facets[id]
	}
	history := map[string]PromptHistoryEntry{}
	for _, entry := range sources.history {
		history[entry.ID] = entry
	}
	for index, prompt := range input.PromptHistory {
		entry, ok := history[prompt.Source.ID]
		if !ok || digestJSON(entry) != prompt.SourceRecordSHA256 {
			return input, ErrWeeklySelfChanged
		}
		prompt.Source = entry
		if prompt.Normalized == "" {
			prompt.Normalized = entry.Prompt
		}
		input.PromptHistory[index] = prompt
	}
	if err := hydratePreviousWeeklyWorking(&input, previous); err != nil {
		return input, err
	}
	return input, input.Validate()
}

type weeklySelfSources struct {
	facets       UserSelfSnapshot
	history      []PromptHistoryEntry
	config       PromptHistoryConfig
	observations []observationRecord
}

// FreezeWeeklySelf selects once. A replay reuses the stored bytes, never newer events.
func FreezeWeeklySelf(root string, options WeeklySelfOptions, normalize func(string, string, string) (string, error), now time.Time) (result WeeklySelfInput, err error) {
	defer func() {
		if err != nil {
			result = WeeklySelfInput{}
		}
	}()
	options.Facets = append([]string(nil), options.Facets...)
	sort.Strings(options.Facets)
	if err = validateWeeklyOptions(options, now); err != nil {
		return result, err
	}
	err = withWeeklySelfSources(root, options, func(sources weeklySelfSources) error {
		return withWeeklySelfState(root, true, func(state *weeklySelfState) error {
			if state.OwnerID != "" && state.OwnerID != options.OwnerID {
				return ErrWeeklySelfChanged
			}
			if state.Working != nil && state.Working.Receipt.OccurrenceID == options.OccurrenceID {
				return ErrWeeklySelfCompacted
			}
			if state.Pending != nil {
				if state.Pending.Input.Options.OccurrenceID != options.OccurrenceID || !reflect.DeepEqual(state.Pending.Input.Options, options) {
					return ErrWeeklySelfChanged
				}
				if !now.Before(state.Pending.Input.RecoverUntil) {
					return ErrWeeklySelfExpired
				}
				if err := validateWeeklySource(sourceFromWeeklyInput(state.Pending.Input), sources, now); err != nil {
					return err
				}
				var hydrateErr error
				result, hydrateErr = hydrateWeeklySelfInput(state.Pending.Input, sources, state.Working)
				return hydrateErr
			}
			if !state.LastScheduledFor.IsZero() && (state.LastOccurrenceID == options.OccurrenceID || !options.ScheduledFor.After(state.LastScheduledFor)) {
				return ErrWeeklySelfChanged
			}
			input, buildErr := buildWeeklySelfInput(options, sources, normalize, now, state.Revision, state.Working)
			if buildErr != nil {
				return buildErr
			}
			manifest, manifestErr := weeklyManifestFromInput(input)
			if manifestErr != nil {
				return manifestErr
			}
			state.SchemaVersion, state.OwnerID, state.Pending = 1, options.OwnerID, &manifest
			state.LastScheduledFor, state.LastOccurrenceID, state.LastInputSHA256 = options.ScheduledFor, options.OccurrenceID, input.InputSHA256
			if err := writeWeeklySelfState(root, *state); err != nil {
				return err
			}
			result = input
			return nil
		})
	})
	return result, err
}

// ReadWeeklySelf is an explicit Yoda working-view reader; normal session readers are unchanged.
func ReadWeeklySelf(root string, options WeeklySelfOptions, now time.Time) (WeeklySelfWorking, error) {
	return readWeeklySelf(root, options, now, nil)
}
func readWeeklySelf(root string, options WeeklySelfOptions, now time.Time, visit func(WeeklySelfWorking) error) (result WeeklySelfWorking, err error) {
	defer func() {
		if err != nil {
			result = WeeklySelfWorking{}
		}
	}()
	options.Facets = append([]string(nil), options.Facets...)
	sort.Strings(options.Facets)
	if err = validateWeeklyOptions(options, now); err != nil {
		return result, err
	}
	err = withWeeklySelfSources(root, options, func(sources weeklySelfSources) error {
		return withWeeklySelfState(root, false, func(state *weeklySelfState) error {
			if state.OwnerID != options.OwnerID || state.Working == nil {
				return os.ErrNotExist
			}
			working := *state.Working
			if !sameWeeklyReadPolicy(options, working.Source.Options) {
				return ErrWeeklySelfChanged
			}
			if !now.Before(working.ExpiresAt) {
				return ErrWeeklySelfExpired
			}
			if err := validateWeeklySource(working.Source, sources, now); err != nil {
				return err
			}
			result = working
			result.Receipt.Verified = true
			if visit != nil {
				return visit(result)
			}
			return nil
		})
	})
	return result, err
}

// PublishWeeklySelf accepts no authority record. The caller must supply its existing synchronous owning fence.
func PublishWeeklySelf(ctx context.Context, root, inputSHA256 string, candidate WeeklySelfCandidate, expectedRevision uint64, commit func(func() error) error, now time.Time) (WeeklySelfReceipt, error) {
	return publishWeeklySelf(ctx, root, inputSHA256, candidate, expectedRevision, commit, now, nil)
}
func publishWeeklySelf(ctx context.Context, root, inputSHA256 string, candidate WeeklySelfCandidate, expectedRevision uint64, commit func(func() error) error, now time.Time, execution *WeeklySelfExecution) (result WeeklySelfReceipt, err error) {
	if commit == nil {
		return result, ErrWeeklySelfUnavailable
	}
	if !validDigest(inputSHA256) || candidate.InputSHA256 != inputSHA256 {
		return result, ErrWeeklySelfChanged
	}
	var window sync.Mutex
	var closed atomic.Bool
	open, invoked, invalid, durable := true, false, false, false
	defer func() { closed.Store(true); window.Lock(); open = false; window.Unlock() }()
	var owningErr error
	var publishedOptions WeeklySelfOptions
	write := func() (writeErr error) {
		window.Lock()
		defer window.Unlock()
		defer func() { owningErr = errors.Join(owningErr, writeErr) }()
		if closed.Load() || !open || invoked {
			invalid = true
			return ErrWeeklySelfBoundary
		}
		invoked = true
		if err := ctx.Err(); err != nil {
			return err
		}
		state, readErr := readWeeklySelfState(root)
		if readErr != nil {
			return readErr
		}
		var options WeeklySelfOptions
		if state.Pending != nil {
			options = state.Pending.Input.Options
		} else if state.Working != nil {
			options = state.Working.Source.Options
		} else {
			return ErrWeeklySelfChanged
		}
		publishedOptions = options
		return withWeeklySelfSources(root, options, func(sources weeklySelfSources) error {
			return withWeeklySelfState(root, false, func(current *weeklySelfState) error {
				if closed.Load() {
					return ErrWeeklySelfBoundary
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				if current.OwnerID != options.OwnerID {
					return ErrWeeklySelfChanged
				}
				if current.Pending == nil {
					if current.Working == nil || current.Working.Receipt.InputSHA256 != inputSHA256 {
						return ErrWeeklySelfChanged
					}
					if !now.Before(current.Working.ExpiresAt) {
						return ErrWeeklySelfExpired
					}
					if err := validateWeeklySource(current.Working.Source, sources, now); err != nil {
						return err
					}
					if weeklyCandidateDigest(candidate) != current.Working.Receipt.WorkingSHA256 {
						return ErrWeeklySelfChanged
					}
					if execution != nil && digestJSON(current.Working.Execution) != digestJSON(execution) {
						return ErrWeeklySelfChanged
					}
					result = current.Working.Receipt
					result.Verified = true
					return nil
				}
				input, hydrateErr := hydrateWeeklySelfInput(current.Pending.Input, sources, current.Working)
				if hydrateErr != nil {
					return hydrateErr
				}
				if input.InputSHA256 != inputSHA256 || current.Revision != expectedRevision || input.ExpectedRevision != expectedRevision {
					return ErrRevisionConflict
				}
				if !now.Before(input.RecoverUntil) {
					return ErrWeeklySelfExpired
				}
				if execution != nil {
					if err := execution.validateSource(input.Options); err != nil {
						return err
					}
				}
				if err := validateWeeklyCandidate(input, candidate); err != nil {
					return err
				}
				if err := validateWeeklySource(sourceFromWeeklyInput(input), sources, now); err != nil {
					return err
				}
				if current.Working != nil {
					for facet := range current.Working.Facets {
						if _, ok := candidate.Facets[facet]; !ok {
							return errors.New("weekly compaction may not drop an existing policy-bound facet")
						}
					}
				}
				if err := ctx.Err(); err != nil {
					return err
				}
				result = WeeklySelfReceipt{"committed", current.Revision + 1, options.OccurrenceID, inputSHA256, weeklyCandidateDigest(candidate), false}
				working := WeeklySelfWorking{Receipt: result, Facets: candidate.Facets, ExpiresAt: now.Add(time.Duration(options.Limits.WorkingRetentionSeconds) * time.Second), Source: sourceFromWeeklyInput(input), Execution: execution}
				current.Revision, current.Pending, current.Working = result.Revision, nil, &working
				if closed.Load() {
					return ErrWeeklySelfBoundary
				}
				if err := writeWeeklySelfState(root, *current); err != nil {
					return err
				}
				durable = true
				stored, err := readWeeklySelfState(root)
				if err != nil {
					return err
				}
				if stored.Working == nil || stored.Revision != result.Revision || stored.Working.Receipt.WorkingSHA256 != result.WorkingSHA256 {
					return ErrWeeklySelfChanged
				}
				result.Verified = true
				if closed.Load() {
					return ErrWeeklySelfBoundary
				}
				return nil
			})
		})
	}
	boundaryErr := commit(write)
	closed.Store(true)
	window.Lock()
	open = false
	boundaryErr = errors.Join(boundaryErr, owningErr)
	if !invoked || invalid {
		boundaryErr = errors.Join(boundaryErr, ErrWeeklySelfBoundary)
	}
	if ctx.Err() != nil {
		boundaryErr = errors.Join(boundaryErr, ctx.Err())
	}
	if boundaryErr != nil && durable {
		result.State, result.Verified = "partial", false
	}
	window.Unlock()
	if boundaryErr == nil && result.Revision > 0 {
		working, readErr := ReadWeeklySelf(root, publishedOptions, now)
		if readErr != nil || working.Receipt.Revision != result.Revision || working.Receipt.WorkingSHA256 != result.WorkingSHA256 {
			boundaryErr = errors.Join(readErr, ErrWeeklySelfChanged)
			result.Verified = false
			if durable {
				result.State = "partial"
			}
		}
	}
	return result, boundaryErr
}

// PruneWeeklySelf is an explicit owned retention operation; read APIs never purge or repair.
func PruneWeeklySelf(root string, options WeeklySelfOptions, now time.Time) (bool, error) {
	options.Facets = append([]string(nil), options.Facets...)
	sort.Strings(options.Facets)
	if err := validateWeeklyOptions(options, now); err != nil {
		return false, err
	}
	changed := false
	err := withWeeklySelfOwner(root, func() error {
		var sources weeklySelfSources
		sourceErr := weeklySelfSourcesLocked(root, options, func(value weeklySelfSources) error { sources = value; return nil })
		return withWeeklySelfState(root, false, func(state *weeklySelfState) error {
			if state.OwnerID != options.OwnerID {
				return ErrWeeklySelfChanged
			}
			dirty := false
			if state.Pending != nil && (!now.Before(state.Pending.Input.RecoverUntil) || (sourceErr == nil && validateWeeklySource(sourceFromWeeklyInput(state.Pending.Input), sources, now) != nil)) {
				state.Pending = nil
				dirty = true
			}
			if state.Working != nil && (!now.Before(state.Working.ExpiresAt) || (sourceErr == nil && validateWeeklySource(state.Working.Source, sources, now) != nil)) {
				state.Working = nil
				dirty = true
			}
			if dirty {
				if err := writeWeeklySelfState(root, *state); err != nil {
					return err
				}
				changed = true
			}
			if sourceErr != nil && (state.Pending != nil || state.Working != nil) {
				return sourceErr
			}
			return nil
		})
	})
	return changed, err
}

func validateWeeklyOptions(o WeeklySelfOptions, now time.Time) error {
	l := o.Limits
	if now.IsZero() || !validDigest(o.OccurrenceID) || !observationIdentifier.MatchString(o.OwnerID) || o.ScheduledFor.IsZero() || o.ScheduledFor.After(now) || len(o.Facets) == 0 || len(o.Facets) > len(facets) || !promptLanguagePattern.MatchString(o.WorkingLanguage) || !observationIdentifier.MatchString(o.TranslatorID) || !observationIdentifier.MatchString(o.TranslatorVersion) {
		return errors.New("invalid weekly self selection")
	}
	for i, id := range o.Facets {
		if _, ok := facets[id]; !ok || (i > 0 && id <= o.Facets[i-1]) {
			return errors.New("weekly self facets must be known and unique")
		}
	}
	if l.MaxPromptEntries < 1 || l.MaxPromptEntries > 8 || l.MaxPromptBytes < 1 || l.MaxPromptBytes > 32<<10 || l.MaxObservations < 1 || l.MaxObservations > 32 || l.MaxInputBytes < 1 || l.MaxInputBytes > 64<<10 || l.MaxEvidenceAgeSeconds < 3600 || l.MaxEvidenceAgeSeconds > 90*24*3600 || l.MaxWorkingFacetBytes < 1 || l.MaxWorkingFacetBytes > maximumOwnerFacetBytes || l.RecoverySeconds < 1 || l.RecoverySeconds > 7*24*3600 || l.WorkingRetentionSeconds < 1 || l.WorkingRetentionSeconds > 90*24*3600 {
		return errors.New("weekly self limits are invalid")
	}
	return nil
}

func withWeeklySelfOwner(root string, operation func() error) (err error) {
	unlock, err := acquireOwnerTransitionFile(filepath.Join(root, "owner/interview/.transition.lock"), false)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	if _, err := readRegistry(root); err != nil {
		return err
	}
	return operation()
}

func withWeeklySelfSources(root string, options WeeklySelfOptions, operation func(weeklySelfSources) error) error {
	return withWeeklySelfOwner(root, func() error { return weeklySelfSourcesLocked(root, options, operation) })
}

func weeklySelfSourcesLocked(root string, options WeeklySelfOptions, operation func(weeklySelfSources) error) error {
	snapshot, err := ProjectSnapshot(root, options.Facets)
	if err != nil {
		return err
	}
	for _, facet := range snapshot.Facets {
		if !containsWeekly(facet.Readers, "yoda") || !facetAnswered(root, facet.Facet) {
			return errors.New("weekly self facet is unreadable or unanswered")
		}
		if facet.Sensitivity == "sensitive" && (!options.SensitiveOwnerAuthorized || !validDigest(options.SensitivePurposeSHA256)) {
			return ErrConfirmationRequired
		}
	}
	return withWeeklyHistorySnapshot(root, func(history []PromptHistoryEntry, config PromptHistoryConfig) error {
		if config.OwnerID != "" && config.OwnerID != options.OwnerID {
			return ErrWeeklySelfChanged
		}
		return withObservationReadSnapshot(root, func(records []observationRecord) error {
			return operation(weeklySelfSources{snapshot, history, config, records})
		})
	})
}

func withWeeklyHistorySnapshot(root string, operation func([]PromptHistoryEntry, PromptHistoryConfig) error) (err error) {
	entries, config, err := promptHistoryPaths(root)
	if err != nil {
		return err
	}
	if err := scheduler.ValidatePrivateDirectory(filepath.Dir(entries)); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		// The absent history is read-only. Validate its existing ancestry with
		// the same portable owner namespace policy without preparing state.
		for ancestor := filepath.Dir(entries); ; ancestor = filepath.Dir(ancestor) {
			if _, lookupErr := os.Lstat(ancestor); lookupErr == nil {
				if err := scheduler.ValidateDirectory(ancestor); err != nil {
					return err
				}
				return operation(nil, DefaultPromptHistoryConfig())
			} else if !errors.Is(lookupErr, os.ErrNotExist) {
				return lookupErr
			}
		}
	}
	unlock, err := acquireExistingOwnerTransition(filepath.Join(filepath.Dir(entries), promptHistoryLockName))
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	if _, err := readOwnerFile(filepath.Join(filepath.Dir(entries), promptHistoryPending), maximumHistoryJournalBytes); err == nil {
		return ErrPromptHistoryRecovery
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	cfg, err := loadPromptHistoryConfig(config)
	if err != nil {
		return err
	}
	records, err := readPromptHistoryEntries(entries)
	if err != nil {
		return err
	}
	if err := validateHistoryPair(promptHistoryPair{cfg, records}); err != nil {
		return err
	}
	return operation(records, cfg)
}

func buildWeeklySelfInput(o WeeklySelfOptions, sources weeklySelfSources, normalize func(string, string, string) (string, error), now time.Time, revision uint64, previous ...*WeeklySelfWorking) (WeeklySelfInput, error) {
	input := WeeklySelfInput{SchemaVersion: 1, ExpectedRevision: revision, Options: o, FrozenAt: now, RecoverUntil: now.Add(time.Duration(o.Limits.RecoverySeconds) * time.Second), Facets: sources.facets, PromptHistory: []WeeklySelfPrompt{}, Observations: []ObservationReceipt{}, ObservationSupport: map[string]string{}, PromptConfigSHA256: digestJSON(sources.config)}
	input.PolicySHA256 = weeklyPolicyDigest(o, sources.facets)
	if len(previous) > 0 {
		if err := attachPreviousWeeklyWorking(&input, sources, previous[0]); err != nil {
			return input, err
		}
	}
	input.HistoryWatermark = WeeklySelfWatermark{Count: len(sources.history), LastSHA256: digestJSON(sources.history)}
	input.ObservationWatermark = WeeklySelfWatermark{Count: len(sources.observations), LastSHA256: digestJSON(sources.observations)}
	history := append([]PromptHistoryEntry(nil), sources.history...)
	sort.Slice(history, func(i, j int) bool {
		if history[i].RecordedAt.Equal(history[j].RecordedAt) {
			return history[i].ID < history[j].ID
		}
		return history[i].RecordedAt.After(history[j].RecordedAt)
	})
	bytes := 0
	for _, entry := range history {
		if entry.OwnerID != o.OwnerID || entry.ScopeKind != PromptScopeGlobal || entry.ScopeID != "owner" || entry.RecordedAt.After(o.ScheduledFor) || now.Sub(entry.RecordedAt) > time.Duration(o.Limits.MaxEvidenceAgeSeconds)*time.Second {
			continue
		}
		if len(input.PromptHistory) >= o.Limits.MaxPromptEntries {
			break
		}
		if bytes+len(entry.Prompt) > o.Limits.MaxPromptBytes {
			return input, errors.New("weekly selected prompt bytes exceed policy")
		}
		input.PromptHistory = append(input.PromptHistory, WeeklySelfPrompt{Source: entry, SourceRecordSHA256: digestJSON(entry)})
		bytes += len(entry.Prompt)
	}

	observations := latestObservations(sources.observations)
	ids := make([]string, 0, len(observations))
	for id := range observations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		record := observations[id]
		receipt := observationReceipt(record, "")
		if !containsWeekly(o.Facets, receipt.Facet) || receipt.ScopeKind != "global" || receipt.ScopeID != "owner" || !IsYodaWeeklyEligible(receipt) || record.RecordedAt.After(o.ScheduledFor) || !record.ExpiresAt.After(now) || now.Sub(record.RecordedAt) > time.Duration(o.Limits.MaxEvidenceAgeSeconds)*time.Second {
			continue
		}
		if len(input.Observations) >= o.Limits.MaxObservations {
			return input, errors.New("weekly selected observations exceed policy")
		}
		cohort := []observationRecord{}
		for _, witness := range observations {
			if witness.ScopeKind == "global" && witness.ScopeID == "owner" && witness.ExpiresAt.After(now) && !witness.RecordedAt.After(o.ScheduledFor) && now.Sub(witness.RecordedAt) <= time.Duration(o.Limits.MaxEvidenceAgeSeconds)*time.Second && witness.State != ObservationExpired {
				cohort = append(cohort, witness)
			}
		}
		if err := validateTransition(record, ObservationCorroborated, cohort); err != nil {
			continue
		}
		for _, witness := range cohort {
			if witness.ID != record.ID && witness.EpisodeID != record.EpisodeID && witness.Facet == record.Facet && witness.Claim == record.Claim && witness.SourceDigest != record.SourceDigest && witness.State != ObservationRejected && witness.State != ObservationRedacted && witness.State != ObservationContradicted {
				input.ObservationSupport[witness.ID] = observationRevision(witness)
			}
		}
		input.Observations = append(input.Observations, receipt)
	}
	// Reserve the closed digest fields before any translation callback receives private text.
	if weeklyJSONBytes(input)+64*(len(input.PromptHistory)+3) > o.Limits.MaxInputBytes {
		return input, errors.New("weekly complete raw input exceeds policy")
	}
	for index, prompt := range input.PromptHistory {
		if normalize == nil {
			return input, errors.New("weekly prompt normalizer unavailable")
		}
		text, err := normalize(prompt.Source.Prompt, prompt.Source.Language, o.WorkingLanguage)
		if err != nil || strings.TrimSpace(text) == "" || !utf8.ValidString(text) || strings.ContainsRune(text, 0) || len(text) > 4*len(prompt.Source.Prompt)+256 || bytes+len(text) > o.Limits.MaxPromptBytes {
			return input, errors.New("weekly prompt normalization exceeds policy")
		}
		prompt.Normalized, prompt.NormalizedSHA256 = text, digest(text)
		input.PromptHistory[index] = prompt
		bytes += len(text)
		if weeklyJSONBytes(input)+64*(len(input.PromptHistory)-index-1+3) > o.Limits.MaxInputBytes {
			return input, errors.New("weekly complete normalized input exceeds policy")
		}
	}

	input.PromptHistorySHA256 = digestJSON(input.PromptHistory)
	input.TranslationSHA256 = digestJSON(struct {
		ID, Version, Language string
		History               []WeeklySelfPrompt
	}{o.TranslatorID, o.TranslatorVersion, o.WorkingLanguage, input.PromptHistory})
	input.InputSHA256 = weeklyInputDigest(input)
	return input, input.Validate()
}

func sourceFromWeeklyInput(input WeeklySelfInput) weeklySelfSource {
	s := weeklySelfSource{Options: input.Options, FacetDigests: map[string]string{}, Prompts: map[string]string{}, Observations: map[string]string{}, ObservationSupport: map[string]string{}, PromptConfigSHA256: input.PromptConfigSHA256, CanonicalSHA256: input.Facets.CanonicalSourceDigest, PromptHistorySHA256: input.PromptHistorySHA256, PolicySHA256: input.PolicySHA256, TranslationSHA256: input.TranslationSHA256}
	if input.PreviousWorking != nil {
		for id, digest := range input.PreviousWorking.Source.Prompts {
			s.Prompts[id] = digest
		}
		for id, digest := range input.PreviousWorking.Source.Observations {
			s.Observations[id] = digest
		}
		for id, digest := range input.PreviousWorking.Source.ObservationSupport {
			s.ObservationSupport[id] = digest
		}
	}
	for id, digest := range input.ObservationSupport {
		s.ObservationSupport[id] = digest
	}
	for id, f := range input.Facets.Facets {
		s.FacetDigests[id] = f.ContentDigest
	}
	for _, p := range input.PromptHistory {
		s.Prompts[p.Source.ID] = p.SourceRecordSHA256
	}
	for _, o := range input.Observations {
		s.Observations[o.ID] = o.Revision
	}
	return s
}

func validateWeeklySource(binding weeklySelfSource, current weeklySelfSources, now time.Time) error {
	if binding.CanonicalSHA256 != current.facets.CanonicalSourceDigest || binding.PromptConfigSHA256 != digestJSON(current.config) || binding.PolicySHA256 != weeklyPolicyDigest(binding.Options, current.facets) || len(binding.FacetDigests) != len(current.facets.Facets) {
		return ErrWeeklySelfChanged
	}
	for id, h := range binding.FacetDigests {
		if current.facets.Facets[id].ContentDigest != h {
			return ErrWeeklySelfChanged
		}
	}
	history := map[string]PromptHistoryEntry{}
	for _, p := range current.history {
		history[p.ID] = p
	}
	for id, h := range binding.Prompts {
		p, ok := history[id]
		if !ok || digestJSON(p) != h || p.OwnerID != binding.Options.OwnerID || p.ScopeKind != PromptScopeGlobal || p.ScopeID != "owner" || now.Sub(p.RecordedAt) > time.Duration(binding.Options.Limits.MaxEvidenceAgeSeconds)*time.Second {
			return ErrWeeklySelfChanged
		}
	}
	observations := latestObservations(current.observations)
	for id, h := range binding.Observations {
		record, ok := observations[id]
		o := observationReceipt(record, "")
		if !ok || o.Revision != h || !IsYodaWeeklyEligible(o) || !record.ExpiresAt.After(now) || now.Sub(record.RecordedAt) > time.Duration(binding.Options.Limits.MaxEvidenceAgeSeconds)*time.Second {
			return ErrWeeklySelfChanged
		}
	}
	for id, h := range binding.ObservationSupport {
		record, ok := observations[id]
		if !ok || observationRevision(record) != h || record.ScopeKind != "global" || record.ScopeID != "owner" || !record.ExpiresAt.After(now) || record.State == ObservationRejected || record.State == ObservationRedacted || record.State == ObservationContradicted || record.State == ObservationExpired || now.Sub(record.RecordedAt) > time.Duration(binding.Options.Limits.MaxEvidenceAgeSeconds)*time.Second {
			return ErrWeeklySelfChanged
		}
	}
	return nil
}

func validateWeeklyCandidate(input WeeklySelfInput, c WeeklySelfCandidate) error {
	if err := input.Validate(); err != nil {
		return err
	}
	if c.SchemaVersion != 1 || input.InputSHA256 != weeklyInputDigest(input) || c.InputSHA256 != input.InputSHA256 || c.CanonicalSHA256 != input.Facets.CanonicalSourceDigest || c.PromptHistorySHA256 != input.PromptHistorySHA256 || c.PolicySHA256 != input.PolicySHA256 || c.TranslationSHA256 != input.TranslationSHA256 || len(c.Facets) != len(input.Facets.Facets) {
		return ErrWeeklySelfChanged
	}
	for id, facet := range input.Facets.Facets {
		text, ok := c.Facets[id]
		if !ok || strings.TrimSpace(text) == "" || !utf8.ValidString(text) || strings.ContainsRune(text, 0) || len(text) > input.Options.Limits.MaxWorkingFacetBytes || !containsWeekly(facet.Readers, "yoda") {
			return errors.New("weekly candidate facet is missing, unknown or oversized")
		}
	}
	return nil
}

func weeklyPolicyDigest(o WeeklySelfOptions, snapshot UserSelfSnapshot) string {
	policies := map[string]FacetPolicy{}
	for id := range snapshot.Facets {
		p, err := snapshot.Policy(id)
		if err != nil {
			return ""
		}
		policies[id] = p
	}
	o.OccurrenceID = ""
	o.ScheduledFor = time.Time{}
	return digestJSON(struct {
		Options  WeeklySelfOptions
		Policies map[string]FacetPolicy
	}{o, policies})
}

func sameWeeklyReadPolicy(a, b WeeklySelfOptions) bool {
	a.OccurrenceID = ""
	b.OccurrenceID = ""
	a.ScheduledFor = time.Time{}
	b.ScheduledFor = time.Time{}
	return reflect.DeepEqual(a, b)
}
func weeklyInputDigest(input WeeklySelfInput) string {
	input.InputSHA256 = ""
	return digestJSON(input)
}
func weeklyCandidateDigest(c WeeklySelfCandidate) string { return digestJSON(c) }
func weeklyJSONBytes(value any) int {
	b, err := json.Marshal(value)
	if err != nil {
		return weeklySelfMaximumStateBytes + 1
	}
	return len(b)
}

func containsWeekly(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
func weeklySelfPath(root string) string { return filepath.Join(root, "owner/self/weekly/current.json") }

func withWeeklySelfState(root string, create bool, operation func(*weeklySelfState) error) (err error) {
	unlock, err := acquireOwnerTransitionFile(filepath.Join(root, "owner/self/weekly/.lock"), create)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	state, err := readWeeklySelfState(root)
	if errors.Is(err, os.ErrNotExist) && create {
		state = weeklySelfState{SchemaVersion: 1}
		err = nil
	}
	if err != nil {
		return err
	}
	return operation(&state)
}

func readWeeklySelfState(root string) (weeklySelfState, error) {
	var state weeklySelfState
	b, err := readOwnerFile(weeklySelfPath(root), weeklySelfMaximumStateBytes)
	if err != nil {
		return state, err
	}
	if err := decodePrivateJSON(b, &state); err != nil {
		return state, err
	}
	if state.SchemaVersion != 1 || !observationIdentifier.MatchString(state.OwnerID) {
		return state, errors.New("weekly self state invalid")
	}
	if state.LastScheduledFor.IsZero() || !validDigest(state.LastOccurrenceID) || !validDigest(state.LastInputSHA256) {
		return state, errors.New("weekly occurrence watermark invalid")
	}
	if state.Pending != nil && (state.Pending.Input.Options.OwnerID != state.OwnerID || state.Pending.Input.ExpectedRevision != state.Revision || state.Pending.Validate() != nil) {
		return state, errors.New("weekly frozen input invalid")
	}
	if state.Pending != nil && (state.Pending.Input.Options.OccurrenceID != state.LastOccurrenceID || state.Pending.Input.InputSHA256 != state.LastInputSHA256 || !state.Pending.Input.Options.ScheduledFor.Equal(state.LastScheduledFor)) {
		return state, errors.New("weekly pending occurrence watermark differs")
	}
	if state.Working != nil {
		w := state.Working
		candidate := WeeklySelfCandidate{SchemaVersion: 1, InputSHA256: w.Receipt.InputSHA256, CanonicalSHA256: w.Source.CanonicalSHA256, PromptHistorySHA256: w.Source.PromptHistorySHA256, PolicySHA256: w.Source.PolicySHA256, TranslationSHA256: w.Source.TranslationSHA256, Facets: w.Facets}
		if w.Execution != nil && w.Execution.validateSource(w.Source.Options) != nil {
			return state, errors.New("weekly execution metadata invalid")
		}
		if !validDigest(w.Receipt.InputSHA256) || !validDigest(w.Receipt.OccurrenceID) || !validDigest(w.Receipt.WorkingSHA256) || w.Receipt.OccurrenceID != w.Source.Options.OccurrenceID || w.Receipt.Revision == 0 || w.Receipt.State != "committed" || w.Receipt.Revision != state.Revision || w.Source.Options.OwnerID != state.OwnerID || w.Receipt.WorkingSHA256 != weeklyCandidateDigest(candidate) || w.ExpiresAt.IsZero() {
			return state, errors.New("weekly working projection invalid")
		}
	}
	return state, nil
}

func writeWeeklySelfState(root string, state weeklySelfState) error {
	body, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if len(body)+1 > weeklySelfMaximumStateBytes {
		return fmt.Errorf("weekly self state exceeds %d bytes", weeklySelfMaximumStateBytes)
	}
	return publishOwnerFile(weeklySelfPath(root), append(body, '\n'), false)
}
