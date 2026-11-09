package packaging

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"time"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
)

// StableCarrier identifies one locally reviewed client slot, not a release or
// launcher authority. The private installed source/client record owns launch.
type StableCarrier struct {
	Profile  string `json:"profile"`
	Protocol string `json:"protocol"`
	ID       string `json:"carrier_id"`
	Version  string `json:"carrier_version"`
}

func (c StableCarrier) validate() error {
	if c.Profile != installer.StableClientProfile || c.Protocol != installer.StableClientProtocol || c.Version != installer.StableCarrierVersion || !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(c.ID) {
		return errors.New("unsupported stable carrier")
	}
	return nil
}

// This is a closed variant of packet-inventory.json. Its local paths and hashes
// reconstruct exact canonical launch metadata; none can mint source admission.
type stablePacketBinding struct {
	Carrier                       StableCarrier `json:"carrier"`
	ManagedRoot                   string        `json:"managed_root"`
	UserHome                      string        `json:"user_home"`
	Workspace                     string        `json:"workspace"`
	BootstrapServerSHA256         string        `json:"bootstrap_server_sha256"`
	BootstrapEmbeddedInputsSHA256 string        `json:"bootstrap_embedded_inputs_sha256"`
}

func (s stablePacketBinding) validate() error {
	if err := s.Carrier.validate(); err != nil {
		return err
	}
	for _, p := range []string{s.ManagedRoot, s.UserHome, s.Workspace} {
		if !filepath.IsAbs(p) || filepath.Clean(p) != p {
			return errors.New("stable packet requires exact absolute local selection")
		}
	}
	hash := regexp.MustCompile(`^[0-9a-f]{64}$`)
	if !hash.MatchString(s.BootstrapServerSHA256) || !hash.MatchString(s.BootstrapEmbeddedInputsSHA256) {
		return errors.New("stable bootstrap identity missing")
	}
	return nil
}
func packetSchema(s *stablePacketBinding) int {
	if s != nil {
		return 2
	}
	return 1
}
func stableBinding(o Options) *stablePacketBinding {
	if o.Carrier == nil {
		return nil
	}
	return &stablePacketBinding{Carrier: *o.Carrier, ManagedRoot: o.Binding.ProductRoot, UserHome: o.Binding.UserHome, Workspace: o.Binding.WorkspacePath, BootstrapServerSHA256: digest(o.serverBody), BootstrapEmbeddedInputsSHA256: o.stableIdentity.EmbeddedInputsSHA256}
}
func prepareStable(o *Options) error {
	if err := o.Carrier.validate(); err != nil {
		return err
	}
	if o.NativeHost != nil {
		return errors.New("stable client carrier uses the MCP-only profile")
	}
	s, err := installtx.ValidateQualifiedLaunch(o.Binding.ProductRoot, filepath.Join(o.Binding.UserHome, ".maestro-install"), installtx.ReleaseBinding{ManifestSHA256: o.Binding.InstalledManifestSHA256, ClientInputsSHA256: o.Binding.InstalledClientInputsSHA256}, time.Now())
	if err != nil {
		return err
	}
	if s.SchemaVersion != 4 || s.Source == nil || s.Server.ExecutionContract != releasecontract.SyncedSourceContract || s.Release != o.Version || s.Server.SHA256 != digest(o.serverBody) {
		return errors.New("stable carrier requires the actual current source-aware installation")
	}
	if o.Server != filepath.Join(o.Binding.ProductRoot, "bin", binaryName(o.target.Platform())) {
		return errors.New("stable carrier requires the actual installed current server")
	}
	i, err := releasecontract.ReadServerIdentity(o.Server)
	if err != nil {
		return err
	}
	if i.EmbeddedInputsSHA256 != s.Server.EmbeddedInputsSHA256 || i.Version != s.Server.Version || i.AuthorityRegistrySHA256 != o.Binding.AuthorityRegistrySHA256 {
		return errors.New("actual current stable identity differs")
	}
	o.stableIdentity = i
	return stableBinding(*o).validate()
}
func stableMetadata(files map[string][]byte, format, platform string, s *stablePacketBinding) error {
	if err := s.validate(); err != nil {
		return err
	}
	launcher := "maestro-install"
	if platform == "win32" {
		launcher += ".exe"
	}
	config := map[string]any{"command": filepath.Join(s.ManagedRoot, launcher), "args": []string{"stable-launch", "--product-root", s.ManagedRoot, "--user-home", s.UserHome, "--workspace", s.Workspace, "--carrier-id", s.Carrier.ID}}
	if format == "plugin" {
		config["type"] = "stdio"
		body, err := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{"maestro": config}}, "", "  ")
		if err != nil {
			return err
		}
		files[".mcp.json"] = append(body, '\n')
		var manifest map[string]any
		if err := json.Unmarshal(files[".claude-plugin/plugin.json"], &manifest); err != nil {
			return err
		}
		delete(manifest, "userConfig")
		body, err = json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return err
		}
		files[".claude-plugin/plugin.json"] = append(body, '\n')
	} else {
		var manifest map[string]any
		if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
			return err
		}
		manifest["name"] = "maestro-connector-" + s.Carrier.ID
		manifest["version"] = s.Carrier.Version
		delete(manifest, "user_config")
		server, ok := manifest["server"].(map[string]any)
		if !ok {
			return errors.New("stable native server metadata missing")
		}
		server["mcp_config"] = config
		body, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return err
		}
		files["manifest.json"] = append(body, '\n')
	}
	files["README-PACKET.md"] = []byte("# Participant-local Maestro carrier\n\nThis packet belongs to one reviewed local client slot. Its fixed installed launcher revalidates the independently admitted source, current materialization and selected workspace before starting the active server. Copied metadata grants no authority. Use the exact management launcher for review/reconciliation; no native loading or production signing is implied.\n")
	return nil
}
func packetEmbeddedDigest(resources []packetResource) (string, error) {
	entries := make([]releasecontract.EmbeddedResource, 0, len(resources))
	for _, r := range resources {
		entries = append(entries, releasecontract.EmbeddedResource{URI: r.URI, Bytes: int64(r.Bytes), SHA256: r.SHA256})
	}
	return releasecontract.EmbeddedDigest(entries)
}

