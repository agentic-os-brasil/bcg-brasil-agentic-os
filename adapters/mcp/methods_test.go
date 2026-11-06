package product_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	baseskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/skills"
	techcoreskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/tech-core/skills"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestPackagedMethodsDiscovery(t *testing.T)      { runMethodDiscovery(t, false) }
func TestPackagedMethodsStdioDiscovery(t *testing.T) { runMethodDiscovery(t, true) }

func runMethodDiscovery(t *testing.T, stdio bool) {
	base := physicalTempDir(t)
	prod, home, ws := filepath.Join(base, "product"), filepath.Join(base, "owner"), filepath.Join(base, "workspace")
	for _, path := range []string{prod, home} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	lookup := func(string) (string, bool) { return "", false }
	h, err := datahome.InitializeFresh(datahome.Options{ProductRoot: prod, UserHome: home, LookupEnv: lookup})
	if err != nil {
		if runtime.GOOS == "windows" && !h.Writable && h.Diagnostic == "ownership-unverified" {
			t.Skip("fresh Windows ownership is unavailable; native method discovery is not qualified")
		}
		t.Fatal(err)
	}
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: ws, DataRoot: filepath.Join(h.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	const secret = "PRIVATE_OWNER_CONTENT_MUST_NOT_BE_A_RESOURCE"
	if err := os.WriteFile(filepath.Join(home, "private.md"), []byte(secret), 0600); err != nil {
		t.Fatal(err)
	}
	options := product.Options{ProductRoot: prod, UserHome: home, WorkspacePath: ws, LookupEnv: lookup}
	ctx := context.Background()
	var transport mcp.Transport
	if stdio {
		binary := filepath.Join(base, "maestro-mcp")
		if out, err := exec.Command("go", "build", "-o", binary, "./cmd/maestro-mcp").CombinedOutput(); err != nil {
			t.Fatalf("build: %s: %v", out, err)
		}
		packet := filepath.Join(base, "plugin")
		if err := packaging.BuildPlugin(packaging.Options{Destination: packet, Server: binary, Version: "0.2.0", Binding: options}); err != nil {
			t.Fatal(err)
		}
		transport = &mcp.CommandTransport{Command: exec.Command(filepath.Join(packet, "scripts", "maestro-mcp"), "serve", "--product-root", prod, "--user-home", home, "--workspace", ws), TerminateDuration: time.Second}
	} else {
		b, err := product.Open(options)
		if err != nil {
			t.Fatal(err)
		}
		a, z := mcp.NewInMemoryTransports()
		ss, err := b.Server().Connect(ctx, a, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer ss.Close()
		transport = z
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "method-test", Version: "1"}, nil).Connect(ctx, transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cs.Close()
	list, err := cs.ListPrompts(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	oracle, err := os.ReadFile("packaging/testdata/method-ids.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Fields(string(oracle))
	if len(want) != 92 {
		t.Fatalf("Spec056 oracle has %d methods", len(want))
	}
	var got []string
	for _, prompt := range list.Prompts {
		got = append(got, prompt.Name)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("canonical prompt inventory: got %d methods: %v", len(got), got)
	}
	resources, err := cs.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	listed := map[string]bool{}
	for _, r := range resources.Resources {
		if strings.Contains(r.URI, home) || strings.Contains(r.Description, home) {
			t.Fatalf("owner path in public resource: %+v", r)
		}
		listed[r.URI] = true
	}
	assertCanonicalAgentResources(t, cs, resources.Resources, secret)
	read := func(uri string) string {
		t.Helper()
		if !listed[uri] {
			t.Fatalf("resource not discoverable: %s", uri)
		}
		r, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.Contents) != 1 || r.Contents[0].URI != uri || strings.Contains(r.Contents[0].Text, secret) {
			t.Fatalf("invalid resource response: %+v", r)
		}
		return r.Contents[0].Text
	}
	var index struct {
		SchemaVersion int `json:"schema_version"`
		Methods       []struct {
			ID              string                               `json:"id"`
			UserInvokedOnly bool                                 `json:"user_invoked_only"`
			Resources       []struct{ Path, SHA256, URI string } `json:"resources"`
		} `json:"methods"`
	}
	if err := json.Unmarshal([]byte(read("maestro://methods/catalog.json")), &index); err != nil {
		t.Fatal(err)
	}
	if index.SchemaVersion != 1 || len(index.Methods) != 92 {
		t.Fatalf("invalid index: %+v", index)
	}
	expectedResources := 6 + 14 // fixed canonical agent resource oracle, separate from methods
	for _, method := range index.Methods {
		files, err := baseskills.Files(method.ID)
		if err != nil {
			files, err = techcoreskills.Files(method.ID)
		}
		if err != nil {
			t.Fatal(err)
		}
		if len(method.Resources) != len(files) {
			t.Fatalf("missing method resources: %s", method.ID)
		}
		expectedResources += len(files)
		for _, resource := range method.Resources {
			canonical, ok := files[resource.Path]
			if !ok || read(resource.URI) != string(canonical) || resource.SHA256 != fmt.Sprintf("%x", sha256.Sum256(canonical)) {
				t.Fatalf("resource bytes/hash differ: %s %s", method.ID, resource.Path)
			}
		}
		p, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: method.ID})
		if err != nil {
			t.Fatal(err)
		}
		encoded, _ := json.Marshal(p)
		if !strings.Contains(string(encoded), "methods_not_authority") || !strings.Contains(string(encoded), "maestro://methods/"+method.ID+"/SKILL.md") || strings.Contains(string(encoded), secret) {
			t.Fatalf("missing authority/resource in prompt %s", method.ID)
		}
		if method.ID == "pocock-implement" && !method.UserInvokedOnly {
			t.Fatal("manual-only method lost catalog metadata")
		}
	}
	if len(listed) != expectedResources {
		t.Fatalf("unexpected resources: got %d want %d public packaged files", len(listed), expectedResources)
	}
	for _, p := range list.Prompts {
		if p.Name == "pocock-implement" && p.Meta["user_invoked_only"] != true {
			t.Fatal("manual-only prompt lost metadata")
		}
	}
	if read("maestro://methods/pocock-upstream.json") != string(techcoreskills.Provenance()) {
		t.Fatal("canonical provenance changed")
	}
	if read("maestro://methods/agent-skill-policy.json") != string(baseskills.AgentSkillPolicy()) {
		t.Fatal("canonical policy changed")
	}
	declaration, err := os.ReadFile("method-contracts.json")
	if err != nil || read("maestro://methods/method-contracts.json") != string(declaration) {
		t.Fatal("canonical method declaration changed")
	}
	license, err := os.ReadFile("../../LICENSE.md")
	if err != nil || read("maestro://methods/LICENSE.md") != string(license) {
		t.Fatal("canonical license changed")
	}
	for _, uri := range []string{"file://" + filepath.Join(home, "private.md"), "maestro://methods/../private.md", "maestro://methods/pocock-tdd/../../private.md", "maestro://methods/%2e%2e/private.md", "maestro://methods/unknown/SKILL.md", "maestro://methods/pocock-tdd/SKILL.md?owner=true"} {
		if r, err := cs.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri}); err == nil {
			t.Fatalf("unsafe/unknown URI admitted %q: %+v", uri, r)
		}
	}
	if _, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "pocock-tdd", Arguments: map[string]string{"root": home}}); err == nil {
		t.Fatal("prompt accepted filesystem argument")
	}
	if _, err := cs.GetPrompt(ctx, &mcp.GetPromptParams{Name: "unknown-method"}); err == nil {
		t.Fatal("unknown method admitted")
	}
}

func TestMethodInventoryCopiesEmbeddedBytes(t *testing.T) {
	a, err := product.PackagedMethods()
	if err != nil {
		t.Fatal(err)
	}
	want := append([]byte(nil), a.Methods[0].Files["SKILL.md"]...)
	a.Methods[0].Files["SKILL.md"][0] = 'X'
	a.Assets["LICENSE.md"][0] = 'X'
	a.Methods[0].Resources[0].SHA256 = "modified"
	b, err := product.PackagedMethods()
	if err != nil {
		t.Fatal(err)
	}
	if string(b.Methods[0].Files["SKILL.md"]) != string(want) || b.Assets["LICENSE.md"][0] == 'X' || b.Methods[0].Resources[0].SHA256 == "modified" {
		t.Fatal("caller changed embedded method inventory")
	}
}
