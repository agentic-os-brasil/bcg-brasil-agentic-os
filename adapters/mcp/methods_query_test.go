package product_test

import (
	"bytes"
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
	"unicode/utf8"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	baseskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/skills"
	techskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/tech-core/skills"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func methodTreeSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, path)
		value := info.Mode().String()
		if info.Mode().IsRegular() {
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			value += fmt.Sprintf(":%x", sha256.Sum256(body))
		}
		out[rel] = value
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

type methodQueryFixture struct {
	root    string
	options product.Options
}

func newMethodQueryFixture(t *testing.T) methodQueryFixture {
	t.Helper()
	root := physicalTempDir(t)
	prod, owner, work := filepath.Join(root, "product"), filepath.Join(root, "owner"), filepath.Join(root, "workspace")
	for _, path := range []string{prod, owner} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	lookup := func(string) (string, bool) { return "", false }
	home, err := datahome.InitializeFresh(datahome.Options{ProductRoot: prod, UserHome: owner, LookupEnv: lookup})
	if err != nil {
		if runtime.GOOS == "windows" && !home.Writable && home.Diagnostic == "ownership-unverified" {
			t.Skip("fresh Windows ownership is unavailable; method queries are not qualified")
		}
		t.Fatal(err)
	}
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: work, DataRoot: filepath.Join(home.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	return methodQueryFixture{root, product.Options{ProductRoot: prod, UserHome: owner, WorkspacePath: work, LookupEnv: lookup}}
}

func (f methodQueryFixture) connect(t *testing.T, binary string) *mcp.ClientSession {
	t.Helper()
	var transport mcp.Transport
	if binary == "" {
		bound, err := product.Open(f.options)
		if err != nil {
			t.Fatal(err)
		}
		a, b := mcp.NewInMemoryTransports()
		server, err := bound.Server().Connect(context.Background(), a, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { server.Close() })
		transport = b
	} else {
		transport = &mcp.CommandTransport{Command: exec.Command(binary, "serve", "--product-root", f.options.ProductRoot, "--user-home", f.options.UserHome, "--workspace", f.options.WorkspacePath), TerminateDuration: time.Second}
	}
	client, err := mcp.NewClient(&mcp.Implementation{Name: "method-query-oracle", Version: "1"}, nil).Connect(context.Background(), transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func methodQueryCall(t *testing.T, client *mcp.ClientSession, tool string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	out, err := client.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func methodQueryDecode(t *testing.T, client *mcp.ClientSession, tool string, args map[string]any, value any) {
	t.Helper()
	out := methodQueryCall(t, client, tool, args)
	if out.IsError {
		t.Fatalf("%v refused: %+v", args, out.Content)
	}
	if len(out.Content) != 1 {
		t.Fatalf("unexpected content count: %d", len(out.Content))
	}
	text, ok := out.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("expected JSON text")
	}
	if err := json.Unmarshal([]byte(text.Text), value); err != nil {
		t.Fatal(err)
	}
}

func assertMethodQueries(t *testing.T, client *mcp.ClientSession) {
	t.Helper()
	oracle, err := os.ReadFile("packaging/testdata/method-ids.txt")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Fields(string(oracle))
	if len(want) != 92 {
		t.Fatal("independent92-method oracle changed")
	}
	var all []product.PackagedMethod
	for offset := 0; ; {
		var page struct {
			Items []product.PackagedMethod `json:"items"`
			Next  int                      `json:"next_offset"`
			More  bool                     `json:"has_more"`
		}
		methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.list", "offset": offset, "limit": 11}, &page)
		if len(page.Items) > 11 || page.Next != offset+len(page.Items) || page.More && page.Next <= offset {
			t.Fatal("method list pagination did not advance within its bound")
		}
		all = append(all, page.Items...)
		if !page.More {
			break
		}
		offset = page.Next
	}
	var got []string
	for _, method := range all {
		got = append(got, method.ID)
	}
	if !sort.StringsAreSorted(got) || !reflect.DeepEqual(got, want) {
		t.Fatalf("methods.list differs from independent catalog: %v", got)
	}
	for _, method := range all {
		files, err := baseskills.Files(method.ID)
		if err != nil {
			files, err = techskills.Files(method.ID)
		}
		if err != nil || len(files) != len(method.Resources) {
			t.Fatalf("incomplete method resources: %s", method.ID)
		}
		if method.ID == "pocock-implement" && !method.UserInvokedOnly {
			t.Fatal("explicit-only method lost its invocation policy")
		}
		for _, resource := range method.Resources {
			expected, exists := files[resource.Path]
			if !exists {
				t.Fatalf("unknown catalogued resource %s", resource.Path)
			}
			var body strings.Builder
			for offset := 0; ; {
				var page struct {
					URI    string `json:"resource_uri"`
					Text   string `json:"text"`
					Hash   string `json:"sha256"`
					Bytes  int    `json:"total_bytes"`
					Offset int    `json:"offset"`
					Next   int    `json:"next_offset"`
					More   bool   `json:"has_more"`
				}
				methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.read", "resource_uri": resource.URI, "offset": offset, "max_bytes": 4096}, &page)
				if page.URI != resource.URI || page.Hash != fmt.Sprintf("%x", sha256.Sum256(expected)) || page.Bytes != len(expected) || page.Offset != offset || len(page.Text) > 4096 || !utf8.ValidString(page.Text) || page.Next != offset+len(page.Text) || page.More && page.Next <= offset {
					t.Fatalf("invalid immutable page: %s offset%d", resource.URI, offset)
				}
				body.WriteString(page.Text)
				if !page.More {
					break
				}
				offset = page.Next
			}
			if body.String() != string(expected) {
				t.Fatalf("resource bytes differ from canonical bundle: %s", resource.URI)
			}
		}
	}
	// Shared policy, dependency declarations and upstream provenance are governed
	// embedded bytes too; reconstruct every advertised shared resource.
	pkg, err := product.PackagedMethods()
	if err != nil {
		t.Fatal(err)
	}
	var listing struct {
		Shared []product.MethodResource `json:"shared_resources"`
	}
	methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.list", "limit": 1}, &listing)
	if len(listing.Shared) != len(pkg.Assets) {
		t.Fatal("shared resource inventory incomplete")
	}
	for _, resource := range listing.Shared {
		expected, ok := pkg.Assets[resource.Path]
		if !ok {
			t.Fatalf("unknown shared resource %s", resource.Path)
		}
		var body strings.Builder
		for offset := 0; ; {
			var page struct {
				Text  string `json:"text"`
				Hash  string `json:"sha256"`
				Bytes int    `json:"total_bytes"`
				Next  int    `json:"next_offset"`
				More  bool   `json:"has_more"`
			}
			methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.read", "resource_uri": resource.URI, "offset": offset}, &page)
			if page.Hash != resource.SHA256 || page.Bytes != len(expected) || len(page.Text) > 4096 || !utf8.ValidString(page.Text) || page.Next != offset+len(page.Text) || page.More && page.Next <= offset {
				t.Fatal("invalid shared resource page")
			}
			body.WriteString(page.Text)
			if !page.More {
				break
			}
			offset = page.Next
		}
		if body.String() != string(expected) {
			t.Fatalf("shared resource drift: %s", resource.URI)
		}
	}
	for _, method := range all {
		var plan struct {
			ID        string `json:"method_id"`
			State     string `json:"state"`
			Execution string `json:"execution_state"`
			Explicit  bool   `json:"user_invoked_only"`
		}
		methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": method.ID}, &plan)
		if plan.ID != method.ID || plan.Explicit != method.UserInvokedOnly || plan.Execution != "not_executed" || plan.State != "requirements_available" && plan.State != "requirements_unavailable" {
			t.Fatalf("invalid plan for %s: %+v", method.ID, plan)
		}
	}
	// Exercise a multibyte boundary explicitly, independently of page length.
	for _, method := range all {
		files, err := baseskills.Files(method.ID)
		if err != nil {
			continue
		}
		body := files["SKILL.md"]
		for offset, b := range body {
			if b >= 0xc2 {
				if !methodQueryCall(t, client, "maestro_query", map[string]any{"operation": "methods.read", "resource_uri": "maestro://methods/" + method.ID + "/SKILL.md", "offset": offset + 1, "max_bytes": 4}).IsError {
					t.Fatal("middle of UTF8 codepoint admitted")
				}
				var page struct {
					Text string `json:"text"`
				}
				methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.read", "resource_uri": "maestro://methods/" + method.ID + "/SKILL.md", "offset": offset, "max_bytes": 4}, &page)
				if !utf8.ValidString(page.Text) || page.Text == "" || len(page.Text) > 4 {
					t.Fatal("invalid bounded UTF8 page")
				}
				goto utf8Checked
			}
		}
	}
	t.Fatal("canonical methods lack a UTF8 boundary fixture")
utf8Checked:
	var policy struct {
		Text string `json:"text"`
	}
	methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.read", "resource_uri": "maestro://methods/METHODS.md"}, &policy)
	if !strings.Contains(policy.Text, "methods_not_authority") {
		t.Fatal("shared method authority is not available through ordinary tools")
	}
	for _, uri := range []string{"file:///owner/private.md", "maestro://owner/private.md", "maestro://methods/../owner/private.md", "maestro://methods/pocock-tdd/%2e%2e/private.md", "maestro://methods/maestro-product:pocock-tdd/SKILL.md", "maestro://methods/pocock-tdd/missing.md", "maestro://methods/pocock-tdd/SKILL.md?private=true"} {
		if out := methodQueryCall(t, client, "maestro_query", map[string]any{"operation": "methods.read", "resource_uri": uri}); !out.IsError {
			t.Fatalf("unknown/private/traversal resource admitted: %s", uri)
		}
	}
	for _, args := range []map[string]any{{"operation": "methods.list", "offset": 93}, {"operation": "methods.list", "limit": 33}, {"operation": "methods.read", "resource_uri": "maestro://methods/METHODS.md", "max_bytes": 4097}, {"operation": "methods.read", "resource_uri": "maestro://methods/METHODS.md", "offset": 99999}} {
		if !methodQueryCall(t, client, "maestro_query", args).IsError {
			t.Fatalf("invalid page admitted: %v", args)
		}
	}
}

func TestMethodQueryDiscoverySDK(t *testing.T) {
	f := newMethodQueryFixture(t)
	client := f.connect(t, "")
	before := methodTreeSnapshot(t, f.root)
	assertMethodQueries(t, client)
	if !reflect.DeepEqual(before, methodTreeSnapshot(t, f.root)) {
		t.Fatal("method queries changed owner/workspace/product state")
	}
}

func TestMethodQueryPlanSDK(t *testing.T) {
	f := newMethodQueryFixture(t)
	client := f.connect(t, "")
	before := methodTreeSnapshot(t, f.root)
	var continuation struct {
		ID         string `json:"method_id"`
		State      string `json:"state"`
		Execution  string `json:"execution_state"`
		Operations []struct {
			Name  string `json:"name"`
			State string `json:"state"`
		} `json:"operations"`
		Outputs []string `json:"output_kinds"`
	}
	methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "execution-continuity"}, &continuation)
	if continuation.ID != "execution-continuity" || continuation.State != "requirements_available" || continuation.Execution != "not_executed" {
		t.Fatalf("untrue continuation plan: %+v", continuation)
	}
	want := map[string]bool{"execution.create": false, "execution.start": false, "execution.checkpoint": false, "execution.pause": false, "execution.resume": false, "execution.next": false}
	for _, operation := range continuation.Operations {
		if operation.State != "registered" {
			t.Fatalf("method required an unregistered operation: %+v", operation)
		}
		if _, ok := want[operation.Name]; ok {
			want[operation.Name] = true
		}
	}
	for name, present := range want {
		if !present {
			t.Fatalf("continuation omitted %s", name)
		}
	}
	if len(continuation.Outputs) == 0 {
		t.Fatal("method has no expected output")
	}
	var engineering struct {
		ExplicitOnly bool   `json:"user_invoked_only"`
		Selection    string `json:"selection"`
		State        string `json:"state"`
		Execution    string `json:"execution_state"`
		Dependencies []struct {
			ID     string `json:"id"`
			State  string `json:"state"`
			Action string `json:"next_action"`
		} `json:"dependencies"`
	}
	methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "pocock-implement"}, &engineering)
	if !engineering.ExplicitOnly || engineering.Selection != "explicit_method_id" || engineering.State != "requirements_unavailable" || engineering.Execution != "not_executed" || len(engineering.Dependencies) == 0 {
		t.Fatalf("engineering method claimed unobserved host execution: %+v", engineering)
	}
	for _, dependency := range engineering.Dependencies {
		if dependency.ID == "" || dependency.State != "unavailable" || dependency.Action == "" {
			t.Fatalf("missing actionable dependency: %+v", dependency)
		}
	}
	for _, id := range []string{"missing-method", "maestro-product:pocock-implement", "../owner", "/pocock-implement", ""} {
		if !methodQueryCall(t, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": id}).IsError {
			t.Fatalf("unknown or spoofed method selected: %q", id)
		}
	}
	if !methodQueryCall(t, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "pocock-implement", "approved": true}).IsError {
		t.Fatal("caller supplied selection authority")
	}
	if !reflect.DeepEqual(before, methodTreeSnapshot(t, f.root)) {
		t.Fatal("method plan executed or changed state")
	}
}

