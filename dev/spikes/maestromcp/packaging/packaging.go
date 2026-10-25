// Package packaging builds recipient-bound experimental packages. Shared
// releases contain only executables; binding to a synthetic root happens locally.
package packaging

import (
	"archive/zip"
	"bytes"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	baseskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/skills"
	techcoreskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/tech-core/skills"
	assets "github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/plugin-assets"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/dev/spikes/maestromcp/spike"
)

type Options struct{ Destination, Server, StateRoot, Workspace, Version, Name string }

func prepare(o Options) (Options, string, error) {
	if o.Name == "" {
		o.Name = "maestro-architecture-spike"
	}
	if o.Version == "" {
		o.Version = "0.0.1"
	}
	if o.Name != "maestro-architecture-spike" && o.Name != "maestro-team-pilot" {
		return o, "", errors.New("unsupported experimental package name")
	}
	if !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(o.Version) {
		return o, "", errors.New("version must be numeric semantic version without leading zeros")
	}
	if o.Destination == "" || o.Server == "" || !filepath.IsAbs(o.StateRoot) || strings.Contains(o.StateRoot, "${") {
		return o, "", errors.New("destination, server and absolute literal state-root are required")
	}
	if _, err := spike.Open(o.StateRoot, o.Workspace); err != nil {
		return o, "", fmt.Errorf("existing synthetic fixture required: %w", err)
	}
	destination, err := filepath.Abs(o.Destination)
	if err != nil {
		return o, "", err
	}
	o.Destination = destination
	parent, err := filepath.EvalSymlinks(filepath.Dir(destination))
	if err != nil {
		return o, "", fmt.Errorf("destination parent must exist: %w", err)
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() {
		return o, "", errors.New("destination parent must be an existing directory")
	}
	home, err := os.UserHomeDir()
	if err != nil || !filepath.IsAbs(home) {
		return o, "", errors.New("current user's absolute home directory unavailable")
	}
	developer, err := filepath.EvalSymlinks(filepath.Join(home, "Developer"))
	if err != nil || !contains(developer, parent) {
		return o, "", errors.New("destination must be physically under the current user's Developer directory")
	}
	state, err := filepath.EvalSymlinks(o.StateRoot)
	if err != nil {
		return o, "", err
	}
	resolved := filepath.Join(parent, filepath.Base(destination))
	if contains(state, resolved) || contains(resolved, state) {
		return o, "", errors.New("package and state-root must not contain one another")
	}
	info, err = os.Lstat(o.Server)
	if err != nil {
		return o, "", fmt.Errorf("server binary: %w", err)
	}
	if !info.Mode().IsRegular() {
		return o, "", errors.New("server must be a regular native binary, not a symlink")
	}
	platform, err := binaryPlatform(o.Server)
	return o, platform, err
}

// Header validation identifies the declared platform; it does not attest code
// signing, native host execution, Desktop acceptance or Windows qualification.
func binaryPlatform(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if m, err := macho.NewFile(f); err == nil && m.Type == macho.TypeExec && (m.Cpu == macho.CpuArm64 || m.Cpu == macho.CpuAmd64) {
		return "darwin", nil
	}
	if p, err := pe.NewFile(f); err == nil && (p.Machine == pe.IMAGE_FILE_MACHINE_AMD64 || p.Machine == pe.IMAGE_FILE_MACHINE_ARM64) && p.Characteristics&0x0002 != 0 && p.Characteristics&0x2000 == 0 {
		return "win32", nil
	}
	if e, err := elf.NewFile(f); err == nil && e.Class == elf.ELFCLASS64 && (e.Machine == elf.EM_X86_64 || e.Machine == elf.EM_AARCH64) && (e.Type == elf.ET_EXEC || e.Type == elf.ET_DYN) {
		return "linux", nil
	}
	return "", errors.New("server must be a supported 64-bit native executable (Mach-O, PE or ELF)")
}
func binaryName(platform string) string {
	if platform == "win32" {
		return "maestro-mcp-spike.exe"
	}
	return "maestro-mcp-spike"
}

// BuildPlugin creates a new local plugin and never copies external state.
func BuildPlugin(options Options) error {
	o, platform, err := prepare(options)
	if err != nil {
		return err
	}
	if err := os.Mkdir(o.Destination, 0700); err != nil {
		return fmt.Errorf("destination must be new; nothing overwritten: %w", err)
	}
	for _, dir := range []string{".claude-plugin", "scripts", "hooks"} {
		if err := os.Mkdir(filepath.Join(o.Destination, dir), 0700); err != nil {
			return err
		}
	}
	name := binaryName(platform)
	if err := copyBinary(o.Server, filepath.Join(o.Destination, "scripts", name)); err != nil {
		return err
	}
	command := "${CLAUDE_PLUGIN_ROOT}/scripts/" + name
	manifests := map[string]any{
		".claude-plugin/plugin.json": map[string]any{"name": o.Name, "version": o.Version, "description": "Non-production synthetic MCP, plugin and filesystem experiment", "author": map[string]string{"name": "Agentic OS Brasil"}},
		".mcp.json":                  map[string]any{"mcpServers": map[string]any{"maestro": map[string]any{"type": "stdio", "command": command, "args": []string{"serve", "--root", o.StateRoot, "--workspace", o.Workspace}}}},
		"hooks/hooks.json":           map[string]any{"hooks": map[string]any{"SessionStart": []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command, "args": []string{"orient", "--revision", o.Version}}}}}}},
	}
	for path, body := range manifests {
		encoded, err := json.MarshalIndent(body, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(o.Destination, filepath.FromSlash(path)), append(encoded, '\n'), 0600); err != nil {
			return err
		}
	}
	err = fs.WalkDir(assets.Files, "skills", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		target := filepath.Join(o.Destination, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.Mkdir(target, 0700)
		}
		body, err := assets.Files.ReadFile(path)
		if err != nil {
			return err
		}
		body = []byte(strings.ReplaceAll(strings.ReplaceAll(string(body), "{{VERSION}}", o.Version), "/maestro-architecture-spike:", "/"+o.Name+":"))
		return os.WriteFile(target, body, 0600)
	})
	if err != nil {
		return err
	}
	return writePocockMethods(o)
}

