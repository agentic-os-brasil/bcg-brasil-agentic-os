package product_test

import (
	"crypto/sha256"
	"fmt"
	"os"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAuthoredBrainResourceHasIndependentCanonicalIdentity(t *testing.T) {
	f := newOwnerFixture(t, false)
	client := f.connect(t, nil)
	resources, err := client.ListResources(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	// Independently inspected canonical files: 244 method files plus six shared
	// policy/catalogue/license/provenance/declaration resources, not old draft identity.
	if len(resources.Resources) != 250 {
		t.Fatalf("immutable current resource closure: got %d want 250", len(resources.Resources))
	}
	const uri = "maestro://methods/maestro-operator/resources/authored-brain.md"
	canonical, err := os.ReadFile("../../bundles/base/skills/maestro-operator/resources/authored-brain.md")
	if err != nil {
		t.Fatal(err)
	}
	seen, found := map[string]bool{}, 0
	for _, resource := range resources.Resources {
		if seen[resource.URI] {
			t.Fatalf("duplicated immutable resource URI: %s", resource.URI)
		}
		seen[resource.URI] = true
		if resource.URI == uri {
			found++
			if resource.Size != int64(len(canonical)) || resource.Meta["sha256"] != fmt.Sprintf("%x", sha256.Sum256(canonical)) {
				t.Fatal("authored recipe metadata differs from canonical bytes", resource)
			}
		}
	}
	if found != 1 {
		t.Fatalf("canonical authored recipe appears %d times", found)
	}
	read, err := client.ReadResource(t.Context(), &mcp.ReadResourceParams{URI: uri})
	if err != nil || len(read.Contents) != 1 || read.Contents[0].URI != uri || read.Contents[0].Text != string(canonical) {
		t.Fatal("public authored recipe differs from canonical bytes", err)
	}
}
