// Package canary owns Maestro's local, metadata-only pilot measurements.
// It deliberately has no federation or network dependency.
package canary

import (
	"bytes"
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
	CapabilityExecution      Capability = "execution"
	CapabilityWorkspaceAgent Capability = "workspace-agent"
	CapabilitySessionContext Capability = "session-context"
	CapabilityInstallation   Capability = "installation"
	CapabilityUpdates        Capability = "updates"
)

// Receipt is a closed tuple. It has no extension map or free-text field.
type Receipt struct {
	SchemaVersion int            `json:"schema_version"`
	RecordedAt    time.Time      `json:"recorded_at"`
	Event         Event          `json:"event"`
	Outcome       Outcome        `json:"outcome"`
	Duration      DurationBucket `json:"duration,omitempty"`
	Count         CountBucket    `json:"count,omitempty"`
	Capability    Capability     `json:"capability,omitempty"`
}

func (receipt Receipt) Validate() error {
	if receipt.SchemaVersion != SchemaVersion || receipt.RecordedAt.IsZero() {
		return errors.New("invalid canary receipt header")
	}
	switch receipt.Event {
	case EventFirstValue:
		if receipt.Outcome != OutcomeSucceeded || !validDuration(receipt.Duration) || receipt.Count != "" || receipt.Capability != "" {
			return errors.New("invalid first-value receipt")
		}
	case EventGoalResume:
		if !operationalOutcome(receipt.Outcome) || receipt.Duration != "" || receipt.Count != "" || receipt.Capability != "" {
			return errors.New("invalid goal-resume receipt")
		}
	case EventInstallation, EventUpdate, EventRollback:
		if !operationalOutcome(receipt.Outcome) || receipt.Duration != "" || receipt.Count != "" || receipt.Capability != "" {
			return errors.New("invalid lifecycle receipt")
		}
	case EventManualIntervention:
		if receipt.Outcome != OutcomeNeutral || !validCount(receipt.Count) || receipt.Duration != "" || receipt.Capability != "" {
			return errors.New("invalid intervention receipt")
		}
	case EventCapabilityFailure:
		if (receipt.Outcome != OutcomeFailed && receipt.Outcome != OutcomeBlocked) || !validCapability(receipt.Capability) || receipt.Duration != "" || receipt.Count != "" {
			return errors.New("invalid capability-failure receipt")
		}
	default:
		return errors.New("invalid canary event")
	}
	return nil
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

type Report struct {
	SchemaVersion       int                `json:"schema_version"`
	ReceiptCount        int                `json:"receipt_count"`
	FirstValue          []DurationCount    `json:"first_value"`
	Resume              []OutcomeCount     `json:"resume"`
	Lifecycle           []LifecycleCount   `json:"lifecycle"`
	ManualInterventions []CountBucketCount `json:"manual_interventions"`
	CapabilityFailures  []CapabilityCount  `json:"capability_failures"`
}

type Store struct {
	Root string
}

func (store Store) Append(receipt Receipt) error {
	if strings.TrimSpace(store.Root) == "" {
		return errors.New("canary root is required")
	}
	if receipt.SchemaVersion == 0 {
		receipt.SchemaVersion = SchemaVersion
	}
	if err := receipt.Validate(); err != nil {
		return err
	}
	root := filepath.Join(store.Root, "receipts")
	if err := ensurePrivateDirectory(root); err != nil {
		return err
	}
	prefix := receipt.RecordedAt.UTC().Format("20060102T150405.000000000Z") + "-" + string(receipt.Event)
	for suffix := 0; suffix < 1000; suffix++ {
		name := prefix
		if suffix > 0 {
			name += fmt.Sprintf("-%d", suffix)
		}
		if err := writeNewJSON(filepath.Join(root, name+".json"), receipt); !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	return errors.New("canary receipt collision limit exceeded")
}

func (store Store) Report() (Report, error) {
	receipts, err := store.receipts()
	if err != nil {
		return Report{}, err
	}
	return aggregate(receipts), nil
}

func (store Store) receipts() ([]Receipt, error) {
	if strings.TrimSpace(store.Root) == "" {
		return nil, errors.New("canary root is required")
	}
	root := filepath.Join(store.Root, "receipts")
	info, statErr := os.Lstat(root)
	if errors.Is(statErr, os.ErrNotExist) {
		return nil, nil
	}
	if statErr != nil {
		return nil, statErr
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, errors.New("canary receipts root must be a private local directory")
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	receipts := make([]Receipt, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 ||
			!strings.HasSuffix(entry.Name(), ".json") {
			return nil, fmt.Errorf("invalid canary receipt entry %q", entry.Name())
		}
		var receipt Receipt
		if err := readStrictJSON(filepath.Join(root, entry.Name()), &receipt); err != nil {
			return nil, err
		}
		if err := receipt.Validate(); err != nil {
			return nil, err
		}
		receipts = append(receipts, receipt)
	}
	sort.Slice(receipts, func(i, j int) bool {
		if receipts[i].RecordedAt.Equal(receipts[j].RecordedAt) {
			return receipts[i].Event < receipts[j].Event
		}
		return receipts[i].RecordedAt.Before(receipts[j].RecordedAt)
	})
	return receipts, nil
}

func ensurePrivateDirectory(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("canary receipts root must be a private local directory")
	}
	if info.Mode().Perm() != 0o700 {
		if err := os.Chmod(path, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func aggregate(receipts []Receipt) Report {
	report := Report{SchemaVersion: SchemaVersion, ReceiptCount: len(receipts)}
	first := map[DurationBucket]int{}
	resume := map[Outcome]int{}
	lifecycle := map[[2]string]int{}
	manual := map[CountBucket]int{}
	failures := map[Capability]int{}
	for _, receipt := range receipts {
		switch receipt.Event {
		case EventFirstValue:
			first[receipt.Duration]++
		case EventGoalResume:
			resume[receipt.Outcome]++
		case EventInstallation, EventUpdate, EventRollback:
			lifecycle[[2]string{string(receipt.Event), string(receipt.Outcome)}]++
		case EventManualIntervention:
			manual[receipt.Count]++
		case EventCapabilityFailure:
			failures[receipt.Capability]++
		}
	}
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
			if count := lifecycle[[2]string{string(event), string(outcome)}]; count > 0 {
				report.Lifecycle = append(report.Lifecycle, LifecycleCount{Event: event, Outcome: outcome, Count: count})
			}
		}
	}
	for _, bucket := range []CountBucket{CountOnce, CountTwoToThree, CountFourToSeven, CountEightPlus} {
		if manual[bucket] > 0 {
			report.ManualInterventions = append(report.ManualInterventions, CountBucketCount{CountBucket: bucket, Count: manual[bucket]})
		}
	}
	for _, capability := range []Capability{CapabilityExecution, CapabilityWorkspaceAgent, CapabilitySessionContext, CapabilityInstallation, CapabilityUpdates} {
		if failures[capability] > 0 {
			report.CapabilityFailures = append(report.CapabilityFailures, CapabilityCount{Capability: capability, Count: failures[capability]})
		}
	}
	return report
}

func operationalOutcome(outcome Outcome) bool {
	return outcome == OutcomeSucceeded || outcome == OutcomeFailed || outcome == OutcomeBlocked
}

func validDuration(value DurationBucket) bool {
	return value == DurationUnderFiveMinutes || value == DurationUnderTenMinutes || value == DurationUnderThirtyMinutes || value == DurationOverThirtyMinutes
}

func validCount(value CountBucket) bool {
	return value == CountOnce || value == CountTwoToThree || value == CountFourToSeven || value == CountEightPlus
}

func validCapability(value Capability) bool {
	return value == CapabilityExecution || value == CapabilityWorkspaceAgent || value == CapabilitySessionContext || value == CapabilityInstallation || value == CapabilityUpdates
}

func writeNewJSON(path string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(body); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func readStrictJSON(path string, value any) error {
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func rejectDuplicateJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := validateJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func validateJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]bool)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("invalid JSON object key")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = true
			if err := validateJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("invalid JSON object")
		}
	case '[':
		for decoder.More() {
			if err := validateJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("invalid JSON array")
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	return nil
}
