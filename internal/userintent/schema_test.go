package userintent

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestUserIntentSchemaCompilesAndRejectsRawObservationContent(t *testing.T) {
	path := filepath.Join("..", "..", "schemas", "user-intent.schema.json")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document any
	if err := json.Unmarshal(body, &document); err != nil {
		t.Fatal(err)
	}
	compiler := jsonschema.NewCompiler()
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		t.Fatal(err)
	}
	resource := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolutePath)}).String()
	if err := compiler.AddResource(resource, document); err != nil {
		t.Fatal(err)
	}
	schema, err := compiler.Compile(resource)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := testSnapshot()
	packet, err := NewIntentReviewPacket("schema-packet", "prompt", "case_direct", "plan", "draft", snapshot, nil, nil, AudienceUser, ConsequenceLow, Reversible)
	if err != nil {
		t.Fatal(err)
	}
	packetBody, err := json.Marshal(packet)
	if err != nil {
		t.Fatal(err)
	}
	var valid any
	if err := json.Unmarshal(packetBody, &valid); err != nil {
		t.Fatal(err)
	}
	if err := schema.Validate(valid); err != nil {
		t.Fatalf("valid packet rejected: %v", err)
	}
	invalid := map[string]any{"schema_version": 1, "observation_id": "obs", "source_event_sha256": strings.Repeat("a", 64), "kind": "explicit_instruction", "facet": "preferences", "signal_key": "preferences", "claim_digest": strings.Repeat("b", 64), "episode_sha256": strings.Repeat("c", 64), "evidence_type": "owner_speech", "owner_authenticated": true, "material": true, "declassified": true, "scope": "global", "confidence_basis_points": 8000, "sensitivity": "normal", "lifecycle": "eligible", "recorded_at": "2026-08-01T12:00:00Z", "expires_at": "2026-09-01T12:00:00Z", "recheck_at": "2026-08-02T12:00:00Z", "user_confirmed": true, "transcript": "secret"}
	if err := schema.Validate(invalid); err == nil {
		t.Fatal("schema accepted raw observation content")
	}
}
