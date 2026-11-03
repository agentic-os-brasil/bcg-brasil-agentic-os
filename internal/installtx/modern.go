package installtx

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/datahome"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releasecontract"
	"github.com/agentic-os-brasil/bcg-brasil-agentic-os/internal/releaseverify"
	"time"
)

// ExecutableIdentity is the authenticated modern executable binding. The
// canonical transaction normalizes these fields into its existing executable
// operations in memory; modern wire never contains historical CLI metadata.
type ExecutableIdentity struct {
	Version              string `json:"version"`
	Protocol             string `json:"protocol"`
	CompatibleBundle     string `json:"compatible_bundle"`
	CompatibleServer     string `json:"compatible_server"`
	EmbeddedInputsSHA256 string `json:"embedded_inputs_sha256"`
	ArtifactName         string `json:"artifact_name"`
	SHA256               string `json:"sha256"`
	Size                 int64  `json:"size"`
}

func validModernIdentity(profile string, server ExecutableIdentity) bool {
	if profile == "" {
		return server == (ExecutableIdentity{})
	}
	if profile != releasecontract.ModernProfile || server.Protocol != releasecontract.ServerProtocol || !versionPattern.MatchString(server.Version) || !sha256Pattern.MatchString(server.SHA256) || !sha256Pattern.MatchString(server.EmbeddedInputsSHA256) || server.Size <= 0 || server.ArtifactName == "" || filepath.Base(server.ArtifactName) != server.ArtifactName {
		return false
	}
	_, a := releasecontract.ParseVersionRange(server.CompatibleBundle)
	_, b := releasecontract.ParseVersionRange(server.CompatibleServer)
	return a == nil && b == nil
}
func serverForManifest(m releasecontract.Manifest, a releasecontract.Artifact) ExecutableIdentity {
	if !m.Modern() {
		return ExecutableIdentity{}
	}
	return ExecutableIdentity{Version: m.Server.Version, Protocol: m.Server.Protocol, CompatibleBundle: m.Server.CompatibleBundle, CompatibleServer: m.Bundle.CompatibleServer, EmbeddedInputsSHA256: m.Server.EmbeddedInputsSHA256, ArtifactName: a.Name, SHA256: a.SHA256, Size: a.Size}
}
func profileForManifest(m releasecontract.Manifest) string {
	if m.Modern() {
		return m.Delivery.Profile
	}
	return ""
}
func schemaForProfile(profile string, legacy int) int {
	if profile == releasecontract.ModernProfile {
		return 3
	}
	return legacy
}
func executableForProfile(osName, profile string) string {
	if profile == releasecontract.ModernProfile {
		if osName == "windows" {
			return "maestro-mcp.exe"
		}
		return "maestro-mcp"
	}
	return executableName(osName)
}

