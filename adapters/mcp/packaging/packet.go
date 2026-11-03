package packaging

import (
	"bytes"
	"crypto/sha256"
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
	"runtime"
	"sort"

	product "github.com/agentic-os-brasil/bcg-brasil-agentic-os/adapters/mcp"
	baseagents "github.com/agentic-os-brasil/bcg-brasil-agentic-os/bundles/base/agents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/claudeagents"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/workspace"
)

const EnforcementMinimumVersion = "2.1.295"
const nativeProfile = "maestro_planner"

type Target struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

func (t Target) Platform() string {
	if t.OS == "windows" {
		return "win32"
	}
	return t.OS
}

type NativeCapability struct {
	State            string `json:"state"`
	Reason           string `json:"reason,omitempty"`
	Profile          string `json:"profile"`
	EvidenceClass    string `json:"evidence_class"`
	HostVersion      string `json:"host_version,omitempty"`
	VendorProvenance string `json:"vendor_provenance"`
}

// NativeHost is process-local authority. Only the attended native producer can
// mint it; there is deliberately no receipt decoder or exported constructor.
type NativeHost struct {
	binding, serverSHA, hostPath, hostSHA, profileSHA, version string
	configurationDirectory, configurationSHA                   string
	instructionPaths                                           []string
	evidenceClass                                              string
	ready                                                      bool
}

func nativeCapability(o Options) NativeCapability {
	c := NativeCapability{State: "unavailable", Reason: "native_host_not_qualified", Profile: nativeProfile, EvidenceClass: "not_observed", VendorProvenance: "operator_selected_installation_not_independently_vendor_attested"}
	if h := o.NativeHost; h != nil {
		if h.validate(o) == nil {
			c.State = "qualified"
			c.Reason = ""
			c.EvidenceClass = h.evidenceClass
			c.HostVersion = h.version
		} else {
			c.Reason = "native_admission_stale_or_invalid"
		}
	}
	return c
}
func (h *NativeHost) validate(o Options) error {
	if h == nil || !h.ready || h.evidenceClass != "native_observed" && h.evidenceClass != "engineering_fixture" {
		return errors.New("native admission unavailable")
	}
	binding, err := bindingFingerprint(o.Binding)
	if err != nil || binding != h.binding {
		return errors.New("native binding changed")
	}
	body, target, err := readNativeBinary(o.Server)
	if err != nil || digest(body) != h.serverSHA || target.OS != runtime.GOOS || target.Arch != runtime.GOARCH {
		return errors.New("native server changed")
	}
	host, _, err := nativeHostIdentity(h.hostPath)
	if err != nil || host != h.hostSHA {
		return errors.New("native host changed")
	}
	profile, err := profileFingerprint(o)
	if err != nil || profile != h.profileSHA {
		return errors.New("native profile changed")
	}
	if h.configurationDirectory != "" {
		configuration, err := nativeUserConfiguration(h.configurationDirectory, h.instructionPaths...)
		if err != nil || configuration != h.configurationSHA {
			return errors.New("native user configuration changed")
		}
	} else if h.evidenceClass != "engineering_fixture" {
		return errors.New("native user configuration unavailable")
	}
	return nil
}

// Snapshot the bounded selected instruction closure, never credential files.
func nativeUserConfiguration(directory string, paths ...string) (string, error) {
	return instructionConfiguration(directory, paths...)
}

func digest(body []byte) string { return fmt.Sprintf("%x", sha256.Sum256(body)) }

