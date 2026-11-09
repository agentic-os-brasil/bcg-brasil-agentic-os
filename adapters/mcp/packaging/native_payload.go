package packaging

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"regexp"
	"sort"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

const nativeManifestName = "maestro-packet.json"

type packetResource struct {
	URI    string `json:"uri"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

// Native diagnostics cannot be decoded into process-local NativeHost authority.
type nativePacketManifest struct {
	SchemaVersion     int              `json:"schema_version"`
	Mode              string           `json:"mode"`
	Target            Target           `json:"target"`
	Native            NativeCapability `json:"native"`
	BindingSHA256     string           `json:"binding_sha256,omitempty"`
	HooksSHA256       string           `json:"hooks_sha256,omitempty"`
	Resources         []packetResource `json:"embedded_resources"`
	References        []assetReference `json:"references"`
	CanonicalAgentIDs []string         `json:"canonical_agent_ids"`
	ExecutionBoundary string           `json:"execution_boundary"`
}

func nativeManifest(target Target, c NativeCapability) (nativePacketManifest, error) {
	p := nativePacketManifest{SchemaVersion: 1, Mode: "mcp_only", Target: target, Native: c, ExecutionBoundary: "Hashes and capability labels describe construction evidence, not publisher identity or host authority. Canonical methods and six projected specialists remain instructions. Only admitted MCP operations and separately observed target-local native hooks execute."}
	if c.State == "qualified" {
		p.Mode = "native"
	}
	if c.EvidenceClass == "engineering_fixture" {
		p.Mode = "native_engineering_fixture"
	}
	if c.State == "qualification_candidate" {
		p.Mode = "qualification_candidate"
	}
	catalog, err := baseagents.Catalog()
	if err != nil {
		return p, err
	}
	for _, a := range catalog.Agents {
		p.CanonicalAgentIDs = append(p.CanonicalAgentIDs, a.ID)
	}
	sort.Strings(p.CanonicalAgentIDs)
	methods, err := product.PackagedMethods()
	if err != nil {
		return p, err
	}
	for _, m := range methods.Methods {
		for _, r := range m.Resources {
			p.Resources = append(p.Resources, packetResource{r.URI, r.SHA256, r.Bytes})
		}
	}
	for name, body := range methods.Assets {
		uri := "maestro://methods/" + name
		if name == "methods-index.json" {
			uri = "maestro://methods/catalog.json"
		}
		p.Resources = append(p.Resources, packetResource{uri, digest(body), len(body)})
	}
	agents, err := baseagents.Files()
	if err != nil {
		return p, err
	}
	for name, body := range agents {
		p.Resources = append(p.Resources, packetResource{"maestro://agents/" + name, digest(body), len(body)})
	}
	sort.Slice(p.Resources, func(i, j int) bool { return p.Resources[i].URI < p.Resources[j].URI })
	p.References = methodAssetReferences(methods, agents)
	return p, nil
}

// Keep the normal portable selection protocol; admission is bound to equality
// with one local owner/workspace, while managed packet relocation stays possible.
func nativeHookArguments(binding, event string) []string {
	return append([]string{"hook", "--event", event, "--binding-sha256", binding}, portableArgs("${CLAUDE_PLUGIN_ROOT}", "serve")[1:]...)
}
func nativeProfilePayload(platform, binding string) (map[string][]byte, error) {
	hooks := map[string]any{}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStart", "SubagentStop", "Stop"} {
		command := map[string]any{"type": "command", "command": "${CLAUDE_PLUGIN_ROOT}/scripts/" + binaryName(platform), "args": nativeHookArguments(binding, event), "timeout": 5}
		if event == "PreToolUse" || event == "UserPromptSubmit" {
			command["onFailure"] = "block"
		}
		hooks[event] = []any{map[string]any{"hooks": []any{command}}}
	}
	body, err := json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
	return map[string][]byte{"hooks/hooks.json": append(body, '\n')}, err
}

func nativeSelectionFingerprint(o product.Options) (string, error) {
	if _, err := product.Open(o); err != nil {
		return "", err
	}
	h, err := datahome.Resolve(datahome.Options{ProductRoot: o.ProductRoot, UserHome: o.UserHome, LookupEnv: o.LookupEnv})
	if err != nil {
		return "", err
	}
	w, err := workspace.Inspect(o.WorkspacePath, filepath.Join(h.Home, "state"))
	if err != nil {
		return "", err
	}
	body, err := json.Marshal([]string{o.UserHome, o.WorkspacePath, h.Home, w.WorkspaceID})
	return digest(body), err
}

// ValidateNativeSelection is equality checking only. It cannot mint admission.
func ValidateNativeSelection(o product.Options, binding string) error {
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(binding) {
		return errors.New("native selection digest unavailable")
	}
	fresh, err := nativeSelectionFingerprint(o)
	if err != nil || fresh != binding {
		return errors.New("native selected target changed")
	}
	return nil
}

func nativePacketPayload(o Options, format string) (map[string][]byte, error) {
	c := nativeCapability(o)
	if o.native != nil {
		c = *o.native
	}
	if format == "mcpb" {
		c.State = "unavailable"
		c.Reason = "desktop_extension_carries_mcp_resources_only"
		c.EvidenceClass = "not_observed"
		c.HostVersion = ""
	}
	p, err := nativeManifest(o.target, c)
	if err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	if c.State == "qualified" || c.State == "qualification_candidate" {
		p.BindingSHA256, err = nativeSelectionFingerprint(o.Binding)
		if err != nil {
			return nil, err
		}
		files, err = nativeProfilePayload(o.target.Platform(), p.BindingSHA256)
		if err != nil {
			return nil, err
		}
		if o.hookOverride != nil {
			files["hooks/hooks.json"] = o.hookOverride
		}
		p.HooksSHA256 = digest(files["hooks/hooks.json"])
	}
	body, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	files[nativeManifestName] = append(body, '\n')
	return files, nil
}

func buildNativeCandidate(o Options, c NativeCapability, hooks []byte) error {
	o.native = &c
	o.hookOverride = hooks
	return buildPortable(o, "plugin")
}

// Preserve the original closed inventory and bind the additional native bytes.
// Candidate instrumentation is local engineering input, never native authority.
func nativeRequiredFiles(root, format string, target Target) (map[string][]byte, error) {
	f, err := scheduler.OpenRegularFile(filepath.Join(root, nativeManifestName))
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(io.LimitReader(f, (1<<20)+1))
	closeErr := f.Close()
	if readErr != nil || closeErr != nil {
		return nil, errors.Join(readErr, closeErr)
	}
	if len(body) > 1<<20 || packetJSON(body) != nil {
		return nil, errors.New("native diagnostic bound or schema")
	}
	d := json.NewDecoder(bytes.NewReader(body))
	d.DisallowUnknownFields()
	var p nativePacketManifest
	if d.Decode(&p) != nil {
		return nil, errors.New("native diagnostic unavailable")
	}
	if p.Target != target {
		return nil, errors.New("native diagnostic target changed")
	}
	if p.Native.Profile != nativeProfile || p.Native.VendorProvenance == "" {
		return nil, errors.New("native diagnostic profile changed")
	}
	if p.Native.State != "unavailable" && p.Native.State != "qualified" && p.Native.State != "qualification_candidate" {
		return nil, errors.New("native diagnostic state changed")
	}
	if format == "mcpb" && p.Native.State != "unavailable" {
		return nil, errors.New("Desktop has no native hooks")
	}
	expected, err := nativeManifest(p.Target, p.Native)
	if err != nil {
		return nil, err
	}
	expected.BindingSHA256 = p.BindingSHA256
	expected.HooksSHA256 = p.HooksSHA256
	want, err := json.MarshalIndent(expected, "", "  ")
	if err != nil || !bytes.Equal(body, append(want, '\n')) {
		return nil, errors.New("native diagnostic canonical resources changed")
	}
	files := map[string][]byte{nativeManifestName: body}
	if p.Native.State == "unavailable" {
		if p.BindingSHA256 != "" || p.HooksSHA256 != "" {
			return nil, errors.New("unavailable native diagnostic contains binding")
		}
		return files, nil
	}
	if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(p.BindingSHA256) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(p.HooksSHA256) {
		return nil, errors.New("native diagnostic binding unavailable")
	}
	if p.Native.State == "qualified" {
		if p.Native.EvidenceClass != "native_observed" && p.Native.EvidenceClass != "engineering_fixture" {
			return nil, errors.New("native diagnostic evidence unavailable")
		}
		files, err = nativeProfilePayload(target.Platform(), p.BindingSHA256)
		if err != nil {
			return nil, err
		}
		if digest(files["hooks/hooks.json"]) != p.HooksSHA256 {
			return nil, errors.New("native hook profile changed")
		}
		files[nativeManifestName] = body
	} else {
		hook, err := scheduler.OpenRegularFile(filepath.Join(root, "hooks/hooks.json"))
		if err != nil {
			return nil, err
		}
		hooks, re := io.ReadAll(io.LimitReader(hook, (64<<10)+1))
		ce := hook.Close()
		if re != nil || ce != nil || len(hooks) > 64<<10 || packetJSON(hooks) != nil || digest(hooks) != p.HooksSHA256 {
			return nil, errors.New("candidate hook profile changed")
		}
		files["hooks/hooks.json"] = hooks
	}
	return files, nil
}