// Modern codecs reject mixed variants before normalization. Legacy codecs
// retain their original fields and meaning, including an absent profile.
func marshalVariant(value any, profile string) ([]byte, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if err = json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	if profile == "" {
		delete(fields, "server")
		delete(fields, "profile")
	} else {
		for _, key := range []string{"cli_version", "cli_artifact_name", "cli_sha256", "cli_size"} {
			delete(fields, key)
		}
		if v, ok := fields["from_cli_version"]; ok {
			fields["from_server_version"] = v
			delete(fields, "from_cli_version")
		}
		if v, ok := fields["staged_cli"]; ok {
			fields["staged_server"] = v
			delete(fields, "staged_cli")
		}
		if v, ok := fields["cli_backup"]; ok {
			fields["server_backup"] = v
			delete(fields, "cli_backup")
		}
		if v, ok := fields["source_cli_backup"]; ok {
			fields["source_server_backup"] = v
			delete(fields, "source_cli_backup")
		}
	}
	return json.Marshal(fields)
}
func decodeVariant(body []byte, target any) (string, ExecutableIdentity, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return "", ExecutableIdentity{}, err
	}
	var profile string
	var server ExecutableIdentity
	if v, ok := fields["profile"]; ok {
		if err := json.Unmarshal(v, &profile); err != nil {
			return "", server, err
		}
	}
	if profile == "" {
		if _, ok := fields["profile"]; ok {
			return "", server, errors.New("legacy state cannot contain profile")
		}
		if _, ok := fields["server"]; ok {
			return "", server, errors.New("legacy state cannot contain server")
		}
	} else {
		if !bytes.Equal(fields["profile"], []byte(`"mcp-local-v1"`)) {
			return "", server, errors.New("unsupported installation profile")
		}
		for _, key := range []string{"cli_version", "cli_artifact_name", "cli_sha256", "cli_size", "from_cli_version", "staged_cli", "cli_backup", "source_cli_backup"} {
			if _, ok := fields[key]; ok {
				return "", server, errors.New("modern state contains legacy CLI fields")
			}
		}
		if err := decodeStrict(fields["server"], &server); err != nil {
			return "", server, err
		}
		if !validModernIdentity(profile, server) {
			return "", server, errors.New("invalid modern server binding")
		}
		for modern, legacy := range map[string]string{"from_server_version": "from_cli_version", "staged_server": "staged_cli", "server_backup": "cli_backup", "source_server_backup": "source_cli_backup"} {
			if v, ok := fields[modern]; ok {
				fields[legacy] = v
				delete(fields, modern)
			}
		}
		fields["cli_version"], _ = json.Marshal(server.Version)
		// Only plans and receipts carry artifact fields; state/snapshot bindings
		// retain the complete typed server identity instead.
		if _, ok := fields["manifest_sha256"]; ok {
			fields["cli_sha256"], _ = json.Marshal(server.SHA256)
			fields["cli_size"], _ = json.Marshal(server.Size)
		}
		if _, ok := fields["staged_cli"]; ok {
			fields["cli_artifact_name"], _ = json.Marshal(server.ArtifactName)
		}
	}
	normalized, err := json.Marshal(fields)
	if err != nil {
		return "", server, err
	}
	return profile, server, decodeStrict(normalized, target)
}
func decodeStrict(body []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

func (v ActivationPlan) MarshalJSON() ([]byte, error) {
	type raw ActivationPlan
	return marshalVariant(raw(v), v.Profile)
}
func (v *ActivationPlan) UnmarshalJSON(body []byte) error {
	type raw ActivationPlan
	var value raw
	_, _, err := decodeVariant(body, &value)
	*v = ActivationPlan(value)
	return err
}
func (v State) MarshalJSON() ([]byte, error) {
	type raw State
	return marshalVariant(raw(v), v.Profile)
}
func (v Snapshot) MarshalJSON() ([]byte, error) {
	type raw Snapshot
	return marshalVariant(raw(v), v.Profile)
}
func (v *Snapshot) UnmarshalJSON(body []byte) error {
	type raw Snapshot
	var value raw
	_, _, err := decodeVariant(body, &value)
	*v = Snapshot(value)
	return err
}
func (v *stateWire) UnmarshalJSON(body []byte) error {
	type raw stateWire
	var value raw
	_, _, err := decodeVariant(body, &value)
	*v = stateWire(value)
	return err
}
func (v ActivationReceipt) MarshalJSON() ([]byte, error) {
	type raw ActivationReceipt
	return marshalVariant(raw(v), v.Profile)
}
func (v *ActivationReceipt) UnmarshalJSON(body []byte) error {
	type raw ActivationReceipt
	var value raw
	_, _, err := decodeVariant(body, &value)
	*v = ActivationReceipt(value)
	return err
}

func checkModernServer(path, managedRoot string, server ExecutableIdentity) error {
	if err := verifyRegularFile(path, server.Size, server.SHA256); err != nil {
		return err
	}
	identity, err := releasecontract.ReadServerIdentity(path)
	if err != nil {
		return err
	}
	if _, err := releaseverify.LoadPinnedAuthorityRegistry(filepath.Join(managedRoot, "trust", "release-authority-registry.json"), identity.AuthorityRegistrySHA256, time.Now); err != nil {
		return err
	}
	if identity.Version != server.Version || identity.Protocol != server.Protocol || identity.CompatibleBundle != server.CompatibleBundle || identity.EmbeddedInputsSHA256 != server.EmbeddedInputsSHA256 {
		return errors.New("activated server identity does not match authenticated release/independent seed")
	}
	return verifyRegularFile(path, server.Size, server.SHA256)
}

// CheckActivatedServer runs the same built-in protocol used by activation.
func CheckActivatedServer(managedRoot string, state State) error {
	if state.Profile != releasecontract.ModernProfile || !validModernIdentity(state.Profile, state.Server) {
		return errors.New("modern installed state required")
	}
	return checkModernServer(filepath.Join(managedRoot, "bin", executableForProfile(state.TargetOS, state.Profile)), managedRoot, state.Server)
}

func verifyModernPlan(plan ActivationPlan) error {
	if plan.Profile != "" {
		if err := validateModernRoots(plan.ManagedRoot, plan.DataRoot, plan.UserHome); err != nil {
			return err
		}
	} else if plan.UserHome != "" {
		return errors.New("legacy plan cannot carry modern home selection")
	}
	if !validModernIdentity(plan.Profile, plan.Server) || plan.SchemaVersion != schemaForProfile(plan.Profile, 2) {
		return errors.New("invalid activation profile/schema")
	}
	if plan.Profile != "" && (plan.CLIVersion != plan.Server.Version || plan.CLISHA256 != plan.Server.SHA256 || plan.CLISize != plan.Server.Size || plan.CLIArtifactName != plan.Server.ArtifactName) {
		return errors.New("normalized executable disagrees with typed modern identity")
	}
	if strings.Contains(plan.Profile, "/") {
		return errors.New("invalid profile")
	}
	return nil
}

func validateModernRoots(managed, data, user string) error {
	if user == "" || !filepath.IsAbs(user) || filepath.Clean(user) != user || data != filepath.Join(user, ".maestro-install") {
		return errors.New("modern transaction requires private UserHome/.maestro-install custody")
	}
	home, err := datahome.PreviewInstallation(datahome.Options{UserHome: user, ProductRoot: managed})
	if err != nil {
		return err
	}
	if within(data, home.Home) || within(home.Home, data) {
		return errors.New("installation state must remain outside admitted home")
	}
	return nil
}
