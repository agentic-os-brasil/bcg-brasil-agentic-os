package releasepack

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
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

func TestCaseAgentSetupUsesCanonicalSkillAndExplicitLegacyAlias(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	body, err := os.ReadFile(filepath.Join(root, "bundles", "base", "skills", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Skills []struct {
			ID            string `json:"id"`
			DisplayName   string `json:"display_name"`
			DefaultPrompt string `json:"default_prompt"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(body, &catalog); err != nil {
		t.Fatal(err)
	}
	var canonical, alias *struct {
		ID            string
		DisplayName   string
		DefaultPrompt string
	}
	for index := range catalog.Skills {
		skill := &catalog.Skills[index]
		switch skill.ID {
		case "case-agent-setup":
			canonical = &struct {
				ID            string
				DisplayName   string
				DefaultPrompt string
			}{skill.ID, skill.DisplayName, skill.DefaultPrompt}
		case "workspace-agent-setup":
			alias = &struct {
				ID            string
				DisplayName   string
				DefaultPrompt string
			}{skill.ID, skill.DisplayName, skill.DefaultPrompt}
		}
	}
	// The guarantee here is the routing shape, not the wording: the canonical
	// entry must invoke itself, and the alias must redirect to the canonical id
	// instead of standing on its own. Asserting the literal English copy would
	// also pin the catalog's language, which is a separate product decision.
	if canonical == nil || canonical.DisplayName == "" || !strings.Contains(canonical.DefaultPrompt, "$case-agent-setup") {
		t.Fatalf("canonical case-agent-setup entry missing or no longer invokes itself: %#v", canonical)
	}
	if alias == nil || alias.DisplayName == "" || alias.DefaultPrompt == "" {
		t.Fatalf("workspace-agent-setup alias entry is missing or empty: %#v", alias)
	}
	if !strings.Contains(alias.DefaultPrompt, "$case-agent-setup") || strings.Contains(alias.DefaultPrompt, "$workspace-agent-setup") {
		t.Fatalf("workspace-agent-setup must redirect to $case-agent-setup rather than invoke itself: %#v", alias)
	}
	for _, relative := range []string{
		"bundles/base/skills/case-agent-setup/SKILL.md",
		"bundles/base/skills/case-agent-setup/agents/openai.yaml",
		"bundles/base/skills/workspace-agent-setup/SKILL.md",
		"bundles/base/skills/workspace-agent-setup/agents/openai.yaml",
	} {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			t.Fatalf("skill migration artifact %s is missing: %v", relative, err)
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
