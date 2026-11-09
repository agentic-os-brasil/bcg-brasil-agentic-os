package product

import (
	"crypto/sha256"
	"fmt"
	"sort"

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

// Identity enumerates only immutable compiled public inputs. Admission, owner
// state and workspace resolution are deliberately absent from this operation.
func Identity(version, compatibleBundle, registrySHA256 string) (releasecontract.ServerIdentity, error) {
	methods, err := PackagedMethods()
	if err != nil {
		return releasecontract.ServerIdentity{}, err
	}
	agents, err := baseagents.Files()
	if err != nil {
		return releasecontract.ServerIdentity{}, err
	}
	identity := releasecontract.ServerIdentity{SchemaVersion: 2, Product: "maestro", Component: "mcp_server", Protocol: releasecontract.ServerProtocol, ExecutionContract: releasecontract.SyncedSourceContract, Version: version, CompatibleBundle: compatibleBundle, AuthorityRegistrySHA256: registrySHA256}
	add := func(uri string, body []byte) {
		identity.Resources = append(identity.Resources, releasecontract.EmbeddedResource{URI: uri, Bytes: int64(len(body)), SHA256: fmt.Sprintf("%x", sha256.Sum256(body))})
	}
	for name, body := range methods.Assets {
		if name == "methods-index.json" {
			name = "catalog.json"
		}
		add("maestro://methods/"+name, body)
	}
	for _, method := range methods.Methods {
		for _, resource := range method.Resources {
			add(resource.URI, method.Files[resource.Path])
		}
	}
	for name, body := range agents {
		add("maestro://agents/"+name, body)
	}
	sort.Slice(identity.Resources, func(i, j int) bool { return identity.Resources[i].URI < identity.Resources[j].URI })
	identity.EmbeddedInputsSHA256, err = releasecontract.EmbeddedDigest(identity.Resources)
	if err != nil {
		return identity, err
	}
	return identity, identity.Validate()
}
