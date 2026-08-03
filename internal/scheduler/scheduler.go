package scheduler

import (
	"bytes"
	"context"
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

type Cadence string

const (
	Daily    Cadence = "daily"
	Weekly   Cadence = "weekly"
	Monthly  Cadence = "monthly"
	Interval Cadence = "interval"
)

type ReceiptState string

const (
	Succeeded   ReceiptState = "succeeded"
	Failed      ReceiptState = "failed"
	Unavailable ReceiptState = "unavailable"
	Suppressed  ReceiptState = "suppressed"
)

var (
	ErrCapabilityUnavailable = errors.New("scheduler capability unavailable")
	jobIDPattern             = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	workspaceIDPattern       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)
	attemptTokenPattern      = regexp.MustCompile(`^[a-f0-9]{32}$`)
)

// Job describes cadence only. Native schedulers may wake the process, but this
// runtime-neutral contract remains responsible for deciding what is due.
type Job struct {
	ID            string
	Cadence       Cadence
	Weekday       time.Weekday
	DayOfMonth    int
	LocalHour     int
	LocalMinute   int
	IntervalHours int
	MaxCatchUp    int
}

type Occurrence struct {
	JobID        string    `json:"job_id"`
	EventID      string    `json:"event_id,omitempty"`
	ScheduledFor time.Time `json:"scheduled_for"`
}

type Receipt struct {
	SchemaVersion int          `json:"schema_version,omitempty"`
	WorkspaceID   string       `json:"workspace_id,omitempty"`
	JobID         string       `json:"job_id"`
	ScheduledFor  time.Time    `json:"scheduled_for"`
	AttemptedAt   time.Time    `json:"attempted_at"`
	State         ReceiptState `json:"state"`
	Error         string       `json:"error,omitempty"`
}

func (receipt Receipt) Err() error {
	if receipt.Error == "" {
		return nil
	}
	if receipt.State == Unavailable {
		return fmt.Errorf("%w: %s", ErrCapabilityUnavailable, receipt.Error)
	}
	return errors.New(receipt.Error)
}

type Enrollment struct {
	SchemaVersion int       `json:"schema_version"`
	WorkspaceID   string    `json:"workspace_id"`
	EnrolledAt    time.Time `json:"enrolled_at"`
}

type Executor interface {
	Execute(context.Context, Occurrence) error
}

type ExecutorFunc func(context.Context, Occurrence) error

func (function ExecutorFunc) Execute(ctx context.Context, occurrence Occurrence) error {
	return function(ctx, occurrence)
}

// PlanDue derives missed work from the durable enrollment boundary and
// successful receipts. Failed or unavailable attempts never make an occurrence
// complete, so a later presence trigger can recover it.
func PlanDue(jobs []Job, enrolledAt time.Time, receipts []Receipt, now time.Time) ([]Occurrence, error) {
	if enrolledAt.IsZero() || now.IsZero() {
		return nil, errors.New("enrollment and current time are required")
	}
	if now.Before(enrolledAt) {
		return nil, errors.New("current time cannot precede enrollment")
	}
	var due []Occurrence
	seen := make(map[string]bool)
	succeeded := make(map[string]bool)
	lastSuccess := make(map[string]time.Time)
	for _, receipt := range receipts {
		if receipt.State == Succeeded {
			succeeded[occurrenceKey(receipt.JobID, receipt.ScheduledFor)] = true
			if receipt.AttemptedAt.After(lastSuccess[receipt.JobID]) {
				lastSuccess[receipt.JobID] = receipt.AttemptedAt
			}
		}
	}
	for _, job := range jobs {
		if err := validateJob(job); err != nil {
			return nil, err
		}
		if seen[job.ID] {
			return nil, fmt.Errorf("duplicate scheduler job %q", job.ID)
		}
		seen[job.ID] = true
		cursor := enrolledAt.In(now.Location())
		if job.Cadence == Interval && !lastSuccess[job.ID].IsZero() {
			cursor = lastSuccess[job.ID].In(now.Location())
		}
		for occurrence := nextOccurrence(job, cursor); !occurrence.After(now); occurrence = nextOccurrence(job, occurrence) {
			if succeeded[occurrenceKey(job.ID, occurrence)] {
				continue
			}
			due = append(due, Occurrence{JobID: job.ID, ScheduledFor: occurrence})
			if countForJob(due, job.ID) >= job.MaxCatchUp {
				break
			}
		}
	}
	sort.Slice(due, func(left, right int) bool {
		if due[left].ScheduledFor.Equal(due[right].ScheduledFor) {
			return due[left].JobID < due[right].JobID
		}
		return due[left].ScheduledFor.Before(due[right].ScheduledFor)
	})
	return due, nil
}