func TestMethodContractsRejectInvalidDeclarations(t *testing.T) {
	packet, err := product.PackagedMethods()
	if err != nil {
		t.Fatal(err)
	}
	canonical := packet.Assets["method-contracts.json"]
	if _, err := product.ReadMethodContracts(bytes.NewReader(canonical)); err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*product.MethodContracts){
		"missing method": func(d *product.MethodContracts) { d.Groups[0].MethodIDs = d.Groups[0].MethodIDs[1:] },
		"duplicate method": func(d *product.MethodContracts) {
			d.Groups[0].MethodIDs = append(d.Groups[0].MethodIDs, d.Groups[0].MethodIDs[0])
		},
		"missing conditional capability": func(d *product.MethodContracts) {
			d.Groups[0].ConditionalEffects = []product.MethodConditionalEffect{{Effect: "write_workspace", Condition: "After user selection", Dependency: "host.missing"}}
		},
		"empty conditional gate": func(d *product.MethodContracts) {
			d.Groups[0].Dependencies = []product.MethodDependency{{ID: "host.repository_write", Kind: "host", Required: false}}
			d.Groups[0].ConditionalEffects = []product.MethodConditionalEffect{{Effect: "write_workspace", Dependency: "host.repository_write"}}
		},
		"unknown method":        func(d *product.MethodContracts) { d.Groups[0].MethodIDs[0] = "invented-method" },
		"nonexistent operation": func(d *product.MethodContracts) { d.Groups[0].RequiredOperations = []string{"execution.invented"} },
		"unknown effect":        func(d *product.MethodContracts) { d.Groups[0].Effects = []string{"execute_anything"} },
		"unknown output":        func(d *product.MethodContracts) { d.Groups[0].OutputKinds = []string{"anything"} },
		"invalid dependency kind": func(d *product.MethodContracts) {
			d.Groups[0].Dependencies = []product.MethodDependency{{ID: "host.commands", Kind: "anything", Required: true}}
		},
		"mismatched dependency prefix": func(d *product.MethodContracts) {
			d.Groups[0].Dependencies = []product.MethodDependency{{ID: "host.commands", Kind: "connector", Required: true}}
		},
		"host cannot be fulfilled by operation names": func(d *product.MethodContracts) {
			d.Groups[0].Dependencies = []product.MethodDependency{{ID: "host.trusted_review", Kind: "host", Required: true, FulfillmentOperations: []string{"owner.status"}}}
		},
		"duplicate fulfillment operation": func(d *product.MethodContracts) {
			d.Groups[0].Dependencies = []product.MethodDependency{{ID: "domain.future_content", Kind: "domain", Required: true, FulfillmentOperations: []string{"content.status", "content.status"}}}
		},
		"malformed fulfillment operation": func(d *product.MethodContracts) {
			d.Groups[0].Dependencies = []product.MethodDependency{{ID: "domain.future_content", Kind: "domain", Required: true, FulfillmentOperations: []string{"../content/status"}}}
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			var declaration product.MethodContracts
			if err := json.Unmarshal(canonical, &declaration); err != nil {
				t.Fatal(err)
			}
			mutate(&declaration)
			body, err := json.Marshal(declaration)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := product.ReadMethodContracts(bytes.NewReader(body)); err == nil {
				t.Fatal("invalid declaration admitted")
			}
		})
	}
	for name, body := range map[string][]byte{
		"unknown field": append([]byte(`{"unknown":true,`), canonical[1:]...),
		"trailing JSON": append(append([]byte{}, canonical...), []byte(` {}`)...),
		"invalid UTF8":  {0xff},
		"oversize":      bytes.Repeat([]byte(" "), (128<<10)+1),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := product.ReadMethodContracts(bytes.NewReader(body)); err == nil {
				t.Fatal("invalid factory bytes admitted")
			}
		})
	}
}