func writePocockMethods(o Options) error {
	catalog, err := techcoreskills.Catalog()
	if err != nil {
		return err
	}
	methods := map[string]map[string][]byte{}
	for _, skill := range catalog.Skills {
		if strings.HasPrefix(skill.ID, "pocock-") {
			files, err := techcoreskills.Files(skill.ID)
			if err != nil {
				return err
			}
			methods[skill.ID] = files
		}
	}
	methods["interaction-profile"], err = baseskills.Files("interaction-profile")
	if err != nil {
		return err
	}
	for id, files := range methods {
		for relative, body := range files {
			target := filepath.Join(o.Destination, "skills", id, filepath.FromSlash(relative))
			if strings.HasSuffix(relative, ".md") {
				body = namespaceMethodReferences(body, o.Name)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			if err := os.WriteFile(target, body, 0600); err != nil {
				return err
			}
		}
	}
	provenance, err := json.MarshalIndent(map[string]any{
		"canonical":  json.RawMessage(techcoreskills.Provenance()),
		"projection": "markdown_invocation_namespace",
		"namespace":  o.Name,
		"hash_scope": "canonical_product_source_before_projection",
	}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(o.Destination, "pocock-upstream.json"), append(provenance, '\n'), 0600)
}

// Claude resolves plugin methods by qualified name; canonical IDs and metadata
// remain shared with ZIP and other runtimes.
func namespaceMethodReferences(body []byte, namespace string) []byte {
	frontmatter := []byte{}
	if bytes.HasPrefix(body, []byte("---\n")) {
		if end := bytes.Index(body[4:], []byte("\n---\n")); end >= 0 {
			end += 9
			frontmatter, body = body[:end], body[end:]
		}
	}
	reference := regexp.MustCompile("[/\\$\"`](?:pocock-[a-z0-9-]+|interaction-profile)\\b")
	projected := reference.ReplaceAllStringFunc(string(body), func(match string) string {
		prefix := match[:1]
		if prefix == "$" {
			prefix = "/"
		}
		return prefix + namespace + ":" + match[1:]
	})
	return append(append([]byte{}, frontmatter...), projected...)
}

// BuildExtension writes only a manifest and selected executable to a new MCPB.
func BuildExtension(options Options) error {
	if filepath.Ext(options.Destination) != ".mcpb" {
		return errors.New("destination must be a new .mcpb file")
	}
	if !filepath.IsAbs(options.Server) {
		return errors.New("absolute server path required")
	}
	o, platform, err := prepare(options)
	if err != nil {
		return err
	}
	if platform != "darwin" && platform != "win32" {
		return errors.New("Desktop extension requires macOS Mach-O or Windows PE executable")
	}
	path := "server/" + binaryName(platform)
	manifest, err := json.MarshalIndent(map[string]any{
		"manifest_version": "0.3", "name": o.Name, "version": o.Version,
		"description":   "Non-production local synthetic Maestro MCP and filesystem experiment",
		"author":        map[string]string{"name": "Agentic OS Brasil"},
		"server":        map[string]any{"type": "binary", "entry_point": path, "mcp_config": map[string]any{"command": "${__dirname}/" + path, "args": []string{"serve", "--root", o.StateRoot, "--workspace", o.Workspace}}},
		"compatibility": map[string]any{"platforms": []string{platform}},
	}, "", "  ")
	if err != nil {
		return err
	}
	binary, err := os.Open(o.Server)
	if err != nil {
		return err
	}
	defer binary.Close()
	artifact, err := os.OpenFile(o.Destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("destination must be new; nothing overwritten: %w", err)
	}
	archive := zip.NewWriter(artifact)
	writeErr := addEntry(archive, "manifest.json", 0600, bytes.NewReader(append(manifest, '\n')))
	if writeErr == nil {
		writeErr = addEntry(archive, path, 0700, binary)
	}
	return errors.Join(writeErr, archive.Close(), artifact.Close())
}
func addEntry(archive *zip.Writer, name string, mode os.FileMode, input io.Reader) error {
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	header.SetMode(mode)
	output, err := archive.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(output, input)
	return err
}
func contains(parent, child string) bool {
	relative, err := filepath.Rel(parent, child)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}
func copyBinary(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	return errors.Join(copyErr, out.Close())
}