func occurrenceKey(jobID string, scheduledFor time.Time) string {
	return jobID + "\x00" + scheduledFor.UTC().Format(time.RFC3339Nano)
}

func RunDue(ctx context.Context, executor Executor, occurrences []Occurrence, attemptedAt time.Time) []Receipt {
	receipts := make([]Receipt, 0, len(occurrences))
	for _, occurrence := range occurrences {
		if ctx.Err() != nil {
			break
		}
		receipt := Receipt{JobID: occurrence.JobID, ScheduledFor: occurrence.ScheduledFor, AttemptedAt: attemptedAt, State: Succeeded}
		err := executor.Execute(ctx, occurrence)
		if err != nil {
			receipt.Error = err.Error()
			if errors.Is(err, ErrCapabilityUnavailable) {
				receipt.State = Unavailable
			} else {
				receipt.State = Failed
			}
		}
		receipts = append(receipts, receipt)
	}
	return receipts
}

func validateJob(job Job) error {
	if !jobIDPattern.MatchString(job.ID) {
		return fmt.Errorf("invalid scheduler job ID %q", job.ID)
	}
	if job.Cadence != Daily && job.Cadence != Weekly && job.Cadence != Monthly && job.Cadence != Interval {
		return fmt.Errorf("invalid cadence for job %q", job.ID)
	}
	if job.Cadence == Interval {
		if job.IntervalHours <= 0 || job.IntervalHours > 8760 {
			return fmt.Errorf("invalid interval for job %q", job.ID)
		}
	} else if job.IntervalHours != 0 {
		return fmt.Errorf("calendar job %q cannot define an interval", job.ID)
	}
	if job.Cadence == Monthly {
		if job.DayOfMonth < 1 || job.DayOfMonth > 28 {
			return fmt.Errorf("monthly job %q requires day of month between 1 and 28", job.ID)
		}
	} else if job.DayOfMonth != 0 {
		return fmt.Errorf("non-monthly job %q cannot define a day of month", job.ID)
	}
	if job.LocalHour < 0 || job.LocalHour > 23 || job.LocalMinute < 0 || job.LocalMinute > 59 {
		return fmt.Errorf("invalid local schedule for job %q", job.ID)
	}
	if job.MaxCatchUp <= 0 {
		return fmt.Errorf("positive catch-up limit required for job %q", job.ID)
	}
	if job.Cadence == Weekly && (job.Weekday < time.Sunday || job.Weekday > time.Saturday) {
		return fmt.Errorf("invalid weekday for job %q", job.ID)
	}
	return nil
}

func nextOccurrence(job Job, after time.Time) time.Time {
	if job.Cadence == Interval {
		return after.Add(time.Duration(job.IntervalHours) * time.Hour)
	}
	location := after.Location()
	candidate := time.Date(after.Year(), after.Month(), after.Day(), job.LocalHour, job.LocalMinute, 0, 0, location)
	if job.Cadence == Weekly {
		days := (int(job.Weekday) - int(candidate.Weekday()) + 7) % 7
		candidate = candidate.AddDate(0, 0, days)
	} else if job.Cadence == Monthly {
		candidate = time.Date(after.Year(), after.Month(), job.DayOfMonth, job.LocalHour, job.LocalMinute, 0, 0, location)
	}
	if !candidate.After(after) {
		if job.Cadence == Daily {
			candidate = candidate.AddDate(0, 0, 1)
		} else if job.Cadence == Weekly {
			candidate = candidate.AddDate(0, 0, 7)
		} else {
			candidate = candidate.AddDate(0, 1, 0)
			candidate = time.Date(candidate.Year(), candidate.Month(), job.DayOfMonth, job.LocalHour, job.LocalMinute, 0, 0, location)
		}
	}
	return candidate
}

