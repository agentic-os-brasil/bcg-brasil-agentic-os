package product

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// Inspect every authoritative variant without exporting a private test hook.
// Even a host truncating the tool description cannot hide parameter contracts.
func TestStartupNeverBootstraps(t *testing.T) {
	if _, err := Open(Options{}); err == nil {
		t.Fatal("unbound startup admitted")
	}
	for _, authoritative := range []map[string]any{productQuerySchema(), productApplySchema()} {
		before, err := json.Marshal(authoritative)
		if err != nil {
			t.Fatal(err)
		}
		presentation, description := toolPresentation(authoritative)
		after, _ := json.Marshal(authoritative)
		if string(before) != string(after) {
			t.Fatal("presentation changed authoritative schema")
		}
		if len([]byte(description)) > 3072 {
			t.Fatal("tool description exceeds supported host bound")
		}
		properties := presentation["properties"].(map[string]any)
		selectors := properties["operation"].(map[string]any)["enum"].([]string)
		variants := authoritative["oneOf"].([]any)
		if len(selectors) != len(variants) || presentation["oneOf"] != nil {
			t.Fatal("selector count or union projection changed")
		}
		allowed := map[string]bool{"operation": true}
		seen := map[string]bool{}
		for _, entry := range variants {
			v := entry.(map[string]any)
			fields := v["properties"].(map[string]any)
			op := fields["operation"].(map[string]any)["const"].(string)
			if seen[op] || !containsSelector(selectors, op) {
				t.Fatal("duplicate or missing selector", op)
			}
			seen[op] = true
			for name := range fields {
				allowed[name] = true
			}
			for name, advertised := range properties {
				if name == "operation" {
					continue
				}
				common := advertised.(map[string]any)
				documentation, _ := common["description"].(string)
				if len([]byte(documentation)) > 4096 {
					t.Fatal("field metadata exceeds supported host bound", name)
				}
				lines := strings.Split(documentation, "\n")
				if len(lines) < 4 || !strings.HasPrefix(lines[0], "Required for: ") || !strings.HasPrefix(lines[1], "Allowed for: ") {
					t.Fatal("field metadata lost to tool-description truncation", op, name)
				}
				required := strings.Split(strings.TrimPrefix(lines[0], "Required for: "), ", ")
				permitted := strings.Split(strings.TrimPrefix(lines[1], "Allowed for: "), ", ")
				exact, present := fields[name]
				if containsSelector(permitted, op) != present || containsSelector(required, op) != containsSelector(v["required"].([]string), name) {
					t.Fatal("required/allowed metadata changed", op, name)
				}
				var contract string
				for _, line := range lines[3:] {
					if strings.HasPrefix(line, op+": ") {
						if contract != "" {
							t.Fatal("duplicate parameter contract", op, name)
						}
						contract = strings.TrimPrefix(line, op+": ")
					}
				}
				if !present {
					if contract != "" {
						t.Fatal("invented parameter contract", op, name)
					}
					continue
				}
				allowed[name] = true
				var documented map[string]any
				if err := json.Unmarshal([]byte(contract), &documented); err != nil {
					t.Fatal("missing operation-specific constraints", op, name, err)
				}
				want, _ := json.Marshal(exact)
				got, _ := json.Marshal(documented)
				if string(want) != string(got) {
					t.Fatal("operation-specific constraints changed", op, name)
				}
				for key, value := range common {
					if key == "description" {
						continue
					}
					expected, present := exact.(map[string]any)[key]
					want, _ := json.Marshal(expected)
					got, _ := json.Marshal(value)
					if !present || string(want) != string(got) {
						t.Fatal("shared metadata narrows a different operation", op, name, key)
					}
				}
			}
		}
		for name := range properties {
			if !allowed[name] {
				t.Fatal("invented presentation field", name)
			}
			delete(allowed, name)
		}
		if len(allowed) != 0 || !reflect.DeepEqual(presentation["required"], []string{"operation"}) || presentation["additionalProperties"] != false {
			t.Fatal("presentation lost finite envelope", allowed)
		}
	}
}

func containsSelector(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
