package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	tech "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/tech-core/skills"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

func TestFactoryProjectsCompleteNativeMethodPackages(t *testing.T) {
	root := t.TempDir()
	if err := run([]string{"--root", root}); err != nil {
		t.Fatal(err)
	}
	canonical, err := tech.Files("pocock-tdd")
	if err != nil {
		t.Fatal(err)
	}
	for _, surface := range []string{".claude/skills", ".agents/skills"} {
		provenance, err := os.ReadFile(filepath.Join(root, surface, "pocock-upstream.json"))
		if err != nil || !bytes.Equal(provenance, tech.Provenance()) {
			t.Fatalf("missing source provenance in %s: %v", surface, err)
		}
		body, err := os.ReadFile(filepath.Join(root, surface, "catalog.json"))
		if err != nil {
			t.Fatal(err)
		}
		var catalog skillsindex.Catalog
		if err := json.Unmarshal(body, &catalog); err != nil || len(catalog.Skills) != 92 {
			t.Fatalf("%s: count=%d err=%v", surface, len(catalog.Skills), err)
		}
		for _, resource := range []string{"SKILL.md", "tests.md", "mocking.md", "LICENSE", "agents/openai.yaml"} {
			body, err := os.ReadFile(filepath.Join(root, surface, "pocock-tdd", resource))
			if err != nil || !bytes.Equal(body, canonical[resource]) {
				t.Fatalf("%s/%s differs or absent: %v", surface, resource, err)
			}
		}
	}
}

func TestFactoryPreservesExistingNativeSkills(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, ".claude", "skills", "pocock-tdd", "tests.md")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("owner method"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"--root", root}); err == nil {
		t.Fatal("factory accepted an existing discovery tree")
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != "owner method" {
		t.Fatal("factory modified an unowned file")
	}
	if _, err := os.Stat(filepath.Join(root, ".agents")); !os.IsNotExist(err) {
		t.Fatal("failed preflight wrote another discovery tree")
	}
}
