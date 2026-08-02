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
	if len(catalog.Jobs) != 17 {
		t.Fatalf("job count = %d, want 17", len(catalog.Jobs))
	}
	for _, job := range catalog.Jobs {
		if job.Availability != Unavailable {
			t.Fatalf("job %s was promoted from catalog-only state", job.ID)
		}
	}
	monthly, found := findJob(catalog.Jobs, "darwin-structural-evolution-proposal")
	if !found || monthly.Trigger != "monthly_or_presence" || monthly.Executor != "local_adapter" || monthly.DefaultEnabled || monthly.Unattended != "never" {
		t.Fatalf("monthly Darwin proposal gained an unsafe default: %#v", monthly)
	}
	if _, found := findJob(catalog.Jobs, "self-refinement-proposal"); found {
		t.Fatal("retired generic self-refinement job remains in the canonical catalog")
	}
	walter, found := findJob(catalog.Jobs, "walter-self-review-weekly")
	if !found || walter.Executor != "model_adapter" || walter.DefaultEnabled || walter.Unattended != "policy_gated" || !strings.Contains(walter.SuccessBoundary, "silent self-ingestion") || strings.Contains(walter.SuccessBoundary, "proposal") {
		t.Fatalf("Walter weekly silent-ingestion contract drifted: %#v", walter)
	}
}

func findJob(jobs []Job, id string) (Job, bool) {
	for _, job := range jobs {
		if job.ID == id {
			return job, true
		}
	}
	return Job{}, false
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
	if !seen["memory-daily"] || !seen["memory-weekly"] || !seen["wiki-reconcile"] || !seen["darwin-structural-evolution-proposal"] {
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

func TestCatalogFixesWalterAndDarwinStructuralSafetyTuples(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	for index := range catalog.Jobs {
		switch catalog.Jobs[index].ID {
		case "walter-self-review-weekly":
			catalog.Jobs[index].Executor = "deterministic"
			if err := catalog.Validate(); err == nil {
				t.Fatal("catalog accepted an unsafe Walter execution tuple")
			}
			return
		}
	}
	t.Fatal("Walter weekly job was not found")
}

func TestCatalogFixesDarwinStructuralSafetyTuple(t *testing.T) {
	catalog, err := LoadFile("../../bundles/base/runtime/maintenance.json")
	if err != nil {
		t.Fatal(err)
	}
	for index := range catalog.Jobs {
		if catalog.Jobs[index].ID != "darwin-structural-evolution-proposal" {
			continue
		}
		catalog.Jobs[index].Unattended = "policy_gated"
		if err := catalog.Validate(); err == nil {
			t.Fatal("catalog accepted an unattended Darwin structural evolution tuple")
		}
		return
	}
	t.Fatal("Darwin structural job was not found")
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
