package product

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	baseskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/skills"
	techskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/tech-core/skills"
)

//go:embed method-contracts.json
var methodContractsBody []byte

// MethodDependency describes a prerequisite; it grants no capability or approval.
type MethodDependency struct {
	ID                    string   `json:"id"`
	Kind                  string   `json:"kind"`
	Required              bool     `json:"required"`
	FulfillmentOperations []string `json:"fulfillment_operations,omitempty"`
}

// MethodConditionalEffect describes an optional follow-on effect and its gate.
type MethodConditionalEffect struct {
	Effect     string `json:"effect"`
	Condition  string `json:"condition"`
	Dependency string `json:"dependency"`
}

type MethodRequirements struct {
	RequiredOperations []string                  `json:"required_operations"`
	Effects            []string                  `json:"effects"`
	ConditionalEffects []MethodConditionalEffect `json:"conditional_effects,omitempty"`
	OutputKinds        []string                  `json:"output_kinds"`
	Dependencies       []MethodDependency        `json:"dependencies"`
}

type MethodContractGroup struct {
	MethodIDs []string `json:"method_ids"`
	MethodRequirements
}

// MethodContracts is the canonical dependency declaration for all product methods.
type MethodContracts struct {
	SchemaVersion int                   `json:"schema_version"`
	Groups        []MethodContractGroup `json:"groups"`
}

// ReadMethodContracts validates a bounded factory declaration against the actual
// canonical methods and registered operation schemas, without executing either.
func ReadMethodContracts(reader io.Reader) (MethodContracts, error) {
	var declaration MethodContracts
	body, err := io.ReadAll(io.LimitReader(reader, (128<<10)+1))
	if err != nil || len(body) > 128<<10 || !utf8.Valid(body) {
		return declaration, errors.New("method_contracts_invalid")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&declaration); err != nil {
		return declaration, errors.New("method_contracts_invalid")
	}
	if decoder.Decode(new(any)) != io.EOF {
		return declaration, errors.New("method_contracts_invalid")
	}
	return declaration, declaration.Validate()
}

func methodOperationNames() map[string]bool {
	operations := map[string]bool{}
	for _, schema := range []map[string]any{productQuerySchema(), productApplySchema()} {
		for _, entry := range schema["oneOf"].([]any) {
			fields := entry.(map[string]any)["properties"].(map[string]any)
			operation := fields["operation"].(map[string]any)["const"].(string)
			operations[operation] = true
		}
	}
	return operations
}

// Validate checks declaration integrity, not host execution or user authorization.
func (declaration MethodContracts) Validate() error {
	invalid := errors.New("method_contracts_invalid")
	want := map[string]bool{}
	base, err := baseskills.Catalog()
	if err != nil {
		return invalid
	}
	tech, err := techskills.Catalog()
	if err != nil {
		return invalid
	}
	for _, skill := range append(base.Skills, tech.Skills...) {
		want[skill.ID] = true
	}
	if declaration.SchemaVersion != 1 || len(declaration.Groups) == 0 || len(declaration.Groups) > len(want) {
		return invalid
	}
	operations := methodOperationNames()
	effects := map[string]bool{"none": true, "read": true, "write_owner": true, "write_workspace": true, "maintenance": true, "installation": true, "host_execution": true, "external_write": true, "write_host": true}
	outputs := map[string]bool{"reviewable_text": true, "execution_checkpoint": true, "owner_revision": true, "owner_page": true, "configuration": true, "case_artifact": true, "knowledge_page": true, "workspace_artifact": true, "memory_generation": true, "navigation_page": true, "diagnostic_report": true, "external_record": true, "host_artifact": true}
	kinds := map[string]bool{"domain": true, "host": true, "connector": true, "provider": true, "runtime": true}
	identifier := regexp.MustCompile(`^[a-z][a-z0-9_]*(\.[a-z][a-z0-9_]*)+$`)
	seen := map[string]bool{}
	for _, group := range declaration.Groups {
		if len(group.MethodIDs) == 0 || len(group.Effects) == 0 || len(group.OutputKinds) == 0 || len(group.Dependencies) > 16 || len(group.ConditionalEffects) > 16 {
			return invalid
		}
		for _, id := range group.MethodIDs {
			if !want[id] || seen[id] {
				return invalid
			}
			seen[id] = true
		}
		for _, set := range []struct {
			values  []string
			allowed map[string]bool
		}{{group.RequiredOperations, operations}, {group.Effects, effects}, {group.OutputKinds, outputs}} {
			found := map[string]bool{}
			for _, value := range set.values {
				if !set.allowed[value] || found[value] {
					return invalid
				}
				found[value] = true
			}
		}
		dependencies := map[string]bool{}
		for _, dependency := range group.Dependencies {
			if !kinds[dependency.Kind] || len(dependency.ID) > 96 || !identifier.MatchString(dependency.ID) || !strings.HasPrefix(dependency.ID, dependency.Kind+".") || dependencies[dependency.ID] {
				return invalid
			}
			if len(dependency.FulfillmentOperations) > 32 || len(dependency.FulfillmentOperations) > 0 && dependency.Kind != "domain" {
				return invalid
			}
			planned := map[string]bool{}
			for _, operation := range dependency.FulfillmentOperations {
				if len(operation) > 96 || !identifier.MatchString(operation) || planned[operation] {
					return invalid
				}
				planned[operation] = true
			}
			dependencies[dependency.ID] = true
		}
		conditional := map[string]bool{}
		for _, effect := range group.ConditionalEffects {
			if !effects[effect.Effect] || effect.Effect == "none" || strings.TrimSpace(effect.Condition) == "" || len(effect.Condition) > 512 || !dependencies[effect.Dependency] || conditional[effect.Effect] {
				return invalid
			}
			for _, dependency := range group.Dependencies {
				if dependency.ID == effect.Dependency && dependency.Required {
					return invalid
				}
			}
			for _, unconditional := range group.Effects {
				if unconditional == effect.Effect {
					return invalid
				}
			}
			conditional[effect.Effect] = true
		}
	}
	if len(seen) != len(want) {
		return invalid
	}
	return nil
}

func (declaration MethodContracts) requirements() map[string]MethodRequirements {
	result := map[string]MethodRequirements{}
	for _, group := range declaration.Groups {
		for _, id := range group.MethodIDs {
			result[id] = group.MethodRequirements
		}
	}
	return result
}
