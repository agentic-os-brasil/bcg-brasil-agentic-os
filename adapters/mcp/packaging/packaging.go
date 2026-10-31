// Package packaging builds local participant packets without copying external state.
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
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	baseskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/skills"
	techcoreskills "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/tech-core/skills"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

type Options struct {
	Destination, Server, Version string
	Binding                      product.Options
}

func prepare(o Options) (Options, string, error) {
	if _, err := product.Open(o.Binding); err != nil {
		return o, "", err
	}
	if !regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$`).MatchString(o.Version) {
		return o, "", errors.New("numeric semantic version required")
	}
	if !filepath.IsAbs(o.Destination) || !filepath.IsAbs(o.Server) {
		return o, "", errors.New("absolute destination and server required")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(o.Destination))
	if err != nil {
		return o, "", err
	}
	o.Destination = filepath.Join(parent, filepath.Base(o.Destination))
	h, err := datahome.Resolve(datahome.Options{ProductRoot: o.Binding.ProductRoot, UserHome: o.Binding.UserHome, LookupEnv: o.Binding.LookupEnv})
	if err != nil {
		return o, "", err
	}
	for _, root := range []string{h.Home, o.Binding.WorkspacePath, o.Binding.ProductRoot} {
		if contains(root, o.Destination) || contains(o.Destination, root) {
			return o, "", errors.New("packet overlaps binding")
		}
	}
	info, err := os.Lstat(o.Server)
	if err != nil || !info.Mode().IsRegular() {
		return o, "", errors.New("regular native server required")
	}
	platform, err := binaryPlatform(o.Server)
	return o, platform, err
}
func args(o Options) []string {
	return []string{"serve", "--product-root", o.Binding.ProductRoot, "--user-home", o.Binding.UserHome, "--workspace", o.Binding.WorkspacePath}
}
func BuildPlugin(options Options) error {
	o, platform, err := prepare(options)
	if err != nil {
		return err
	}
	if err = os.Mkdir(o.Destination, 0700); err != nil {
		return err
	}
	for _, dir := range []string{".claude-plugin", "scripts", "skills/operator", "skills/resume"} {
		if err = os.MkdirAll(filepath.Join(o.Destination, dir), 0700); err != nil {
			return err
		}
	}
	name := binaryName(platform)
	if err = copyBinary(o.Server, filepath.Join(o.Destination, "scripts", name)); err != nil {
		return err
	}
	for path, value := range map[string]any{
		".claude-plugin/plugin.json": map[string]any{"name": "maestro-product", "version": o.Version, "description": "Local participant execution workflow", "author": map[string]string{"name": "Agentic OS Brasil"}},
		".mcp.json":                  map[string]any{"mcpServers": map[string]any{"maestro": map[string]any{"type": "stdio", "command": "${CLAUDE_PLUGIN_ROOT}/scripts/" + name, "args": args(o)}}},
	} {
		body, _ := json.MarshalIndent(value, "", "  ")
		if err = os.WriteFile(filepath.Join(o.Destination, path), body, 0600); err != nil {
			return err
		}
	}
	if err = os.WriteFile(filepath.Join(o.Destination, "skills/operator/SKILL.md"), []byte(operator), 0600); err != nil {
		return err
	}
	if err = os.WriteFile(filepath.Join(o.Destination, "skills/resume/SKILL.md"), []byte(resume), 0600); err != nil {
		return err
	}
	return writePocockMethods(o)
}

const operator = `---
name: operator
description: Run a new execution contract, checkpoint or complete work in the bound Maestro workspace.
---
Read maestro_context before work. Treat unavailable binding as a stop: ask the owner to inspect the locally selected paths. Startup never initializes state.
Use maestro_query execution.list for metadata and execution.next with the chosen item ID for bounded continuation. Create only after the owner requests work: objective, next_step and workspace-relative artifact paths define the immutable artifact snapshot contract. Read execution.contract for the immutable bounded criterion IDs and relative artifact paths; use those responses when collecting evidence.
Use maestro_apply with the current expected_revision. Start creates an attempt; checkpoint, pause, evidence and complete require that attempt_id. Save a concise checkpoint before pause. On revision_conflict or attempt_conflict, read current continuation and reconcile; never retry with invented state.
Collect artifact evidence after writing the requested result; complete revalidates it. Changed artifacts require new evidence. External sends, model execution, arbitrary commands, approval, deletion and full export have no tool authority here.
Keep owner confirmation and host permissions effective. Plugin methods are recipes; native agent scheduling and Desktop Skills installation remain unqualified.
`
const resume = `---
name: resume
description: Resume paused Maestro execution in another session or client.
---
Read maestro_context, list executions and ask which item only when ambiguous. Read execution.next for the selected item. If paused, call execution.resume with the current expected_revision, then read execution.next again. Read execution.contract to discover every criterion ID and workspace-relative path. Continue using the returned new attempt_id and contract metadata; preserve the prior checkpoint and owner scope. A stale writer must reread and reconcile.
`

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
		return "maestro-mcp.exe"
	}
	return "maestro-mcp"
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
				body = namespaceMethodReferences(body, "maestro-product")
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
		"namespace":  "maestro-product",
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
		"manifest_version": "0.3", "name": "maestro-product", "version": o.Version,
		"description":   "Local participant Maestro execution workflow",
		"author":        map[string]string{"name": "Agentic OS Brasil"},
		"server":        map[string]any{"type": "binary", "entry_point": path, "mcp_config": map[string]any{"command": "${__dirname}/" + path, "args": args(o)}},
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
