package memory

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
)

// Policy is the runtime-neutral contract for memory persistence and context injection.
type Policy struct {
	SchemaVersion    int                    `json:"schema_version"`
	Storage          StoragePolicy          `json:"storage"`
	Layers           []Layer                `json:"layers"`
	Lifetime         LifetimePolicy         `json:"lifetime"`
	Cycles           DreamingCycles         `json:"cycles"`
	Dreaming         DreamingPolicy         `json:"dreaming"`
	ContextInjection ContextInjectionPolicy `json:"context_injection"`
}

type StoragePolicy struct {
	Scope                       string `json:"scope"`
	WorkspaceIsolation          bool   `json:"workspace_isolation"`
	ManagedCoreContainsUserData bool   `json:"managed_core_contains_user_data"`
}

type Layer struct {
	ID         string `json:"id"`
	Role       string `json:"role"`
	Input      string `json:"input"`
	SourceMode string `json:"source_mode"`
	Budget     string `json:"context_budget"`
	DrillDown  bool   `json:"drill_down"`
}

type LifetimePolicy struct {
	Input            string `json:"input"`
	Promotion        string `json:"promotion"`
	Automatic        bool   `json:"automatic"`
	Eligibility      string `json:"eligibility"`
	VersionedUpdates bool   `json:"versioned_updates"`
	DirectOverwrite  bool   `json:"direct_overwrite"`
	Budget           string `json:"context_budget"`
	DrillDown        bool   `json:"drill_down"`
}

type DreamingCycles struct {
	Daily  DreamingCycle `json:"daily"`
	Weekly DreamingCycle `json:"weekly"`
}

type DreamingCycle struct {
	Depth         string   `json:"depth"`
	Outputs       []string `json:"outputs"`
	LifetimeWrite bool     `json:"lifetime_write"`
}

type DreamingPolicy struct {
	SourceSelection          string `json:"source_selection"`
	Idempotency              string `json:"idempotency"`
	StageBeforeActivation    bool   `json:"stage_before_activation"`
	ValidateBeforeActivation bool   `json:"validate_before_activation"`
	AtomicActivation         bool   `json:"atomic_activation"`
	PreserveLastKnownGood    bool   `json:"preserve_last_known_good"`
	MutateSources            bool   `json:"mutate_sources"`
	ProvenanceRequired       bool   `json:"provenance_required"`
}

type ContextInjectionPolicy struct {
	Order                  []string `json:"order"`
	PerLayerBudgetRequired bool     `json:"per_layer_budget_required"`
	MissingLayer           string   `json:"missing_layer"`
	RawUnboundedFallback   bool     `json:"raw_unbounded_fallback"`
}

// LoadFile decodes one policy and rejects fields not owned by the canonical contract.
func LoadFile(path string) (Policy, error) {
	file, err := os.Open(path)
	if err != nil {
		return Policy{}, err
	}
	defer file.Close()

	return Load(file)
}

