//go:build !windows

package packaging_test

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp/packaging"
	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Exercise the existing public builder with a real admitted build-time source.
// Runtime users must select their own external binding after distribution.
func TestPortableProductPacketDoesNotExportPrivateBinding(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	prod, owner, selected := filepath.Join(root, "managed source"), filepath.Join(root, "private owner [build-only]"), filepath.Join(root, "private workspace [build-only]")
	for _, path := range []string{prod, owner} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	lookup := func(string) (string, bool) { return "", false }
	home, err := datahome.InitializeFresh(datahome.Options{ProductRoot: prod, UserHome: owner, LookupEnv: lookup})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := workspace.Initialize(workspace.Options{WorkspacePath: selected, DataRoot: filepath.Join(home.Home, "state")}); err != nil {
		t.Fatal(err)
	}
	marker := []byte("PRIVATE_PACKET_EXPORT_MARKER_492")
	if err := os.WriteFile(filepath.Join(owner, "credentials-private.txt"), marker, 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "compiled-server")
	if output, err := exec.Command("go", "build", "-o", binary, "../cmd/maestro-mcp").CombinedOutput(); err != nil {
		t.Fatalf("real product build: %s %v", output, err)
	}
	destination := filepath.Join(root, "portable output [space] $() &")
	options := packaging.Options{Destination: destination, Server: binary, Version: "0.2.0", Binding: product.Options{ProductRoot: prod, UserHome: owner, WorkspacePath: selected, LookupEnv: lookup}}
	if err := packaging.BuildPlugin(options); err != nil {
		t.Fatal(err)
	}
	assertAgentPacket(t, destination, true)
	body, err := os.ReadFile(filepath.Join(destination, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var configuration struct {
		Servers map[string]struct {
			Command string   `json:"command"`
			Args    []string `json:"args"`
		} `json:"mcpServers"`
	}
	if json.Unmarshal(body, &configuration) != nil {
		t.Fatal("emitted MCP configuration is invalid")
	}
	for _, arg := range configuration.Servers["maestro"].Args {
		if arg == owner || arg == selected || arg == home.Home {
			t.Fatalf("distributed packet exports build-time private binding: %q", arg)
		}
	}
	for _, arg := range configuration.Servers["maestro"].Args {
		if arg == prod {
			t.Fatal("distributed packet points outside its emitted managed assets")
		}
	}
	if _, err := os.Stat(filepath.Join(destination, "managed", "runtime", "capabilities.json")); err != nil {
		t.Fatalf("runnable managed runtime payload missing: %v", err)
	}
	// Every emitted non-binary file and archive path must be independent of the
	// admitted source. A directory walk also protects the exact inventory role.
	for _, forbidden := range []string{owner, selected, home.Home, prod, string(marker)} {
		if err := filepath.WalkDir(destination, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			body, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.Contains(body, []byte(forbidden)) {
				return fmt.Errorf("private source exported in %s", path)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := packaging.ValidatePacket(destination); err != nil {
		t.Fatal(err)
	}
	t.Run("failed_stage_preserves_private_state", func(t *testing.T) {
		before := packetSnapshot(t, owner)
		failed := options
		failed.Destination = filepath.Join(root, "failed publication")
		failed.Binding.AuthorityRegistryPath = filepath.Join(root, "absent public registry")
		failed.Binding.AuthorityRegistrySHA256 = strings.Repeat("0", 64)
		if err := packaging.BuildPlugin(failed); err == nil {
			t.Fatal("invalid public authority accepted")
		}
		if _, err := os.Lstat(failed.Destination); !os.IsNotExist(err) {
			t.Fatalf("failed unpublished target remains: %v", err)
		}
		stages, err := filepath.Glob(filepath.Join(root, ".maestro-packet-*"))
		if err != nil || len(stages) != 0 {
			t.Fatalf("unpublished stage retained: %v %v", stages, err)
		}
		if !reflect.DeepEqual(before, packetSnapshot(t, owner)) {
			t.Fatal("failed staging changed private owner")
		}
	})
	t.Run("concurrent_publication_preserves_complete_winner", func(t *testing.T) {
		target := filepath.Join(root, "concurrent new destination")
		start := make(chan struct{})
		results := make(chan error, 2)
		for _, version := range []string{"0.2.1", "0.2.2"} {
			candidate := options
			candidate.Destination = target
			candidate.Version = version
			go func() { <-start; results <- packaging.BuildPlugin(candidate) }()
		}
		close(start)
		success := 0
		for range 2 {
			if <-results == nil {
				success++
			}
		}
		if success != 1 {
			t.Fatalf("exclusive publication successful writers: %d", success)
		}
		status, err := packaging.ValidatePacket(target)
		if err != nil || (status.Version != "0.2.1" && status.Version != "0.2.2") {
			t.Fatalf("winning target incomplete: %+v %v", status, err)
		}
		stages, err := filepath.Glob(filepath.Join(root, ".maestro-packet-*"))
		if err != nil || len(stages) != 0 {
			t.Fatalf("losing publication stage retained: %v %v", stages, err)
		}
	})
	t.Run("restrictive_umask", func(t *testing.T) {
		old := syscall.Umask(0077)
		defer syscall.Umask(old)
		restricted := options
		restricted.Destination = filepath.Join(root, "restrictive umask packet")
		if err := packaging.BuildPlugin(restricted); err != nil {
			t.Fatalf("portable builder under restrictive umask: %v", err)
		}
		if _, err := packaging.ValidatePacket(restricted.Destination); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("inventory_rejects_self_consistent_missing_or_extra_assets", func(t *testing.T) {
		for _, path := range []string{"managed/runtime/capabilities.json", "managed/agents/catalog.json", "agents/gamma-guardian.md"} {
			exercisePacketInventory(t, destination, path)
		}
	})
	sourceBefore := packetSnapshot(t, owner)
	packetBefore := packetSnapshot(t, destination)
	if err := packaging.BuildPlugin(options); err == nil {
		t.Fatal("existing destination overwritten")
	}
	if !reflect.DeepEqual(sourceBefore, packetSnapshot(t, owner)) || !reflect.DeepEqual(packetBefore, packetSnapshot(t, destination)) {
		t.Fatal("repeat build changed private or managed state")
	}
	// Move the real output away from all builder paths before execution.
	relocated := filepath.Join(root, "relocated plugin [space] $() &")
	if err := os.Rename(destination, relocated); err != nil {
		t.Fatal(err)
	}
	t.Run("plugin_emitted_runtime", func(t *testing.T) { exercisePortableRuntime(t, relocated, root) })
	if runtime.GOOS == "darwin" {
		options.Destination = filepath.Join(root, "portable archive [space] $() &.mcpb")
		if err := packaging.BuildExtension(options); err != nil {
			t.Fatal(err)
		}
		archiveBefore, err := os.ReadFile(options.Destination)
		if err != nil {
			t.Fatal(err)
		}
		if err := packaging.BuildExtension(options); err == nil {
			t.Fatal("existing MCPB overwritten")
		}
		archiveAfter, err := os.ReadFile(options.Destination)
		if err != nil || !bytes.Equal(archiveBefore, archiveAfter) {
			t.Fatal("existing MCPB changed")
		}
		extracted := filepath.Join(root, "relocated extension [space] $() &")
		extractPortableFixture(t, options.Destination, extracted)
		assertAgentPacket(t, extracted, false)
		if _, err := packaging.ValidatePacket(extracted); err != nil {
			t.Fatal(err)
		}
		runtimeRoot := filepath.Join(root, "extension runtime selections")
		if err := os.Mkdir(runtimeRoot, 0700); err != nil {
			t.Fatal(err)
		}
		t.Run("mcpb_emitted_runtime", func(t *testing.T) { exercisePortableRuntime(t, extracted, runtimeRoot) })
	}

}

// This expands only the documented client placeholders into argv. It exercises
// the emitted executable/configuration; it does not claim a client loader ran.
func selectedPacketCommand(t *testing.T, root, owner, workspace string) (string, []string) {
	t.Helper()
	var command string
	var args []string
	if body, err := os.ReadFile(filepath.Join(root, ".mcp.json")); err == nil {
		var cfg struct {
			Servers map[string]struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"mcpServers"`
		}
		if err := json.Unmarshal(body, &cfg); err != nil {
			t.Fatal(err)
		}
		command, args = cfg.Servers["maestro"].Command, cfg.Servers["maestro"].Args
	} else {
		var cfg struct {
			Server struct {
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
	}
	replace := strings.NewReplacer("${CLAUDE_PLUGIN_ROOT}", root, "${__dirname}", root, "${user_config.user_home}", owner, "${user_config.workspace}", workspace)
	command = replace.Replace(command)
	for i := range args {
		args[i] = replace.Replace(args[i])
	}
	if len(args) != 8 || args[0] != "serve" || args[1] != "--packet" {
		t.Fatalf("unexpected emitted runtime contract: %q", args)
	}
	return command, args
}

func packetSnapshot(t *testing.T, root string) map[string]string {
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
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
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

func extractPortableFixture(t *testing.T, archive, target string) {
	t.Helper()
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	reader, err := zip.OpenReader(archive)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if entry.Mode().IsDir() || !entry.Mode().IsRegular() || filepath.IsAbs(entry.Name) || strings.Contains(entry.Name, "..") {
			t.Fatal("unexpected emitted archive entry")
		}
		target := filepath.Join(target, filepath.FromSlash(entry.Name))
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			t.Fatal(err)
		}
		input, err := entry.Open()
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(io.LimitReader(input, 512<<20))
		input.Close()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, body, entry.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
}

func exercisePortableRuntime(t *testing.T, packet, outside string) {
	t.Helper()
	owner, selected := filepath.Join(outside, "runtime owner [space] $() &"), filepath.Join(outside, "runtime workspace [space] $() &")
	if err := os.Mkdir(owner, 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"authored.synthetic.md", "credentials.synthetic", "client-registration.synthetic.json"} {
		if err := os.WriteFile(filepath.Join(owner, name), []byte("PRESERVE_"+name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	command, args := selectedPacketCommand(t, packet, owner, selected)
	managed := packetSnapshot(t, packet)
	before := packetSnapshot(t, owner)
	run := func(verb string, wantError bool) []byte {
		t.Helper()
		argv := append([]string(nil), args...)
		argv[0] = verb
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, command, argv...)
		body, err := cmd.CombinedOutput()
		if (err != nil) != wantError {
			t.Fatalf("emitted %s: %s, %v", verb, body, err)
		}
		return body
	}
	run("serve", true)
	if !reflect.DeepEqual(before, packetSnapshot(t, owner)) {
		t.Fatal("read-only startup initialized owner")
	}
	var inspection struct {
		Writable   bool   `json:"writable"`
		Diagnostic string `json:"diagnostic"`
	}
	if err := json.Unmarshal(run("inspect", false), &inspection); err != nil || inspection.Writable {
		t.Fatalf("fresh doctor changed admission: %+v %v", inspection, err)
	}
	if !reflect.DeepEqual(before, packetSnapshot(t, owner)) {
		t.Fatal("doctor initialized owner")
	}
	var initialized struct {
		State       string `json:"state"`
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(run("init", false), &initialized); err != nil || initialized.State != "ready" || initialized.WorkspaceID == "" {
		t.Fatalf("emitted preparation failed: %+v %v", initialized, err)
	}
	prepared := packetSnapshot(t, owner)
	workspaceBefore := packetSnapshot(t, selected)
	transport := &mcp.CommandTransport{Command: exec.Command(command, args...), TerminateDuration: time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, err := mcp.NewClient(&mcp.Implementation{Name: "portable-packet-oracle", Version: "1"}, nil).Connect(ctx, transport, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.CallTool(ctx, &mcp.CallToolParams{Name: "maestro_context", Arguments: map[string]any{}})
	if err != nil || result.IsError {
		t.Fatalf("emitted server context: %+v %v", result, err)
	}
	if len(result.Content) != 1 {
		t.Fatal("missing context response")
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatal("non-text context response")
	}
	body := []byte(text.Text)
	var got struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := json.Unmarshal(body, &got); err != nil || got.WorkspaceID != initialized.WorkspaceID {
		t.Fatalf("emitted binding mismatch: %s %v", body, err)
	}
	assertEmittedAgentResources(t, ctx, client)
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(run("inspect", false), &inspection); err != nil || !inspection.Writable || inspection.Diagnostic != "ready" {
		t.Fatalf("prepared doctor: %+v %v", inspection, err)
	}
	run("init", false)
	if !reflect.DeepEqual(prepared, packetSnapshot(t, owner)) || !reflect.DeepEqual(workspaceBefore, packetSnapshot(t, selected)) {
		t.Fatal("repeat preparation/server/doctor changed owner, memory, registration or workspace")
	}
	// An invalid runtime selection must fail before changing either selected tree.
	bad := append([]string(nil), args...)
	bad[0] = "init"
	bad[len(bad)-1] = filepath.Join(packet, "unapproved workspace")
	if output, err := exec.Command(command, bad...).CombinedOutput(); err == nil {
		t.Fatalf("overlapping setup admitted: %s", output)
	}
	if !reflect.DeepEqual(prepared, packetSnapshot(t, owner)) || !reflect.DeepEqual(workspaceBefore, packetSnapshot(t, selected)) || !reflect.DeepEqual(managed, packetSnapshot(t, packet)) {
		t.Fatal("failed setup or runtime modified managed/private state")
	}
	// Corruption is not repaired or allowed to initialize a different selection.
	configuration := filepath.Join(packet, "managed", "runtime", "capabilities.json")
	original, err := os.ReadFile(configuration)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configuration, []byte("{}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	corrupt := packetSnapshot(t, packet)
	run("init", true)
	run("inspect", true)
	run("serve", true)
	if !reflect.DeepEqual(corrupt, packetSnapshot(t, packet)) || !reflect.DeepEqual(prepared, packetSnapshot(t, owner)) {
		t.Fatal("integrity refusal repaired assets or changed private state")
	}
	if err := os.WriteFile(configuration, original, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := packaging.ValidatePacket(packet); err != nil {
		t.Fatal(err)
	}
}

// Self-consistent rows must not authorize missing canonical payload or the
// addition of private/development files. No repair happens on validation.
func exercisePacketInventory(t *testing.T, packet, name string) {
	t.Helper()
	manifestPath := filepath.Join(packet, packaging.InventoryName)
	original, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	var inventory map[string]any
	if err := json.Unmarshal(original, &inventory); err != nil {
		t.Fatal(err)
	}
	rows := inventory["files"].([]any)
	assetPath := filepath.Join(packet, filepath.FromSlash(name))
	asset, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatal(err)
	}
	restore := func() {
		t.Helper()
		if err := os.WriteFile(manifestPath, original, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(assetPath, asset, 0644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(assetPath, 0644); err != nil {
			t.Fatal(err)
		}
	}
	defer restore()
	write := func() {
		t.Helper()
		body, err := json.Marshal(inventory)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifestPath, body, 0644); err != nil {
			t.Fatal(err)
		}
	}
	refused := func() {
		t.Helper()
		before := packetSnapshot(t, packet)
		if _, err := packaging.ValidatePacket(packet); err == nil {
			t.Fatal("altered payload admitted")
		}
		if !reflect.DeepEqual(before, packetSnapshot(t, packet)) {
			t.Fatal("validator repaired packet")
		}
	}
	var retained []any
	for _, row := range rows {
		if row.(map[string]any)["path"] != name {
			retained = append(retained, row)
		}
	}
	inventory["files"] = retained
	write()
	if err := os.Remove(assetPath); err != nil {
		t.Fatal(err)
	}
	refused()
	restore()
	inventory["files"] = rows
	extra := []byte("PRIVATE_EXPORT_MUST_NOT_BE_ALLOWED")
	extraPath := filepath.Join(packet, "private-extra.txt")
	if err := os.WriteFile(extraPath, extra, 0644); err != nil {
		t.Fatal(err)
	}
	inventory["files"] = append(append([]any(nil), rows...), map[string]any{"path": "private-extra.txt", "sha256": fmt.Sprintf("%x", sha256.Sum256(extra)), "bytes": len(extra), "mode": 0644})
	write()
	refused()
	if err := os.Remove(extraPath); err != nil {
		t.Fatal(err)
	}
	restore()
	inventory["files"] = append(append([]any(nil), rows...), rows[0])
	write()
	refused()
	restore()
	if err := os.WriteFile(manifestPath, bytes.Replace(original, []byte(`"schema_version": 1`), []byte(`"schema_version": 1, "schema_version": 1`), 1), 0644); err != nil {
		t.Fatal(err)
	}
	refused()
	restore()
	if err := os.Chmod(assetPath, 0777); err != nil {
		t.Fatal(err)
	}
	refused()
	restore()
	if err := os.Remove(assetPath); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(filepath.Dir(packet), "same-bytes-outside.json")
	if err := os.WriteFile(outside, asset, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, assetPath); err != nil {
		t.Fatal(err)
	}
	refused()
	if err := os.Remove(assetPath); err != nil {
		t.Fatal(err)
	}
	restore()
}

func assertEmittedAgentResources(t *testing.T, ctx context.Context, client *mcp.ClientSession) {
	t.Helper()
	canonical, err := baseagents.Files()
	if err != nil {
		t.Fatal(err)
	}
	agents, methods := 0, 0
	params := &mcp.ListResourcesParams{}
	seen := map[string]bool{}
	for {
		list, err := client.ListResources(ctx, params)
		if err != nil {
			t.Fatal(err)
		}
		for _, resource := range list.Resources {
			if strings.HasPrefix(resource.URI, "maestro://methods/") {
				methods++
			}
			if !strings.HasPrefix(resource.URI, "maestro://agents/") {
				continue
			}
			agents++
			path := strings.TrimPrefix(resource.URI, "maestro://agents/")
			want, ok := canonical[path]
			if !ok {
				t.Fatal("unexpected emitted agent URI", resource.URI)
			}
			result, err := client.ReadResource(ctx, &mcp.ReadResourceParams{URI: resource.URI})
			mime := "text/markdown"
			if strings.HasSuffix(path, ".json") {
				mime = "application/json"
			}
			if err != nil || result == nil || len(result.Contents) != 1 || result.Contents[0].Text != string(want) || result.Contents[0].MIMEType != mime || resource.MIMEType != mime || resource.Size != int64(len(want)) || resource.Meta["sha256"] != fmt.Sprintf("%x", sha256.Sum256(want)) {
				t.Fatal("emitted agent canonical resource mismatch", resource.URI, err)
			}
		}
		if list.NextCursor == "" {
			break
		}
		if seen[list.NextCursor] {
			t.Fatal("repeated emitted resource cursor")
		}
		seen[list.NextCursor] = true
		params.Cursor = list.NextCursor
	}
	if agents != 14 || methods != 249 {
		t.Fatalf("emitted discovery: agents=%d methods=%d", agents, methods)
	}
	for _, uri := range []string{"maestro://agents/../private.md", "maestro://agents/%2e%2e/private.md", "maestro://agents/maestro/AGENT.md?owner=true", "maestro://agents/brain/owner/self.md"} {
		if _, err := client.ReadResource(ctx, &mcp.ReadResourceParams{URI: uri}); err == nil {
			t.Fatal("unsafe emitted agent URI admitted", uri)
		}
	}
}
