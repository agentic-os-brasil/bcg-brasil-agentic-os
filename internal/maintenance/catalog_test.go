package maintenance

import (
	"strings"
	"testing"
)

func TestCatalogRejectsUnknownFields(t *testing.T) {
	_, err := Parse(strings.NewReader(`{"schema_version":1,"catalog_state":"catalog_only","jobs":[],"unknown":true}`))
	if err == nil {
		t.Fatal("unknown catalog field was accepted")
	}
}

func TestCatalogRequiresUniversalMaintenancePlane(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Validate(); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Jobs) != 14 {
		t.Fatalf("job count = %d, want 14", len(catalog.Jobs))
	}
	for _, job := range catalog.Jobs {
		if job.Availability != Unavailable {
			t.Fatalf("job %s was promoted from catalog-only state", job.ID)
		}
	}
}

func TestPresenceIncludesDailyAndWeeklyCatchUpButNotEventOnlyJobs(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	jobs, err := catalog.ForTrigger("presence")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, job := range jobs {
		seen[job.ID] = true
	}
	if !seen["memory-daily"] || !seen["memory-weekly"] || !seen["wiki-reconcile"] {
		t.Fatalf("presence catch-up omitted core jobs: %#v", seen)
	}
	if seen["wiki-incremental-sync"] {
		t.Fatal("event-only wiki sync was incorrectly scheduled by presence")
	}
}

func TestManagedJobsCannotWritePrivateState(t *testing.T) {
	catalog := Catalog{
		SchemaVersion: 1,
		CatalogState:  CatalogOnly,
		Jobs: []Job{{
			ID: "managed-health", Category: "runtime", Trigger: "daily", Executor: "deterministic", Scope: "managed",
			Availability: Unavailable, AvailabilityReason: "not installed", DefaultEnabled: true,
			Unattended: "deterministic_only", Writes: []string{"wiki_private"}, SuccessBoundary: "check",
		}},
	}
	if err := catalog.Validate(); err == nil {
		t.Fatal("managed job with private writes was accepted")
	}
}

func TestPublishedSchemaCompilesAndMatchesCatalog(t *testing.T) {
	if err := ValidateSchemaFile("../../schemas/maintenance-jobs.schema.json"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateSchemaAndCatalog("../../schemas/maintenance-jobs.schema.json", "../../bundles/base/runtime/maintenance.json"); err != nil {
		t.Fatal(err)
	}
}

func TestCatalogRejectsMixedNoneWrites(t *testing.T) {
	catalog := Catalog{
		SchemaVersion: 1,
		CatalogState:  CatalogOnly,
		Jobs: []Job{{
			ID: "mixed-writes", Category: "runtime", Trigger: "daily", Executor: "deterministic", Scope: "owner",
			Availability: Unavailable, AvailabilityReason: "not installed", DefaultEnabled: true,
			Unattended: "deterministic_only", Writes: []string{"none", "runtime_index"}, SuccessBoundary: "check",
		}},
	}
	if err := catalog.Validate(); err == nil {
		t.Fatal("mixed none and concrete writes were accepted")
	}
}
