package maintenance

import (
	"errors"
	"fmt"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
)

// OccurrenceAuthorization is the metadata-only handoff from an authoritative
// scheduler/event gate. It deliberately excludes command IDs so retries bind
// to the same occurrence.
type OccurrenceAuthorization struct {
	WorkspaceID  string
	JobID        string
	Trigger      Trigger
	EventID      string
	ScheduledFor time.Time
}

// ExecutionAuthority is a closed, validated catalog + policy + occurrence
// grant. Its fields are private so a worker cannot substitute a permissive
// callback for unavailable/default-disabled/attended policy.
type ExecutionAuthority struct {
	catalog        Catalog
	occurrences    map[string]OccurrenceAuthorization
	activated      map[string]bool
	localQualified map[string]string
	attended       bool
	ready          bool
}

func NewExecutionAuthority(catalog Catalog, occurrences []OccurrenceAuthorization, activatedJobs []string, attended bool) (ExecutionAuthority, error) {
	if err := catalog.Validate(); err != nil {
		return ExecutionAuthority{}, err
	}
	authority := ExecutionAuthority{
		catalog: catalog, occurrences: make(map[string]OccurrenceAuthorization, len(occurrences)),
		activated: make(map[string]bool, len(activatedJobs)), attended: attended, ready: true,
	}
	for _, jobID := range activatedJobs {
		job, found := findCatalogJob(catalog, jobID)
		if !found || job.Availability != Available {
			return ExecutionAuthority{}, fmt.Errorf("maintenance activation references an unqualified job %q", jobID)
		}
		authority.activated[jobID] = true
	}
	for _, occurrence := range occurrences {
		if err := authority.addOccurrence(occurrence); err != nil {
			return ExecutionAuthority{}, err
		}
	}
	return authority, nil
}

// NewLocalExecutionAuthority grants a narrowly scoped, explicit Canary
// qualification without changing the shipped catalog state. The digest is
// evidence for this local enrollment only; it never promotes a catalog job.
func NewLocalExecutionAuthority(catalog Catalog, occurrences []OccurrenceAuthorization, localQualification map[string]string, activatedJobs []string, attended bool) (ExecutionAuthority, error) {
	if err := catalog.Validate(); err != nil {
		return ExecutionAuthority{}, err
	}
	authority := ExecutionAuthority{catalog: catalog, occurrences: make(map[string]OccurrenceAuthorization, len(occurrences)), activated: make(map[string]bool, len(activatedJobs)), localQualified: make(map[string]string, len(localQualification)), attended: attended, ready: true}
	for jobID, digest := range localQualification {
		job, found := findCatalogJob(catalog, jobID)
		if !found || !digestPattern.MatchString(digest) || (job.Availability != Unavailable && job.Availability != Available) {
			return ExecutionAuthority{}, fmt.Errorf("local qualification references an invalid job %q", jobID)
		}
		if job.Availability == Available && digest != job.QualificationDigest {
			return ExecutionAuthority{}, fmt.Errorf("local qualification does not bind the catalog evidence for %q", jobID)
		}
		authority.localQualified[jobID] = digest
	}
	for _, jobID := range activatedJobs {
		job, found := findCatalogJob(catalog, jobID)
		if !found || (job.Availability != Available && authority.localQualified[jobID] == "") {
			return ExecutionAuthority{}, fmt.Errorf("maintenance activation references an unqualified job %q", jobID)
		}
		authority.activated[jobID] = true
	}
	for _, occurrence := range occurrences {
		if err := authority.addOccurrence(occurrence); err != nil {
			return ExecutionAuthority{}, err
		}
	}
	return authority, nil
}

func (authority *ExecutionAuthority) addOccurrence(occurrence OccurrenceAuthorization) error {
	if !commandIDPattern.MatchString(occurrence.WorkspaceID) || !commandIDPattern.MatchString(occurrence.JobID) || !validTrigger(occurrence.Trigger) || occurrence.ScheduledFor.IsZero() {
		return errors.New("maintenance occurrence authorization is invalid")
	}
	if (occurrence.Trigger == TriggerEvent || occurrence.Trigger == TriggerContinuous) != commandIDPattern.MatchString(occurrence.EventID) {
		return errors.New("maintenance event occurrence authorization is invalid")
	}
	key := authorityOccurrenceKey(occurrence.WorkspaceID, occurrence.JobID, occurrence.Trigger, occurrence.EventID, occurrence.ScheduledFor)
	if _, duplicate := authority.occurrences[key]; duplicate {
		return errors.New("duplicate maintenance occurrence authorization")
	}
	authority.occurrences[key] = occurrence
	return nil
}

func (authority ExecutionAuthority) Ready() bool {
	return authority.ready
}

func (authority ExecutionAuthority) Authorize(command Command, now time.Time) (scheduler.Occurrence, error) {
	if !authority.ready {
		return scheduler.Occurrence{}, errors.New("maintenance execution authority is unavailable")
	}
	if err := command.Validate(now); err != nil {
		return scheduler.Occurrence{}, err
	}
	job, found := findCatalogJob(authority.catalog, command.JobID)
	if !found || !triggerMatches(job.Trigger, string(command.Trigger)) {
		return scheduler.Occurrence{}, errors.New("maintenance command is outside the qualified catalog trigger")
	}
	locallyQualified := authority.localQualified[job.ID] != ""
	if (job.Availability != Available && !locallyQualified) || (!job.DefaultEnabled && !authority.activated[job.ID]) {
		return scheduler.Occurrence{}, errors.New("maintenance command job is unavailable or disabled")
	}
	if (job.Unattended == "policy_gated" || job.Unattended == "never") && !authority.attended {
		return scheduler.Occurrence{}, errors.New("maintenance command requires attended authority")
	}
	key := authorityOccurrenceKey(command.WorkspaceID, command.JobID, command.Trigger, command.EventID, command.ScheduledFor)
	if _, found := authority.occurrences[key]; !found {
		return scheduler.Occurrence{}, errors.New("maintenance command was not emitted by the authoritative occurrence gate")
	}
	return scheduler.Occurrence{JobID: command.JobID, ScheduledFor: command.ScheduledFor}, nil
}

func authorityOccurrenceKey(workspaceID, jobID string, trigger Trigger, eventID string, scheduledFor time.Time) string {
	return workspaceID + "\x00" + occurrenceKey(jobID, trigger, eventID, scheduledFor)
}

func findCatalogJob(catalog Catalog, jobID string) (Job, bool) {
	for _, job := range catalog.Jobs {
		if job.ID == jobID {
			return job, true
		}
	}
	return Job{}, false
}

func occurrenceKey(jobID string, trigger Trigger, eventID string, scheduledFor time.Time) string {
	if trigger == TriggerEvent || trigger == TriggerContinuous {
		return jobID + "\x00event\x00" + eventID
	}
	return jobID + "\x00scheduled\x00" + scheduledFor.UTC().Format(time.RFC3339Nano)
}
