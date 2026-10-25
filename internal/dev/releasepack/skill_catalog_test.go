package releasepack

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
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
	if len(catalog.Skills) != 39 {
		t.Fatalf("tech-core skill count = %d, want 39 (12 existing and 27 stable Pocock methods)", len(catalog.Skills))
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

func TestPocockStableSkillsetShipsCompletePackages(t *testing.T) {
	_, sourceFile, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(sourceFile), "..", "..", ".."))
	allowlist, err := LoadAllowlist(filepath.Join(root, "bundles", "base", "distribution.json"))
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]string{}
	for _, entry := range allowlist.Files {
		allowed[entry.Path] = entry.Source
	}
	names := strings.Fields("ask-matt diagnosing-bugs grill-with-docs triage improve-codebase-architecture setup-matt-pocock-skills tdd to-spec to-tickets wayfinder implement implement-spec prototype research domain-modeling codebase-design code-review pr retro wizard grill-me grilling handoff teach to-questionnaire wait-what writing-for-agents")
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			relative := "bundles/tech-core/skills/pocock-" + name
			body, err := os.ReadFile(filepath.Join(root, relative, "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(body), "name: pocock-"+name) || !strings.Contains(string(body), "$interaction-profile") {
				t.Fatal("skill lacks namespaced identity or canonical interaction profile")
			}
			for _, required := range []string{"SKILL.md", "LICENSE", "agents/openai.yaml"} {
				if _, err := os.Stat(filepath.Join(root, relative, required)); err != nil {
					t.Fatal(err)
				}
			}
			if err := filepath.WalkDir(filepath.Join(root, relative), func(file string, entry os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if entry.IsDir() {
					return nil
				}
				resource, err := filepath.Rel(root, file)
				if err != nil {
					return err
				}
				resource = filepath.ToSlash(resource)
				if allowed[resource] != resource {
					t.Errorf("package resource lacks exact distribution mapping: %s", resource)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
	provenance := "bundles/tech-core/skills/pocock-upstream.json"
	if allowed[provenance] != provenance {
		t.Error("upstream provenance is not distributed")
	}
	body, err := os.ReadFile(filepath.Join(root, provenance))
	if err != nil {
		t.Fatal(err)
	}
	var pin struct {
		Revision string `json:"upstream_revision"`
		Version  string `json:"upstream_version"`
		License  string `json:"license"`
		Skills   []struct {
			ID       string `json:"id"`
			UserOnly bool   `json:"user_invoked_only"`
			Files    []struct {
				Path        string `json:"path"`
				SourceHash  string `json:"upstream_sha256"`
				ShippedHash string `json:"shipped_sha256"`
			} `json:"files"`
		} `json:"skills"`
	}
	if err := json.Unmarshal(body, &pin); err != nil {
		t.Fatal(err)
	}
	if pin.Revision != "49dd158d1076134a641b33efb035946536778336" || pin.Version != "1.3.1" || pin.License != "MIT" || len(pin.Skills) != 27 {
		t.Fatalf("unexpected upstream pin: %+v", pin)
	}
	seen := map[string]bool{}
	manualOnly := 0
	resources := 0
	for _, skill := range pin.Skills {
		if seen[skill.ID] {
			t.Fatalf("duplicate provenance skill: %s", skill.ID)
		}
		seen[skill.ID] = true
		if skill.UserOnly {
			manualOnly++
		}
		base := filepath.Join(root, "bundles", "tech-core", "skills", skill.ID)
		for _, resource := range skill.Files {
			if filepath.Clean(resource.Path) != resource.Path || filepath.IsAbs(resource.Path) || strings.HasPrefix(resource.Path, "..") {
				t.Fatalf("unsafe resource pointer: %s", resource.Path)
			}
			body, err := os.ReadFile(filepath.Join(base, filepath.FromSlash(resource.Path)))
			if err != nil {
				t.Fatal(err)
			}
			if len(resource.SourceHash) != 64 || fmt.Sprintf("%x", sha256.Sum256(body)) != resource.ShippedHash {
				t.Errorf("provenance drift: %s/%s", skill.ID, resource.Path)
			}
			resources++
		}
		front, err := os.ReadFile(filepath.Join(base, "SKILL.md"))
		if err != nil {
			t.Fatal(err)
		}
		projection, err := os.ReadFile(filepath.Join(base, "agents", "openai.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(front), "disable-model-invocation: true") != skill.UserOnly || strings.Contains(string(projection), "allow_implicit_invocation: false") != skill.UserOnly {
			t.Errorf("invocation policy drift: %s", skill.ID)
		}
	}
	if manualOnly != 16 || resources != 106 {
		t.Fatalf("incomplete provenance: %d manual methods, %d resources", manualOnly, resources)
	}
}
