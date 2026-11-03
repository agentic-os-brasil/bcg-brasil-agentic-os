package packaging

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installer"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/installtx"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/scheduler"
	"io"
	"io/fs"
	"path/filepath"
	"regexp"
	"time"
)

// StableCarrier fixes native bootstrap identity independently of server release.
type StableCarrier struct {
	Profile  string `json:"profile"`
	Protocol string `json:"protocol"`
	ID       string `json:"carrier_id"`
	Version  string `json:"carrier_version"`
}

func (c StableCarrier) validate() error {
	if c.Profile != installer.StableClientProfile || c.Protocol != installer.StableClientProtocol || c.Version != installer.StableCarrierVersion || !regexp.MustCompile("^[0-9a-f]{32}$").MatchString(c.ID) {
		return errors.New("unsupported stable carrier")
	}
	return nil
}
func prepareStable(o *Options) error {
	if err := o.Carrier.validate(); err != nil {
		return err
	}
	if o.NativeHost != nil {
		return errors.New("stable carrier is MCP-only")
	}
	s, err := installtx.ValidateQualifiedLaunch(o.Binding.ProductRoot, filepath.Join(o.Binding.UserHome, ".maestro-install"), installtx.ReleaseBinding{ManifestSHA256: o.Binding.InstalledManifestSHA256, ClientInputsSHA256: o.Binding.InstalledClientInputsSHA256}, time.Now())
	if err != nil {
		return err
	}
	if s.SchemaVersion != 4 || s.Source == nil || s.Server.ExecutionContract != releasecontract.SyncedSourceContract || s.Release != o.Version || s.Server.SHA256 != digest(o.serverBody) {
		return errors.New("stable carrier requires actual current source-aware installation")
	}
	if o.Server != filepath.Join(o.Binding.ProductRoot, "bin", binaryName(o.target.Platform())) {
		return errors.New("stable packet requires installed current server")
	}
	i, err := releasecontract.ReadServerIdentity(o.Server)
	if err != nil {
		return err
	}
	if i.EmbeddedInputsSHA256 != s.Server.EmbeddedInputsSHA256 || i.Version != s.Server.Version || i.AuthorityRegistrySHA256 != o.Binding.AuthorityRegistrySHA256 {
		return errors.New("actual current stable identity differs")
	}
	o.stableIdentity = i
	return nil
}

// InspectNativeExecutable observes bytes/target and grants no NativeHost.
func InspectNativeExecutable(path string) (string, Target, error) { return nativeHostIdentity(path) }
func validateStableInventory(p packetInventory, c StableCarrier) error {
	if err := c.validate(); err != nil {
		return err
	}
	if p.SchemaVersion != 2 || p.Profile != c.Profile || p.Protocol != c.Protocol || p.CarrierID != c.ID || p.CarrierVersion != c.Version || p.Mode != "mcp_only" || p.Native.State != "unavailable" {
		return errors.New("stable inventory profile differs")
	}
	hash := regexp.MustCompile("^[0-9a-f]{64}$")
	if !hash.MatchString(p.BootstrapServerSHA256) || !hash.MatchString(p.BootstrapEmbeddedInputsSHA256) {
		return errors.New("stable bootstrap provenance missing")
	}
	resources := make([]releasecontract.EmbeddedResource, 0, len(p.Resources))
	previous := ""
	for _, r := range p.Resources {
		if r.URI <= previous || r.Bytes <= 0 || !hash.MatchString(r.SHA256) {
			return errors.New("invalid stable resources")
		}
		previous = r.URI
		resources = append(resources, releasecontract.EmbeddedResource{URI: r.URI, Bytes: int64(r.Bytes), SHA256: r.SHA256})
	}
	h, err := releasecontract.EmbeddedDigest(resources)
	if err != nil || len(resources) == 0 || h != p.BootstrapEmbeddedInputsSHA256 {
		return errors.New("stable resource inventory differs")
	}
	return nil
}
func compareStableIdentity(p packetInventory, i releasecontract.ServerIdentity) error {
	if err := i.Validate(); err != nil {
		return err
	}
	if p.BootstrapEmbeddedInputsSHA256 != i.EmbeddedInputsSHA256 || len(p.Resources) != len(i.Resources) {
		return errors.New("packet resource identity differs")
	}
	for n, r := range p.Resources {
		e := i.Resources[n]
		if r.URI != e.URI || r.SHA256 != e.SHA256 || int64(r.Bytes) != e.Bytes {
			return errors.New("packet current resources differ")
		}
	}
	return nil
}

