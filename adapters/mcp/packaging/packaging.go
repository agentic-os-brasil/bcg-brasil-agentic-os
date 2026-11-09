// Package packaging builds local participant packets without copying external state.
package packaging

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"debug/elf"
	"debug/macho"
	"debug/pe"
	"encoding/json"
	"errors"
	"fmt"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode"
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
	registry, err := packagedAuthority(o)
	if err != nil {
		return err
	}
	if len(registry) > 0 {
		directory := filepath.Join(o.Destination, "managed", "trust")
		if err = os.MkdirAll(directory, 0700); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(directory, "release-authority-registry.json"), registry, 0600); err != nil {
			return err
		}
	}
	return writeMethods(o)
}

const operator = `---
name: operator
description: Start or resume reviewed owner onboarding, workspace memory or execution work in the bound Maestro workspace.
---
Read maestro_context for admitted binding metadata, then inspect the current maestro_query and maestro_apply schemas for implemented operations. Treat unavailable binding as a stop: ask the owner to inspect the locally selected paths. Startup never initializes state.
Discover installed methods through maestro_query methods.list, inspect an exact method_id with methods.plan, and read its exact returned resource_uri through methods.read. Requirements are metadata, not host qualification, execution or user approval; explicit-only methods remain manually selected.
For owner onboarding, read maestro://methods/maestro-onboarding/SKILL.md through MCP resources and query owner.status first. Offer a track only when track is selection_required; resume required/in_progress at its exact next_question, route review_required to owner.onboarding_review and trusted owner.confirm_onboarding, and continue ordinary work when complete. Select owner.track only for an explicit owner choice, because even the same selection invalidates final confirmation. Use owner.interview, owner.propose, owner.review and trusted owner.apply for each answer; re-read owner.status after application or confirmation. Declining or deferring preserves saved progress. Metadata and model assertions never approve a change. If trusted host review is unavailable, ask the local operator to use owner-review/owner-apply or owner-onboarding-review/owner-confirm-onboarding; local owner-track requires explicit --confirm. This recipe grants no shell or owner-file fallback.
Capture only text the user explicitly selects for memory, using memory.capture and exact trusted review; never auto-capture conversations. Discover IDs and visibility through memory.list or memory.search, then use memory.read or memory.context for eligible bounded pages. Use memory.correct or memory.delete with the current capture revision; memory.retention requires an explicit cutoff and current visibility revision. memory.consolidate runs local deterministic daily or weekly synthesis for an explicit date. Lifetime promotion requires canonical generation evidence and trusted review of the exact candidate.
For memory, use memory-review followed by memory-apply with the same JSON request on stdin, its exact --review-digest and explicit --confirm when trusted host review is unavailable. This is an operator fallback, not permission for the model to run arbitrary commands or supply approval.
For local documents, query content.status, use content.convert with a workspace-relative source_path, then content.read for bounded markdown/document slices. Review knowledge.create or knowledge.append against the exact artifact, page, text and expected_page_revision; use knowledge.collect for named page readback. When trusted review is unavailable, the local operator uses knowledge-review then knowledge-apply with the same JSON stdin, exact --review-digest and explicit --confirm. Conversion never installs a runtime or writes knowledge automatically.
For maintenance, query maintenance.status and maintenance.report; use exact trusted review for maintenance.qualify, maintenance.enroll, maintenance.pause, maintenance.resume and maintenance.revoke, carrying the actual expected_revision. Qualification observes the real named handler; enrollment is separate and never installs a background service. maintenance.run dispatches active due jobs only. Without trusted forms, the local operator uses maintenance-review/maintenance-apply with the same JSON stdin, exact --review-digest and explicit --confirm. For private memory navigation, use wiki.status, then wiki.read or wiki.source with the actual generation and returned references; queries never compile or repair. A stale or revoked view remains unavailable until an admitted owning job rebuilds it.
Use maestro_query execution.list for metadata and execution.next with the chosen item ID for bounded continuation. Create only after the owner requests work: objective, next_step and workspace-relative artifact paths define the immutable artifact snapshot contract. Read execution.contract for the immutable bounded criterion IDs and relative artifact paths; use those responses when collecting evidence.
Use maestro_apply with the current expected_revision. Start creates an attempt; checkpoint, pause, evidence and complete require that attempt_id. Save a concise checkpoint before pause. On revision_conflict or attempt_conflict, read current continuation and reconcile; never retry with invented state.
Collect artifact evidence after writing the requested result; complete revalidates it. Changed artifacts require new evidence. Scoped memory deletion and retention use their reviewed operations. External sends, model execution, arbitrary commands, approval, arbitrary file deletion and full export have no tool authority here.
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

func writeMethods(o Options) error {
	methods, err := product.PackagedMethods()
	if err != nil {
		return err
	}
	var ids []string
	for _, method := range methods.Methods {
		ids = append(ids, regexp.QuoteMeta(method.ID))
	}
	sort.Slice(ids, func(i, j int) bool { return len(ids[i]) > len(ids[j]) })
	reference := regexp.MustCompile("[/\\$\"`](?:" + strings.Join(ids, "|") + ")\\b")
	projectedHashes := map[string]string{}
	for _, method := range methods.Methods {
		for relative, body := range method.Files {
			path := "skills/" + method.ID + "/" + relative
			target := filepath.Join(o.Destination, filepath.FromSlash(path))
			if strings.HasSuffix(relative, ".md") {
				body = namespaceMethodReferences(body, "maestro-product", reference)
			}
			if relative == "SKILL.md" {
				end := bytes.Index(body[4:], []byte("\n---\n"))
				if end < 0 {
					return errors.New("embedded method frontmatter unavailable")
				}
				end += 9
				prefix := append([]byte(nil), body[:end]...)
				prefix = append(prefix, []byte("\nBefore applying this method, read [Maestro product authority](../../METHODS.md) and its [canonical policy](../../agent-skill-policy.json). Use the admitted implemented MCP operations for domain effects; an instruction does not grant filesystem, native CLI, installation or external-action authority.\n")...)
				body = append(prefix, body[end:]...)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
				return err
			}
			if err := os.WriteFile(target, body, 0600); err != nil {
				return err
			}
			projectedHashes[path] = fmt.Sprintf("%x", sha256.Sum256(body))
		}
	}
	provenance, err := json.MarshalIndent(map[string]any{
		"canonical":         json.RawMessage(methods.Assets["pocock-upstream.json"]),
		"projection":        "markdown_invocation_namespace",
		"namespace":         "maestro-product",
		"hash_scope":        "canonical_product_source_before_projection",
		"authority_overlay": "SKILL.md_pointer_to_packaged_METHODS.md",
	}, "", "  ")
	if err != nil {
		return err
	}
	methods.Assets["pocock-upstream.json"] = append(provenance, '\n')
	projection, err := json.MarshalIndent(map[string]any{"namespace": "maestro-product", "projection": "markdown_invocation_namespace", "hash_scope": "distributed_plugin_bytes", "files": projectedHashes}, "", "  ")
	if err != nil {
		return err
	}
	methods.Assets["methods-projection.json"] = append(projection, '\n')
	var names []string
	for name := range methods.Assets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(o.Destination, name), methods.Assets[name], 0600); err != nil {
			return err
		}
	}
	return nil
}

