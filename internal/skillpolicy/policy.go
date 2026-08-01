// Package skillpolicy binds managed methods to governed agent roles without
// turning skills into tool, data, memory or delegation authority.
package skillpolicy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/agentcatalog"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/skillsindex"
)

type Policy struct {
	SchemaVersion int             `json:"schema_version"`
	Mode          string          `json:"mode"`
	Direct        []DirectRule    `json:"direct"`
	Delegated     []DelegatedRule `json:"delegated"`
}

type DirectRule struct {
	Role     string   `json:"role"`
	SkillIDs []string `json:"skill_ids"`
}

type DelegatedRule struct {
	FromRole string   `json:"from_role"`
	ToRole   string   `json:"to_role"`
	SkillIDs []string `json:"skill_ids"`
}

// Registry is a policy compiled against the managed skills catalog. It is the
// only form accepted by dispatch code, so an unvalidated policy cannot grant a
// method selection through a packet.
type Registry struct {
	policy Policy
}

func ParseFile(path string) (Policy, error) {
	file, err := os.Open(path)
	if err != nil {
		return Policy{}, err
	}
	defer file.Close()
	return Parse(file)
}

func Parse(reader io.Reader) (Policy, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var policy Policy
	if err := decoder.Decode(&policy); err != nil {
		return Policy{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Policy{}, errors.New("agent skill policy contains multiple JSON values")
		}
		return Policy{}, err
	}
	return policy, nil
}

func (policy Policy) Validate(skills skillsindex.Catalog, agents agentcatalog.Catalog) error {
	if err := skills.Validate(); err != nil {
		return err
	}
	if err := agents.Validate(); err != nil {
		return err
	}
	if policy.SchemaVersion != 1 || policy.Mode != "methods_not_authority" {
		return errors.New("agent skill policy has an unsupported contract")
	}
	known := make(map[string]bool, len(skills.Skills))
	for _, skill := range skills.Skills {
		known[skill.ID] = true
	}
	if err := validateDirect(policy.Direct, known); err != nil {
		return err
	}
	return validateDelegated(policy.Delegated, known, agents)
}

func Compile(policy Policy, skills skillsindex.Catalog, agents agentcatalog.Catalog) (Registry, error) {
	if err := policy.Validate(skills, agents); err != nil {
		return Registry{}, err
	}
	return Registry{policy: policy}, nil
}

func (policy Policy) AllowsDirect(role, skillID string) bool {
	role = canonicalRole(role)
	for _, rule := range policy.Direct {
		if canonicalRole(rule.Role) == role && contains(rule.SkillIDs, skillID) {
			return true
		}
	}
	return false
}

func (policy Policy) AllowsDelegated(fromRole, toRole, skillID string) bool {
	fromRole, toRole = canonicalRole(fromRole), canonicalRole(toRole)
	for _, rule := range policy.Delegated {
		if canonicalRole(rule.FromRole) == fromRole && canonicalRole(rule.ToRole) == toRole && contains(rule.SkillIDs, skillID) {
			return true
		}
	}
	return false
}

func (registry Registry) AllowsDirect(role, skillID string) bool {
	return registry.policy.AllowsDirect(role, skillID)
}

func (registry Registry) AllowsDelegated(fromRole, toRole, skillID string) bool {
	return registry.policy.AllowsDelegated(fromRole, toRole, skillID)
}

func validateDirect(rules []DirectRule, known map[string]bool) error {
	previous := ""
	for _, rule := range rules {
		if !directRole(rule.Role) || rule.Role <= previous || len(rule.SkillIDs) == 0 {
			return errors.New("agent skill direct rules are invalid or unsorted")
		}
		if err := validateSkills(rule.SkillIDs, known); err != nil {
			return fmt.Errorf("direct role %s: %w", rule.Role, err)
		}
		previous = rule.Role
	}
	return nil
}

func directRole(role string) bool {
	return canonicalRole(role) == "case_agent"
}

func validateDelegated(rules []DelegatedRule, known map[string]bool, agents agentcatalog.Catalog) error {
	_ = known
	_ = agents
	if len(rules) != 0 {
		return errors.New("delegated skill rules are forbidden at Maestro depth one")
	}
	return nil
}

func canonicalRole(role string) string {
	switch role {
	case "account_agent":
		return "client_account_agent"
	case "workspace_agent":
		return "case_agent"
	default:
		return role
	}
}

func validateSkills(skillIDs []string, known map[string]bool) error {
	if !sort.StringsAreSorted(skillIDs) {
		return errors.New("skill IDs must be sorted")
	}
	previous := ""
	for _, skillID := range skillIDs {
		if skillID == "" || skillID == previous || !known[skillID] {
			return errors.New("skill IDs must be unique managed skills")
		}
		previous = skillID
	}
	return nil
}

func contains(values []string, wanted string) bool {
	index := sort.SearchStrings(values, wanted)
	return index < len(values) && values[index] == wanted
}
