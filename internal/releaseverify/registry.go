package releaseverify

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"
)

const maximumAuthorityRegistryBytes = 1 << 20

var authorityIdentifierPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,127}$`)

type AuthorityRegistry struct {
	product string
	clock   func() time.Time
	keys    map[string]authorityKey
}

type authorityKey struct {
	publicKey  ed25519.PublicKey
	status     string
	validFrom  time.Time
	validUntil time.Time
}

type authorityRegistryDocument struct {
	SchemaVersion int              `json:"schema_version"`
	Product       string           `json:"product"`
	Authorities   []authorityEntry `json:"authorities"`
}

type authorityEntry struct {
	Issuer     string  `json:"issuer"`
	KeyID      string  `json:"key_id"`
	Algorithm  string  `json:"algorithm"`
	PublicKey  string  `json:"public_key"`
	Status     string  `json:"status"`
	ValidFrom  string  `json:"valid_from"`
	ValidUntil string  `json:"valid_until"`
	RevokedAt  *string `json:"revoked_at,omitempty"`
}

func ParseAuthorityRegistry(reader io.Reader, clock func() time.Time) (AuthorityRegistry, error) {
	if clock == nil {
		return AuthorityRegistry{}, errors.New("release authority registry clock is required")
	}
	body, err := io.ReadAll(io.LimitReader(reader, maximumAuthorityRegistryBytes+1))
	if err != nil {
		return AuthorityRegistry{}, fmt.Errorf("read release authority registry: %w", err)
	}
	if len(body) > maximumAuthorityRegistryBytes {
		return AuthorityRegistry{}, fmt.Errorf("release authority registry exceeds %d bytes", maximumAuthorityRegistryBytes)
	}
	if err := rejectDuplicateRegistryJSONKeys(body); err != nil {
		return AuthorityRegistry{}, fmt.Errorf("decode release authority registry: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var document authorityRegistryDocument
	if err := decoder.Decode(&document); err != nil {
		return AuthorityRegistry{}, fmt.Errorf("decode release authority registry: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return AuthorityRegistry{}, errors.New("decode release authority registry: multiple JSON values")
		}
		return AuthorityRegistry{}, fmt.Errorf("decode release authority registry trailing content: %w", err)
	}
	return document.registryWithClock(clock)
}

func LoadAuthorityRegistry(path string, clock func() time.Time) (AuthorityRegistry, error) {
	file, err := os.Open(path)
	if err != nil {
		return AuthorityRegistry{}, err
	}
	defer file.Close()
	return ParseAuthorityRegistry(file, clock)
}

func (registry AuthorityRegistry) Lookup(product, issuer, keyID string) (ed25519.PublicKey, bool) {
	if product != registry.product || registry.clock == nil {
		return nil, false
	}
	key, ok := registry.keys[issuer+"/"+keyID]
	if !ok || key.status != "active" {
		return nil, false
	}
	now := registry.clock()
	if now.Before(key.validFrom) || !now.Before(key.validUntil) {
		return nil, false
	}
	return append(ed25519.PublicKey(nil), key.publicKey...), true
}

func (document authorityRegistryDocument) registryWithClock(clock func() time.Time) (AuthorityRegistry, error) {
	if document.SchemaVersion != 1 {
		return AuthorityRegistry{}, fmt.Errorf("unsupported release authority registry schema version %d", document.SchemaVersion)
	}
	if document.Product != "maestro" {
		return AuthorityRegistry{}, errors.New("release authority registry product must be maestro")
	}
	if len(document.Authorities) == 0 {
		return AuthorityRegistry{}, errors.New("release authority registry must contain at least one authority")
	}
	keys := map[string]authorityKey{}
	seen := map[string]bool{}
	for index, authority := range document.Authorities {
		if !authorityIdentifierPattern.MatchString(authority.Issuer) || !authorityIdentifierPattern.MatchString(authority.KeyID) {
			return AuthorityRegistry{}, fmt.Errorf("authority %d issuer and key_id must be bounded identifiers", index)
		}
		identity := authority.Issuer + "/" + authority.KeyID
		if seen[identity] {
			return AuthorityRegistry{}, fmt.Errorf("duplicate release authority %s", identity)
		}
		seen[identity] = true
		if authority.Algorithm != "ed25519" {
			return AuthorityRegistry{}, fmt.Errorf("authority %s uses unsupported algorithm %q", identity, authority.Algorithm)
		}
		publicKey, err := decodeAuthorityPublicKey(authority.PublicKey)
		if err != nil {
			return AuthorityRegistry{}, fmt.Errorf("authority %s: %w", identity, err)
		}
		validFrom, err := parseAuthorityTime("valid_from", authority.ValidFrom)
		if err != nil {
			return AuthorityRegistry{}, fmt.Errorf("authority %s: %w", identity, err)
		}
		validUntil, err := parseAuthorityTime("valid_until", authority.ValidUntil)
		if err != nil {
			return AuthorityRegistry{}, fmt.Errorf("authority %s: %w", identity, err)
		}
		if !validFrom.Before(validUntil) {
			return AuthorityRegistry{}, fmt.Errorf("authority %s validity window is empty or reversed", identity)
		}
		switch authority.Status {
		case "active":
			if authority.RevokedAt != nil {
				return AuthorityRegistry{}, fmt.Errorf("active authority %s cannot declare revoked_at", identity)
			}
		case "revoked":
			if authority.RevokedAt == nil {
				return AuthorityRegistry{}, fmt.Errorf("revoked authority %s must declare revoked_at", identity)
			}
			revokedAt, err := parseAuthorityTime("revoked_at", *authority.RevokedAt)
			if err != nil {
				return AuthorityRegistry{}, fmt.Errorf("authority %s: %w", identity, err)
			}
			if revokedAt.Before(validFrom) || !revokedAt.Before(validUntil) {
				return AuthorityRegistry{}, fmt.Errorf("authority %s revoked_at must be inside its validity window", identity)
			}
		default:
			return AuthorityRegistry{}, fmt.Errorf("authority %s has unsupported status %q", identity, authority.Status)
		}
		keys[identity] = authorityKey{
			publicKey:  append(ed25519.PublicKey(nil), publicKey...),
			status:     authority.Status,
			validFrom:  validFrom,
			validUntil: validUntil,
		}
	}
	return AuthorityRegistry{product: document.Product, clock: clock, keys: keys}, nil
}

func decodeAuthorityPublicKey(encoded string) (ed25519.PublicKey, error) {
	decoded, err := base64.StdEncoding.Strict().DecodeString(encoded)
	if err != nil || base64.StdEncoding.EncodeToString(decoded) != encoded {
		return nil, errors.New("public_key must use canonical standard base64")
	}
	if len(decoded) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("public_key must decode to %d bytes", ed25519.PublicKeySize)
	}
	return ed25519.PublicKey(decoded), nil
}

func parseAuthorityTime(field, value string) (time.Time, error) {
	if value == "" || value[len(value)-1] != 'Z' {
		return time.Time{}, fmt.Errorf("%s must be an RFC3339 UTC timestamp", field)
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s must be an RFC3339 UTC timestamp", field)
	}
	return parsed, nil
}

func rejectDuplicateRegistryJSONKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := walkRegistryJSONValue(decoder, token); err != nil {
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

func walkRegistryJSONValue(decoder *json.Decoder, token json.Token) error {
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
			if err := walkRegistryJSONValue(decoder, valueToken); err != nil {
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
			if err := walkRegistryJSONValue(decoder, valueToken); err != nil {
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

func ValidateAuthorityRegistrySchemaFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	var schema map[string]any
	if err := decoder.Decode(&schema); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("release authority registry schema contains multiple JSON values")
		}
		return err
	}
	if schema["$schema"] != "https://json-schema.org/draft/2020-12/schema" {
		return errors.New("release authority registry schema must use JSON Schema draft 2020-12")
	}
	if schema["$id"] != "urn:bcg-brasil-agentic-os:schema:release-authority-registry:v1" {
		return errors.New("release authority registry schema has an unexpected identifier")
	}
	if schema["type"] != "object" || schema["additionalProperties"] != false {
		return errors.New("release authority registry schema root must be a closed object")
	}
	required, ok := schema["required"].([]any)
	if !ok || len(required) != 3 {
		return errors.New("release authority registry schema required fields are incomplete")
	}
	for index, expected := range []string{"schema_version", "product", "authorities"} {
		if required[index] != expected {
			return fmt.Errorf("release authority registry schema required field %d must be %q", index, expected)
		}
	}
	properties, ok := schema["properties"].(map[string]any)
	if !ok {
		return errors.New("release authority registry schema properties are missing")
	}
	if !schemaPropertyEquals(properties, "schema_version", "const", float64(1)) {
		return errors.New("release authority registry schema_version must be fixed at 1")
	}
	if !schemaPropertyEquals(properties, "product", "const", "maestro") {
		return errors.New("release authority registry product must be fixed at maestro")
	}
	authorities, ok := properties["authorities"].(map[string]any)
	if !ok || authorities["type"] != "array" || authorities["minItems"] != float64(1) {
		return errors.New("release authority registry authorities must be a non-empty array")
	}
	items, ok := authorities["items"].(map[string]any)
	if !ok || items["$ref"] != "#/$defs/authority" {
		return errors.New("release authority registry authorities must use the authority definition")
	}
	definitions, ok := schema["$defs"].(map[string]any)
	if !ok {
		return errors.New("release authority registry definitions are missing")
	}
	identifier, ok := definitions["identifier"].(map[string]any)
	if !ok || identifier["type"] != "string" || identifier["pattern"] != authorityIdentifierPattern.String() {
		return errors.New("release authority registry identifier definition is incomplete")
	}
	timestamp, ok := definitions["utcTimestamp"].(map[string]any)
	if !ok || timestamp["type"] != "string" || timestamp["format"] != "date-time" || timestamp["pattern"] != "Z$" {
		return errors.New("release authority registry UTC timestamp definition is incomplete")
	}
	authority, ok := definitions["authority"].(map[string]any)
	if !ok || authority["type"] != "object" || authority["additionalProperties"] != false {
		return errors.New("release authority registry authority definition must be a closed object")
	}
	authorityRequired, ok := authority["required"].([]any)
	expectedAuthorityFields := []string{"issuer", "key_id", "algorithm", "public_key", "status", "valid_from", "valid_until"}
	if !ok || len(authorityRequired) != len(expectedAuthorityFields) {
		return errors.New("release authority registry authority required fields are incomplete")
	}
	for index, expected := range expectedAuthorityFields {
		if authorityRequired[index] != expected {
			return fmt.Errorf("release authority registry authority required field %d must be %q", index, expected)
		}
	}
	authorityProperties, ok := authority["properties"].(map[string]any)
	if !ok {
		return errors.New("release authority registry authority properties are missing")
	}
	for _, name := range []string{"issuer", "key_id"} {
		if !schemaPropertyEquals(authorityProperties, name, "$ref", "#/$defs/identifier") {
			return fmt.Errorf("release authority registry authority %s constraint is incomplete", name)
		}
	}
	if !schemaPropertyEquals(authorityProperties, "algorithm", "const", "ed25519") {
		return errors.New("release authority registry algorithm must be fixed at ed25519")
	}
	publicKeyProperty, ok := authorityProperties["public_key"].(map[string]any)
	if !ok || publicKeyProperty["type"] != "string" || publicKeyProperty["pattern"] != "^[A-Za-z0-9+/]{43}=$" {
		return errors.New("release authority registry public_key constraint is incomplete")
	}
	if !schemaEnumEquals(authorityProperties, "status", []string{"active", "revoked"}) {
		return errors.New("release authority registry status constraint is incomplete")
	}
	for _, name := range []string{"valid_from", "valid_until", "revoked_at"} {
		if !schemaPropertyEquals(authorityProperties, name, "$ref", "#/$defs/utcTimestamp") {
			return fmt.Errorf("release authority registry authority %s constraint is incomplete", name)
		}
	}
	if !schemaRevocationConditional(authority["allOf"]) {
		return errors.New("release authority registry revocation conditional is incomplete")
	}
	return nil
}

func schemaPropertyEquals(properties map[string]any, name, key string, expected any) bool {
	property, ok := properties[name].(map[string]any)
	return ok && property[key] == expected
}

func schemaEnumEquals(properties map[string]any, name string, expected []string) bool {
	property, ok := properties[name].(map[string]any)
	if !ok {
		return false
	}
	values, ok := property["enum"].([]any)
	if !ok || len(values) != len(expected) {
		return false
	}
	for index, value := range values {
		if value != expected[index] {
			return false
		}
	}
	return true
}

func schemaRevocationConditional(value any) bool {
	allOf, ok := value.([]any)
	if !ok || len(allOf) != 1 {
		return false
	}
	conditional, ok := allOf[0].(map[string]any)
	if !ok {
		return false
	}
	ifNode, ok := conditional["if"].(map[string]any)
	if !ok || !schemaRequiredEquals(ifNode["required"], []string{"status"}) {
		return false
	}
	ifProperties, ok := ifNode["properties"].(map[string]any)
	if !ok || !schemaPropertyEquals(ifProperties, "status", "const", "revoked") {
		return false
	}
	thenNode, ok := conditional["then"].(map[string]any)
	if !ok || !schemaRequiredEquals(thenNode["required"], []string{"revoked_at"}) {
		return false
	}
	elseNode, ok := conditional["else"].(map[string]any)
	if !ok {
		return false
	}
	notNode, ok := elseNode["not"].(map[string]any)
	return ok && schemaRequiredEquals(notNode["required"], []string{"revoked_at"})
}

func schemaRequiredEquals(value any, expected []string) bool {
	required, ok := value.([]any)
	if !ok || len(required) != len(expected) {
		return false
	}
	for index, name := range expected {
		if required[index] != name {
			return false
		}
	}
	return true
}
