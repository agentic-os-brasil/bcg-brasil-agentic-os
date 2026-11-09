package packaging

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	basememory "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/memory"
	baseruntime "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/runtime"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
)

func managedProductPayload(namespace string) (map[string][]byte, error) {
	files := map[string][]byte{}
	for name, body := range baseruntime.Files() {
		files["managed/runtime/"+name] = body
	}
	for name, body := range basememory.Files() {
		files["managed/memory/"+name] = body
	}
	agents, err := baseagents.Files()
	if err != nil {
		return nil, err
	}
	for name, body := range agents {
		files["managed/agents/"+name] = body
	}
	if namespace != "" {
		projections, err := claudeagents.Project(namespace)
		if err != nil {
			return nil, err
		}
		for _, projection := range projections {
			files[projection.Path] = projection.Body
		}
	}
	return files, nil
}
func runtimeSelections() map[string]any {
	return map[string]any{
		"user_home": map[string]any{"type": "directory", "title": "Owner directory", "description": "Select the existing owner directory outside the managed packet.", "required": true},
		"workspace": map[string]any{"type": "directory", "title": "Workspace", "description": "Select the workspace outside managed assets. Prepare it explicitly before starting the server.", "required": true},
	}
}
func portableArgs(root, command string) []string {
	return []string{command, "--packet", "--product-root", root, "--user-home", "${user_config.user_home}", "--workspace", "${user_config.workspace}"}
}
func portableMetadata(format, platform, version string) (map[string][]byte, string, error) {
	name := binaryName(platform)
	files := map[string][]byte{}
	server := "scripts/" + name
	if format == "plugin" {
		body, err := json.MarshalIndent(map[string]any{"name": "maestro-product", "version": version, "description": "Portable managed Maestro product; select external runtime paths.", "author": map[string]string{"name": "Agentic OS Brasil"}, "userConfig": runtimeSelections()}, "", "  ")
		if err != nil {
			return nil, "", err
		}
		files[".claude-plugin/plugin.json"] = append(body, '\n')
		body, err = json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"maestro": map[string]any{"type": "stdio", "command": "${CLAUDE_PLUGIN_ROOT}/" + server, "args": portableArgs("${CLAUDE_PLUGIN_ROOT}", "serve")}}}, "", "  ")
		if err != nil {
			return nil, "", err
		}
		files[".mcp.json"] = append(body, '\n')
	} else {
		server = "server/" + name
		body, err := json.MarshalIndent(map[string]any{"manifest_version": "0.3", "name": "maestro-product", "version": version, "description": "Portable managed Maestro product; explicit external preparation.", "author": map[string]string{"name": "Agentic OS Brasil"}, "user_config": runtimeSelections(), "server": map[string]any{"type": "binary", "entry_point": server, "mcp_config": map[string]any{"command": "${__dirname}/" + server, "args": portableArgs("${__dirname}", "serve")}}, "compatibility": map[string]any{"platforms": []string{platform}}}, "", "  ")
		if err != nil {
			return nil, "", err
		}
		files["manifest.json"] = append(body, '\n')
	}
	files["skills/operator/SKILL.md"] = []byte(operator)
	files["skills/resume/SKILL.md"] = []byte(resume)
	files["README-PACKET.md"] = []byte("# Portable Maestro packet\n\nSelect Owner directory and Workspace in the client configuration; no builder's private binding is included. Use only paths outside this packet. Startup never initializes state.\n\nPreparation: run the emitted " + server + " with `init --packet --product-root <this-packet> --user-home <selected-owner> --workspace <selected-workspace>`. Server uses `serve` with the same explicit selection. Doctor uses the existing `inspect` command with those arguments. Run preparation only after explicit approval; it does not install a service, acquire a model/runtime or change client settings.\n\nManaged files are immutable during these operations. The packet inventory records their bytes and modes and excludes itself. It is an integrity diagnostic, not publisher identity or owner approval. MCPB method files are inert canonical resources, not an assertion of native Desktop Skills installation.\n")
	return files, server, nil
}
func buildPortable(options Options, format string) error {
	if format == "mcpb" && filepath.Ext(options.Destination) != ".mcpb" {
		return errors.New("destination must be a new .mcpb file")
	}
	o, platform, err := prepare(options)
	if err != nil {
		return err
	}
	if format == "mcpb" && platform != "darwin" && platform != "win32" {
		return errors.New("Desktop extension requires macOS Mach-O or Windows PE executable")
	}
	if _, err := os.Lstat(o.Destination); err == nil {
		return errors.New("packet destination must be new")
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	stage, err := os.MkdirTemp(filepath.Dir(o.Destination), ".maestro-packet-")
	if err != nil {
		return err
	}
	created, err := os.Lstat(stage)
	if err != nil {
		return err
	}
	defer func() {
		if now, e := os.Lstat(stage); e == nil && os.SameFile(created, now) {
			_ = os.RemoveAll(stage)
		}
	}()
	namespace := ""
	if format == "plugin" {
		namespace = "maestro-product"
	}
	files, err := methodPayload(namespace)
	if err != nil {
		return err
	}
	managed, err := managedProductPayload(namespace)
	if err != nil {
		return err
	}
	for name, body := range managed {
		files[name] = body
	}
	metadata, server, err := portableMetadata(format, platform, o.Version)
	if err != nil {
		return err
	}
	for name, body := range metadata {
		if _, exists := files[name]; exists {
			return fmt.Errorf("packet asset collision: %s", name)
		}
		files[name] = body
	}
	native, err := nativePacketPayload(o, format)
	if err != nil {
		return err
	}
	for name, body := range native {
		if _, exists := files[name]; exists {
			return fmt.Errorf("native packet asset collision: %s", name)
		}
		files[name] = body
	}
	registry, err := packagedAuthority(o)
	if err != nil {
		return err
	}
	if len(registry) > 0 {
		files["managed/trust/release-authority-registry.json"] = registry
	}
	root, err := os.OpenRoot(stage)
	if err != nil {
		return err
	}
	defer root.Close()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if !packetPath(name) {
			return errors.New("invalid embedded packet asset")
		}
		if err := root.MkdirAll(filepath.Dir(filepath.FromSlash(name)), 0700); err != nil {
			return err
		}
		file, err := root.OpenFile(filepath.FromSlash(name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		_, writeErr := file.Write(files[name])
		modeErr := file.Chmod(0644)
		closeErr := file.Close()
		if writeErr != nil || modeErr != nil || closeErr != nil {
			return errors.Join(writeErr, modeErr, closeErr)
		}
	}
	if err := root.MkdirAll(filepath.Dir(filepath.FromSlash(server)), 0700); err != nil {
		return err
	}
	if err := copyBinary(o.Server, filepath.Join(stage, filepath.FromSlash(server))); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Join(stage, filepath.FromSlash(server)), 0755); err != nil {
		return err
	}
	if body, target, err := readNativeBinary(filepath.Join(stage, filepath.FromSlash(server))); err != nil || target != o.target || digest(body) != digest(o.serverBody) {
		return errors.New("selected server changed while packaging")
	}
	if err := writeInventory(stage, format, o.Version, platform, server); err != nil {
		return err
	}
	if _, err := ValidatePacket(stage); err != nil {
		return err
	}
	if format == "plugin" {
		if err := root.Close(); err != nil {
			return err
		}
		return publishPacket(stage, o.Destination)
	}
	artifact, err := os.CreateTemp(filepath.Dir(o.Destination), ".maestro-mcpb-")
	if err != nil {
		return err
	}
	temporary := artifact.Name()
	defer os.Remove(temporary)
	archive := zip.NewWriter(artifact)
	entries, err := scanPacket(stage)
	if err != nil {
		archive.Close()
		artifact.Close()
		return err
	}
	for i := range entries {
		if entries[i].Path == server {
			entries[i].Mode = 0755
		}
	}
	entries = append(entries, packetFile{Path: InventoryName, Mode: 0644})
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	var writeErr error
	for _, entry := range entries {
		file, err := root.Open(filepath.FromSlash(entry.Path))
		if err != nil {
			writeErr = err
			break
		}
		writeErr = addEntry(archive, entry.Path, os.FileMode(entry.Mode), file)
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			writeErr = errors.Join(writeErr, closeErr)
			break
		}
	}
	writeErr = errors.Join(writeErr, archive.Close(), artifact.Sync(), artifact.Close())
	if writeErr != nil {
		return writeErr
	}
	return publishPacket(temporary, o.Destination)
}

// Do not permit a self-consistent edited inventory to omit canonical managed
// configuration or recipes. Native bytes/public registry remain separate inputs.
func requiredPacketFiles(format, platform, version string) (map[string][]byte, string, error) {
	namespace := ""
	if format == "plugin" {
		namespace = "maestro-product"
	}
	files, err := methodPayload(namespace)
	if err != nil {
		return nil, "", err
	}
	managed, err := managedProductPayload(namespace)
	if err != nil {
		return nil, "", err
	}
	for name, body := range managed {
		files[name] = body
	}
	metadata, server, err := portableMetadata(format, platform, version)
	if err != nil {
		return nil, "", err
	}
	for name, body := range metadata {
		files[name] = body
	}
	return files, server, nil
}