func TestMethodPlansResolveMaintenanceKeepWikiUnavailable(t *testing.T) {
	f := newMethodQueryFixture(t)
	client := f.connect(t, "")
	before := methodTreeSnapshot(t, f.root)
	var plan struct {
		State        string `json:"state"`
		Execution    string `json:"execution_state"`
		Dependencies []struct {
			ID          string   `json:"id"`
			State       string   `json:"state"`
			Reason      string   `json:"reason"`
			Fulfillment []string `json:"fulfillment_operations"`
			Missing     []string `json:"missing_operations"`
		} `json:"dependencies"`
	}
	methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "dream-memory"}, &plan)
	if plan.State != "requirements_unavailable" || plan.Execution != "not_executed" {
		t.Fatalf("missing domain operations were presented as available: %+v", plan)
	}
	want := map[string][]string{
		"domain.maintenance_lifecycle": {"maintenance.enroll", "maintenance.pause", "maintenance.qualify", "maintenance.report", "maintenance.resume", "maintenance.revoke", "maintenance.run", "maintenance.status"},
		"domain.workspace_navigation":  {"wiki.read", "wiki.source", "wiki.status"},
	}
	unattested := map[string]bool{"host.trusted_review": true, "host.background_wake": true}
	for _, dependency := range plan.Dependencies {
		if unattested[dependency.ID] {
			if dependency.State != "unavailable" || dependency.Reason != "dependency_not_attested_by_method_selection" || len(dependency.Fulfillment) != 0 || len(dependency.Missing) != 0 {
				t.Fatalf("method selection attested host capability: %+v", dependency)
			}
			delete(unattested, dependency.ID)
			continue
		}
		operations, ok := want[dependency.ID]
		state, reason, missing := "unavailable", "owning_operations_not_registered", operations
		if dependency.ID == "domain.maintenance_lifecycle" {
			state, reason, missing = "available", "owning_operations_registered", nil
		}
		if !ok || dependency.State != state || dependency.Reason != reason || !reflect.DeepEqual(dependency.Fulfillment, operations) || len(dependency.Missing) != len(missing) || (len(missing) > 0 && !reflect.DeepEqual(dependency.Missing, missing)) {
			t.Fatalf("missing dependency lost its exact owning requirements: %+v", dependency)
		}
		delete(want, dependency.ID)
	}
	if len(unattested) != 0 {
		t.Fatalf("unattested host dependencies were omitted: %v", unattested)
	}
	if len(want) != 0 {
		t.Fatalf("missing dependencies were omitted: %v", want)
	}
	if !reflect.DeepEqual(before, methodTreeSnapshot(t, f.root)) {
		t.Fatal("dependency planning changed owner/workspace state")
	}
}

