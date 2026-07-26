package capabilitybundle_test

import (
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/capabilitybundle"
)

func TestPlanForDataScienceResolvesDependenciesButDoesNotActivateBundles(t *testing.T) {
	catalog, err := capabilitybundle.Parse(strings.NewReader(validCatalog))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := catalog.PlanForTracks([]string{"data-science"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != capabilitybundle.Unavailable || len(plan.Bundles) != 3 || plan.Bundles[0].ID != "base" || plan.Bundles[1].ID != "data-practice" || plan.Bundles[2].ID != "engineering-core" {
		t.Fatalf("plan = %#v", plan)
	}
	if !strings.Contains(plan.Reason, "not implemented") {
		t.Fatalf("reason = %q", plan.Reason)
	}
}

func TestPlanForConsultingKeepsTheBaseBundleOnly(t *testing.T) {
	catalog, err := capabilitybundle.Parse(strings.NewReader(validCatalog))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := catalog.PlanForTracks([]string{"consulting"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != "base_only" || len(plan.Bundles) != 1 || plan.Bundles[0].ID != "base" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestParseRejectsOptionalBundleThatClaimsActivation(t *testing.T) {
	broken := strings.Replace(validCatalog, `"availability": "unavailable", "availability_reason": "release activation is not implemented"`, `"availability": "included", "availability_reason": ""`, 1)
	if _, err := capabilitybundle.Parse(strings.NewReader(broken)); err == nil || !strings.Contains(err.Error(), "must remain explicitly unavailable") {
		t.Fatalf("Parse() error = %v", err)
	}
}

const validCatalog = `{
  "schema_version": 1,
  "bundles": [
    {"id": "base", "display_name": "Base", "availability": "included", "availability_reason": "", "depends_on": [], "tracks": ["consulting"], "catalog_pointer": "bundles/base/skills/catalog.json"},
    {"id": "engineering-core", "display_name": "Engineering Core", "availability": "unavailable", "availability_reason": "release activation is not implemented", "depends_on": ["base"], "tracks": ["software-engineering", "technical-explorer"], "catalog_pointer": "bundles/engineering-core/skills/catalog.json"},
    {"id": "data-practice", "display_name": "Data Practice", "availability": "unavailable", "availability_reason": "release activation is not implemented", "depends_on": ["engineering-core"], "tracks": ["data-engineering", "data-science"], "catalog_pointer": "bundles/data-practice/skills/catalog.json"}
  ]
}`