type inspectedFile struct {
	body []byte
	mode uint32
}

func inspectInventory(files map[string]inspectedFile, c StableCarrier) (packetInventory, error) {
	var p packetInventory
	f, ok := files["maestro-packet.json"]
	if !ok || len(f.body) > 1<<20 || f.mode != 0600 {
		return p, errors.New("bounded private packet inventory missing")
	}
	if err := releasecontract.DecodeStrictJSON(f.body, &p); err != nil {
		return p, err
	}
	if err := validateStableInventory(p, c); err != nil {
		return p, err
	}
	if len(p.Files) == 0 || len(p.Files) > 4096 || len(files) != len(p.Files)+1 {
		return p, errors.New("packet closure differs")
	}
	previous := ""
	for _, e := range p.Files {
		if !fs.ValidPath(e.Path) || e.Path <= previous || e.Path == "maestro-packet.json" {
			return p, errors.New("invalid or duplicate packet entry")
		}
		previous = e.Path
		f, ok := files[e.Path]
		if !ok || f.mode != e.Mode || digest(f.body) != e.SHA256 {
			return p, fmt.Errorf("packet file differs: %s", e.Path)
		}
	}
	server := "scripts/" + binaryName(p.Target.Platform())
	if _, ok := files["manifest.json"]; ok {
		server = "server/" + binaryName(p.Target.Platform())
	}
	f, ok = files[server]
	if !ok || f.mode != 0700 || digest(f.body) != p.BootstrapServerSHA256 {
		return p, errors.New("bootstrap server differs")
	}
	if err := validateStableConfig(files, c); err != nil {
		return p, err
	}
	return p, nil
}
func pluginFiles(root string) (map[string]inspectedFile, error) {
	if err := scheduler.ValidateDirectory(root); err != nil {
		return nil, err
	}
	out := map[string]inspectedFile{}
	var total int64
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return scheduler.ValidatePrivateDirectory(path)
		}
		if len(out) >= 4096 || !d.Type().IsRegular() {
			return errors.New("unsafe or excessive packet entries")
		}
		f, e := scheduler.OpenRegularFile(path)
		if e != nil {
			return e
		}
		info, e := f.Stat()
		if e == nil {
			e = stablePrivateFile(info)
		}
		if e != nil {
			f.Close()
			return e
		}
		body, e := io.ReadAll(io.LimitReader(f, 128<<20+1))
		f.Close()
		if e != nil || len(body) > 128<<20 {
			return errors.New("bounded packet entry unavailable")
		}
		total += int64(len(body))
		if total > 192<<20 {
			return errors.New("packet closure too large")
		}
		rel, e := filepath.Rel(root, path)
		if e != nil {
			return e
		}
		out[filepath.ToSlash(rel)] = inspectedFile{body, uint32(info.Mode().Perm())}
		return nil
	})
	return out, err
}
func extensionFiles(path string) (map[string]inspectedFile, []byte, error) {
	f, err := scheduler.OpenRegularFile(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, nil, err
	}
	if err = stablePrivateFile(info); err != nil {
		return nil, nil, err
	}
	body, err := io.ReadAll(io.LimitReader(f, 192<<20+1))
	if err != nil || len(body) > 192<<20 {
		return nil, nil, errors.New("bounded MCPB unavailable")
	}
	z, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, nil, err
	}
	if len(z.File) > 4097 {
		return nil, nil, errors.New("MCPB too many entries")
	}
	out := map[string]inspectedFile{}
	var total int
	for _, e := range z.File {
		if !fs.ValidPath(e.Name) || !e.Mode().IsRegular() {
			return nil, nil, errors.New("unsafe MCPB entry")
		}
		if _, ok := out[e.Name]; ok {
			return nil, nil, errors.New("duplicate MCPB entry")
		}
		r, err := e.Open()
		if err != nil {
			return nil, nil, err
		}
		data, err := io.ReadAll(io.LimitReader(r, 128<<20+1))
		r.Close()
		total += len(data)
		if err != nil || len(data) > 128<<20 || total > 192<<20 {
			return nil, nil, errors.New("MCPB expanded bound")
		}
		out[e.Name] = inspectedFile{data, uint32(e.Mode().Perm())}
	}
	return out, body, nil
}
func InspectStablePlugin(root string, c StableCarrier, i releasecontract.ServerIdentity, expectedServerSHA string) error {
	files, err := pluginFiles(root)
	if err != nil {
		return err
	}
	p, err := inspectInventory(files, c)
	if err != nil {
		return err
	}
	if p.BootstrapServerSHA256 != expectedServerSHA {
		return errors.New("current server bytes differ")
	}
	return compareStableIdentity(p, i)
}
func InspectStableExtension(path string, c StableCarrier, i releasecontract.ServerIdentity, expectedServerSHA string) (string, error) {
	files, _, err := extensionFiles(path)
	if err != nil {
		return "", err
	}
	p, err := inspectInventory(files, c)
	if err != nil {
		return "", err
	}
	if p.BootstrapServerSHA256 != expectedServerSHA {
		return "", errors.New("current server bytes differ")
	}
	if err = compareStableIdentity(p, i); err != nil {
		return "", err
	}
	manifest, ok := files["manifest.json"]
	if !ok {
		return "", errors.New("native manifest missing")
	}
	return digest(manifest.body), nil
}
func InspectReusedStableExtension(path string, c StableCarrier, packageSHA, serverSHA, embeddedSHA, nativeSHA string) error {
	files, body, err := extensionFiles(path)
	if err != nil {
		return err
	}
	if digest(body) != packageSHA {
		return errors.New("reused package bytes differ")
	}
	p, err := inspectInventory(files, c)
	if err != nil {
		return err
	}
	if p.BootstrapServerSHA256 != serverSHA || p.BootstrapEmbeddedInputsSHA256 != embeddedSHA || digest(files["manifest.json"].body) != nativeSHA {
		return errors.New("reused bootstrap/native contract differs")
	}
	return nil
}

