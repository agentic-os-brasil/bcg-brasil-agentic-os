package baseagents

import (
	"bytes"
	"embed"
	"errors"
	"fmt"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/activationpolicy"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
)

//go:embed catalog.json
var catalogJSON []byte

//go:embed helix-registry.json
var helixRegistryJSON []byte

//go:embed templates/*/AGENT.md
var templates embed.FS

func Catalog() (agentcatalog.Catalog, error) {
	return agentcatalog.Parse(bytes.NewReader(catalogJSON))
}

type HelixRegistry struct {
	SchemaVersion int                         `json:"schema_version"`
	Authority     string                      `json:"authority"`
	Experts       []activationpolicy.PAExpert `json:"experts"`
}

func ManagedHelixRegistry() (HelixRegistry, error) {
	var registry HelixRegistry
	if err := activationpolicy.DecodeStrict(helixRegistryJSON, &registry); err != nil {
		return HelixRegistry{}, err
	}
	if registry.SchemaVersion != 1 || registry.Authority != "helix-brasil" {
		return HelixRegistry{}, errors.New("managed Helix registry authority is invalid")
	}
	previous := ""
	for _, expert := range registry.Experts {
		if expert.ID <= previous || !activationpolicy.IsValidPublishedPAExpert(expert) {
			return HelixRegistry{}, errors.New("managed Helix experts must be valid, unique and sorted")
		}
		previous = expert.ID
	}
	return registry, nil
}

func Template(role string) ([]byte, error) {
	if role == "account_agent" {
		role = "client_account_agent"
	}
	if role == "workspace_agent" {
		role = "case_agent"
	}
	switch role {
	case "case_agent", "client_account_agent", "pa_expert", "practice_agent", "capability_specialist", "subject_specialist":
	default:
		return nil, fmt.Errorf("no managed scaffold template for role %q", role)
	}
	body, err := templates.ReadFile("templates/" + role + "/AGENT.md")
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), body...), nil
}
