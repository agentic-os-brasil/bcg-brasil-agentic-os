package releasecontract

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sort"
	"time"
)

const ModernProfile = "mcp-local-v1"
const ServerProtocol = "maestro-mcp-identity-v1"
const InstallerProtocol = "maestro-install-seed-v1"

type ServerComponent struct {
	Version              string `json:"version"`
	CompatibleBundle     string `json:"compatible_bundle"`
	Protocol             string `json:"protocol"`
	EmbeddedInputsSHA256 string `json:"embedded_inputs_sha256"`
}
type DeliveryComponent struct {
	Profile            string `json:"profile"`
	InstallerVersion   string `json:"installer_version"`
	InstallerProtocol  string `json:"installer_protocol"`
	ClientInputsSHA256 string `json:"client_inputs_sha256"`
}

func (m Manifest) Modern() bool { return m.SchemaVersion == 2 }
func (m Manifest) ExecutableVersion() string {
	if m.Modern() {
		return m.Server.Version
	}
	return m.CLI.Version
}
func (m Manifest) CompatibleExecutable() string {
	if m.Modern() {
		return m.Bundle.CompatibleServer
	}
	return m.Bundle.CompatibleCLI
}

// MarshalJSON preserves the v1 wire and emits only the authenticated variant.
func (m Manifest) MarshalJSON() ([]byte, error) {
	type wire Manifest
	body, err := json.Marshal(wire(m))
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	if m.Modern() {
		delete(fields, "cli")
	} else {
		delete(fields, "server")
		delete(fields, "delivery")
	}
	return json.Marshal(fields)
}

func validateWireFields(body []byte, version int) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return err
	}
	var bundle map[string]json.RawMessage
	if err := json.Unmarshal(fields["bundle"], &bundle); err != nil {
		return err
	}
	if version == 2 {
		if _, ok := fields["cli"]; ok {
			return errors.New("modern manifest cannot carry cli")
		}
		if _, ok := bundle["compatible_cli"]; ok {
			return errors.New("modern bundle cannot carry compatible_cli")
		}
	} else {
		for _, name := range []string{"server", "delivery"} {
			if _, ok := fields[name]; ok {
				return errors.New("legacy manifest cannot carry modern fields")
			}
		}
		if _, ok := bundle["compatible_server"]; ok {
			return errors.New("legacy bundle cannot carry compatible_server")
		}
	}
	var artifacts []map[string]json.RawMessage
	if err := json.Unmarshal(fields["artifacts"], &artifacts); err != nil {
		return err
	}
	for _, a := range artifacts {
		if _, ok := a["adapter"]; ok {
			var kind string
			_ = json.Unmarshal(a["kind"], &kind)
			if version != 2 || kind != "runtime_pack" {
				return errors.New("adapter is forbidden on this artifact")
			}
		}
	}
	return nil
}

func (m Manifest) validateComponents() error {
	if !m.Modern() {
		if m.Server != (ServerComponent{}) || m.Delivery != (DeliveryComponent{}) || m.Bundle.CompatibleServer != "" {
			return errors.New("legacy manifest has modern component metadata")
		}
		return validateComponents(m.CLI, m.Bundle)
	}
	if m.CLI != (CLIComponent{}) || m.Bundle.CompatibleCLI != "" || m.Delivery.Profile != ModernProfile || m.Server.Protocol != ServerProtocol || m.Delivery.InstallerProtocol != InstallerProtocol {
		return errors.New("modern release requires the closed MCP delivery profile")
	}
	if _, err := parseVersion(m.Delivery.InstallerVersion); err != nil {
		return fmt.Errorf("installer version: %w", err)
	}
	if !hashPattern.MatchString(m.Server.EmbeddedInputsSHA256) || !hashPattern.MatchString(m.Delivery.ClientInputsSHA256) {
		return errors.New("modern input identities are required")
	}
	return validateComponents(CLIComponent{m.Server.Version, m.Server.CompatibleBundle}, BundleComponent{Version: m.Bundle.Version, CompatibleCLI: m.Bundle.CompatibleServer})
}

// CanonicalJSON is UTF-8 JSON with sorted object keys and no whitespace. All
// digest vectors use JSON objects (maps), explicit required fields and sorted
// arrays. It does not normalize signed manifest bytes.
func CanonicalJSON(value any) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var object any
	if err = decoder.Decode(&object); err != nil {
		return nil, err
	}
	return json.Marshal(object)
}