// Load decodes one policy from a managed bundle or another trusted reader.
func Load(reader io.Reader) (Policy, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var policy Policy
	if err := decoder.Decode(&policy); err != nil {
		return Policy{}, err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

// ValidateSchemaFile ensures the published schema remains parseable and identifies the supported draft.
func ValidateSchemaFile(path string) error {
	return validateSchemaFile(path, "urn:bcg-brasil-agentic-os:schema:memory-policy:v1")
}

func ValidateArtifactSchemaFile(path string) error {
	return validateSchemaFile(path, "urn:bcg-brasil-agentic-os:schema:memory-artifact:v1")
}

func ValidateCommitSchemaFile(path string) error {
	return validateSchemaFile(path, "urn:bcg-brasil-agentic-os:schema:memory-commit:v1")
}

func validateSchemaFile(path, expectedID string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	var schema map[string]any
	if err := decoder.Decode(&schema); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		return errors.New("memory policy schema must use JSON Schema draft 2020-12")
	}
	if schema["$id"] != expectedID {
		return errors.New("memory schema has an unexpected identifier")
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return err
	}
	return errors.New("policy contains multiple JSON values")
}

// Validate rejects policies that weaken the accepted memory safety contract.
func (policy Policy) Validate() error {
	var problems []error
	if policy.SchemaVersion != 1 {
		problems = append(problems, fmt.Errorf("schema_version must be 1"))
	}
	if policy.Storage.Scope != "user_local" {
		problems = append(problems, fmt.Errorf("storage.scope must be user_local"))
	}
	if !policy.Storage.WorkspaceIsolation {
		problems = append(problems, fmt.Errorf("storage.workspace_isolation must be true"))
	}
	if policy.Storage.ManagedCoreContainsUserData {
		problems = append(problems, fmt.Errorf("managed core cannot contain user data"))
	}

	expectedLayers := []Layer{
		{ID: "L1", Role: "daily", Input: "sanitized_signals", SourceMode: "append_only", Budget: "required_runtime_configuration", DrillDown: true},
		{ID: "L2", Role: "weekly", Input: "L1", SourceMode: "immutable", Budget: "required_runtime_configuration", DrillDown: true},
		{ID: "L3", Role: "medium_term_thematic", Input: "L2", SourceMode: "immutable", Budget: "required_runtime_configuration", DrillDown: true},
	}
	if !slices.Equal(policy.Layers, expectedLayers) {
		problems = append(problems, fmt.Errorf("layers must preserve the canonical L1 -> L2 -> L3 contract"))
	}

	if policy.Lifetime.Input != "L3" || policy.Lifetime.Promotion != "weekly_deep_dream" || !policy.Lifetime.Automatic || policy.Lifetime.Eligibility != "required_policy" || !policy.Lifetime.VersionedUpdates || policy.Lifetime.DirectOverwrite || policy.Lifetime.Budget != "required_runtime_configuration" || !policy.Lifetime.DrillDown {
		problems = append(problems, fmt.Errorf("lifetime memory must require eligibility and use weekly deep dreaming with versioned, non-overwriting updates"))
	}
	if policy.Cycles.Daily.Depth != "light" || !slices.Equal(policy.Cycles.Daily.Outputs, []string{"L1"}) || policy.Cycles.Daily.LifetimeWrite {
		problems = append(problems, fmt.Errorf("daily dreaming must be light and write only L1"))
	}
	if policy.Cycles.Weekly.Depth != "deep" || !slices.Equal(policy.Cycles.Weekly.Outputs, []string{"L2", "L3", "lifetime"}) || !policy.Cycles.Weekly.LifetimeWrite {
		problems = append(problems, fmt.Errorf("weekly dreaming must deeply consolidate L2, L3 and lifetime"))
	}
	if policy.Dreaming.SourceSelection != "deterministic" || policy.Dreaming.Idempotency != "source_fingerprint" {
		problems = append(problems, fmt.Errorf("dreaming must use deterministic selection and source fingerprints"))
	}
	if !policy.Dreaming.StageBeforeActivation || !policy.Dreaming.ValidateBeforeActivation || !policy.Dreaming.AtomicActivation {
		problems = append(problems, fmt.Errorf("dreaming must stage, validate and atomically activate outputs"))
	}
	if !policy.Dreaming.PreserveLastKnownGood || policy.Dreaming.MutateSources || !policy.Dreaming.ProvenanceRequired {
		problems = append(problems, fmt.Errorf("dreaming must preserve last known-good output, provenance and immutable sources"))
	}

	expectedOrder := []string{"lifetime", "L3", "L2", "L1"}
	if !slices.Equal(policy.ContextInjection.Order, expectedOrder) {
		problems = append(problems, fmt.Errorf("context injection order must be lifetime -> L3 -> L2 -> L1"))
	}
	if !policy.ContextInjection.PerLayerBudgetRequired || policy.ContextInjection.MissingLayer != "skip_with_diagnostic" || policy.ContextInjection.RawUnboundedFallback {
		problems = append(problems, fmt.Errorf("context injection requires bounded layers, diagnostics and no raw unbounded fallback"))
	}

	return errors.Join(problems...)
}
