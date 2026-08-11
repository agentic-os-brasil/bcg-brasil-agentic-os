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

//go:embed pa-expert-registry.json
var paExpertRegistryJSON []byte

//go:embed templates/*/AGENT.md
var templates embed.FS

//go:embed case-agent/AGENT.md client-account-agent/AGENT.md darwin/AGENT.md pa-expert/AGENT.md walter/AGENT.md
var definitions embed.FS

func Catalog() (agentcatalog.Catalog, error) {
	return agentcatalog.Parse(bytes.NewReader(catalogJSON))
}

type PAExpertRegistry struct {
	SchemaVersion int                         `json:"schema_version"`
	Authority     string                      `json:"authority"`
	Experts       []activationpolicy.PAExpert `json:"experts"`
}

func ManagedPAExpertRegistry() (PAExpertRegistry, error) {
	var registry PAExpertRegistry
	if err := activationpolicy.DecodeStrict(paExpertRegistryJSON, &registry); err != nil {
		return PAExpertRegistry{}, err
	}
	if registry.SchemaVersion != 2 || registry.Authority != "pa-expert-registry-v2" {
		return PAExpertRegistry{}, errors.New("managed PA Expert registry authority is invalid")
	}
	previous := ""
	for _, expert := range registry.Experts {
		if expert.ID <= previous || !activationpolicy.IsValidPublishedPAExpert(expert) {
			return PAExpertRegistry{}, errors.New("managed PA Experts must be valid, unique and sorted")
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
	case "case_agent", "client_account_agent", "pa_expert", "quality_guardian":
	default:
		return nil, fmt.Errorf("no managed scaffold template for role %q", role)
	}
	body, err := templates.ReadFile("templates/" + role + "/AGENT.md")
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), body...), nil
}

// Definition returns a canonical managed specialist prompt. Maestro is
// intentionally absent because the main session owns the hub role.
func Definition(id string) ([]byte, error) {
	switch id {
	case "case-agent", "client-account-agent", "darwin", "pa-expert", "walter":
	default:
		return nil, fmt.Errorf("no managed native definition for %q", id)
	}
	body, err := definitions.ReadFile(id + "/AGENT.md")
	if err != nil {
		return nil, err
	}
	return append([]byte(nil), body...), nil
}
