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
	if !strings.Contains(plan.Reason, "qualified runtime") {
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
	broken := strings.Replace(validCatalog, `"availability": "optional", "availability_reason": "release activation is available"`, `"availability": "included", "availability_reason": ""`, 1)
	if _, err := capabilitybundle.Parse(strings.NewReader(broken)); err == nil || !strings.Contains(err.Error(), "must be optional or unavailable") {
		t.Fatalf("Parse() error = %v", err)
	}
}

func TestParseRejectsDuplicateTrackAcrossBundles(t *testing.T) {
	broken := strings.Replace(validCatalog, `"tracks": ["software-engineering", "technical-explorer"]`, `"tracks": ["consulting", "technical-explorer"]`, 1)
	if _, err := capabilitybundle.Parse(strings.NewReader(broken)); err == nil || !strings.Contains(err.Error(), "claimed by bundles") {
		t.Fatalf("Parse() error = %v", err)
	}
}

func TestParseRejectsTwoNodeDependencyCycle(t *testing.T) {
	broken := strings.Replace(validCatalog, `"depends_on": ["base"]`, `"depends_on": ["data-practice"]`, 1)
	if _, err := capabilitybundle.Parse(strings.NewReader(broken)); err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("Parse() error = %v", err)
	}
}

func TestParseRejectsLongDependencyCycle(t *testing.T) {
	const longCycleCatalog = `{
  "schema_version": 1,
  "bundles": [
    {"id": "base", "display_name": "Base", "availability": "included", "availability_reason": "", "depends_on": [], "tracks": ["consulting"], "catalog_pointer": "bundles/base/skills/catalog.json"},
    {"id": "alpha", "display_name": "Alpha", "availability": "unavailable", "availability_reason": "release activation is not implemented", "depends_on": ["gamma"], "tracks": ["alpha-track"], "catalog_pointer": "bundles/alpha/skills/catalog.json"},
    {"id": "beta", "display_name": "Beta", "availability": "unavailable", "availability_reason": "release activation is not implemented", "depends_on": ["alpha"], "tracks": ["beta-track"], "catalog_pointer": "bundles/beta/skills/catalog.json"},
    {"id": "gamma", "display_name": "Gamma", "availability": "unavailable", "availability_reason": "release activation is not implemented", "depends_on": ["beta"], "tracks": ["gamma-track"], "catalog_pointer": "bundles/gamma/skills/catalog.json"}
  ]
}`
	if _, err := capabilitybundle.Parse(strings.NewReader(longCycleCatalog)); err == nil || !strings.Contains(err.Error(), "dependency cycle") {
		t.Fatalf("Parse() error = %v", err)
	}
}

const validCatalog = `{
  "schema_version": 1,
  "bundles": [
    {"id": "base", "display_name": "Base", "availability": "included", "availability_reason": "", "depends_on": [], "tracks": ["consulting"], "catalog_pointer": "bundles/base/skills/catalog.json"},
	    {"id": "engineering-core", "display_name": "Engineering Core", "availability": "optional", "availability_reason": "release activation is available", "depends_on": ["base"], "tracks": ["software-engineering", "technical-explorer"], "catalog_pointer": "bundles/engineering-core/skills/catalog.json"},
    {"id": "data-practice", "display_name": "Data Practice", "availability": "unavailable", "availability_reason": "release activation is not implemented", "depends_on": ["engineering-core"], "tracks": ["data-engineering", "data-science"], "catalog_pointer": "bundles/data-practice/skills/catalog.json"}
  ]
}`
