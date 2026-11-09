package releasecontract

import (
	"fmt"
	"strings"
	"testing"
)

func TestSourceAwareServerIdentityAcceptsExplicitExecutionContract(t *testing.T) {
	resources := []EmbeddedResource{{URI: "maestro://test", Bytes: 1, SHA256: strings.Repeat("a", 64)}}
	digest, err := EmbeddedDigest(resources)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte(fmt.Sprintf(`{"schema_version":2,"product":"maestro","component":"mcp_server","protocol":"maestro-mcp-identity-v1","version":"0.1.0","compatible_bundle":">=0.1.0 <0.2.0","authority_registry_sha256":"%s","resources":[{"uri":"maestro://test","bytes":1,"sha256":"%s"}],"embedded_inputs_sha256":"%s","execution_contract":"maestro-synced-source-v1"}`, strings.Repeat("b", 64), strings.Repeat("a", 64), digest))
	var identity ServerIdentity
	if err := DecodeStrictJSON(body, &identity); err != nil {
		t.Fatalf("explicit source-aware identity should decode: %v", err)
	}
	if err := identity.Validate(); err != nil {
		t.Fatalf("explicit source-aware identity should validate: %v", err)
	}
	for _, change := range []struct {
		version  int
		contract string
	}{{2, ""}, {2, "unrecognized"}, {1, "maestro-synced-source-v1"}} {
		identity.SchemaVersion = change.version
		identity.ExecutionContract = change.contract
		if err := identity.Validate(); err == nil {
			t.Fatalf("accepted incompatible identity schema %d contract %q", change.version, change.contract)
		}
	}
}
