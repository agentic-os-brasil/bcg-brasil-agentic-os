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
	SchemaVersion  int                          `json:"schema_version"`
	Hub            string                       `json:"hub"`
	Delegation     DelegationPolicy             `json:"delegation"`
	NativeAdvisory NativeAdvisoryPolicy         `json:"native_advisory"`
	LegacyAliases  map[string]LegacyRoleAlias   `json:"legacy_aliases"`
	LegacyIDs      map[string]LegacyIDMigration `json:"legacy_ids"`
	Agents         []Agent                      `json:"agents"`
}

// NativeAdvisoryPolicy describes bounded consultations that a model-backed
// host runtime may perform. It is deliberately separate from DelegationPolicy,
// which remains the strict signed-packet replay backend.
type NativeAdvisoryPolicy struct {
	Mode                string           `json:"mode"`
	MaxDepth            int              `json:"max_depth"`
	MaxChildrenPerAgent int              `json:"max_children_per_agent"`
	AllowedEdges        []DelegationEdge `json:"allowed_edges"`
}

// LegacyRoleAlias keeps existing local registrations readable while ensuring
// every new route and scaffold resolves to the canonical role graph.
type LegacyRoleAlias struct {
	CanonicalRole string `json:"canonical_role"`
	Status        string `json:"status"`
	Migration     string `json:"migration,omitempty"`
}

// LegacyIDMigration documents prefixes retained only to reject old
// registrations deterministically. They never become a second identity.
type LegacyIDMigration struct {
	CanonicalRole string `json:"canonical_role"`
	Status        string `json:"status"`
	Migration     string `json:"migration"`
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
	DisplayName      string `json:"display_name,omitempty"`
	DefaultEmoji     string `json:"default_emoji,omitempty"`
	OwnershipScope   string `json:"ownership_scope,omitempty"`
	Customizable     bool   `json:"customizable,omitempty"`
}

var safeAgentID = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

func ValidAgentID(id string) bool {
	return safeAgentID.MatchString(id)
}

var roleContracts = map[string]struct {
	direct        bool
	tools         string
	mayDelegate   bool
	inputContract string
}{
	"account_agent":        {false, "scoped", false, "bounded_account_packet"}, // compatibility only
	"case_agent":           {false, "scoped", false, "bounded_case_packet"},
	"client_account_agent": {false, "scoped", false, "bounded_client_account_packet"},
	"errand_helper":        {false, "scoped", false, "bounded_errand_packet"},
	"governance_analyst":   {false, "scoped", false, "bounded_health_packet"},
	"hub":                  {true, "none", true, "session_context_packet"},
	"pa_expert":            {false, "none", false, "bounded_advisory_packet"},
	"quality_guardian":     {false, "scoped", false, "bounded_quality_packet"},
	"reviewer":             {false, "none", false, "sealed_review_packet"},
	"workspace_agent":      {false, "scoped", false, "bounded_workspace_packet"}, // compatibility only
}

type RoleContract struct {
	DirectUserAccess bool
	ToolAccess       string
	MayDelegate      bool
	InputContract    string
}

func (catalog Catalog) ContractForRole(role string) (RoleContract, bool) {
	if canonical := catalog.CanonicalRole(role); canonical != role {
		if canonical == "client_account_agent" {
			role = canonical
		} else if canonical == "case_agent" {
			role = canonical
		}
	}
	contract, ok := roleContracts[role]
	if !ok {
		return RoleContract{}, false
	}
	return RoleContract{
		DirectUserAccess: contract.direct,
		ToolAccess:       contract.tools,
		MayDelegate:      contract.mayDelegate,
		InputContract:    contract.inputContract,
	}, true
}

// CanonicalRole resolves a compatibility name to the role used by the
// current hierarchy. Unknown roles are returned unchanged so callers can
// still produce a precise validation error.
func (catalog Catalog) CanonicalRole(role string) string {
	if alias, ok := catalog.LegacyAliases[role]; ok {
		if alias.Status == "compatibility_only" {
			return alias.CanonicalRole
		}
	}
	return role
}

