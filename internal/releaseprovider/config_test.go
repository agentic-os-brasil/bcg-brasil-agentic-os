package releaseprovider

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProviderConfigActivatesOnlyCompleteApprovedRegistration(t *testing.T) {
	config, err := ParseConfig(strings.NewReader(`{
	  "schema_version": 1,
	  "state": "approved",
	  "provider": "github",
	  "auth_base": "https://github.com",
	  "api_base": "https://api.github.com",
	  "client_id": "Iv1.maestro-pilot",
	  "owner": "agentic-os-brasil",
	  "repository": "maestro-releases",
	  "reason": ""
	}`))
	if err != nil {
		t.Fatalf("ParseConfig() error = %v", err)
	}
	store := &memorySecretBackend{values: map[string][]byte{}}
	service := config.AuthService(func() SecureStore { return newNativeSecureStore(store) })
	if service.Flow.ClientID != "Iv1.maestro-pilot" || service.Flow.BaseURL != "https://github.com" {
		t.Fatalf("unexpected configured auth service: %#v", service.Flow)
	}
	status, err := service.Status()
	if err != nil || status.State != "unauthenticated" {
		t.Fatalf("Status() = %#v, %v", status, err)
	}
}

func TestProviderConfigUnavailableNeverConstructsNativeStore(t *testing.T) {
	config, err := ParseConfig(strings.NewReader(`{
	  "schema_version": 1,
	  "state": "unavailable",
	  "provider": "github",
	  "auth_base": "https://github.com",
	  "api_base": "https://api.github.com",
	  "client_id": "",
	  "owner": "",
	  "repository": "",
	  "reason": "registration pending"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	called := false
	service := config.AuthService(func() SecureStore {
		called = true
		return newNativeSecureStore(&memorySecretBackend{values: map[string][]byte{}})
	})
	if called {
		t.Fatal("unavailable provider constructed a native credential store")
	}
	if _, err := service.Status(); err == nil {
		t.Fatal("unavailable provider exposed an auth status")
	}
}

func TestProviderConfigRejectsPartialDuplicateAndUnknownInput(t *testing.T) {
	tests := map[string]string{
		"partial": `{
		  "schema_version":1,"state":"unavailable","provider":"github",
		  "auth_base":"https://github.com","api_base":"https://api.github.com",
		  "client_id":"partial","owner":"","repository":"","reason":"pending"
		}`,
		"duplicate": `{
		  "schema_version":1,"state":"approved","state":"unavailable","provider":"github",
		  "auth_base":"https://github.com","api_base":"https://api.github.com",
		  "client_id":"","owner":"","repository":"","reason":"pending"
		}`,
		"unknown": `{
		  "schema_version":1,"state":"unavailable","provider":"github",
		  "auth_base":"https://github.com","api_base":"https://api.github.com",
		  "client_id":"","owner":"","repository":"","reason":"pending","token":"secret"
		}`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseConfig(strings.NewReader(body)); err == nil {
				t.Fatal("ParseConfig() accepted unsafe input")
			}
		})
	}
}

func TestProviderConfigSchemaRetainsExecutableContract(t *testing.T) {
	path := filepath.Join("..", "..", "schemas", "release-provider.schema.json")
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := ValidateProviderConfigSchema(file); err != nil {
		t.Fatalf("ValidateProviderConfigSchema() error = %v", err)
	}

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(map[string]any){
		"gutted state conditional": func(schema map[string]any) {
			schema["allOf"] = []any{map[string]any{}}
		},
		"gutted identifier pattern": func(schema map[string]any) {
			properties := schema["properties"].(map[string]any)
			clientID := properties["client_id"].(map[string]any)
			clientID["pattern"] = ".*"
		},
	} {
		t.Run(name, func(t *testing.T) {
			var schema map[string]any
			if err := json.Unmarshal(body, &schema); err != nil {
				t.Fatal(err)
			}
			mutate(schema)
			mutated, err := json.Marshal(schema)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateProviderConfigSchema(bytes.NewReader(mutated)); err == nil {
				t.Fatal("ValidateProviderConfigSchema() accepted a gutted managed contract")
			}
		})
	}
}
