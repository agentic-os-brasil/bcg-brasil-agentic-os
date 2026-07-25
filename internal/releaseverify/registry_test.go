package releaseverify

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseAuthorityRegistryExposesOnlyCurrentlyActiveKeys(t *testing.T) {
	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	activeKey := base64.StdEncoding.EncodeToString(bytesOf(1, ed25519.PublicKeySize))
	revokedKey := base64.StdEncoding.EncodeToString(bytesOf(2, ed25519.PublicKeySize))
	expiredKey := base64.StdEncoding.EncodeToString(bytesOf(3, ed25519.PublicKeySize))

	body := `{
  "schema_version": 1,
  "product": "maestro",
  "authorities": [
    {
      "issuer": "maestro-release",
      "key_id": "pilot-active",
      "algorithm": "ed25519",
      "public_key": "` + activeKey + `",
      "status": "active",
      "valid_from": "2026-01-01T00:00:00Z",
      "valid_until": "2027-01-01T00:00:00Z"
    },
    {
      "issuer": "maestro-release",
      "key_id": "pilot-revoked",
      "algorithm": "ed25519",
      "public_key": "` + revokedKey + `",
      "status": "revoked",
      "valid_from": "2026-01-01T00:00:00Z",
      "valid_until": "2027-01-01T00:00:00Z",
      "revoked_at": "2026-07-20T00:00:00Z"
    },
    {
      "issuer": "maestro-release",
      "key_id": "pilot-expired",
      "algorithm": "ed25519",
      "public_key": "` + expiredKey + `",
      "status": "active",
      "valid_from": "2025-01-01T00:00:00Z",
      "valid_until": "2026-01-01T00:00:00Z"
    }
  ]
}`

	registry, err := ParseAuthorityRegistry(strings.NewReader(body), func() time.Time { return now })
	if err != nil {
		t.Fatalf("ParseAuthorityRegistry() error = %v", err)
	}
	got, ok := registry.Lookup("maestro", "maestro-release", "pilot-active")
	if !ok || string(got) != string(bytesOf(1, ed25519.PublicKeySize)) {
		t.Fatalf("Lookup(active) = %x, %v", got, ok)
	}
	for _, keyID := range []string{"pilot-revoked", "pilot-expired"} {
		if _, ok := registry.Lookup("maestro", "maestro-release", keyID); ok {
			t.Fatalf("Lookup(%s) exposed an unavailable authority", keyID)
		}
	}
}

func TestAuthorityRegistryRechecksValidityAtEveryLookup(t *testing.T) {
	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	registry, err := ParseAuthorityRegistry(
		strings.NewReader(validAuthorityRegistryJSON()),
		func() time.Time { return now },
	)
	if err != nil {
		t.Fatalf("ParseAuthorityRegistry() error = %v", err)
	}
	if _, ok := registry.Lookup("maestro", "maestro-release", "pilot-active"); !ok {
		t.Fatal("Lookup() rejected an authority inside its validity window")
	}
	now = time.Date(2027, time.January, 1, 0, 0, 0, 0, time.UTC)
	if _, ok := registry.Lookup("maestro", "maestro-release", "pilot-active"); ok {
		t.Fatal("Lookup() accepted an authority after valid_until")
	}
}

