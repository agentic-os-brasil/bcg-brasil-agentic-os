// Package packaging builds local participant packets without copying external state.
package packaging

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
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
	NativeHost                   *NativeHost
	Carrier                      *StableCarrier
	serverBody                   []byte
	stableIdentity               releasecontract.ServerIdentity
	target                       Target
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
	o.serverBody, o.target, err = readNativeBinary(o.Server)
	if err != nil {
		return o, "", err
	}
	reader := installtx.ReadStateForManagedRoot
	if o.Carrier != nil {
		reader = installtx.ReadSourceState
	}
	state, stateErr := reader(filepath.Join(o.Binding.UserHome, ".maestro-install"), o.Binding.ProductRoot)
	if stateErr == nil && state.SchemaVersion == 4 {
		if digest(o.serverBody) != state.Server.SHA256 || o.Version != state.Release {
			return o, "", errors.New("packet executable differs from active signed release")
		}
		o.Binding.InstalledManifestSHA256 = state.Binding.ManifestSHA256
		o.Binding.InstalledClientInputsSHA256 = state.Binding.ClientInputsSHA256
	} else if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return o, "", stateErr
	}
	if o.Carrier != nil {
		if err = prepareStable(&o); err != nil {
			return o, "", err
		}
	}
	return o, o.target.Platform(), err
}
func args(o Options) []string {
	if o.Carrier != nil {
		return []string{"stable-launch", "--product-root", o.Binding.ProductRoot, "--user-home", o.Binding.UserHome, "--workspace", o.Binding.WorkspacePath, "--carrier-id", o.Carrier.ID}
	}
	command := "serve"
	if o.Binding.InstalledManifestSHA256 != "" {
		command = "launch"
	}
	return append([]string{command, "--product-root", o.Binding.ProductRoot, "--user-home", o.Binding.UserHome, "--workspace", o.Binding.WorkspacePath}, installedArgs(o.Binding)...)
}
func installedArgs(binding product.Options) []string {
	if binding.InstalledManifestSHA256 == "" && binding.InstalledClientInputsSHA256 == "" {
		return nil
	}
	return []string{"--installed-manifest", binding.InstalledManifestSHA256, "--installed-client-inputs", binding.InstalledClientInputsSHA256}
}
func launchCommand(o Options, fallback string) string {
	if o.Binding.InstalledManifestSHA256 == "" {
		return fallback
	}
	name := "maestro-install"
	if o.target.OS == "windows" {
		name += ".exe"
	}
	return filepath.Join(o.Binding.ProductRoot, name)
}
func BuildPlugin(options Options) error {
	o, platform, err := prepare(options)
	if err != nil {
		return err
	}
	capability := nativeCapability(o)
	return buildPlugin(o, platform, capability)
}

func buildPlugin(o Options, platform string, capability NativeCapability) error {
	return buildPluginWithHooks(o, platform, capability, nil)
}

