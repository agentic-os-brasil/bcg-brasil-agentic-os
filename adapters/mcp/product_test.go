package product_test

import (
	"archive/zip"
	"context"
	"encoding/json"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestStartupNeverBootstraps(t *testing.T) {
	if _, err := product.Open(product.Options{}); err == nil {
		t.Fatal("unbound startup admitted")
	}
}

func TestPublicWorkflow(t *testing.T)                  { runWorkflow(t, false) }
func TestCompiledStdioWorkflowAndPackets(t *testing.T) { runWorkflow(t, true) }
func runWorkflow(t *testing.T, stdio bool) {
	if runtime.GOOS == "windows" {
		t.Skip("verified owner admission unavailable")
	}
	base := t.TempDir()
	prod := filepath.Join(base, "product")
	home := filepath.Join(base, "owner")
	ws := filepath.Join(base, "workspace")
	for _, p := range []string{prod, home} {
		if err := os.Mkdir(p, 0700); err != nil {
			t.Fatal(err)
		}
	}
	o := product.Options{ProductRoot: prod, UserHome: home, WorkspacePath: ws, LookupEnv: func(string) (string, bool) { return "", false }}
	if _, err := product.Open(o); err == nil {
		t.Fatal("startup initialized missing state")
	}
	binary := ""
	if stdio {
		binary = filepath.Join(base, "maestro-mcp")
		build := exec.Command("go", "build", "-o", binary, "./cmd/maestro-mcp")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("build: %s %v", out, err)
		}
		init := exec.Command(binary, "init", "--product-root", prod, "--user-home", home, "--workspace", ws)
		if out, err := init.CombinedOutput(); err != nil {
			t.Fatalf("init: %s %v", out, err)
		}
	}
	h, err := datahome.InitializeFresh(datahome.Options{ProductRoot: prod, UserHome: home, LookupEnv: o.LookupEnv})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: ws, DataRoot: filepath.Join(h.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"result.md", "second.md"} {
		if err := os.WriteFile(filepath.Join(ws, path), []byte("first"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	connect := func() *mcp.ClientSession {
		if stdio {
			cmd := exec.Command(binary, "serve", "--product-root", prod, "--user-home", home, "--workspace", ws)
			cs, err := mcp.NewClient(&mcp.Implementation{Name: "stdio-test", Version: "1"}, nil).Connect(context.Background(), &mcp.CommandTransport{Command: cmd, TerminateDuration: time.Second}, nil)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { cs.Close() })
			return cs
		}
		b, err := product.Open(o)
		if err != nil {
			t.Fatal(err)
		}
		a, z := mcp.NewInMemoryTransports()
		ss, err := b.Server().Connect(context.Background(), a, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { ss.Close() })
		client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
		cs, err := client.Connect(context.Background(), z, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { cs.Close() })
		return cs
	}
	a, b := connect(), connect()
	call := func(c *mcp.ClientSession, tool string, args map[string]any, wantError bool) map[string]any {
		t.Helper()
		out, err := c.CallTool(context.Background(), &mcp.CallToolParams{Name: tool, Arguments: args})
		if err != nil {
			t.Fatal(err)
		}
		if out.IsError != wantError {
			t.Fatalf("%s %#v: %+v", tool, args, out)
		}
		var v map[string]any
		if err := json.Unmarshal([]byte(out.Content[0].(*mcp.TextContent).Text), &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	create := call(a, "maestro_apply", map[string]any{"operation": "execution.create", "objective": "deliver result", "next_step": "write result", "artifacts": []string{"result.md", "second.md"}}, false)
	id := create["item_id"].(string)
	call(a, "maestro_apply", map[string]any{"operation": "execution.create", "objective": "private second objective", "next_step": "later", "artifacts": []string{"result.md"}}, false)
	page := call(a, "maestro_query", map[string]any{"operation": "execution.list", "offset": 0, "limit": 1}, false)
	if page["has_more"] != true || len(page["items"].([]any)) != 1 {
		t.Fatalf("pagination: %v", page)
	}
	row := page["items"].([]any)[0].(map[string]any)
	if _, leaked := row["objective"]; leaked {
		t.Fatal("list exported contract content")
	}

	mutate := func(c *mcp.ClientSession, op string, rev float64, attempt string, extra map[string]any, fail bool) map[string]any {
		t.Helper()
		in := map[string]any{"operation": "execution." + op, "item_id": id, "expected_revision": rev}
		if attempt != "" {
			in["attempt_id"] = attempt
		}
		for k, v := range extra {
			in[k] = v
		}
		return call(c, "maestro_apply", in, fail)
	}
	started := mutate(a, "start", 1, "", nil, false)
	attempt := started["attempt_id"].(string)
	cp := mutate(a, "checkpoint", 2, attempt, map[string]any{"summary": "result prepared", "next_step": "verify result"}, false)
	mutate(b, "pause", 2, attempt, nil, true)
	mutate(a, "pause", cp["state_revision"].(float64), attempt, nil, false)
	restarted := connect()
	discovered := call(restarted, "maestro_query", map[string]any{"operation": "execution.list", "limit": 32}, false)
	var pausedMetadata map[string]any
	for _, row := range discovered["items"].([]any) {
		candidate := row.(map[string]any)
		if candidate["state"] == "paused" {
			if pausedMetadata != nil {
				t.Fatal("ambiguous paused fixture")
			}
			pausedMetadata = candidate
		}
	}
	if pausedMetadata == nil {
		t.Fatal("fresh client cannot discover paused item")
	}
	id = pausedMetadata["item_id"].(string)
	resumed := mutate(restarted, "resume", pausedMetadata["state_revision"].(float64), "", nil, false)
	rev := resumed["state_revision"].(float64)
	newAttempt := resumed["attempt_id"].(string)
	if newAttempt == attempt {
		t.Fatal("resume reused attempt")
	}
	mutate(a, "checkpoint", rev, attempt, map[string]any{"summary": "stale", "next_step": "bad"}, true)
	next := call(restarted, "maestro_query", map[string]any{"operation": "execution.next", "item_id": id}, false)
	if next["next_step"] != "verify result" {
		t.Fatalf("lost continuation: %v", next)
	}
	contract := call(restarted, "maestro_query", map[string]any{"operation": "execution.contract", "item_id": id}, false)
	criteria := contract["criteria"].([]any)
	criterion := criteria[0].(map[string]any)
	criterionID := criterion["id"].(string)
	artifactPath := criterion["path"].(string)
	if len(criteria) != 2 {
		t.Fatal("fresh client lost artifact count")
	}
	var evidence map[string]any
	for _, entry := range criteria {
		metadata := entry.(map[string]any)
		if _, err := os.Stat(filepath.Join(ws, metadata["path"].(string))); err != nil {
			t.Fatal(err)
		}
		evidence = mutate(restarted, "evidence", rev, newAttempt, map[string]any{"criterion_id": metadata["id"]}, false)
		rev = evidence["state_revision"].(float64)
	}
	if err := os.WriteFile(filepath.Join(ws, artifactPath), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	mutate(restarted, "complete", evidence["state_revision"].(float64), newAttempt, nil, true)
	fresh := mutate(restarted, "evidence", evidence["state_revision"].(float64), newAttempt, map[string]any{"criterion_id": criterionID}, false)
	complete := mutate(restarted, "complete", fresh["state_revision"].(float64), newAttempt, nil, false)
	if complete["state"] != "completed" {
		t.Fatal(complete)
	}
	for _, args := range []map[string]any{
		{"operation": "execution.list", "workspace_id": "foreign"},
		{"operation": "execution.next", "item_id": id, "objective": "cross variant"},
		{"operation": "execution.delete", "item_id": id},
	} {
		call(a, "maestro_query", args, true)
	}
	for _, path := range []string{"../escape", "/absolute/private", "nested/.bcgos/workspace.json"} {
		call(a, "maestro_apply", map[string]any{"operation": "execution.create", "objective": "bad", "next_step": "bad", "artifacts": []string{path}}, true)
	}
	call(a, "maestro_apply", map[string]any{"operation": "execution.create", "objective": strings.Repeat("private-content", 2000), "next_step": "bad", "artifacts": []string{"result.md"}}, true)
	call(a, "maestro_apply", map[string]any{"operation": "execution.create", "objective": "bad", "next_step": "bad", "artifacts": []string{"result.md"}, "require_yoda_review": true}, true)
	if stdio {
		for _, version := range []string{"0.1.0", "0.1.1"} {
			packet := packaging.Options{Destination: filepath.Join(base, "plugin-"+version), Server: binary, Version: version, Binding: o}
			if err := packaging.BuildPlugin(packet); err != nil {
				t.Fatal(err)
			}
			for _, skill := range []string{"operator", "resume", "interaction-profile", "pocock-tdd"} {
				if _, err := os.Stat(filepath.Join(packet.Destination, "skills", skill, "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			}
			manifest, err := os.ReadFile(filepath.Join(packet.Destination, ".mcp.json"))
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(manifest), home) || !strings.Contains(string(manifest), ws) {
				t.Fatal("plugin lost binding")
			}
			if runtime.GOOS == "darwin" {
				packet.Destination = filepath.Join(base, "maestro-"+version+".mcpb")
				if err := packaging.BuildExtension(packet); err != nil {
					t.Fatal(err)
				}
				archive, err := zip.OpenReader(packet.Destination)
				if err != nil {
					t.Fatal(err)
				}
				if len(archive.File) != 2 {
					t.Fatal("MCPB must contain only manifest and executable")
				}
				archive.Close()
			}
			after := call(connect(), "maestro_query", map[string]any{"operation": "execution.next", "item_id": id}, false)
			if after["state"] != "completed" {
				t.Fatal("package update lost external state")
			}
		}
	}
	foreign := filepath.Join(base, "other")
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: foreign, DataRoot: filepath.Join(h.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	fo := o
	fo.WorkspacePath = foreign
	bound, err := product.Open(fo)
	if err != nil {
		t.Fatal(err)
	}
	x, y := mcp.NewInMemoryTransports()
	fs, err := bound.Server().Connect(context.Background(), x, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fs.Close()
	fc, err := mcp.NewClient(&mcp.Implementation{Name: "foreign", Version: "1"}, nil).Connect(context.Background(), y, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer fc.Close()
	call(fc, "maestro_query", map[string]any{"operation": "execution.next", "item_id": id}, true)
	store, err := execution.OpenConfinedStore(filepath.Join(h.Home, "state"))
	if err != nil {
		t.Fatal(err)
	}
	ctxMetadata := call(a, "maestro_context", map[string]any{}, false)
	privateItem, err := store.Create(execution.CreateInput{WorkspaceID: ctxMetadata["workspace_id"].(string), Objective: "outside client", InitialNextStep: "later", Criteria: []execution.Criterion{{ID: "private", Type: execution.CriterionArtifactSnapshot, TargetRef: "bcgos://workspace/.bcgos/workspace.json"}}, AllowedRefs: []string{"bcgos://workspace/.bcgos/workspace.json"}})
	if err != nil {
		t.Fatal(err)
	}
	call(a, "maestro_apply", map[string]any{"operation": "execution.start", "item_id": privateItem.State.ItemID, "expected_revision": 1}, true)
	call(a, "maestro_query", map[string]any{"operation": "execution.contract", "item_id": privateItem.State.ItemID}, true)
	privateItem, err = store.Start(privateItem.State.WorkspaceID, privateItem.State.ItemID, 1)
	if err != nil {
		t.Fatal(err)
	}
	call(a, "maestro_apply", map[string]any{"operation": "execution.evidence", "item_id": privateItem.State.ItemID, "expected_revision": 2, "attempt_id": privateItem.State.ActiveAttemptID, "criterion_id": "private"}, true)
	untouched, err := store.Inspect(privateItem.State.WorkspaceID, privateItem.State.ItemID)
	if err != nil || untouched.State.StateRevision != 2 {
		t.Fatal("private evidence advanced revision")
	}

	commandItem, err := store.Create(execution.CreateInput{WorkspaceID: ctxMetadata["workspace_id"].(string), Objective: "other client command contract", InitialNextStep: "inspect", Criteria: []execution.Criterion{{ID: "tool", Type: execution.CriterionCommandCheck, Command: []string{"go", "version"}}}})
	if err != nil {
		t.Fatal(err)
	}
	call(a, "maestro_apply", map[string]any{"operation": "execution.start", "item_id": commandItem.State.ItemID, "expected_revision": 1}, true)
	commandItem, err = store.Start(commandItem.State.WorkspaceID, commandItem.State.ItemID, 1)
	if err != nil {
		t.Fatal(err)
	}
	call(a, "maestro_apply", map[string]any{"operation": "execution.evidence", "item_id": commandItem.State.ItemID, "expected_revision": 2, "attempt_id": commandItem.State.ActiveAttemptID, "criterion_id": "tool"}, true)
	collected, err := store.CollectEvidence(commandItem.State.WorkspaceID, commandItem.State.ItemID, execution.EvidenceInput{WorkspaceRoot: ws, ExpectedRevision: 2, AttemptID: commandItem.State.ActiveAttemptID, CriterionID: "tool"})
	if err != nil {
		t.Fatal(err)
	}
	call(a, "maestro_apply", map[string]any{"operation": "execution.complete", "item_id": commandItem.State.ItemID, "expected_revision": collected.Item.State.StateRevision, "attempt_id": commandItem.State.ActiveAttemptID}, true)
	entries, err := os.ReadDir(prod)
	if err != nil || len(entries) != 0 {
		t.Fatal("workflow wrote into product tree")
	}
	if err := os.WriteFile(filepath.Join(ws, ".bcgos", "workspace.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	call(a, "maestro_context", map[string]any{}, true)

}

func TestWindowsAdmissionRefused(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows admission contract")
	}
	base := t.TempDir()
	prod := filepath.Join(base, "product")
	home := filepath.Join(base, "owner")
	for _, path := range []string{prod, home} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	lookup := func(string) (string, bool) { return "", false }
	state, err := datahome.InitializeFresh(datahome.Options{ProductRoot: prod, UserHome: home, LookupEnv: lookup})
	if err == nil || state.Writable || state.Diagnostic != "ownership-unverified" {
		t.Fatalf("unverified Windows owner admitted: %+v %v", state, err)
	}
	if _, err := product.Open(product.Options{ProductRoot: prod, UserHome: home, WorkspacePath: filepath.Join(base, "workspace"), LookupEnv: lookup}); err == nil {
		t.Fatal("Windows product store admitted")
	}
	if _, err := os.Lstat(filepath.Join(home, ".maestro")); !os.IsNotExist(err) {
		t.Fatal("refused bootstrap created state")
	}
}

func TestCLIInitRefusesUnsafeBindingsWithoutWrites(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("positive init admission unavailable")
	}
	binary := filepath.Join(t.TempDir(), "maestro-mcp")
	if out, err := exec.Command("go", "build", "-o", binary, "./cmd/maestro-mcp").CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v", out, err)
	}
	for _, kind := range []string{"product", "private-home", "symlink", "symlink-ancestor", "brain-child", "metadata-child", "leaf-child"} {
		t.Run(kind, func(t *testing.T) {
			base := t.TempDir()
			prod, home, target := filepath.Join(base, "product"), filepath.Join(base, "owner"), filepath.Join(base, "target")
			for _, p := range []string{prod, home, target} {
				if err := os.Mkdir(p, 0700); err != nil {
					t.Fatal(err)
				}
			}
			ws := prod
			switch kind {
			case "brain-child", "metadata-child", "leaf-child":
				ws = filepath.Join(base, "workspace")
				if err := os.Mkdir(ws, 0700); err != nil {
					t.Fatal(err)
				}
				leaf := "brain"
				if kind == "metadata-child" {
					leaf = ".bcgos"
				}
				if kind == "leaf-child" {
					leaf = "README.md"
				}
				if err := os.Symlink(prod, filepath.Join(ws, leaf)); err != nil {
					t.Fatal(err)
				}
			case "private-home":
				ws = filepath.Join(home, ".maestro", "brain", "workspace")
			case "symlink", "symlink-ancestor":
				ws = filepath.Join(base, "link")
				if err := os.Symlink(target, ws); err != nil {
					t.Fatal(err)
				}
				if kind == "symlink-ancestor" {
					ws = filepath.Join(ws, "new", "workspace")
				}
			}
			cmd := exec.Command(binary, "init", "--product-root", prod, "--user-home", home, "--workspace", ws)
			out, err := cmd.CombinedOutput()
			if err == nil || strings.Contains(string(out), `"state":"ready"`) {
				t.Fatalf("unsafe init claimed ready: %s", out)
			}
			for _, p := range []string{prod, home, target} {
				entries, err := os.ReadDir(p)
				if err != nil || len(entries) != 0 {
					t.Fatalf("rejected init wrote into %s: %v %v", p, entries, err)
				}
			}
		})
	}
}
