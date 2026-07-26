// Package canary owns Maestro's local, metadata-only pilot observability.
// It has no dependency on federation and cannot serialize workspace/client
// content, exception strings, prompts, paths or arbitrary attributes.
package canary

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = 1

type Event string

const (
	EventFirstValue         Event = "first_value"
	EventGoalResume         Event = "goal_resume"
	EventInstallation       Event = "installation"
	EventUpdate             Event = "update"
	EventRollback           Event = "rollback"
	EventManualIntervention Event = "manual_intervention"
	EventCapabilityFailure  Event = "capability_failure"
)

type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	OutcomeBlocked   Outcome = "blocked"
	OutcomeNeutral   Outcome = "neutral"
)

type DurationBucket string

const (
	DurationUnderFiveMinutes   DurationBucket = "under_five_minutes"
	DurationUnderTenMinutes    DurationBucket = "under_ten_minutes"
	DurationUnderThirtyMinutes DurationBucket = "under_thirty_minutes"
	DurationOverThirtyMinutes  DurationBucket = "over_thirty_minutes"
)

type CountBucket string

const (
	CountOnce        CountBucket = "once"
	CountTwoToThree  CountBucket = "two_to_three"
	CountFourToSeven CountBucket = "four_to_seven"
	CountEightPlus   CountBucket = "eight_plus"
)

type Capability string

const (
	CapabilityLongRunningGoals Capability = "long-running-goals"
	CapabilityWorkspaceAgent   Capability = "workspace-agent"
	CapabilitySessionContext   Capability = "session-context"
	CapabilityInstallation     Capability = "installation"
	CapabilityUpdates          Capability = "updates"
)

// Receipt is intentionally closed. Zero-value optional fields are omitted;
// event-specific validation prevents them from becoming arbitrary metadata.
type Receipt struct {
	SchemaVersion int            `json:"schema_version"`
	RecordedAt    time.Time      `json:"recorded_at"`
	Event         Event          `json:"event"`
	Outcome       Outcome        `json:"outcome"`
	Duration      DurationBucket `json:"duration,omitempty"`
	Count         CountBucket    `json:"count,omitempty"`
	Capability    Capability     `json:"capability,omitempty"`
}

type DurationCount struct {
	Duration DurationBucket `json:"duration"`
	Count    int            `json:"count"`
}
type OutcomeCount struct {
	Outcome Outcome `json:"outcome"`
	Count   int     `json:"count"`
}
type LifecycleCount struct {
	Event   Event   `json:"event"`
	Outcome Outcome `json:"outcome"`
	Count   int     `json:"count"`
}
type CountBucketCount struct {
	CountBucket CountBucket `json:"count_bucket"`
	Count       int         `json:"count"`
}
type CapabilityCount struct {
	Capability Capability `json:"capability"`
	Count      int        `json:"count"`
}

// Report is a local operational dashboard. It has no installation or person
// identity, and every grouping is a closed typed tuple rather than a map.
type Report struct {
	SchemaVersion       int                `json:"schema_version"`
	ReceiptCount        int                `json:"receipt_count"`
	FirstValue          []DurationCount    `json:"first_value"`
	Resume              []OutcomeCount     `json:"resume"`
	Lifecycle           []LifecycleCount   `json:"lifecycle"`
	ManualInterventions []CountBucketCount `json:"manual_interventions"`
	CapabilityFailures  []CapabilityCount  `json:"capability_failures"`
}

func (report Report) String() string { encoded, _ := json.Marshal(report); return string(encoded) }

type Store struct{ Root string }