func TestMethodQueryPacketRestartContinuation(t *testing.T) {
	f := newMethodQueryFixture(t)
	binaryName := "maestro-mcp"
	if runtime.GOOS == "windows" {
		binaryName += ".exe"
	}
	binary := filepath.Join(f.root, binaryName)
	if out, err := exec.Command("go", "build", "-o", binary, "./cmd/maestro-mcp").CombinedOutput(); err != nil {
		t.Fatalf("build: %s: %v", out, err)
	}
	packet := filepath.Join(f.root, "plugin")
	if err := packaging.BuildPlugin(packaging.Options{Destination: packet, Server: binary, Version: "0.0.0", Binding: f.options}); err != nil {
		t.Fatal(err)
	}
	shipping := filepath.Join(packet, "scripts", binaryName)
	client := f.connect(t, shipping)
	before := methodTreeSnapshot(t, f.root)
	assertMethodQueries(t, client)
	if !reflect.DeepEqual(before, methodTreeSnapshot(t, f.root)) {
		t.Fatal("compiled method queries changed state")
	}
	var plan struct {
		State     string `json:"state"`
		Execution string `json:"execution_state"`
	}
	methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "execution-continuity"}, &plan)
	if plan.State != "requirements_available" || plan.Execution != "not_executed" {
		t.Fatalf("incorrect continuation plan: %+v", plan)
	}
	files, err := baseskills.Files("execution-continuity")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(files["SKILL.md"]), "## MCP product and plugin") {
		t.Fatal("packet instructions have no MCP-first caller")
	}
	if err := os.WriteFile(filepath.Join(f.options.WorkspacePath, "EXEMPLO.md"), []byte("Synthetic reviewed output.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	call := func(c *mcp.ClientSession, tool string, args map[string]any) map[string]any {
		t.Helper()
		var out map[string]any
		methodQueryDecode(t, c, tool, args, &out)
		return out
	}
	created := call(client, "maestro_apply", map[string]any{"operation": "execution.create", "objective": "Review synthetic packet continuation", "next_step": "Prepare synthetic output", "artifacts": []string{"EXEMPLO.md"}})
	id := created["item_id"].(string)
	started := call(client, "maestro_apply", map[string]any{"operation": "execution.start", "item_id": id, "expected_revision": created["state_revision"]})
	attempt := started["attempt_id"].(string)
	checkpoint := call(client, "maestro_apply", map[string]any{"operation": "execution.checkpoint", "item_id": id, "expected_revision": started["state_revision"], "attempt_id": attempt, "summary": "SK1_PACKET_RESTART", "next_step": "Read the same reviewed output after restart"})
	paused := call(client, "maestro_apply", map[string]any{"operation": "execution.pause", "item_id": id, "expected_revision": checkpoint["state_revision"], "attempt_id": attempt})
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	restarted := f.connect(t, shipping)
	next := call(restarted, "maestro_query", map[string]any{"operation": "execution.next", "item_id": id})
	if next["summary"] != "SK1_PACKET_RESTART" || next["next_step"] != "Read the same reviewed output after restart" {
		t.Fatalf("lost checkpoint: %+v", next)
	}
	resumed := call(restarted, "maestro_apply", map[string]any{"operation": "execution.resume", "item_id": id, "expected_revision": paused["state_revision"]})
	if resumed["attempt_id"] == attempt || resumed["attempt_id"] == "" {
		t.Fatal("restart reused or lost attempt")
	}
	if !methodQueryCall(t, restarted, "maestro_apply", map[string]any{"operation": "execution.checkpoint", "item_id": id, "expected_revision": resumed["state_revision"], "attempt_id": attempt, "summary": "stale", "next_step": "must refuse"}).IsError {
		t.Fatal("stale attempt admitted")
	}
	if !reflect.DeepEqual(before[filepath.Join("plugin", "scripts", binaryName)], methodTreeSnapshot(t, f.root)[filepath.Join("plugin", "scripts", binaryName)]) {
		t.Fatal("packet binary changed during continuation")
	}
}

