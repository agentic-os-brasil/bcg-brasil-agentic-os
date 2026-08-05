package releasepack

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"testing"
)

func TestBaseSkillCatalogIsClosedByDistributionAllowlist(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	allowlist, err := LoadAllowlist(filepath.Join(root, "bundles", "base", "distribution.json"))
	if err != nil {
		t.Fatal(err)
	}
	allowed := make(map[string]bool, len(allowlist.Files))
	for _, entry := range allowlist.Files {
		allowed[entry.Path] = true
	}

	body, err := os.ReadFile(filepath.Join(root, "bundles", "base", "skills", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Skills []struct {
			ID           string `json:"id"`
			RelativePath string `json:"relative_path"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Skills) == 0 {
		t.Fatal("base skill catalog is empty")
	}
	for _, skill := range catalog.Skills {
		if !allowed[skill.RelativePath] {
			t.Errorf("catalog skill %q is missing from distribution allowlist: %s", skill.ID, skill.RelativePath)
		}
		projection := path.Join(path.Dir(skill.RelativePath), "agents", "openai.yaml")
		if !allowed[projection] {
			t.Errorf("catalog skill %q projection is missing from distribution allowlist: %s", skill.ID, projection)
		}
	}

	for _, schema := range []string{
		"schemas/sharepoint-project-source-selection.schema.json",
		"schemas/sharepoint-work-catalog.schema.json",
		"schemas/sharepoint-work-enrollment.schema.json",
		"schemas/sharepoint-work-import-receipt.schema.json",
	} {
		if !allowed[schema] {
			t.Errorf("prior-work contract schema is missing from distribution allowlist: %s", schema)
		}
	}
}

func TestTechCoreSkillCatalogIsClosedByDistributionAllowlist(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	allowlist, err := LoadAllowlist(filepath.Join(root, "bundles", "base", "distribution.json"))
	if err != nil {
		t.Fatal(err)
	}
	allowed := make(map[string]bool, len(allowlist.Files))
	for _, entry := range allowlist.Files {
		allowed[entry.Path] = true
	}
	body, err := os.ReadFile(filepath.Join(root, "bundles", "tech-core", "skills", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Skills []struct {
			ID           string `json:"id"`
			RelativePath string `json:"relative_path"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		t.Fatal(err)
	}
	if len(catalog.Skills) != 12 {
		t.Fatalf("tech-core skill count = %d, want 12", len(catalog.Skills))
	}
	for _, skill := range catalog.Skills {
		target := path.Join("bundles/tech-core", skill.RelativePath)
		if !allowed[target] {
			t.Errorf("tech-core skill %q is missing from distribution allowlist: %s", skill.ID, target)
		}
		projection := path.Join(path.Dir(target), "agents", "openai.yaml")
		if !allowed[projection] {
			t.Errorf("tech-core skill %q projection is missing from distribution allowlist: %s", skill.ID, projection)
		}
	}
}