func (m Manifest) ClientInputsDigest() (string, error) {
	artifacts := append([]Artifact(nil), m.Artifacts...)
	sort.Slice(artifacts, func(i, j int) bool {
		a, b := artifacts[i], artifacts[j]
		return a.Kind+"/"+a.OS+"/"+a.Arch+"/"+a.Name < b.Kind+"/"+b.OS+"/"+b.Arch+"/"+b.Name
	})
	body, err := CanonicalJSON(map[string]any{"profile": m.Delivery.Profile, "installer_protocol": m.Delivery.InstallerProtocol, "installer_version": m.Delivery.InstallerVersion, "server_protocol": m.Server.Protocol, "embedded_inputs_sha256": m.Server.EmbeddedInputsSHA256, "artifacts": artifacts})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(body)), nil
}

type EmbeddedResource struct {
	URI    string `json:"uri"`
	Bytes  int64  `json:"bytes"`
	SHA256 string `json:"sha256"`
}
type ServerIdentity struct {
	SchemaVersion           int                `json:"schema_version"`
	Product                 string             `json:"product"`
	Component               string             `json:"component"`
	Protocol                string             `json:"protocol"`
	Version                 string             `json:"version"`
	CompatibleBundle        string             `json:"compatible_bundle"`
	AuthorityRegistrySHA256 string             `json:"authority_registry_sha256"`
	Resources               []EmbeddedResource `json:"resources"`
	EmbeddedInputsSHA256    string             `json:"embedded_inputs_sha256"`
}

func EmbeddedDigest(resources []EmbeddedResource) (string, error) {
	body, err := CanonicalJSON(resources)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(body)), nil
}

func (i ServerIdentity) Validate() error {
	if i.SchemaVersion != 1 || i.Product != "maestro" || i.Component != "mcp_server" || i.Protocol != ServerProtocol {
		return errors.New("unsupported server identity")
	}
	if _, err := parseVersion(i.Version); err != nil {
		return err
	}
	if _, err := ParseVersionRange(i.CompatibleBundle); err != nil {
		return err
	}
	if !hashPattern.MatchString(i.AuthorityRegistrySHA256) || len(i.Resources) == 0 || len(i.Resources) > 1024 {
		return errors.New("server has no bounded release seed/inventory")
	}
	previous := ""
	for _, r := range i.Resources {
		if r.URI <= previous || len(r.URI) > 1024 || r.Bytes <= 0 || !hashPattern.MatchString(r.SHA256) {
			return errors.New("server resource identity invalid or unsorted")
		}
		previous = r.URI
	}
	digest, err := EmbeddedDigest(i.Resources)
	if err != nil || digest != i.EmbeddedInputsSHA256 {
		return errors.New("server embedded inventory digest mismatch")
	}
	return nil
}

type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	if b.Len()+len(p) > maximumManifestBytes {
		return 0, errors.New("identity output exceeds 1 MiB")
	}
	return b.Buffer.Write(p)
}

// ReadServerIdentity always executes the real protocol with bounded output and
// a finite deadline. It has no injectable healthy-check path.
func ReadServerIdentity(path string) (ServerIdentity, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, path, "identity")
	command.WaitDelay = time.Second
	var output boundedOutput
	command.Stdout = &output
	command.Stderr = io.Discard
	if err := command.Run(); err != nil {
		return ServerIdentity{}, fmt.Errorf("server identity: %w", err)
	}
	if err := rejectDuplicateJSONKeys(output.Bytes()); err != nil {
		return ServerIdentity{}, err
	}
	var identity ServerIdentity
	decoder := json.NewDecoder(bytes.NewReader(output.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&identity); err != nil {
		return identity, err
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		return identity, err
	}
	return identity, identity.Validate()
}

// DecodeStrictJSON validates one bounded JSON value, including duplicate keys.
func DecodeStrictJSON(body []byte, target any) error {
	if len(body) > maximumManifestBytes {
		return errors.New("JSON exceeds 1 MiB")
	}
	if err := rejectDuplicateJSONKeys(body); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	return ensureSingleJSONValue(decoder)
}