// Qualification changes only its candidate hook config. Inventory is computed
// after the exact final candidate bytes, never before a post-build mutation.
func buildPluginWithHooks(o Options, platform string, capability NativeCapability, hookOverride []byte) error {
	var err error
	if err = os.Mkdir(o.Destination, 0700); err != nil {
		return err
	}
	for _, dir := range []string{".claude-plugin", "scripts", "skills/operator", "skills/resume"} {
		if err = os.MkdirAll(filepath.Join(o.Destination, dir), 0700); err != nil {
			return err
		}
	}
	name := binaryName(platform)
	if err = writePacketFile(o.Destination, "scripts/"+name, o.serverBody, 0700); err != nil {
		return err
	}
	for path, value := range map[string]any{
		".claude-plugin/plugin.json": map[string]any{"name": "maestro-product", "version": o.Version, "description": "Local participant execution workflow", "author": map[string]string{"name": "Agentic OS Brasil"}},
		".mcp.json":                  map[string]any{"mcpServers": map[string]any{"maestro": map[string]any{"type": "stdio", "command": launchCommand(o, "${CLAUDE_PLUGIN_ROOT}/scripts/"+name), "args": args(o)}}},
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
	if err = writeMethods(o); err != nil {
		return err
	}
	if err = writeCanonicalAgents(o.Destination); err != nil {
		return err
	}
	if capability.State == "qualified" || capability.State == "qualification_candidate" {
		files, err := nativeProfileFiles(o)
		if err != nil {
			return err
		}
		if hookOverride != nil {
			files["hooks/hooks.json"] = hookOverride
		}
		for path, body := range files {
			if err := writePacketFile(o.Destination, path, body, 0600); err != nil {
				return err
			}
		}
	}
	return writePluginInventory(o, capability)
}

const operator = `---
name: operator
description: Use reviewed owner and workspace memory workflows or continue an execution contract in the bound Maestro workspace.
---
Read maestro_context for admitted binding metadata, then inspect the current maestro_query and maestro_apply schemas for implemented operations. Treat unavailable binding as a stop: ask the owner to inspect the locally selected paths. Startup never initializes state.
For owner onboarding or refinement, query owner.status and owner.interview for the selected quick or complete track. Use owner.track, owner.propose, owner.apply and owner.confirm_onboarding as applicable. owner.review and owner.onboarding_review return metadata; complete private text is presented through trusted review before application or final confirmation. Model assertions and metadata never count as approval.
Capture only text the user explicitly selects for memory, using memory.capture and exact trusted review; never auto-capture conversations. Discover IDs and visibility through memory.list or memory.search, then use memory.read or memory.context for eligible bounded pages. Use memory.correct or memory.delete with the current capture revision; memory.retention requires an explicit cutoff and current visibility revision. memory.consolidate runs local deterministic daily or weekly synthesis for an explicit date. Lifetime promotion requires canonical generation evidence and trusted review of the exact candidate.
If trusted host review is unavailable, ask the local operator to use owner-review/owner-apply or owner-onboarding-review/owner-confirm-onboarding; local owner-track requires explicit --confirm. For memory, use memory-review followed by memory-apply with the same JSON request on stdin, its exact --review-digest and explicit --confirm. These are operator fallbacks, not permission for the model to run arbitrary commands or supply approval.
For local documents, query content.status, use content.convert with a workspace-relative source_path, then content.read for bounded markdown/document slices. Review knowledge.create or knowledge.append against the exact artifact, page, text and expected_page_revision; use knowledge.collect for named page readback. When trusted review is unavailable, the local operator uses knowledge-review then knowledge-apply with the same JSON stdin, exact --review-digest and explicit --confirm. Conversion never installs a runtime or writes knowledge automatically.
For execution, query execution.list for metadata and execution.next with the chosen item ID for bounded continuation. Create only after the owner requests work: objective, next_step and workspace-relative artifact paths define the immutable artifact snapshot contract. Read execution.contract for the bounded criterion IDs and relative paths used to collect evidence.
Apply execution changes with the current expected_revision. Start creates an attempt; checkpoint, pause, evidence and complete require that attempt_id. Save a concise checkpoint before pause. On revision_conflict or attempt_conflict, read current continuation and reconcile. Collect artifact evidence after writing the requested result; complete revalidates it. Changed artifacts require new evidence.
Scoped memory deletion and retention use their reviewed operations. Arbitrary file deletion, full export, arbitrary commands, model execution and external sends remain ungranted. Keep owner confirmation and host permissions effective. Plugin methods are recipes; unavailable domains remain unavailable, and native agent scheduling and Desktop Skills installation remain unqualified.
`
const resume = `---
name: resume
description: Resume paused Maestro execution in another session or client.
---
Read maestro_context, list executions and ask which item only when ambiguous. Read execution.next for the selected item. If paused, call execution.resume with the current expected_revision, then read execution.next again. Read execution.contract to discover every criterion ID and workspace-relative path. Continue using the returned new attempt_id and contract metadata; preserve the prior checkpoint and owner scope. A stale writer must reread and reconcile.
`

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
				body, err = claudeagents.ProjectAgentInvocations(body, claudeagents.PluginNamespace)
				if err != nil {
					return err
				}
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
	manifestName, manifestVersion := "maestro-product", o.Version
	if o.Carrier != nil {
		manifestName = "maestro-connector-" + o.Carrier.ID
		manifestVersion = o.Carrier.Version
	}
	manifest, err := json.MarshalIndent(map[string]any{
		"manifest_version": "0.3", "name": manifestName, "version": manifestVersion,
		"description":   "Local participant Maestro execution workflow",
		"author":        map[string]string{"name": "Agentic OS Brasil"},
		"server":        map[string]any{"type": "binary", "entry_point": path, "mcp_config": map[string]any{"command": launchCommand(o, "${__dirname}/"+path), "args": args(o)}},
		"compatibility": map[string]any{"platforms": []string{platform}},
	}, "", "  ")
	if err != nil {
		return err
	}
	artifact, err := os.OpenFile(o.Destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("destination must be new; nothing overwritten: %w", err)
	}
	archive := zip.NewWriter(artifact)
	writeErr := addEntry(archive, "manifest.json", 0600, bytes.NewReader(append(manifest, '\n')))
	if writeErr == nil {
		writeErr = addEntry(archive, path, 0700, bytes.NewReader(o.serverBody))
	}
	if writeErr == nil {
		registry, err := packagedAuthority(o)
		writeErr = err
		if writeErr == nil && len(registry) > 0 {
			writeErr = addEntry(archive, "managed/trust/release-authority-registry.json", 0600, bytes.NewReader(registry))
		}
	}
	if writeErr == nil {
		inventory, err := extensionInventory(o, manifest, path)
		writeErr = err
		if err == nil {
			writeErr = addEntry(archive, "maestro-packet.json", 0600, bytes.NewReader(inventory))
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
