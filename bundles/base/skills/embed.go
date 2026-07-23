package baseskills

import (
	"bytes"
	_ "embed"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

//go:embed catalog.json
var catalogJSON []byte

func Catalog() (skillsindex.Catalog, error) {
	return skillsindex.Parse(bytes.NewReader(catalogJSON))
}