// RejectLegacyRegistration is the single migration boundary for retired
// roles and IDs. Compatibility aliases remain readable; deprecated practice
// identities fail closed and require an explicit PA Expert kind.
func (catalog Catalog) RejectLegacyRegistration(agentID, role string) error {
	if alias, ok := catalog.LegacyAliases[role]; ok && strings.HasPrefix(alias.Status, "deprecated") {
		return fmt.Errorf("legacy role %q is deprecated; %s", role, alias.Migration)
	}
	for prefix, migration := range catalog.LegacyIDs {
		if strings.HasPrefix(agentID, prefix) && strings.HasPrefix(migration.Status, "deprecated") {
			return fmt.Errorf("legacy agent ID %q is deprecated; %s", agentID, migration.Migration)
		}
	}
	return nil
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
	if err := validateLegacyAliases(catalog.LegacyAliases); err != nil {
		return err
	}
	if err := validateLegacyIDs(catalog.LegacyIDs); err != nil {
		return err
	}
	if catalog.Delegation.Mode != "maestro_planner" || catalog.Delegation.RegisteredChains != "bounded_sequential" || catalog.Delegation.MaxActiveBranches != 1 || catalog.Delegation.MaxDepth != 1 || catalog.Delegation.MaxChildrenPerAgent != 0 || catalog.Delegation.MaxErrandHelpers != 1 || catalog.Delegation.ErrandScope != "basic_reversible" {
		return errors.New("agent catalog must enforce Maestro planning, one active spoke and depth one")
	}
	if err := validateDelegationEdges(catalog.Delegation.AllowedEdges); err != nil {
		return err
	}
	if catalog.NativeAdvisory.Mode != "host_runtime" || catalog.NativeAdvisory.MaxDepth != 2 || catalog.NativeAdvisory.MaxChildrenPerAgent != 2 {
		return errors.New("agent catalog must declare the bounded native advisory profile")
	}
	if err := validateNativeAdvisoryEdges(catalog.NativeAdvisory.AllowedEdges); err != nil {
		return err
	}
	if len(catalog.Agents) < 3 {
		return errors.New("agent catalog is missing the Maestro core")
	}

	seen := make(map[string]Agent, len(catalog.Agents))
	previous := ""
	directUsers := 0
	for _, agent := range catalog.Agents {
		if !ValidAgentID(agent.ID) || agent.ID <= previous {
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
		"maestro":              {"hub", true, "none", true, "session_context_packet"},
		"yoda":               {"reviewer", false, "none", false, "sealed_review_packet"},
		"darwin":               {"governance_analyst", false, "scoped", false, "bounded_health_packet"},
		"case-agent":           {"case_agent", false, "scoped", false, "bounded_case_packet"},
		"client-account-agent": {"client_account_agent", false, "scoped", false, "bounded_client_account_packet"},
		"pa-expert":            {"pa_expert", false, "none", false, "bounded_advisory_packet"},
		"gamma-guardian":       {"quality_guardian", false, "scoped", false, "bounded_quality_packet"},
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
	fromRole = catalog.CanonicalRole(fromRole)
	toRole = catalog.CanonicalRole(toRole)
	if fromRole != "hub" || depth != 1 {
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

func (catalog Catalog) AllowsNativeConsultation(fromRole, toRole string, depth int) bool {
	if depth < 1 || depth > catalog.NativeAdvisory.MaxDepth {
		return false
	}
	fromRole = catalog.CanonicalRole(fromRole)
	toRole = catalog.CanonicalRole(toRole)
	for _, edge := range catalog.NativeAdvisory.AllowedEdges {
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
	role = catalog.CanonicalRole(role)
	for _, edge := range catalog.Delegation.AllowedEdges {
		if edge.FromRole == role {
			return true
		}
	}
	return false
}

func validateDelegationEdges(edges []DelegationEdge) error {
	wanted := []DelegationEdge{{FromRole: "hub", ToRoles: []string{"case_agent", "client_account_agent", "errand_helper", "governance_analyst", "pa_expert", "quality_guardian", "reviewer"}}}
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

func validateNativeAdvisoryEdges(edges []DelegationEdge) error {
	wanted := []DelegationEdge{
		{FromRole: "hub", ToRoles: []string{"case_agent", "client_account_agent", "errand_helper", "governance_analyst", "pa_expert", "quality_guardian", "reviewer"}},
		{FromRole: "case_agent", ToRoles: []string{"client_account_agent", "pa_expert", "quality_guardian", "reviewer"}},
		{FromRole: "client_account_agent", ToRoles: []string{"case_agent", "pa_expert", "reviewer"}},
	}
	if len(edges) != len(wanted) {
		return errors.New("agent catalog native advisory graph is incomplete")
	}
	for index, edge := range edges {
		expected := wanted[index]
		if edge.FromRole != expected.FromRole || len(edge.ToRoles) != len(expected.ToRoles) {
			return errors.New("agent catalog native advisory edges must use the canonical sorted role graph")
		}
		for targetIndex, target := range edge.ToRoles {
			if target != expected.ToRoles[targetIndex] {
				return errors.New("agent catalog native advisory edges must use the canonical sorted role graph")
			}
		}
	}
	return nil
}

func validateLegacyAliases(aliases map[string]LegacyRoleAlias) error {
	if len(aliases) == 0 {
		// Older in-memory fixtures predate the explicit migration map. The
		// checked-in catalog always carries it; accepting empty fixtures keeps
		// compatibility tests focused on their stated contract.
		return nil
	}
	wanted := map[string]LegacyRoleAlias{
		"account_agent":      {CanonicalRole: "client_account_agent", Status: "compatibility_only"},
		"workspace_agent":    {CanonicalRole: "case_agent", Status: "compatibility_only"},
		"practice_agent":     {CanonicalRole: "pa_expert", Status: "deprecated_rejected", Migration: "re-register as pa_expert with expert_kind FPA or IPA"},
		"subject_specialist": {CanonicalRole: "pa_expert", Status: "deprecated_rejected", Migration: "re-register as pa_expert with expert_kind FPA or IPA"},
	}
	if len(aliases) != len(wanted) {
		return errors.New("agent catalog must declare the complete legacy-role migration map")
	}
	for legacy, expected := range wanted {
		alias, ok := aliases[legacy]
		if !ok || alias != expected {
			return errors.New("agent catalog legacy-role migration map is invalid")
		}
	}
	return nil
}

func validateLegacyIDs(ids map[string]LegacyIDMigration) error {
	if len(ids) == 0 {
		return nil
	}
	wanted := map[string]LegacyIDMigration{
		"practice-": {CanonicalRole: "pa_expert", Status: "deprecated_rejected", Migration: "re-register as pa-expert-{fpa|ipa}-{scope}"},
		"subject-":  {CanonicalRole: "pa_expert", Status: "deprecated_rejected", Migration: "re-register as pa-expert-{fpa|ipa}-{scope}"},
	}
	if len(ids) != len(wanted) {
		return errors.New("agent catalog must declare the complete legacy-ID migration map")
	}
	for prefix, expected := range wanted {
		if got, ok := ids[prefix]; !ok || got != expected {
			return errors.New("agent catalog legacy-ID migration map is invalid")
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
	for _, role := range []string{"case_agent", "client_account_agent", "pa_expert"} {
		templatePath := filepath.Join(root, "templates", role, "AGENT.md")
		body, err := os.ReadFile(templatePath)
		if err != nil {
			return fmt.Errorf("read managed %s scaffold template: %w", role, err)
		}
		if len(body) == 0 || strings.Contains(string(body), "{{") ||
			strings.Contains(string(body), "}}") {
			return fmt.Errorf("managed %s scaffold template must be non-empty and data-free", role)
		}
	}
	for _, role := range []string{"practice_agent", "subject_specialist"} {
		templatePath := filepath.Join(root, "templates", role, "AGENT.md")
		body, err := os.ReadFile(templatePath)
		if err != nil || !strings.Contains(strings.ToLower(string(body)), "deprecated") || !strings.Contains(string(body), "pa_expert") {
			return fmt.Errorf("legacy %s template must remain an explicit deprecation marker", role)
		}
	}
	return nil
}