func TestMethodQueryBrainIndexDefaultRequirementsSDK(t *testing.T) {
	f := newMethodQueryFixture(t)
	client := f.connect(t, "")
	before := methodTreeSnapshot(t, f.root)
	var plan struct {
		State        string   `json:"state"`
		Execution    string   `json:"execution_state"`
		Effects      []string `json:"effects"`
		Operations   []any    `json:"operations"`
		Dependencies []struct {
			ID          string   `json:"id"`
			Required    bool     `json:"required"`
			State       string   `json:"state"`
			Action      string   `json:"next_action"`
			Fulfillment []string `json:"fulfillment_operations"`
		} `json:"dependencies"`
	}
	methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "brain-index"}, &plan)
	if plan.State != "requirements_unavailable" || plan.Execution != "not_executed" {
		t.Fatalf("compiler selection claimed execution or admission: %+v", plan)
	}
	if !reflect.DeepEqual(plan.Effects, []string{"read", "host_execution", "write_owner"}) {
		t.Fatalf("default compiler owner publication effects are inaccurate: %v", plan.Effects)
	}
	if len(plan.Operations) != 0 {
		t.Fatalf("read-only memory/wiki operations cannot implement authored compilation: %v", plan.Operations)
	}
	want := map[string]bool{"domain.authored_brain_compiler": true, "host.command_run": true}
	for _, dependency := range plan.Dependencies {
		if !want[dependency.ID] || !dependency.Required || dependency.State != "unavailable" || dependency.Action == "" || len(dependency.Fulfillment) != 0 {
			t.Fatalf("default compiler capability can be omitted or fulfilled by navigation: %+v", dependency)
		}
		delete(want, dependency.ID)
	}
	if len(want) != 0 {
		t.Fatalf("default compiler prerequisites are missing: %v", want)
	}
	if !reflect.DeepEqual(before, methodTreeSnapshot(t, f.root)) {
		t.Fatal("compiler plan executed a command or published owner files")
	}
}

