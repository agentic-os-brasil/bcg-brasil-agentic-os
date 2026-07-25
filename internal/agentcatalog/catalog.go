// Package agentcatalog validates the runtime-neutral managed-agent catalog.
package agentcatalog

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Catalog struct {
	SchemaVersion int              `json:"schema_version"`
	Hub           string           `json:"hub"`
	Delegation    DelegationPolicy `json:"delegation"`
	Agents        []Agent          `json:"agents"`
}

type DelegationPolicy struct {
	Mode                string           `json:"mode"`
	RegisteredChains    string           `json:"registered_chains"`
	MaxActiveBranches   int              `json:"max_active_branches"`
	MaxDepth            int              `json:"max_depth"`
	MaxChildrenPerAgent int              `json:"max_children_per_agent"`
	MaxErrandHelpers    int              `json:"max_errand_helpers"`
	ErrandScope         string           `json:"errand_scope"`
	AllowedEdges        []DelegationEdge `json:"allowed_edges"`
}

type DelegationEdge struct {
	FromRole string   `json:"from_role"`
	ToRoles  []string `json:"to_roles"`
}

type Agent struct {
	ID               string `json:"id"`
	Role             string `json:"role"`
	DirectUserAccess bool   `json:"direct_user_access"`
	ToolAccess       string `json:"tool_access"`
	MayDelegate      bool   `json:"may_delegate"`
	InputContract    string `json:"input_contract"`
	RelativePath     string `json:"relative_path"`
}

var safeAgentID = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

var roleContracts = map[string]struct {
	direct        bool
	tools         string
	mayDelegate   bool
	inputContract string
}{
	"account_agent":         {false, "scoped", true, "bounded_account_packet"},
	"capability_specialist": {false, "scoped", false, "minimum_work_packet"},
	"errand_helper":         {false, "scoped", false, "bounded_errand_packet"},
	"governance_analyst":    {false, "none", false, "bounded_health_packet"},
	"hub":                   {true, "none", true, "session_context_packet"},
	"practice_agent":        {false, "scoped", true, "bounded_practice_packet"},
	"reviewer":              {false, "none", false, "sealed_review_packet"},
	"subject_specialist":    {false, "scoped", false, "bounded_subject_packet"},
	"workspace_agent":       {false, "scoped", true, "bounded_workspace_packet"},
}

func Parse(reader io.Reader) (Catalog, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var catalog Catalog
	if err := decoder.Decode(&catalog); err != nil {
		return Catalog{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return Catalog{}, errors.New("agent catalog contains multiple JSON values")
		}
		return Catalog{}, err
	}
	if err := catalog.Validate(); err != nil {
		return Catalog{}, err
	}
	return catalog, nil
}

func ParseFile(path string) (Catalog, error) {
	file, err := os.Open(path)
	if err != nil {
		return Catalog{}, err
	}
	defer file.Close()
	return Parse(file)
}

