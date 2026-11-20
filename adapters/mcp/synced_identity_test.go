package product_test

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
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
	if len(identity.Resources) != 270 { // 254 method resources plus 16 canonical agent resources.
		t.Fatalf("immutable resource closure changed: got %d, want 270", len(identity.Resources))
	}
	for _, path := range []string{
		"start-day/resources/owner-atlas-mcp.md",
		"maestro-operator/resources/foreground-agents.md",
	} {
		canonical, err := os.ReadFile("../../bundles/base/skills/" + path)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, resource := range identity.Resources {
			if resource.URI == "maestro://methods/"+path {
				found = true
				if resource.Bytes != int64(len(canonical)) || resource.SHA256 != fmt.Sprintf("%x", sha256.Sum256(canonical)) {
					t.Fatalf("method resource identity differs from canonical bytes: %s", path)
				}
			}
		}
		if !found {
			t.Fatalf("method resource is absent from immutable identity: %s", path)
		}
	}
}
