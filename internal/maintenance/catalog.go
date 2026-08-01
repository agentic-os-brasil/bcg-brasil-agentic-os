// Package maintenance validates the declarative, platform-neutral maintenance
// catalog shipped with the base bundle. It describes work, not wake-up
// mechanics or authorization to execute it.
package maintenance

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
)

const (
	SchemaVersion    = 1
	CatalogOnly      = "catalog_only"
	RuntimeQualified = "runtime_qualified"
	Available        = "available"
	Unavailable      = "unavailable"
)

var (
	validCategories = map[string]bool{"memory": true, "wiki": true, "runtime": true, "self": true, "update": true}
	validTriggers   = map[string]bool{
		"continuous": true, "presence": true, "daily": true, "weekly": true, "monthly": true, "event": true,
		"daily_or_presence": true, "weekly_or_presence": true, "monthly_or_presence": true,
		"bundle_update_or_presence": true, "lifecycle_cadence": true,
	}
	validExecutors  = map[string]bool{"deterministic": true, "local_adapter": true, "model_adapter": true}
	validScopes     = map[string]bool{"owner": true, "workspace": true, "managed": true}
	validUnattended = map[string]bool{"deterministic_only": true, "policy_gated": true, "never": true}
	validWrites     = map[string]bool{
		"none": true, "memory_l1": true, "memory_rollups": true, "wiki_private": true,
		"runtime_index": true, "owner_observation": true, "local_diagnostics": true,
	}
)

type Catalog struct {
	SchemaVersion int    `json:"schema_version"`
	CatalogState  string `json:"catalog_state"`
	Jobs          []Job  `json:"jobs"`
}

type Job struct {
	ID                  string   `json:"id"`
	Category            string   `json:"category"`
	Trigger             string   `json:"trigger"`
	Executor            string   `json:"executor"`
	Scope               string   `json:"scope"`
	Availability        string   `json:"availability"`
	AvailabilityReason  string   `json:"availability_reason"`
	DefaultEnabled      bool     `json:"default_enabled"`
	Unattended          string   `json:"unattended"`
	Writes              []string `json:"writes"`
	SuccessBoundary     string   `json:"success_boundary"`
	QualificationDigest string   `json:"qualification_digest,omitempty"`
}

func Parse(reader io.Reader) (Catalog, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var catalog Catalog
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Catalog{}, errors.New("maintenance catalog contains multiple JSON values")
		}
		return Catalog{}, err
	}
	return catalog, catalog.Validate()
}

func LoadFile(path string) (Catalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return Catalog{}, err
	}
	defer file.Close()
	return Parse(file)
}

