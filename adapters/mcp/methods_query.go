package product

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func methodsQueries() []any {
	return []any{
		variant("methods.plan", map[string]any{"method_id": map[string]any{"type": "string", "maxLength": 96, "pattern": `^[a-z][a-z0-9-]*$`}}, "method_id"),
		variant("methods.list", map[string]any{"offset": map[string]any{"type": "integer", "minimum": 0, "maximum": 128}, "limit": map[string]any{"type": "integer", "minimum": 1, "maximum": 32}}),
		variant("methods.read", map[string]any{
			"resource_uri": map[string]any{"type": "string", "maxLength": 512, "pattern": `^maestro://methods/[A-Za-z0-9][A-Za-z0-9._/-]*$`},
			"offset":       map[string]any{"type": "integer", "minimum": 0, "maximum": 128 << 10},
			"max_bytes":    map[string]any{"type": "integer", "minimum": 4, "maximum": 4096},
		}, "resource_uri"),
	}
}

func (b *Bound) handleMethods(in input) *mcp.CallToolResult {
	methods, err := PackagedMethods()
	if err != nil {
		return methodQueryFailure("methods_unavailable")
	}
	switch in.Operation {
	case "methods.list":
		limit := in.Limit
		if limit == 0 {
			limit = 16
		}
		if in.Offset < 0 || in.Offset > len(methods.Methods) || limit < 1 || limit > 32 {
			return methodQueryFailure("method_page_invalid")
		}
		end := min(in.Offset+limit, len(methods.Methods))
		return result(map[string]any{"items": methods.Methods[in.Offset:end], "total": len(methods.Methods), "next_offset": end, "has_more": end < len(methods.Methods), "shared_resources": sharedMethodResources(methods), "mode": "methods_not_authority"}, nil)
	case "methods.plan":
		for _, method := range methods.Methods {
			if method.ID != in.MethodID {
				continue
			}
			requirements := methods.Requirements[method.ID]
			operations := make([]map[string]string, 0, len(requirements.RequiredOperations))
			for _, operation := range requirements.RequiredOperations {
				operations = append(operations, map[string]string{"name": operation, "state": "registered"})
			}
			dependencies := make([]map[string]any, 0, len(requirements.Dependencies))
			state := "requirements_available"
			for _, dependency := range requirements.Dependencies {
				if dependency.Required {
					state = "requirements_unavailable"
				}
				dependencies = append(dependencies, map[string]any{"id": dependency.ID, "kind": dependency.Kind, "required": dependency.Required, "state": "unavailable", "reason": "dependency_not_attested_by_method_selection", "next_action": methodDependencyAction(dependency)})
			}
			return result(map[string]any{"method_id": method.ID, "instruction_uri": "maestro://methods/" + method.ID + "/SKILL.md", "user_invoked_only": method.UserInvokedOnly, "selection": "explicit_method_id", "state": state, "execution_state": "not_executed", "operations": operations, "effects": requirements.Effects, "output_kinds": requirements.OutputKinds, "dependencies": dependencies, "mode": "methods_not_authority", "next_action": "Read the method instructions. Confirm actual inputs and named dependencies before an authorized effect; the plan neither executes nor grants approval."}, nil)
		}
		return methodQueryFailure("method_unavailable")
	case "methods.read":
		// Exact embedded identities never become paths or URI-resolution inputs.
		var body []byte
		for name, content := range methods.Assets {
			uri := "maestro://methods/" + name
			if name == "methods-index.json" {
				uri = "maestro://methods/catalog.json"
			}
			if uri == in.ResourceURI {
				body = content
			}
		}
		for _, method := range methods.Methods {
			for _, resource := range method.Resources {
				if resource.URI == in.ResourceURI {
					body = method.Files[resource.Path]
				}
			}
		}
		if body == nil {
			return methodQueryFailure("method_resource_unavailable")
		}
		limit := in.MaxBytes
		if limit == 0 {
			limit = 4096
		}
		if in.Offset < 0 || in.Offset > len(body) || limit < 4 || limit > 4096 || in.Offset < len(body) && !utf8.RuneStart(body[in.Offset]) {
			return methodQueryFailure("method_page_invalid")
		}
		end := min(in.Offset+limit, len(body))
		for end > in.Offset && !utf8.Valid(body[in.Offset:end]) {
			end--
		}
		return result(map[string]any{"resource_uri": in.ResourceURI, "text": string(body[in.Offset:end]), "sha256": fmt.Sprintf("%x", sha256.Sum256(body)), "total_bytes": len(body), "offset": in.Offset, "next_offset": end, "has_more": end < len(body), "mode": "methods_not_authority"}, nil)
	}
	return result(nil, errors.New("invalid_method_operation"))
}

func sharedMethodResources(methods MethodPackage) []MethodResource {
	resources := make([]MethodResource, 0, len(methods.Assets))
	for name, body := range methods.Assets {
		uri := "maestro://methods/" + name
		if name == "methods-index.json" {
			uri = "maestro://methods/catalog.json"
		}
		resources = append(resources, MethodResource{Path: name, URI: uri, SHA256: fmt.Sprintf("%x", sha256.Sum256(body)), Bytes: len(body)})
	}
	sort.Slice(resources, func(i, j int) bool { return resources[i].URI < resources[j].URI })
	return resources
}

func methodDependencyAction(dependency MethodDependency) string {
	switch dependency.Kind {
	case "domain":
		return "Use or implement the owning Maestro operation for " + dependency.ID + "; keep this effect pending."
	case "connector":
		return "Verify an authorized, scope-bound " + dependency.ID + " connector before its use."
	case "provider":
		return "Admit the explicit " + dependency.ID + " provider configuration, credentials and execution authority before a model call."
	case "runtime":
		return "Inspect content.status and admit the required " + dependency.ID + " runtime; selecting a method installs nothing."
	default:
		return "Verify the authorized " + dependency.ID + " capability in the selected client before using it."
	}
}

func methodQueryFailure(code string) *mcp.CallToolResult {
	out := result(map[string]any{"code": code, "next_action": "List installed methods and use an exact returned method ID or immutable resource URI and a valid page."}, nil)
	out.IsError = true
	return out
}
