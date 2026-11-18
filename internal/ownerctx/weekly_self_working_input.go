package ownerctx

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"
)

const maximumWeeklyDependencyRecords = 256

func attachPreviousWeeklyWorking(input *WeeklySelfInput, sources weeklySelfSources, working *WeeklySelfWorking) error {
	if working == nil {
		return nil
	}
	if err := validateWeeklySource(working.Source, sources, input.FrozenAt); err != nil {
		return err
	}
	body, err := json.Marshal(working)
	if err != nil {
		return err
	}
	var copy WeeklySelfWorking
	if err := json.Unmarshal(body, &copy); err != nil {
		return err
	}
	copy.Receipt.Verified = false
	input.PreviousWorking = &copy
	return validatePreviousWeeklyWorking(*input)
}
func validatePreviousWeeklyWorking(input WeeklySelfInput) error {
	previous := input.PreviousWorking
	if previous == nil {
		return nil
	}
	if previous.Receipt.State != "committed" || previous.Receipt.Revision == 0 || previous.Receipt.Revision != input.ExpectedRevision || !validDigest(previous.Receipt.InputSHA256) || previous.Receipt.OccurrenceID == input.Options.OccurrenceID || previous.Receipt.OccurrenceID != previous.Source.Options.OccurrenceID || !previous.ExpiresAt.After(input.FrozenAt) || !sameWeeklyReadPolicy(previous.Source.Options, input.Options) || previous.Source.CanonicalSHA256 != input.Facets.CanonicalSourceDigest || previous.Source.PolicySHA256 != input.PolicySHA256 || len(previous.Facets) != len(input.Facets.Facets) {
		return ErrWeeklySelfChanged
	}
	candidate := WeeklySelfCandidate{SchemaVersion: 1, InputSHA256: previous.Receipt.InputSHA256, CanonicalSHA256: previous.Source.CanonicalSHA256, PromptHistorySHA256: previous.Source.PromptHistorySHA256, PolicySHA256: previous.Source.PolicySHA256, TranslationSHA256: previous.Source.TranslationSHA256, Facets: previous.Facets}
	if previous.Receipt.WorkingSHA256 != weeklyCandidateDigest(candidate) {
		return ErrWeeklySelfChanged
	}
	for id, text := range previous.Facets {
		facet, ok := input.Facets.Facets[id]
		if !ok || !containsWeekly(facet.Readers, "yoda") || strings.TrimSpace(text) == "" || !utf8.ValidString(text) || strings.ContainsRune(text, 0) || len(text) > input.Options.Limits.MaxWorkingFacetBytes {
			return errors.New("previous weekly working facet violates read/budget policy")
		}
	}
	for _, prompt := range input.PromptHistory {
		if digest := previous.Source.Prompts[prompt.Source.ID]; digest != "" && digest != prompt.SourceRecordSHA256 {
			return ErrWeeklySelfChanged
		}
	}
	for _, observation := range input.Observations {
		if revision := previous.Source.Observations[observation.ID]; revision != "" && revision != observation.Revision {
			return ErrWeeklySelfChanged
		}
	}
	return validateWeeklyDependencyBounds(sourceFromWeeklyInput(input))
}
func validateWeeklyDependencyBounds(source weeklySelfSource) error {
	total := 0
	for _, entries := range []map[string]string{source.Prompts, source.Observations, source.ObservationSupport} {
		total += len(entries)
		for id, digest := range entries {
			if !observationIdentifier.MatchString(id) || !validDigest(digest) {
				return errors.New("weekly dependency identity is invalid")
			}
		}
	}
	if total > maximumWeeklyDependencyRecords {
		return errors.New("weekly inherited source count exceeds fixed bound")
	}
	return nil
}
func hydratePreviousWeeklyWorking(manifest *WeeklySelfInput, current *WeeklySelfWorking) error {
	if manifest.PreviousWorking == nil {
		return nil
	}
	if current == nil {
		return ErrWeeklySelfChanged
	}
	expected := *manifest.PreviousWorking
	actual := *current
	actual.Facets = nil
	actual.Receipt.Verified = false
	if digestJSON(expected) != digestJSON(actual) {
		return ErrWeeklySelfChanged
	}
	body, err := json.Marshal(current)
	if err != nil {
		return err
	}
	var copy WeeklySelfWorking
	if err := json.Unmarshal(body, &copy); err != nil {
		return err
	}
	copy.Receipt.Verified = false
	manifest.PreviousWorking = &copy
	return nil
}
