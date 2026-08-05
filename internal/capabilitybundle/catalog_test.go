package capabilitybundle_test

import (
	"strings"
	"testing"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/capabilitybundle"
)

func TestPlanForDataScienceResolvesOptionalDependencies(t *testing.T) {
	catalog, err := capabilitybundle.Parse(strings.NewReader(validCatalog))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := catalog.PlanForTracks([]string{"data-science"})
	if err != nil {
		t.Fatal(err)
	}
	if plan.State != capabilitybundle.Optional || len(plan.Bundles) != 2 || plan.Bundles[0].ID != "base" || plan.Bundles[1].ID != "tech-core" {
		t.Fatalf("plan = %#v", plan)
	}
	if !strings.Contains(plan.Reason, "confirmed interview") {
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
	if _, err := capabilitybundle.Parse(strings.NewReader(broken)); err == nil || !strings.Contains(err.Error(), "must be optional with a reason") {
		t.Fatalf("Parse() error = %v", err)
	}
}

func TestParseRejectsDuplicateTrackAcrossBundles(t *testing.T) {
	broken := strings.Replace(validCatalog, `"tracks": ["software-engineering", "technical-explorer", "data-engineering", "data-science"]`, `"tracks": ["consulting", "technical-explorer", "data-engineering", "data-science"]`, 1)
	if _, err := capabilitybundle.Parse(strings.NewReader(broken)); err == nil || !strings.Contains(err.Error(), "claimed by bundles") {
		t.Fatalf("Parse() error = %v", err)
	}
}

func TestParseRejectsSelfDependency(t *testing.T) {
	broken := strings.Replace(validCatalog, `"depends_on": ["base"]`, `"depends_on": ["tech-core"]`, 1)
	if _, err := capabilitybundle.Parse(strings.NewReader(broken)); err == nil || !strings.Contains(err.Error(), "invalid dependency") {
		t.Fatalf("Parse() error = %v", err)
	}
}

func TestParseRejectsLongDependencyCycle(t *testing.T) {
	const longCycleCatalog = `{
  "schema_version": 1,
  "bundles": [
    {"id": "base", "display_name": "Base", "availability": "included", "availability_reason": "", "depends_on": [], "tracks": ["consulting"], "catalog_pointer": "bundles/base/skills/catalog.json"},
    {"id": "alpha", "display_name": "Alpha", "availability": "optional", "availability_reason": "release activation is available", "depends_on": ["gamma"], "tracks": ["alpha-track"], "catalog_pointer": "bundles/alpha/skills/catalog.json"},
    {"id": "beta", "display_name": "Beta", "availability": "optional", "availability_reason": "release activation is available", "depends_on": ["alpha"], "tracks": ["beta-track"], "catalog_pointer": "bundles/beta/skills/catalog.json"},
    {"id": "gamma", "display_name": "Gamma", "availability": "optional", "availability_reason": "release activation is available", "depends_on": ["beta"], "tracks": ["gamma-track"], "catalog_pointer": "bundles/gamma/skills/catalog.json"}
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
    {"id": "tech-core", "display_name": "Tech Core", "availability": "optional", "availability_reason": "release activation is available", "depends_on": ["base"], "tracks": ["software-engineering", "technical-explorer", "data-engineering", "data-science"], "catalog_pointer": "bundles/tech-core/skills/catalog.json"}
  ]
}`