func TestParseAuthorityRegistryRejectsUnsafeContracts(t *testing.T) {
	now := time.Date(2026, time.July, 25, 12, 0, 0, 0, time.UTC)
	valid := validAuthorityRegistryJSON()
	tests := map[string]string{
		"unknown field":     strings.Replace(valid, `"product": "maestro"`, `"product": "maestro", "extra": true`, 1),
		"duplicate field":   strings.Replace(valid, `"product": "maestro"`, `"product": "maestro", "product": "maestro"`, 1),
		"wrong product":     strings.Replace(valid, `"product": "maestro"`, `"product": "other"`, 1),
		"unknown algorithm": strings.Replace(valid, `"algorithm": "ed25519"`, `"algorithm": "rsa"`, 1),
		"short key": strings.Replace(
			valid,
			base64.StdEncoding.EncodeToString(bytesOf(1, ed25519.PublicKeySize)),
			base64.StdEncoding.EncodeToString([]byte("short")),
			1,
		),
		"invalid status":            strings.Replace(valid, `"status": "active"`, `"status": "pending"`, 1),
		"invalid window":            strings.Replace(valid, `"valid_until": "2027-01-01T00:00:00Z"`, `"valid_until": "2025-01-01T00:00:00Z"`, 1),
		"active with revocation":    strings.Replace(valid, `"valid_until": "2027-01-01T00:00:00Z"`, `"valid_until": "2027-01-01T00:00:00Z", "revoked_at": "2026-07-20T00:00:00Z"`, 1),
		"revoked without timestamp": strings.Replace(valid, `"status": "active"`, `"status": "revoked"`, 1),
		"duplicate authority": strings.Replace(
			valid,
			"\n  ]",
			",\n"+strings.TrimSuffix(strings.TrimPrefix(validAuthorityEntry(), "    "), "\n")+"\n  ]",
			1,
		),
	}

	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseAuthorityRegistry(strings.NewReader(body), func() time.Time { return now }); err == nil {
				t.Fatal("ParseAuthorityRegistry() accepted an unsafe registry")
			}
		})
	}
}

func TestPublishedAuthorityRegistrySchemaHasExpectedIdentity(t *testing.T) {
	path := filepath.Join("..", "..", "schemas", "release-authority-registry.schema.json")
	if err := ValidateAuthorityRegistrySchemaFile(path); err != nil {
		t.Fatalf("ValidateAuthorityRegistrySchemaFile() error = %v", err)
	}
}

func TestValidateAuthorityRegistrySchemaFileRejectsGuttedSchema(t *testing.T) {
	publishedPath := filepath.Join("..", "..", "schemas", "release-authority-registry.schema.json")
	published, err := os.ReadFile(publishedPath)
	if err != nil {
		t.Fatal(err)
	}
	withoutPublicKeyType := strings.Replace(
		string(published),
		`"public_key": {
          "type": "string",`,
		`"public_key": {`,
		1,
	)
	if withoutPublicKeyType == string(published) {
		t.Fatal("test fixture did not remove the public_key type")
	}
	for name, body := range map[string]string{
		"missing root contract":                `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"urn:bcg-brasil-agentic-os:schema:release-authority-registry:v1"}`,
		"public key accepts non-string values": withoutPublicKeyType,
		"empty authority definition": `{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "urn:bcg-brasil-agentic-os:schema:release-authority-registry:v1",
  "type": "object",
  "additionalProperties": false,
  "required": ["schema_version", "product", "authorities"],
  "properties": {
    "schema_version": {"const": 1},
    "product": {"const": "maestro"},
    "authorities": {"type": "array", "minItems": 1, "items": {"$ref": "#/$defs/authority"}}
  },
  "$defs": {"authority": {}}
}`,
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "schema.json")
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := ValidateAuthorityRegistrySchemaFile(path); err == nil {
				t.Fatal("ValidateAuthorityRegistrySchemaFile() accepted a schema without the contract")
			}
		})
	}
}

func validAuthorityRegistryJSON() string {
	return `{
  "schema_version": 1,
  "product": "maestro",
  "authorities": [
` + validAuthorityEntry() + `  ]
}`
}

func validAuthorityEntry() string {
	return `    {
      "issuer": "maestro-release",
      "key_id": "pilot-active",
      "algorithm": "ed25519",
      "public_key": "` + base64.StdEncoding.EncodeToString(bytesOf(1, ed25519.PublicKeySize)) + `",
      "status": "active",
      "valid_from": "2026-01-01T00:00:00Z",
      "valid_until": "2027-01-01T00:00:00Z"
    }
`
}

func bytesOf(value byte, count int) []byte {
	result := make([]byte, count)
	for index := range result {
		result[index] = value
	}
	return result
}
