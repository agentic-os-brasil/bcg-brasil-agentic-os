package baseagents

import (
	"bytes"
	"embed"
	"fmt"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
)

//go:embed catalog.json
var catalogJSON []byte

//go:embed templates/*/AGENT.md
var templates embed.FS

func Catalog() (agentcatalog.Catalog, error) {
	return agentcatalog.Parse(bytes.NewReader(catalogJSON))
}

func Template(role string) ([]byte, error) {
	switch role {
	case "account_agent", "practice_agent", "workspace_agent", "capability_specialist", "subject_specialist":
	default:
		return nil, fmt.Errorf("no managed scaffold template for role %q", role)
	}
	body, err := templates.ReadFile("templates/" + role + "/AGENT.md")
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), body...), nil
}
