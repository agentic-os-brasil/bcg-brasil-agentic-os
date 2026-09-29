package acceptance

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestIdentitySchemaSupportsScaffoldAndResumableInterview(t *testing.T) {
	raw, err := os.ReadFile("../schemas/identity.schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	if err := compiler.AddResource("identity.json", document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile("identity.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name, input string
		valid       bool
	}{
		{"legacy scaffold", `{"schema_version":1,"display_name":"Usuário","role":"","context":"","initialized":false}`, true},
		{"name captured before role", `{"name":"Alex","captured_at":"2026-09-29T00:00:00Z"}`, true},
		{"complete existing profile", `{"name":"Alex","role":"Consultant","captured_at":"2026-09-29T00:00:00Z"}`, true},
		{"initialized requires role", `{"initialized":true,"name":"Alex","captured_at":"2026-09-29T00:00:00Z"}`, false},
		{"empty object", `{}`, false},
		{"empty captured name", `{"name":"","captured_at":"2026-09-29T00:00:00Z"}`, false},
		{"unknown fields", `{"name":"Alex","captured_at":"2026-09-29T00:00:00Z","secret":"never"}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var v any
			if err := json.Unmarshal([]byte(tc.input), &v); err != nil {
				t.Fatal(err)
			}
			err := schema.Validate(v)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v, want %v: %v", err == nil, tc.valid, err)
			}
		})
	}
}