func (catalog Catalog) Validate() error {
	if catalog.SchemaVersion != 1 {
		return fmt.Errorf("agent catalog schema version %d is unsupported", catalog.SchemaVersion)
	}
	if catalog.Hub != "maestro" {
		return fmt.Errorf("agent catalog hub must be maestro, got %q", catalog.Hub)
	}
	if catalog.Delegation.Mode != "role_gated_chains" || catalog.Delegation.RegisteredChains != "governed_unbounded" || catalog.Delegation.MaxActiveBranches != 1 || catalog.Delegation.MaxDepth != 2 || catalog.Delegation.MaxChildrenPerAgent != 1 || catalog.Delegation.MaxErrandHelpers != 1 || catalog.Delegation.ErrandScope != "basic_reversible" {
		return errors.New("agent catalog must enforce governed chains, one active branch, one child and role-gated depth two")
	}
	if err := validateDelegationEdges(catalog.Delegation.AllowedEdges); err != nil {
		return err
	}
	if len(catalog.Agents) < 3 {
		return errors.New("agent catalog is missing the Maestro core")
	}

	seen := make(map[string]Agent, len(catalog.Agents))
	previous := ""
	directUsers := 0
	for _, agent := range catalog.Agents {
		if !safeAgentID.MatchString(agent.ID) || agent.ID <= previous {
			return errors.New("agent catalog IDs must be non-empty, unique and sorted")
		}
		if agent.RelativePath != "agents/"+agent.ID+"/AGENT.md" {
			return fmt.Errorf("agent %s has an invalid role, input contract or definition pointer", agent.ID)
		}
		contract, ok := roleContracts[agent.Role]
		if !ok {
			return fmt.Errorf("agent %s has unsupported role %q", agent.ID, agent.Role)
		}
		if agent.DirectUserAccess != contract.direct || agent.ToolAccess != contract.tools || agent.MayDelegate != contract.mayDelegate || agent.InputContract != contract.inputContract {
			return fmt.Errorf("agent %s violates the %s role contract", agent.ID, agent.Role)
		}
		if agent.DirectUserAccess {
			directUsers++
			if agent.ID != catalog.Hub {
				return fmt.Errorf("agent %s cannot address the user directly", agent.ID)
			}
		}
		if agent.MayDelegate && !catalog.roleMayDelegate(agent.Role) {
			return fmt.Errorf("agent %s has no authorized outgoing delegation edge", agent.ID)
		}
		seen[agent.ID] = agent
		previous = agent.ID
	}
	if directUsers != 1 {
		return errors.New("Maestro must be the sole direct user interface")
	}

	wanted := map[string]struct {
		role          string
		direct        bool
		tools         string
		mayDelegate   bool
		inputContract string
	}{
		"maestro": {"hub", true, "none", true, "session_context_packet"},
		"walter":  {"reviewer", false, "none", false, "sealed_review_packet"},
		"darwin":  {"governance_analyst", false, "none", false, "bounded_health_packet"},
	}
	for id, contract := range wanted {
		agent, ok := seen[id]
		if !ok {
			return fmt.Errorf("agent catalog is missing core agent %s", id)
		}
		if agent.Role != contract.role || agent.DirectUserAccess != contract.direct || agent.ToolAccess != contract.tools || agent.MayDelegate != contract.mayDelegate || agent.InputContract != contract.inputContract {
			return fmt.Errorf("core agent %s violates the lean orchestration contract", id)
		}
	}
	return nil
}

func (catalog Catalog) AllowsDelegation(fromRole, toRole string, depth int) bool {
	if depth < 1 || depth > catalog.Delegation.MaxDepth {
		return false
	}
	if (fromRole == "hub" && depth != 1) || (fromRole != "hub" && depth != 2) {
		return false
	}
	for _, edge := range catalog.Delegation.AllowedEdges {
		if edge.FromRole != fromRole {
			continue
		}
		for _, allowed := range edge.ToRoles {
			if allowed == toRole {
				return true
			}
		}
	}
	return false
}

func (catalog Catalog) roleMayDelegate(role string) bool {
	for _, edge := range catalog.Delegation.AllowedEdges {
		if edge.FromRole == role {
			return true
		}
	}
	return false
}

func validateDelegationEdges(edges []DelegationEdge) error {
	wanted := []DelegationEdge{
		{FromRole: "account_agent", ToRoles: []string{"capability_specialist"}},
		{FromRole: "hub", ToRoles: []string{"account_agent", "errand_helper", "governance_analyst", "practice_agent", "reviewer", "workspace_agent"}},
		{FromRole: "practice_agent", ToRoles: []string{"subject_specialist"}},
		{FromRole: "workspace_agent", ToRoles: []string{"capability_specialist"}},
	}
	if len(edges) != len(wanted) {
		return errors.New("agent catalog has an incomplete or unauthorized delegation graph")
	}
	for index, edge := range edges {
		expected := wanted[index]
		if edge.FromRole != expected.FromRole || len(edge.ToRoles) != len(expected.ToRoles) {
			return errors.New("agent catalog delegation edges must use the canonical sorted role graph")
		}
		for targetIndex, target := range edge.ToRoles {
			if target != expected.ToRoles[targetIndex] {
				return errors.New("agent catalog delegation edges must use the canonical sorted role graph")
			}
		}
	}
	return nil
}

func ValidateDir(root string) error {
	catalog, err := ParseFile(filepath.Join(root, "catalog.json"))
	if err != nil {
		return err
	}
	for _, agent := range catalog.Agents {
		definitionPath := filepath.Join(filepath.Dir(root), filepath.FromSlash(agent.RelativePath))
		relative, err := filepath.Rel(root, definitionPath)
		if err != nil || relative == "." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
			return fmt.Errorf("managed agent %s definition escapes the agent root", agent.ID)
		}
		body, err := os.ReadFile(definitionPath)
		if err != nil {
			return fmt.Errorf("read managed agent %s: %w", agent.ID, err)
		}
		if len(body) == 0 {
			return fmt.Errorf("managed agent %s has an empty definition", agent.ID)
		}
	}
	return nil
}
