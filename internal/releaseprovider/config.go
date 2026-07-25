package releaseprovider

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"unicode"
	"unicode/utf8"
)

const maximumProviderConfigBytes = 16 << 10

var providerIdentifierPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

type Config struct {
	SchemaVersion int    `json:"schema_version"`
	State         string `json:"state"`
	Provider      string `json:"provider"`
	AuthBase      string `json:"auth_base"`
	APIBase       string `json:"api_base"`
	ClientID      string `json:"client_id"`
	Owner         string `json:"owner"`
	Repository    string `json:"repository"`
	Reason        string `json:"reason"`
}

func ParseConfig(reader io.Reader) (Config, error) {
	body, err := io.ReadAll(io.LimitReader(reader, maximumProviderConfigBytes+1))
	if err != nil {
		return Config{}, fmt.Errorf("read release provider configuration: %w", err)
	}
	if len(body) > maximumProviderConfigBytes {
		return Config{}, fmt.Errorf("release provider configuration exceeds %d bytes", maximumProviderConfigBytes)
	}
	if err := rejectDuplicateProviderJSONKeys(body); err != nil {
		return Config{}, fmt.Errorf("decode release provider configuration: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var config Config
	if err := decoder.Decode(&config); err != nil {
		return Config{}, fmt.Errorf("decode release provider configuration: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Config{}, errors.New("decode release provider configuration: multiple JSON values")
		}
		return Config{}, fmt.Errorf("decode release provider configuration trailing content: %w", err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (config Config) Validate() error {
	if config.SchemaVersion != 1 {
		return fmt.Errorf("unsupported release provider schema version %d", config.SchemaVersion)
	}
	if config.Provider != "github" ||
		config.AuthBase != "https://github.com" ||
		config.APIBase != "https://api.github.com" {
		return errors.New("release provider must use the approved GitHub endpoints")
	}
	switch config.State {
	case "unavailable":
		if config.ClientID != "" || config.Owner != "" || config.Repository != "" {
			return errors.New("unavailable release provider cannot contain a partial registration")
		}
		if err := validateProviderReason(config.Reason); err != nil {
			return err
		}
	case "approved":
		if !providerIdentifierPattern.MatchString(config.ClientID) ||
			!providerIdentifierPattern.MatchString(config.Owner) ||
			!providerIdentifierPattern.MatchString(config.Repository) {
			return errors.New("approved release provider identifiers are invalid")
		}
		if len(config.Owner) > 100 || len(config.Repository) > 100 {
			return errors.New("approved release repository coordinates exceed 100 characters")
		}
		if config.Reason != "" {
			return errors.New("approved release provider cannot declare an unavailable reason")
		}
	default:
		return fmt.Errorf("unsupported release provider state %q", config.State)
	}
	return nil
}

func (config Config) Approved() bool {
	return config.Validate() == nil && config.State == "approved"
}

func (config Config) AuthService(storeFactory func() SecureStore) AuthService {
	if !config.Approved() || storeFactory == nil {
		return AuthService{Store: UnavailableStore{}}
	}
	store := storeFactory()
	if store == nil {
		return AuthService{Store: UnavailableStore{}}
	}
	return AuthService{
		Flow:  DeviceFlowClient{ClientID: config.ClientID, BaseURL: config.AuthBase},
		Store: store,
	}
}

func validateProviderReason(reason string) error {
	if reason == "" || len(reason) > 256 || !utf8.ValidString(reason) {
		return errors.New("unavailable release provider reason is invalid")
	}
	for _, character := range reason {
		if unicode.IsControl(character) {
			return errors.New("unavailable release provider reason contains a control character")
		}
	}
	return nil
}

func rejectDuplicateProviderJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := walkProviderJSONValue(decoder, token); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple JSON values")
		}
		return err
	}
	return nil
}

func walkProviderJSONValue(decoder *json.Decoder, token json.Token) error {
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = true
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := walkProviderJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim('}') {
			return errors.New("unterminated JSON object")
		}
	case '[':
		for decoder.More() {
			valueToken, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := walkProviderJSONValue(decoder, valueToken); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil {
			return err
		}
		if end != json.Delim(']') {
			return errors.New("unterminated JSON array")
		}
	default:
		return fmt.Errorf("unexpected JSON delimiter %q", delimiter)
	}
	return nil
}

func ValidateProviderConfigSchema(reader io.Reader) error {
	var schema map[string]any
	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&schema); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("release provider schema contains trailing content")
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" ||
		schema["$id"] != "urn:bcg-brasil-agentic-os:schema:release-provider:v1" ||
		schema["type"] != "object" ||
		schema["additionalProperties"] != false {
		return errors.New("release provider schema root contract is incomplete")
	}
	required, ok := schema["required"].([]any)
	if !ok || len(required) != 9 {
		return errors.New("release provider schema required fields are incomplete")
	}
	expected := []string{
		"schema_version", "state", "provider", "auth_base", "api_base",
		"client_id", "owner", "repository", "reason",
	}
	for index, field := range expected {
		if required[index] != field {
			return fmt.Errorf("release provider schema required field %d must be %q", index, field)
		}
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok || len(properties) != len(expected) {
		return errors.New("release provider schema properties are incomplete")
	}
	if !schemaConfigConst(properties, "schema_version", float64(1)) ||
		!schemaConfigConst(properties, "provider", "github") ||
		!schemaConfigConst(properties, "auth_base", "https://github.com") ||
		!schemaConfigConst(properties, "api_base", "https://api.github.com") {
		return errors.New("release provider schema fixed identities are incomplete")
	}
	state, ok := properties["state"].(map[string]any)
	if !ok || len(state) != 1 {
		return errors.New("release provider schema states are incomplete")
	}
	states, ok := state["enum"].([]any)
	if !ok || len(states) != 2 || states[0] != "unavailable" || states[1] != "approved" {
		return errors.New("release provider schema states are incomplete")
	}
	for name, maximum := range map[string]float64{
		"client_id":  128,
		"owner":      100,
		"repository": 100,
	} {
		property, ok := properties[name].(map[string]any)
		if !ok ||
			len(property) != 3 ||
			property["type"] != "string" ||
			property["maxLength"] != maximum ||
			property["pattern"] != "^[A-Za-z0-9._-]*$" {
			return fmt.Errorf("release provider schema %s constraints are incomplete", name)
		}
	}
	reason, ok := properties["reason"].(map[string]any)
	if !ok || len(reason) != 2 || reason["type"] != "string" || reason["maxLength"] != float64(256) {
		return errors.New("release provider schema reason constraints are incomplete")
	}
	constraints, ok := schema["allOf"].([]any)
	if !ok || len(constraints) != 1 {
		return errors.New("release provider schema state constraints are incomplete")
	}
	conditional, ok := constraints[0].(map[string]any)
	if !ok || len(conditional) != 3 {
		return errors.New("release provider schema state conditional is incomplete")
	}
	ifClause, ok := conditional["if"].(map[string]any)
	if !ok || !schemaApprovedStateCondition(ifClause) {
		return errors.New("release provider schema approved-state condition is incomplete")
	}
	thenClause, ok := conditional["then"].(map[string]any)
	if !ok || !schemaProviderFieldLengths(thenClause, map[string]float64{
		"client_id": 1, "owner": 1, "repository": 1,
	}, map[string]float64{"reason": 0}) {
		return errors.New("release provider schema approved-state constraints are incomplete")
	}
	elseClause, ok := conditional["else"].(map[string]any)
	if !ok || !schemaProviderFieldLengths(elseClause,
		map[string]float64{"reason": 1},
		map[string]float64{"client_id": 0, "owner": 0, "repository": 0},
	) {
		return errors.New("release provider schema unavailable-state constraints are incomplete")
	}
	return nil
}

func schemaConfigConst(properties map[string]any, name string, expected any) bool {
	property, ok := properties[name].(map[string]any)
	return ok && len(property) == 1 && property["const"] == expected
}

func schemaApprovedStateCondition(clause map[string]any) bool {
	properties, ok := clause["properties"].(map[string]any)
	if !ok || len(properties) != 1 || !schemaConfigConst(properties, "state", "approved") {
		return false
	}
	required, ok := clause["required"].([]any)
	return ok && len(required) == 1 && required[0] == "state"
}

func schemaProviderFieldLengths(
	clause map[string]any,
	minimums map[string]float64,
	maximums map[string]float64,
) bool {
	properties, ok := clause["properties"].(map[string]any)
	if !ok || len(properties) != len(minimums)+len(maximums) {
		return false
	}
	for name, expected := range minimums {
		property, ok := properties[name].(map[string]any)
		if !ok || len(property) != 1 || property["minLength"] != expected {
			return false
		}
	}
	for name, expected := range maximums {
		property, ok := properties[name].(map[string]any)
		if !ok || len(property) != 1 || property["maxLength"] != expected {
			return false
		}
	}
	return true
}