func (catalog Catalog) Validate() error {
	if catalog.SchemaVersion != SchemaVersion || (catalog.CatalogState != CatalogOnly && catalog.CatalogState != RuntimeQualified) {
		return errors.New("maintenance catalog has an invalid schema version or state")
	}
	if len(catalog.Jobs) == 0 {
		return errors.New("maintenance catalog must contain jobs")
	}
	seen := map[string]bool{}
	for _, job := range catalog.Jobs {
		if !validID(job.ID) || seen[job.ID] {
			return fmt.Errorf("maintenance catalog has invalid or duplicate job %q", job.ID)
		}
		seen[job.ID] = true
		if !validCategories[job.Category] || !validTriggers[job.Trigger] || !validExecutors[job.Executor] || !validScopes[job.Scope] {
			return fmt.Errorf("maintenance job %q has an invalid category, trigger, executor or scope", job.ID)
		}
		if job.SuccessBoundary == "" {
			return fmt.Errorf("maintenance job %q requires a success boundary", job.ID)
		}
		switch job.Availability {
		case Unavailable:
			if job.AvailabilityReason == "" || job.QualificationDigest != "" {
				return fmt.Errorf("unavailable maintenance job %q requires a reason and no qualification", job.ID)
			}
		case Available:
			if catalog.CatalogState != RuntimeQualified || job.AvailabilityReason != "" || !digestPattern.MatchString(job.QualificationDigest) {
				return fmt.Errorf("available maintenance job %q requires runtime-qualified evidence", job.ID)
			}
		default:
			return fmt.Errorf("maintenance job %q has invalid availability", job.ID)
		}
		if catalog.CatalogState == CatalogOnly && job.Availability != Unavailable {
			return fmt.Errorf("catalog-only maintenance job %q cannot be available", job.ID)
		}
		if !validUnattended[job.Unattended] {
			return fmt.Errorf("maintenance job %q has invalid unattended policy", job.ID)
		}
		if job.Executor == "deterministic" && job.Unattended != "deterministic_only" {
			return fmt.Errorf("deterministic job %q must use deterministic_only unattended policy", job.ID)
		}
		if job.Executor == "model_adapter" && job.Unattended == "deterministic_only" {
			return fmt.Errorf("model-backed job %q cannot use deterministic_only unattended policy", job.ID)
		}
		if job.Scope == "managed" && containsForbiddenWrite(job.Writes) {
			return fmt.Errorf("managed job %q cannot write user or private state", job.ID)
		}
		if len(job.Writes) == 0 {
			return fmt.Errorf("maintenance job %q must declare writes", job.ID)
		}
		writes := map[string]bool{}
		for _, write := range job.Writes {
			if !validWrites[write] || writes[write] {
				return fmt.Errorf("maintenance job %q has invalid or duplicate write %q", job.ID, write)
			}
			writes[write] = true
		}
		if writes["none"] && len(writes) != 1 {
			return fmt.Errorf("maintenance job %q cannot combine none with concrete writes", job.ID)
		}
	}
	for _, required := range []string{
		"memory-l1-capture", "memory-daily", "memory-weekly", "memory-retention-check",
		"wiki-incremental-sync", "wiki-reconcile", "wiki-integrity-check", "skills-index-refresh",
		"runtime-health-check", "capability-recheck", "runtime-drift-check", "self-observation-capture",
		"self-refinement-proposal", "update-check", "darwin-structural-evolution-proposal",
		"darwin-housekeeping-daily", "darwin-deep-weekly", "walter-self-review-weekly",
	} {
		if !seen[required] {
			return fmt.Errorf("maintenance catalog is missing required universal job %q", required)
		}
	}
	return nil
}

func (catalog Catalog) ForTrigger(trigger string) ([]Job, error) {
	if !validTriggers[trigger] {
		return nil, fmt.Errorf("invalid maintenance trigger %q", trigger)
	}
	var jobs []Job
	for _, job := range catalog.Jobs {
		if triggerMatches(job.Trigger, trigger) {
			jobs = append(jobs, job)
		}
	}
	sort.Slice(jobs, func(left, right int) bool { return jobs[left].ID < jobs[right].ID })
	return jobs, nil
}

func triggerMatches(jobTrigger, trigger string) bool {
	if jobTrigger == "lifecycle_cadence" {
		return trigger == "continuous" || trigger == "event" || trigger == "daily" || trigger == "weekly" || trigger == "presence"
	}
	if trigger == "continuous" {
		return jobTrigger == "event"
	}
	if jobTrigger == trigger || jobTrigger == trigger+"_or_presence" {
		return true
	}
	return trigger == "presence" && (jobTrigger == "daily_or_presence" || jobTrigger == "weekly_or_presence" || jobTrigger == "monthly_or_presence" || jobTrigger == "bundle_update_or_presence")
}

func containsForbiddenWrite(writes []string) bool {
	for _, write := range writes {
		if write == "memory_l1" || write == "memory_rollups" || write == "wiki_private" || write == "owner_observation" {
			return true
		}
	}
	return false
}

func validID(value string) bool {
	if len(value) < 2 || len(value) > 64 || value[0] < 'a' || value[0] > 'z' {
		return false
	}
	for _, character := range value {
		if !(character >= 'a' && character <= 'z' || character >= '0' && character <= '9' || character == '-') {
			return false
		}
	}
	return true
}