func readNativeBinary(path string) ([]byte, Target, error) {
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return nil, Target{}, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > 128<<20 {
		return nil, Target{}, errors.New("bounded native executable required")
	}
	body, err := io.ReadAll(io.LimitReader(f, (128<<20)+1))
	if err != nil || len(body) > 128<<20 {
		return nil, Target{}, errors.New("native executable size")
	}
	r := bytes.NewReader(body)
	if m, err := macho.NewFile(r); err == nil && m.Type == macho.TypeExec {
		switch m.Cpu {
		case macho.CpuArm64:
			return body, Target{"darwin", "arm64"}, nil
		case macho.CpuAmd64:
			return body, Target{"darwin", "amd64"}, nil
		}
	}
	if p, err := pe.NewFile(r); err == nil && p.Characteristics&0x0002 != 0 && p.Characteristics&0x2000 == 0 {
		switch p.Machine {
		case pe.IMAGE_FILE_MACHINE_AMD64:
			return body, Target{"windows", "amd64"}, nil
		case pe.IMAGE_FILE_MACHINE_ARM64:
			return body, Target{"windows", "arm64"}, nil
		}
	}
	if e, err := elf.NewFile(r); err == nil && e.Class == elf.ELFCLASS64 && (e.Type == elf.ET_EXEC || e.Type == elf.ET_DYN) {
		switch e.Machine {
		case elf.EM_X86_64:
			return body, Target{"linux", "amd64"}, nil
		case elf.EM_AARCH64:
			return body, Target{"linux", "arm64"}, nil
		}
	}
	return nil, Target{}, errors.New("supported 64-bit native executable required")
}

func bindingFingerprint(o product.Options) (string, error) {
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
	body, err := json.Marshal([]any{o.ProductRoot, o.UserHome, o.WorkspacePath, h.Home, w.WorkspaceID, o.AuthorityRegistryPath, o.AuthorityRegistrySHA256, o.InstalledManifestSHA256, o.InstalledClientInputsSHA256})
	if err != nil {
		return "", err
	}
	return digest(body), nil
}

func hookArguments(o Options, event string) []string {
	return append([]string{"hook", "--event", event, "--product-root", o.Binding.ProductRoot, "--user-home", o.Binding.UserHome, "--workspace", o.Binding.WorkspacePath}, installedArgs(o.Binding)...)
}
func nativeProfileFiles(o Options) (map[string][]byte, error) {
	files := map[string][]byte{}
	agents, err := claudeagents.Project(claudeagents.PluginNamespace)
	if err != nil {
		return nil, err
	}
	for _, agent := range agents {
		files[agent.Path] = agent.Body
	}
	hooks := map[string]any{}
	for _, event := range []string{"SessionStart", "UserPromptSubmit", "PreToolUse", "PostToolUse", "SubagentStart", "SubagentStop", "Stop"} {
		command := map[string]any{"type": "command", "command": "${CLAUDE_PLUGIN_ROOT}/scripts/" + binaryName(o.target.Platform()), "args": hookArguments(o, event), "timeout": 5}
		if event == "PreToolUse" || event == "UserPromptSubmit" {
			command["onFailure"] = "block"
		}
		hooks[event] = []any{map[string]any{"hooks": []any{command}}}
	}
	body, err := json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
	if err != nil {
		return nil, err
	}
	files["hooks/hooks.json"] = append(body, '\n')
	return files, nil
}
func profileFingerprint(o Options) (string, error) {
	files, err := nativeProfileFiles(o)
	if err != nil {
		return "", err
	}
	configuration, err := targetConfigurationSnapshot(o.Binding.WorkspacePath)
	if err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]any{"generated": files, "target_configuration": configuration})
	if err != nil {
		return "", err
	}
	return digest(body), nil
}

// Bind launch-time project instruction alternatives and complete rule membership.
func targetConfigurationSnapshot(workspace string) (map[string]string, error) {
	s := newInstructionSources()
	if err := s.ancestry(workspace, true); err != nil {
		return nil, err
	}
	return s.files, nil
}

func writePacketFile(root, path string, body []byte, mode fs.FileMode) error {
	if !fs.ValidPath(path) {
		return errors.New("invalid packet path")
	}
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	f, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(body)
	return errors.Join(writeErr, f.Close())
}
func writeCanonicalAgents(root string) error {
	files, err := baseagents.Files()
	if err != nil {
		return err
	}
	for path, body := range files {
		if err := writePacketFile(root, "managed/agents/"+path, body, 0600); err != nil {
			return err
		}
	}
	return nil
}

type packetFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Mode   uint32 `json:"mode"`
}
type packetResource struct {
	URI    string `json:"uri"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}
type packetInventory struct {
	SchemaVersion                 int              `json:"schema_version"`
	Profile                       string           `json:"profile,omitempty"`
	Protocol                      string           `json:"protocol,omitempty"`
	CarrierID                     string           `json:"carrier_id,omitempty"`
	CarrierVersion                string           `json:"carrier_version,omitempty"`
	BootstrapServerSHA256         string           `json:"bootstrap_server_sha256,omitempty"`
	BootstrapEmbeddedInputsSHA256 string           `json:"bootstrap_embedded_inputs_sha256,omitempty"`
	Mode                          string           `json:"mode"`
	Target                        Target           `json:"target"`
	Native                        NativeCapability `json:"native"`
	Files                         []packetFile     `json:"files"`
	Resources                     []packetResource `json:"embedded_resources"`
	References                    []assetReference `json:"references"`
	CanonicalAgentIDs             []string         `json:"canonical_agent_ids"`
	ExecutionBoundary             string           `json:"execution_boundary"`
}

func inventory(o Options, c NativeCapability) (packetInventory, error) {
	p := packetInventory{SchemaVersion: 1, Mode: "mcp_only", Target: o.target, Native: c, ExecutionBoundary: "Methods and helper resources are inert instructions; only implemented admitted MCP operations and qualified native profile execute. Legacy ZIP commands and absent external assets remain unavailable."}
	if o.Carrier != nil {
		p.SchemaVersion = 2
		p.Profile = o.Carrier.Profile
		p.Protocol = o.Carrier.Protocol
		p.CarrierID = o.Carrier.ID
		p.CarrierVersion = o.Carrier.Version
		p.BootstrapServerSHA256 = digest(o.serverBody)
		p.BootstrapEmbeddedInputsSHA256 = o.stableIdentity.EmbeddedInputsSHA256
	}
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
	if o.Carrier != nil {
		if err := compareStableIdentity(p, o.stableIdentity); err != nil {
			return p, err
		}
	}
	p.References = methodAssetReferences(methods, agents)
	return p, nil
}
func writePluginInventory(o Options, c NativeCapability) error {
	p, err := inventory(o, c)
	if err != nil {
		return err
	}
	err = filepath.WalkDir(o.Destination, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !d.Type().IsRegular() {
			return errors.New("nonregular packet entry")
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(o.Destination, path)
		if err != nil {
			return err
		}
		p.Files = append(p.Files, packetFile{filepath.ToSlash(relative), digest(body), uint32(info.Mode().Perm())})
		return nil
	})
	if err != nil {
		return err
	}
	sort.Slice(p.Files, func(i, j int) bool { return p.Files[i].Path < p.Files[j].Path })
	body, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	return writePacketFile(o.Destination, "maestro-packet.json", append(body, '\n'), 0600)
}
func extensionInventory(o Options, manifest []byte, path string) ([]byte, error) {
	c := nativeCapability(o)
	c.State = "unavailable"
	c.Reason = "desktop_extension_carries_mcp_resources_only"
	p, err := inventory(o, c)
	if err != nil {
		return nil, err
	}
	p.Files = []packetFile{{"manifest.json", digest(append(manifest, '\n')), 0600}, {path, digest(o.serverBody), 0700}}
	registry, err := packagedAuthority(o)
	if err != nil {
		return nil, err
	}
	if len(registry) > 0 {
		p.Files = append(p.Files, packetFile{"managed/trust/release-authority-registry.json", digest(registry), 0600})
	}
	if o.Carrier != nil {
		sort.Slice(p.Files, func(i, j int) bool { return p.Files[i].Path < p.Files[j].Path })
	}
	body, err := json.MarshalIndent(p, "", "  ")
	return append(body, '\n'), err
}