func countForJob(occurrences []Occurrence, jobID string) int {
	count := 0
	for _, occurrence := range occurrences {
		if occurrence.JobID == jobID {
			count++
		}
	}
	return count
}

// Store persists only metadata-safe enrollment and execution receipts. Job
// outputs and professional content belong to their owning subsystems.
type Store struct {
	Root string
}

func (store Store) EnsureEnrollment(workspaceID string, enrolledAt time.Time) (Enrollment, error) {
	if err := validateStoreInput(store.Root, workspaceID); err != nil {
		return Enrollment{}, err
	}
	workspaceRoot, err := ensurePrivateTree(store.Root, "workspaces", workspaceID)
	if err != nil {
		return Enrollment{}, err
	}
	path := filepath.Join(workspaceRoot, "enrollment.json")
	if enrollment, err := readEnrollment(path); err == nil {
		return enrollment, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return Enrollment{}, err
	}
	if enrolledAt.IsZero() {
		return Enrollment{}, errors.New("enrollment time is required")
	}
	enrollment := Enrollment{SchemaVersion: 1, WorkspaceID: workspaceID, EnrolledAt: enrolledAt}
	if err := writeNewJSON(path, enrollment); err != nil {
		if errors.Is(err, os.ErrExist) {
			return readEnrollment(path)
		}
		return Enrollment{}, err
	}
	return enrollment, nil
}

// LoadEnrollment is a read-only authority preflight. It never creates the
// enrollment boundary; callers must validate local attendance or persisted
// preauthorization before invoking EnsureEnrollment.
func (store Store) LoadEnrollment(workspaceID string) (Enrollment, error) {
	if store.Root == "" || !workspaceIDPattern.MatchString(workspaceID) {
		return Enrollment{}, errors.New("invalid scheduler enrollment lookup")
	}
	root, err := lookupPrivateTree(store.Root, "workspaces", workspaceID)
	if err != nil {
		return Enrollment{}, err
	}
	return readEnrollment(filepath.Join(root, "enrollment.json"))
}

func (store Store) AppendReceipt(workspaceID string, receipt Receipt) error {
	if err := validateStoreInput(store.Root, workspaceID); err != nil {
		return err
	}
	if err := validateReceipt(receipt); err != nil {
		return err
	}
	receipt.SchemaVersion = 1
	receipt.WorkspaceID = workspaceID
	directory, err := ensurePrivateTree(store.Root, "workspaces", workspaceID, "receipts", receipt.JobID)
	if err != nil {
		return err
	}
	name := receipt.AttemptedAt.UTC().Format("20060102T150405.000000000Z") + "-" + receipt.ScheduledFor.UTC().Format("20060102T150405.000000000Z") + ".json"
	return writeNewJSON(filepath.Join(directory, name), receipt)
}

