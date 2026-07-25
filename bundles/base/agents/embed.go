package baseagents

import (
	"bytes"
	_ "embed"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
)

//go:embed catalog.json
var catalogJSON []byte

func Catalog() (agentcatalog.Catalog, error) {
	return agentcatalog.Parse(bytes.NewReader(catalogJSON))
}
