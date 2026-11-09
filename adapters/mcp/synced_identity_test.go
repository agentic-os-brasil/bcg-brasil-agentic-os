package product_test

import (
	"encoding/json"
	"strings"
	"testing"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
)

// The public immutable identity must distinguish a source-aware executable from
// the real Task1 binary, even when its semantic version and resources match.
func TestIdentityAdvertisesSignedSourceExecutionContract(t *testing.T) {
	identity, err := product.Identity("1.2.3", ">=1.2.0 <1.3.0", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		SchemaVersion     int    `json:"schema_version"`
		ExecutionContract string `json:"execution_contract"`
	}
	if err := json.Unmarshal(body, &wire); err != nil {
		t.Fatal(err)
	}
	if wire.SchemaVersion != 2 || wire.ExecutionContract != "maestro-synced-source-v1" {
		t.Fatalf("actual identity cannot qualify signed-source execution: schema=%d contract=%q", wire.SchemaVersion, wire.ExecutionContract)
	}
	if len(identity.Resources) != 262 {
		t.Fatalf("immutable resource closure changed: got %d, want 262", len(identity.Resources))
	}
}