// InspectNativeExecutable observes bytes/target and grants no NativeHost.
func InspectNativeExecutable(path string) (string, Target, error) { return nativeHostIdentity(path) }
func inspectStableStatus(status PacketStatus, c StableCarrier, i releasecontract.ServerIdentity, serverSHA string) error {
	if err := c.validate(); err != nil {
		return err
	}
	if err := i.Validate(); err != nil {
		return err
	}
	if status.Stable == nil || *status.Stable != c || status.NativeMode != "mcp_only" || status.ServerSHA256 != serverSHA || status.EmbeddedInputsSHA256 != i.EmbeddedInputsSHA256 || status.Version != i.Version || status.RegistrySHA256 != i.AuthorityRegistrySHA256 {
		return errors.New("current stable packet identity differs")
	}
	return nil
}
func InspectStablePlugin(root string, c StableCarrier, i releasecontract.ServerIdentity, serverSHA string) error {
	status, err := ValidatePacket(root)
	if err != nil {
		return err
	}
	return inspectStableStatus(status, c, i, serverSHA)
}
func InspectStableExtension(path string, c StableCarrier, i releasecontract.ServerIdentity, serverSHA string) (string, error) {
	status, err := ValidateArchive(path)
	if err != nil {
		return "", err
	}
	if err = inspectStableStatus(status, c, i, serverSHA); err != nil {
		return "", err
	}
	return status.NativeContractSHA256, nil
}

// InspectReusedStableExtension recognizes only the exact archive previously
// admitted into the private client record. Its old canonical assets must not be
// reinterpreted using this installer's newer or older linked method bundle. No
// caller-supplied digest is authority: the owning caller reads all expected values
// from the bound private record before invoking this observer.
func InspectReusedStableExtension(path string, c StableCarrier, packageSHA, serverSHA, embeddedSHA, nativeSHA string) error {
	if err := c.validate(); err != nil {
		return err
	}
	hash := regexp.MustCompile(`^[0-9a-f]{64}$`)
	for _, value := range []string{packageSHA, serverSHA, embeddedSHA, nativeSHA} {
		if !hash.MatchString(value) {
			return errors.New("recorded stable packet identity missing")
		}
	}
	actual, err := installer.ClientArchiveDigest(path)
	if err != nil {
		return err
	}
	if actual != packageSHA {
		return errors.New("previously admitted stable archive changed; preserved")
	}
	return nil
}

// VerifyStablePacket uses the sole canonical inventory. A caller from an older
// installer must invoke this through the actual authenticated current generator,
// whose linked canonical assets match the new installed release.
func VerifyStablePacket(path, format string, c StableCarrier, i releasecontract.ServerIdentity, serverSHA string) (PacketStatus, error) {
	var status PacketStatus
	var err error
	switch format {
	case "plugin":
		status, err = ValidatePacket(path)
	case "mcpb":
		status, err = ValidateArchive(path)
	default:
		return status, errors.New("exact stable packet format required")
	}
	if err != nil {
		return PacketStatus{}, err
	}
	if err = inspectStableStatus(status, c, i, serverSHA); err != nil {
		return PacketStatus{}, err
	}
	return status, nil
}