func TestMethodQueryArchitectureRecipeRequirementsSDK(t *testing.T) {
	f := newMethodQueryFixture(t)
	client := f.connect(t, "")
	before := methodTreeSnapshot(t, f.root)
	var plan struct {
		State       string   `json:"state"`
		Execution   string   `json:"execution_state"`
		Effects     []string `json:"effects"`
		Outputs     []string `json:"output_kinds"`
		Conditional []struct {
			Effect     string `json:"effect"`
			Condition  string `json:"condition"`
			Dependency string `json:"dependency"`
		} `json:"conditional_effects"`
		Dependencies []struct {
			ID       string `json:"id"`
			Required bool   `json:"required"`
			State    string `json:"state"`
			Action   string `json:"next_action"`
		} `json:"dependencies"`
	}
	methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "pocock-improve-codebase-architecture"}, &plan)
	if plan.State != "requirements_unavailable" || plan.Execution != "not_executed" {
		t.Fatalf("architecture selection claimed execution or admission: %+v", plan)
	}
	has := func(values []string, want string) bool {
		for _, v := range values {
			if v == want {
				return true
			}
		}
		return false
	}
	for _, effect := range []string{"read", "host_execution", "write_host"} {
		if !has(plan.Effects, effect) {
			t.Fatalf("canonical architecture recipe omitted effect %s", effect)
		}
	}
	if !has(plan.Outputs, "host_artifact") {
		t.Fatal("canonical saved HTML report is absent from outputs")
	}
	if has(plan.Effects, "write_workspace") {
		t.Fatal("follow-on glossary edits are not unconditional")
	}
	for _, id := range []string{"host.repository_read", "host.subagents", "host.artifact_write", "host.open_local_artifact"} {
		found := false
		for _, dep := range plan.Dependencies {
			if dep.ID == id {
				found = true
				if !dep.Required || dep.State != "unavailable" || dep.Action == "" {
					t.Fatalf("mandatory architecture capability incorrectly declared: %+v", dep)
				}
			}
		}
		if !found {
			t.Fatalf("canonical architecture recipe omitted dependency %s", id)
		}
	}
	found := false
	for _, effect := range plan.Conditional {
		if effect.Effect == "write_workspace" && effect.Dependency == "host.repository_write" && strings.Contains(effect.Condition, "user selects") {
			found = true
		}
	}
	if !found {
		t.Fatal("conditional glossary/domain edits have no declared condition and capability")
	}
	for _, dep := range plan.Dependencies {
		if dep.ID == "host.repository_write" && dep.Required {
			t.Fatal("conditional repository write was made mandatory")
		}
	}
	if !reflect.DeepEqual(before, methodTreeSnapshot(t, f.root)) {
		t.Fatal("architecture requirements executed host effects")
	}
}

