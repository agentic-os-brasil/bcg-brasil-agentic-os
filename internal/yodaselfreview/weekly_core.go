package yodaselfreview

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ownerctx"
)

// WeeklyCompactionInput is private quoted data, with no current interactive task.
type WeeklyCompactionInput struct {
	source                ownerctx.WeeklySelfInput
	payloadSHA256         string
	PreviousWorking       map[string]string             `json:"previous_working,omitempty"`
	PreviousWorkingSHA256 string                        `json:"previous_working_sha256,omitempty"`
	SchemaVersion         int                           `json:"schema_version"`
	JobID                 string                        `json:"job_id"`
	Mode                  string                        `json:"mode"`
	InputSHA256           string                        `json:"input_sha256"`
	CanonicalSHA256       string                        `json:"canonical_sha256"`
	PromptHistorySHA256   string                        `json:"prompt_history_sha256"`
	TranslationSHA256     string                        `json:"translation_sha256"`
	PolicySHA256          string                        `json:"policy_sha256"`
	WorkingLanguage       string                        `json:"working_language"`
	CanonicalSnapshot     ownerctx.UserSelfSnapshot     `json:"canonical_snapshot"`
	PromptHistory         []PromptEvidence              `json:"prompt_history"`
	Observations          []ownerctx.ObservationReceipt `json:"observations"`
}

// This contract registers no adapter and grants no model invocation authority.
type WeeklyCompactionAdapter interface {
	ID() string
	Compact(context.Context, WeeklyCompactionInput) (ownerctx.WeeklySelfCandidate, error)
}

func BuildWeeklyCompactionInput(frozen ownerctx.WeeklySelfInput) (WeeklyCompactionInput, error) {
	var result WeeklyCompactionInput
	if err := frozen.Validate(); err != nil {
		return result, err
	}
	result = WeeklyCompactionInput{SchemaVersion: 1, JobID: WeeklyJobID, Mode: "quoted_data_no_tools", InputSHA256: frozen.InputSHA256, CanonicalSHA256: frozen.Facets.CanonicalSourceDigest, PromptHistorySHA256: frozen.PromptHistorySHA256, TranslationSHA256: frozen.TranslationSHA256, PolicySHA256: frozen.PolicySHA256, WorkingLanguage: frozen.Options.WorkingLanguage, CanonicalSnapshot: frozen.Facets, PromptHistory: []PromptEvidence{}, Observations: append([]ownerctx.ObservationReceipt(nil), frozen.Observations...)}
	if frozen.PreviousWorking != nil {
		result.PreviousWorking = map[string]string{}
		for id, text := range frozen.PreviousWorking.Facets {
			result.PreviousWorking[id] = text
		}
		result.PreviousWorkingSHA256 = frozen.PreviousWorking.Receipt.WorkingSHA256
	}
	result.CanonicalSnapshot.Facets = map[string]ownerctx.SnapshotFacet{}
	for id, facet := range frozen.Facets.Facets {
		facet.Readers = append([]string(nil), facet.Readers...)
		result.CanonicalSnapshot.Facets[id] = facet
	}
	for _, entry := range frozen.PromptHistory {
		result.PromptHistory = append(result.PromptHistory, PromptEvidence{ID: entry.Source.ID, OriginalText: entry.Source.Prompt, NormalizedText: entry.Normalized, SourceLanguage: entry.Source.Language, WorkingLanguage: frozen.Options.WorkingLanguage, OriginalSHA256: entry.Source.SHA256, NormalizedSHA256: entry.NormalizedSHA256, QuotedData: true, Instructional: false})
	}
	body, err := json.Marshal(result)
	if err != nil {
		return WeeklyCompactionInput{}, err
	}
	if len(body) > MaxContextBytes {
		return WeeklyCompactionInput{}, errors.New("weekly model input exceeds complete context bound")
	}
	// Retain an independent private validation snapshot. It never enters JSON,
	// diagnostics, receipts, or a new cache. Exported transport fields are bound
	// to their exact payload so an adapter cannot mutate the admitted input.
	original, err := json.Marshal(frozen)
	if err != nil {
		return WeeklyCompactionInput{}, err
	}
	if err := json.Unmarshal(original, &result.source); err != nil {
		return WeeklyCompactionInput{}, err
	}
	result.payloadSHA256 = fmt.Sprintf("%x", sha256.Sum256(body))
	return result, nil
}

func (input WeeklyCompactionInput) Validate() error {
	if err := input.source.Validate(); err != nil {
		return err
	}
	body, err := json.Marshal(input)
	if err != nil {
		return err
	}
	if len(body) > MaxContextBytes || input.payloadSHA256 == "" || fmt.Sprintf("%x", sha256.Sum256(body)) != input.payloadSHA256 {
		return errors.New("weekly private transport snapshot changed")
	}
	return nil
}

// ParseCandidate reuses the canonical owner validator; model envelopes cannot
// substitute their own source identity, facet set, read policy, or byte budget.
func (input WeeklyCompactionInput) ParseCandidate(body []byte) (ownerctx.WeeklySelfCandidate, error) {
	if err := input.Validate(); err != nil {
		return ownerctx.WeeklySelfCandidate{}, err
	}
	return ownerctx.ParseWeeklySelfCandidate(input.source, body)
}