func (store Store) Append(receipt Receipt) error {
	if strings.TrimSpace(store.Root) == "" {
		return errors.New("canary root is required")
	}
	if receipt.SchemaVersion != 0 && receipt.SchemaVersion != SchemaVersion {
		return errors.New("unsupported canary receipt schema version")
	}
	receipt.SchemaVersion = SchemaVersion
	if err := receipt.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(store.receiptsRoot(), 0o700); err != nil {
		return err
	}
	prefix := receipt.RecordedAt.UTC().Format("20060102T150405.000000000Z") + "-" + string(receipt.Event)
	for suffix := 0; suffix < 1000; suffix++ {
		name := prefix
		if suffix > 0 {
			name += fmt.Sprintf("-%d", suffix)
		}
		err := writeNewJSON(filepath.Join(store.receiptsRoot(), name+".json"), receipt)
		if !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	return errors.New("canary receipt collision limit exceeded")
}

func (store Store) Report() (Report, error) {
	if strings.TrimSpace(store.Root) == "" {
		return Report{}, errors.New("canary root is required")
	}
	receipts, err := store.receipts()
	if err != nil {
		return Report{}, err
	}
	return aggregate(receipts), nil
}

func (store Store) receiptsRoot() string { return filepath.Join(store.Root, "receipts") }
func (store Store) receipts() ([]Receipt, error) {
	entries, err := os.ReadDir(store.receiptsRoot())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	receipts := make([]Receipt, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			return nil, fmt.Errorf("invalid canary receipt entry %q", entry.Name())
		}
		var receipt Receipt
		if err := readStrictJSON(filepath.Join(store.receiptsRoot(), entry.Name()), &receipt); err != nil {
			return nil, err
		}
		if err := receipt.Validate(); err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	sort.Slice(receipts, func(i, j int) bool { return receipts[i].RecordedAt.Before(receipts[j].RecordedAt) })
	return receipts, nil
}

func aggregate(receipts []Receipt) Report {
	first := map[DurationBucket]int{}
	resume := map[Outcome]int{}
	lifecycle := map[string]int{}
	manual := map[CountBucket]int{}
	failures := map[Capability]int{}
	for _, receipt := range receipts {
		switch receipt.Event {
		case EventFirstValue:
			first[receipt.Duration]++
		case EventGoalResume:
			resume[receipt.Outcome]++
		case EventInstallation, EventUpdate, EventRollback:
			lifecycle[string(receipt.Event)+"\x00"+string(receipt.Outcome)]++
		case EventManualIntervention:
			manual[receipt.Count]++
		case EventCapabilityFailure:
			failures[receipt.Capability]++
		}
	}
	report := Report{SchemaVersion: SchemaVersion, ReceiptCount: len(receipts)}
	for _, bucket := range []DurationBucket{DurationUnderFiveMinutes, DurationUnderTenMinutes, DurationUnderThirtyMinutes, DurationOverThirtyMinutes} {
		if first[bucket] > 0 {
			report.FirstValue = append(report.FirstValue, DurationCount{Duration: bucket, Count: first[bucket]})
		}
	}
	for _, outcome := range []Outcome{OutcomeSucceeded, OutcomeFailed, OutcomeBlocked} {
		if resume[outcome] > 0 {
			report.Resume = append(report.Resume, OutcomeCount{Outcome: outcome, Count: resume[outcome]})
		}
	}
	for _, event := range []Event{EventInstallation, EventUpdate, EventRollback} {
		for _, outcome := range []Outcome{OutcomeSucceeded, OutcomeFailed, OutcomeBlocked} {
			if count := lifecycle[string(event)+"\x00"+string(outcome)]; count > 0 {
				report.Lifecycle = append(report.Lifecycle, LifecycleCount{Event: event, Outcome: outcome, Count: count})
			}
		}
	}
	for _, bucket := range []CountBucket{CountOnce, CountTwoToThree, CountFourToSeven, CountEightPlus} {
		if manual[bucket] > 0 {
			report.ManualInterventions = append(report.ManualInterventions, CountBucketCount{CountBucket: bucket, Count: manual[bucket]})
		}
	}
	for _, capability := range []Capability{CapabilityLongRunningGoals, CapabilityWorkspaceAgent, CapabilitySessionContext, CapabilityInstallation, CapabilityUpdates} {
		if failures[capability] > 0 {
			report.CapabilityFailures = append(report.CapabilityFailures, CapabilityCount{Capability: capability, Count: failures[capability]})
		}
	}
	return report
}

func (receipt Receipt) Validate() error {
	if receipt.SchemaVersion != SchemaVersion || receipt.RecordedAt.IsZero() || !validEvent(receipt.Event) || !validOutcome(receipt.Outcome) {
		return errors.New("invalid canary receipt")
	}
	switch receipt.Event {
	case EventFirstValue:
		if receipt.Outcome != OutcomeSucceeded || !validDuration(receipt.Duration) || receipt.Count != "" || receipt.Capability != "" {
			return errors.New("invalid first-value receipt")
		}
	case EventGoalResume:
		if (receipt.Outcome != OutcomeSucceeded && receipt.Outcome != OutcomeFailed && receipt.Outcome != OutcomeBlocked) || receipt.Duration != "" || receipt.Count != "" || receipt.Capability != "" {
			return errors.New("invalid goal-resume receipt")
		}
	case EventInstallation, EventUpdate, EventRollback:
		if (receipt.Outcome != OutcomeSucceeded && receipt.Outcome != OutcomeFailed && receipt.Outcome != OutcomeBlocked) || receipt.Duration != "" || receipt.Count != "" || receipt.Capability != "" {
			return errors.New("invalid lifecycle receipt")
		}
	case EventManualIntervention:
		if receipt.Outcome != OutcomeNeutral || !validCount(receipt.Count) || receipt.Duration != "" || receipt.Capability != "" {
			return errors.New("invalid manual-intervention receipt")
		}
	case EventCapabilityFailure:
		if (receipt.Outcome != OutcomeFailed && receipt.Outcome != OutcomeBlocked) || !validCapability(receipt.Capability) || receipt.Duration != "" || receipt.Count != "" {
			return errors.New("invalid capability-failure receipt")
		}
	}
	return nil
}
func validEvent(event Event) bool {
	return event == EventFirstValue || event == EventGoalResume || event == EventInstallation || event == EventUpdate || event == EventRollback || event == EventManualIntervention || event == EventCapabilityFailure
}
func validOutcome(outcome Outcome) bool {
	return outcome == OutcomeSucceeded || outcome == OutcomeFailed || outcome == OutcomeBlocked || outcome == OutcomeNeutral
}
func validDuration(value DurationBucket) bool {
	return value == DurationUnderFiveMinutes || value == DurationUnderTenMinutes || value == DurationUnderThirtyMinutes || value == DurationOverThirtyMinutes
}
func validCount(value CountBucket) bool {
	return value == CountOnce || value == CountTwoToThree || value == CountFourToSeven || value == CountEightPlus
}
func validCapability(value Capability) bool {
	return value == CapabilityLongRunningGoals || value == CapabilityWorkspaceAgent || value == CapabilitySessionContext || value == CapabilityInstallation || value == CapabilityUpdates
}

func readStrictJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("canary receipt contains multiple JSON values")
		}
		return err
	}
	return nil
}
func writeNewJSON(path string, value any) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(file).Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func ValidateSchemaFile(path string) error {
	return validateSchemaFile(path, "urn:bcg-brasil-agentic-os:schema:canary-report:v1")
}

func ValidateReceiptSchemaFile(path string) error {
	return validateSchemaFile(path, "urn:bcg-brasil-agentic-os:schema:canary-receipt:v1")
}

func validateSchemaFile(path, expectedID string) error {
	var schema map[string]any
	if err := readStrictJSON(path, &schema); err != nil {
		return err
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" || schema["$id"] != expectedID {
		return errors.New("invalid canary schema")
	}
	return nil
}
