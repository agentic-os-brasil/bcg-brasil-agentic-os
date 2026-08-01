package engineeringcoreskills

import (
	"bytes"
	"embed"
	"fmt"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

//go:embed catalog.json
var catalogJSON []byte

//go:embed */SKILL.md
var skillFiles embed.FS

func Catalog() (skillsindex.Catalog, error) {
	return skillsindex.Parse(bytes.NewReader(catalogJSON))
}

func Skill(id string) ([]byte, error) {
	if id == "" {
		return nil, fmt.Errorf("skill ID is required")
	}
	body, err := skillFiles.ReadFile(id + "/SKILL.md")
	if err != nil {
		return nil, fmt.Errorf("read embedded engineering skill %s: %w", id, err)
	}
	return append([]byte(nil), body...), nil
}
