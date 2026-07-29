package maintenance

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const maintenanceSchemaID = "urn:bcg-brasil-agentic-os:schema:maintenance-jobs:v1"

// ValidateSchemaFile compiles the published maintenance schema and verifies
// its stable root contract. Compilation is deliberately executable so a
// malformed $ref or unsupported schema keyword cannot ship silently.
func ValidateSchemaFile(path string) error {
	_, err := loadSchema(path)
	return err
}

// ValidateSchemaAndCatalog proves that the shipped catalog is accepted by the
// published schema and by the semantic validator. Both layers are required:
// JSON Schema protects the wire contract while Catalog.Validate protects
// bounded policy invariants that are intentionally not expressed in JSON
// Schema.
func ValidateSchemaAndCatalog(schemaPath, catalogPath string) error {
	schema, err := loadSchema(schemaPath)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(catalogPath)
	if err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	var document any
	if err := decoder.Decode(&document); err != nil {
		return fmt.Errorf("decode maintenance catalog for schema validation: %w", err)
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		return err
	}
	if err := schema.Validate(document); err != nil {
		return fmt.Errorf("maintenance catalog does not satisfy published schema: %w", err)
	}
	if _, err := Parse(bytes.NewReader(body)); err != nil {
		return fmt.Errorf("maintenance catalog semantic validation: %w", err)
	}
	return nil
}

func loadSchema(path string) (*jsonschema.Schema, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	var raw map[string]any
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode maintenance schema: %w", err)
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		return nil, err
	}
	if raw["$schema"] != "https://json-schema.org/draft/2020-12/schema" ||
		raw["$id"] != maintenanceSchemaID || raw["type"] != "object" ||
		raw["additionalProperties"] != false {
		return nil, errors.New("maintenance schema root contract is incomplete")
	}
	required, ok := raw["required"].([]any)
	if !ok || len(required) != 3 || required[0] != "schema_version" || required[1] != "catalog_state" || required[2] != "jobs" {
		return nil, errors.New("maintenance schema required fields are incomplete")
	}
	compiler := jsonschema.NewCompiler()
	const resource = "maintenance-jobs.schema.json"
	if err := compiler.AddResource(resource, raw); err != nil {
		return nil, fmt.Errorf("add maintenance schema: %w", err)
	}
	schema, err := compiler.Compile(resource)
	if err != nil {
		return nil, fmt.Errorf("compile maintenance schema: %w", err)
	}
	return schema, nil
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("maintenance JSON contains multiple values")
		}
		return fmt.Errorf("maintenance JSON trailing content: %w", err)
	}
	return nil
}
