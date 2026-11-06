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
	if len(identity.Resources) != 263 {
		t.Fatalf("immutable resource closure changed: got %d, want 263", len(identity.Resources))
	}
	for index, resource := range identity.Resources {
		if index > 0 && identity.Resources[index-1].URI >= resource.URI {
			t.Fatal("identity URI closure is duplicated or unsorted")
		}
	}
	// Independent canonical files retain both composition inputs; the catalog
	// under test cannot serve as its own expected resource oracle.
	for uri, source := range map[string]string{
		"maestro://methods/method-contracts.json":                        "method-contracts.json",
		"maestro://methods/maestro-operator/resources/authored-brain.md": "../../bundles/base/skills/maestro-operator/resources/authored-brain.md",
	} {
		canonical, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		found := 0
		for _, resource := range identity.Resources {
			if resource.URI == uri {
				found++
				if resource.SHA256 != fmt.Sprintf("%x", sha256.Sum256(canonical)) || resource.Bytes != int64(len(canonical)) {
					t.Fatal("canonical resource identity hash/size mismatch", uri)
				}
			}
		}
		if found != 1 {
			t.Fatal("canonical resource URI must occur exactly once", uri, found)
		}
	}
}