func TestMethodPlansReflectRegisteredContentWithoutRuntimeAuthority(t *testing.T) {
	f := newMethodQueryFixture(t)
	client := f.connect(t, "")
	before := methodTreeSnapshot(t, f.root)
	var plan struct {
		State        string `json:"state"`
		Execution    string `json:"execution_state"`
		Dependencies []struct {
			ID          string   `json:"id"`
			State       string   `json:"state"`
			Reason      string   `json:"reason"`
			Action      string   `json:"next_action"`
			Fulfillment []string `json:"fulfillment_operations"`
			Missing     []string `json:"missing_operations"`
		} `json:"dependencies"`
	}
	methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "ingest-content"}, &plan)
	if plan.State != "requirements_unavailable" || plan.Execution != "not_executed" {
		t.Fatalf("registered content operations granted runtime or effect authority: %+v", plan)
	}
	registered := map[string][]string{
		"domain.document_conversion_operations": {"content.convert", "content.read", "content.status"},
		"domain.workspace_knowledge_operations": {"knowledge.append", "knowledge.collect", "knowledge.create"},
	}
	unattested := map[string]bool{"runtime.document_conversion": true, "domain.owner_knowledge_pages": true, "host.trusted_review": true}
	for _, dependency := range plan.Dependencies {
		if operations, ok := registered[dependency.ID]; ok {
			if dependency.State != "available" || dependency.Reason != "owning_operations_registered" || !reflect.DeepEqual(dependency.Fulfillment, operations) || len(dependency.Missing) != 0 || dependency.Action == "" {
				t.Fatalf("registered domain lost its exact owning operations: %+v", dependency)
			}
			delete(registered, dependency.ID)
			continue
		}
		if !unattested[dependency.ID] || dependency.State != "unavailable" || dependency.Reason != "dependency_not_attested_by_method_selection" || dependency.Action == "" {
			t.Fatalf("registered content attested another dependency: %+v", dependency)
		}
		delete(unattested, dependency.ID)
	}
	if len(registered) != 0 || len(unattested) != 0 {
		t.Fatalf("content plan omitted dependencies: registered=%v unattested=%v", registered, unattested)
	}
	var status struct {
		State string `json:"state"`
	}
	methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "content.status"}, &status)
	if status.State != "unavailable" {
		t.Fatalf("method planning installed or admitted the unseeded runtime: %+v", status)
	}
	plan.Dependencies = nil
	methodQueryDecode(t, client, "maestro_query", map[string]any{"operation": "methods.plan", "method_id": "yoda"}, &plan)
	provider := map[string]bool{"provider.senior_review_model": true, "domain.review_dispatch": true}
	if plan.State != "requirements_unavailable" || plan.Execution != "not_executed" {
		t.Fatalf("content registration granted provider execution: %+v", plan)
	}
	for _, dependency := range plan.Dependencies {
		if !provider[dependency.ID] || dependency.State != "unavailable" || dependency.Reason != "dependency_not_attested_by_method_selection" || dependency.Action == "" {
			t.Fatalf("content registration attested provider or review dispatch: %+v", dependency)
		}
		delete(provider, dependency.ID)
	}
	if len(provider) != 0 {
		t.Fatalf("provider prerequisites were omitted: %v", provider)
	}
	if !reflect.DeepEqual(before, methodTreeSnapshot(t, f.root)) {
		t.Fatal("content dependency planning changed owner/workspace/product state")
	}
}