// Claude resolves plugin methods by qualified name; canonical IDs and metadata
// remain shared with ZIP and other runtimes.
var skillToolInvocations = regexp.MustCompile("(?i)\\bSkill tool (?:with|twice, for) [\"`][a-z0-9-]+[\"`](?: and [\"`][a-z0-9-]+[\"`])*")

func namespaceMethodReferences(body []byte, namespace string, reference *regexp.Regexp) []byte {
	frontmatter := []byte{}
	if bytes.HasPrefix(body, []byte("---\n")) {
		if end := bytes.Index(body[4:], []byte("\n---\n")); end >= 0 {
			end += 9
			frontmatter, body = body[:end], body[end:]
		}
	}
	var projected bytes.Buffer
	start := 0
	skillCalls := skillToolInvocations.FindAllIndex(body, -1)
	for _, span := range reference.FindAllIndex(body, -1) {
		lo, hi := span[0], span[1]
		invocation := body[lo] == '/' || body[lo] == '$'
		// Quoted IDs are Skill arguments only within an explicit Skill-tool call.
		for _, call := range skillCalls {
			if lo >= call[0] && hi <= call[1] {
				invocation = true
				break
			}
		}
		if !invocation {
			continue
		}
		// Resource paths and URLs are file references, not plugin invocations.
		pathSuffix := hi < len(body) && strings.ContainsRune("/-", rune(body[hi]))
		fileExtension := hi+1 < len(body) && body[hi] == '.' && unicode.IsLetter(rune(body[hi+1]))
		pathPrefix := body[lo] == '/' && lo > 0 && strings.ContainsRune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_./:", rune(body[lo-1]))
		if pathSuffix || fileExtension || pathPrefix {
			continue
		}
		projected.Write(body[start:lo])
		prefix := body[lo]
		if prefix == '$' {
			prefix = '/'
		}
		projected.WriteByte(prefix)
		projected.WriteString(namespace + ":")
		projected.Write(body[lo+1 : hi])
		start = hi
	}
	projected.Write(body[start:])
	return append(append([]byte{}, frontmatter...), projected.Bytes()...)
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
	if writeErr == nil {
		registry, err := packagedAuthority(o)
		writeErr = err
		if writeErr == nil && len(registry) > 0 {
			writeErr = addEntry(archive, "managed/trust/release-authority-registry.json", 0600, bytes.NewReader(registry))
		}
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

// Packaging carries only the independently pinned public registry, never runtime or workspace state.
func packagedAuthority(o Options) ([]byte, error) {
	if o.Binding.AuthorityRegistrySHA256 == "" {
		return nil, nil
	}
	if _, err := releaseverify.LoadPinnedAuthorityRegistry(o.Binding.AuthorityRegistryPath, o.Binding.AuthorityRegistrySHA256, time.Now); err != nil {
		return nil, err
	}
	f, err := scheduler.OpenRegularFile(o.Binding.AuthorityRegistryPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	body, err := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 1<<20 || fmt.Sprintf("%x", sha256.Sum256(body)) != o.Binding.AuthorityRegistrySHA256 {
		return nil, errors.New("public authority changed")
	}
	return body, nil
}
