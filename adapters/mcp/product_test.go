package product_test

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/execution"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
	base := physicalTempDir(t)
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
		binary = filepath.Join(base, productTestBinaryName())
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
	beforeRefusal := call(a, "maestro_query", map[string]any{"operation": "execution.list", "limit": 32}, false)
	for _, path := range []string{"../escape", "/absolute/private", "/", "//absolute/private", `\absolute\private`, `C:\absolute\private`, "C:/absolute/private", "nested/.bcgos/workspace.json"} {
		call(a, "maestro_apply", map[string]any{"operation": "execution.create", "objective": "bad", "next_step": "bad", "artifacts": []string{path}}, true)
		afterRefusal := call(a, "maestro_query", map[string]any{"operation": "execution.list", "limit": 32}, false)
		if !reflect.DeepEqual(beforeRefusal, afterRefusal) {
			t.Fatalf("unsafe artifact %q changed execution metadata", path)
		}
	}
	call(a, "maestro_apply", map[string]any{"operation": "execution.create", "objective": strings.Repeat("private-content", 2000), "next_step": "bad", "artifacts": []string{"result.md"}}, true)
	call(a, "maestro_apply", map[string]any{"operation": "execution.create", "objective": "bad", "next_step": "bad", "artifacts": []string{"result.md"}, "require_yoda_review": true}, true)
	if stdio {
		portableOwner, portableWorkspace := filepath.Join(base, "portable owner [space] $() &"), filepath.Join(base, "portable workspace [space] $() &")
		if err := os.Mkdir(portableOwner, 0700); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"authored.synthetic.md", "credentials.synthetic", "client-registration.synthetic.json"} {
			if err := os.WriteFile(filepath.Join(portableOwner, name), []byte("PRESERVE_"+name), 0600); err != nil {
				t.Fatal(err)
			}
		}
		portableWorkspaceID := ""
		for _, version := range []string{"0.1.0", "0.1.1"} {
			packet := packaging.Options{Destination: filepath.Join(base, "plugin [space] $() & "+version), Server: binary, Version: version, Binding: o}
			buildArgs := []string{"plugin", "--destination", packet.Destination, "--server", binary, "--version", version, "--product-root", o.ProductRoot, "--user-home", o.UserHome, "--workspace", o.WorkspacePath}
			if output, err := exec.Command(binary, buildArgs...).CombinedOutput(); err != nil {
				t.Fatalf("actual plugin builder: %s %v", output, err)
			}
			relocated := filepath.Join(base, "relocated plugin [space] $() & "+version)
			if err := os.Rename(packet.Destination, relocated); err != nil {
				t.Fatal(err)
			}
			packet.Destination = relocated
			for _, skill := range []string{"operator", "resume", "interaction-profile", "pocock-tdd"} {
				if _, err := os.Stat(filepath.Join(packet.Destination, "skills", skill, "SKILL.md")); err != nil {
					t.Fatal(err)
				}
			}
			pathsGuide, err := os.ReadFile(filepath.Join(packet.Destination, "skills", "interaction-profile", "resources", "owner-paths.md"))
			if err != nil || !strings.Contains(string(pathsGuide), "home paths") {
				t.Fatalf("packaged interaction profile lost its owner-path contract: %v", err)
			}
			manifest, err := os.ReadFile(filepath.Join(packet.Destination, ".mcp.json"))
			if err != nil {
				t.Fatal(err)
			}
			var configuration struct {
				Servers map[string]struct {
					Type    string   `json:"type"`
					Command string   `json:"command"`
					Args    []string `json:"args"`
				} `json:"mcpServers"`
			}
			if err := json.Unmarshal(manifest, &configuration); err != nil {
				t.Fatal("plugin binding is not valid JSON", err)
			}
			server, ok := configuration.Servers["maestro"]
			wantArgs := []string{"serve", "--packet", "--product-root", "${CLAUDE_PLUGIN_ROOT}", "--user-home", "${user_config.user_home}", "--workspace", "${user_config.workspace}"}
			if !ok || len(configuration.Servers) != 1 || server.Type != "stdio" || server.Command != "${CLAUDE_PLUGIN_ROOT}/scripts/"+productTestBinaryName() || !reflect.DeepEqual(server.Args, wantArgs) {
				t.Fatal("plugin lost its exact decoded executable or external selection arguments")
			}
			if !t.Run("portable_plugin_"+strings.ReplaceAll(version, ".", "_"), func(t *testing.T) {
				portableWorkspaceID = runPortableProductPacket(t, packet.Destination, "plugin", portableOwner, portableWorkspace, portableWorkspaceID)
			}) {
				t.FailNow()
			}
			if runtime.GOOS == "darwin" || runtime.GOOS == "windows" {
				packet.Destination = filepath.Join(base, "maestro [space] $() & "+version+".mcpb")
				buildArgs[0], buildArgs[2] = "mcpb", packet.Destination
				if output, err := exec.Command(binary, buildArgs...).CombinedOutput(); err != nil {
					t.Fatalf("actual MCPB builder: %s %v", output, err)
				}
				archive, err := zip.OpenReader(packet.Destination)
				if err != nil {
					t.Fatal(err)
				}
				if len(archive.File) < 250 {
					t.Fatal("MCPB lost its complete managed payload")
				}
				archive.Close()
				if !t.Run("portable_mcpb_"+strings.ReplaceAll(version, ".", "_"), func(t *testing.T) {
					extracted := filepath.Join(base, "extension [space] $() & "+version)
					extractProductPacketFixture(t, packet.Destination, extracted)
					portableWorkspaceID = runPortableProductPacket(t, extracted, "mcpb", portableOwner, portableWorkspace, portableWorkspaceID)
				}) {
					t.FailNow()
				}
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

func TestCLIInitRefusesUnsafeBindingsWithoutWrites(t *testing.T) {
	binary := filepath.Join(physicalTempDir(t), productTestBinaryName())
	if out, err := exec.Command("go", "build", "-o", binary, "./cmd/maestro-mcp").CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v", out, err)
	}
	for _, kind := range []string{"product", "private-home", "symlink", "symlink-ancestor", "brain-child", "metadata-child", "leaf-child"} {
		t.Run(kind, func(t *testing.T) {
			base := physicalTempDir(t)
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

// Match strict production binding admission while retaining deliberately selected test symlinks.
func physicalTempDir(t *testing.T) string {
	t.Helper()
	path, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func productTestBinaryName() string {
	if runtime.GOOS == "windows" {
		return "maestro-mcp.exe"
	}
	return "maestro-mcp"
}

// Exercise only the actual emitted product's fixed preparation/diagnostic/server
// journey. Expanding the two documented argv selections is not client-loader proof.
func runPortableProductPacket(t *testing.T, root, format, owner, selected, expectedID string) string {
	t.Helper()
	if status, err := packaging.ValidatePacket(root); err != nil || status.Format != format {
		t.Fatalf("complete emitted %s packet unavailable: %+v %v", format, status, err)
	}
	var command string
	var args []string
	rootToken := "${CLAUDE_PLUGIN_ROOT}"
	if format == "plugin" {
		var cfg struct {
			Servers map[string]struct {
				Type    string   `json:"type"`
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"mcpServers"`
		}
		body, err := os.ReadFile(filepath.Join(root, ".mcp.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &cfg); err != nil {
			t.Fatal(err)
		}
		server, ok := cfg.Servers["maestro"]
		if !ok || len(cfg.Servers) != 1 || server.Type != "stdio" {
			t.Fatal("emitted Code config is not one stdio server")
		}
		command, args = server.Command, server.Args
		if command != rootToken+"/scripts/"+productTestBinaryName() {
			t.Fatal("Code emitted a different executable")
		}
	} else if format == "mcpb" {
		rootToken = "${__dirname}"
		var cfg struct {
			Server struct {
				Type   string `json:"type"`
				Entry  string `json:"entry_point"`
				Config struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
				} `json:"mcp_config"`
			} `json:"server"`
		}
		body, err := os.ReadFile(filepath.Join(root, "manifest.json"))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &cfg); err != nil {
			t.Fatal(err)
		}
		command, args = cfg.Server.Config.Command, cfg.Server.Config.Args
		if cfg.Server.Type != "binary" || cfg.Server.Entry != "server/"+productTestBinaryName() || command != rootToken+"/"+cfg.Server.Entry {
			t.Fatal("MCPB emitted a different executable")
		}
	} else {
		t.Fatal("unknown packet fixture format")
	}
	wantArgs := []string{"serve", "--packet", "--product-root", rootToken, "--user-home", "${user_config.user_home}", "--workspace", "${user_config.workspace}"}
	if !reflect.DeepEqual(args, wantArgs) {
		t.Fatal("emitted argv captures a private binding or changes the runtime contract")
	}
	replace := strings.NewReplacer(rootToken, root, "${user_config.user_home}", owner, "${user_config.workspace}", selected)
	command = replace.Replace(command)
	for i := range args {
		args[i] = replace.Replace(args[i])
	}
	managedBefore := methodTreeSnapshot(t, root)
	ownerBefore := methodTreeSnapshot(t, owner)
	var workspaceBefore map[string]string
	if expectedID != "" {
		workspaceBefore = methodTreeSnapshot(t, selected)
	}
	run := func(verb string, wantError bool) []byte {
		t.Helper()
		argv := append([]string(nil), args...)
		argv[0] = verb
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		body, err := exec.CommandContext(ctx, command, argv...).CombinedOutput()
		if (err != nil) != wantError {
			t.Fatalf("emitted %s: %s %v", verb, body, err)
		}
		return body
	}
	var inspection struct {
		Diagnostic string `json:"diagnostic"`
		Writable   bool   `json:"writable"`
	}
	if err := json.Unmarshal(run("inspect", false), &inspection); err != nil {
		t.Fatal(err)
	}
	if expectedID == "" {
		if inspection.Writable || inspection.Diagnostic != "fresh-unconfirmed" {
			t.Fatalf("fresh packet doctor: %+v", inspection)
		}
		run("serve", true)
		if _, err := os.Lstat(selected); !os.IsNotExist(err) {
			t.Fatal("startup or inspect created workspace")
		}
	} else if !inspection.Writable || inspection.Diagnostic != "ready" {
		t.Fatalf("existing packet doctor: %+v", inspection)
	}
	if !reflect.DeepEqual(ownerBefore, methodTreeSnapshot(t, owner)) {
		t.Fatal("read-only packet startup or inspection changed owner state")
	}
	var initialized struct {
		State       string `json:"state"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(run("init", false), &initialized); err != nil || initialized.State != "ready" || initialized.WorkspaceID == "" {
		t.Fatalf("emitted preparation: %+v %v", initialized, err)
	}
	if expectedID != "" {
		if initialized.WorkspaceID != expectedID {
			t.Fatal("new packet changed the existing workspace identity")
		}
		// Compare the pre-existing complete state before adopting any post-init
		// baseline. A first setup by a new packet must not erase old private data.
		if !reflect.DeepEqual(ownerBefore, methodTreeSnapshot(t, owner)) || !reflect.DeepEqual(workspaceBefore, methodTreeSnapshot(t, selected)) {
			t.Fatal("first init by new packet changed existing private owner or workspace state")
		}
	}
	if expectedID == "" {
		if err := os.WriteFile(filepath.Join(selected, "authored-preserved.synthetic.md"), []byte("PRESERVE_PORTABLE_AUTHORED_WORK"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(selected, "brain", "knowledge"), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(selected, "brain", "knowledge", "packet.md"), []byte("# Authored packet evidence\n\nPRESERVE_AUTHORED_PAGE_495\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	const historyText = "PORTABLE_REVIEWED_PRIVATE_HISTORY_492"
	reviewed := 0
	trustedReview := func(_ context.Context, request *mcp.ElicitRequest) (*mcp.ElicitResult, error) {
		if request == nil || request.Params == nil || !strings.Contains(request.Params.Message, historyText) {
			return &mcp.ElicitResult{Action: "decline"}, nil
		}
		reviewed++
		return &mcp.ElicitResult{Action: "accept", Content: map[string]any{"approved": true}}, nil
	}
	client, err := mcp.NewClient(&mcp.Implementation{Name: "emitted-product-packet", Version: "1"}, &mcp.ClientOptions{ElicitationHandler: trustedReview}).Connect(ctx, &mcp.CommandTransport{Command: exec.CommandContext(ctx, command, args...), TerminateDuration: time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "maestro_context", Arguments: map[string]any{}})
	if err != nil || result == nil || result.IsError || len(result.Content) != 1 {
		t.Fatalf("emitted server context: %+v %v", result, err)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("non-text emitted context")
	}
	var contextResult struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal([]byte(text.Text), &contextResult); err != nil || contextResult.WorkspaceID != initialized.WorkspaceID {
		t.Fatal("emitted server bound another workspace", err)
	}
	// Compare the entire fixed public resource set and exact canonical bytes,
	// independently from the projected native definitions inside the packet.
	var provenance struct {
		Resources []struct {
			URI, SHA256 string
			Bytes       int
		} `json:"embedded_resources"`
	}
	provenanceBody, err := os.ReadFile(filepath.Join(root, "maestro-packet.json"))
	if err != nil || json.Unmarshal(provenanceBody, &provenance) != nil {
		t.Fatal("packet resource provenance missing", err)
	}
	resources, err := client.ListResources(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(resources.Resources) != len(provenance.Resources) {
		t.Fatal("packet and actual transport resource inventory differ")
	}
	listed := map[string]*mcp.Resource{}
	for _, resource := range resources.Resources {
		listed[resource.URI] = resource
	}
	for _, resource := range provenance.Resources {
		listedResource := listed[resource.URI]
		if listedResource == nil {
			t.Fatal("packet public resource omitted", resource.URI)
		}
		read, err := client.ReadResource(ctx, &mcp.ReadResourceParams{URI: resource.URI})
		if err != nil || len(read.Contents) != 1 {
			t.Fatal("packet resource unavailable", resource.URI, err)
		}
		body := []byte(read.Contents[0].Text)
		if len(body) != resource.Bytes || fmt.Sprintf("%x", sha256.Sum256(body)) != resource.SHA256 || listedResource.Size != int64(len(body)) {
			t.Fatal("packet canonical resource bytes changed", resource.URI)
		}
	}
	collected := contentCall(t, client, "maestro_query", map[string]any{"operation": "knowledge.collect", "purpose": "Read the owner's authored page through the actual carrier", "pages": []string{"knowledge/packet.md"}}, false)
	pages, ok := collected["pages"].([]any)
	if !ok || len(pages) != 1 || pages[0].(map[string]any)["content"] != "# Authored packet evidence\n\nPRESERVE_AUTHORED_PAGE_495\n" {
		t.Fatal("carrier lost authored knowledge readback", collected)
	}
	if expectedID == "" {
		captured := memoryCall(t, client, "memory.capture", map[string]any{"text": historyText}, false)
		if captured["eligible"] != true || captured["revision"] != float64(1) || reviewed != 1 {
			t.Fatal("canonical private canary did not receive exact trusted review")
		}
	} else if reviewed != 0 {
		t.Fatal("ordinary packet readback unexpectedly requested memory mutation")
	}
	page := memoryCall(t, client, "memory.list", map[string]any{"limit": 2}, false)
	captures, ok := page["captures"].([]any)
	if !ok || len(captures) != 1 {
		t.Fatal("canonical reviewed private history missing after packet setup")
	}
	capture, ok := captures[0].(map[string]any)
	if !ok || capture["id"] == nil {
		t.Fatal("private history has no canonical identity")
	}
	read := memoryCall(t, client, "memory.read", map[string]any{"capture_id": capture["id"]}, false)
	readCapture, ok := read["capture"].(map[string]any)
	if !ok || read["text"] != historyText || readCapture["id"] != capture["id"] || readCapture["revision"] != float64(1) || readCapture["eligible"] != true {
		t.Fatal("canonical private history changed across packet setup")
	}
	authored, err := os.ReadFile(filepath.Join(selected, "authored-preserved.synthetic.md"))
	if err != nil || string(authored) != "PRESERVE_PORTABLE_AUTHORED_WORK" {
		t.Fatal("existing authored workspace canary was not preserved", err)
	}
	call := func(c *mcp.ClientSession, tool string, args map[string]any, fail bool) map[string]any {
		return contentCall(t, c, tool, args, fail)
	}
	const summary = "PORTABLE_CHECKPOINT_495"
	const nextStep = "Resume the same reviewed packet work"
	items := call(client, "maestro_query", map[string]any{"operation": "execution.list", "limit": 32}, false)["items"].([]any)
	var itemID string
	if expectedID == "" {
		created := call(client, "maestro_apply", map[string]any{"operation": "execution.create", "objective": "Continue the same work across emitted Code and MCPB carriers", "next_step": "Prepare packet evidence", "artifacts": []string{"authored-preserved.synthetic.md"}}, false)
		itemID = created["item_id"].(string)
		started := call(client, "maestro_apply", map[string]any{"operation": "execution.start", "item_id": itemID, "expected_revision": created["state_revision"]}, false)
		checkpoint := call(client, "maestro_apply", map[string]any{"operation": "execution.checkpoint", "item_id": itemID, "expected_revision": started["state_revision"], "attempt_id": started["attempt_id"], "summary": summary, "next_step": nextStep}, false)
		call(client, "maestro_apply", map[string]any{"operation": "execution.pause", "item_id": itemID, "expected_revision": checkpoint["state_revision"], "attempt_id": started["attempt_id"]}, false)
	} else {
		if len(items) != 1 {
			t.Fatal("carrier continuation is missing or ambiguous", items)
		}
		itemID = items[0].(map[string]any)["item_id"].(string)
	}
	prior := call(client, "maestro_query", map[string]any{"operation": "execution.next", "item_id": itemID}, false)
	if prior["state"] != "paused" || prior["summary"] != summary || prior["next_step"] != nextStep || prior["attempt_id"] == "" {
		t.Fatal("carrier changed checkpoint revision, attempt or summary", prior)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, err := mcp.NewClient(&mcp.Implementation{Name: "restarted-emitted-packet", Version: "1"}, nil).Connect(ctx, &mcp.CommandTransport{Command: exec.CommandContext(ctx, command, args...), TerminateDuration: time.Second}, nil)
	if err != nil {
		t.Fatal(err)
	}
	replayed := call(restarted, "maestro_query", map[string]any{"operation": "execution.next", "item_id": itemID}, false)
	if !reflect.DeepEqual(prior, replayed) {
		t.Fatal("same-carrier restart changed continuation", prior, replayed)
	}
	resumed := call(restarted, "maestro_apply", map[string]any{"operation": "execution.resume", "item_id": itemID, "expected_revision": prior["state_revision"]}, false)
	if resumed["attempt_id"] == "" || resumed["attempt_id"] == prior["attempt_id"] {
		t.Fatal("carrier restart reused attempt identity")
	}
	call(restarted, "maestro_apply", map[string]any{"operation": "execution.checkpoint", "item_id": itemID, "expected_revision": prior["state_revision"], "attempt_id": resumed["attempt_id"], "summary": "stale revision", "next_step": "must refuse"}, true)
	call(restarted, "maestro_apply", map[string]any{"operation": "execution.checkpoint", "item_id": itemID, "expected_revision": resumed["state_revision"], "attempt_id": prior["attempt_id"], "summary": "stale attempt", "next_step": "must refuse"}, true)
	call(restarted, "maestro_apply", map[string]any{"operation": "execution.pause", "item_id": itemID, "expected_revision": resumed["state_revision"], "attempt_id": resumed["attempt_id"]}, false)
	if err := restarted.Close(); err != nil {
		t.Fatal(err)
	}
	// Fresh setup intentionally added the reviewed canary. Every later first
	// init was already checked against its complete pre-init state above.
	preparedOwner, preparedWorkspace := methodTreeSnapshot(t, owner), methodTreeSnapshot(t, selected)
	if err := json.Unmarshal(run("inspect", false), &inspection); err != nil || !inspection.Writable || inspection.Diagnostic != "ready" {
		t.Fatalf("ready emitted doctor: %+v %v", inspection, err)
	}
	run("init", false)
	bad := append([]string(nil), args...)
	bad[0] = "init"
	bad[len(bad)-1] = filepath.Join(root, "forbidden private overlap")
	if _, err := exec.CommandContext(ctx, command, bad...).CombinedOutput(); err == nil {
		t.Fatal("overlapping packet setup admitted")
	}
	if !reflect.DeepEqual(preparedOwner, methodTreeSnapshot(t, owner)) || !reflect.DeepEqual(preparedWorkspace, methodTreeSnapshot(t, selected)) {
		t.Fatal("emitted startup/repeat/failure changed private owner, authored, credential or registration state")
	}
	if !reflect.DeepEqual(managedBefore, methodTreeSnapshot(t, root)) {
		t.Fatal("emitted runtime wrote managed assets")
	}
	for _, name := range []string{"authored.synthetic.md", "credentials.synthetic", "client-registration.synthetic.json"} {
		body, err := os.ReadFile(filepath.Join(owner, name))
		if err != nil || string(body) != "PRESERVE_"+name {
			t.Fatal("private neighbour not preserved", name, err)
		}
	}
	return initialized.WorkspaceID
}

// Unpack only the real builder's bounded ordinary files into an exclusive test
// directory. The product inventory then independently validates the exact payload.
func extractProductPacketFixture(t *testing.T, archive, destination string) {
	t.Helper()
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if !fs.ValidPath(entry.Name) || strings.Contains(entry.Name, "\\") || !entry.Mode().IsRegular() || entry.UncompressedSize64 > 512<<20 {
			t.Fatal("unsafe emitted archive entry")
		}
		path := filepath.Join(destination, filepath.FromSlash(entry.Name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		input, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		output, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, entry.Mode().Perm())
		if err != nil {
			input.Close()
			t.Fatal(err)
		}
		_, copyErr := io.Copy(output, io.LimitReader(input, 512<<20))
		inputErr, outputErr := input.Close(), output.Close()
		if copyErr != nil || inputErr != nil || outputErr != nil {
			t.Fatal(copyErr, inputErr, outputErr)
		}
	}
}
