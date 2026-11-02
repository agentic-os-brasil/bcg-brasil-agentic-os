package maintenance

import (
	catalog "github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/maintenancecatalog"
	"io"
)

type Catalog = catalog.Catalog
type Job = catalog.Job

const (
	SchemaVersion         = catalog.SchemaVersion
	CatalogOnly           = catalog.CatalogOnly
	RuntimeQualified      = catalog.RuntimeQualified
	Available             = catalog.Available
	Unavailable           = catalog.Unavailable
	MemoryCheckpointJobID = catalog.MemoryCheckpointJobID
	MemoryLightDreamJobID = catalog.MemoryLightDreamJobID
	MemoryDeepDreamJobID  = catalog.MemoryDeepDreamJobID
)

func Parse(r io.Reader) (Catalog, error) { return catalog.Parse(r) }
func LoadFile(p string) (Catalog, error) { return catalog.LoadFile(p) }
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

func validWrite(write string) bool { return catalog.ValidWrite(write) }
