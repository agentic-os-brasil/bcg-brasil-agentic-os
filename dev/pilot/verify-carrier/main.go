// Development-only validation; never distributed with the native CLI.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/ingest/docling/acquisition"
	"os"
	"path/filepath"
)

func main() {
	if len(os.Args) != 4 {
		panic("carrier directory, public registry and absent cache path required")
	}
	root, registry, cache := os.Args[1], os.Args[2], os.Args[3]
	body, e := os.ReadFile(registry)
	if e != nil {
		panic(e)
	}
	sum := sha256.Sum256(body)
	var recipe acquisition.Recipe
	b, e := os.ReadFile(filepath.Join(root, "runtime-recipe", acquisition.RecipeName))
	if e != nil {
		panic(e)
	}
	if e = json.Unmarshal(b, &recipe); e != nil {
		panic(e)
	}
	release := filepath.Join(root, "runtime-release")
	var manifest struct{ Artifacts []struct{ Kind, Name string } }
	b, e = os.ReadFile(filepath.Join(release, "release-manifest.json"))
	if e != nil {
		panic(e)
	}
	if e = json.Unmarshal(b, &manifest); e != nil {
		panic(e)
	}
	server := ""
	for _, a := range manifest.Artifacts {
		if a.Kind == "mcp_server" {
			server = filepath.Join(release, a.Name)
		}
	}
	_, e = acquisition.Review(context.Background(), acquisition.Options{ReleaseDirectory: release, RecipeDirectory: filepath.Join(root, "runtime-recipe"), ServerPath: server, RegistryPath: registry, RegistrySHA256: hex.EncodeToString(sum[:]), CacheRoot: cache})
	if e != nil {
		panic(e)
	}
	fmt.Println("authenticated acquisition carrier; no download or runtime execution")
}
