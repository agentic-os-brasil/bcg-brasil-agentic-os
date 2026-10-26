package baseskills

import (
	"bytes"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

//go:embed catalog.json
var catalogJSON []byte

//go:embed agent-skill-policy.json
var agentSkillPolicyJSON []byte

//go:embed all:*/*
var skillFiles embed.FS

func Catalog() (skillsindex.Catalog, error) {
	return skillsindex.Parse(bytes.NewReader(catalogJSON))
}

func AgentSkillPolicy() []byte {
	return append([]byte(nil), agentSkillPolicyJSON...)
}

func Skill(id string) ([]byte, error) {
	if id == "" {
		return nil, fmt.Errorf("skill ID is required")
	}
	body, err := skillFiles.ReadFile(id + "/SKILL.md")
	if err != nil {
		return nil, fmt.Errorf("read embedded skill %s: %w", id, err)
	}
	return append([]byte(nil), body...), nil
}

// Files returns independent copies of every resource belonging to a catalogued
// method. Paths are relative to its skill directory, never to the factory root.
func Files(id string) (map[string][]byte, error) {
	catalog, err := Catalog()
	if err != nil {
		return nil, err
	}
	known := false
	for _, skill := range catalog.Skills {
		if skill.ID == id {
			known = true
			break
		}
	}
	if !known {
		return nil, fmt.Errorf("unknown embedded skill %q", id)
	}
	files := make(map[string][]byte)
	err = fs.WalkDir(skillFiles, id, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("embedded skill resource is not regular: %s", path)
		}
		relative := strings.TrimPrefix(path, id+"/")
		if !fs.ValidPath(relative) {
			return fmt.Errorf("unsafe embedded skill resource %q", relative)
		}
		body, err := skillFiles.ReadFile(path)
		if err != nil {
			return err
		}
		files[relative] = append([]byte(nil), body...)
		return nil
	})
	return files, err
}