func (store Store) Receipts(workspaceID string) ([]Receipt, error) {
	if err := validateStoreInput(store.Root, workspaceID); err != nil {
		return nil, err
	}
	root, err := ensurePrivateTree(store.Root, "workspaces", workspaceID, "receipts")
	if err != nil {
		return nil, err
	}
	var receipts []Receipt
	entries, err := secureReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	for _, jobEntry := range entries {
		if jobEntry.Type()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("scheduler receipt path must not be a symlink: %s", jobEntry.Name())
		}
		if !jobEntry.IsDir() {
			continue
		}
		jobRoot := filepath.Join(root, jobEntry.Name())
		files, readErr := secureReadDir(jobRoot)
		if readErr != nil {
			return nil, readErr
		}
		for _, entry := range files {
			if entry.Type()&os.ModeSymlink != 0 {
				return nil, fmt.Errorf("scheduler receipt path must not be a symlink: %s", entry.Name())
			}
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			var receipt Receipt
			if err := readStrictJSON(filepath.Join(jobRoot, entry.Name()), &receipt); err != nil {
				return nil, err
			}
			if receipt.SchemaVersion != 1 || receipt.WorkspaceID != workspaceID || receipt.JobID != jobEntry.Name() {
				return nil, fmt.Errorf("invalid scheduler receipt %s", entry.Name())
			}
			if err := validateReceipt(receipt); err != nil {
				return nil, err
			}
			receipt.SchemaVersion = 0
			receipt.WorkspaceID = ""
			receipts = append(receipts, receipt)
		}
	}
	sort.Slice(receipts, func(left, right int) bool { return receipts[left].AttemptedAt.Before(receipts[right].AttemptedAt) })
	return receipts, nil
}

func (store Store) workspaceRoot(workspaceID string) string {
	return filepath.Join(store.Root, "workspaces", workspaceID)
}

func validateStoreInput(root, workspaceID string) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("scheduler root is required")
	}
	if !workspaceIDPattern.MatchString(workspaceID) {
		return fmt.Errorf("invalid workspace ID %q", workspaceID)
	}
	return nil
}

func validateReceipt(receipt Receipt) error {
	if !jobIDPattern.MatchString(receipt.JobID) || receipt.ScheduledFor.IsZero() || receipt.AttemptedAt.IsZero() {
		return errors.New("invalid scheduler receipt")
	}
	if receipt.State != Succeeded && receipt.State != Failed && receipt.State != Unavailable && receipt.State != Suppressed {
		return errors.New("invalid scheduler receipt state")
	}
	if receipt.State == Succeeded && receipt.Error != "" {
		return errors.New("successful scheduler receipt cannot contain an error")
	}
	if receipt.State != Succeeded && strings.TrimSpace(receipt.Error) == "" {
		return errors.New("unsuccessful scheduler receipt requires an error")
	}
	return nil
}

func readEnrollment(path string) (Enrollment, error) {
	var enrollment Enrollment
	if err := readStrictJSON(path, &enrollment); err != nil {
		return Enrollment{}, err
	}
	if enrollment.SchemaVersion != 1 || !workspaceIDPattern.MatchString(enrollment.WorkspaceID) || enrollment.EnrolledAt.IsZero() {
		return Enrollment{}, errors.New("invalid scheduler enrollment")
	}
	return enrollment, nil
}

func readStrictJSON(path string, target any) error {
	body, err := secureReadFile(path)
	if err != nil {
		return err
	}
	return decodeStrictJSON(body, target)
}

func readStrictJSONInDirectory(directory *secureDirectory, name string, target any) error {
	body, err := directory.readFile(name)
	if err != nil {
		return err
	}
	return decodeStrictJSON(body, target)
}

func decodeStrictJSON(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("scheduler file contains multiple JSON values")
		}
		return err
	}
	return nil
}

func writeNewJSON(path string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return secureWriteNewFile(path, body)
}

func writeNewJSONInDirectory(directory *secureDirectory, name string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return directory.writeNewFile(name, body)
}

// ValidateSchemaFile keeps the published scheduler-state contract wired into
// the executable test suite without introducing a runtime schema dependency.
func ValidateSchemaFile(path string) error {
	var schema map[string]any
	if err := readStrictJSON(path, &schema); err != nil {
		return err
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		return errors.New("scheduler state schema must use JSON Schema draft 2020-12")
	}
	if schema["$id"] != "urn:bcg-brasil-agentic-os:schema:scheduler-state:v1" {
		return errors.New("scheduler state schema has an unexpected identifier")
	}
	return nil
}