type stableConfiguration struct {
	Type    string   `json:"type,omitempty"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

func validateStableConfig(files map[string]inspectedFile, c StableCarrier) error {
	var config stableConfiguration
	if manifest, desktop := files["manifest.json"]; desktop {
		var m map[string]json.RawMessage
		if err := releasecontract.DecodeStrictJSON(manifest.body, &m); err != nil {
			return err
		}
		var name, version string
		if json.Unmarshal(m["name"], &name) != nil || json.Unmarshal(m["version"], &version) != nil || name != "maestro-connector-"+c.ID || version != c.Version {
			return errors.New("stable native manifest identity differs")
		}
		var server struct {
			Type          string              `json:"type"`
			Entry         string              `json:"entry_point"`
			Configuration stableConfiguration `json:"mcp_config"`
		}
		if err := releasecontract.DecodeStrictJSON(m["server"], &server); err != nil {
			return err
		}
		if server.Type != "binary" || !fs.ValidPath(server.Entry) {
			return errors.New("stable native server differs")
		}
		if _, ok := files[server.Entry]; !ok {
			return errors.New("native entry point absent")
		}
		config = server.Configuration
	} else {
		f, ok := files[".mcp.json"]
		if !ok {
			return errors.New("stable MCP configuration missing")
		}
		var m struct {
			Servers map[string]stableConfiguration `json:"mcpServers"`
		}
		if err := releasecontract.DecodeStrictJSON(f.body, &m); err != nil {
			return err
		}
		if len(m.Servers) != 1 {
			return errors.New("stable server selection differs")
		}
		var found bool
		config, found = m.Servers["maestro"]
		if !found || config.Type != "stdio" {
			return errors.New("stable stdio configuration differs")
		}
	}
	a := config.Args
	if len(a) != 9 || a[0] != "stable-launch" || a[1] != "--product-root" || a[3] != "--user-home" || a[5] != "--workspace" || a[7] != "--carrier-id" || a[8] != c.ID {
		return errors.New("stable launch argv differs")
	}
	for _, path := range []string{a[2], a[4], a[6]} {
		if !filepath.IsAbs(path) || filepath.Clean(path) != path {
			return errors.New("stable absolute binding differs")
		}
	}
	expected := filepath.Join(a[2], "maestro-install")
	if filepath.Ext(config.Command) == ".exe" {
		expected += ".exe"
	}
	if config.Command != expected {
		return errors.New("stable installed launcher differs")
	}
	return nil
}
