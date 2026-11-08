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
	// Independent canonical file and literal URI: matching a production
	// catalog to itself cannot prove the new disclosed recipe was retained.
	canonical, err := os.ReadFile("../../bundles/base/skills/maestro-operator/resources/authored-brain.md")
	if err != nil {
		t.Fatal(err)
	}
	const authoredURI = "maestro://methods/maestro-operator/resources/authored-brain.md"
	found := 0
	for index, resource := range identity.Resources {
		if index > 0 && identity.Resources[index-1].URI >= resource.URI {
			t.Fatal("identity URI closure is duplicated or unsorted")
		}
		if resource.URI == authoredURI {
			found++
			if resource.Bytes != int64(len(canonical)) || resource.SHA256 != fmt.Sprintf("%x", sha256.Sum256(canonical)) {
				t.Fatal("authored recipe identity differs from canonical source URI/hash/size", resource)
			}
		}
	}
	if found != 1 {
		t.Fatal("identity must contain exactly one canonical authored recipe", found)
	}
	if len(identity.Resources) != 262 {
		t.Fatalf("immutable resource closure changed: got %d, want 262", len(identity.Resources))
	}
}
